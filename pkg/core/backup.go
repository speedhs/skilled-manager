package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

const (
	maxBackups     = 20
	maxBackupBytes = 256 << 20
)

var backupIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,200}$`)

// Backup is one retained copy of a managed deployment.
type Backup struct {
	ID     string `json:"id"`
	Skill  string `json:"skill"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
}

type backupMeta struct {
	ID     string `json:"id"`
	Skill  string `json:"skill"`
	Target string `json:"target"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
}

func (m *Manager) backupLocked(targetID, skill, src string) (string, error) {
	if m.dryRun {
		return "", nil
	}
	info, err := m.files().Lstat(src)
	if err != nil {
		return "", err
	}
	if err := ValidateSkillName(skill); err != nil {
		return "", err
	}
	id := time.Now().UTC().Format("20060102T150405.000000000Z") + "__" + targetID + "__" + skill + "__" + randHex(2)
	if err := validateBackupID(id); err != nil {
		return "", err
	}
	root := filepath.Join(m.ConfigDir, "backups", id)
	payload := filepath.Join(root, "payload")
	if err := copyTreeMode(m.files(), src, src, payload, true); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	kind := "file"
	if info.IsDir() {
		kind = "dir"
	}
	meta := backupMeta{ID: id, Skill: skill, Target: targetID, Path: src, Kind: kind}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	if err := writeAtomic(filepath.Join(root, "meta.json"), append(raw, '\n'), 0o644); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	m.pruneBackups()
	m.log().Info("backed up deployment", "id", id, "skill", skill, "target", targetID)
	return id, nil
}

func validateBackupID(id string) error {
	if !backupIDPattern.MatchString(id) || stringsContainsDotDot(id) {
		return fmt.Errorf("%w: backup id %q", ErrInvalidName, id)
	}
	return nil
}

func stringsContainsDotDot(s string) bool {
	return filepath.Clean(s) != s || len(s) >= 2 && s[0] == '.' && s[1] == '.'
}

// ListBackups returns retained backups, newest first.
func (m *Manager) ListBackups() ([]Backup, error) {
	root := filepath.Join(m.ConfigDir, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Backup{}, nil
		}
		return nil, err
	}
	var out []Backup
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateBackupID(entry.Name()); err != nil {
			continue
		}
		meta, err := readBackupMeta(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, Backup(meta))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func readBackupMeta(dir string) (backupMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return backupMeta{}, err
	}
	var meta backupMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return backupMeta{}, err
	}
	if err := validateBackupID(meta.ID); err != nil {
		return backupMeta{}, err
	}
	return meta, nil
}

// Restore puts a backup back at the destination it was taken from.
// The destination must still be the configured path for that skill and
// target. An unmanaged file at that path is left untouched.
func (m *Manager) Restore(ctx context.Context, id string) error {
	if err := validateBackupID(id); err != nil {
		return err
	}
	return m.mutate(ctx, func() error {
		return m.restoreLocked(id)
	})
}

func (m *Manager) restoreLocked(id string) error {
	meta, err := readBackupMeta(filepath.Join(m.ConfigDir, "backups", id))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: backup %s", ErrNotFound, id)
		}
		return err
	}
	target, ok := m.targetByID(meta.Target)
	if !ok {
		return fmt.Errorf("%w: target %q", ErrNotFound, meta.Target)
	}
	dest, err := GetTargetDestinationPath(target, meta.Skill)
	if err != nil {
		return err
	}
	if !samePath(dest, meta.Path) {
		return fmt.Errorf("%w: backup path %s is not the current destination %s", ErrConflict, meta.Path, dest)
	}
	payload := filepath.Join(m.ConfigDir, "backups", id, "payload")
	info, err := m.files().Lstat(dest)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s is a symlink; disable it before restoring a copy", ErrConflict, dest)
		}
		rec, tracked := m.state.GetRecord(meta.Target, meta.Skill)
		if !tracked {
			return fmt.Errorf("%w: collision at %s", ErrConflict, dest)
		}
		if _, _, err := authorizeDelete(m.files(), dest, rec); err != nil {
			return err
		}
		if m.dryRun {
			return nil
		}
		if err := m.replaceTree(target, meta.Skill, payload, dest, rec); err != nil {
			return err
		}
	} else if m.dryRun {
		return nil
	} else {
		if err := m.files().MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		staging, err := m.stageCopy(payload)
		if err != nil {
			return err
		}
		if err := m.publishNew(staging, dest); err != nil {
			return err
		}
	}
	return m.recordDeployment(target, meta.Skill, dest, ModeCopy, false)
}

func (m *Manager) pruneBackups() {
	root := filepath.Join(m.ConfigDir, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	type item struct {
		name string
		size int64
	}
	var items []item
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		size := dirSize(filepath.Join(root, entry.Name()))
		items = append(items, item{name: entry.Name(), size: size})
		total += size
	}
	for len(items) > maxBackups || (len(items) > 0 && total > maxBackupBytes) {
		old := items[0]
		_ = os.RemoveAll(filepath.Join(root, old.name))
		total -= old.size
		items = items[1:]
	}
}

func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			n += info.Size()
		}
		return nil
	})
	return n
}
