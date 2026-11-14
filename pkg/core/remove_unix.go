//go:build unix

package core

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// removeTreeNoFollow deletes path without following symlinks.
// When id is valid, the directory entry at path must still be that object
// or nothing is deleted.
func removeTreeNoFollow(path string, id FileID) error {
	parent, name := splitParent(path)
	pfd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer closeFD(pfd)

	fd, err := unix.Openat(pfd, name, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EPERM) {
			return fmt.Errorf("%w: %s is a symlink", ErrConflict, path)
		}
		return err
	}
	got, st, err := identityFromFd(fd)
	if err != nil {
		closeFD(fd)
		return err
	}
	if id.Valid && !got.same(id) {
		closeFD(fd)
		return fmt.Errorf("%w: identity changed at %s", ErrConflict, path)
	}
	if st.Mode&unix.S_IFMT == unix.S_IFDIR {
		if err := removeChildren(fd); err != nil {
			closeFD(fd)
			return err
		}
	}
	closeFD(fd)
	return unlinkIfSame(pfd, name, st)
}

func removeChildren(dirfd int) error {
	names, err := dirNames(dirfd)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := removeChild(dirfd, name); err != nil {
			return err
		}
	}
	return nil
}

func dirNames(fd int) ([]string, error) {
	var names []string
	buf := make([]byte, 8192)
	for {
		n, err := unix.ReadDirent(fd, buf)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		rest := buf[:n]
		for len(rest) > 0 {
			var batch []string
			consumed, _, batch := unix.ParseDirent(rest, 1<<20, batch)
			if consumed <= 0 {
				break
			}
			names = append(names, batch...)
			rest = rest[consumed:]
		}
	}
	return names, nil
}

func removeChild(dirfd int, name string) error {
	fd, err := unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EPERM) || errors.Is(err, unix.ENOTDIR) {
			var st unix.Stat_t
			if lerr := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); lerr != nil {
				return err
			}
			if st.Mode&unix.S_IFMT == unix.S_IFLNK {
				return unix.Unlinkat(dirfd, name, 0)
			}
		}
		return err
	}
	_, st, serr := identityFromFd(fd)
	if serr != nil {
		closeFD(fd)
		return serr
	}
	if st.Mode&unix.S_IFMT == unix.S_IFDIR {
		if err := removeChildren(fd); err != nil {
			closeFD(fd)
			return err
		}
		closeFD(fd)
		return unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR)
	}
	closeFD(fd)
	return unix.Unlinkat(dirfd, name, 0)
}

func closeFD(fd int) {
	// The operation result is already decided. A close error cannot restore the file.
	_ = unix.Close(fd) //nolint:errcheck // #nosec G104 -- close after the result is decided
}

func unlinkIfSame(parentfd int, name string, expect unix.Stat_t) error {
	var st unix.Stat_t
	if err := unix.Fstatat(parentfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if st.Ino != expect.Ino || st.Dev != expect.Dev {
		return fmt.Errorf("%w: path changed before delete", ErrConflict)
	}
	flags := 0
	if expect.Mode&unix.S_IFMT == unix.S_IFDIR {
		flags = unix.AT_REMOVEDIR
	}
	return unix.Unlinkat(parentfd, name, flags)
}
