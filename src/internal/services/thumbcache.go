package services

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"tdl-ui/internal/safeio"
)

// 缓存种类（对应语义化子目录）。
const (
	cacheKindThumb   = "thumbs"   // 清晰小图
	cacheKindPreview = "previews" // 预览大图
)

// thumbCache 缩略图磁盘缓存：缓存根由 rootFn 每次求值，配置变更即时生效。
// 文件名 <dialogID>_<messageID>.jpg 全局唯一，天然防重复防覆盖。
type thumbCache struct {
	rootFn  func() string
	trusted func(string) bool

	mu       sync.Mutex
	epoch    uint64                   // 清空代际号：Clear 时递增，使在途写盘自我作废（LOGIC-002）
	inflight map[string]chan struct{} // 进程内 singleflight，防并发重复拉取
}

// newThumbCache 缓存根由 rootFn 注入（通常为 config.Manager.CacheDir）。
func newThumbCache(rootFn func() string, trusted ...func(string) bool) *thumbCache {
	check := func(string) bool { return true }
	if len(trusted) > 0 {
		check = trusted[0]
	}
	return &thumbCache{
		rootFn:   rootFn,
		trusted:  check,
		inflight: make(map[string]chan struct{}),
	}
}

// path 缓存文件路径：<root>/<kind>/<dialogID>_<messageID>.jpg。
func (c *thumbCache) path(kind string, dialogID int64, messageID int) string {
	name := strconv.FormatInt(dialogID, 10) + "_" + strconv.Itoa(messageID) + ".jpg"
	return filepath.Join(c.rootFn(), kind, name)
}

// Get 命中磁盘缓存直接读；未命中经 fetch 拉取并原子落盘。
// 同一 key 的并发请求仅触发一次拉取，其余等待后重读缓存。
func (c *thumbCache) Get(kind string, dialogID int64, messageID int, fetch func() ([]byte, error)) ([]byte, error) {
	if err := c.ensureKind(kind); err != nil {
		return nil, err
	}
	p := c.path(kind, dialogID, messageID)
	for {
		if b, err := readCacheFile(p, 32<<20); err == nil {
			return b, nil
		}

		c.mu.Lock()
		if ch, ok := c.inflight[p]; ok {
			c.mu.Unlock()
			<-ch // 等拉取方结束后重读；若其失败则下一轮自己成为拉取方
			continue
		}
		// A writer may have completed after our first read but before we took
		// the lock. Do not start a second fetch after its inflight entry is gone.
		if b, err := readCacheFile(p, 32<<20); err == nil {
			c.mu.Unlock()
			return b, nil
		}
		ch := make(chan struct{})
		c.inflight[p] = ch
		e := c.epoch // LOGIC-002：快照清空代际，写盘前校验
		c.mu.Unlock()

		b, err := fetch()

		c.mu.Lock()
		if err == nil && len(b) > 0 {
			if c.epoch == e {
				// 写盘留在锁内：与 Clear 互斥，杜绝清空后幽灵文件与
				// Windows 下 RemoveAll 撞上 rename 的 Access is denied（LOGIC-002）。
				err = writeFileAtomic(p, b)
			}
			// 代际已变：本次拉取先于 Clear 发起，跳过落盘仅内存返回
		}
		delete(c.inflight, p)
		c.mu.Unlock()
		close(ch)

		if err != nil {
			return nil, err
		}
		return b, nil
	}
}

// Clear 在锁保护下清空 thumbs/previews 两个子目录后重建（不递归删根目录）。
// 持锁可阻止新拉取在清理期间启动；同时递增 epoch 使在途拉取的写盘自我作废，
// 写盘也持锁校验代际，二者互斥（SVC-18 + LOGIC-002）。
func (c *thumbCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++ // 作废所有清空前发起的在途写盘
	root := c.rootFn()
	for _, kind := range []string{cacheKindThumb, cacheKindPreview} {
		if err := c.ensureKind(kind); err != nil {
			return err
		}
		sub := filepath.Join(root, kind)
		if err := safeio.RemoveChild(root, kind); err != nil {
			return err
		}
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return err
		}
		if err := c.ensureKind(kind); err != nil {
			return err
		}
	}
	return nil
}

func (c *thumbCache) ensureKind(kind string) error {
	top := strings.Split(filepath.Clean(kind), string(filepath.Separator))[0]
	if top != cacheKindThumb && top != cacheKindPreview {
		return errors.New("缓存种类无效")
	}
	dir := filepath.Join(c.rootFn(), top)
	return safeio.ClaimCache(dir, c.trusted == nil || c.trusted(dir))
}

// writeFileAtomic tmp + rename 原子写，避免读到半截文件。
func writeFileAtomic(path string, b []byte) error {
	if err := safeio.CheckPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return safeio.WriteFile(path, b, 0o600)
}

// isConcurrentWriteErr 判定 rename 失败是否属于"并发写入者已占据目标"类错误：
// fs.ErrExist（EEXIST）或 Windows 的共享冲突/拒绝访问（文件被同 key 写入方占用）。
func isConcurrentWriteErr(err error) bool {
	if errors.Is(err, fs.ErrExist) {
		return true
	}
	if runtime.GOOS == "windows" {
		// ERROR_SHARING_VIOLATION(32) / ERROR_ACCESS_DENIED(5)：
		// 并发 rename 到同名目标或目标被读取方短暂占用的典型错误码
		var errno syscall.Errno
		if errors.As(err, &errno) {
			return errno == 32 || errno == 5
		}
	}
	return false
}

// InvalidateWrites prevents fetches started before shutdown from recreating
// cache files after a confirmed cleanup. It keeps existing cache files intact.
func (c *thumbCache) InvalidateWrites() { c.mu.Lock(); c.epoch++; c.mu.Unlock() }

func readCacheFile(path string, limit int64) ([]byte, error) {
	if err := safeio.CheckPath(path); err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("缓存文件类型或大小无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("缓存文件过大")
	}
	return b, nil
}
