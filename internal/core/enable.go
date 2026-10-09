package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnableResult provides details on the enable action performed.
type EnableResult struct {
	SkillName    string
	TargetID     string
	TargetPath   string
	Mode         string
	Fallback     bool
	Message      string
	RecordedHash string
}

// Enable activates a canonical skill for a specific target.
func (m *Manager) Enable(skillName, targetID string) (*EnableResult, error) {
	target, exists := m.GetTargetByID(targetID)
	if !exists {
		return nil, fmt.Errorf("target %q not found", targetID)
	}

	canonicalSkillPath := filepath.Join(m.CanonicalDir, skillName)
	canonicalSkill, err := ParseSkill(canonicalSkillPath)
	if err != nil {
		return nil, fmt.Errorf("cannot enable skill %q: invalid or missing canonical skill: %w", skillName, err)
	}

	destPath := GetTargetDestinationPath(target, skillName)

	// Check status to enforce collision rule
	status, err := m.Status(skillName, targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to check status: %w", err)
	}

	if status.Code == StatusConflict {
		return nil, fmt.Errorf("cannot enable %q on %s: %s", skillName, target.Label, status.Message)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory %s: %w", filepath.Dir(destPath), err)
	}

	result := &EnableResult{
		SkillName:  skillName,
		TargetID:   targetID,
		TargetPath: destPath,
	}

	// Handle Gemini TOML format
	if target.Format == FormatGeminiTOML {
		tomlStr, err := SkillToGeminiTOML(*canonicalSkill)
		if err != nil {
			return nil, fmt.Errorf("failed to generate Gemini TOML: %w", err)
		}

		if err := os.WriteFile(destPath, []byte(tomlStr), 0644); err != nil {
			return nil, fmt.Errorf("failed to write Gemini TOML to %s: %w", destPath, err)
		}

		contentHash := HashString(tomlStr)
		m.GetState().SetRecord(targetID, skillName, TargetRecord{
			Mode:   ModeCopy,
			Format: FormatGeminiTOML,
			Hash:   contentHash,
			Path:   destPath,
		})

		if err := m.SaveState(); err != nil {
			return nil, fmt.Errorf("failed to save state: %w", err)
		}

		result.Mode = ModeCopy
		result.RecordedHash = contentHash
		result.Message = "Generated Gemini TOML command"
		return result, nil
	}

	// Handle Symlink Mode
	if target.Mode == ModeSymlink {
		// If already exists and is our symlink or tracked copy, remove old link/dir before recreating
		if status.Code == StatusEnabled || status.Code == StatusBroken || status.Code == StatusDrifted {
			if isSym, _, _ := IsSymlink(destPath); isSym {
				_ = os.Remove(destPath)
			} else if m.GetState().IsTracked(targetID, skillName) {
				_ = os.RemoveAll(destPath)
			}
		}

		linkRes, err := CreateSymlinkOrFallback(canonicalSkillPath, destPath)
		if err != nil {
			return nil, err
		}

		var currentHash string
		if linkRes.Type == LinkTypeCopy {
			currentHash, _ = HashDirectory(canonicalSkillPath)
		}

		m.GetState().SetRecord(targetID, skillName, TargetRecord{
			Mode:     string(linkRes.Type),
			Format:   FormatSkill,
			Hash:     currentHash,
			Path:     destPath,
			Fallback: linkRes.Fallback,
		})

		if err := m.SaveState(); err != nil {
			return nil, fmt.Errorf("failed to save state: %w", err)
		}

		result.Mode = string(linkRes.Type)
		result.Fallback = linkRes.Fallback
		result.RecordedHash = currentHash
		result.Message = linkRes.Message
		return result, nil
	}

	// Handle Copy Mode
	if target.Mode == ModeCopy {
		if status.Code == StatusEnabled || status.Code == StatusDrifted {
			if m.GetState().IsTracked(targetID, skillName) {
				_ = os.RemoveAll(destPath)
			}
		}

		if err := CopyDirectory(canonicalSkillPath, destPath); err != nil {
			return nil, fmt.Errorf("failed to copy skill to %s: %w", destPath, err)
		}

		canonicalHash, err := HashDirectory(canonicalSkillPath)
		if err != nil {
			return nil, fmt.Errorf("failed to hash canonical skill directory: %w", err)
		}

		m.GetState().SetRecord(targetID, skillName, TargetRecord{
			Mode:   ModeCopy,
			Format: FormatSkill,
			Hash:   canonicalHash,
			Path:   destPath,
		})

		if err := m.SaveState(); err != nil {
			return nil, fmt.Errorf("failed to save state: %w", err)
		}

		result.Mode = ModeCopy
		result.RecordedHash = canonicalHash
		result.Message = "Copied skill directory"
		return result, nil
	}

	return nil, fmt.Errorf("unsupported mode %q for target %q", target.Mode, targetID)
}

// Disable deactivates a skill for a specific target, safely removing only files created by Skills Manager.
func (m *Manager) Disable(skillName, targetID string) error {
	target, exists := m.GetTargetByID(targetID)
	if !exists {
		return fmt.Errorf("target %q not found", targetID)
	}

	canonicalSkillPath := filepath.Join(m.CanonicalDir, skillName)
	destPath := GetTargetDestinationPath(target, skillName)

	lstat, err := os.Lstat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Already gone; clean state if present
			m.GetState().DeleteRecord(targetID, skillName)
			return m.SaveState()
		}
		return fmt.Errorf("failed to inspect %s: %w", destPath, err)
	}

	// Safety check 1: Symlink check
	if lstat.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(destPath)
		if err == nil {
			if !filepath.IsAbs(linkTarget) {
				linkTarget = filepath.Join(filepath.Dir(destPath), linkTarget)
			}
			if !PathsReferToSameLocation(linkTarget, canonicalSkillPath) {
				return fmt.Errorf("refusing to remove %s: symlink points to %s, not canonical skill %s", destPath, linkTarget, canonicalSkillPath)
			}
		}
		if err := os.Remove(destPath); err != nil {
			return fmt.Errorf("failed to remove symlink %s: %w", destPath, err)
		}
		m.GetState().DeleteRecord(targetID, skillName)
		return m.SaveState()
	}

	// Safety check 2: Regular file or directory must be tracked in state.json
	if !m.GetState().IsTracked(targetID, skillName) {
		return fmt.Errorf("refusing to remove %s: target exists but is not tracked in state.json and not a symlink to canonical store (collision protection)", destPath)
	}

	if target.Format == FormatGeminiTOML {
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove Gemini TOML file %s: %w", destPath, err)
		}
	} else {
		if err := os.RemoveAll(destPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove skill directory %s: %w", destPath, err)
		}
	}

	m.GetState().DeleteRecord(targetID, skillName)
	return m.SaveState()
}

// SyncReport summarizes the results of a synchronization operation.
type SyncReport struct {
	Updated []string
	Skipped []string
	Errors  []string
}

// Sync synchronizes all active / drifted targets with canonical store.
func (m *Manager) Sync() (*SyncReport, error) {
	skills, err := ListSkills(m.CanonicalDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list canonical skills: %w", err)
	}

	targets := m.GetTargets()
	report := &SyncReport{}

	for _, skill := range skills {
		for _, target := range targets {
			status, err := m.Status(skill.Name, target.ID)
			if err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("%s / %s: %v", skill.Name, target.Label, err))
				continue
			}

			switch status.Code {
			case StatusDrifted:
				// Re-enable to update copy or TOML
				res, err := m.Enable(skill.Name, target.ID)
				if err != nil {
					report.Errors = append(report.Errors, fmt.Sprintf("%s / %s: %v", skill.Name, target.Label, err))
				} else {
					report.Updated = append(report.Updated, fmt.Sprintf("%s -> %s (%s)", skill.Name, target.Label, res.Message))
				}

			case StatusBroken:
				// Repair broken symlink if canonical exists
				_, err := m.Enable(skill.Name, target.ID)
				if err != nil {
					report.Errors = append(report.Errors, fmt.Sprintf("Repair %s / %s: %v", skill.Name, target.Label, err))
				} else {
					report.Updated = append(report.Updated, fmt.Sprintf("Repaired %s -> %s", skill.Name, target.Label))
				}

			case StatusConflict:
				report.Skipped = append(report.Skipped, fmt.Sprintf("%s / %s: %s", skill.Name, target.Label, status.Message))

			case StatusEnabled, StatusDisabled:
				// No action needed
			}
		}
	}

	return report, nil
}
