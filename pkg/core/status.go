package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// StatusCode represents the state of a skill for a given target.
type StatusCode string

const (
	StatusEnabled  StatusCode = "enabled"
	StatusDisabled StatusCode = "disabled"
	StatusConflict StatusCode = "conflict"
	StatusDrifted  StatusCode = "drifted"
	StatusBroken   StatusCode = "broken"
)

// CellStatus provides detailed status information for a (skill, target) pair.
type CellStatus struct {
	Code         StatusCode `json:"code"`
	Message      string     `json:"message"`
	ActualPath   string     `json:"actual_path"`
	IsSymlink    bool       `json:"is_symlink,omitempty"`
	LinkTarget   string     `json:"link_target,omitempty"`
	ModeFallback bool       `json:"mode_fallback,omitempty"`
}

// GetTargetDestinationPath computes the target path for a given skill and target config.
func GetTargetDestinationPath(target Target, skillName string) string {
	targetDir := ExpandPath(target.Path)
	if target.Format == FormatGeminiTOML {
		return filepath.Join(targetDir, skillName+".toml")
	}
	return filepath.Join(targetDir, skillName)
}

// Status evaluates the current status of a skill for a target.
func (m *Manager) Status(skillName string, targetID string) (CellStatus, error) {
	target, exists := m.GetTargetByID(targetID)
	if !exists {
		return CellStatus{}, fmt.Errorf("target %q not found", targetID)
	}

	canonicalSkillPath := filepath.Join(m.CanonicalDir, skillName)
	destPath := GetTargetDestinationPath(target, skillName)

	status := CellStatus{
		ActualPath: destPath,
	}

	// Check if destination path exists
	lstat, err := os.Lstat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			status.Code = StatusDisabled
			status.Message = "Not installed in target"
			return status, nil
		}
		return CellStatus{}, fmt.Errorf("failed to inspect target path %s: %w", destPath, err)
	}

	// 1. Check if it is a symbolic link
	if lstat.Mode()&os.ModeSymlink != 0 {
		status.IsSymlink = true
		linkTarget, err := os.Readlink(destPath)
		if err != nil {
			status.Code = StatusBroken
			status.Message = fmt.Sprintf("Broken symlink (cannot read link target): %v", err)
			return status, nil
		}

		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(destPath), linkTarget)
		}
		status.LinkTarget = linkTarget

		if PathsReferToSameLocation(linkTarget, canonicalSkillPath) {
			// Symlink points to our canonical skill folder
			if _, err := os.Stat(canonicalSkillPath); os.IsNotExist(err) {
				status.Code = StatusBroken
				status.Message = fmt.Sprintf("Symlink points to non-existent canonical skill: %s", canonicalSkillPath)
				return status, nil
			}
			status.Code = StatusEnabled
			status.Message = "Symlinked to canonical skill"
			return status, nil
		}

		// Symlink exists but points elsewhere -> Conflict
		status.Code = StatusConflict
		status.Message = fmt.Sprintf("Collision: Symlink points to %s instead of canonical %s", linkTarget, canonicalSkillPath)
		return status, nil
	}

	// 2. Target exists as a regular file or directory (not a symlink)
	state := m.GetState()
	record, isTracked := state.GetRecord(target.ID, skillName)

	// Case 2A: Gemini TOML format
	if target.Format == FormatGeminiTOML {
		if !isTracked {
			status.Code = StatusConflict
			status.Message = fmt.Sprintf("Collision: File %s already exists and is not managed by Skills Manager", destPath)
			return status, nil
		}

		// Parse canonical skill
		canonicalSkill, err := ParseSkill(canonicalSkillPath)
		if err != nil {
			status.Code = StatusBroken
			status.Message = fmt.Sprintf("Canonical skill error: %v", err)
			return status, nil
		}

		actualContent, err := os.ReadFile(destPath)
		if err != nil {
			status.Code = StatusBroken
			status.Message = fmt.Sprintf("Cannot read destination TOML file: %v", err)
			return status, nil
		}

		if IsGeminiTOMLMatch(*canonicalSkill, string(actualContent)) {
			status.Code = StatusEnabled
			status.Message = "Gemini TOML command is active and synchronized"
			return status, nil
		}

		status.Code = StatusDrifted
		status.Message = "Gemini TOML file differs from canonical SKILL.md (sync needed)"
		return status, nil
	}

	// Case 2B: Mode copy or symlink fallback to copy
	if isTracked {
		status.ModeFallback = record.Fallback

		canonicalHash, err := HashDirectory(canonicalSkillPath)
		if err != nil {
			status.Code = StatusBroken
			status.Message = fmt.Sprintf("Canonical skill error: %v", err)
			return status, nil
		}

		destHash, err := HashDirectory(destPath)
		if err != nil {
			status.Code = StatusBroken
			status.Message = fmt.Sprintf("Cannot hash target directory: %v", err)
			return status, nil
		}

		if canonicalHash == destHash {
			status.Code = StatusEnabled
			if record.Fallback {
				status.Message = "Active via copy fallback (synchronized)"
			} else {
				status.Message = "Active copy (synchronized)"
			}
			return status, nil
		}

		status.Code = StatusDrifted
		status.Message = "Target copy differs from canonical skill (drift detected, sync needed)"
		return status, nil
	}

	// Not tracked and not a symlink to canonical -> Conflict
	status.Code = StatusConflict
	status.Message = fmt.Sprintf("Collision: Path %s already exists and is not managed by Skills Manager", destPath)
	return status, nil
}
