package engine

import "golang.org/x/sys/unix"

func renameExclusiveAt(from int, src string, to int, dst string) error {
	return unix.Renameat2(from, src, to, dst, unix.RENAME_NOREPLACE)
}
