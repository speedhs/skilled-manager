package core

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ImportZip copies skills from a zip into the canonical store.
// It rejects absolute paths, "..", symlink entries, unsafe modes, and archives
// that exceed the size, count, or compression-ratio limits. Existing skills
// are left unchanged.
func (m *Manager) ImportZip(ctx context.Context, r io.ReaderAt, size int64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}
	if err := vetZip(zr); err != nil {
		return nil, err
	}
	var imported []string
	err = m.mutate(ctx, func() error {
		if m.dryRun {
			imported = append(imported, zipSkillNames(zr)...)
			return nil
		}
		staging := filepath.Join(m.ConfigDir, "staging", "zip-"+randHex(4))
		if err := os.MkdirAll(staging, 0o755); err != nil {
			return err
		}
		defer os.RemoveAll(staging)
		if err := extractZip(zr, staging); err != nil {
			return err
		}
		entries, err := os.ReadDir(staging)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() {
				continue
			}
			if err := ValidateSkillName(entry.Name()); err != nil {
				return err
			}
			dest, err := skillDir(m.CanonicalDir, entry.Name())
			if err != nil {
				return err
			}
			if _, err := os.Lstat(dest); err == nil {
				return fmt.Errorf("%w: %s", ErrExists, entry.Name())
			}
			if _, err := ParseSkill(filepath.Join(staging, entry.Name())); err != nil {
				return fmt.Errorf("%w: %s", err, entry.Name())
			}
			if err := m.publishNew(filepath.Join(staging, entry.Name()), dest); err != nil {
				return err
			}
			imported = append(imported, entry.Name())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return imported, nil
}

func vetZip(zr *zip.Reader) error {
	var unpacked uint64
	if len(zr.File) > maxZipEntries {
		return fmt.Errorf("%w: zip has more than %d entries", ErrInvalidPath, maxZipEntries)
	}
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" {
			continue
		}
		if _, err := safeZipName(name); err != nil {
			return err
		}
		if err := rejectZipMode(f.Mode()); err != nil {
			return err
		}
		if f.UncompressedSize64 > maxZipFileBytes {
			return fmt.Errorf("%w: zip entry %s is too large", ErrInvalidPath, f.Name)
		}
		unpacked += f.UncompressedSize64
		if err := zipTooLarge(len(zr.File), unpacked); err != nil {
			return err
		}
		if err := zipRatio(f.CompressedSize64, f.UncompressedSize64); err != nil {
			return err
		}
	}
	return nil
}

func extractZip(zr *zip.Reader, dest string) error {
	var unpacked uint64
	for i, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" {
			continue
		}
		clean, err := safeZipName(name)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(clean))
		if err := ensureWithin(dest, target); err != nil {
			return err
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		n, err := writeLimited(target, rc, maxZipFileBytes)
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		size, err := nonNegative(n)
		if err != nil {
			return err
		}
		unpacked += size
		if err := zipTooLarge(i+1, unpacked); err != nil {
			return err
		}
	}
	return nil
}

func writeLimited(path string, r io.Reader, limit int64) (int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidPath, path, limit)
	}
	return n, f.Sync()
}

func zipSkillNames(zr *zip.Reader) []string {
	seen := map[string]struct{}{}
	var names []string
	for _, f := range zr.File {
		clean, err := safeZipName(strings.TrimSuffix(f.Name, "/"))
		if err != nil {
			continue
		}
		top := clean
		if i := strings.IndexByte(clean, '/'); i >= 0 {
			top = clean[:i]
		}
		top = path.Base(top)
		if _, ok := seen[top]; ok {
			continue
		}
		if ValidateSkillName(top) != nil {
			continue
		}
		seen[top] = struct{}{}
		names = append(names, top)
	}
	return names
}
