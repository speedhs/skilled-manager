package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// EnableResult describes one successful enable.
type EnableResult struct {
	SkillName    string `json:"skill_name"`
	TargetID     string `json:"target_id"`
	TargetPath   string `json:"target_path"`
	Mode         string `json:"mode"`
	Fallback     bool   `json:"fallback,omitempty"`
	Message      string `json:"message"`
	RecordedHash string `json:"recorded_hash,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
}

// Enable deploys skillName to targetID.
// A conflict is returned as ErrConflict and the destination is not modified.
func (m *Manager) Enable(ctx context.Context, skillName, targetID string) (*EnableResult, error) {
	var res *EnableResult
	err := m.mutate(ctx, func() error {
		var err error
		res, err = m.enableLocked(ctx, skillName, targetID)
		return err
	})
	return res, err
}

func (m *Manager) enableLocked(ctx context.Context, skillName, targetID string) (*EnableResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, ok := m.targetByID(targetID)
	if !ok {
		return nil, fmt.Errorf("%w: target %q", ErrNotFound, targetID)
	}
	canonical, err := skillDir(m.CanonicalDir, skillName)
	if err != nil {
		return nil, err
	}
	if _, err := ParseSkill(canonical); err != nil {
		return nil, err
	}
	dest, err := GetTargetDestinationPath(target, skillName)
	if err != nil {
		return nil, err
	}
	st, err := m.statusLocked(skillName, targetID)
	if err != nil {
		return nil, err
	}
	if st.Code == StatusConflict {
		return nil, fmt.Errorf("%w: collision at %s: %s", ErrConflict, dest, st.Message)
	}
	if st.Code == StatusEnabled {
		return &EnableResult{
			SkillName: skillName, TargetID: targetID, TargetPath: dest,
			Mode: target.Mode, Message: "already enabled",
		}, nil
	}
	if m.dryRun {
		return &EnableResult{
			SkillName: skillName, TargetID: targetID, TargetPath: dest,
			Mode: target.Mode, DryRun: true,
			Message: "dry-run: would enable (" + string(st.Code) + ")",
		}, nil
	}
	if err := m.files().MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	switch {
	case target.Format == FormatGeminiTOML:
		return m.enableTOML(target, skillName, canonical, dest, st)
	case target.Mode == ModeCopy:
		return m.enableCopy(target, skillName, canonical, dest, st)
	case target.Mode == ModeSymlink:
		return m.enableSymlink(target, skillName, canonical, dest, st)
	default:
		return nil, fmt.Errorf("%w: mode %q", ErrInvalidPath, target.Mode)
	}
}

func (m *Manager) enableSymlink(target Target, skillName, canonical, dest string, st CellStatus) (*EnableResult, error) {
	switch st.Code {
	case StatusBroken:
		raw, err := m.files().Readlink(dest)
		if err != nil || !linksTo(dest, raw, canonical) {
			return nil, fmt.Errorf("%w: refusing to replace %s", ErrConflict, dest)
		}
		if err := m.files().Remove(dest); err != nil {
			return nil, err
		}
	case StatusDrifted:
		rec, ok := m.state.GetRecord(target.ID, skillName)
		if !ok {
			return nil, fmt.Errorf("%w: missing state for %s", ErrConflict, dest)
		}
		if _, err := m.backupLocked(target.ID, skillName, dest); err != nil {
			return nil, err
		}
		id, _, err := authorizeDelete(m.files(), dest, rec)
		if err != nil {
			return nil, err
		}
		if err := m.files().RemoveTree(dest, id); err != nil {
			return nil, err
		}
	case StatusDisabled:
		// Destination is absent. Symlink fails if that stops being true.
	default:
		return nil, fmt.Errorf("%w: %s", ErrConflict, st.Message)
	}
	link, err := m.linkCanonical(canonical, dest)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%w: collision at %s", ErrConflict, dest)
		}
		return nil, err
	}
	res := &EnableResult{
		SkillName: skillName, TargetID: target.ID, TargetPath: dest,
		Mode: string(link.Type), Fallback: link.Fallback, Message: link.Message,
	}
	if err := m.recordDeployment(target, skillName, dest, string(link.Type), link.Fallback); err != nil {
		if link.Type == LinkTypeSymlink || link.Type == LinkTypeJunction {
			_ = m.files().Remove(dest)
		} else if id, idErr := m.files().Identity(dest); idErr == nil {
			_ = m.files().RemoveTree(dest, id)
		}
		return nil, err
	}
	if rec, ok := m.state.GetRecord(target.ID, skillName); ok {
		res.RecordedHash = rec.Hash
	}
	return res, nil
}

func (m *Manager) linkCanonical(canonical, dest string) (LinkResult, error) {
	if err := m.files().Symlink(canonical, dest); err != nil {
		return linkFallback(m.files(), canonical, dest, err)
	}
	return LinkResult{Type: LinkTypeSymlink, Message: "symlink created"}, nil
}

func (m *Manager) enableCopy(target Target, skillName, canonical, dest string, st CellStatus) (*EnableResult, error) {
	switch st.Code {
	case StatusDisabled:
		staging, err := m.stageCopy(canonical)
		if err != nil {
			return nil, err
		}
		if err := m.publishNew(staging, dest); err != nil {
			return nil, err
		}
	case StatusDrifted:
		rec, ok := m.state.GetRecord(target.ID, skillName)
		if !ok {
			return nil, fmt.Errorf("%w: missing state for %s", ErrConflict, dest)
		}
		if err := m.replaceTree(target, skillName, canonical, dest, rec); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrConflict, st.Message)
	}
	if err := m.recordDeployment(target, skillName, dest, ModeCopy, false); err != nil {
		if st.Code == StatusDisabled {
			if id, idErr := m.files().Identity(dest); idErr == nil {
				_ = m.files().RemoveTree(dest, id)
			}
		}
		return nil, err
	}
	res := &EnableResult{
		SkillName: skillName, TargetID: target.ID, TargetPath: dest,
		Mode: ModeCopy, Message: "copied skill directory",
	}
	if rec, ok := m.state.GetRecord(target.ID, skillName); ok {
		res.RecordedHash = rec.Hash
	}
	return res, nil
}

func (m *Manager) enableTOML(target Target, skillName, canonical, dest string, st CellStatus) (*EnableResult, error) {
	skill, err := ParseSkill(canonical)
	if err != nil {
		return nil, err
	}
	text, err := SkillToGeminiTOML(*skill)
	if err != nil {
		return nil, err
	}
	data := []byte(text)
	switch st.Code {
	case StatusDisabled:
		if err := m.files().WriteFileExclusive(dest, data, 0o644); err != nil {
			if os.IsExist(err) {
				return nil, fmt.Errorf("%w: collision at %s", ErrConflict, dest)
			}
			return nil, err
		}
	case StatusDrifted:
		rec, ok := m.state.GetRecord(target.ID, skillName)
		if !ok {
			return nil, fmt.Errorf("%w: missing state for %s", ErrConflict, dest)
		}
		if _, _, err := authorizeDelete(m.files(), dest, rec); err != nil {
			return nil, err
		}
		if _, err := m.backupLocked(target.ID, skillName, dest); err != nil {
			return nil, err
		}
		if err := m.files().WriteFileAtomic(dest, data, 0o644); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrConflict, st.Message)
	}
	if err := m.recordDeployment(target, skillName, dest, ModeCopy, false); err != nil {
		if st.Code == StatusDisabled {
			if id, idErr := m.files().Identity(dest); idErr == nil {
				_ = m.files().RemoveTree(dest, id)
			}
		}
		return nil, err
	}
	res := &EnableResult{
		SkillName: skillName, TargetID: target.ID, TargetPath: dest,
		Mode: ModeCopy, Message: "generated Gemini TOML command", RecordedHash: HashString(text),
	}
	return res, nil
}

func (m *Manager) recordDeployment(target Target, skillName, dest, mode string, fallback bool) error {
	id, err := m.files().Identity(dest)
	if err != nil {
		return err
	}
	var hash, canHash string
	if target.Format == FormatGeminiTOML {
		body, err := m.files().ReadFile(dest)
		if err != nil {
			return err
		}
		hash = HashString(string(body))
	} else if mode == string(LinkTypeSymlink) || mode == string(LinkTypeJunction) {
		hash = ""
	} else {
		hash, err = HashDirectory(dest)
		if err != nil {
			return err
		}
	}
	canHash, err = HashDirectory(filepath.Join(m.CanonicalDir, skillName))
	if err != nil {
		return err
	}
	// Copies and TOML store content hashes. Symlinks leave the deployment hash empty.
	m.state.SetRecord(target.ID, skillName, TargetRecord{
		Mode:          mode,
		Format:        target.Format,
		Hash:          hash,
		CanonicalHash: canHash,
		Path:          dest,
		Fallback:      fallback,
		Device:        id.Dev,
		Inode:         id.Ino,
		IdentityValid: id.Valid,
	})
	if err := m.state.SaveTo(m.files(), m.StatePath); err != nil {
		m.state.DeleteRecord(target.ID, skillName)
		return err
	}
	return nil
}

func (m *Manager) stageCopy(src string) (string, error) {
	root := filepath.Join(m.ConfigDir, "staging")
	if err := m.files().MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	staging := filepath.Join(root, filepath.Base(src)+"-"+randHex(4))
	if err := copyTree(m.files(), src, src, staging); err != nil {
		_ = m.files().RemoveTree(staging, FileID{})
		return "", err
	}
	return staging, nil
}

func (m *Manager) publishNew(staging, dest string) error {
	if err := hermeticGuard(dest); err != nil {
		_ = m.files().RemoveTree(staging, FileID{})
		return err
	}
	err := m.files().RenameExclusive(staging, dest)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		_ = m.files().RemoveTree(staging, FileID{})
		return err
	}
	if err := copyTree(m.files(), staging, staging, dest); err != nil {
		if id, idErr := m.files().Identity(dest); idErr == nil {
			_ = m.files().RemoveTree(dest, id)
		}
		_ = m.files().RemoveTree(staging, FileID{})
		return err
	}
	_ = m.files().RemoveTree(staging, FileID{})
	return nil
}

func (m *Manager) replaceTree(target Target, skillName, src, dest string, rec TargetRecord) error {
	id, _, err := authorizeDelete(m.files(), dest, rec)
	if err != nil {
		return err
	}
	if _, err := m.backupLocked(target.ID, skillName, dest); err != nil {
		return err
	}
	staging, err := m.stageCopy(src)
	if err != nil {
		return err
	}
	hold := filepath.Join(filepath.Dir(dest), ".hold-"+randHex(4))
	if err := m.files().Rename(dest, hold); err != nil {
		_ = m.files().RemoveTree(staging, FileID{})
		return err
	}
	if err := m.publishNew(staging, dest); err != nil {
		_ = m.files().Rename(hold, dest)
		return err
	}
	if err := m.files().RemoveTree(hold, id); err != nil {
		newID, _ := m.files().Identity(dest)
		_ = m.files().RemoveTree(dest, newID)
		_ = m.files().Rename(hold, dest)
		return fmt.Errorf("%w: destination changed during replace", ErrConflict)
	}
	return nil
}

func authorizeDelete(fsys fileSystem, dest string, rec TargetRecord) (FileID, string, error) {
	info, err := fsys.Lstat(dest)
	if err != nil {
		return FileID{}, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return FileID{}, "", fmt.Errorf("%w: %s is a symlink", ErrConflict, dest)
	}
	id, err := fsys.Identity(dest)
	if err != nil {
		return FileID{}, "", err
	}
	hash, err := HashDirectory(dest)
	if err != nil {
		return FileID{}, "", err
	}
	if rec.IdentityValid {
		if !id.same(recordID(rec)) {
			return FileID{}, "", fmt.Errorf("%w: file identity changed at %s", ErrConflict, dest)
		}
		return id, hash, nil
	}
	if rec.Hash != "" && rec.Hash == hash {
		return id, hash, nil
	}
	return FileID{}, "", fmt.Errorf("%w: refusing to delete %s without a matching identity", ErrConflict, dest)
}

// Disable removes a deployment this tool created.
// Symlinks are removed only when they point at the canonical skill.
// Copies and TOML files are removed only when state identity matches.
func (m *Manager) Disable(ctx context.Context, skillName, targetID string) error {
	return m.mutate(ctx, func() error {
		return m.disableLocked(skillName, targetID)
	})
}

func (m *Manager) disableLocked(skillName, targetID string) error {
	target, ok := m.targetByID(targetID)
	if !ok {
		return fmt.Errorf("%w: target %q", ErrNotFound, targetID)
	}
	canonical, err := skillDir(m.CanonicalDir, skillName)
	if err != nil {
		return err
	}
	dest, err := GetTargetDestinationPath(target, skillName)
	if err != nil {
		return err
	}
	info, err := m.files().Lstat(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if m.dryRun {
				return nil
			}
			m.state.DeleteRecord(targetID, skillName)
			return m.state.SaveTo(m.files(), m.StatePath)
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		raw, rerr := m.files().Readlink(dest)
		if rerr != nil {
			return fmt.Errorf("%w: cannot read symlink %s: %v", ErrConflict, dest, rerr)
		}
		if !linksTo(dest, raw, canonical) {
			return fmt.Errorf("%w: collision at %s: symlink points to %s", ErrConflict, dest, resolveLink(dest, raw))
		}
		if m.dryRun {
			return nil
		}
		if err := m.files().Remove(dest); err != nil {
			return err
		}
		m.state.DeleteRecord(targetID, skillName)
		return m.state.SaveTo(m.files(), m.StatePath)
	}
	rec, tracked := m.state.GetRecord(targetID, skillName)
	if !tracked {
		return fmt.Errorf("%w: collision at %s: not managed by Skills Manager", ErrConflict, dest)
	}
	if _, _, err := authorizeDelete(m.files(), dest, rec); err != nil {
		return err
	}
	if m.dryRun {
		return nil
	}
	if _, err := m.backupLocked(targetID, skillName, dest); err != nil {
		return err
	}
	id := recordID(rec)
	if !id.Valid {
		id, _ = m.files().Identity(dest)
	}
	if err := m.files().RemoveTree(dest, id); err != nil {
		return err
	}
	m.state.DeleteRecord(targetID, skillName)
	return m.state.SaveTo(m.files(), m.StatePath)
}

// SyncReport is the result of Sync.
type SyncReport struct {
	SchemaVersion int      `json:"schema_version"`
	DryRun        bool     `json:"dry_run"`
	Updated       []string `json:"updated"`
	Conflicts     []string `json:"conflicts"`
	Skipped       []string `json:"skipped"`
	Errors        []string `json:"errors"`
}

// Sync updates deployments where only the canonical skill changed.
// Edits on the target, and edits on both sides, are reported as conflicts
// and left untouched.
func (m *Manager) Sync(ctx context.Context) (*SyncReport, error) {
	var report *SyncReport
	err := m.mutate(ctx, func() error {
		var err error
		report, err = m.syncLocked(ctx)
		return err
	})
	return report, err
}

func (m *Manager) syncLocked(ctx context.Context) (*SyncReport, error) {
	report := &SyncReport{
		SchemaVersion: JSONSchemaVersion,
		DryRun:        m.dryRun,
		Updated:       []string{},
		Conflicts:     []string{},
		Skipped:       []string{},
		Errors:        []string{},
	}
	skills, err := ListSkills(m.CanonicalDir)
	if err != nil {
		return nil, err
	}
	for _, skill := range skills {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, target := range m.targets {
			st, err := m.statusLocked(skill.Name, target.ID)
			if err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("%s / %s: %v", skill.Name, target.ID, err))
				continue
			}
			switch st.Code {
			case StatusDrifted, StatusBroken:
				res, err := m.enableLocked(ctx, skill.Name, target.ID)
				if err != nil {
					report.Errors = append(report.Errors, fmt.Sprintf("%s / %s: %v", skill.Name, target.ID, err))
					continue
				}
				report.Updated = append(report.Updated, fmt.Sprintf("%s -> %s (%s)", skill.Name, target.Label, res.Message))
			case StatusConflict:
				line := fmt.Sprintf("%s / %s: %s", skill.Name, target.Label, st.Message)
				report.Conflicts = append(report.Conflicts, line)
				report.Skipped = append(report.Skipped, line)
			}
		}
	}
	return report, nil
}

// JSONSchemaVersion is the version of CLI JSON documents.
const JSONSchemaVersion = 1
