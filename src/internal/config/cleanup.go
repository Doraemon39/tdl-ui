package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
	"tdl-ui/internal/safeio"
)

const pendingCleanupName = ".pending-cleanup.json"

type CleanupSpec struct {
	Action  string   `json:"action"`
	DataDir string   `json:"dataDir"`
	Targets []string `json:"targets"`
}

var ownedData = []string{"kv", "scripts", "cache", "tmp", "logs", "webview", "config.yaml", "settings.json", "settings.json.bak", "tasks.db", "tasks.db-wal", "tasks.db-shm", "tasks.json", "tasks.json.imported", "logging.yaml", "logging.yaml.imported"}

func HasPendingCleanup(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, pendingCleanupName))
	return err == nil
}

func (m *Manager) downloadPaths(s Settings) ([]string, error) {
	paths := []string{s.DownloadDir}
	// Read-only: cleanup must also preserve historical destinations, even after
	// the user changed the default directory or restarted the application.
	dbPath := filepath.Join(m.dataDir, "tasks.db")
	if _, err := os.Lstat(dbPath); os.IsNotExist(err) {
		return paths, nil
	} else if err != nil {
		return nil, err
	}
	if err := safeio.CheckPath(dbPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, "SELECT dir FROM tasks")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dir string
		if err := rows.Scan(&dir); err != nil {
			return nil, err
		}
		if dir != "" {
			paths = append(paths, dir)
		}
	}
	return paths, rows.Err()
}

func (m *Manager) CleanupPlan(action string) (CleanupSpec, error) {
	plan := CleanupSpec{Action: action, DataDir: m.dataDir, Targets: []string{}}
	if action == "direct" {
		return plan, nil
	}
	if action != "clean_cache" && action != "wipe_all" {
		return plan, errors.New("退出清理动作无效")
	}
	if err := safeio.CheckPath(m.dataDir); err != nil {
		return plan, err
	}
	if filepath.Dir(m.dataDir) == m.dataDir {
		return plan, errors.New("数据目录不能是磁盘根目录")
	}
	s := m.Get()
	downloads, err := m.downloadPaths(s)
	if err != nil {
		return plan, err
	}
	if action == "wipe_all" {
		for _, custom := range []struct{ path, standard string }{{s.CacheDir, "cache"}, {s.TempDir, "tmp"}, {s.Log.Dir, "logs"}} {
			if custom.path != "" && safeio.Overlap(m.dataDir, custom.path) && !safeio.Equal(custom.path, filepath.Join(m.dataDir, custom.standard)) {
				return plan, errors.New("自定义目录与应用数据目录重叠，请先使用独立目录，再重置应用")
			}
		}
		for _, dir := range downloads {
			if safeio.Overlap(m.dataDir, dir) {
				return plan, errors.New("下载目录与应用数据目录重叠，拒绝清除应用数据")
			}
		}
		plan.Targets = append(plan.Targets, ownedData...)
	} else {
		// Exit cleanup is deliberately confined to the default media cache.
		if s.CacheDir == "" || safeio.Equal(s.CacheDir, filepath.Join(m.dataDir, "cache")) {
			plan.Targets = append(plan.Targets, filepath.Join("cache", "thumbs"), filepath.Join("cache", "previews"))
		}
		if s.TempDir == "" || safeio.Equal(s.TempDir, filepath.Join(m.dataDir, "tmp")) {
			plan.Targets = append(plan.Targets, filepath.Join("tmp", "video"))
		}
	}
	for _, name := range plan.Targets {
		target := filepath.Join(m.dataDir, name)
		if err := safeio.CheckPath(target); err != nil {
			return plan, err
		}
		for _, dir := range downloads {
			if safeio.Overlap(target, dir) {
				return plan, errors.New("清理范围与下载目录重叠，已拒绝")
			}
		}
	}
	return plan, nil
}

func (m *Manager) ScheduleCleanup(action string) error {
	plan, err := m.CleanupPlan(action)
	if err != nil || action == "direct" {
		return err
	}
	b, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	return safeio.WriteFile(filepath.Join(m.dataDir, pendingCleanupName), b, 0o600)
}

func (m *Manager) Cleanup(action string) error {
	plan, err := m.CleanupPlan(action)
	if err != nil {
		return err
	}
	// All paths are checked before the first mutation. Unknown files, custom
	// directories and downloaded media are never part of this allowlist.
	for _, name := range plan.Targets {
		if err := safeio.RemoveChild(m.dataDir, name); err != nil {
			return err
		}
	}
	err = os.Remove(filepath.Join(m.dataDir, pendingCleanupName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (m *Manager) resumeCleanup() (bool, error) {
	path := filepath.Join(m.dataDir, pendingCleanupName)
	if !HasPendingCleanup(m.dataDir) {
		return false, nil
	}
	if err := safeio.CheckPath(path); err != nil {
		return false, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var stored CleanupSpec
	if err := json.Unmarshal(b, &stored); err != nil {
		return false, err
	}
	if !safeio.Equal(stored.DataDir, m.dataDir) {
		return false, errors.New("退出清理记录目录不匹配")
	}
	// Never execute targets supplied by a file or command line; reconstruct the
	// allowlist from the validated action and current config every time.
	if err := m.Cleanup(stored.Action); err != nil {
		return false, err
	}
	return stored.Action == "wipe_all", nil
}

// RunPendingCleanup is used by the short-lived helper after it obtains the
// exclusive application lock. It never starts a WebView or a Telegram client.
func RunPendingCleanup(dir string) error {
	if err := safeio.CheckPath(dir); err != nil {
		return err
	}
	m := &Manager{dataDir: dir, settings: defaultSettings(dir)}
	if b, err := os.ReadFile(m.configPath()); err == nil {
		if err := m.applyYAML(b); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	_, err := m.resumeCleanup()
	return err
}

func (m *Manager) ValidateMediaCacheClear() error {
	s := m.Get()
	return m.validateCachePaths(s)
}

func (m *Manager) validateCachePaths(s Settings) error {
	downloads, err := m.downloadPaths(s)
	if err != nil {
		return err
	}
	cache, temp := s.CacheDir, s.TempDir
	if cache == "" {
		cache = filepath.Join(m.dataDir, "cache")
	}
	if temp == "" {
		temp = filepath.Join(m.dataDir, "tmp")
	}
	for _, target := range []string{filepath.Join(cache, "thumbs"), filepath.Join(cache, "previews"), filepath.Join(temp, "video")} {
		if err := safeio.CheckPath(target); err != nil {
			return err
		}
		for _, dir := range downloads {
			if safeio.Overlap(target, dir) {
				return errors.New("缓存清理范围与下载目录重叠")
			}
		}
		for _, name := range []string{"kv", "scripts", "webview", "config.yaml", "tasks.db", "logs"} {
			if safeio.Overlap(target, filepath.Join(m.dataDir, name)) {
				return errors.New("缓存清理范围与应用重要数据重叠")
			}
		}
	}
	return nil
}
