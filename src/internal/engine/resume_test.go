package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/pkg/tmessage"

	"tdl-ui/internal/manifest"
	"tdl-ui/internal/scriptapi"
)

func TestResumeDoesNotRevalidateAlreadyJournaledFiles(t *testing.T) {
	dir := t.TempDir()
	data := []byte("data")
	if err := os.WriteFile(filepath.Join(dir, "saved.mp4"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	entry := completedEntry(1)
	entry.MediaID, entry.SHA256, entry.SavedName = "document:123", hex.EncodeToString(digest[:]), "saved.mp4"
	r := manifest.NewRegistry()
	info, err := r.Create(dir, manifest.Header{AccountID: 1, DialogID: 20, DialogType: "channel"}, []manifest.Entry{entry})
	if err != nil {
		t.Fatal(err)
	}
	session, err := r.Open(info.Path, 1)
	if err != nil {
		t.Fatal(err)
	}
	key := resumeKey(20, 1, "channel")
	i, err := newIter(nil, nil, []*tmessage.Dialog{{Peer: &tg.InputPeerChannel{ChannelID: 20}, Messages: []int{1}}}, IterOptions{
		Dir: dir, Template: "{{ .FileName }}", Manifest: session, RecordManifest: true,
		LocalMatches: map[string]manifest.LocalMatch{key: {Entry: entry, Path: "saved.mp4"}},
		OnLocalDone: func(int64, string, manifest.Entry) error {
			return errors.New("already journaled file was redundantly verified and recorded")
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
		t.Fatalf("resume did not use its confirmed journal: ok=%v skip=%v err=%v", ok, skip, i.Err())
	}
}

func TestAutomaticJournalCanBeCheckedAfterRestartWithoutRewriting(t *testing.T) {
	dir := t.TempDir()
	m := newStubManager(nil)
	task := addQueuedTask(m, "automatic-resume")
	task.opts = TaskOptions{Dir: dir, AccountID: 1}
	entry := completedEntry(1)
	digest := sha256.Sum256([]byte("data"))
	entry.SHA256, entry.SavedName = hex.EncodeToString(digest[:]), "saved.mp4"
	path, err := task.recordCompletion(20, "channel", entry)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m2 := newStubManager(nil)
	resumed := addQueuedTask(m2, "automatic-resume")
	resumed.opts = task.opts
	if !resumed.completionRecorded(20, "channel", entry) {
		t.Fatal("existing automatic journal was not recognized after restart")
	}
	if resumed.completionRecorded(20, "private", entry) {
		t.Fatal("automatic journal crossed source boundaries")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("completion lookup changed the journal")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 2 { // journal and the lock from the original append
		t.Fatalf("completion lookup created a journal for another source: %v %v", files, err)
	}
}

func TestTaskActivityTracksRunStagesAndClearsAfterCompletion(t *testing.T) {
	m := newStubManager(func(_ context.Context, task *Task) error {
		if task.view().Activity == "" {
			t.Error("connection activity is missing")
		}
		task.setActivity("正在核对已有下载记录…")
		if task.view().Activity != "正在核对已有下载记录…" {
			t.Error("record-check activity is missing")
		}
		return nil
	})
	task := addQueuedTask(m, "activity")
	m.run(task)
	if v := task.view(); v.Status != StatusDone || v.Activity != "" {
		t.Fatalf("stale activity after completion: %+v", v)
	}
}

func TestResumeStillRepairsAnIncompleteJournal(t *testing.T) {
	// The fast path must not turn a missing or outdated record into a match.
	dir := t.TempDir()
	m := newStubManager(nil)
	task := addQueuedTask(m, "missing-record")
	task.opts = TaskOptions{Dir: dir, AccountID: 1}
	entry := completedEntry(1)
	digest := sha256.Sum256([]byte("data"))
	entry.SHA256, entry.SavedName = hex.EncodeToString(digest[:]), "saved.mp4"
	if task.completionRecorded(20, "channel", entry) {
		t.Fatal("missing journal was reported as complete")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("missing-record check created an empty journal")
	}
	if _, err := task.recordCompletion(20, "channel", entry); err != nil {
		t.Fatal(err)
	}
	if !task.completionRecorded(20, "channel", entry) {
		t.Fatal("repaired record was not recognized")
	}
	info := scriptapi.FileInfo{MessageDate: entry.Date}
	changed := localCompletionEntry(info, manifest.LocalMatch{Entry: entry, Path: "renamed.mp4"})
	if task.completionRecorded(20, "channel", changed) {
		t.Fatal("outdated saved name was reported as complete")
	}
}
