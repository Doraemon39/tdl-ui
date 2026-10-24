//go:build !windows

package engine

import "os"

func publishExclusive(root *os.Root, dir, src, dst string) error {
	if err := root.Link(src, dst); err != nil {
		return err
	}
	// Publication is already complete; failure to remove a redundant private
	// temporary name must not turn a complete download into a failed record.
	_ = root.Remove(src)
	return nil
}
