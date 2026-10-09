package core

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupTestEnv creates isolated temporary directories for canonical and config stores.
func setupTestEnv(t *testing.T) (*Manager, string, string) {
	t.Helper()
	tempDir := t.TempDir()
	canonicalDir := filepath.Join(tempDir, "agent-skills")
	configDir := filepath.Join(tempDir, "agent-skills-manager")

	mgr, err := NewManager(
		WithCanonicalDir(canonicalDir),
		WithConfigDir(configDir),
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Create fake target directories
	claudeDir := filepath.Join(tempDir, "claude-skills")
	codexDir := filepath.Join(tempDir, "codex-skills")
	opencodeDir := filepath.Join(tempDir, "opencode-skills")
	geminiDir := filepath.Join(tempDir, "gemini-commands")

	targets := []Target{
		{ID: "claude", Label: "Claude Code", Path: claudeDir, Mode: ModeSymlink, Format: FormatSkill},
		{ID: "codex", Label: "Codex CLI", Path: codexDir, Mode: ModeSymlink, Format: FormatSkill},
		{ID: "opencode", Label: "OpenCode", Path: opencodeDir, Mode: ModeCopy, Format: FormatSkill},
		{ID: "gemini", Label: "Gemini CLI", Path: geminiDir, Mode: ModeCopy, Format: FormatGeminiTOML},
	}

	if err := mgr.UpdateTargets(targets); err != nil {
		t.Fatalf("failed to update targets: %v", err)
	}

	return mgr, canonicalDir, configDir
}

func TestFrontmatterParsingAndValidation(t *testing.T) {
	t.Run("valid frontmatter", func(t *testing.T) {
		content := `---
name: code-reviewer
description: Expert code reviewer for Go and TypeScript
author: speed
---
# Code Reviewer
Always check for error handling and memory leaks.
`
		parsed, err := ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error parsing valid frontmatter: %v", err)
		}
		if parsed.Frontmatter.Name != "code-reviewer" {
			t.Errorf("expected name 'code-reviewer', got %q", parsed.Frontmatter.Name)
		}
		if parsed.Frontmatter.Description != "Expert code reviewer for Go and TypeScript" {
			t.Errorf("expected description 'Expert code reviewer for Go and TypeScript', got %q", parsed.Frontmatter.Description)
		}
		if !strings.Contains(parsed.Body, "# Code Reviewer") {
			t.Errorf("expected body to contain header, got %q", parsed.Body)
		}

		err = ValidateFrontmatter(parsed.Frontmatter, "code-reviewer")
		if err != nil {
			t.Errorf("expected frontmatter to validate against matching directory name: %v", err)
		}
	})

	t.Run("missing delimiters", func(t *testing.T) {
		content := `# No frontmatter
Just markdown text.
`
		_, err := ParseFrontmatter(content)
		if err == nil {
			t.Fatal("expected error on missing frontmatter, got nil")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		fm := Frontmatter{Name: "", Description: "Some description"}
		err := ValidateFrontmatter(fm, "some-folder")
		if err != ErrEmptySkillName {
			t.Fatalf("expected ErrEmptySkillName, got %v", err)
		}
	})

	t.Run("empty description", func(t *testing.T) {
		fm := Frontmatter{Name: "my-skill", Description: "   "}
		err := ValidateFrontmatter(fm, "my-skill")
		if err != ErrEmptyDescription {
			t.Fatalf("expected ErrEmptyDescription, got %v", err)
		}
	})

	t.Run("name mismatch with folder", func(t *testing.T) {
		fm := Frontmatter{Name: "skill-one", Description: "Test"}
		err := ValidateFrontmatter(fm, "skill-two")
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("expected name mismatch error, got %v", err)
		}
	})

	t.Run("serialization round-trip", func(t *testing.T) {
		fm := Frontmatter{
			Name:        "serialize-test",
			Description: "Serialization test description",
		}
		body := "## Test Body\nSome instructions."
		serialized, err := SerializeSkillMarkdown(fm, body)
		if err != nil {
			t.Fatalf("failed to serialize: %v", err)
		}

		parsed, err := ParseFrontmatter(serialized)
		if err != nil {
			t.Fatalf("failed to parse serialized content: %v", err)
		}

		if parsed.Frontmatter.Name != fm.Name || parsed.Frontmatter.Description != fm.Description {
			t.Errorf("round-trip mismatch: got %+v, expected %+v", parsed.Frontmatter, fm)
		}
		if strings.TrimSpace(parsed.Body) != strings.TrimSpace(body) {
			t.Errorf("round-trip body mismatch: got %q, expected %q", parsed.Body, body)
		}
	})
}

func TestEnableDisableSymlinkMode(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	// Create skill in canonical store
	skill, err := CreateSkill(canonicalDir, "symlink-skill", "Symlink skill test", "# Symlink instructions")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// Status before enable should be disabled
	status, err := mgr.Status(skill.Name, "claude")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusDisabled {
		t.Fatalf("expected StatusDisabled, got %s", status.Code)
	}

	// Enable for Claude (Symlink mode)
	res, err := mgr.Enable(skill.Name, "claude")
	if err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	if res.SkillName != "symlink-skill" || res.TargetID != "claude" {
		t.Errorf("unexpected enable result: %+v", res)
	}

	// Status after enable should be enabled
	status, err = mgr.Status(skill.Name, "claude")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusEnabled {
		t.Fatalf("expected StatusEnabled, got %s (msg: %s)", status.Code, status.Message)
	}
	if !status.IsSymlink {
		t.Errorf("expected status.IsSymlink to be true")
	}

	// Disable for Claude
	err = mgr.Disable(skill.Name, "claude")
	if err != nil {
		t.Fatalf("disable failed: %v", err)
	}

	// Status should be disabled again
	status, err = mgr.Status(skill.Name, "claude")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusDisabled {
		t.Fatalf("expected StatusDisabled, got %s", status.Code)
	}
}

func TestEnableDisableCopyMode(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	skill, err := CreateSkill(canonicalDir, "copy-skill", "Copy skill test", "# Copy instructions")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// Enable for OpenCode (Copy mode)
	res, err := mgr.Enable(skill.Name, "opencode")
	if err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	if res.Mode != ModeCopy {
		t.Errorf("expected ModeCopy, got %s", res.Mode)
	}

	// Check status is enabled
	status, err := mgr.Status(skill.Name, "opencode")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusEnabled {
		t.Fatalf("expected StatusEnabled, got %s (msg: %s)", status.Code, status.Message)
	}

	// Verify state is tracked
	if !mgr.GetState().IsTracked("opencode", skill.Name) {
		t.Fatalf("expected state to track opencode / %s", skill.Name)
	}

	// Disable
	err = mgr.Disable(skill.Name, "opencode")
	if err != nil {
		t.Fatalf("disable failed: %v", err)
	}

	status, err = mgr.Status(skill.Name, "opencode")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusDisabled {
		t.Fatalf("expected StatusDisabled, got %s", status.Code)
	}
}

func TestEnableDisableGeminiTOML(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	skill, err := CreateSkill(canonicalDir, "gemini-skill", "Gemini description", "Write a greeting")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// Enable for Gemini CLI
	res, err := mgr.Enable(skill.Name, "gemini")
	if err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	if res.Mode != ModeCopy {
		t.Errorf("expected mode copy for toml, got %s", res.Mode)
	}

	// Check status
	status, err := mgr.Status(skill.Name, "gemini")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusEnabled {
		t.Fatalf("expected StatusEnabled, got %s (msg: %s)", status.Code, status.Message)
	}

	// Check that destination file exists and is .toml
	if !strings.HasSuffix(status.ActualPath, "gemini-skill.toml") {
		t.Errorf("expected .toml path, got %s", status.ActualPath)
	}

	// Disable
	err = mgr.Disable(skill.Name, "gemini")
	if err != nil {
		t.Fatalf("disable failed: %v", err)
	}

	status, err = mgr.Status(skill.Name, "gemini")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusDisabled {
		t.Fatalf("expected StatusDisabled, got %s", status.Code)
	}
}

func TestCollisionRefusal(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	skill, err := CreateSkill(canonicalDir, "collision-skill", "Collision test", "# Body")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	targets := mgr.GetTargets()
	claudeTarget := targets[0]
	targetDir := ExpandPath(claudeTarget.Path)
	_ = os.MkdirAll(targetDir, 0755)

	// User manually created a folder with collision-skill name
	foreignDir := filepath.Join(targetDir, "collision-skill")
	if err := os.MkdirAll(foreignDir, 0755); err != nil {
		t.Fatalf("failed to create foreign dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(foreignDir, "manual.txt"), []byte("manual file"), 0644)

	// Status should report collision/conflict
	status, err := mgr.Status(skill.Name, "claude")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusConflict {
		t.Fatalf("expected StatusConflict, got %s (msg: %s)", status.Code, status.Message)
	}

	// Attempting to enable MUST be refused
	_, err = mgr.Enable(skill.Name, "claude")
	if err == nil {
		t.Fatal("expected Enable to fail on collision, but it succeeded")
	}
	if !strings.Contains(err.Error(), "Collision") && !strings.Contains(err.Error(), "collision") && !strings.Contains(err.Error(), "not managed") {
		t.Errorf("expected collision message in error, got: %v", err)
	}

	// Attempting to disable MUST NOT delete foreign directory
	err = mgr.Disable(skill.Name, "claude")
	if err == nil {
		t.Fatal("expected Disable to refuse deleting unmanaged directory, but got nil")
	}
	if _, statErr := os.Stat(foreignDir); statErr != nil {
		t.Fatalf("foreign directory was wrongfully deleted! %v", statErr)
	}
}

func TestDriftDetection(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	skill, err := CreateSkill(canonicalDir, "drift-skill", "Drift test initial", "# Initial body")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// Enable for OpenCode (copy mode)
	_, err = mgr.Enable(skill.Name, "opencode")
	if err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	// Enable for Gemini (gemini-toml)
	_, err = mgr.Enable(skill.Name, "gemini")
	if err != nil {
		t.Fatalf("enable gemini failed: %v", err)
	}

	// Initial status should be enabled
	status, _ := mgr.Status(skill.Name, "opencode")
	if status.Code != StatusEnabled {
		t.Fatalf("expected enabled, got %s", status.Code)
	}

	// Modify canonical skill
	time.Sleep(10 * time.Millisecond)
	newContent := `---
name: drift-skill
description: Drift test updated description
---
# Updated body with new instructions
`
	_, err = WriteSkill(skill.Path, newContent)
	if err != nil {
		t.Fatalf("write skill failed: %v", err)
	}

	// OpenCode status should now be drifted!
	status, err = mgr.Status(skill.Name, "opencode")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusDrifted {
		t.Fatalf("expected StatusDrifted for opencode, got %s (msg: %s)", status.Code, status.Message)
	}

	// Gemini status should now be drifted!
	status, err = mgr.Status(skill.Name, "gemini")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Code != StatusDrifted {
		t.Fatalf("expected StatusDrifted for gemini, got %s (msg: %s)", status.Code, status.Message)
	}

	// Sync should fix drift
	report, err := mgr.Sync()
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(report.Updated) < 2 {
		t.Errorf("expected at least 2 updated skills during sync, got %d (%v)", len(report.Updated), report.Updated)
	}

	// Status should be enabled again
	status, _ = mgr.Status(skill.Name, "opencode")
	if status.Code != StatusEnabled {
		t.Errorf("expected StatusEnabled after sync, got %s", status.Code)
	}
	status, _ = mgr.Status(skill.Name, "gemini")
	if status.Code != StatusEnabled {
		t.Errorf("expected StatusEnabled for gemini after sync, got %s", status.Code)
	}
}

func TestImportExistingSkills(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	targets := mgr.GetTargets()
	claudeDir := ExpandPath(targets[0].Path)
	geminiDir := ExpandPath(targets[3].Path)
	_ = os.MkdirAll(claudeDir, 0755)
	_ = os.MkdirAll(geminiDir, 0755)

	// Create an untracked real skill folder in Claude's target dir
	claudeSkillDir := filepath.Join(claudeDir, "untracked-claude-skill")
	_ = os.MkdirAll(claudeSkillDir, 0755)
	claudeSkillContent := `---
name: untracked-claude-skill
description: An untracked skill discovered in claude target
---
# Untracked
Instructions
`
	_ = os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte(claudeSkillContent), 0644)

	// Create an untracked Gemini command TOML
	geminiTOMLPath := filepath.Join(geminiDir, "untracked-gemini.toml")
	geminiTOMLContent := `description = "Discovered gemini command"
prompt = """
# Discovered Prompt
Gemini prompt instructions
"""
`
	_ = os.WriteFile(geminiTOMLPath, []byte(geminiTOMLContent), 0644)

	// Scan candidates
	candidates, err := mgr.ScanImportCandidates()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d (%+v)", len(candidates), candidates)
	}

	// Import all candidates
	imported, errs := mgr.ImportSelected(candidates)
	if len(errs) > 0 {
		t.Fatalf("import errors: %v", errs)
	}
	if len(imported) != 2 {
		t.Fatalf("expected 2 imported skills, got %d", len(imported))
	}

	// Verify skills now exist in canonical store
	skills, err := ListSkills(canonicalDir)
	if err != nil {
		t.Fatalf("list skills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 canonical skills, got %d", len(skills))
	}

	// Verify statuses are now enabled in their respective targets
	status, err := mgr.Status("untracked-claude-skill", "claude")
	if err != nil || status.Code != StatusEnabled {
		t.Errorf("expected untracked-claude-skill to be enabled on claude, got code=%s, err=%v", status.Code, err)
	}

	status, err = mgr.Status("untracked-gemini", "gemini")
	if err != nil || status.Code != StatusEnabled {
		t.Errorf("expected untracked-gemini to be enabled on gemini, got code=%s, err=%v", status.Code, err)
	}
}

func TestExportZip(t *testing.T) {
	mgr, canonicalDir, _ := setupTestEnv(t)

	_, err := CreateSkill(canonicalDir, "zip-skill-1", "First zip skill", "# Skill 1")
	if err != nil {
		t.Fatalf("failed to create skill 1: %v", err)
	}
	_, err = CreateSkill(canonicalDir, "zip-skill-2", "Second zip skill", "# Skill 2")
	if err != nil {
		t.Fatalf("failed to create skill 2: %v", err)
	}

	// Add a subfile in scripts/ to test subdirectory handling
	scriptsDir := filepath.Join(canonicalDir, "zip-skill-1", "scripts")
	_ = os.MkdirAll(scriptsDir, 0755)
	_ = os.WriteFile(filepath.Join(scriptsDir, "helper.sh"), []byte("#!/bin/sh\necho hello"), 0755)

	var buf bytes.Buffer
	err = mgr.ExportZip([]string{}, &buf)
	if err != nil {
		t.Fatalf("export zip failed: %v", err)
	}

	// Inspect ZIP
	zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	foundSkill1 := false
	foundSkill2 := false
	foundScript := false

	for _, f := range zipReader.File {
		if f.Name == "zip-skill-1/SKILL.md" {
			foundSkill1 = true
		}
		if f.Name == "zip-skill-2/SKILL.md" {
			foundSkill2 = true
		}
		if f.Name == "zip-skill-1/scripts/helper.sh" {
			foundScript = true
		}
	}

	if !foundSkill1 || !foundSkill2 || !foundScript {
		t.Errorf("zip missing expected files: skill1=%v, skill2=%v, script=%v", foundSkill1, foundSkill2, foundScript)
	}
}
