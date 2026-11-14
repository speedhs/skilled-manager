//go:build fyne_gui

package ui

import (
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/speedhs/skilled-manager/pkg/core"
)

func setupTestManager(t *testing.T) (*core.Manager, string) {
	t.Helper()
	tempDir := t.TempDir()
	canonicalDir := filepath.Join(tempDir, "agent-skills")
	configDir := filepath.Join(tempDir, "agent-skills-manager")

	mgr, err := core.NewManager(
		core.WithCanonicalDir(canonicalDir),
		core.WithConfigDir(configDir),
	)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	targets := []core.Target{
		{
			ID:     "claude",
			Label:  "Claude Code",
			Path:   filepath.Join(tempDir, "claude-skills"),
			Mode:   core.ModeSymlink,
			Format: core.FormatSkill,
		},
		{
			ID:     "gemini",
			Label:  "Gemini CLI",
			Path:   filepath.Join(tempDir, "gemini-commands"),
			Mode:   core.ModeCopy,
			Format: core.FormatGeminiTOML,
		},
	}

	if err := mgr.UpdateTargets(targets); err != nil {
		t.Fatalf("failed to update targets: %v", err)
	}

	return mgr, canonicalDir
}

func TestSkillListPane(t *testing.T) {
	_ = test.NewApp()

	var selectedSkill core.Skill
	pane := NewSkillListPane(func(skill core.Skill) {
		selectedSkill = skill
	})

	skills := []core.Skill{
		{Name: "alpha-skill", Description: "Alpha description"},
		{Name: "beta-skill", Description: "Beta description"},
		{Name: "gamma-skill", Description: "Gamma description"},
	}

	pane.SetSkills(skills)

	if len(pane.filteredSkills) != 3 {
		t.Fatalf("expected 3 filtered skills, got %d", len(pane.filteredSkills))
	}

	pane.searchEntry.SetText("beta")
	waitFilter(t, func() bool {
		return len(pane.filteredSkills) == 1 && pane.filteredSkills[0].Name == "beta-skill"
	})
	if len(pane.filteredSkills) != 1 || pane.filteredSkills[0].Name != "beta-skill" {
		t.Fatalf("search filter failed: expected 1 skill 'beta-skill', got %d", len(pane.filteredSkills))
	}

	pane.searchEntry.SetText("")
	waitFilter(t, func() bool { return len(pane.filteredSkills) == 3 })
	if len(pane.filteredSkills) != 3 {
		t.Fatalf("clearing search filter failed: expected 3, got %d", len(pane.filteredSkills))
	}

	// Test selection
	pane.SelectSkillByName("gamma-skill")
	if selectedSkill.Name != "gamma-skill" {
		t.Errorf("expected selected skill gamma-skill, got %s", selectedSkill.Name)
	}
}

func waitFilter(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestEditorPaneValidationAndSave(t *testing.T) {
	testApp := test.NewApp()
	testWin := testApp.NewWindow("Test")

	saved := false
	editor := NewEditorPane(testWin, func() {
		saved = true
	})

	_, canonicalDir := setupTestManager(t)
	skill, err := core.CreateSkill(canonicalDir, "editor-test", "Initial Description", "# Instructions")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	editor.LoadSkill(skill)

	// Valid content
	if editor.saveBtn.Disabled() {
		t.Errorf("expected save button to be enabled for valid skill")
	}

	// Invalid content: empty name
	invalidContent := `---
name: ""
description: Some description
---
# Body
`
	editor.entry.SetText(invalidContent)
	if !editor.saveBtn.Disabled() {
		t.Errorf("expected save button to be disabled for invalid empty name")
	}

	// Valid updated content
	updatedContent := `---
name: editor-test
description: Updated description for editor test
---
# Updated instructions
`
	editor.entry.SetText(updatedContent)
	if editor.saveBtn.Disabled() {
		t.Errorf("expected save button to be enabled for valid updated content")
	}

	// Save
	editor.save()
	if !saved {
		t.Errorf("expected onSaved callback to be triggered")
	}

	// Reload from canonical to verify file write
	reloaded, err := core.ParseSkill(skill.Path)
	if err != nil {
		t.Fatalf("failed to reload skill: %v", err)
	}
	if reloaded.Description != "Updated description for editor test" {
		t.Errorf("expected updated description in file, got %q", reloaded.Description)
	}
}

func TestMatrixPaneRendering(t *testing.T) {
	testApp := test.NewApp()
	testWin := testApp.NewWindow("Matrix Test")

	mgr, canonicalDir := setupTestManager(t)
	skill, err := core.CreateSkill(canonicalDir, "matrix-skill", "Matrix skill description", "# Instructions")
	if err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	matrix := NewMatrixPane(mgr, testWin, nil)
	matrix.SetData([]core.Skill{*skill}, mgr.GetTargets())
	if len(matrix.content.Objects) == 0 {
		t.Fatalf("expected matrix content to have objects")
	}
	st := matrix.statusFor(skill.Name, "claude")
	if GetStatusLabel(st.Code) != "Disabled" {
		t.Fatalf("status text = %q, want Disabled", GetStatusLabel(st.Code))
	}
}
