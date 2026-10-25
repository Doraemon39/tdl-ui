//go:build !windows

package engine

import "golang.org/x/sys/unix"

func readDiskSpace(dir string) (diskSpace, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return diskSpace{}, err
	}
	return diskSpace{
		total:     uint64(st.Blocks) * uint64(st.Bsize),
		free:      uint64(st.Bfree) * uint64(st.Bsize),
		available: uint64(st.Bavail) * uint64(st.Bsize),
	}, nil
}
