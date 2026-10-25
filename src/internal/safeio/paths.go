// Package safeio confines application writes and cleanup to explicit local paths.
package safeio

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Equal(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func Within(parent, child string) bool {
	if Equal(parent, child) {
		return true
	}
	if runtime.GOOS == "windows" {
		parent, child = strings.ToLower(parent), strings.ToLower(child)
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && filepath.IsLocal(rel)
}

func Overlap(a, b string) bool { return Within(a, b) || Within(b, a) }

// MkdirPrivate gives new data roots an owner-only boundary on Unix and an
// explicit owner/System/Administrators ACL on Windows. Existing ACLs are kept.
func MkdirPrivate(dir string) error {
	if err := CheckPath(dir); err != nil {
		return err
	}
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return errors.New("数据目录不是普通目录")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if err := mkdirPrivate(dir); err != nil && !os.IsExist(err) {
		return err
	}
	return CheckPath(dir)
}

// CheckPath rejects symlinks and Windows junctions in existing path components.
// A missing suffix is permitted so callers can safely create a new directory.
func CheckPath(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("必须使用绝对路径")
	}
	path = filepath.Clean(path)
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && reparsePoint(p, info) {
			return errors.New("路径包含符号链接或目录联接，拒绝操作")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}

// RemoveChild never follows a link and cannot operate on the root itself.
func RemoveChild(dir, name string) error {
	if !filepath.IsLocal(name) || name == "." {
		return errors.New("清理路径越界")
	}
	if err := CheckPath(filepath.Join(dir, name)); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(name)
}

// WriteFile replaces one file after its complete contents reach disk.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := CheckPath(path); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tdl-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
