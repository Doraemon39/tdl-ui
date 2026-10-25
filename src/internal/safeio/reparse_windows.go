package safeio

import (
	"golang.org/x/sys/windows"
	"os"
	"syscall"
	"unsafe"
)

func reparsePoint(path string, info os.FileInfo) bool {
	return reparsePointWithTag(info, func() (uint32, error) {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return 0, err
		}
		var data windows.Win32finddata
		h, err := windows.FindFirstFile(name, &data)
		if err != nil {
			return 0, err
		}
		defer windows.FindClose(h)
		return data.Reserved0, nil
	})
}

func reparsePointWithTag(info os.FileInfo, readTag func() (uint32, error)) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return info.Mode()&os.ModeIrregular != 0
	}
	tag, err := readTag()
	// CLOUD and CLOUD_1..F are data placeholders, not namespace redirects.
	// Keep rejecting name-surrogate and unknown tags, including mount points.
	return err != nil || tag&^uint32(0x0000f000) != 0x9000001a
}

func mkdirPrivate(dir string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(sa))
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	return windows.CreateDirectory(path, &sa)
}
