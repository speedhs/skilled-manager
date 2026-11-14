package core

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxZipEntries       = 10000
	maxZipUncompressed  = 512 << 20
	maxZipFileBytes     = 64 << 20
	maxZipRatio         = 100
	zipRatioMinUnpacked = 1 << 20
)

func safeZipName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty zip entry", ErrInvalidPath)
	}
	if strings.Contains(name, `\`) || strings.Contains(name, "\x00") {
		return "", fmt.Errorf("%w: zip entry %q", ErrInvalidPath, name)
	}
	if path.IsAbs(name) || filepath.IsAbs(name) {
		return "", fmt.Errorf("%w: absolute zip entry %q", ErrInvalidPath, name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: zip entry %q escapes the archive", ErrInvalidPath, name)
	}
	return clean, nil
}

func rejectZipMode(mode os.FileMode) error {
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: zip contains a symlink", ErrInvalidPath)
	}
	if mode&os.ModeSetuid != 0 || mode&os.ModeSetgid != 0 || mode&os.ModeDevice != 0 {
		return fmt.Errorf("%w: zip entry has an unsafe mode", ErrInvalidPath)
	}
	return nil
}

func zipTooLarge(entries int, uncompressed uint64) error {
	if entries > maxZipEntries {
		return fmt.Errorf("%w: zip has more than %d entries", ErrInvalidPath, maxZipEntries)
	}
	if uncompressed > maxZipUncompressed {
		return fmt.Errorf("%w: zip uncompressed size exceeds %d bytes", ErrInvalidPath, maxZipUncompressed)
	}
	return nil
}

func zipRatio(compressed, uncompressed uint64) error {
	if uncompressed < zipRatioMinUnpacked || compressed == 0 {
		return nil
	}
	if uncompressed/compressed > maxZipRatio {
		return fmt.Errorf("%w: zip compression ratio exceeds %d", ErrInvalidPath, maxZipRatio)
	}
	return nil
}
