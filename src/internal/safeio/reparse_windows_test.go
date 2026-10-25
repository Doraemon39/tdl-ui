package safeio

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

type reparseInfo struct {
	os.FileInfo
	mode       os.FileMode
	attributes uint32
}

func (i reparseInfo) Mode() os.FileMode { return i.mode }
func (i reparseInfo) Sys() any          { return &syscall.Win32FileAttributeData{FileAttributes: i.attributes} }

func TestWindowsPathChecksDistinguishCloudFromRedirects(t *testing.T) {
	info := reparseInfo{mode: os.ModeDir | os.ModeIrregular, attributes: syscall.FILE_ATTRIBUTE_REPARSE_POINT | syscall.FILE_ATTRIBUTE_DIRECTORY}
	for n := uint32(0); n < 16; n++ {
		tag := uint32(0x9000001a) | (n << 12)
		if reparsePointWithTag(info, func() (uint32, error) { return tag, nil }) {
			t.Fatalf("cloud tag %#x was rejected", tag)
		}
	}
	for _, tag := range []uint32{0xa000000c, 0xa0000003, 0xa000001d, 0x8000001b, 0xb000001a, 0} {
		if !reparsePointWithTag(info, func() (uint32, error) { return tag, nil }) {
			t.Fatalf("redirect or unknown tag %#x accepted", tag)
		}
	}
	if !reparsePointWithTag(info, func() (uint32, error) { return 0, errors.New("metadata unavailable") }) {
		t.Fatal("tag lookup failure accepted")
	}
	ordinary := reparseInfo{}
	if reparsePointWithTag(ordinary, func() (uint32, error) { t.Fatal("queried an ordinary file"); return 0, nil }) {
		t.Fatal("ordinary path rejected")
	}
	link := reparseInfo{mode: os.ModeSymlink, attributes: syscall.FILE_ATTRIBUTE_REPARSE_POINT}
	if !reparsePointWithTag(link, func() (uint32, error) { t.Fatal("symlink mode should be rejected directly"); return 0x9000001a, nil }) {
		t.Fatal("symlink was accepted")
	}
}
