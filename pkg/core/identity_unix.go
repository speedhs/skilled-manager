//go:build unix

package core

import (
	"golang.org/x/sys/unix"
)

func fileIdentity(path string) (FileID, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return FileID{}, err
	}
	return FileID{Dev: uint64(st.Dev), Ino: st.Ino, Valid: true}, nil
}

func identityFromFd(fd int) (FileID, unix.Stat_t, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return FileID{}, unix.Stat_t{}, err
	}
	return FileID{Dev: uint64(st.Dev), Ino: st.Ino, Valid: true}, st, nil
}
