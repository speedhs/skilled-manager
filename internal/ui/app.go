package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/user/skills-manager/internal/core"
)

// SkillsApp encapsulates the Fyne desktop application.
type SkillsApp struct {
	fyneApp       fyne.App
	window        fyne.Window
	mgr           *core.Manager
	watcher       *core.FileWatcher
	skillListPane *SkillListPane
	matrixPane    *MatrixPane
	editorPane    *EditorPane
	toolbar       *AppToolbar
	statusLabel   *widget.Label
	skills        []core.Skill
}

// NewSkillsApp initializes the desktop application.
func NewSkillsApp(mgr *core.Manager) (*SkillsApp, error) {
	a := app.NewWithID("com.agent.skillsmanager")
	win := a.NewWindow("Skills Manager - AI Agent Skill Orchestrator")
	win.Resize(fyne.NewSize(1100, 720))

	sa := &SkillsApp{
		fyneApp: a,
		window:  win,
		mgr:     mgr,
	}

	// Status bar at bottom
	sa.statusLabel = widget.NewLabel(fmt.Sprintf("Canonical Store: %s", mgr.CanonicalDir))

	// Skill list pane (left)
	sa.skillListPane = NewSkillListPane(func(skill core.Skill) {
		sa.editorPane.LoadSkill(&skill)
	})

	// Matrix pane (center-top)
	sa.matrixPane = NewMatrixPane(mgr, win, func() {
		sa.Refresh()
	})

	// Editor pane (center-bottom)
	sa.editorPane = NewEditorPane(win, func() {
		sa.Refresh()
	})

	// Toolbar (top)
	sa.toolbar = NewAppToolbar(
		mgr,
		win,
		func(newSkill *core.Skill) {
			sa.Refresh()
			if newSkill != nil {
				sa.skillListPane.SelectSkillByName(newSkill.Name)
				sa.editorPane.LoadSkill(newSkill)
			}
		},
		func() {
			sa.Refresh()
		},
		func() {
			sa.Refresh()
		},
		func() {
			sa.Refresh()
		},
		func() {
			sa.Refresh()
		},
	)

	// Build Split Layouts
	// Right area: VSplit with Matrix on top and Editor on bottom
	rightSplit := container.NewVSplit(
		sa.matrixPane.Container(),
		sa.editorPane.Container(),
	)
	rightSplit.SetOffset(0.48) // 48% matrix, 52% editor

	// Full area: HSplit with Left Skill List and Right area
	mainSplit := container.NewHSplit(
		sa.skillListPane.Container(),
		rightSplit,
	)
	mainSplit.SetOffset(0.24) // 24% left list, 76% right

	// Top: Toolbar, Center: mainSplit, Bottom: status bar
	rootContent := container.NewBorder(
		sa.toolbar.Container(),
		container.NewVBox(widget.NewSeparator(), sa.statusLabel),
		nil, nil,
		mainSplit,
	)

	win.SetContent(rootContent)

	// Initialize File Watcher with debouncing
	watcher, err := core.NewFileWatcher(mgr, 250*time.Millisecond)
	if err == nil {
		sa.watcher = watcher
		go sa.watchLoop()
	}

	win.SetOnClosed(func() {
		if sa.watcher != nil {
			_ = sa.watcher.Close()
		}
	})

	sa.Refresh()

	return sa, nil
}

// Run displays the window and starts the Fyne event loop.
func (sa *SkillsApp) Run() {
	sa.window.ShowAndRun()
}

// Refresh reloads state and skills from disk and updates all panes.
func (sa *SkillsApp) Refresh() {
	if err := sa.mgr.Reload(); err != nil {
		dialog.ShowError(fmt.Errorf("failed to reload targets/state: %w", err), sa.window)
		return
	}

	skills, err := core.ListSkills(sa.mgr.CanonicalDir)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to list canonical skills: %w", err), sa.window)
		return
	}
	sa.skills = skills

	// Update list
	selectedName := ""
	if sel := sa.skillListPane.GetSelectedSkill(); sel != nil {
		selectedName = sel.Name
	}

	sa.skillListPane.SetSkills(skills)
	sa.matrixPane.SetData(skills, sa.mgr.GetTargets())

	// Restore or set editor selection
	if selectedName != "" {
		sa.skillListPane.SelectSkillByName(selectedName)
	} else if len(skills) > 0 {
		sa.skillListPane.SelectSkillByName(skills[0].Name)
	} else {
		sa.editorPane.LoadSkill(nil)
	}

	sa.statusLabel.SetText(fmt.Sprintf("Canonical Store: %s | %d skills | Targets: %d",
		sa.mgr.CanonicalDir, len(skills), len(sa.mgr.GetTargets())))

	if sa.watcher != nil {
		sa.watcher.RefreshPaths()
	}
}

func (sa *SkillsApp) watchLoop() {
	for range sa.watcher.NotifyChan() {
		// Run refresh on Fyne UI thread
		fyne.Do(func() {
			sa.Refresh()
		})
	}
}
