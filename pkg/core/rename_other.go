//go:build unix && !linux && !darwin

package core

import (
	"fmt"
	"os"
)

// renameNoReplace is a best-effort exclusive rename for Unix systems other
// than Linux and Darwin. The check and the rename are not atomic there.
func renameNoReplace(oldpath, newpath string) error {
	if _, err := os.Lstat(newpath); err == nil {
		return fmt.Errorf("%w: %s exists", ErrConflict, newpath)
	}
	return os.Rename(oldpath, newpath)
}
