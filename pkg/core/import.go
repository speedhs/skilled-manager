package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportCandidate is an unmanaged skill found in a target directory.
type ImportCandidate struct {
	TargetID    string `json:"target_id"`
	TargetLabel string `json:"target_label"`
	SkillName   string `json:"skill_name"`
	Description string `json:"description"`
	SourcePath  string `json:"source_path"`
	IsToml      bool   `json:"is_toml"`
}

// ScanImportCandidates lists real skills in target directories that are not
// in the canonical store. Symlinks are ignored.
func (m *Manager) ScanImportCandidates(ctx context.Context) ([]ImportCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.scanLocked(ctx)
}

func (m *Manager) scanLocked(ctx context.Context) ([]ImportCandidate, error) {
	var out []ImportCandidate
	for _, target := range m.targets {
		dir, err := ExpandPath(target.Path)
		if err != nil {
			continue
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			path := filepath.Join(dir, entry.Name())
			inf, err := os.Lstat(path)
			if err != nil || inf.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if target.Format == FormatGeminiTOML {
				c, ok := m.tomlCandidate(target, dir, entry.Name(), path, inf)
				if ok {
					out = append(out, c)
				}
				continue
			}
			if !inf.IsDir() {
				continue
			}
			c, ok := m.dirCandidate(target, entry.Name(), path)
			if ok {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func (m *Manager) tomlCandidate(target Target, dir, name, path string, info os.FileInfo) (ImportCandidate, bool) {
	if info.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".toml") {
		return ImportCandidate{}, false
	}
	skillName := strings.TrimSuffix(name, filepath.Ext(name))
	if err := ValidateSkillName(skillName); err != nil {
		return ImportCandidate{}, false
	}
	if m.state.IsTracked(target.ID, skillName) {
		return ImportCandidate{}, false
	}
	if _, err := os.Lstat(filepath.Join(m.CanonicalDir, skillName)); err == nil {
		return ImportCandidate{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ImportCandidate{}, false
	}
	desc := ""
	if skill, err := GeminiTOMLToSkill(skillName, string(body)); err == nil {
		desc = skill.Description
	}
	return ImportCandidate{
		TargetID: target.ID, TargetLabel: target.Label, SkillName: skillName,
		Description: desc, SourcePath: path, IsToml: true,
	}, true
}

func (m *Manager) dirCandidate(target Target, name, path string) (ImportCandidate, bool) {
	if err := ValidateSkillName(name); err != nil {
		return ImportCandidate{}, false
	}
	if m.state.IsTracked(target.ID, name) {
		return ImportCandidate{}, false
	}
	if _, err := os.Lstat(filepath.Join(m.CanonicalDir, name)); err == nil {
		return ImportCandidate{}, false
	}
	desc := ""
	skill, err := ParseSkill(path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ImportCandidate{}, false
	}
	if err == nil {
		if skill.Name != name {
			return ImportCandidate{}, false
		}
		desc = skill.Description
	}
	return ImportCandidate{
		TargetID: target.ID, TargetLabel: target.Label, SkillName: name,
		Description: desc, SourcePath: path, IsToml: false,
	}, true
}

// ImportCandidate moves one candidate into the canonical store and links it
// back. On failure the source is restored and the canonical copy is removed.
func (m *Manager) ImportCandidate(ctx context.Context, candidate ImportCandidate) error {
	return m.mutate(ctx, func() error {
		return m.importLocked(ctx, candidate)
	})
}

// ImportSelected imports each candidate. One failure does not stop the rest.
func (m *Manager) ImportSelected(ctx context.Context, candidates []ImportCandidate) ([]string, []error) {
	var imported []string
	var errs []error
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := m.ImportCandidate(ctx, c); err != nil {
			errs = append(errs, fmt.Errorf("import %q from %s: %w", c.SkillName, c.TargetLabel, err))
			continue
		}
		imported = append(imported, c.SkillName)
	}
	return imported, errs
}

func (m *Manager) importLocked(ctx context.Context, c ImportCandidate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateSkillName(c.SkillName); err != nil {
		return err
	}
	target, ok := m.targetByID(c.TargetID)
	if !ok {
		return fmt.Errorf("%w: target %q", ErrNotFound, c.TargetID)
	}
	if err := m.rejectUnsafeTarget(target); err != nil {
		return err
	}
	root, err := ExpandPath(target.Path)
	if err != nil {
		return err
	}
	src, err := ExpandPath(c.SourcePath)
	if err != nil {
		return err
	}
	if err := ensureWithin(root, src); err != nil {
		return err
	}
	if err := hermeticGuard(src); err != nil {
		return err
	}
	dest, err := skillDir(m.CanonicalDir, c.SkillName)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("%w: %s", ErrExists, c.SkillName)
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: refusing to import symlink %s", ErrConflict, src)
	}
	if m.dryRun {
		return nil
	}
	if c.IsToml {
		return m.importToml(ctx, c, src, dest)
	}
	return m.importDir(ctx, c, src, dest)
}

func (m *Manager) importToml(ctx context.Context, c ImportCandidate, src, dest string) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	skill, err := GeminiTOMLToSkill(c.SkillName, string(body))
	if err != nil {
		return err
	}
	generated, err := SkillToGeminiTOML(*skill)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(dest, "SKILL.md"), []byte(skill.Content), 0o644); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	// The generated command drops comments and unknown keys. Keep the original
	// bytes in the backup store before the source file is replaced.
	if generated != string(body) {
		id, err := m.backupLocked(c.TargetID, c.SkillName, src)
		if err != nil {
			_ = os.RemoveAll(dest)
			return fmt.Errorf("refusing to replace %s without a backup of the original TOML: %w", src, err)
		}
		m.notes = append(m.notes, fmt.Sprintf("imported %s from TOML; original file kept as backup %s because comments and unknown keys are not stored in SKILL.md", c.SkillName, id))
	}
	bak := src + ".import-bak-" + randHex(3)
	if err := os.Rename(src, bak); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	if _, err := m.enableLocked(ctx, c.SkillName, c.TargetID); err != nil {
		_ = os.RemoveAll(dest)
		_ = os.Rename(bak, src)
		return err
	}
	id, idErr := fileIdentity(bak)
	if idErr == nil {
		_ = removeTreeNoFollow(bak, id)
	}
	return nil
}

func (m *Manager) importDir(ctx context.Context, c ImportCandidate, src, dest string) error {
	staging, err := m.stageCopy(src)
	if err != nil {
		return err
	}
	if err := m.publishNew(staging, dest); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(dest, "SKILL.md")); errors.Is(err, os.ErrNotExist) {
		fm := Frontmatter{Name: c.SkillName, Description: fallbackDesc(c)}
		text, serr := SerializeSkillMarkdown(fm, "# "+c.SkillName+"\n")
		if serr != nil {
			_ = removeImported(dest)
			return serr
		}
		if werr := writeAtomic(filepath.Join(dest, "SKILL.md"), []byte(text), 0o644); werr != nil {
			_ = removeImported(dest)
			return werr
		}
	} else if _, err := ParseSkill(dest); err != nil {
		_ = removeImported(dest)
		return err
	}
	bak := src + ".import-bak-" + randHex(3)
	if err := os.Rename(src, bak); err != nil {
		_ = removeImported(dest)
		return err
	}
	if _, err := m.enableLocked(ctx, c.SkillName, c.TargetID); err != nil {
		_ = os.Rename(bak, src)
		_ = removeImported(dest)
		return err
	}
	id, err := fileIdentity(bak)
	if err != nil {
		return nil
	}
	_ = removeTreeNoFollow(bak, id)
	return nil
}

func fallbackDesc(c ImportCandidate) string {
	if strings.TrimSpace(c.Description) != "" {
		return c.Description
	}
	return "Imported skill " + c.SkillName
}

func removeImported(dest string) error {
	id, err := fileIdentity(dest)
	if err != nil {
		return err
	}
	return removeTreeNoFollow(dest, id)
}
