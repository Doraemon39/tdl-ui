//go:build !windows

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnixExclusivePublicationPreservesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(dir, "source.tmp"), []byte("download"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishExclusive(root, dir, "source.tmp", "target"); !os.IsExist(err) {
		t.Fatalf("collision error=%v", err)
	}
	for name, want := range map[string]string{"source.tmp": "download", "target": "existing"} {
		b, err := root.ReadFile(name)
		if err != nil || string(b) != want {
			t.Fatalf("%s changed: %q, %v", name, b, err)
		}
	}
}

func TestUnixExclusivePublicationFollowsRootAfterDirectoryMove(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "original")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.WriteFile(filepath.Join(dir, "source.tmp"), []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := publishExclusive(root, dir, "source.tmp", "target"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(moved, "target")); err != nil || string(b) != "complete" {
		t.Fatalf("publication failed: %q, %v", b, err)
	}
	if _, err := root.Lstat("source.tmp"); !os.IsNotExist(err) {
		t.Fatal("completed temporary file retained")
	}
}
