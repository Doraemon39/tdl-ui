package safeio

import (
	"errors"
	"os"
	"path/filepath"
)

const cacheMarker = ".tdl-ui-cache-v1"

// ClaimCache refuses to treat an unrelated existing custom directory as an
// application cache. Only known default roots may adopt legacy cache contents.
func ClaimCache(dir string, allowLegacy bool) error {
	if err := CheckPath(dir); err != nil {
		return err
	}
	marker := filepath.Join(dir, cacheMarker)
	if err := CheckPath(marker); err != nil {
		return err
	}
	if st, err := os.Stat(marker); err == nil {
		if !st.Mode().IsRegular() || st.Size() != 0 {
			return errors.New("缓存目录标记无效，拒绝写入或清理")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(entries) > 0 && !allowLegacy {
		return errors.New("自定义缓存子目录已有未归属本应用的文件，请选择独立的缓存目录")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		st, err := os.Stat(marker)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() || st.Size() != 0 {
			return errors.New("缓存标记无效")
		}
		return nil
	}
	if err != nil {
		return err
	}
	return f.Close()
}
