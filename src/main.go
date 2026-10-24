//go:build !bindings

// tdl-ui：基于 Wails v2 的 tdl 桌面 GUI。
// 核心下载能力复用 ref/tdl 子模块（github.com/iyear/tdl，AGPL-3.0）。
package main

import (
	"context"
	"fmt"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/iyear/tdl/pkg/kv"

	"tdl-ui/internal/applock"
	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/events"
	"tdl-ui/internal/logging"
	"tdl-ui/internal/script"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/services"
	"tdl-ui/internal/store"
)

var logApp = logging.L("app")

func main() {
	if runCleanupHelper() {
		return
	}
	dataDir, portable, err := config.ResolveDataDir()
	if err != nil {
		showNativeError(fmt.Sprintf("初始化配置失败: %v", err))
		return
	}
	wait := time.Second
	if config.HasPendingCleanup(dataDir) {
		wait = 35 * time.Second
	}
	lockCtx, lockCancel := context.WithTimeout(context.Background(), wait)
	dataGuard, err := applock.Acquire(lockCtx, dataDir)
	lockCancel()
	if err != nil {
		showNativeError(fmt.Sprintf("无法打开应用数据目录（另一个实例可能仍在运行或清理）: %v", err))
		return
	}
	defer dataGuard.Close()
	cfg, err := config.NewManagerForDataDir(dataDir, portable)
	if err != nil {
		showNativeError(fmt.Sprintf("初始化配置失败: %v", err))
		log.Fatalf("初始化配置失败: %v", err)
	}

	emitter := &events.Emitter{}

	// 日志初始化：数据目录下 logging.yaml 存在时导入一次并改名，
	// 避免每次启动静默覆盖设置页修改的日志配置（SVC-06）
	logCfg := cfg.Get().Log
	loggingYAML := filepath.Join(cfg.DataDir(), "logging.yaml")
	var logImportNotes, logImportWarns []string
	if b, err := os.ReadFile(loggingYAML); err == nil {
		if parsed, perr := logging.ParseYAML(b, cfg.Get().Log.Dir); perr != nil {
			logImportWarns = append(logImportWarns, fmt.Sprintf("解析 logging.yaml 失败，保持现有日志配置: %v", perr))
		} else {
			logCfg = parsed
			settings := cfg.Get()
			settings.Log = parsed
			if uerr := cfg.Update(settings); uerr != nil {
				logImportWarns = append(logImportWarns, fmt.Sprintf("持久化 logging.yaml 导入的日志配置失败: %v", uerr))
			}
			if rerr := os.Rename(loggingYAML, loggingYAML+".imported"); rerr != nil {
				logImportWarns = append(logImportWarns, fmt.Sprintf("重命名 logging.yaml 失败，下次启动仍会重新导入: %v", rerr))
			}
			logImportNotes = append(logImportNotes, "已导入 logging.yaml 日志配置（原文件改名为 logging.yaml.imported）")
		}
	}
	logging.Init(logCfg, func(batch []logging.LogEntry) {
		emitter.Emit(events.Log, batch)
	})
	for _, m := range logImportNotes {
		logApp.Infof("%s", m)
	}
	for _, m := range logImportWarns {
		logApp.Warnf("%s", m)
	}
	logApp.Infof("应用启动，数据目录: %s，日志目录: %s", cfg.DataDir(), logging.CurrentDir())

	// bolt kv 全局唯一实例，登录与下载共享（bbolt 文件锁不允许重复打开）
	kvs, err := kv.New(kv.DriverBolt, map[string]any{"path": cfg.KVDir()})
	if err != nil {
		logApp.Errorf("初始化存储失败: %v", err)
		logging.Close()
		log.Fatalf("初始化存储失败: %v", err)
	}
	logApp.Infof("bolt 存储就绪: %s", cfg.KVDir())

	// 任务持久化：SQLite（tasks.db），首次运行时从旧 tasks.json 一次性导入
	taskStore, err := store.Open(filepath.Join(cfg.DataDir(), "tasks.db"))
	if err != nil {
		logApp.Errorf("初始化任务存储失败: %v", err)
		_ = kvs.Close()
		logging.Close()
		log.Fatalf("初始化任务存储失败: %v", err)
	}
	if err := taskStore.ImportLegacyJSON(context.Background(), filepath.Join(cfg.DataDir(), "tasks.json")); err != nil {
		logApp.Warnf("导入旧任务记录失败: %v", err)
	}
	logApp.Infof("任务存储就绪: %s", filepath.Join(cfg.DataDir(), "tasks.db"))

	// 脚本日志转发到前端，同时写入日志系统
	scriptLogger := logging.L("script")
	scriptapi.SetLogSink(func(msg string) {
		scriptLogger.Infof("%s", strings.TrimRight(msg, "\n"))
		emitter.Emit(events.ScriptLog, msg)
	})

	scriptStore := script.NewStore(cfg.ScriptDir())

	// 首次启动时播种内置脚本模板（默认禁用）
	if !cfg.Get().TemplatesSeeded {
		if n, err := scriptStore.SeedTemplates(); err != nil {
			logApp.Warnf("写入内置脚本模板失败: %v", err)
		} else {
			_ = cfg.MarkTemplatesSeeded()
			logApp.Infof("已写入 %d 个内置脚本模板（默认禁用）", n)
		}
	}

	taskManager := engine.NewManager(engine.Deps{
		Cfg:     cfg,
		KV:      kvs,
		Emitter: emitter,
		Scripts: scriptStore,
		Store:   taskStore,
	})

	authSvc := services.NewAuthService(cfg, kvs, emitter)
	downloadSvc := services.NewDownloadService(cfg, taskManager, emitter)
	scriptSvc := services.NewScriptService(scriptStore, cfg)
	chatSvc := services.NewChatService(cfg, kvs)
	// 设置页「清空缓存」经 thumbCache 在锁保护下执行，避免与缓存写入竞争（SVC-18）
	var exitMu sync.Mutex
	exitAction := ""
	settingsSvc := services.NewSettingsService(cfg, chatSvc.ClearThumbCache, emitter, func(action string) error {
		ctx := emitter.Ctx()
		if ctx == nil {
			return fmt.Errorf("应用尚未就绪")
		}
		exitMu.Lock()
		exitAction = action
		exitMu.Unlock()
		runtime.Quit(ctx)
		return nil
	})
	cleanupSafe := false
	logSvc := services.NewLogService(cfg, emitter)
	// 会话变更（登出/重登/导入）时关闭对话服务常驻连接，下次查询用新会话自动重建
	authSvc.OnSessionChanged = func() {
		if err := chatSvc.StopForSessionChange(); err != nil {
			logApp.Warnf("停止旧会话失败: %v", err)
		}
		taskManager.Manifests().Clear()
	}
	// 活跃下载与登录流程会互踩同一份会话，重登/登出前拒绝
	authSvc.HasActiveDownloads = taskManager.HasActive
	// 启动对账：以 kv 会话为权威源校准展示用登录态（LOGIC-003）
	authSvc.ReconcileOnStartup()

	err = wails.Run(&options.App{
		Title:     "tdl UI",
		Width:     1200,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
			// /media/thumb、/media/preview、/media/local、/media/video
			// 已下载视频直接由磁盘回放（零 API 消耗），故注入本地文件解析器
			Handler: services.NewMediaHandler(chatSvc, func(dialogID int64, messageID int, dialogType string) (string, bool) {
				f, ok, err := taskManager.DownloadedFile(dialogID, messageID, dialogType)
				if err != nil || !ok {
					return "", false
				}
				return f.Path, true
			}),
		},
		BackgroundColour: &options.RGBA{R: 250, G: 250, B: 251, A: 1},
		OnStartup: func(ctx context.Context) {
			emitter.Bind(ctx)
			logApp.Infof("Wails 运行时就绪，窗口已启动")
		},
		OnBeforeClose: func(ctx context.Context) bool {
			exitMu.Lock()
			action := exitAction
			if action == "" {
				action = cfg.Get().ExitAction
			}
			if action == "direct" || action == "clean_cache" || action == "wipe_all" {
				if err := cfg.ScheduleCleanup(action); err != nil {
					exitMu.Unlock()
					_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.ErrorDialog, Title: "无法安全清理", Message: err.Error()})
					return true
				}
				exitAction = action
				exitMu.Unlock()
				return false
			}
			exitMu.Unlock()
			emitter.Emit(events.AppBeforeClose, nil)
			return true
		},
		OnShutdown: func(ctx context.Context) {
			// 关闭编排：先解绑事件发射（webview 已在销毁，SVC-19），
			// 再停业务 goroutine（下载任务、常驻连接）并等其退出，
			// 最后关存储与日志，避免进度丢失与“database is closed”刷屏
			emitter.Unbind()
			logApp.Infof("应用退出：停止下载任务与常驻连接")
			authStopped := authSvc.Shutdown() == nil
			downloadsStopped := taskManager.StopAndWait(15 * time.Second)
			chatStopped := chatSvc.StopAndWait(5 * time.Second)
			cleanupSafe = downloadsStopped && chatStopped && authStopped
			logApp.Infof("业务已停止，关闭存储与日志")
			_ = taskStore.Close()
			_ = kvs.Close()
			logging.Close()
		},
		Bind: []interface{}{
			authSvc,
			downloadSvc,
			scriptSvc,
			settingsSvc,
			chatSvc,
			logSvc,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			WebviewUserDataPath:  cfg.WebviewDataDir(),
		},
	})
	// Wails.Run returning does not mean Windows released the WebView profile.
	// The deferred helper waits for this process's data lock, then retries the
	// residual browser locks. It never initializes another WebView.
	exitMu.Lock()
	action := exitAction
	exitMu.Unlock()
	if action == "clean_cache" || action == "wipe_all" {
		if !cleanupSafe {
			showNativeError("退出清理未执行：后台任务尚未完全停止。下次启动前将重试已确认的清理。")
		} else if action == "wipe_all" {
			if cleanupErr := startCleanupHelper(cfg.DataDir()); cleanupErr != nil {
				showNativeError(fmt.Sprintf("无法启动退出清理: %v。记录已保留，下次启动前将重试。", cleanupErr))
			}
		} else if cleanupErr := cfg.Cleanup(action); cleanupErr != nil {
			showNativeError(fmt.Sprintf("退出清理未完成: %v。下次启动前将重试。", cleanupErr))
		}
	}
	if err != nil {
		logApp.Errorf("启动失败: %v", err)
		logging.Close()
		log.Fatalf("启动失败: %v", err)
	}
}
