//go:build !windows

package engine

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

func publishExclusive(root *os.Root, dir, src, dst string) error {
	err := renameExclusiveRoot(root, src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.ENOSYS) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.EINVAL) {
		return err
	}
	// Older kernels/filesystems may lack exclusive rename. Hard linking remains
	// a safe fallback; an existence check followed by Rename could overwrite a
	// file created by another process between those operations.
	if err := root.Link(src, dst); err != nil {
		return err
	}
	// Publication is already complete; failure to remove a redundant private
	// temporary name must not turn a complete download into a failed record.
	_ = root.Remove(src)
	return nil
}

func renameExclusiveRoot(root *os.Root, src, dst string) error {
	from, err := root.Open(filepath.Dir(src))
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := root.Open(filepath.Dir(dst))
	if err != nil {
		return err
	}
	defer to.Close()
	if err := renameExclusiveAt(int(from.Fd()), filepath.Base(src), int(to.Fd()), filepath.Base(dst)); err != nil {
		return &os.LinkError{Op: "rename_noreplace", Old: src, New: dst, Err: err}
	}
	return nil
}
