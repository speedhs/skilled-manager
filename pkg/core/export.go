package core

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ExportZip writes a zip of the named skills, or every canonical skill when
// skillNames is empty. Symlinks are rejected. Entry names stay inside the archive.
func (m *Manager) ExportZip(ctx context.Context, skillNames []string, writer io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := skillNames
	if len(names) == 0 {
		skills, err := ListSkills(m.CanonicalDir)
		if err != nil {
			return err
		}
		for _, s := range skills {
			names = append(names, s.Name)
		}
	}
	zw := zip.NewWriter(writer)
	entries := 0
	var unpacked uint64
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			_ = zw.Close()
			return err
		}
		dir, err := skillDir(m.CanonicalDir, name)
		if err != nil {
			_ = zw.Close()
			return err
		}
		info, err := os.Lstat(dir)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("%w: skill %s", ErrNotFound, name)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			_ = zw.Close()
			return fmt.Errorf("%w: %s is not a skill directory", ErrInvalidPath, dir)
		}
		err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(m.CanonicalDir, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			clean, err := safeZipName(rel)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if err := rejectZipMode(info.Mode()); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			entries++
			if d.IsDir() {
				if err := zipTooLarge(entries, unpacked); err != nil {
					return err
				}
				_, err := zw.Create(clean + "/")
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: %s is a symlink", ErrInvalidPath, path)
			}
			size, err := nonNegative(info.Size())
			if err != nil {
				return err
			}
			if size > maxZipFileBytes {
				return fmt.Errorf("%w: %s is larger than %d bytes", ErrInvalidPath, path, maxZipFileBytes)
			}
			unpacked += size
			if err := zipTooLarge(entries, unpacked); err != nil {
				return err
			}
			hdr, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = clean
			hdr.Method = zip.Deflate
			hdr.SetMode(0o644)
			w, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			f, err := openRegular(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(w, f)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}
