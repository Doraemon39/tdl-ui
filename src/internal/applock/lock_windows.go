// Package applock ensures one process owns a data directory, including cleanup.
package applock

import (
	"context"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"tdl-ui/internal/safeio"
)

type Guard struct {
	handle     windows.Handle
	overlapped windows.Overlapped
}

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
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	if err := safeio.CheckPath(path); err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	g := &Guard{handle: h}
	for {
		err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &g.overlapped)
		if err == nil {
			return g, nil
		}
		if err != windows.ERROR_LOCK_VIOLATION {
			windows.CloseHandle(h)
			return nil, err
		}
		select {
		case <-ctx.Done():
			windows.CloseHandle(h)
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (g *Guard) Close() error {
	_ = windows.UnlockFileEx(g.handle, 0, 1, 0, &g.overlapped)
	return windows.CloseHandle(g.handle)
}
