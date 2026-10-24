package services

import (
	"github.com/go-faster/errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"tdl-ui/internal/config"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
)

var logSettings = logging.L("settings")

// SettingsService 应用设置读写。
type SettingsService struct {
	cfg     *config.Manager
	emitter *events.Emitter
	quit    func(string) error
	// clearThumbCache 缓存清理委托（SVC-18：经 thumbCache 在锁保护下执行）
	clearThumbCache func() error
}

// NewSettingsService creates the bound service with optional desktop callbacks.
func NewSettingsService(cfg *config.Manager, clearThumbCache func() error, emitter *events.Emitter, quit func(string) error) *SettingsService {
	return &SettingsService{cfg: cfg, clearThumbCache: clearThumbCache, emitter: emitter, quit: quit}
}
func (s *SettingsService) IsPortable() bool { return s.cfg.IsPortable() }
func (s *SettingsService) OpenDataDir() error {
	return openDirectory(s.cfg.DataDir())
}
func (s *SettingsService) QuitApp(action string, remember bool) error {
	switch action {
	case "direct", "clean_cache", "wipe_all":
	default:
		return errors.New("退出动作无效")
	}
	if action == "wipe_all" && remember {
		return errors.New("清除应用数据必须每次确认")
	}
	if s.quit == nil {
		return errors.New("应用尚未就绪")
	}
	if _, err := s.cfg.CleanupPlan(action); err != nil {
		return err
	}
	if action == "wipe_all" {
		ctx := s.emitter.Ctx()
		if ctx == nil {
			return errors.New("应用尚未就绪")
		}
		button, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.WarningDialog, Title: "确认清除应用数据", Message: "将清除登录会话、配置、任务记录、脚本及默认缓存。已下载文件、自定义目录和未知文件保留。确认清除并退出？", Buttons: []string{"取消", "清除并退出"}, DefaultButton: "取消", CancelButton: "取消"})
		if err != nil {
			return err
		}
		if button != "清除并退出" {
			return errors.New("已取消清除应用数据")
		}
	}
	if remember {
		settings := s.cfg.Get()
		settings.ExitAction = action
		if err := s.cfg.UpdatePreferences(settings); err != nil {
			return err
		}
	}
	return s.quit(action)
}

// Get 获取当前设置。
func (s *SettingsService) Get() config.Settings { return s.cfg.Get() }

// Save 保存设置，日志配置立即热生效。
func (s *SettingsService) Save(settings config.Settings) error {
	if err := s.cfg.UpdatePreferences(settings); err != nil {
		logSettings.Errorf("保存设置失败: %v", err)
		return err
	}
	logging.Reconfigure(s.cfg.Get().Log)
	logSettings.Infof("设置已保存，日志配置已热更新（级别=%s 目标=%s）", s.cfg.Get().Log.Level, s.cfg.Get().Log.Targets)
	return nil
}

// DataDir 返回应用数据目录（用于界面展示）。
func (s *SettingsService) DataDir() string { return s.cfg.DataDir() }

// ClearCache 清空缓存目录下的 thumbs/、previews/ 两个子目录后重建（不递归删根目录）。
// 经 thumbCache 在锁保护下执行，避免与进行中的缓存写入竞争（SVC-18）。
func (s *SettingsService) ClearCache() error {
	if err := s.cfg.ValidateMediaCacheClear(); err != nil {
		return err
	}
	if err := s.clearThumbCache(); err != nil {
		logSettings.Errorf("清空缓存失败: %v", err)
		return err
	}
	logSettings.Infof("缓存已清空: %s", s.cfg.CacheDir())
	return nil
}

// RecentDirs 返回指定用途的最近目录历史（最近优先，上限 3 条）。
func (s *SettingsService) RecentDirs(kind string) []string {
	return s.cfg.RecentDirs(kind)
}

// AddRecentDir 将指定目录插入该用途的最近历史顶部，去重并裁剪到 3 条上限，返回更新后的列表。
func (s *SettingsService) AddRecentDir(kind, dir string) ([]string, error) {
	list, err := s.cfg.AddRecentDir(kind, dir)
	if err != nil {
		logSettings.Errorf("保存最近目录失败: kind=%s err=%v", kind, err)
		return nil, err
	}
	return list, nil
}
