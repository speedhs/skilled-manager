package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/speedhs/skilled-manager/pkg/core"
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
				showError(fmt.Errorf("skill name cannot be empty"), win)
				return
			}
			if desc == "" {
				showError(fmt.Errorf("skill description cannot be empty"), win)
				return
			}
			if body == "" {
				body = fmt.Sprintf("# %s\n\nProvide agent instructions here.\n", name)
			}

			go func() {
				skill, err := core.CreateSkill(mgr.CanonicalDir, name, desc, body)
				fyne.Do(func() {
					if err != nil {
						showError(err, win)
						return
					}
					dialog.ShowInformation("Skill Created", fmt.Sprintf("Created skill %q at %s", skill.Name, skill.Path), win)
					if onSuccess != nil {
						onSuccess(skill)
					}
				})
			}()
		},
		win,
	)

	d.Resize(fyne.NewSize(500, 400))
	d.Show()
}

// ShowImportDialog scans target directories off the UI thread, then opens the picker.
func ShowImportDialog(mgr *core.Manager, win fyne.Window, onComplete func()) {
	go func() {
		candidates, err := mgr.ScanImportCandidates(context.Background())
		fyne.Do(func() {
			presentImportDialog(mgr, win, onComplete, candidates, err)
		})
	}()
}

func presentImportDialog(mgr *core.Manager, win fyne.Window, onComplete func(), candidates []core.ImportCandidate, err error) {
	if err != nil {
		showError(fmt.Errorf("failed to scan for skills: %w", err), win)
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

			go func() {
				imported, errs := mgr.ImportSelected(context.Background(), selected)
				notes := mgr.Notes()
				fyne.Do(func() {
					msg := fmt.Sprintf("Imported %d skill(s) into canonical store:\n", len(imported))
					for _, name := range imported {
						msg += fmt.Sprintf("- %s\n", name)
					}
					for _, note := range notes {
						msg += fmt.Sprintf("\n%s\n", note)
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
				})
			}()
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
		idEntry      *widget.Entry
		labelEntry   *widget.Entry
		pathEntry    *widget.Entry
		modeSelect   *widget.Select
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
			expanded := widget.NewLabel("")
			refreshExpanded := func(path string) {
				got, err := core.ExpandPath(strings.TrimSpace(path))
				if err != nil {
					expanded.SetText("Does not expand: " + err.Error())
					return
				}
				expanded.SetText("Effective path: " + got)
			}
			refreshExpanded(t.Path)
			p.OnChanged = refreshExpanded

			mode := widget.NewSelect([]string{core.ModeSymlink, core.ModeCopy}, nil)
			mode.SetSelected(t.Mode)

			format := widget.NewSelect([]string{core.FormatSkill, core.FormatGeminiTOML}, nil)
			format.SetSelected(t.Format)

			tf := &targetForm{
				idEntry:      id,
				labelEntry:   lbl,
				pathEntry:    p,
				modeSelect:   mode,
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
					showError(fmt.Errorf("at least one target must remain configured"), win)
				}
			})
			removeBtn.Importance = widget.DangerImportance

			note := ""
			if t.Builtin {
				note = "Built-in target. Changing the path does not change the upstream default for new installs."
			}
			card := widget.NewCard(fmt.Sprintf("Target #%d: %s", idx+1, t.Label), note, container.NewVBox(
				widget.NewForm(
					widget.NewFormItem("ID", id),
					widget.NewFormItem("Label", lbl),
					widget.NewFormItem("Path", p),
					widget.NewFormItem("", expanded),
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
					showError(fmt.Errorf("target ID and Path cannot be empty"), win)
					return
				}
				candidate := core.Target{
					ID:     id,
					Label:  lbl,
					Path:   p,
					Mode:   m,
					Format: f,
				}
				if err := core.ValidateTarget(candidate); err != nil {
					showError(err, win)
					return
				}
				newTargets = append(newTargets, candidate)
			}

			go func() {
				err := mgr.UpdateTargets(newTargets)
				fyne.Do(func() {
					if err != nil {
						showError(fmt.Errorf("failed to save targets: %w", err), win)
						return
					}
					dialog.ShowInformation("Saved", "Target configurations updated successfully.", win)
					if onSaved != nil {
						onSaved()
					}
				})
			}()
		},
		win,
	)

	d.Resize(fyne.NewSize(620, 500))
	d.Show()
}

// ShowSyncReportDialog performs synchronization and displays a detailed report dialog.
func ShowSyncReportDialog(mgr *core.Manager, win fyne.Window, onDone func()) {
	dialog.ShowConfirm("Sync deployments", "Update deployments whose canonical skill changed and whose deployed bytes did not. Conflicts are left untouched.", func(ok bool) {
		if !ok {
			return
		}
		go func() {
			report, err := mgr.Sync(context.Background())
			fyne.Do(func() {
				showSyncReport(report, err, win, onDone)
			})
		}()
	}, win)
}

// ShowDoctorDialog runs doctor off the UI thread and shows every check.
func ShowDoctorDialog(mgr *core.Manager, win fyne.Window) {
	go func() {
		report, err := mgr.Doctor(context.Background())
		fyne.Do(func() {
			if err != nil {
				showError(err, win)
				return
			}
			var b strings.Builder
			for _, check := range report.Checks {
				fmt.Fprintf(&b, "%s: %s\n%s\n", check.Name, check.Status, check.Detail)
				if check.Fix != "" {
					fmt.Fprintf(&b, "Fix: %s\n", check.Fix)
				}
				b.WriteString("\n")
			}
			for _, warning := range report.Warnings {
				fmt.Fprintf(&b, "Warning: %s\n", warning)
			}
			entry := widget.NewMultiLineEntry()
			entry.SetText(b.String())
			entry.Disable()
			d := dialog.NewCustom("Doctor", "Close", container.NewScroll(entry), win)
			d.Resize(fyne.NewSize(640, 420))
			d.Show()
		})
	}()
}

func showSyncReport(report *core.SyncReport, err error, win fyne.Window, onDone func()) {
	if err != nil {
		showError(fmt.Errorf("sync failed: %w", err), win)
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
	home, err := core.ExpandPath("~")
	if err != nil {
		home = "."
	}
	defaultPath := filepath.Join(home, "agent-skills.zip")
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

			outPath, err := core.ExpandPath(strings.TrimSpace(pathEntry.Text))
			if err != nil || outPath == "" {
				showError(fmt.Errorf("output path is empty or did not expand: %w", err), win)
				return
			}
			go func() {
				f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
				if err != nil {
					fyne.Do(func() {
						showError(fmt.Errorf("failed to create zip file: %w", err), win)
					})
					return
				}
				exportErr := mgr.ExportZip(context.Background(), nil, f)
				_ = f.Close()
				if exportErr != nil {
					_ = os.Remove(outPath)
				}
				fyne.Do(func() {
					if exportErr != nil {
						showError(fmt.Errorf("failed to export zip: %w", exportErr), win)
						return
					}
					dialog.ShowInformation("Export Succeeded", fmt.Sprintf("Exported all canonical skills to:\n%s", outPath), win)
				})
			}()
		},
		win,
	)

	d.Resize(fyne.NewSize(500, 200))
	d.Show()
}
