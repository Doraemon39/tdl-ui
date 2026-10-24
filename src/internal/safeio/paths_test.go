package safeio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoundedRemovalRejectsTraversalAndLinks(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	file := filepath.Join(outside, "keep")
	os.WriteFile(file, []byte("important"), 0o600)
	if err := RemoveChild(dir, "../"+filepath.Base(outside)); err == nil {
		t.Fatal("parent traversal accepted")
	}
	if err := RemoveChild(dir, "."); err == nil {
		t.Fatal("root removal accepted")
	}
	link := filepath.Join(dir, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := RemoveChild(dir, "linked"); err == nil {
		t.Fatal("linked directory accepted")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("external file changed")
	}
}

func TestAtomicWritePreservesExistingFileOnInvalidTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	os.WriteFile(file, []byte("old"), 0o600)
	if err := WriteFile(file, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "new" {
		t.Fatal("atomic write lost contents")
	}
	if err := WriteFile(dir, []byte("bad"), 0o600); err == nil {
		t.Fatal("directory replacement accepted")
	}
}
