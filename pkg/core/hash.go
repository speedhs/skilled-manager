package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// HashIgnoreNames are path components omitted from content hashes.
// The hash is of file bytes and relative names only. mtime and permissions
// are not included. Symlinks contribute their target string, not the
// target's bytes. .git directories are not walked.
var HashIgnoreNames = map[string]struct{}{
	".git":      {},
	".DS_Store": {},
	"Thumbs.db": {},
}

// HashString returns the hex SHA-256 of s.
func HashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HashFile streams the SHA-256 of a regular file.
// A symlink is hashed as its link text, not the file it points at.
func HashFile(filePath string) (string, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(filePath)
		if err != nil {
			return "", err
		}
		return HashString("link:" + target), nil
	}
	f, err := openRegular(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashDirectory returns a deterministic content hash of dirPath.
// See HashIgnoreNames for what is left out.
func HashDirectory(dirPath string) (string, error) {
	info, err := os.Lstat(dirPath)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return HashFile(dirPath)
	}

	type entry struct {
		rel  string
		hash string
	}
	var entries []entry
	err = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dirPath {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if _, skip := HashIgnoreNames[name]; skip {
				return fs.SkipDir
			}
			return nil
		}
		if _, skip := HashIgnoreNames[name]; skip {
			return nil
		}
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		sum, err := HashFile(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", path, err)
		}
		entries = append(entries, entry{rel: rel, hash: sum})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s:%s\n", e.rel, e.hash)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
