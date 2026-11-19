//go:build unix

package cmd

import (
	"io/fs"
	"syscall"
)

// fileOwner returns the uid and gid of the file info describes.
func fileOwner(info fs.FileInfo) (uid, gid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}

	return int(st.Uid), int(st.Gid), true
}
