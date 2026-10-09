package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode constants
const (
	ModeSymlink = "symlink"
	ModeCopy    = "copy"
)

// Format constants
const (
	FormatSkill      = "skill"
	FormatGeminiTOML = "gemini-toml"
)

// Target represents a tool target configuration.
type Target struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Format string `json:"format"`
}

// DefaultTargets returns the default list of targets supported by Skills Manager.
func DefaultTargets() []Target {
	return []Target{
		{
			ID:     "claude",
			Label:  "Claude Code",
			Path:   "~/.claude/skills",
			Mode:   ModeSymlink,
			Format: FormatSkill,
		},
		{
			ID:     "codex",
			Label:  "Codex CLI",
			Path:   "~/.codex/skills",
			Mode:   ModeSymlink,
			Format: FormatSkill,
		},
		{
			ID:     "opencode",
			Label:  "OpenCode",
			Path:   "~/.config/opencode/skill",
			Mode:   ModeSymlink,
			Format: FormatSkill,
		},
		{
			ID:     "gemini",
			Label:  "Gemini CLI",
			Path:   "~/.gemini/commands",
			Mode:   ModeCopy,
			Format: FormatGeminiTOML,
		},
	}
}

// LoadTargets loads targets from targets.json or initializes it with defaults if missing.
func LoadTargets(filePath string) ([]Target, error) {
	cleanPath := ExpandPath(filePath)

	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		defaults := DefaultTargets()
		if err := SaveTargets(cleanPath, defaults); err != nil {
			return defaults, fmt.Errorf("failed to create default targets file at %s: %w", cleanPath, err)
		}
		return defaults, nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read targets file %s: %w", cleanPath, err)
	}

	var targets []Target
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, fmt.Errorf("failed to parse targets JSON from %s: %w", cleanPath, err)
	}

	// Validate targets
	for i, t := range targets {
		if strings.TrimSpace(t.ID) == "" {
			return nil, fmt.Errorf("target at index %d has empty ID", i)
		}
		if strings.TrimSpace(t.Path) == "" {
			return nil, fmt.Errorf("target %q has empty path", t.ID)
		}
		if t.Mode == "" {
			targets[i].Mode = ModeSymlink
		}
		if t.Format == "" {
			targets[i].Format = FormatSkill
		}
	}

	return targets, nil
}

// SaveTargets saves the list of targets to targets.json.
func SaveTargets(filePath string, targets []Target) error {
	cleanPath := ExpandPath(filePath)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create targets directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode targets JSON: %w", err)
	}

	// Write atomically or direct
	if err := os.WriteFile(cleanPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write targets file %s: %w", cleanPath, err)
	}

	return nil
}
