package manifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCompletionLookupRequiresMatchingConfirmedRecord(t *testing.T) {
	e := entry(1)
	digest := sha256.Sum256([]byte("abc"))
	e.SHA256, e.SavedName = hex.EncodeToString(digest[:]), "saved.mp4"
	r := NewRegistry()
	info, err := r.Create(t.TempDir(), header(), []Entry{e})
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.Open(info.Path, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasCompletion(77, "channel", e) {
		t.Fatal("confirmed journal entry was not recognized")
	}
	for _, tc := range []struct {
		name   string
		change func(*Entry)
	}{
		{"missing digest", func(e *Entry) { e.SHA256 = "" }},
		{"different digest", func(e *Entry) { e.SHA256 = strings.Repeat("0", 64) }},
		{"different media", func(e *Entry) { e.MediaID = "document:456" }},
		{"different size", func(e *Entry) { e.Size++ }},
		{"different original name", func(e *Entry) { e.Name = "other.mp4" }},
		{"different saved name", func(e *Entry) { e.SavedName = "other.mp4" }},
		{"different date", func(e *Entry) { e.Date++ }},
		{"unfinished", func(e *Entry) { e.State = "pending" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := e
			tc.change(&changed)
			if s.HasCompletion(77, "channel", changed) {
				t.Fatal("mismatched completion was accepted")
			}
		})
	}
	if s.HasCompletion(78, "channel", e) || s.HasCompletion(77, "private", e) {
		t.Fatal("completion lookup crossed source boundaries")
	}
}

func TestOpeningExistingTaskDoesNotCreateEmptyJournal(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewRegistry().OpenExistingTask(dir, "resume", header()); !os.IsNotExist(err) {
		t.Fatalf("expected missing journal: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("resume check created files: %v %v", files, err)
	}
}

func TestDigestVerificationStopsBetweenReadsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := digestReader{ctx: ctx, reader: strings.NewReader("abcdef")}
	var buf [1]byte
	if n, err := reader.Read(buf[:]); n != 1 || err != nil {
		t.Fatalf("first read failed: %d %v", n, err)
	}
	cancel()
	if n, err := reader.Read(buf[:]); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled verification continued: %d %v", n, err)
	}
	if n, err := io.Copy(io.Discard, reader); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled hash continued: %d %v", n, err)
	}
}
