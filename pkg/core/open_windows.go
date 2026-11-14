//go:build windows

package core

import (
	"fmt"
	"os"
)

// openRegular opens path after rejecting a symlink.
// Windows has no O_NOFOLLOW on os.OpenFile; the caller also Lstats first.
func openRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrInvalidPath, path)
	}
	return os.Open(path)
}
