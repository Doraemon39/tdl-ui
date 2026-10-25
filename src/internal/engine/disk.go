package engine

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

type diskSpace struct {
	total, free, available uint64
}

type diskLimitError struct {
	dir   string
	limit int
}

func (e *diskLimitError) Error() string {
	return fmt.Sprintf("下载目录所在磁盘已达到 %d%% 使用率上限，或下一次写入将超过上限，已停止下载。清理磁盘空间后，可导入清单并重新选择原下载范围，继续剩余文件。目录：%s", e.limit, e.dir)
}

// Share the check/write lock across tasks, including tasks targeting different
// directories on the same volume. Network downloads remain concurrent.
var diskWriteMu sync.Mutex

type diskGuard struct {
	dir    string
	limit  int
	ctx    context.Context
	cancel context.CancelFunc
	space  func(string) (diskSpace, error)
	mu     sync.Mutex
	err    error
}

func (g *diskGuard) Err() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.err
}

func (g *diskGuard) check(bytes uint64) error {
	if err := g.Err(); err != nil {
		return err
	}
	if err := g.ctx.Err(); err != nil {
		return err
	}
	s, err := g.space(g.dir)
	if err == nil {
		if s.total == 0 || s.free > s.total {
			err = fmt.Errorf("磁盘容量信息无效：%s", g.dir)
		} else {
			// Integer arithmetic avoids rounding and multiplication overflow.
			ceiling := s.total/100*uint64(g.limit) + s.total%100*uint64(g.limit)/100
			used := s.total - s.free
			if used >= ceiling || bytes > ceiling-used || bytes > s.available {
				err = &diskLimitError{dir: g.dir, limit: g.limit}
			}
		}
	} else {
		err = fmt.Errorf("无法检查下载目录所在磁盘的空间：%w", err)
	}
	if err != nil {
		g.mu.Lock()
		if g.ctx.Err() != nil && g.err == nil {
			g.mu.Unlock()
			return g.ctx.Err()
		}
		if g.err == nil {
			g.err = err
		}
		g.mu.Unlock()
		g.cancel()
	}
	return err
}

// Watch even while the download is waiting on the network: another program may
// consume the free space. The caller joins this goroutine before finalization.
func (g *diskGuard) watch() func() {
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-g.ctx.Done():
				return
			case <-ticker.C:
				if g.check(0) != nil {
					return
				}
			}
		}
	}()
	return func() { close(stop); <-done }
}

type diskWriter struct {
	file  *os.File
	guard *diskGuard
}

func (w diskWriter) WriteAt(p []byte, off int64) (int, error) {
	diskWriteMu.Lock()
	defer diskWriteMu.Unlock()
	if err := w.guard.ctx.Err(); err != nil {
		return 0, err
	}
	// Parallel parts can extend a non-sparse file far beyond this part's size.
	// Account for that extension before writing, rather than after filling disk.
	st, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	growth := max(int64(len(p)), off+int64(len(p))-st.Size())
	if err := w.guard.check(uint64(growth)); err != nil {
		return 0, context.Canceled
	}
	return w.file.WriteAt(p, off)
}
