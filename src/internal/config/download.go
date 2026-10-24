package config

import (
	"errors"
	"path/filepath"
	"tdl-ui/internal/safeio"
)

func (m *Manager) ValidateDownloadDir(dir string) error {
	if err := safeio.CheckPath(dir); err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	s := m.Get()
	cache, temp := s.CacheDir, s.TempDir
	if cache == "" {
		cache = filepath.Join(m.dataDir, "cache")
	}
	if temp == "" {
		temp = filepath.Join(m.dataDir, "tmp")
	}
	protected := []string{filepath.Join(cache, "thumbs"), filepath.Join(cache, "previews"), filepath.Join(temp, "video"), s.Log.Dir}
	for _, name := range []string{"kv", "scripts", "webview", "config.yaml", "tasks.db"} {
		protected = append(protected, filepath.Join(m.dataDir, name))
	}
	for _, path := range protected {
		if path != "" && safeio.Overlap(dir, path) {
			return errors.New("下载目录与应用会话、缓存或日志目录重叠，请选择独立目录")
		}
	}
	return nil
}
