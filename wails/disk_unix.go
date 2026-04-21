//go:build !windows

package main

import "syscall"

// diskFree returns (available bytes, total bytes, nil) for the filesystem
// hosting path. Used by the free-space guard before accepting a session.
// Uses statfs on POSIX: available is Bavail * Bsize (space the user can
// actually write to, honouring the reserved-blocks quota root enjoys).
func diskFree(path string) (uint64, uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := uint64(st.Bsize)
	return uint64(st.Bavail) * bsize, uint64(st.Blocks) * bsize, nil
}
