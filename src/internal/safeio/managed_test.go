package safeio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomCacheNeverAdoptsUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "personal-video.mp4")
	os.WriteFile(file, []byte("important"), 0o600)
	if err := ClaimCache(dir, false); err == nil {
		t.Fatal("unrelated directory claimed as cache")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("unrelated media changed")
	}
}

func TestNewCustomCacheCanBeReused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "video")
	if err := ClaimCache(dir, false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "part"), []byte("cached"), 0o600)
	if err := ClaimCache(dir, false); err != nil {
		t.Fatal("owned cache could not be reused", err)
	}
}
