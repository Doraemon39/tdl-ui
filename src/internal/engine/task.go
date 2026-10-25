package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
	"tdl-ui/internal/manifest"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/store"
)

var logEngine = logging.L("engine")

// Namespace 与 tdl CLI 默认命名空间保持一致，Telegram 会话数据互不干扰地存于自身 kv 目录。
const Namespace = "default"

// reconnectTimeout 客户端断线重连超时。
const reconnectTimeout = 5 * time.Minute

// 任务状态。
const (
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusPaused   = "paused"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
)

// TaskOptions 创建任务的参数。URLs 与 Selections 至少一项非空，可混用。
type TaskOptions struct {
	ManifestPath   string      `json:"manifestPath,omitempty"`
	ManifestMode   string      `json:"manifestMode,omitempty"`
	RecordManifest bool        `json:"recordManifest,omitempty"`
	AccountID      int64       `json:"accountId,omitempty"`
	URLs           []string    `json:"urls"`
	Selections     []Selection `json:"selections"`
	Label          string      `json:"label"` // 任务显示名（选集下载时由前端提供，如「频道名 × 12 个文件」）
	Dir            string      `json:"dir"`
	ScriptName     string      `json:"scriptName"`
	Template       string      `json:"template"`
	RewriteExt     bool        `json:"rewriteExt"`
	SkipSame       bool        `json:"skipSame"`
	// ConflictAction 同名处理动作：skip=跳过同名文件 / overwrite=覆盖同名文件
	ConflictAction string `json:"conflictAction,omitempty"`
	// ConflictCheck 同名比对依据：target=按最终保存文件名 / origin=按 Telegram 原始文件名
	ConflictCheck string `json:"conflictCheck,omitempty"`
	Group         bool   `json:"group"`
	Restart       bool   `json:"restart"`
	// Renames 手动重命名映射：messageId → 自定义文件名（不含扩展名），
	// 优先级高于脚本 Rename 与命名模板（FE-25）。
	TargetNames   map[string]string `json:"targetNames,omitempty"`
	OverwriteKeys map[string]bool   `json:"overwriteKeys,omitempty"`
	Renames       map[int]string    `json:"renames,omitempty"`
}

// TaskFile 任务内单个文件记录（下载中为 .tmp 临时路径，完成后为最终路径）。
type TaskFile struct {
	SHA256       string `json:"sha256,omitempty"`
	MediaID      string `json:"mediaId,omitempty"`
	AccountID    int64  `json:"accountId,omitempty"`
	OriginalName string `json:"originalName,omitempty"`
	DialogType   string `json:"dialogType,omitempty"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	// State: downloading / done / failed / skipped
	State string `json:"state"`
	// Reason 跳过原因或失败原因
	Reason string `json:"reason,omitempty"`
	// 来源对话与消息（对话媒体页"已下载"标记依赖此关联）
	DialogID  int64 `json:"dialogId,omitempty"`
	MessageID int   `json:"messageId,omitempty"`
}

// TaskView 任务视图（前端展示 / task:update 事件负载）。
type TaskView struct {
	ID         string   `json:"id"`
	URLs       []string `json:"urls"`
	Label      string   `json:"label,omitempty"`
	Dir        string   `json:"dir"`
	ScriptName string   `json:"scriptName"`
	Status     string   `json:"status"`
	Error      string   `json:"error,omitempty"`
	Notice     string   `json:"notice,omitempty"`
	Total      int      `json:"total"`
	Finished   int      `json:"finished"`
	Failed     int      `json:"failed"`
	Skipped    int      `json:"skipped"`
	FileCount  int      `json:"fileCount"`
	CreatedAt  string   `json:"createdAt"`
	// Speed 任务级聚合下载速度，字节/秒
	Speed int64 `json:"speed,omitempty"`
}

// Task 一次下载任务。
type Task struct {
	manifestMu          sync.Mutex
	completionManifests map[string]*manifest.Session
	fileIndex           map[string]int
	manifest            *manifest.Session
	manifestError       string
	ID                  string
	CreatedAt           string

	opts      TaskOptions
	contracts *script.Contracts
	// scriptSrc 创建时的脚本源码快照，恢复时优先用它重建契约（SCR-04）
	scriptSrc string
	mgr       *Manager

	mu           sync.Mutex
	status       string
	errMsg       string
	total        int
	finished     int
	failed       int
	skipped      int
	lastSkipEmit time.Time
	paused       bool // 取消是否由“暂停”触发
	removed      bool // 已从管理器移除，阻断并发 Resume 复活僵尸下载
	cancel       context.CancelFunc
	files        []TaskFile    // 任务内文件记录（含 .tmp 未完成文件）
	runDone      chan struct{} // 本次 run 结束时关闭，供删除文件前等待任务真正停止

	speedMu    sync.Mutex
	fileSpeeds map[int]int64 // fileId -> 当前速度（字节/秒）
}

// stopWait 取消运行中的任务并等待其真正退出（Windows 下句柄占用会导致删除失败）。
func (t *Task) stopWait(timeout time.Duration) bool {
	t.mu.Lock()
	if t.cancel != nil {
		t.paused = false
		t.cancel()
	}
	if t.status == StatusQueued {
		t.status = StatusCanceled
	}
	ch := t.runDone
	t.mu.Unlock()

	if ch == nil {
		return true
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// ---- Task 内部方法 ----

func (t *Task) view() TaskView {
	t.mu.Lock()
	defer t.mu.Unlock()
	var speed int64
	t.speedMu.Lock()
	for _, s := range t.fileSpeeds {
		speed += s
	}
	t.speedMu.Unlock()
	errMsg, notice := t.errMsg, ""
	if t.status == StatusDone {
		notice, errMsg = errMsg, ""
	}
	return TaskView{
		ID:         t.ID,
		URLs:       t.opts.URLs,
		Label:      t.opts.Label,
		Dir:        t.opts.Dir,
		ScriptName: t.opts.ScriptName,
		Status:     t.status,
		Error:      errMsg,
		Notice:     notice,
		Total:      t.total,
		Finished:   t.finished,
		Failed:     t.failed,
		Skipped:    t.skipped,
		FileCount:  len(t.files),
		CreatedAt:  t.CreatedAt,
		Speed:      speed,
	}
}

func (t *Task) info() scriptapi.TaskInfo {
	v := t.view()
	return scriptapi.TaskInfo{
		ID:       v.ID,
		Dir:      v.Dir,
		Total:    v.Total,
		Finished: v.Finished,
		Failed:   v.Failed,
		Status:   v.Status,
	}
}

func (t *Task) emitUpdate() {
	t.persistState()
	t.mgr.deps.Emitter.Emit(events.Task, t.view())
}

func (t *Task) emitFile(ev FileEvent) {
	// 维护任务级速度聚合缓存
	t.speedMu.Lock()
	if t.fileSpeeds == nil {
		t.fileSpeeds = make(map[int]int64)
	}
	if ev.State == "downloading" {
		t.fileSpeeds[ev.FileID] = ev.Speed
	} else {
		delete(t.fileSpeeds, ev.FileID)
	}
	t.speedMu.Unlock()

	t.mgr.deps.Emitter.Emit(events.TaskFile, ev)
}

// ---- 文件记录维护（由 progress 回调，行级写 store） ----

// addFile 登记文件（同路径覆盖，断点重跑时天然去重）。
func fileCoordinate(f TaskFile) string {
	if f.DialogID == 0 || f.MessageID == 0 {
		return ""
	}
	return "c:" + resumeKey(f.DialogID, f.MessageID, f.DialogType)
}

func (t *Task) addFile(f TaskFile) string {
	f.AccountID = t.opts.AccountID
	t.mu.Lock()
	if t.fileIndex == nil {
		t.fileIndex = make(map[string]int, len(t.files)*2+2)
		for idx, item := range t.files {
			t.fileIndex["p:"+item.Path] = idx
			if key := fileCoordinate(item); key != "" {
				t.fileIndex[key] = idx
			}
		}
	}
	idx, replaced := t.fileIndex["p:"+f.Path]
	if !replaced {
		if key := fileCoordinate(f); key != "" {
			idx, replaced = t.fileIndex[key]
		}
	}
	oldPath, oldState := "", ""
	if replaced {
		old := t.files[idx]
		oldPath, oldState = old.Path, old.State
		delete(t.fileIndex, "p:"+old.Path)
		if key := fileCoordinate(old); key != "" {
			delete(t.fileIndex, key)
		}
		t.files[idx] = f
	} else {
		idx = len(t.files)
		t.files = append(t.files, f)
	}
	t.fileIndex["p:"+f.Path] = idx
	if key := fileCoordinate(f); key != "" {
		t.fileIndex[key] = idx
	}
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		record := store.File{TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State, DialogID: f.DialogID, MessageID: f.MessageID, OriginalName: f.OriginalName, Reason: f.Reason, DialogType: f.DialogType, AccountID: f.AccountID, SHA256: f.SHA256, MediaID: f.MediaID}
		var err error
		if oldPath != "" && oldPath != f.Path {
			err = t.mgr.deps.Store.FinishFile(context.Background(), t.ID, oldPath, record)
		} else {
			err = t.mgr.deps.Store.UpsertFile(context.Background(), record)
		}
		if err != nil {
			logStoreErr(err, "写入文件记录失败: id=%s err=%v", t.ID, err)
		}
	}

	return oldState
}

// finishFile 把 oldPath（.tmp）条目替换为最终文件记录；
// 若最终路径已存在旧条目（历史运行留下）先移除，避免重复。
func (t *Task) finishFile(oldPath string, f TaskFile) {
	f.AccountID = t.opts.AccountID
	t.mu.Lock()
	kept := t.files[:0]
	for _, x := range t.files {
		if x.Path == f.Path && x.Path != oldPath {
			continue
		}
		kept = append(kept, x)
	}
	t.files = kept
	t.fileIndex = nil
	replaced := false
	for i := range t.files {
		if t.files[i].Path == oldPath {
			t.files[i] = f
			replaced = true
			break
		}
	}
	if !replaced {
		t.files = append(t.files, f)
	}
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		if err := t.mgr.deps.Store.FinishFile(context.Background(), t.ID, oldPath, store.File{
			TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State,
			DialogID: f.DialogID, MessageID: f.MessageID, OriginalName: f.OriginalName, Reason: f.Reason, DialogType: f.DialogType, AccountID: f.AccountID, SHA256: f.SHA256, MediaID: f.MediaID,
		}); err != nil {
			logStoreErr(err, "完成文件记录失败: id=%s err=%v", t.ID, err)
		}
	}
}

// markFileFailed 标记文件失败。
func (t *Task) markFileFailed(path, reason string) {
	t.mu.Lock()
	var record *TaskFile
	for i := range t.files {
		if t.files[i].Path == path {
			t.files[i].State = "failed"
			t.files[i].Reason = reason
			copy := t.files[i]
			record = &copy
			break
		}
	}
	t.mu.Unlock()
	if record != nil && t.mgr.deps.Store != nil {
		f := *record
		if err := t.mgr.deps.Store.UpsertFile(context.Background(), store.File{TaskID: t.ID, Name: f.Name, Path: f.Path, Size: f.Size, State: f.State, DialogID: f.DialogID, MessageID: f.MessageID, OriginalName: f.OriginalName, Reason: f.Reason, DialogType: f.DialogType, AccountID: f.AccountID, SHA256: f.SHA256, MediaID: f.MediaID}); err != nil {
			logStoreErr(err, "保存失败原因出错: %v", err)
		}
	}
}

// dropFile 移除文件条目（临时文件已被清理）。
func (t *Task) dropFile(path string) {
	t.mu.Lock()
	kept := t.files[:0]
	for _, x := range t.files {
		if x.Path != path {
			kept = append(kept, x)
		}
	}
	t.files = kept
	t.fileIndex = nil
	t.mu.Unlock()
	if t.mgr.deps.Store != nil {
		if err := t.mgr.deps.Store.DropFile(context.Background(), t.ID, path); err != nil {
			logStoreErr(err, "移除文件记录失败: id=%s err=%v", t.ID, err)
		}
	}
}

// onFileDone 由 progress 回调：更新计数并触发脚本钩子。
func (t *Task) onFileDone(info scriptapi.FileInfo) {
	t.mu.Lock()
	t.finished++
	t.mu.Unlock()
	t.emitUpdate()

	if err := t.contracts.SafeOnFileDone(info); err != nil {
		t.mgr.deps.Emitter.Emit(events.ScriptLog,
			fmt.Sprintf("[%s] OnFileDone 钩子出错: %v", t.ID, err))
	}
}

func (t *Task) onFileFailed() {
	t.mu.Lock()
	t.failed++
	t.mu.Unlock()
	t.emitUpdate()
}

// onFileSkipped records an outcome, never ownership of an existing disk file.
func (t *Task) onFileSkipped(f TaskFile) {
	f.Path = "skipped://" + resumeKey(f.DialogID, f.MessageID, f.DialogType)
	previous := t.addFile(f)
	t.mu.Lock()
	if previous != "skipped" {
		t.skipped++
	}
	if previous == "done" && t.finished > 0 {
		t.finished--
	}
	shouldEmit := time.Since(t.lastSkipEmit) >= defaultProgressEmitInterval
	if shouldEmit {
		t.lastSkipEmit = time.Now()
	}
	t.mu.Unlock()
	if shouldEmit {
		t.emitUpdate()
	}
}

// waitFinalization joins the last status write and task hook without holding the
// task lock. This closes the brief gap between a terminal event and worker exit.
func (t *Task) waitFinalization() error {
	t.mu.Lock()
	done := t.runDone
	active := t.status == StatusRunning || t.status == StatusQueued
	t.mu.Unlock()
	if done == nil || active {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(12 * time.Second):
		return errors.New("任务仍在收尾，请稍后重试")
	}
}
