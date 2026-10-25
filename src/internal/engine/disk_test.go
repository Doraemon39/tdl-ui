package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/pkg/tmessage"

	"tdl-ui/internal/events"
	"tdl-ui/internal/manifest"
)

func testDiskGuard(t *testing.T, limit int, space func(string) (diskSpace, error)) *diskGuard {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &diskGuard{dir: t.TempDir(), limit: limit, ctx: ctx, cancel: cancel, space: space}
}

func TestDiskGuardLimitsAndProbeFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		space     diskSpace
		bytes     uint64
		wantLimit bool
	}{
		{"below", diskSpace{1000, 101, 101}, 0, false},
		{"at limit", diskSpace{1000, 100, 100}, 0, true},
		{"over limit", diskSpace{1000, 90, 90}, 0, true},
		{"next write crosses", diskSpace{1000, 150, 150}, 51, true},
		{"next write fits", diskSpace{1000, 150, 150}, 50, false},
		{"quota exhausted", diskSpace{1000, 500, 5}, 6, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := testDiskGuard(t, 90, func(string) (diskSpace, error) { return tc.space, nil })
			err := g.check(tc.bytes)
			var limit *diskLimitError
			if errors.As(err, &limit) != tc.wantLimit {
				t.Fatalf("unexpected result: %v", err)
			}
			if tc.wantLimit && !errors.Is(g.ctx.Err(), context.Canceled) {
				t.Fatal("download was not stopped")
			}
		})
	}
	g := testDiskGuard(t, 90, func(string) (diskSpace, error) { return diskSpace{}, os.ErrPermission })
	if !errors.Is(g.check(0), os.ErrPermission) || g.ctx.Err() == nil {
		t.Fatal("probe failure must stop downloads")
	}
}

func TestDiskWriterRejectsParallelPartExtensionBeforeWriting(t *testing.T) {
	g := testDiskGuard(t, 90, func(string) (diskSpace, error) { return diskSpace{1000, 500, 500}, nil })
	f, err := os.Create(filepath.Join(g.dir, "part.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := diskWriter{file: f, guard: g}
	if n, err := w.WriteAt([]byte("part"), 450); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("unsafe part written: %d %v", n, err)
	}
	st, _ := f.Stat()
	if st.Size() != 0 {
		t.Fatal("file was extended past the limit")
	}
}

func TestDiskWriterConcurrentChunksStopAtLimit(t *testing.T) {
	var f *os.File
	g := testDiskGuard(t, 90, func(string) (diskSpace, error) {
		st, err := f.Stat()
		if err != nil {
			return diskSpace{}, err
		}
		return diskSpace{1000, uint64(150 - st.Size()), uint64(150 - st.Size())}, nil
	})
	var err error
	f, err = os.Create(filepath.Join(g.dir, "part.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := diskWriter{file: f, guard: g}
	if _, err := w.WriteAt(make([]byte, 10), 0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 1; n < 9; n++ {
		wg.Go(func() { _, _ = w.WriteAt(make([]byte, 10), int64(n*10)) })
	}
	wg.Wait()
	st, _ := f.Stat()
	if st.Size() > 50 || st.Size() < 10 || g.Err() == nil {
		t.Fatalf("limit not enforced: size=%d err=%v", st.Size(), g.Err())
	}
}

func TestDiskWatcherStopsWhenOtherProgramsConsumeSpace(t *testing.T) {
	g := testDiskGuard(t, 90, func(string) (diskSpace, error) { return diskSpace{1000, 90, 90}, nil })
	stop := g.watch()
	defer stop()
	select {
	case <-g.ctx.Done():
		if g.Err() == nil {
			t.Fatal("stop reason missing")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("external disk use was not detected")
	}
}

type erroredDownloadIter struct {
	started chan struct{}
	next    bool
}

func (i *erroredDownloadIter) Next(context.Context) bool {
	if !i.next {
		i.next = true
		return true
	}
	<-i.started
	return false
}
func (i *erroredDownloadIter) Value() downloader.Elem { return &iterElem{} }
func (i *erroredDownloadIter) Err() error             { return os.ErrPermission }

type blockedCompletionProgress struct {
	started, finishing, release chan struct{}
	cancel                      context.CancelFunc
}

func (p *blockedCompletionProgress) OnAdd(downloader.Elem)                                { p.cancel(); close(p.started) }
func (p *blockedCompletionProgress) OnDownload(downloader.Elem, downloader.ProgressState) {}
func (p *blockedCompletionProgress) OnDone(downloader.Elem, error)                        { close(p.finishing); <-p.release }

func TestIteratorErrorWaitsForFileCompletionBeforeReturning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &blockedCompletionProgress{started: make(chan struct{}), finishing: make(chan struct{}), release: make(chan struct{}), cancel: cancel}
	i := &erroredDownloadIter{started: p.started}
	returned := make(chan struct{})
	go func() {
		_ = downloader.New(downloader.Options{Iter: settlingIter{i}, Progress: p}).Download(ctx, 1)
		close(returned)
	}()
	<-p.finishing
	select {
	case <-returned:
		t.Fatal("download returned before the file was finalized")
	case <-time.After(30 * time.Millisecond):
	}
	close(p.release)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("download did not finish after finalization")
	}
	if !errors.Is(i.Err(), os.ErrPermission) {
		t.Fatal("iterator error was lost")
	}
}

func completedEntry(id int) manifest.Entry {
	return manifest.Entry{MessageID: id, MediaID: "media", Name: "clip.mp4", Size: 4, Date: 1700000000, State: "done", Evidence: "source"}
}

func TestDiskStopAppendsImportedManifestAndFinishesWithNotice(t *testing.T) {
	dir := t.TempDir()
	m := newStubManager(func(_ context.Context, task *Task) error {
		if _, err := task.recordCompletion(20, "channel", completedEntry(2)); err != nil {
			return err
		}
		return &diskLimitError{dir: dir, limit: 90}
	})
	info, err := m.manifests.Create(dir, manifest.Header{AccountID: 1, DialogID: 20, DialogType: "channel"}, []manifest.Entry{completedEntry(1)})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(info.Path)
	task := addQueuedTask(m, "disk-stop")
	task.opts = TaskOptions{Dir: dir, AccountID: 1, ManifestPath: info.Path, RecordManifest: true}
	task.manifest, err = m.manifests.Open(info.Path, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.run(task)
	v := task.view()
	if v.Status != StatusDone || v.Error != "" || !strings.Contains(v.Notice, info.Path) {
		t.Fatalf("unexpected completion: %+v", v)
	}
	after, _ := os.ReadFile(info.Path)
	if !strings.HasPrefix(string(after), string(before)) {
		t.Fatal("imported journal was overwritten")
	}
	s, err := m.manifests.Open(info.Path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Info().Total != 2 || !s.ShouldSkip(20, "channel", 2, "media", 4, "completed") {
		t.Fatal("completed download is missing")
	}
}

func TestAutomaticManifestSurvivesRestartAndSeparatesSources(t *testing.T) {
	dir := t.TempDir()
	m := newStubManager(nil)
	task := addQueuedTask(m, "automatic")
	task.opts = TaskOptions{Dir: dir, AccountID: 1}
	path, err := task.recordCompletion(20, "channel", completedEntry(1))
	if err != nil {
		t.Fatal(err)
	}
	other, err := task.recordCompletion(20, "private", completedEntry(1))
	if err != nil || path == other {
		t.Fatalf("source collision: %s %s %v", path, other, err)
	}
	// A fresh manager has no cached sessions, like an application restart.
	m2 := newStubManager(nil)
	resumed := addQueuedTask(m2, "automatic")
	resumed.opts = task.opts
	got, err := resumed.recordCompletion(20, "channel", completedEntry(2))
	if err != nil || got != path {
		t.Fatalf("did not reuse automatic journal: %s %v", got, err)
	}
	s, err := m2.manifests.Open(path, 1)
	if err != nil || s.Info().Total != 2 {
		t.Fatalf("lost previous completed files: %v", err)
	}
	if s.ShouldSkip(20, "channel", 3, "media", 4, "completed") {
		t.Fatal("unfinished file was recorded")
	}
}

func TestDiskStopManifestFailureIsNotReportedAsSuccess(t *testing.T) {
	m := newStubManager(func(_ context.Context, task *Task) error {
		task.recordManifestError(os.ErrPermission)
		return &diskLimitError{limit: 90}
	})
	task := addQueuedTask(m, "manifest-failure")
	m.run(task)
	if v := task.view(); v.Status != StatusFailed || !strings.Contains(v.Error, "清单未能更新") {
		t.Fatalf("failure hidden: %+v", v)
	}
}

func TestDiskStopNoticeSurvivesTaskReload(t *testing.T) {
	repo := newMemRepo()
	m := NewManager(Deps{Emitter: &events.Emitter{}, Store: repo})
	m.executor = func(context.Context, *Task) error { return &diskLimitError{dir: "downloads", limit: 85} }
	task := addQueuedTask(m, "persisted-stop")
	if err := repo.InsertTask(context.Background(), toStoreTask(task), nil); err != nil {
		t.Fatal(err)
	}
	m.run(task)
	loaded := NewManager(Deps{Emitter: &events.Emitter{}, Store: repo})
	v := loaded.List()[0]
	if v.Status != StatusDone || v.Error != "" || !strings.Contains(v.Notice, "85%") {
		t.Fatalf("stop notice lost on reload: %+v", v)
	}
}

func TestReadDiskSpaceForDownloadDirectory(t *testing.T) {
	s, err := readDiskSpace(t.TempDir())
	if err != nil || s.total == 0 || s.free > s.total {
		t.Fatalf("invalid disk stats: %+v %v", s, err)
	}
}

func TestAutomaticManifestsDoNotExhaustImportedManifestRegistry(t *testing.T) {
	dir := t.TempDir()
	m := newStubManager(nil)
	for n := range 257 {
		task := addQueuedTask(m, fmt.Sprintf("automatic-%d", n))
		task.opts = TaskOptions{Dir: dir, AccountID: 1}
		if _, err := task.recordCompletion(20, "channel", completedEntry(1)); err != nil {
			t.Fatalf("automatic journal %d: %v", n, err)
		}
	}
	if _, err := m.manifests.Create(dir, manifest.Header{AccountID: 1, DialogID: 20, DialogType: "channel"}, nil); err != nil {
		t.Fatalf("automatic journals blocked an explicit import: %v", err)
	}
}

func TestDiskStopDoesNotHideFailedFiles(t *testing.T) {
	m := newStubManager(func(_ context.Context, task *Task) error {
		task.mu.Lock()
		task.failed = 1
		task.mu.Unlock()
		return &diskLimitError{limit: 90}
	})
	task := addQueuedTask(m, "failed-before-disk-stop")
	m.run(task)
	if v := task.view(); v.Status != StatusFailed || !strings.Contains(v.Error, "部分文件") || !strings.Contains(v.Error, "90%") {
		t.Fatalf("failed file hidden by disk stop: %+v", v)
	}
}

func TestCanceledDiskGuardDoesNotTurnUserCancellationIntoDiskStop(t *testing.T) {
	g := testDiskGuard(t, 90, func(string) (diskSpace, error) { return diskSpace{1000, 90, 90}, nil })
	g.cancel()
	if err := g.check(0); !errors.Is(err, context.Canceled) || g.Err() != nil {
		t.Fatalf("user cancellation replaced with disk stop: check=%v guard=%v", err, g.Err())
	}
}

func TestResumeRepairsCompletionJournalAfterWriteFailureAndRestart(t *testing.T) {
	for _, imported := range []bool{false, true} {
		t.Run(fmt.Sprintf("imported=%v", imported), func(t *testing.T) {
			dir := t.TempDir()
			m := newStubManager(nil)
			task := addQueuedTask(m, "repair")
			task.opts = TaskOptions{Dir: dir, AccountID: 1}
			path := filepath.Join(dir, "tdl-media-channel-20-repair.jsonl")
			if imported {
				info, err := m.manifests.Create(dir, manifest.Header{AccountID: 1, DialogID: 20, DialogType: "channel"}, nil)
				if err != nil {
					t.Fatal(err)
				}
				path = info.Path
				task.opts.ManifestPath = path
				task.opts.RecordManifest = true
				task.manifest, err = m.manifests.Open(path, 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(path+".lock", 0o700); err != nil {
				t.Fatal(err)
			}
			entry := completedEntry(1)
			entry.MediaID = "document:123"
			digest := sha256.Sum256([]byte("data"))
			entry.SHA256 = hex.EncodeToString(digest[:])
			if err := os.WriteFile(filepath.Join(dir, "saved.mp4"), []byte("data"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := task.recordCompletion(20, "channel", entry); err == nil {
				t.Fatal("journal write unexpectedly succeeded")
			}
			if err := os.Remove(path + ".lock"); err != nil {
				t.Fatal(err)
			}
			m2 := newStubManager(nil)
			resumed := addQueuedTask(m2, "repair")
			resumed.opts = task.opts
			if imported {
				var err error
				resumed.manifest, err = m2.manifests.Open(path, 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			key := resumeKey(20, 1, "channel")
			i, err := newIter(nil, nil, []*tmessage.Dialog{{Peer: &tg.InputPeerChannel{ChannelID: 20}, Messages: []int{1}}}, IterOptions{
				Dir: dir, Template: "{{ .FileName }}", Manifest: resumed.manifest, RecordManifest: imported,
				LocalMatches: map[string]manifest.LocalMatch{key: {Entry: entry, Path: "saved.mp4"}},
				OnLocalDone: func(dialogID int64, typ string, entry manifest.Entry) error {
					_, err := resumed.recordCompletion(dialogID, typ, entry)
					return err
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer i.Close()
			i.SetFinished(map[string]struct{}{key: {}})
			message := &tg.Message{ID: 1, Date: int(entry.Date)}
			message.SetMedia(&tg.MessageMediaDocument{Document: &tg.Document{ID: 123, Size: 4, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "clip.mp4"}}}})
			from := (&peers.Manager{}).Channel(&tg.Channel{ID: 20})
			if ok, skip := i.processSingle(context.Background(), message, from, false); ok || !skip || i.Err() != nil {
				t.Fatalf("completed file did not resume safely: ok=%v skip=%v err=%v", ok, skip, i.Err())
			}
			s, err := m2.manifests.Open(path, 1)
			if err != nil {
				t.Fatal(err)
			}
			info := s.Info()
			if len(info.Entries) != 1 || info.Entries[0].SHA256 != entry.SHA256 || info.Entries[0].SavedName != "saved.mp4" {
				t.Fatalf("completed record was not repaired: %+v", info)
			}
		})
	}
}
