package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImportCandidate represents a skill found in a target directory that is not yet in canonical store.
type ImportCandidate struct {
	TargetID    string `json:"target_id"`
	TargetLabel string `json:"target_label"`
	SkillName   string `json:"skill_name"`
	Description string `json:"description"`
	SourcePath  string `json:"source_path"`
	IsToml      bool   `json:"is_toml"`
}

// ScanImportCandidates scans all target directories for real (non-symlink) skills not in canonical store.
func (m *Manager) ScanImportCandidates() ([]ImportCandidate, error) {
	targets := m.GetTargets()
	state := m.GetState()
	var candidates []ImportCandidate

	for _, target := range targets {
		targetDir := ExpandPath(target.Path)
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(targetDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			entryPath := filepath.Join(targetDir, entry.Name())

			// Skip if it's a symlink pointing to canonical
			if isSym, linkTarget, _ := IsSymlink(entryPath); isSym {
				canonicalPath := filepath.Join(m.CanonicalDir, entry.Name())
				if PathsReferToSameLocation(linkTarget, canonicalPath) {
					continue
				}
			}

			// Handle Gemini TOML targets
			if target.Format == FormatGeminiTOML {
				if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".toml") {
					continue
				}

				skillName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
				if state.IsTracked(target.ID, skillName) {
					continue
				}

				// Check if skill name already exists in canonical
				canonicalSkillPath := filepath.Join(m.CanonicalDir, skillName)
				if _, err := os.Stat(canonicalSkillPath); err == nil {
					// Already exists in canonical
					continue
				}

				content, err := os.ReadFile(entryPath)
				if err != nil {
					continue
				}

				skill, err := GeminiTOMLToSkill(skillName, string(content))
				desc := ""
				if err == nil {
					desc = skill.Description
				}

				candidates = append(candidates, ImportCandidate{
					TargetID:    target.ID,
					TargetLabel: target.Label,
					SkillName:   skillName,
					Description: desc,
					SourcePath:  entryPath,
					IsToml:      true,
				})
				continue
			}

			// Handle regular skill directories
			if !entry.IsDir() {
				continue
			}

			skillName := entry.Name()
			if state.IsTracked(target.ID, skillName) {
				continue
			}

			// Check if already in canonical
			canonicalSkillPath := filepath.Join(m.CanonicalDir, skillName)
			if _, err := os.Stat(canonicalSkillPath); err == nil {
				continue
			}

			// Try parsing SKILL.md inside
			desc := "Discovered skill"
			skillFile := filepath.Join(entryPath, "SKILL.md")
			if _, err := os.Stat(skillFile); os.IsNotExist(err) {
				skillFile = filepath.Join(entryPath, "skill.md")
			}

			if parsed, err := ParseSkillFile(skillFile); err == nil {
				desc = parsed.Description
				if parsed.Name != "" {
					skillName = parsed.Name
				}
			}

			candidates = append(candidates, ImportCandidate{
				TargetID:    target.ID,
				TargetLabel: target.Label,
				SkillName:   skillName,
				Description: desc,
				SourcePath:  entryPath,
				IsToml:      false,
			})
		}
	}

	return candidates, nil
}

// ImportCandidate moves the selected candidate into canonical store and leaves behind a link/copy.
func (m *Manager) ImportCandidate(candidate ImportCandidate) error {
	destCanonicalDir := filepath.Join(m.CanonicalDir, candidate.SkillName)
	if _, err := os.Stat(destCanonicalDir); err == nil {
		return fmt.Errorf("canonical skill %q already exists at %s", candidate.SkillName, destCanonicalDir)
	}

	if candidate.IsToml {
		content, err := os.ReadFile(candidate.SourcePath)
		if err != nil {
			return fmt.Errorf("failed to read source toml %s: %w", candidate.SourcePath, err)
		}

		skill, err := GeminiTOMLToSkill(candidate.SkillName, string(content))
		if err != nil {
			return fmt.Errorf("failed to parse Gemini TOML: %w", err)
		}

		if err := os.MkdirAll(destCanonicalDir, 0755); err != nil {
			return fmt.Errorf("failed to create canonical directory %s: %w", destCanonicalDir, err)
		}

		skillFilePath := filepath.Join(destCanonicalDir, "SKILL.md")
		if err := os.WriteFile(skillFilePath, []byte(skill.Content), 0644); err != nil {
			return fmt.Errorf("failed to write SKILL.md: %w", err)
		}

		// Remove source TOML file
		_ = os.Remove(candidate.SourcePath)
	} else {
		// Move directory to canonical store
		err := os.Rename(candidate.SourcePath, destCanonicalDir)
		if err != nil {
			// Cross-device rename fallback: Copy then Remove
			if copyErr := CopyDirectory(candidate.SourcePath, destCanonicalDir); copyErr != nil {
				return fmt.Errorf("failed to copy directory during import: %w", copyErr)
			}
			_ = os.RemoveAll(candidate.SourcePath)
		}

		// Ensure SKILL.md exists
		skillFilePath := filepath.Join(destCanonicalDir, "SKILL.md")
		if _, err := os.Stat(skillFilePath); os.IsNotExist(err) {
			// Check if lowercase exists
			lowerPath := filepath.Join(destCanonicalDir, "skill.md")
			if _, lErr := os.Stat(lowerPath); lErr == nil {
				_ = os.Rename(lowerPath, skillFilePath)
			} else {
				// Create default SKILL.md
				fm := Frontmatter{
					Name:        candidate.SkillName,
					Description: candidate.Description,
				}
				content, _ := SerializeSkillMarkdown(fm, "# "+candidate.SkillName+"\n")
				_ = os.WriteFile(skillFilePath, []byte(content), 0644)
			}
		}
	}

	// Now re-enable for that target to leave a link/copy behind per that target's mode!
	_, err := m.Enable(candidate.SkillName, candidate.TargetID)
	if err != nil {
		return fmt.Errorf("imported to canonical, but failed to link back to %s: %w", candidate.TargetLabel, err)
	}

	return nil
}

// ImportSelected imports multiple candidates.
func (m *Manager) ImportSelected(candidates []ImportCandidate) ([]string, []error) {
	var imported []string
	var errs []error

	for _, c := range candidates {
		if err := m.ImportCandidate(c); err != nil {
			errs = append(errs, fmt.Errorf("failed to import %q from %s: %w", c.SkillName, c.TargetLabel, err))
		} else {
			imported = append(imported, c.SkillName)
		}
	}

	return imported, errs
}
