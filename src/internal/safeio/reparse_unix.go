//go:build !windows

package safeio

import "os"

func reparsePoint(path string, info os.FileInfo) bool {
	return info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}
func mkdirPrivate(dir string) error { return os.Mkdir(dir, 0o700) }
