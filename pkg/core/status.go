package core

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// StatusCode is the deployment state of one skill on one target.
type StatusCode string

const (
	// StatusEnabled means the deployment matches the canonical skill.
	StatusEnabled StatusCode = "enabled"
	// StatusDisabled means nothing is deployed at the destination.
	StatusDisabled StatusCode = "disabled"
	// StatusConflict means the destination is not ours, or both sides changed.
	StatusConflict StatusCode = "conflict"
	// StatusDrifted means the canonical skill changed and the deployment did not.
	StatusDrifted StatusCode = "drifted"
	// StatusBroken means a symlink is unreadable or its canonical target is gone.
	StatusBroken StatusCode = "broken"
)

// CellStatus is the status of one skill on one target.
type CellStatus struct {
	Code         StatusCode `json:"code"`
	Message      string     `json:"message"`
	ActualPath   string     `json:"actual_path"`
	IsSymlink    bool       `json:"is_symlink,omitempty"`
	LinkTarget   string     `json:"link_target,omitempty"`
	ModeFallback bool       `json:"mode_fallback,omitempty"`
}

// Status reports the deployment state of skillName on targetID.
func (m *Manager) Status(ctx context.Context, skillName, targetID string) (CellStatus, error) {
	if err := ctx.Err(); err != nil {
		return CellStatus{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.statusLocked(skillName, targetID)
}

func (m *Manager) statusLocked(skillName, targetID string) (CellStatus, error) {
	target, ok := m.targetByID(targetID)
	if !ok {
		return CellStatus{}, fmt.Errorf("%w: target %q", ErrNotFound, targetID)
	}
	if err := ValidateSkillName(skillName); err != nil {
		return CellStatus{}, err
	}
	dest, err := GetTargetDestinationPath(target, skillName)
	if err != nil {
		return CellStatus{}, err
	}
	canonical, err := skillDir(m.CanonicalDir, skillName)
	if err != nil {
		return CellStatus{}, err
	}
	st := CellStatus{ActualPath: dest}
	info, err := m.files().Lstat(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			st.Code = StatusDisabled
			st.Message = "Not installed in target"
			return st, nil
		}
		return CellStatus{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return m.symlinkStatus(st, dest, canonical)
	}
	rec, tracked := m.state.GetRecord(target.ID, skillName)
	if !tracked {
		st.Code = StatusConflict
		st.Message = fmt.Sprintf("collision: %s exists and is not managed by Skills Manager", dest)
		return st, nil
	}
	st.ModeFallback = rec.Fallback
	if target.Format == FormatGeminiTOML {
		return m.tomlStatus(st, canonical, dest, rec)
	}
	return m.copyStatus(st, canonical, dest, rec)
}

func (m *Manager) symlinkStatus(st CellStatus, dest, canonical string) (CellStatus, error) {
	st.IsSymlink = true
	raw, err := m.files().Readlink(dest)
	if err != nil {
		st.Code = StatusBroken
		st.Message = fmt.Sprintf("symlink at %s cannot be read: %v", dest, err)
		return st, nil
	}
	st.LinkTarget = resolveLink(dest, raw)
	if !linksTo(dest, raw, canonical) {
		st.Code = StatusConflict
		st.Message = fmt.Sprintf("collision: symlink %s points to %s, not %s", dest, st.LinkTarget, canonical)
		return st, nil
	}
	if _, err := os.Lstat(canonical); err != nil {
		st.Code = StatusBroken
		st.Message = fmt.Sprintf("symlink points at missing canonical skill %s", canonical)
		return st, nil
	}
	st.Code = StatusEnabled
	st.Message = "symlinked to the canonical skill"
	return st, nil
}

func (m *Manager) tomlStatus(st CellStatus, canonical, dest string, rec TargetRecord) (CellStatus, error) {
	skill, err := ParseSkill(canonical)
	if err != nil {
		st.Code = StatusBroken
		st.Message = err.Error()
		return st, nil
	}
	body, err := m.files().ReadFile(dest)
	if err != nil {
		st.Code = StatusBroken
		st.Message = err.Error()
		return st, nil
	}
	if IsGeminiTOMLMatch(*skill, string(body)) {
		st.Code = StatusEnabled
		st.Message = "Gemini TOML command matches the canonical skill"
		return st, nil
	}
	fileHash := HashString(string(body))
	canHash, err := HashDirectory(canonical)
	if err != nil {
		st.Code = StatusBroken
		st.Message = err.Error()
		return st, nil
	}
	return classifyDrift(st, rec, canHash, fileHash, "Gemini TOML"), nil
}

func (m *Manager) copyStatus(st CellStatus, canonical, dest string, rec TargetRecord) (CellStatus, error) {
	canHash, err := HashDirectory(canonical)
	if err != nil {
		st.Code = StatusBroken
		st.Message = err.Error()
		return st, nil
	}
	dstHash, err := HashDirectory(dest)
	if err != nil {
		st.Code = StatusBroken
		st.Message = err.Error()
		return st, nil
	}
	if canHash == dstHash {
		st.Code = StatusEnabled
		if rec.Fallback {
			st.Message = "copy fallback matches the canonical skill"
		} else {
			st.Message = "copy matches the canonical skill"
		}
		return st, nil
	}
	return classifyDrift(st, rec, canHash, dstHash, "copy"), nil
}

// classifyDrift decides who changed since the record was written.
// A legacy record with an empty baseline is a conflict when the two sides
// differ, because there is no safe way to pick one.
func classifyDrift(st CellStatus, rec TargetRecord, canHash, dstHash, kind string) CellStatus {
	if rec.CanonicalHash == "" || rec.Hash == "" {
		st.Code = StatusConflict
		st.Message = fmt.Sprintf("collision: %s changed and the state record has no baseline hash; not overwriting", kind)
		return st
	}
	canChanged := canHash != rec.CanonicalHash
	dstChanged := dstHash != rec.Hash
	switch {
	case canChanged && dstChanged:
		st.Code = StatusConflict
		st.Message = fmt.Sprintf("conflict: both the canonical skill and the %s changed", kind)
	case dstChanged:
		st.Code = StatusConflict
		st.Message = fmt.Sprintf("conflict: the %s was edited and the canonical skill was not", kind)
	case canChanged:
		st.Code = StatusDrifted
		st.Message = fmt.Sprintf("%s is behind the canonical skill", kind)
	default:
		// The two hashes differ, but neither differs from the stored baseline.
		// That ledger cannot be trusted, so this is a conflict.
		st.Code = StatusConflict
		st.Message = fmt.Sprintf("conflict: %s does not match the canonical skill and the baseline hashes disagree", kind)
	}
	return st
}
