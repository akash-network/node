package cmd

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// aclXattr is the extended attribute that holds a file's POSIX access ACL.
const aclXattr = "system.posix_acl_access"

// keepAccessACL gives f the POSIX access ACL of the file at path, or none when that file has none, though f may have
// inherited one from its directory's default ACL. The mode alone does not carry an ACL: with one, the group mode
// bits are the ACL mask, which a file without the ACL would grant to its owning group. A filesystem without POSIX
// ACLs has none to copy; an NFSv4 ACL, which is kept in another attribute, is not copied.
func keepAccessACL(f *os.File, path string) error {
	fd := int(f.Fd())

	acl, err := getxattr(path, aclXattr)
	switch {
	case errors.Is(err, unix.EOPNOTSUPP):
		return nil
	case errors.Is(err, unix.ENODATA):
		return dropInheritedACL(fd)
	case err != nil:
		return err
	}

	return unix.Fsetxattr(fd, aclXattr, acl, 0)
}

// dropInheritedACL removes the access ACL that the file fd inherited, if any. Removing one takes owning the file,
// which keepOwnerAndMode may have given away, so a file without one is left alone.
func dropInheritedACL(fd int) error {
	switch _, err := unix.Fgetxattr(fd, aclXattr, nil); {
	case errors.Is(err, unix.ENODATA), errors.Is(err, unix.EOPNOTSUPP):
		return nil
	case err != nil:
		return err
	}

	if err := unix.Fremovexattr(fd, aclXattr); err != nil && !errors.Is(err, unix.ENODATA) {
		return err
	}

	return nil
}

// getxattr reads the extended attribute name of path.
func getxattr(path, name string) ([]byte, error) {
	for {
		n, err := unix.Getxattr(path, name, nil)
		if err != nil {
			return nil, err
		}

		buf := make([]byte, n)
		n, err = unix.Getxattr(path, name, buf)
		if errors.Is(err, unix.ERANGE) {
			continue // the attribute grew between the calls
		}
		if err != nil {
			return nil, err
		}

		return buf[:n], nil
	}
}
