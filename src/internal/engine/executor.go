package engine

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"

	"github.com/iyear/tdl/core/dcpool"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/storage"
	coretclient "github.com/iyear/tdl/core/tclient"
	pkgtclient "github.com/iyear/tdl/pkg/tclient"
	"github.com/iyear/tdl/pkg/tmessage"

	"tdl-ui/internal/config"
	"tdl-ui/internal/manifest"
	"tdl-ui/internal/scriptapi"
	"tdl-ui/internal/store"
)

// run 驱动任务执行并根据结果迁移状态。
func (m *Manager) run(t *Task) {
	if !m.exists(t.ID) { // 记录已被删除（如删除全部文件），不再执行
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})

	t.mu.Lock()
	if t.removed || t.status != StatusQueued {
		// 排队期间已被暂停/取消/移除，放弃执行
		t.mu.Unlock()
		cancel()
		return
	}
	t.cancel = cancel
	t.runDone = runDone
	t.status = StatusRunning
	t.mu.Unlock()
	logEngine.Infof("任务开始运行: id=%s", t.ID)
	t.emitUpdate()

	_ = t.contracts.SafeOnTaskStart(t.info())

	exec := m.executor
	if exec == nil {
		exec = m.execute
	}
	err := exec(ctx, t)
	t.mu.Lock()
	// telegram.Client.Run can return nil after context cancellation. Preserve
	// the user's stop request before canceling this run's context ourselves.
	if err == nil {
		err = ctx.Err()
	}
	t.cancel = nil
	manifestError, failed := t.manifestError, t.failed
	total, completed := t.total, t.finished+t.skipped
	t.mu.Unlock()
	cancel()
	var diskLimit *diskLimitError
	if err == nil || errors.As(err, &diskLimit) {
		prefix := ""
		if err != nil {
			prefix = err.Error() + "\n"
		}
		if manifestError != "" {
			err = errors.New(prefix + "文件已保存，但清单未能更新：" + manifestError)
		} else if failed > 0 {
			err = errors.New(prefix + "部分文件未能完成保存，请查看失败记录后重试")
		} else if err == nil && total > completed {
			err = errors.New("下载在处理完全部项目之前结束，请重试剩余项目")
		}
	}
	notice := ""
	if errors.As(err, &diskLimit) {
		notice = t.diskStopNotice(err)
	}

	t.mu.Lock()
	switch {
	case notice != "":
		t.status = StatusDone
		t.errMsg = notice
	case err == nil:
		t.status = StatusDone
		t.errMsg = ""
	case errors.Is(err, context.Canceled):
		if t.paused {
			t.status = StatusPaused
		} else {
			t.status = StatusCanceled
		}
	default:
		t.status = StatusFailed
		t.errMsg = err.Error()
	}
	status := t.status
	t.mu.Unlock()

	if status == StatusFailed {
		logEngine.Errorf("任务失败: id=%s err=%v", t.ID, err)
	} else {
		logEngine.Infof("任务结束: id=%s 状态=%s", t.ID, status)
	}

	t.emitUpdate()
	_ = t.contracts.SafeOnTaskDone(t.info())
	t.manifestMu.Lock()
	t.manifest = nil
	t.completionManifests = nil
	t.manifestMu.Unlock()
	t.mu.Lock()
	t.runDone = nil
	t.mu.Unlock()
	close(runDone)
}

// execute 单次任务执行：建连 → 解析链接 → 装配迭代器 → 断点续传 → 下载。
// 装配流程对应 ref/tdl/app/dl/dl.go 的 Run。
func (m *Manager) execute(ctx context.Context, t *Task) (rerr error) {
	release, err := m.deps.Cfg.UseSession()
	if err != nil {
		return err
	}
	defer release()
	if err := normalizeTaskPolicies(&t.opts); err != nil {
		return err
	}
	var settings config.Settings
	if m.deps.Cfg != nil {
		settings = m.deps.Cfg.Get()
	} else {
		settings = config.Settings{Threads: 4, Limit: 2, ProgressIntervalMs: 500, DiskUsageLimit: 90}
	}
	if t.opts.ManifestPath != "" {
		session, err := m.manifests.Open(t.opts.ManifestPath, settings.LoggedInUserID)
		if err != nil {
			return err
		}
		t.manifest = session
	}
	t.mu.Lock()
	t.manifestError = ""
	t.mu.Unlock()
	if m.deps.Cfg != nil && (t.opts.AccountID <= 0 || t.opts.AccountID != settings.LoggedInUserID) {
		return errors.New("任务账号与当前会话不一致，请重新选择来源")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	guard := &diskGuard{dir: t.opts.Dir, limit: settings.DiskUsageLimit, ctx: ctx, cancel: cancel, space: readDiskSpace}
	defer func() {
		if guardErr := guard.Err(); guardErr != nil && (rerr == nil || errors.Is(rerr, context.Canceled)) {
			rerr = guardErr
		}
	}()
	if err := guard.check(0); err != nil {
		return err
	}
	defer guard.watch()()

	// 每轮从 task_items 表重组装 URLs/Selections，与持久化的消息项保持一致。
	urls, selections := t.opts.URLs, t.opts.Selections
	if m.deps.Store != nil {
		items, err := m.deps.Store.ListItems(ctx, t.ID)
		if err != nil {
			return errors.Wrap(err, "读取任务消息项失败")
		}
		if len(items) > 0 {
			urls, selections = itemsToOptions(items)
			t.mu.Lock()
			t.opts.URLs = urls
			t.opts.Selections = selections
			t.mu.Unlock()
		}
	}

	kvd, err := m.deps.KV.Open(Namespace)
	if err != nil {
		return errors.Wrap(err, "open kv")
	}

	c, err := pkgtclient.New(ctx, pkgtclient.Options{
		KV:               kvd,
		Proxy:            config.EffectiveProxy(settings),
		ReconnectTimeout: reconnectTimeout,
	}, false)
	if err != nil {
		return errors.Wrap(err, "create client")
	}

	return coretclient.RunWithAuth(ctx, c, func(ctx context.Context) error {
		self, err := c.Self(ctx)
		if err != nil {
			return err
		}
		if self.ID != t.opts.AccountID {
			return errors.New("实际登录账号与任务归属不一致，已停止下载")
		}
		pool := dcpool.NewPool(c,
			int64(settings.PoolSize),
			coretclient.NewDefaultMiddlewares(ctx, reconnectTimeout)...)
		defer func() { _ = pool.Close() }()

		manager := peers.Options{Storage: storage.NewPeers(kvd)}.Build(pool.Default(ctx))
		var dialogs []*tmessage.Dialog
		single := map[string]bool{}
		if len(urls) > 0 {
			var err error
			dialogs, single, err = resolveMessageLinks(ctx, manager, urls)
			if err != nil {
				return errors.Wrap(err, "解析消息链接失败")
			}
		}

		// 选集下载：按对话 + 消息 ID 直接解析，与链接结果合并
		if len(selections) > 0 {
			d, err := selectionsToDialogs(ctx, manager, selections)
			if err != nil {
				return err
			}
			dialogs = append(dialogs, d...)
		}

		origins := map[string]string{}
		if t.opts.ConflictCheck == "origin" && t.opts.ConflictAction != "smart" {
			var err error
			origins, err = m.knownOrigins(t.opts.Dir)
			if err != nil {
				return errors.Wrap(err, "读取原始文件名记录失败")
			}
		}
		localMatches, err := m.KnownLocalMatches(t.opts.Dir)
		if err != nil {
			return err
		}
		it, err := newIter(pool, manager, dialogs, IterOptions{
			Disk: guard,
			OnLocalDone: func(dialogID int64, typ string, entry manifest.Entry) error {
				_, err := t.recordCompletion(dialogID, typ, entry)
				return err
			},
			OverwriteKeys: t.opts.OverwriteKeys,
			LocalMatches:  localMatches, RecordManifest: t.opts.RecordManifest,
			Dir: t.opts.Dir, SingleKeys: single, Manifest: t.manifest, ManifestMode: t.opts.ManifestMode,
			RewriteExt:     t.opts.RewriteExt,
			SkipSame:       t.opts.SkipSame,
			ConflictAction: t.opts.ConflictAction, ConflictCheck: t.opts.ConflictCheck, KnownOrigins: origins, Outputs: &m.outputs, TargetNames: t.opts.TargetNames,
			Template:  t.opts.Template,
			Group:     t.opts.Group,
			Contracts: t.contracts,
			Renames:   t.opts.Renames,
			OnSkip: func(info scriptapi.FileInfo, name, reason, dialogType string) {
				t.onFileSkipped(TaskFile{Name: name, Size: info.FileSize, State: "skipped", Reason: reason, DialogID: info.DialogID, MessageID: info.MessageID, OriginalName: info.FileName, DialogType: dialogType})
			},
			OnTotalDecr: func() {
				// ENG-06：被跳过的消息同步递减 total，保证任务 Done 时 finished 能追平 total；
				// 不即时 emit（大量无媒体消息时避免事件风暴），由下一次进度事件带出。
				t.mu.Lock()
				if t.total > 0 {
					t.total--
				}
				t.mu.Unlock()
			},
		})
		if err != nil {
			return err
		}
		// 暂停/取消后清理 elem 通道内滞留的文件句柄与孤儿 .tmp（ENG-02）
		defer it.Close()

		t.mu.Lock()
		t.total = it.Total()
		t.mu.Unlock()
		if err := it.prepareLocal(ctx); err != nil {
			return err
		}

		// 断点续传：从 resume_keys 加载已完成集合（内容坐标）；Restart 则清空进度
		if t.opts.Restart {
			if m.deps.Store != nil {
				_ = m.deps.Store.DeleteResume(ctx, t.ID)
			}
		} else if m.deps.Store != nil {
			finished, err := m.deps.Store.LoadFinished(ctx, t.ID)
			if err != nil {
				return errors.Wrap(err, "加载断点失败")
			}
			t.mu.Lock()
			for _, f := range t.files {
				if f.State == "done" {
					finished[resumeKey(f.DialogID, f.MessageID, f.DialogType)] = struct{}{}
				}
			}
			t.mu.Unlock()
			it.SetFinished(finished)
		}

		t.mu.Lock()
		t.finished = 0
		t.failed = 0
		t.skipped = 0
		for _, f := range t.files {
			switch f.State {
			case "done":
				t.finished++
			case "skipped":
				t.skipped++
			}
		}
		t.mu.Unlock()
		t.emitUpdate()

		defer func() { // 保存或清理断点（暂停/取消时任务 ctx 已取消，必须用 Background 完成写入，STO-03）
			if m.deps.Store == nil {
				return
			}
			if rerr != nil {
				if saveErr := m.deps.Store.SaveFinished(context.Background(), t.ID, it.Finished()); saveErr != nil &&
					!errors.Is(saveErr, store.ErrStoreClosed) { // 关闭阶段兜底写入失败不污染任务错误（ARC-002）
					rerr = errors.Wrapf(rerr, "save resume: %v", saveErr)
				}
			} else {
				if err := m.deps.Store.SaveFinished(context.Background(), t.ID, it.Finished()); err != nil {
					logStoreErr(err, "保存断点失败: %v", err)
				}
			}
		}()

		derr := downloader.New(downloader.Options{
			Pool:    pool,
			Threads: settings.Threads,
			Iter:    settlingIter{it},
			// 进度节流间隔可经配置调优，零值回退默认 200ms（ARC-003）
			Progress: newProgress(t, it, t.opts.RewriteExt,
				time.Duration(settings.ProgressIntervalMs)*time.Millisecond),
		}).Download(ctx, settings.Limit)
		if iterErr := it.Err(); iterErr != nil {
			return iterErr
		}
		if derr == nil {
			t.mu.Lock()
			t.total = it.discovered
			failed := t.failed
			manifestError := t.manifestError
			t.mu.Unlock()
			if manifestError != "" {
				return errors.New("文件已保存，但清单未能更新：" + manifestError)
			}
			if failed > 0 {
				return errors.New("部分文件未能完成保存，请查看失败记录后重试")
			}
		}
		return derr
	})
}
