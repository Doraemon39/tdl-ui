package engine

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

// MoveFileEx without REPLACE_EXISTING atomically refuses a raced-in file and
// works on Windows filesystems which do not support hard links.
func publishExclusive(root *os.Root, dir, src, dst string) error {
	from, err := windows.UTF16PtrFromString(filepath.Join(dir, src))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(dir, dst))
	if err != nil {
		return err
	}
	err = windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
	if err == windows.ERROR_ALREADY_EXISTS || err == windows.ERROR_FILE_EXISTS {
		return &os.PathError{Op: "publish", Path: dst, Err: os.ErrExist}
	}
	return err
}
