package services

import (
	"os"
	"path/filepath"
	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
	"tdl-ui/internal/manifest"
	"testing"
	"time"
)

func TestReviewCapacityEvictsOnlyOldestUnexpiredResult(t *testing.T) {
	s, dir, item := manifestFixture(t)
	s.reviews = make(map[string]mediaReview)
	for i := 0; i < 8; i++ {
		token := fmtCoordinate(7, i)
		s.reviews[token] = mediaReview{owner: 42, at: time.Now().Add(-time.Duration(8-i) * time.Minute)}
	}
	latest, err := s.ReviewLocalMedia(dir, 7, "channel", "fixture", []MediaItem{item})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.reviews) != 8 {
		t.Fatalf("capacity insertion discarded reviews: got %d", len(s.reviews))
	}
	if _, ok := s.reviews[fmtCoordinate(7, 0)]; ok {
		t.Fatal("oldest review was not evicted")
	}
	for i := 1; i < 8; i++ {
		if _, ok := s.reviews[fmtCoordinate(7, i)]; !ok {
			t.Fatalf("unexpired review %d was lost", i)
		}
	}
	if _, ok := s.reviews[latest.Token]; !ok {
		t.Fatal("new review is missing")
	}
}

func manifestFixture(t *testing.T) (*DownloadService, string, MediaItem) {
	t.Helper()
	cfg, err := config.NewManagerAt(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	settings := cfg.Get()
	settings.LoggedInUserID = 42
	if err := cfg.Update(settings); err != nil {
		t.Fatal(err)
	}
	m := engine.NewManager(engine.Deps{Cfg: cfg})
	s := NewDownloadService(cfg, m, nil)
	dir := t.TempDir()
	item := MediaItem{DialogID: 7, MessageID: 1, MediaID: "document:100", Name: "clip.mp4", Size: 3, Date: 1700000000}
	return s, dir, item
}

func TestReviewedExportIncludesOnlyExistingConfirmedMatches(t *testing.T) {
	s, dir, item := manifestFixture(t)
	os.WriteFile(filepath.Join(dir, item.Name), []byte("abc"), 0o600)
	missing := item
	missing.MessageID = 2
	missing.Name = "missing.mp4"
	cmp, err := s.ReviewLocalMedia(dir, 7, "channel", "fixture", []MediaItem{item, missing})
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.ExportReviewedManifest(cmp.Token, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.Done != 1 || info.Entries[0].MessageID != 1 {
		t.Fatal("export included unmatched media")
	}
	if _, err := s.ReadManifest(info.Path); err != nil {
		t.Fatal(err)
	}
}

func TestDeletedFileOrEmptyMatchNeverCreatesManifest(t *testing.T) {
	s, dir, item := manifestFixture(t)
	os.WriteFile(filepath.Join(dir, item.Name), []byte("abc"), 0o600)
	cmp, err := s.ReviewLocalMedia(dir, 7, "channel", "fixture", []MediaItem{item})
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, item.Name))
	out := t.TempDir()
	if _, err := s.ExportReviewedManifest(cmp.Token, out, nil); err == nil {
		t.Fatal("deleted match exported")
	}
	files, _ := os.ReadDir(out)
	if len(files) != 0 {
		t.Fatal("empty export created a file")
	}
}

func TestDuplicateExportRequiresExactDateAndExplicitChoice(t *testing.T) {
	s, dir, item := manifestFixture(t)
	os.WriteFile(filepath.Join(dir, item.Name), []byte("abc"), 0o600)
	other := item
	other.MessageID = 2
	other.MediaID = "document:200"
	other.Date++
	cmp, err := s.ReviewLocalMedia(dir, 7, "channel", "fixture", []MediaItem{item, other})
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Matched != 0 {
		t.Fatal("duplicate inferred")
	}
	choice := manifest.LocalChoice{MessageID: 2, Path: item.Name, MediaID: other.MediaID, Date: item.Date, Confirmed: true}
	if _, err := s.ExportReviewedManifest(cmp.Token, t.TempDir(), []manifest.LocalChoice{choice}); err == nil {
		t.Fatal("wrong date accepted")
	}
	choice.Date = other.Date
	choice.Confirmed = false
	if _, err := s.ExportReviewedManifest(cmp.Token, t.TempDir(), []manifest.LocalChoice{choice}); err == nil {
		t.Fatal("unconfirmed choice accepted")
	}
	choice.Confirmed = true
	info, err := s.ExportReviewedManifest(cmp.Token, t.TempDir(), []manifest.LocalChoice{choice})
	if err != nil {
		t.Fatal(err)
	}
	if info.Done != 1 || info.Entries[0].MessageID != 2 {
		t.Fatal("confirmation widened to other media")
	}
}

func TestReviewCannotBeExportedAfterAccountSwitch(t *testing.T) {
	s, dir, item := manifestFixture(t)
	os.WriteFile(filepath.Join(dir, item.Name), []byte("abc"), 0o600)
	cmp, err := s.ReviewLocalMedia(dir, 7, "channel", "fixture", []MediaItem{item})
	if err != nil {
		t.Fatal(err)
	}
	settings := s.cfg.Get()
	settings.LoggedInUserID = 99
	s.cfg.Update(settings)
	if _, err := s.ExportReviewedManifest(cmp.Token, t.TempDir(), nil); err == nil {
		t.Fatal("old account review reused")
	}
}

func TestPreviewAndManifestSkipDoNotDoubleCount(t *testing.T) {
	s, dir, item := manifestFixture(t)
	info, err := s.manager.Manifests().Create(t.TempDir(), manifest.Header{AccountID: 42, DialogID: 7, DialogType: "channel"}, []manifest.Entry{{MessageID: item.MessageID, MediaID: item.MediaID, Name: item.Name, Size: item.Size, Date: item.Date, State: "done", Evidence: "source"}})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewDownload(dir, 7, "channel", []MediaItem{item}, info.Path)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Total != 1 || preview.ManifestSkipped != 1 || preview.Pending != 0 {
		t.Fatal("preview double counted a manifest skip")
	}
	if _, err := s.PreviewDownload(dir, 8, "channel", []MediaItem{item}, info.Path); err == nil {
		t.Fatal("foreign candidate accepted")
	}
}
