package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"text/template"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/tmedia"
	"github.com/iyear/tdl/core/util/tutil"
	"github.com/iyear/tdl/pkg/tmessage"
	"github.com/iyear/tdl/pkg/tplfunc"
	"github.com/iyear/tdl/pkg/utils"

	"tdl-ui/internal/manifest"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
)

const tempExt = ".tmp"

// maxAlbumSize Telegram 相册最多 10 项。elem 通道容量必须 ≥ 该值（ENG-09）：
// processGrouped 会在一次 process 调用内连续向 elem 发送整组元素，
// 而消费方在 Next() 返回 true 后才逐个 Value()；配合 Next() 顶部的
// len(i.elem) > 0 检查，保证发送永不阻塞。改动三者之一需同步评审。
const maxAlbumSize = 10

// fileTemplate 命名模板的数据模型，与 tdl 保持一致。
type fileTemplate struct {
	DialogID     int64
	MessageID    int
	MessageDate  int64
	FileName     string
	FileCaption  string
	FileSize     string
	DownloadDate int64
}

// IterOptions 迭代器装配参数。
type IterOptions struct {
	CompletionRecorded func(dialogID int64, typ string, entry manifest.Entry) bool
	Disk               *diskGuard
	OnLocalDone        func(dialogID int64, typ string, entry manifest.Entry) error
	LocalMatches       map[string]manifest.LocalMatch
	RecordManifest     bool
	Manifest           *manifest.Session
	ManifestMode       string
	SingleKeys         map[string]bool
	Dir                string
	RewriteExt         bool
	SkipSame           bool
	ConflictAction     string
	ConflictCheck      string
	KnownOrigins       map[string]string
	Outputs            *outputCoordinator
	Template           string
	Group              bool
	Contracts          *script.Contracts // 可选：过滤/命名脚本契约
	TargetNames        map[string]string
	OverwriteKeys      map[string]bool
	Renames            map[int]string // 可选：手动重命名映射（messageId → 自定义文件名，不含扩展名）
	OnSkip             func(info scriptapi.FileInfo, name, reason, dialogType string)
	// OnTotalDecr 消息被跳过（无媒体/已删除/被过滤/同名同大小）时回调，
	// 供消费方递减 total，避免任务 Done 后进度仍显示 12/20（ENG-06）。
	// 注：断点命中的跳过已计入 finished，不触发此回调。
	OnTotalDecr func()
}

// iter 下载元素迭代器（对应 ref/tdl/app/dl/iter.go，去除 include/exclude，
// 由脚本 Filter 接管过滤，Rename 接管命名）。
type iter struct {
	duplicateCounts map[string]int
	messageCache    map[string]*tg.Message
	missingMessages map[string]bool
	root            *os.Root
	outputs         *outputCoordinator
	seen            map[string]bool
	discovered      int
	pool            dcpool.Pool
	manager         *peers.Manager
	dialogs         []*tmessage.Dialog
	tpl             *template.Template
	opts            IterOptions

	mu           *sync.Mutex // 保护 finished/err 与索引推进；counter/elem 仅由 Next/Value 的单 goroutine 驱动方访问
	finished     map[string]struct{}
	dialogIndex  int
	messageIndex int

	counter int
	elem    chan downloader.Elem
	err     error
}

func newIter(pool dcpool.Pool, manager *peers.Manager, dialogs []*tmessage.Dialog, opts IterOptions) (*iter, error) {
	dialogs = mergeDialogRanges(dialogs)
	tpl, err := template.New("dl").
		Funcs(tplfunc.FuncMap(tplfunc.All...)).
		Parse(opts.Template)
	if err != nil {
		return nil, errors.Wrap(err, "parse template")
	}

	if len(dialogs) == 0 {
		return nil, errors.New("至少需要一条有效的消息链接")
	}

	root, err := os.OpenRoot(opts.Dir)
	if err != nil {
		return nil, err
	}
	if opts.Outputs == nil {
		opts.Outputs = &outputCoordinator{}
	}
	if opts.ConflictAction == "" {
		if opts.SkipSame {
			opts.ConflictAction = "skip"
		} else {
			opts.ConflictAction = "overwrite"
		}
	}
	// 保持 fingerprint 稳定（与 tdl 相同的排序规则）
	sortDialogs(dialogs)

	return &iter{
		root: root, outputs: opts.Outputs, seen: make(map[string]bool),
		pool:    pool,
		manager: manager,
		dialogs: dialogs,
		tpl:     tpl,
		opts:    opts,

		mu:       &sync.Mutex{},
		finished: make(map[string]struct{}),
		counter:  -1,
		elem:     make(chan downloader.Elem, maxAlbumSize), // 分组消息缓冲（ENG-09 不变量）
	}, nil
}

func mergeDialogRanges(dialogs []*tmessage.Dialog) []*tmessage.Dialog {
	out := []*tmessage.Dialog{}
	index := map[string]*tmessage.Dialog{}
	seen := map[string]map[int]bool{}
	for _, d := range dialogs {
		key := fmt.Sprintf("%T:%d", d.Peer, tutil.GetInputPeerID(d.Peer))
		target := index[key]
		if target == nil {
			clone := *d
			clone.Messages = nil
			target = &clone
			index[key] = target
			seen[key] = map[int]bool{}
			out = append(out, target)
		}
		for _, id := range d.Messages {
			if !seen[key][id] {
				seen[key][id] = true
				target.Messages = append(target.Messages, id)
			}
		}
	}
	return out
}

func (i *iter) Next(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		i.setErr(ctx.Err()) // ENG-07：与 Err()/process 统一持锁读写
		return false
	default:
	}

	if len(i.elem) > 0 { // 分组消息尚未消费完
		return true
	}

	for {
		if err := ctx.Err(); err != nil {
			i.setErr(err)
			return false
		}
		ok, skip := i.process(ctx)
		if skip {
			continue
		}
		return ok
	}
}

func (i *iter) process(ctx context.Context) (ret bool, skip bool) {
	// ENG-08：锁内仅做终止判断与索引快照/推进，网络与磁盘 I/O 全部在锁外执行，
	// 避免 worker 的 Finish/Finished（断点持久化）被 I/O 长时间阻塞。
	i.mu.Lock()
	if i.dialogIndex >= len(i.dialogs) || i.messageIndex >= len(i.dialogs[i.dialogIndex].Messages) || i.err != nil {
		i.mu.Unlock()
		return false, false
	}
	peer, msg := i.dialogs[i.dialogIndex].Peer, i.dialogs[i.dialogIndex].Messages[i.messageIndex]
	var batchIDs []int
	if i.opts.Manifest != nil || i.opts.ConflictAction == "smart" {
		source := i.dialogs[i.dialogIndex].Messages
		batchIDs = append([]int(nil), source[i.messageIndex:min(len(source), i.messageIndex+100)]...)
	}
	if i.messageIndex++; i.messageIndex >= len(i.dialogs[i.dialogIndex].Messages) {
		i.dialogIndex++
		i.messageIndex = 0
	}
	i.mu.Unlock()

	from, err := i.manager.FromInputPeer(ctx, peer)
	if err != nil {
		i.setErr(errors.Wrap(err, "resolve from input peer"))
		return false, false
	}
	key := resumeKey(from.ID(), msg, peerDialogType(from))
	if i.seen[key] {
		return false, true
	}
	var message *tg.Message
	if i.opts.Manifest != nil || i.opts.ConflictAction == "smart" {
		message, err = i.manifestMessage(ctx, from, msg, batchIDs)
	} else {
		message, err = tutil.GetSingleMessage(ctx, i.pool.Default(ctx), peer, msg)
	}
	if err != nil {
		if errors.Is(err, tutil.ErrMessageDeleted) { // 已删除消息直接跳过
			i.decrTotal()
			return false, true
		}
		i.setErr(errors.Wrap(err, "resolve message"))
		return false, false
	}

	if _, ok := message.GetGroupedID(); ok && i.opts.Group && !i.opts.SingleKeys[resumeKey(from.ID(), message.ID, peerDialogType(from))] {
		return i.processGrouped(ctx, message, from)
	}

	return i.processSingle(ctx, message, from, false)
}

// resumeKey 断点坐标：内容坐标 "dialogID:messageID"。
func resumeKey(dialogID int64, messageID int, typ ...string) string {
	key := fmt.Sprintf("%d:%d", dialogID, messageID)
	if len(typ) > 0 && PeerNamespace(typ[0]) != "" {
		return PeerNamespace(typ[0]) + ":" + key
	}
	return key
}

// processSingle 处理单条消息；inGroup 表示来自相册展开（整组在 total 中仅占 1 个名额，
// 组内跳过不单独递减 total，由 processGrouped 整组结算）。
func (i *iter) processSingle(ctx context.Context, message *tg.Message, from peers.Peer, inGroup bool) (bool, bool) {
	key := resumeKey(from.ID(), message.ID, peerDialogType(from))
	if i.seen[key] {
		return false, true
	}
	i.seen[key] = true
	item, ok := tmedia.GetMedia(message)
	if !ok { // 无媒体消息
		if !inGroup {
			i.decrTotal()
		}
		return false, true
	}

	i.discovered++
	info := scriptapi.FileInfo{
		DialogID:    from.ID(),
		MessageID:   message.ID,
		MessageDate: int64(message.Date),
		FileName:    item.Name,
		FileCaption: message.Message,
		FileSize:    item.Size,
	}
	if i.isFinished(key) {
		// A previous run may have published the file before its journal write
		// failed. Repair that record before treating the resume key as settled.
		if match, ok := i.opts.LocalMatches[key]; ok && match.Entry.MediaID == MediaIdentity(message) && match.Entry.Name == item.Name && match.Entry.Size == item.Size {
			entry := localCompletionEntry(info, match)
			typ := peerDialogType(from)
			if i.opts.Manifest.HasCompletion(info.DialogID, typ, entry) || i.opts.CompletionRecorded != nil && i.opts.CompletionRecorded(info.DialogID, typ, entry) {
				return false, true
			}
			if manifest.VerifyDigestContext(ctx, i.root, match.Path, item.Size, match.Entry.SHA256) {
				if err := i.recordLocalDone(info, typ, match); err != nil {
					i.setErr(err)
					return false, false
				}
			} else if err := ctx.Err(); err != nil {
				i.setErr(err)
				return false, false
			}
		}
		return false, true
	}

	if i.opts.Manifest != nil && i.opts.Manifest.ShouldSkip(from.ID(), peerDialogType(from), message.ID, MediaIdentity(message), item.Size, i.opts.ManifestMode) {
		i.notifySkip(info, info.FileName, "已在导入清单中，按清单跳过", peerDialogType(from))
		return false, true
	}
	if i.opts.ConflictAction == "smart" {
		if match, ok := i.opts.LocalMatches[key]; ok && match.Entry.MediaID == MediaIdentity(message) && match.Entry.Name == item.Name && match.Entry.Size == item.Size && manifest.VerifyDigestContext(ctx, i.root, match.Path, item.Size, match.Entry.SHA256) {
			if err := i.recordLocalDone(info, peerDialogType(from), match); err != nil {
				i.setErr(err)
				return false, false
			}
			i.notifySkip(info, info.FileName, "本地已有：原名、大小及来源比对通过", peerDialogType(from))
			return false, true
		}
		if err := ctx.Err(); err != nil {
			i.setErr(err)
			return false, false
		}
	}
	// 脚本过滤：返回 false 则跳过该文件
	if keep, err := i.opts.Contracts.SafeFilter(info); err != nil {
		logEngine.Warnf("过滤脚本出错，默认保留: %v", err)
	} else if !keep {
		i.notifySkip(info, info.FileName, "被过滤脚本跳过", peerDialogType(from))
		return false, true
	}

	// 命名：手动重命名 > 脚本 Rename > 默认模板
	name := i.opts.TargetNames[key]
	// 1. 手动重命名（用户指定，最高优先级）
	if customName, ok := i.opts.Renames[info.MessageID]; name == "" && ok && customName != "" {
		ext := filepath.Ext(info.FileName)
		name = customName + ext
	}
	// 2. 脚本 Rename
	if name == "" && i.opts.Contracts.HasRename() {
		if n, err := i.opts.Contracts.SafeRename(info); err != nil {
			logEngine.Warnf("命名脚本出错，回退默认模板: %v", err)
		} else {
			name = n
		}
	}
	if name == "" {
		toName := bytes.Buffer{}
		if err := i.tpl.Execute(&toName, &fileTemplate{
			DialogID:     info.DialogID,
			MessageID:    info.MessageID,
			MessageDate:  info.MessageDate,
			FileName:     info.FileName,
			FileCaption:  info.FileCaption,
			FileSize:     utils.Byte.FormatBinaryBytes(info.FileSize),
			DownloadDate: time.Now().Unix(),
		}); err != nil {
			i.setErr(errors.Wrap(err, "execute template"))
			return false, false
		}
		name = toName.String()
	}

	name, err := cleanOutputName(name)
	if err != nil {
		i.setErr(err)
		return false, false
	}
	action := i.opts.ConflictAction
	if action == "overwrite" && i.opts.OverwriteKeys != nil && !i.opts.OverwriteKeys[key] {
		action = "rename"
	}
	to, name, err := i.allocate(name, info.FileName, action)
	if errors.Is(err, errOutputSkipped) {
		i.notifySkip(info, nameOrOriginal(name, info.FileName), "本地或队列中已存在同名文件", peerDialogType(from))
		return false, true
	}
	if err != nil {
		i.setErr(errors.Wrap(err, "准备目标文件失败"))
		return false, false
	}

	i.counter++
	i.elem <- &iterElem{
		id:        i.counter,
		resumeKey: resumeKey(from.ID(), message.ID, peerDialogType(from)),

		from:    from,
		fromMsg: message,
		file:    item,
		info:    info,

		to: to, targetName: name, outputAction: action,
		disk: i.opts.Disk,
	}

	return true, false
}

func localCompletionEntry(info scriptapi.FileInfo, match manifest.LocalMatch) manifest.Entry {
	entry := match.Entry
	entry.Date = info.MessageDate
	entry.SavedName = filepath.Base(match.Path)
	entry.Evidence = "source"
	return entry
}

func (i *iter) recordLocalDone(info scriptapi.FileInfo, typ string, match manifest.LocalMatch) error {
	entry := localCompletionEntry(info, match)
	if i.opts.OnLocalDone != nil {
		return i.opts.OnLocalDone(info.DialogID, typ, entry)
	}
	if i.opts.Manifest != nil && i.opts.RecordManifest && i.opts.Manifest.MatchesSource(info.DialogID, typ) {
		return i.opts.Manifest.MarkDone(entry)
	}
	return nil
}

// The upstream downloader returns early on Iter.Err without joining workers.
// Defer that error until Download has called wg.Wait, so completion journals and
// file handles are fully settled before task finalization or iterator cleanup.
type settlingIter struct{ downloader.Iter }

func (i settlingIter) Err() error { return nil }

func (i *iter) processGrouped(ctx context.Context, message *tg.Message, from peers.Peer) (bool, bool) {
	grouped, err := tutil.GetGroupedMessages(ctx, i.pool.Default(ctx), from.InputPeer(), message)
	if err != nil {
		i.setErr(errors.Wrapf(err, "resolve grouped message %d/%d", from.ID(), message.ID))
		return false, false
	}

	hasValid := false
	for _, msg := range grouped {
		ret, skip := i.processSingle(ctx, msg, from, true)
		if !ret && !skip {
			return false, false
		}
		if ret {
			hasValid = true
		}
	}
	return hasValid, !hasValid
}

func nameOrOriginal(name, original string) string {
	if name != "" {
		return name
	}
	return original
}

func (i *iter) notifySkip(info scriptapi.FileInfo, name, reason, dialogType string) {
	if i.opts.OnSkip != nil {
		i.opts.OnSkip(info, name, reason, dialogType)
	}
}

// decrTotal 通知消费方一条消息被跳过、不再计入 total（ENG-06）。
func (i *iter) decrTotal() {
	if i.opts.OnTotalDecr != nil {
		i.opts.OnTotalDecr()
	}
}

// setErr / Err / isFinished 统一持锁访问共享字段（ENG-07）。
func (i *iter) setErr(err error) {
	i.mu.Lock()
	i.err = err
	i.mu.Unlock()
}

func (i *iter) isFinished(key string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	_, ok := i.finished[key]
	return ok
}

func (i *iter) Value() downloader.Elem { return <-i.elem }

// Close 排空 elem 缓冲通道，关闭滞留元素的文件句柄并删除对应 .tmp 文件。
// 暂停/取消时 downloader 已停止消费，通道内未被消费的元素若不清理，
// fd 会泄漏到进程退出，孤儿 .tmp 也因未登记而无法被删除流程覆盖。
func (i *iter) Close() {
	defer i.root.Close()
	for {
		select {
		case e := <-i.elem:
			if el, ok := e.(*iterElem); ok && el.to != nil {
				name := el.to.Name()
				_ = el.to.Close()
				rel, err := filepath.Rel(i.opts.Dir, name)
				if err == nil {
					_ = i.root.Remove(rel)
				}
				i.release(el.targetName)
			}
		default:
			return
		}
	}
}

func (i *iter) Err() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.err
}

func (i *iter) SetFinished(finished map[string]struct{}) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.finished = finished
}

// Finished 返回断点集合的拷贝，避免消费方在不持锁的情况下与 Finish 并发迭代/写同一 map。
func (i *iter) Finished() map[string]struct{} {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make(map[string]struct{}, len(i.finished))
	for k := range i.finished {
		out[k] = struct{}{}
	}
	return out
}

func (i *iter) Finish(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.finished[key] = struct{}{}
}

func (i *iter) Total() int {
	i.mu.Lock()
	defer i.mu.Unlock()

	total := 0
	for _, m := range i.dialogs {
		total += len(m.Messages)
	}
	return total
}

func sortDialogs(dialogs []*tmessage.Dialog) {
	sort.Slice(dialogs, func(i, j int) bool {
		return tutil.GetInputPeerID(dialogs[i].Peer) <
			tutil.GetInputPeerID(dialogs[j].Peer)
	})

	for _, m := range dialogs {
		sort.Ints(m.Messages)
	}
}

func peerDialogType(p peers.Peer) string {
	switch p.(type) {
	case peers.User:
		return DialogPrivate
	case peers.Chat:
		return DialogGroup
	case peers.Channel:
		return DialogChannel
	default:
		return ""
	}
}
