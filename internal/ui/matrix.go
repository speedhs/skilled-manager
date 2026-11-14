package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/speedhs/skilled-manager/pkg/core"
)

// MatrixPane renders skills × tools in a virtualized table.
// Each cell's text is the status, so the state is not conveyed by color alone.
type MatrixPane struct {
	mgr       *core.Manager
	window    fyne.Window
	container *fyne.Container
	content   *fyne.Container
	table     *widget.Table
	skills    []core.Skill
	targets   []core.Target
	statuses  map[string]core.CellStatus
	onUpdate  func()
	busy      bool
}

// NewMatrixPane creates a new MatrixPane.
func NewMatrixPane(mgr *core.Manager, win fyne.Window, onUpdate func()) *MatrixPane {
	mp := &MatrixPane{
		mgr:      mgr,
		window:   win,
		onUpdate: onUpdate,
		statuses: map[string]core.CellStatus{},
	}
	mp.table = widget.NewTable(
		func() (int, int) {
			rows := len(mp.skills)
			if rows == 0 {
				rows = 1
			}
			return rows + 1, len(mp.targets) + 1
		},
		func() fyne.CanvasObject {
			btn := widget.NewButton(" ", nil)
			btn.Alignment = widget.ButtonAlignLeading
			return btn
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			btn := obj.(*widget.Button)
			btn.Enable()
			btn.Importance = widget.MediumImportance
			btn.OnTapped = nil
			mp.paintCell(id, btn)
		},
	)
	mp.table.SetColumnWidth(0, 200)
	mp.table.OnSelected = func(id widget.TableCellID) {
		mp.table.Unselect(id)
		if mp.busy || id.Row == 0 || id.Col == 0 {
			return
		}
		if id.Row-1 >= len(mp.skills) || id.Col-1 >= len(mp.targets) {
			return
		}
		skill := mp.skills[id.Row-1]
		target := mp.targets[id.Col-1]
		mp.onCell(skill, target, mp.statusFor(skill.Name, target.ID))
	}
	header := container.NewVBox(
		widget.NewLabelWithStyle("Tool Deployment Matrix", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Select a cell to enable or disable that skill. The label is the status."),
	)
	mp.content = container.NewStack(mp.table)
	mp.container = container.NewBorder(header, nil, nil, nil, mp.content)
	return mp
}

func matrixKey(skill, target string) string {
	return skill + "\x00" + target
}

// SetData updates the skills and targets displayed in the matrix.
func (mp *MatrixPane) SetData(skills []core.Skill, targets []core.Target) {
	mp.skills = skills
	mp.targets = targets
	for i := range targets {
		mp.table.SetColumnWidth(i+1, 150)
	}
	mp.table.Refresh()
}

// SetStatuses stores statuses computed off the UI thread.
func (mp *MatrixPane) SetStatuses(statuses map[string]core.CellStatus) {
	if statuses == nil {
		statuses = map[string]core.CellStatus{}
	}
	mp.statuses = statuses
	mp.table.Refresh()
}

func (mp *MatrixPane) statusFor(skill, target string) core.CellStatus {
	if st, ok := mp.statuses[matrixKey(skill, target)]; ok {
		return st
	}
	st, err := mp.mgr.Status(context.Background(), skill, target)
	if err != nil {
		return core.CellStatus{Code: core.StatusBroken, Message: err.Error()}
	}
	return st
}

func (mp *MatrixPane) paintCell(id widget.TableCellID, btn *widget.Button) {
	if id.Row == 0 {
		if id.Col == 0 {
			btn.SetText("Skill")
		} else if id.Col-1 < len(mp.targets) {
			target := mp.targets[id.Col-1]
			btn.SetText(target.Label)
		} else {
			btn.SetText("")
		}
		btn.Disable()
		return
	}
	if len(mp.skills) == 0 {
		if id.Col == 0 {
			btn.SetText("No skills yet")
		} else {
			btn.SetText("")
		}
		btn.Disable()
		return
	}
	skill := mp.skills[id.Row-1]
	if id.Col == 0 {
		btn.SetText(skill.Name)
		btn.Disable()
		return
	}
	if id.Col-1 >= len(mp.targets) {
		btn.SetText("")
		btn.Disable()
		return
	}
	target := mp.targets[id.Col-1]
	st := mp.statusFor(skill.Name, target.ID)
	btn.SetText(GetStatusLabel(st.Code))
	if st.Code == core.StatusConflict || mp.busy {
		btn.Disable()
	}
}

func (mp *MatrixPane) onCell(skill core.Skill, target core.Target, st core.CellStatus) {
	if st.Code == core.StatusConflict {
		showError(fmt.Errorf("%s", st.Message), mp.window)
		return
	}
	enable := st.Code == core.StatusDisabled || st.Code == core.StatusBroken
	title := "Disable " + skill.Name
	detail := fmt.Sprintf("Remove the deployment of %s on %s.\n\nPath:\n%s\n\n%s\n\nOnly a link or copy this tool recorded is removed.", skill.Name, target.Label, st.ActualPath, st.Message)
	if enable {
		title = "Enable " + skill.Name
		detail = fmt.Sprintf("Deploy %s to %s.\n\nPath:\n%s\n\n%s", skill.Name, target.Label, st.ActualPath, st.Message)
	}
	dialog.ShowConfirm(title, detail, func(ok bool) {
		if !ok {
			return
		}
		mp.applyToggle(skill.Name, target.ID, enable)
	}, mp.window)
}

func (mp *MatrixPane) applyToggle(skillName, targetID string, enable bool) {
	mp.busy = true
	mp.table.Refresh()
	go func() {
		var res *core.EnableResult
		var err error
		if enable {
			res, err = mp.mgr.Enable(context.Background(), skillName, targetID)
		} else {
			err = mp.mgr.Disable(context.Background(), skillName, targetID)
		}
		fyne.Do(func() {
			mp.busy = false
			if err != nil {
				showError(err, mp.window)
				mp.table.Refresh()
				return
			}
			if res != nil && res.Fallback {
				dialog.ShowInformation("Fallback mode", res.Message, mp.window)
			}
			if mp.onUpdate != nil {
				mp.onUpdate()
			} else {
				mp.table.Refresh()
			}
		})
	}()
}

// Container returns the Fyne canvas object for the matrix.
func (mp *MatrixPane) Container() fyne.CanvasObject {
	return mp.container
}
