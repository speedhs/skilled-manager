package core

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LinkType describes the link mechanism used
type LinkType string

const (
	LinkTypeSymlink  LinkType = "symlink"
	LinkTypeJunction LinkType = "junction"
	LinkTypeCopy     LinkType = "copy"
)

// LinkResult holds the outcome of creating a link/copy
type LinkResult struct {
	Type     LinkType
	Fallback bool
	Message  string
}

// CreateSymlinkOrFallback creates a symlink from targetPath pointing to canonicalPath.
// On Windows, if symlink fails due to lack of privileges, it falls back to a directory junction (mklink /J),
// and if that fails, falls back to copying the directory.
func CreateSymlinkOrFallback(canonicalPath, targetPath string) (LinkResult, error) {
	canonicalClean := ExpandPath(canonicalPath)
	targetClean := ExpandPath(targetPath)

	// Ensure target parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetClean), 0755); err != nil {
		return LinkResult{}, fmt.Errorf("failed to create target parent directory: %w", err)
	}

	// Try standard symlink first
	err := os.Symlink(canonicalClean, targetClean)
	if err == nil {
		return LinkResult{
			Type:     LinkTypeSymlink,
			Fallback: false,
			Message:  "Symlink created successfully",
		}, nil
	}

	// On non-Windows platforms, return symlink error directly
	if runtime.GOOS != "windows" {
		return LinkResult{}, fmt.Errorf("failed to create symlink from %s to %s: %w", targetClean, canonicalClean, err)
	}

	// Windows fallback 1: Directory Junction (mklink /J)
	junctionErr := createWindowsJunction(canonicalClean, targetClean)
	if junctionErr == nil {
		return LinkResult{
			Type:     LinkTypeJunction,
			Fallback: true,
			Message:  "Symlink failed (privileges required). Created directory junction fallback.",
		}, nil
	}

	// Windows fallback 2: Directory Copy
	copyErr := CopyDirectory(canonicalClean, targetClean)
	if copyErr == nil {
		return LinkResult{
			Type:     LinkTypeCopy,
			Fallback: true,
			Message:  fmt.Sprintf("Symlink (%v) and Junction (%v) failed. Created directory copy fallback.", err, junctionErr),
		}, nil
	}

	return LinkResult{}, fmt.Errorf("all linking methods failed on Windows. Symlink err: %v, Junction err: %v, Copy err: %w", err, junctionErr, copyErr)
}

// createWindowsJunction runs 'cmd /c mklink /J <target> <source>' on Windows
func createWindowsJunction(canonicalPath, targetPath string) error {
	cmd := exec.Command("cmd", "/c", "mklink", "/J", targetPath, canonicalPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J failed: %s (err: %w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CopyDirectory recursively copies srcDir to dstDir.
func CopyDirectory(srcDir, dstDir string) error {
	srcClean := ExpandPath(srcDir)
	dstClean := ExpandPath(dstDir)

	srcInfo, err := os.Stat(srcClean)
	if err != nil {
		return fmt.Errorf("source directory does not exist: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", srcClean)
	}

	if err := os.MkdirAll(dstClean, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination directory %s: %w", dstClean, err)
	}

	entries, err := os.ReadDir(srcClean)
	if err != nil {
		return fmt.Errorf("failed to read source directory %s: %w", srcClean, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcClean, entry.Name())
		dstPath := filepath.Join(dstClean, entry.Name())

		if entry.IsDir() {
			if err := CopyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFile copies a single file from src to dst.
func CopyFile(src, dst string) error {
	srcClean := ExpandPath(src)
	dstClean := ExpandPath(dst)

	in, err := os.Open(srcClean)
	if err != nil {
		return err
	}
	defer in.Close()

	srcStat, err := in.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dstClean), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dstClean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcStat.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return nil
}

// IsSymlink checks if a path is a symbolic link or Windows reparse point
func IsSymlink(path string) (bool, string, error) {
	cleanPath := ExpandPath(path)
	lstat, err := os.Lstat(cleanPath)
	if err != nil {
		return false, "", err
	}

	if lstat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(cleanPath)
		if err != nil {
			return true, "", err
		}
		// If link target is relative, make it absolute relative to the symlink's directory
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cleanPath), target)
		}
		return true, filepath.Clean(target), nil
	}

	return false, "", nil
}

// PathsReferToSameLocation checks if path1 and path2 refer to the same filesystem location.
func PathsReferToSameLocation(path1, path2 string) bool {
	c1, err1 := filepath.Abs(ExpandPath(path1))
	c2, err2 := filepath.Abs(ExpandPath(path2))
	if err1 != nil || err2 != nil {
		return false
	}
	// On Windows, paths are case-insensitive
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(c1), filepath.Clean(c2))
	}
	return filepath.Clean(c1) == filepath.Clean(c2)
}
