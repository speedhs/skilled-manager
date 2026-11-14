//go:build darwin

package core

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldpath, newpath string) error {
	err := unix.RenameatxNp(unix.AT_FDCWD, oldpath, unix.AT_FDCWD, newpath, unix.RENAME_EXCL)
	if errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("%w: %s exists", ErrConflict, newpath)
	}
	return err
}
