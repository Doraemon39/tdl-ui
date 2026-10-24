//go:build !windows

package applock

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"tdl-ui/internal/safeio"
	"time"
)

type Guard struct{ file *os.File }

func Acquire(ctx context.Context, dir string) (*Guard, error) {
	if err := safeio.CheckPath(dir); err != nil {
		return nil, err
	}
	if err := safeio.MkdirPrivate(dir); err != nil {
		return nil, err
	}
	return AcquireFile(ctx, filepath.Join(dir, ".instance.lock"))
}

func AcquireFile(ctx context.Context, path string) (*Guard, error) {
	if err := safeio.CheckPath(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return &Guard{f}, nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (g *Guard) Close() error { return g.file.Close() }
