package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/skilled-manager/skills-manager/pkg/core"
)

// EditorPane provides a multi-line markdown editor for SKILL.md with frontmatter validation.
type EditorPane struct {
	window          fyne.Window
	container       *fyne.Container
	titleLabel      *widget.Label
	validationLabel *widget.Label
	entry           *widget.Entry
	saveBtn         *widget.Button
	currentSkill    *core.Skill
	onSaved         func()
}

// NewEditorPane creates a new EditorPane.
func NewEditorPane(win fyne.Window, onSaved func()) *EditorPane {
	ep := &EditorPane{
		window:  win,
		onSaved: onSaved,
	}

	ep.titleLabel = widget.NewLabelWithStyle("SKILL.md Editor", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ep.validationLabel = widget.NewLabel("Select a skill to edit")

	ep.entry = widget.NewMultiLineEntry()
	ep.entry.Wrapping = fyne.TextWrapBreak
	ep.entry.SetPlaceHolder("--- \nname: my-skill\ndescription: Skill description\n---\n# Instructions...")

	ep.saveBtn = widget.NewButtonWithIcon("Save SKILL.md", theme.DocumentSaveIcon(), func() {
		ep.save()
	})
	ep.saveBtn.Importance = widget.HighImportance
	ep.saveBtn.Disable()

	ep.entry.OnChanged = func(content string) {
		ep.validate(content)
	}

	headerLeft := container.NewVBox(ep.titleLabel, ep.validationLabel)
	header := container.NewBorder(nil, nil, headerLeft, ep.saveBtn)

	ep.container = container.NewBorder(header, nil, nil, nil, ep.entry)

	return ep
}

// LoadSkill loads a skill into the editor.
func (ep *EditorPane) LoadSkill(skill *core.Skill) {
	ep.currentSkill = skill
	if skill == nil {
		ep.titleLabel.SetText("SKILL.md Editor")
		ep.validationLabel.SetText("Select a skill from the left to edit")
		ep.entry.SetText("")
		ep.entry.Disable()
		ep.saveBtn.Disable()
		return
	}

	ep.entry.Enable()
	ep.titleLabel.SetText(fmt.Sprintf("Editing: %s (%s)", skill.Name, skill.Path))
	ep.entry.SetText(skill.Content)
	ep.validate(skill.Content)
}

// Container returns the Fyne canvas object for the editor.
func (ep *EditorPane) Container() fyne.CanvasObject {
	return ep.container
}

func (ep *EditorPane) validate(content string) {
	if ep.currentSkill == nil {
		ep.saveBtn.Disable()
		return
	}

	parsed, err := core.ParseFrontmatter(content)
	if err != nil {
		ep.validationLabel.SetText(fmt.Sprintf("[Error] Frontmatter error: %v", err))
		ep.saveBtn.Disable()
		return
	}

	if err := core.ValidateFrontmatter(parsed.Frontmatter, ep.currentSkill.Name); err != nil {
		ep.validationLabel.SetText(fmt.Sprintf("[Error] Validation error: %v", err))
		ep.saveBtn.Disable()
		return
	}

	ep.validationLabel.SetText("[Valid] Frontmatter valid (name matches folder, description non-empty)")
	ep.saveBtn.Enable()
}

func (ep *EditorPane) save() {
	if ep.currentSkill == nil {
		return
	}

	content := ep.entry.Text
	updatedSkill, err := core.WriteSkill(ep.currentSkill.Path, content)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to save skill: %w", err), ep.window)
		return
	}

	ep.currentSkill = updatedSkill
	ep.validate(content)

	dialog.ShowInformation("Saved", fmt.Sprintf("Successfully saved SKILL.md for %q", updatedSkill.Name), ep.window)

	if ep.onSaved != nil {
		ep.onSaved()
	}
}
