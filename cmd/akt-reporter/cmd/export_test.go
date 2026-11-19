package cmd

import "os"

// KeepOwnerAndMode gives f the owner and permission bits of the file old describes, as init --overwrite does for the
// temporary file that replaces reporter.toml.
var KeepOwnerAndMode = keepOwnerAndMode

// SetConfigWriter makes w write reporter.toml until restore runs.
func SetConfigWriter(w func(*os.File, []byte) (int, error)) (restore func()) {
	prev := writeFileData
	writeFileData = w

	return func() { writeFileData = prev }
}

// SetACLKeeper makes keep give the temporary reporter.toml the old file's access ACL until restore runs.
func SetACLKeeper(keep func(*os.File, string) error) (restore func()) {
	prev := keepACL
	keepACL = keep

	return func() { keepACL = prev }
}
