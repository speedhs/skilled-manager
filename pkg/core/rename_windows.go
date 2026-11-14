//go:build windows

package core

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

func renameNoReplace(oldpath, newpath string) error {
	from, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}
	// No MOVEFILE_REPLACE_EXISTING: an existing destination is left untouched.
	err = windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
		return fmt.Errorf("%w: %s exists", ErrConflict, newpath)
	}
	// publishNew copies across devices when it sees EXDEV.
	if errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) {
		return syscall.EXDEV
	}
	return err
}
