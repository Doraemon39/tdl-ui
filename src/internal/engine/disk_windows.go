//go:build windows

package engine

import "golang.org/x/sys/windows"

func readDiskSpace(dir string) (diskSpace, error) {
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return diskSpace{}, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(path, &available, &total, &free); err != nil {
		return diskSpace{}, err
	}
	return diskSpace{total: total, free: free, available: available}, nil
}
