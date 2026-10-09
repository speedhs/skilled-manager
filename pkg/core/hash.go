package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// HashString computes the SHA-256 hex string of a string.
func HashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// HashFile computes the SHA-256 hex string of a file's content.
func HashFile(filePath string) (string, error) {
	f, err := os.Open(filePath)
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

// HashDirectory computes a deterministic SHA-256 hex string of a directory's contents.
// It includes all files and relative paths in sorted order.
func HashDirectory(dirPath string) (string, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return HashFile(dirPath)
	}

	type fileEntry struct {
		relPath string
		hash    string
	}

	var entries []fileEntry

	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		// Normalize path separators to forward slash for cross-platform stability
		rel = filepath.ToSlash(rel)

		fileHash, err := HashFile(path)
		if err != nil {
			return fmt.Errorf("failed to hash %s: %w", path, err)
		}

		entries = append(entries, fileEntry{
			relPath: rel,
			hash:    fileHash,
		})
		return nil
	})

	if err != nil {
		return "", err
	}

	// Sort entries by relative path
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relPath < entries[j].relPath
	})

	// Hash the combined entries
	combinedHasher := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(combinedHasher, "%s:%s\n", entry.relPath, entry.hash)
	}

	return hex.EncodeToString(combinedHasher.Sum(nil)), nil
}
