package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/skilled-manager/skills-manager/pkg/core"
)

// MatrixPane renders the matrix of skills x tools.
type MatrixPane struct {
	mgr       *core.Manager
	window    fyne.Window
	container *fyne.Container
	content   *fyne.Container
	skills    []core.Skill
	targets   []core.Target
	onUpdate  func()
}

// NewMatrixPane creates a new MatrixPane.
func NewMatrixPane(mgr *core.Manager, win fyne.Window, onUpdate func()) *MatrixPane {
	mp := &MatrixPane{
		mgr:      mgr,
		window:   win,
		onUpdate: onUpdate,
	}

	mp.content = container.NewVBox()
	scroll := container.NewScroll(mp.content)

	header := container.NewVBox(
		widget.NewLabelWithStyle("Tool Deployment Matrix", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Click a cell checkbox to toggle skill enablement. Hover or click ℹ for status details."),
	)

	mp.container = container.NewBorder(header, nil, nil, nil, scroll)
	return mp
}

// SetData updates the skills and targets displayed in the matrix.
func (mp *MatrixPane) SetData(skills []core.Skill, targets []core.Target) {
	mp.skills = skills
	mp.targets = targets
	mp.Render()
}

// Render rebuilds the matrix view.
func (mp *MatrixPane) Render() {
	mp.content.Objects = nil

	if len(mp.skills) == 0 {
		emptyCard := widget.NewCard("", "", container.NewCenter(
			widget.NewLabel("No skills in canonical store. Click 'New Skill' or 'Import Existing' to begin."),
		))
		mp.content.Add(emptyCard)
		mp.content.Refresh()
		return
	}

	// Build Header Row
	headerRow := container.NewHBox()
	skillColHeader := widget.NewLabelWithStyle("Skill", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	skillColContainer := container.NewHBox(skillColHeader)
	// Reserve fixed width for skill column
	skillColContainer.Resize(fyne.NewSize(200, 36))
	headerRow.Add(container.NewGridWrap(fyne.NewSize(200, 36), skillColHeader))

	for _, target := range mp.targets {
		modeStr := target.Mode
		if target.Format == core.FormatGeminiTOML {
			modeStr = "gemini-toml"
		}
		lbl := fmt.Sprintf("%s\n(%s)", target.Label, modeStr)
		targetHeader := widget.NewLabelWithStyle(lbl, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		headerRow.Add(container.NewGridWrap(fyne.NewSize(160, 48), targetHeader))
	}

	mp.content.Add(headerRow)
	mp.content.Add(widget.NewSeparator())

	// Build Matrix Rows
	for _, skill := range mp.skills {
		rowContainer := container.NewHBox()

		// Skill label
		skillLabel := widget.NewLabelWithStyle(skill.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		rowContainer.Add(container.NewGridWrap(fyne.NewSize(200, 42), skillLabel))

		// Target cells
		for _, target := range mp.targets {
			cell := mp.buildCell(skill, target)
			rowContainer.Add(container.NewGridWrap(fyne.NewSize(160, 42), cell))
		}

		mp.content.Add(rowContainer)
		mp.content.Add(widget.NewSeparator())
	}

	mp.content.Refresh()
}

// buildCell creates the interactive widget for a (skill, target) cell.
func (mp *MatrixPane) buildCell(skill core.Skill, target core.Target) fyne.CanvasObject {
	status, err := mp.mgr.Status(skill.Name, target.ID)
	if err != nil {
		status = core.CellStatus{
			Code:    core.StatusBroken,
			Message: fmt.Sprintf("Error checking status: %v", err),
		}
	}

	// Status indicator dot
	dot := canvas.NewCircle(GetStatusColor(status.Code))
	dot.Resize(fyne.NewSize(10, 10))
	dotContainer := container.NewGridWrap(fyne.NewSize(12, 12), dot)

	// Checkbox
	isChecked := status.Code == core.StatusEnabled || status.Code == core.StatusDrifted
	check := widget.NewCheck("", nil)
	check.Checked = isChecked

	// Disable checkbox if conflict to prevent accidental toggle
	if status.Code == core.StatusConflict {
		check.Disable()
	}

	check.OnChanged = func(wantEnabled bool) {
		if wantEnabled {
			res, err := mp.mgr.Enable(skill.Name, target.ID)
			if err != nil {
				dialog.ShowError(err, mp.window)
				check.SetChecked(false)
				return
			}
			if res.Fallback {
				dialog.ShowInformation("Fallback Mode", res.Message, mp.window)
			}
		} else {
			err := mp.mgr.Disable(skill.Name, target.ID)
			if err != nil {
				dialog.ShowError(err, mp.window)
				check.SetChecked(true)
				return
			}
		}
		if mp.onUpdate != nil {
			mp.onUpdate()
		}
	}

	// Status text
	statusLbl := widget.NewLabel(GetStatusLabel(status.Code))

	// Info button with tooltip/dialog
	infoBtn := widget.NewButton("Info", func() {
		title := fmt.Sprintf("%s on %s", skill.Name, target.Label)
		msg := fmt.Sprintf("Status: %s\nTarget Path: %s\n\nDetails:\n%s",
			GetStatusLabel(status.Code),
			status.ActualPath,
			status.Message,
		)
		if status.IsSymlink && status.LinkTarget != "" {
			msg += fmt.Sprintf("\n\nSymlink Target: %s", status.LinkTarget)
		}
		dialog.ShowInformation(title, msg, mp.window)
	})
	infoBtn.Importance = widget.LowImportance

	return container.NewHBox(
		container.NewCenter(dotContainer),
		check,
		statusLbl,
		infoBtn,
	)
}

// Container returns the Fyne canvas object for the matrix.
func (mp *MatrixPane) Container() fyne.CanvasObject {
	return mp.container
}
