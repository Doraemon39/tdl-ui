//go:build !windows && !linux && !darwin

package engine

import "syscall"

func renameExclusiveAt(from int, src string, to int, dst string) error {
	return syscall.ENOSYS
}
