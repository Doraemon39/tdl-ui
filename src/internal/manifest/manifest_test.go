package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestJournalRecoversInterruptedTailWithoutRecordingIt(t *testing.T) {
	r := NewRegistry()
	info, err := r.Create(t.TempDir(), header(), []Entry{entry(1)})
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(info.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.Write([]byte(`{"messageId":2,"mediaId":"document:`))
	f.Close()
	s, err := r.Open(info.Path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if s.Info().Done != 1 || s.Info().Ignored != 1 {
		t.Fatal("partial completion trusted")
	}
	if err := s.MarkDone(entry(3)); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewRegistry().Open(info.Path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Info().Done != 2 || loaded.Info().Ignored != 0 {
		t.Fatal("journal recovery lost confirmed entries")
	}
}

func TestDistinctRegistriesCoordinateWrites(t *testing.T) {
	info, err := NewRegistry().Create(t.TempDir(), header(), nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := NewRegistry().Open(info.Path, 42)
	b, _ := NewRegistry().Open(info.Path, 42)
	var wg sync.WaitGroup
	for id := 1; id <= 20; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s := a
			if id%2 == 0 {
				s = b
			}
			if err := s.MarkDone(entry(id)); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	loaded, err := NewRegistry().Open(info.Path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Info().Done != 20 {
		t.Fatal("concurrent registries lost entries")
	}
}

func TestConfirmedVersionTwoJSONMigratesOnAppend(t *testing.T) {
	dir := t.TempDir()
	h := header()
	h.Version = 2
	b, _ := json.Marshal(document{h, []Entry{entry(1)}})
	path := filepath.Join(dir, "old.json")
	os.WriteFile(path, b, 0o600)
	s, err := NewRegistry().Open(path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkDone(entry(2)); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewRegistry().Open(path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Info().Done != 2 || loaded.Info().Header.Version != Version {
		t.Fatal("confirmed legacy records lost")
	}
}

func entry(id int) Entry {
	return Entry{MessageID: id, MediaID: "document:123", Name: "clip.mp4", Size: 3, Date: 1700000000, State: "done", Evidence: "source"}
}
func header() Header {
	return Header{AccountID: 42, DialogID: 77, DialogType: "supergroup", Title: "test"}
}

func TestManifestAccountPeerAndMediaIsolation(t *testing.T) {
	r := NewRegistry()
	info, err := r.Create(t.TempDir(), header(), []Entry{entry(1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Open(info.Path, 43); err == nil {
		t.Fatal("foreign account accepted")
	}
	s, err := r.Open(info.Path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !s.ShouldSkip(77, "channel", 1, "document:123", 3, "") {
		t.Fatal("same namespace lost")
	}
	for _, bad := range []struct {
		d          int64
		typ, media string
		size       int64
	}{{78, "channel", "document:123", 3}, {77, "private", "document:123", 3}, {77, "channel", "document:456", 3}, {77, "channel", "document:123", 4}} {
		if s.ShouldSkip(bad.d, bad.typ, 1, bad.media, bad.size, "") {
			t.Fatal("wrong source skipped")
		}
	}
}

func TestConcurrentCompletionPersistsAllRecords(t *testing.T) {
	r := NewRegistry()
	info, err := r.Create(t.TempDir(), header(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := r.Open(info.Path, 42)
	var wg sync.WaitGroup
	for id := 1; id <= 25; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := s.MarkDone(entry(id)); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	loaded, err := NewRegistry().Open(info.Path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Info().Done != 25 {
		t.Fatal("completion records lost")
	}
	if err := s.MarkDone(Entry{MessageID: 100}); err == nil {
		t.Fatal("incomplete completion accepted")
	}
}

func TestComparisonExcludesAmbiguousAndDeletedFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "clip.mp4"), []byte("abc"), 0o600)
	entries := []Entry{entry(1), entry(2)}
	cmp, err := Compare(dir, entries, nil, EntryKey)
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Matched != 0 || cmp.Ambiguous != 2 {
		t.Fatal("duplicate names inferred")
	}
	cmp, err = Compare(dir, entries[:1], nil, EntryKey)
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Matched != 1 {
		t.Fatal("unique name/size match lost")
	}
	os.Remove(filepath.Join(dir, "clip.mp4"))
	cmp, err = Compare(dir, entries[:1], nil, EntryKey)
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Matched != 0 {
		t.Fatal("deleted file matched")
	}
}

func TestDigestRejectsSameSizeReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	os.WriteFile(path, []byte("abc"), 0o600)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	h := sha256.Sum256([]byte("abc"))
	digest := hex.EncodeToString(h[:])
	if !VerifyDigest(root, "clip.mp4", 3, digest) {
		t.Fatal("original digest rejected")
	}
	os.WriteFile(path, []byte("xyz"), 0o600)
	if VerifyDigest(root, "clip.mp4", 3, digest) {
		t.Fatal("same-size replacement accepted")
	}
}

func TestIncompleteOrLegacyEntriesNeverSkip(t *testing.T) {
	for _, e := range []Entry{{MessageID: 1, MediaID: "document:1", Name: "x", Size: 1, Date: 1, State: "pending", Evidence: "source"}, {MessageID: 1, MediaID: "document:1", Name: "x", Size: 1, Date: 1, State: "done", Evidence: "inferred"}} {
		if validEntry(e) {
			t.Fatal("unconfirmed entry accepted")
		}
	}
}
