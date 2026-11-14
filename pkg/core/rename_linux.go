//go:build linux

package core

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldpath, newpath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_NOREPLACE)
	if errors.Is(err, syscall.EEXIST) || errors.Is(err, syscall.ENOTEMPTY) {
		return fmt.Errorf("%w: %s exists", ErrConflict, newpath)
	}
	return err
}
