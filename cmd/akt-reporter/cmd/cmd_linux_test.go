package cmd_test

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"pkg.akt.dev/node/v2/operator/reporter/config"
)

// Linux keeps POSIX ACLs in these extended attributes, as version 2 followed by a tag, permissions and ID per entry,
// little-endian (include/uapi/linux/posix_acl_xattr.h).
const (
	accessACLXattr  = "system.posix_acl_access"
	defaultACLXattr = "system.posix_acl_default"

	aclUserObj  = 0x01
	aclUser     = 0x02
	aclGroupObj = 0x04
	aclMask     = 0x10
	aclOther    = 0x20
	aclNoID     = 0xffffffff
)

type aclEntry struct {
	tag, perm uint16
	id        uint32
}

func posixACL(entries ...aclEntry) []byte {
	b := binary.LittleEndian.AppendUint32(nil, 2)
	for _, e := range entries {
		b = binary.LittleEndian.AppendUint16(b, e.tag)
		b = binary.LittleEndian.AppendUint16(b, e.perm)
		b = binary.LittleEndian.AppendUint32(b, e.id)
	}

	return b
}

// readsAs4242 is the ACL that setfacl -m u:4242:r gives a 0600 file: user 4242 reads it, the owning group does not,
// and the group mode bits show the mask.
var readsAs4242 = posixACL(
	aclEntry{aclUserObj, 6, aclNoID},
	aclEntry{aclUser, 4, 4242},
	aclEntry{aclGroupObj, 0, aclNoID},
	aclEntry{aclMask, 4, aclNoID},
	aclEntry{aclOther, 0, aclNoID},
)

// accessACL returns the access ACL of path; nil means it has none.
func accessACL(t *testing.T, path string) []byte {
	t.Helper()

	buf := make([]byte, 1024)
	n, err := unix.Getxattr(path, accessACLXattr, buf)
	if errors.Is(err, unix.ENODATA) {
		return nil
	}
	require.NoError(t, err)

	return buf[:n]
}

// effectiveCaps returns the capabilities the process holds, from /proc/self/status.
func effectiveCaps(t *testing.T) uint64 {
	t.Helper()

	data, err := os.ReadFile("/proc/self/status")
	require.NoError(t, err)
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "CapEff:"); ok {
			caps, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
			require.NoError(t, err)
			return caps
		}
	}
	t.Fatal("/proc/self/status reports no CapEff")

	return 0
}

// setACL sets an ACL attribute, skipping the test where the filesystem has no POSIX ACLs.
func setACL(t *testing.T, path, attr string, acl []byte) {
	t.Helper()

	err := unix.Setxattr(path, attr, acl, 0)
	if errors.Is(err, unix.EOPNOTSUPP) {
		t.Skip("the filesystem has no POSIX ACLs")
	}
	require.NoError(t, err)
}

func TestInitOverwriteKeepsAccessACL(t *testing.T) {
	seed := func(t *testing.T) (home, path string) { // an old 0600 reporter.toml in a fresh home
		home = t.TempDir()
		path = config.FilePath(home)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("old = 1\n"), 0o600))
		return home, path
	}
	mode := func(t *testing.T, path string) os.FileMode {
		info, err := os.Stat(path)
		require.NoError(t, err)
		return info.Mode().Perm()
	}

	t.Run("keeps the file's access ACL", func(t *testing.T) {
		home, path := seed(t)
		setACL(t, path, accessACLXattr, readsAs4242)
		want := accessACL(t, path)
		require.Equal(t, os.FileMode(0o640), mode(t, path))

		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite")
		require.Equal(t, 0, code, stderr)
		require.Equal(t, want, accessACL(t, path), "user 4242 still reads the file and the owning group still does not")
		require.Equal(t, os.FileMode(0o640), mode(t, path))
	})
	t.Run("drops an ACL the old file did not have", func(t *testing.T) {
		home, path := seed(t)
		require.NoError(t, os.Chmod(path, 0o640)) //nolint:gosec // group-readable, so an inherited mask lets user 4242 read
		// Set after the file was written, the directory's default ACL would let user 4242 read a new file there.
		setACL(t, filepath.Dir(path), defaultACLXattr, readsAs4242)
		require.Nil(t, accessACL(t, path))

		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite")
		require.Equal(t, 0, code, stderr)
		require.Nil(t, accessACL(t, path), "user 4242 gets no access the old file did not give")
		require.Equal(t, os.FileMode(0o640), mode(t, path))
	})
	t.Run("a file without an ACL is replaced without CAP_FOWNER", func(t *testing.T) {
		const capChown, capFowner = 1 << 0, 1 << 3
		if caps := effectiveCaps(t); caps&capChown == 0 || caps&capFowner != 0 {
			t.Skip("needs CAP_CHOWN without CAP_FOWNER, as under capsh --drop=cap_fowner")
		}
		home, path := seed(t)
		require.NoError(t, os.Chown(path, 4242, -1)) // the replacement is given to user 4242, so init no longer owns it

		code, _, stderr := execute(t, "", "init", "--home", home, "--overwrite", "--chain-id", "other-1")
		require.Equal(t, 0, code, stderr)
		data, err := os.ReadFile(path) //nolint:gosec // a test directory
		require.NoError(t, err)
		require.Contains(t, string(data), `chain-id = "other-1"`)
	})
}
