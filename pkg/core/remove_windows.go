//go:build windows

package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func removeTreeNoFollow(path string, id FileID) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if isReparse(info) {
		return fmt.Errorf("%w: %s is a reparse point", ErrConflict, path)
	}
	got, err := fileIdentity(path)
	if err != nil {
		return err
	}
	if id.Valid && !got.same(id) {
		return fmt.Errorf("%w: identity changed at %s", ErrConflict, path)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			cinfo, err := os.Lstat(child)
			if err != nil {
				return err
			}
			if isReparse(cinfo) {
				if err := os.Remove(child); err != nil {
					return err
				}
				continue
			}
			if err := removeTreeNoFollow(child, FileID{}); err != nil {
				return err
			}
		}
		again, err := fileIdentity(path)
		if err != nil {
			return err
		}
		if id.Valid && !again.same(id) {
			return fmt.Errorf("%w: identity changed at %s", ErrConflict, path)
		}
	}
	return os.Remove(path)
}

func isReparse(info fs.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	raw, ok := info.Sys().(*windows.Win32FileAttributeData)
	if !ok || raw == nil {
		return false
	}
	return raw.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
