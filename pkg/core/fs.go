package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// FileID is the identity of a directory entry, not of a symlink target.
type FileID struct {
	Dev   uint64
	Ino   uint64
	Valid bool
}

func (id FileID) same(other FileID) bool {
	return id.Valid && other.Valid && id.Dev == other.Dev && id.Ino == other.Ino
}

// fileSystem is the disk port. Tests substitute faultFS.
type fileSystem interface {
	Lstat(name string) (fs.FileInfo, error)
	Readlink(name string) (string, error)
	Symlink(oldname, newname string) error
	Remove(name string) error
	MkdirAll(path string, perm fs.FileMode) error
	ReadDir(name string) ([]os.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	Open(name string) (io.ReadCloser, error)
	WriteFileAtomic(name string, data []byte, perm fs.FileMode) error
	WriteFileExclusive(name string, data []byte, perm fs.FileMode) error
	Rename(oldpath, newpath string) error
	RenameExclusive(oldpath, newpath string) error
	CopyFile(src, dst string) error
	RemoveTree(path string, id FileID) error
	Identity(path string) (FileID, error)
}

type osFS struct{}

func (osFS) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }

func (osFS) Readlink(name string) (string, error) { return os.Readlink(name) }

func (osFS) Symlink(oldname, newname string) error { return os.Symlink(oldname, newname) }

func (osFS) Remove(name string) error { return os.Remove(name) }

func (osFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }

func (osFS) ReadDir(name string) ([]os.DirEntry, error) { return os.ReadDir(name) }

func (osFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

func (osFS) Open(name string) (io.ReadCloser, error) { return os.Open(name) }

func (osFS) WriteFileAtomic(name string, data []byte, perm fs.FileMode) error {
	return writeAtomic(name, data, perm)
}

func (osFS) WriteFileExclusive(name string, data []byte, perm fs.FileMode) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(name)
		return werr
	}
	if serr != nil {
		_ = os.Remove(name)
		return serr
	}
	if cerr != nil {
		_ = os.Remove(name)
		return cerr
	}
	return nil
}

func (osFS) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }

func (osFS) RenameExclusive(oldpath, newpath string) error { return renameNoReplace(oldpath, newpath) }

func (osFS) CopyFile(src, dst string) error { return copyFileStream(src, dst) }

func (osFS) RemoveTree(path string, id FileID) error { return removeTreeNoFollow(path, id) }

func (osFS) Identity(path string) (FileID, error) { return fileIdentity(path) }

// faultFS returns a configured error for op or op:path before delegating.
type faultFS struct {
	osFS
	fail map[string]error
}

func (f faultFS) err(op, path string) error {
	if f.fail == nil {
		return nil
	}
	if err, ok := f.fail[op+":"+path]; ok {
		return err
	}
	if err, ok := f.fail[op]; ok {
		return err
	}
	return nil
}

func (f faultFS) WriteFileAtomic(name string, data []byte, perm fs.FileMode) error {
	if err := f.err("WriteFileAtomic", name); err != nil {
		return err
	}
	return f.osFS.WriteFileAtomic(name, data, perm)
}

func (f faultFS) WriteFileExclusive(name string, data []byte, perm fs.FileMode) error {
	if err := f.err("WriteFileExclusive", name); err != nil {
		return err
	}
	return f.osFS.WriteFileExclusive(name, data, perm)
}

func (f faultFS) CopyFile(src, dst string) error {
	if err := f.err("CopyFile", src); err != nil {
		return err
	}
	return f.osFS.CopyFile(src, dst)
}

func (f faultFS) Symlink(oldname, newname string) error {
	if err := f.err("Symlink", newname); err != nil {
		return err
	}
	return f.osFS.Symlink(oldname, newname)
}

func (f faultFS) Rename(oldpath, newpath string) error {
	if err := f.err("Rename", newpath); err != nil {
		return err
	}
	return f.osFS.Rename(oldpath, newpath)
}

func (f faultFS) RenameExclusive(oldpath, newpath string) error {
	if err := f.err("RenameExclusive", newpath); err != nil {
		return err
	}
	return f.osFS.RenameExclusive(oldpath, newpath)
}

func (f faultFS) RemoveTree(path string, id FileID) error {
	if err := f.err("RemoveTree", path); err != nil {
		return err
	}
	return f.osFS.RemoveTree(path, id)
}

func writeAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	syncDir(dir)
	return nil
}

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

func copyFileStream(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrInvalidPath, src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".copy-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	perm := info.Mode().Perm() & 0o755
	if perm&0o111 == 0 {
		perm = 0o644
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func copyTree(fsys fileSystem, root, src, dst string) error {
	return copyTreeMode(fsys, root, src, dst, false)
}

// copyTreeMode copies src to dst. Symlinks are recreated, never followed.
// When keepExternal is false, absolute links and links that leave root are rejected
// so a deployment cannot point at a path outside the skill. Backups set
// keepExternal so the recorded tree matches what was deployed, including a
// symlink a user added later, without reading or writing the link target.
func copyTreeMode(fsys fileSystem, root, src, dst string, keepExternal bool) error {
	info, err := fsys.Lstat(src)
	if err != nil {
		return err
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		target, err := fsys.Readlink(src)
		if err != nil {
			return err
		}
		if !keepExternal {
			if filepath.IsAbs(target) {
				return fmt.Errorf("%w: refusing to copy absolute symlink %s", ErrInvalidPath, src)
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(src), target))
			if err := ensureWithin(root, resolved); err != nil {
				return fmt.Errorf("%w: symlink %s escapes the skill", ErrInvalidPath, src)
			}
		}
		return fsys.Symlink(target, dst)
	}
	if mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0 {
		return fmt.Errorf("%w: unsupported file type %s", ErrInvalidPath, src)
	}
	if info.IsDir() {
		if err := fsys.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := fsys.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTreeMode(fsys, root, filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name()), keepExternal); err != nil {
				return err
			}
		}
		return nil
	}
	return fsys.CopyFile(src, dst)
}

// nonNegative converts a file size after rejecting a negative value.
func nonNegative(n int64) (uint64, error) {
	if n < 0 {
		return 0, fmt.Errorf("%w: negative size", ErrInvalidPath)
	}
	return uint64(n), nil // #nosec G115 -- n was checked non-negative
}
