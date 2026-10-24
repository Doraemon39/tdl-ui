//go:build !windows

package safeio

import "os"

func reparsePoint(info os.FileInfo) bool { return false }
func mkdirPrivate(dir string) error      { return os.Mkdir(dir, 0o700) }
