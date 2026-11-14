package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/speedhs/skilled-manager/pkg/core"
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
	saved           string
	dirty           bool
	onSaved         func()
	prompting       bool
}

// Dirty reports unsaved editor text.
func (ep *EditorPane) Dirty() bool { return ep.dirty }

// SkillName is the skill currently open in the editor.
func (ep *EditorPane) SkillName() string {
	if ep.currentSkill == nil {
		return ""
	}
	return ep.currentSkill.Name
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
		ep.saveAsync()
	})
	ep.saveBtn.Importance = widget.HighImportance
	ep.saveBtn.Disable()
	undoBtn := widget.NewButton("Undo", func() {
		ep.entry.TypedShortcut(&fyne.ShortcutUndo{})
	})
	redoBtn := widget.NewButton("Redo", func() {
		ep.entry.TypedShortcut(&fyne.ShortcutRedo{})
	})

	ep.entry.OnChanged = func(content string) {
		ep.dirty = content != ep.saved
		ep.validate(content)
	}

	headerLeft := container.NewVBox(ep.titleLabel, ep.validationLabel)
	header := container.NewBorder(nil, nil, headerLeft, container.NewHBox(undoBtn, redoBtn, ep.saveBtn))

	ep.container = container.NewBorder(header, nil, nil, nil, ep.entry)

	return ep
}

// LoadSkill loads a skill into the editor.
// Unsaved text is kept until the user confirms a discard.
func (ep *EditorPane) LoadSkill(skill *core.Skill) {
	if ep.prompting {
		return
	}
	same := skill != nil && ep.currentSkill != nil && skill.Path == ep.currentSkill.Path
	if same && skill.Content == ep.saved {
		return
	}
	if same && ep.dirty && skill.Content != ep.saved {
		ep.prompting = true
		dialog.ShowConfirm("Skill changed on disk", "Reload the file from disk and discard your edits?", func(ok bool) {
			ep.prompting = false
			if ok {
				ep.dirty = false
				ep.load(skill)
			}
		}, ep.window)
		return
	}
	if ep.dirty && !same {
		ep.prompting = true
		dialog.ShowConfirm("Unsaved changes", "Discard unsaved edits to this skill?", func(ok bool) {
			ep.prompting = false
			if !ok {
				return
			}
			ep.dirty = false
			ep.load(skill)
		}, ep.window)
		return
	}
	ep.load(skill)
}

func (ep *EditorPane) load(skill *core.Skill) {
	ep.currentSkill = skill
	ep.dirty = false
	if skill == nil {
		ep.titleLabel.SetText("SKILL.md Editor")
		ep.validationLabel.SetText("Select a skill from the left to edit")
		ep.saved = ""
		ep.entry.SetText("")
		ep.entry.Disable()
		ep.saveBtn.Disable()
		return
	}
	if len(skill.Content) > maxEditorBytes {
		ep.titleLabel.SetText(fmt.Sprintf("Editing: %s (%s)", skill.Name, skill.Path))
		ep.validationLabel.SetText("This SKILL.md is larger than 1 MiB. Edit it in an external editor, then refresh.")
		ep.saved = ""
		ep.entry.SetText("")
		ep.entry.Disable()
		ep.saveBtn.Disable()
		return
	}

	ep.entry.Enable()
	ep.titleLabel.SetText(fmt.Sprintf("Editing: %s (%s)", skill.Name, skill.Path))
	ep.saved = skill.Content
	ep.entry.SetText(skill.Content)
	ep.dirty = false
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

const maxEditorBytes = 1 << 20

func (ep *EditorPane) saveAsync() {
	if ep.currentSkill == nil {
		return
	}
	content := ep.entry.Text
	path := ep.currentSkill.Path
	ep.saveBtn.Disable()
	go func() {
		updated, err := core.WriteSkill(path, content)
		fyne.Do(func() {
			if err != nil {
				showError(fmt.Errorf("failed to save skill: %w", err), ep.window)
				ep.validate(ep.entry.Text)
				return
			}
			ep.finishSave(updated, content)
		})
	}()
}

func (ep *EditorPane) save() {
	if ep.currentSkill == nil {
		return
	}

	content := ep.entry.Text
	updatedSkill, err := core.WriteSkill(ep.currentSkill.Path, content)
	if err != nil {
		showError(fmt.Errorf("failed to save skill: %w", err), ep.window)
		return
	}
	ep.finishSave(updatedSkill, content)
}

func (ep *EditorPane) finishSave(updatedSkill *core.Skill, content string) {

	ep.currentSkill = updatedSkill
	ep.saved = content
	ep.dirty = false
	ep.validate(content)

	dialog.ShowInformation("Saved", fmt.Sprintf("Successfully saved SKILL.md for %q", updatedSkill.Name), ep.window)

	if ep.onSaved != nil {
		ep.onSaved()
	}
}
