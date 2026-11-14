package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LinkType is how a deployment was materialized.
type LinkType string

const (
	// LinkTypeSymlink is a symbolic link.
	LinkTypeSymlink LinkType = "symlink"
	// LinkTypeJunction is a Windows directory junction.
	LinkTypeJunction LinkType = "junction"
	// LinkTypeCopy is a real directory or file copy.
	LinkTypeCopy LinkType = "copy"
)

// LinkResult describes a link that was created.
type LinkResult struct {
	Type     LinkType
	Fallback bool
	Message  string
}

// CreateSymlinkOrFallback links targetPath at canonicalPath.
// On non-Windows systems a symlink failure is returned. On Windows a
// failure falls back to a directory junction and then a copy.
func CreateSymlinkOrFallback(canonicalPath, targetPath string) (LinkResult, error) {
	canonicalClean, err := ExpandPath(canonicalPath)
	if err != nil {
		return LinkResult{}, err
	}
	targetClean, err := ExpandPath(targetPath)
	if err != nil {
		return LinkResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetClean), 0o755); err != nil {
		return LinkResult{}, err
	}
	if err := os.Symlink(canonicalClean, targetClean); err != nil {
		res, ferr := linkFallback(osFS{}, canonicalClean, targetClean, err)
		if ferr != nil && runtime.GOOS != "windows" {
			return LinkResult{}, fmt.Errorf("symlink %s: %w", targetClean, ferr)
		}
		return res, ferr
	}
	return LinkResult{Type: LinkTypeSymlink, Message: "symlink created"}, nil
}

// CopyDirectory copies srcDir to dstDir without following symlinks.
// Absolute symlinks and links that point outside srcDir are rejected.
func CopyDirectory(srcDir, dstDir string) error {
	src, err := ExpandPath(srcDir)
	if err != nil {
		return err
	}
	dst, err := ExpandPath(dstDir)
	if err != nil {
		return err
	}
	return copyTree(osFS{}, src, src, dst)
}

// CopyFile copies one regular file. Symlinks are not followed.
func CopyFile(src, dst string) error {
	srcClean, err := ExpandPath(src)
	if err != nil {
		return err
	}
	dstClean, err := ExpandPath(dst)
	if err != nil {
		return err
	}
	info, err := os.Lstat(srcClean)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: refusing to follow symlink %s", ErrInvalidPath, srcClean)
	}
	return copyFileStream(srcClean, dstClean)
}

// IsSymlink reports whether path is a symlink and, when it is, the
// absolute cleaned target. The target is not followed further.
func IsSymlink(path string) (bool, string, error) {
	clean, err := ExpandPath(path)
	if err != nil {
		return false, "", err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return false, "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, "", nil
	}
	target, err := os.Readlink(clean)
	if err != nil {
		return true, "", err
	}
	return true, resolveLink(clean, target), nil
}

// PathsReferToSameLocation reports whether two paths are the same
// lexical path. Darwin and Windows compare case-insensitively.
// Symlinks are not followed.
func PathsReferToSameLocation(path1, path2 string) bool {
	a, err1 := ExpandPath(path1)
	b, err2 := ExpandPath(path2)
	if err1 != nil || err2 != nil {
		return false
	}
	return samePath(a, b)
}

func safeWindowsArg(path string) error {
	if strings.ContainsAny(path, "\r\n\"&|<>^") {
		return fmt.Errorf("%w: path contains a shell metacharacter", ErrInvalidPath)
	}
	return nil
}
