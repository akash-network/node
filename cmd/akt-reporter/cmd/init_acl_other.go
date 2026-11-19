//go:build !linux

package cmd

import "os"

// keepAccessACL keeps no ACL: outside Linux this build has no ACL API, so a replaced reporter.toml takes only the
// old file's mode, owner and group. An ACL on the old file is dropped, deny entries included, and the replacement
// may inherit entries from its directory.
func keepAccessACL(*os.File, string) error { return nil }
