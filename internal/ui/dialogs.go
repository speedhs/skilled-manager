package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/skilled-manager/skills-manager/pkg/core"
)

// ShowNewSkillDialog opens a modal dialog to create a new skill in canonical store.
func ShowNewSkillDialog(mgr *core.Manager, win fyne.Window, onSuccess func(skill *core.Skill)) {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("e.g. git-commit-helper")

	descEntry := widget.NewEntry()
	descEntry.SetPlaceHolder("e.g. Generates conventional commit messages")

	bodyEntry := widget.NewMultiLineEntry()
	bodyEntry.SetPlaceHolder("Instructions and prompt markdown...")
	bodyEntry.SetMinRowsVisible(8)

	form := widget.NewForm(
		widget.NewFormItem("Skill Name", nameEntry),
		widget.NewFormItem("Description", descEntry),
		widget.NewFormItem("Instructions", bodyEntry),
	)

	d := dialog.NewCustomConfirm(
		"Create New Skill",
		"Create",
		"Cancel",
		form,
		func(ok bool) {
			if !ok {
				return
			}

			name := strings.TrimSpace(nameEntry.Text)
			desc := strings.TrimSpace(descEntry.Text)
			body := strings.TrimSpace(bodyEntry.Text)

			if name == "" {
				dialog.ShowError(fmt.Errorf("skill name cannot be empty"), win)
				return
			}
			if desc == "" {
				dialog.ShowError(fmt.Errorf("skill description cannot be empty"), win)
				return
			}
			if body == "" {
				body = fmt.Sprintf("# %s\n\nProvide agent instructions here.\n", name)
			}

			skill, err := core.CreateSkill(mgr.CanonicalDir, name, desc, body)
			if err != nil {
				dialog.ShowError(err, win)
				return
			}

			dialog.ShowInformation("Skill Created", fmt.Sprintf("Created skill %q at %s", skill.Name, skill.Path), win)
			if onSuccess != nil {
				onSuccess(skill)
			}
		},
		win,
	)

	d.Resize(fyne.NewSize(500, 400))
	d.Show()
}

// ShowImportDialog opens a modal to scan and import existing untracked skills.
func ShowImportDialog(mgr *core.Manager, win fyne.Window, onComplete func()) {
	candidates, err := mgr.ScanImportCandidates()
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to scan for skills: %w", err), win)
		return
	}

	if len(candidates) == 0 {
		dialog.ShowInformation("Import Existing Skills", "No untracked skills were found across any target directories.\nAll existing skills in targets are already managed or symlinked.", win)
		return
	}

	type candidateItem struct {
		candidate core.ImportCandidate
		check     *widget.Check
	}

	var items []candidateItem
	checksContainer := container.NewVBox()

	for _, c := range candidates {
		desc := c.Description
		if desc == "" {
			desc = "No description found"
		}
		label := fmt.Sprintf("%s (%s)\nSource: %s\n%s", c.SkillName, c.TargetLabel, c.SourcePath, desc)
		chk := widget.NewCheck(label, nil)
		chk.Checked = true // Checked by default
		items = append(items, candidateItem{candidate: c, check: chk})
		checksContainer.Add(chk)
		checksContainer.Add(widget.NewSeparator())
	}

	scroll := container.NewScroll(checksContainer)
	scroll.SetMinSize(fyne.NewSize(550, 300))

	content := container.NewBorder(
		widget.NewLabel(fmt.Sprintf("Discovered %d untracked skill(s). Select which to import into the canonical store:", len(candidates))),
		nil, nil, nil,
		scroll,
	)

	d := dialog.NewCustomConfirm(
		"Import Existing Skills",
		"Import Selected",
		"Cancel",
		content,
		func(ok bool) {
			if !ok {
				return
			}

			var selected []core.ImportCandidate
			for _, it := range items {
				if it.check.Checked {
					selected = append(selected, it.candidate)
				}
			}

			if len(selected) == 0 {
				return
			}

			imported, errs := mgr.ImportSelected(selected)
			msg := fmt.Sprintf("Imported %d skill(s) into canonical store:\n", len(imported))
			for _, name := range imported {
				msg += fmt.Sprintf("- %s\n", name)
			}
			if len(errs) > 0 {
				msg += "\nErrors:\n"
				for _, e := range errs {
					msg += fmt.Sprintf("[Error] %v\n", e)
				}
			}

			dialog.ShowInformation("Import Results", msg, win)
			if onComplete != nil {
				onComplete()
			}
		},
		win,
	)

	d.Resize(fyne.NewSize(600, 420))
	d.Show()
}

// ShowSettingsDialog opens target settings configuration editor.
func ShowSettingsDialog(mgr *core.Manager, win fyne.Window, onSaved func()) {
	targets := mgr.GetTargets()

	type targetForm struct {
		idEntry     *widget.Entry
		labelEntry  *widget.Entry
		pathEntry   *widget.Entry
		modeSelect  *widget.Select
		formatSelect *widget.Select
	}

	var forms []*targetForm
	listContainer := container.NewVBox()

	renderList := func() {
		listContainer.Objects = nil
		for i, t := range targets {
			idx := i
			id := widget.NewEntry()
			id.SetText(t.ID)

			lbl := widget.NewEntry()
			lbl.SetText(t.Label)

			p := widget.NewEntry()
			p.SetText(t.Path)

			mode := widget.NewSelect([]string{core.ModeSymlink, core.ModeCopy}, nil)
			mode.SetSelected(t.Mode)

			format := widget.NewSelect([]string{core.FormatSkill, core.FormatGeminiTOML}, nil)
			format.SetSelected(t.Format)

			tf := &targetForm{
				idEntry:     id,
				labelEntry:  lbl,
				pathEntry:   p,
				modeSelect:  mode,
				formatSelect: format,
			}
			if idx < len(forms) {
				forms[idx] = tf
			} else {
				forms = append(forms, tf)
			}

			removeBtn := widget.NewButton("Remove", func() {
				if len(targets) > 1 {
					targets = append(targets[:idx], targets[idx+1:]...)
					forms = append(forms[:idx], forms[idx+1:]...)
					// re-render
					listContainer.Refresh()
				} else {
					dialog.ShowError(fmt.Errorf("at least one target must remain configured"), win)
				}
			})
			removeBtn.Importance = widget.DangerImportance

			card := widget.NewCard(fmt.Sprintf("Target #%d: %s", idx+1, t.Label), "", container.NewVBox(
				widget.NewForm(
					widget.NewFormItem("ID", id),
					widget.NewFormItem("Label", lbl),
					widget.NewFormItem("Path", p),
					widget.NewFormItem("Mode", mode),
					widget.NewFormItem("Format", format),
				),
				removeBtn,
			))
			listContainer.Add(card)
		}
		listContainer.Refresh()
	}

	renderList()

	addBtn := widget.NewButton("+ Add Target", func() {
		targets = append(targets, core.Target{
			ID:     fmt.Sprintf("custom-%d", len(targets)+1),
			Label:  "Custom Tool",
			Path:   "~/.custom/skills",
			Mode:   core.ModeSymlink,
			Format: core.FormatSkill,
		})
		renderList()
	})

	resetBtn := widget.NewButton("Reset to Defaults", func() {
		targets = core.DefaultTargets()
		forms = nil
		renderList()
	})

	scroll := container.NewScroll(listContainer)
	scroll.SetMinSize(fyne.NewSize(580, 360))

	bottomBar := container.NewHBox(addBtn, resetBtn)
	content := container.NewBorder(
		widget.NewLabel("Configure tool deployment targets:"),
		bottomBar,
		nil, nil,
		scroll,
	)

	d := dialog.NewCustomConfirm(
		"Settings - Tool Targets",
		"Save",
		"Cancel",
		content,
		func(ok bool) {
			if !ok {
				return
			}

			var newTargets []core.Target
			for i, tf := range forms {
				if i >= len(targets) {
					break
				}
				id := strings.TrimSpace(tf.idEntry.Text)
				lbl := strings.TrimSpace(tf.labelEntry.Text)
				p := strings.TrimSpace(tf.pathEntry.Text)
				m := tf.modeSelect.Selected
				f := tf.formatSelect.Selected

				if id == "" || p == "" {
					dialog.ShowError(fmt.Errorf("target ID and Path cannot be empty"), win)
					return
				}

				newTargets = append(newTargets, core.Target{
					ID:     id,
					Label:  lbl,
					Path:   p,
					Mode:   m,
					Format: f,
				})
			}

			if err := mgr.UpdateTargets(newTargets); err != nil {
				dialog.ShowError(fmt.Errorf("failed to save targets: %w", err), win)
				return
			}

			dialog.ShowInformation("Saved", "Target configurations updated successfully.", win)
			if onSaved != nil {
				onSaved()
			}
		},
		win,
	)

	d.Resize(fyne.NewSize(620, 500))
	d.Show()
}

// ShowSyncReportDialog performs synchronization and displays a detailed report dialog.
func ShowSyncReportDialog(mgr *core.Manager, win fyne.Window, onDone func()) {
	report, err := mgr.Sync()
	if err != nil {
		dialog.ShowError(fmt.Errorf("sync failed: %w", err), win)
		return
	}

	msg := "Synchronization Results:\n\n"

	if len(report.Updated) > 0 {
		msg += fmt.Sprintf("Synchronized (%d):\n", len(report.Updated))
		for _, u := range report.Updated {
			msg += fmt.Sprintf("  - %s\n", u)
		}
		msg += "\n"
	}

	if len(report.Skipped) > 0 {
		msg += fmt.Sprintf("Skipped Conflicts (%d):\n", len(report.Skipped))
		for _, s := range report.Skipped {
			msg += fmt.Sprintf("  - %s\n", s)
		}
		msg += "\n"
	}

	if len(report.Errors) > 0 {
		msg += fmt.Sprintf("Errors (%d):\n", len(report.Errors))
		for _, e := range report.Errors {
			msg += fmt.Sprintf("  - %s\n", e)
		}
		msg += "\n"
	}

	if len(report.Updated) == 0 && len(report.Skipped) == 0 && len(report.Errors) == 0 {
		msg += "All skill targets are already synchronized and up to date."
	}

	dialog.ShowInformation("Sync All", msg, win)
	if onDone != nil {
		onDone()
	}
}

// ShowExportZipDialog prompts the user and exports canonical skills as a ZIP file.
func ShowExportZipDialog(mgr *core.Manager, win fyne.Window) {
	defaultPath := filepath.Join(core.ExpandPath("~"), "agent-skills.zip")
	pathEntry := widget.NewEntry()
	pathEntry.SetText(defaultPath)

	form := widget.NewForm(
		widget.NewFormItem("ZIP Output Path", pathEntry),
	)

	d := dialog.NewCustomConfirm(
		"Export Skills Archive",
		"Export ZIP",
		"Cancel",
		form,
		func(ok bool) {
			if !ok {
				return
			}

			outPath := core.ExpandPath(strings.TrimSpace(pathEntry.Text))
			if outPath == "" {
				dialog.ShowError(fmt.Errorf("output path cannot be empty"), win)
				return
			}

			f, err := os.Create(outPath)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to create zip file: %w", err), win)
				return
			}
			defer f.Close()

			if err := mgr.ExportZip(nil, f); err != nil {
				dialog.ShowError(fmt.Errorf("failed to export zip: %w", err), win)
				return
			}

			dialog.ShowInformation("Export Succeeded", fmt.Sprintf("Exported all canonical skills to:\n%s", outPath), win)
		},
		win,
	)

	d.Resize(fyne.NewSize(500, 200))
	d.Show()
}
