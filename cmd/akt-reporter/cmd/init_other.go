//go:build !unix

package cmd

import "io/fs"

// fileOwner reports no owner, because this platform has no uid and gid to keep.
func fileOwner(fs.FileInfo) (uid, gid int, ok bool) {
	return 0, 0, false
}
