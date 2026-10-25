package engine

// 状态机转换与断点持久化的单元测试（ENG-05）。
// 通过 Manager.executor 注入打桩执行器，不触碰真实 Telegram 客户端。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tdl-ui/internal/events"
	"tdl-ui/internal/store"
)

// newStubManager 返回注入打桩执行器的管理器。
func newStubManager(exec func(ctx context.Context, t *Task) error) *Manager {
	m := NewManager(Deps{Emitter: &events.Emitter{}})
	m.executor = exec
	return m
}

// addQueuedTask 直接登记一个排队任务（绕过 Create 的 URL/目录校验）。
func addQueuedTask(m *Manager, id string) *Task {
	t := &Task{ID: id, mgr: m, status: StatusQueued}
	m.tasks[id] = t
	m.order = append(m.order, id)
	return t
}

func taskStatus(t *Task) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// waitStatus 轮询等待任务进入目标状态。
func waitStatus(t *testing.T, task *Task, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if taskStatus(task) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待状态 %s 超时，当前 %s", want, taskStatus(task))
}

// 执行成功 → done。
func TestRunTransitionsToDone(t *testing.T) {
	m := newStubManager(func(_ context.Context, _ *Task) error { return nil })
	task := addQueuedTask(m, "t1")
	m.run(task)
	if got := taskStatus(task); got != StatusDone {
		t.Fatalf("预期 done，实际 %s", got)
	}
}

// 执行失败 → failed 且记录错误信息。
func TestRunTransitionsToFailed(t *testing.T) {
	m := newStubManager(func(_ context.Context, _ *Task) error {
		return os.ErrPermission
	})
	task := addQueuedTask(m, "t1")
	m.run(task)
	if got := taskStatus(task); got != StatusFailed {
		t.Fatalf("预期 failed，实际 %s", got)
	}
	task.mu.Lock()
	errMsg := task.errMsg
	task.mu.Unlock()
	if errMsg == "" {
		t.Fatal("failed 状态应携带错误信息")
	}
}

// 运行中 Pause → paused；Resume 后重新执行 → done。
func TestPauseRunningThenResume(t *testing.T) {
	release := make(chan struct{})
	m := newStubManager(func(ctx context.Context, _ *Task) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	})
	task := addQueuedTask(m, "t1")
	go m.run(task)
	waitStatus(t, task, StatusRunning)

	if err := m.Pause("t1"); err != nil {
		t.Fatalf("Pause 失败: %v", err)
	}
	waitStatus(t, task, StatusPaused)

	close(release) // 之后的执行立即成功
	if err := m.Resume("t1"); err != nil {
		t.Fatalf("Resume 失败: %v", err)
	}
	waitStatus(t, task, StatusDone)
}

// 运行中 Cancel → canceled。
func TestCancelRunningTask(t *testing.T) {
	m := newStubManager(func(ctx context.Context, _ *Task) error {
		<-ctx.Done()
		return ctx.Err()
	})
	task := addQueuedTask(m, "t1")
	go m.run(task)
	waitStatus(t, task, StatusRunning)

	if err := m.Cancel("t1"); err != nil {
		t.Fatalf("Cancel 失败: %v", err)
	}
	waitStatus(t, task, StatusCanceled)
}

// telegram.Client.Run normalizes context.Canceled to nil. Exercise that
// boundary instead of assuming every executor propagates cancellation.
func TestPauseAndCancelSurviveExecutorSwallowingCancellation(t *testing.T) {
	for _, pause := range []bool{true, false} {
		name, want := "cancel", StatusCanceled
		if pause {
			name, want = "pause", StatusPaused
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			returned := make(chan struct{})
			m := newStubManager(func(ctx context.Context, _ *Task) error {
				close(started)
				<-ctx.Done()
				return nil
			})
			task := addQueuedTask(m, name)
			go func() { m.run(task); close(returned) }()
			<-started
			var err error
			if pause {
				err = m.Pause(task.ID)
			} else {
				err = m.Cancel(task.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-returned:
			case <-time.After(3 * time.Second):
				t.Fatal("task did not finish after cancellation")
			}
			if v := task.view(); v.Status != want {
				t.Fatalf("cancellation was reported as success: %+v", v)
			}
			m.executor = func(context.Context, *Task) error { return nil }
			if err := m.Resume(task.ID); err != nil {
				t.Fatalf("task cannot resume: %v", err)
			}
			waitStatus(t, task, StatusDone)
			if err := task.waitFinalization(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRunDoesNotHideFileFailuresWhenExecutorReturnsNil(t *testing.T) {
	m := newStubManager(func(_ context.Context, task *Task) error {
		task.onFileFailed()
		return nil
	})
	task := addQueuedTask(m, "file-failure")
	m.run(task)
	if v := task.view(); v.Status != StatusFailed || v.Failed != 1 || v.Error == "" {
		t.Fatalf("failed file was reported as success: %+v", v)
	}
}

func TestRunDoesNotReportAnInterruptedRangeAsDone(t *testing.T) {
	m := newStubManager(func(_ context.Context, task *Task) error {
		task.mu.Lock()
		task.total, task.finished, task.skipped = 10, 2, 1
		task.mu.Unlock()
		return nil
	})
	task := addQueuedTask(m, "unfinished-range")
	m.run(task)
	if v := task.view(); v.Status != StatusFailed || v.Error == "" {
		t.Fatalf("unfinished range was reported as success: %+v", v)
	}
}

func TestReloadRecoversTasksIncorrectlyMarkedDone(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		total, finished, failed, skipped int
		notice                           string
		want                             string
	}{
		{"interrupted range", 1631, 720, 0, 446, "", StatusPaused},
		{"unreported failures", 3, 2, 1, 0, "", StatusFailed},
		{"completed range", 3, 2, 0, 1, "", StatusDone},
		{"intentional disk stop", 1631, 720, 0, 446, "磁盘达到使用率上限", StatusDone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMemRepo()
			repo.tasks["legacy"] = store.Task{ID: "legacy", Status: StatusDone, Total: tc.total, Finished: tc.finished, Failed: tc.failed, Error: tc.notice}
			for n := range tc.skipped {
				repo.files["legacy"] = append(repo.files["legacy"], store.File{State: "skipped", MessageID: n + 1})
			}
			m := NewManager(Deps{Emitter: &events.Emitter{}, Store: repo})
			v := m.List()[0]
			if v.Status != tc.want || v.Finished != tc.finished || v.Skipped != tc.skipped {
				t.Fatalf("incorrect recovery: %+v", v)
			}
			if repo.tasks["legacy"].Status != tc.want {
				t.Fatal("recovered status was not persisted")
			}
			if tc.want == StatusFailed && v.Error == "" {
				t.Fatal("failed task has no explanation")
			}
			if tc.notice != "" && v.Notice != tc.notice {
				t.Fatal("intentional stop notice was changed")
			}
		})
	}
}

func TestRecoveredTaskResumesOriginalRangeAndPreservesProgress(t *testing.T) {
	repo := newMemRepo()
	repo.tasks["legacy"] = store.Task{ID: "legacy", Status: StatusDone, Total: 3, Finished: 1, DownloadOptions: marshalOutputOptions(TaskOptions{AccountID: 42})}
	repo.items["legacy"] = []store.Item{{ItemType: store.ItemTypeSelection, DialogID: 20, DialogType: DialogChannel, MessageIDs: []int{1, 2, 3}}}
	repo.files["legacy"] = []store.File{{State: "done", MessageID: 1, Path: "saved.mp4"}, {State: "skipped", MessageID: 2}}
	key := resumeKey(20, 1, DialogChannel)
	repo.finished["legacy"] = map[string]struct{}{key: {}}
	m := NewManager(Deps{Emitter: &events.Emitter{}, Store: repo})
	task := m.tasks["legacy"]
	if task.status != StatusPaused {
		t.Fatalf("expected recovered pause, got %s", task.status)
	}
	executed := make(chan struct{})
	m.executor = func(_ context.Context, task *Task) error {
		task.mu.Lock()
		task.finished++
		task.mu.Unlock()
		close(executed)
		return nil
	}
	if err := m.Resume("legacy"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executed:
	case <-time.After(3 * time.Second):
		t.Fatal("recovered task was not restarted")
	}
	waitStatus(t, task, StatusDone)
	if err := task.waitFinalization(); err != nil {
		t.Fatal(err)
	}
	if task.opts.AccountID != 42 || len(task.opts.Selections) != 1 || len(task.opts.Selections[0].MessageIDs) != 3 || task.opts.Selections[0].MessageIDs[2] != 3 {
		t.Fatal("recovery changed the original account or selection")
	}
	if len(repo.files["legacy"]) != 2 || repo.files["legacy"][0].Path != "saved.mp4" {
		t.Fatal("recovery changed the saved file records")
	}
	if _, ok := repo.finished["legacy"][key]; !ok {
		t.Fatal("recovery discarded the completed-file resume key")
	}
}

// 排队态直接 Pause/Cancel（ENG-04）；run 启动时放弃执行。
func TestQueuedDirectTransitions(t *testing.T) {
	executed := false
	m := newStubManager(func(_ context.Context, _ *Task) error {
		executed = true
		return nil
	})

	p := addQueuedTask(m, "tp")
	if err := m.Pause("tp"); err != nil {
		t.Fatalf("排队态 Pause 失败: %v", err)
	}
	if got := taskStatus(p); got != StatusPaused {
		t.Fatalf("预期 paused，实际 %s", got)
	}
	m.run(p) // 迟到的 run 必须放弃执行
	if executed {
		t.Fatal("已暂停的排队任务不应被执行")
	}

	c := addQueuedTask(m, "tc")
	if err := m.Cancel("tc"); err != nil {
		t.Fatalf("排队态 Cancel 失败: %v", err)
	}
	if got := taskStatus(c); got != StatusCanceled {
		t.Fatalf("预期 canceled，实际 %s", got)
	}
}

// Remove 后任务不可 Resume 复活（ENG-03）。
func TestRemoveBlocksResume(t *testing.T) {
	m := newStubManager(func(_ context.Context, _ *Task) error { return nil })
	task := addQueuedTask(m, "t1")
	task.status = StatusPaused

	if err := m.Remove("t1"); err != nil {
		t.Fatalf("Remove 失败: %v", err)
	}
	task.mu.Lock()
	removed := task.removed
	task.mu.Unlock()
	if !removed {
		t.Fatal("Remove 后应标记 removed")
	}
	// 持有旧引用直接调 run（模拟并发窗口）：不得执行
	executed := false
	m.executor = func(_ context.Context, _ *Task) error { executed = true; return nil }
	m.run(task)
	if executed {
		t.Fatal("已移除的任务不应被执行")
	}
}

// 终态限制：done 任务不可 Pause/Cancel；running 任务不可 Resume。
func TestInvalidTransitionsRejected(t *testing.T) {
	m := newStubManager(nil)
	d := addQueuedTask(m, "td")
	d.status = StatusDone
	if err := m.Pause("td"); err == nil {
		t.Fatal("done 任务 Pause 应报错")
	}
	if err := m.Cancel("td"); err == nil {
		t.Fatal("done 任务 Cancel 应报错")
	}
	r := addQueuedTask(m, "tr")
	r.status = StatusRunning
	if err := m.Resume("tr"); err == nil {
		t.Fatal("running 任务 Resume 应报错")
	}
}

// Resume 复位错误信息与失败计数，并强制断点续传。
func TestResumeResetsState(t *testing.T) {
	done := make(chan struct{})
	m := newStubManager(func(_ context.Context, _ *Task) error {
		close(done)
		return nil
	})
	task := addQueuedTask(m, "t1")
	task.status = StatusFailed
	task.errMsg = "boom"
	task.failed = 3
	task.opts.Restart = true

	if err := m.Resume("t1"); err != nil {
		t.Fatalf("Resume 失败: %v", err)
	}
	task.mu.Lock()
	errMsg, failed, restart := task.errMsg, task.failed, task.opts.Restart
	task.mu.Unlock()
	if errMsg != "" || failed != 0 {
		t.Fatalf("Resume 应复位错误与失败计数，实际 errMsg=%q failed=%d", errMsg, failed)
	}
	if restart {
		t.Fatal("Resume 应强制断点续传（Restart=false）")
	}
	<-done
	waitStatus(t, task, StatusDone)
}

// 断点持久化：saveResumeKey 逐条落库后 LoadFinished 能读回同一集合；
// 带 store 的管理器重启后运行/排队任务恢复为 paused。
func TestResumePersistenceRoundtrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("打开 store 失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	m := NewManager(Deps{Emitter: &events.Emitter{}, Store: st})
	id, err0 := func() (string, error) {
		task := &Task{
			ID: "t1", CreatedAt: "2026-01-01 00:00:00", mgr: m, status: StatusRunning,
			opts: TaskOptions{Dir: t.TempDir(), URLs: []string{"https://t.me/c/1/2"}},
		}
		if err := st.InsertTask(context.Background(), toStoreTask(task), optionsToItems(task.opts)); err != nil {
			return "", err
		}
		m.tasks[task.ID] = task
		m.order = append(m.order, task.ID)

		task.saveResumeKey("1_2")
		task.saveResumeKey("1_3")
		return task.ID, nil
	}()
	if err0 != nil {
		t.Fatalf("准备任务失败: %v", err0)
	}

	got, err := st.LoadFinished(context.Background(), id)
	if err != nil {
		t.Fatalf("LoadFinished 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("断点集合应有 2 项，实际 %d", len(got))
	}
	if _, ok := got["1_2"]; !ok {
		t.Fatal("断点集合缺少 1_2")
	}

	// 模拟重启：新管理器从 store 恢复，running 任务应落为 paused（可断点续传）
	m2 := NewManager(Deps{Emitter: &events.Emitter{}, Store: st})
	t2, err := m2.get(id)
	if err != nil {
		t.Fatalf("重启后任务丢失: %v", err)
	}
	if got := taskStatus(t2); got != StatusPaused {
		t.Fatalf("重启后 running 任务应为 paused，实际 %s", got)
	}
}
