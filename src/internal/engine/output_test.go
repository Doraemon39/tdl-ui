package engine

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentDownloadsPreserveExistingAndEachOther(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "clip.mp4"), []byte("original"), 0o600)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	i := &iter{root: root, outputs: &outputCoordinator{}, opts: IterOptions{Dir: dir, ConflictAction: "smart"}}
	var wg sync.WaitGroup
	names := make(chan string, 12)
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, name, err := i.allocate("clip.mp4", "clip.mp4")
			if err != nil {
				t.Error(err)
				return
			}
			f.Write([]byte("download"))
			f.Close()
			e := &iterElem{to: f, targetName: name}
			path, err := i.publish(e, name)
			i.release(name)
			if err != nil {
				t.Error(err)
				return
			}
			names <- path
		}()
	}
	wg.Wait()
	close(names)
	seen := map[string]bool{}
	for name := range names {
		if seen[name] {
			t.Fatal("two downloads shared one output")
		}
		seen[name] = true
	}
	if len(seen) != 12 {
		t.Fatal("lost download")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "clip.mp4"))
	if string(b) != "original" {
		t.Fatal("existing file overwritten")
	}
}

func TestOutputRejectsEscapesAndReservedPaths(t *testing.T) {
	for _, name := range []string{"../secret", "C:\\secret", "NUL.txt", "file:stream", "CON", "hello\nworld"} {
		if _, err := cleanOutputName(name); err == nil {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
}

func TestPublishRaceDoesNotOverwriteNewFile(t *testing.T) {
	dir := t.TempDir()
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	i := &iter{root: root, outputs: &outputCoordinator{}, opts: IterOptions{Dir: dir, ConflictAction: "smart"}}
	f, name, err := i.allocate("clip.mp4", "")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("ours"))
	f.Close()
	os.WriteFile(filepath.Join(dir, name), []byte("raced"), 0o600)
	path, err := i.publish(&iterElem{to: f, targetName: name}, name)
	if err != nil {
		t.Fatal(err)
	}
	if path == filepath.Join(dir, name) {
		t.Fatal("raced target overwritten")
	}
	b, _ := os.ReadFile(filepath.Join(dir, name))
	if string(b) != "raced" {
		t.Fatal("raced file changed")
	}
}

func TestScopedRedownloadNeverExpandsSourceTask(t *testing.T) {
	dir := t.TempDir()
	m := newStubManager(func(ctx context.Context, t *Task) error { return nil })
	source := &Task{ID: "source", mgr: m, status: StatusDone, opts: TaskOptions{Dir: dir, Template: "{{ .FileName }}", Group: true, URLs: []string{"https://t.me/example/1"}}, files: []TaskFile{{Name: "one.mp4", Path: filepath.Join(dir, "one.mp4"), State: "done", DialogID: 55, DialogType: DialogChannel, MessageID: 1}, {Name: "two.mp4", Path: filepath.Join(dir, "two.mp4"), State: "done", DialogID: 55, DialogType: DialogChannel, MessageID: 2}}}
	m.tasks[source.ID] = source
	m.order = []string{source.ID}
	if err := m.RedownloadFiles(source.ID, []string{source.files[1].Path}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	created := m.tasks[m.order[1]]
	m.mu.Unlock()
	if created.opts.Group || len(created.opts.URLs) != 0 || len(created.opts.Selections) != 1 || created.opts.Selections[0].MessageIDs[0] != 2 {
		t.Fatal("redownload widened selection")
	}
	if source.opts.Group != true || len(source.files) != 2 {
		t.Fatal("source task mutated")
	}
	if err := m.RedownloadFiles(source.ID, []string{"unregistered"}); err == nil {
		t.Fatal("unregistered source accepted")
	}
	m.StopAndWait(time.Second)
}
