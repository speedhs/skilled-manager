package ui

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/speedhs/skilled-manager/internal/version"
	"github.com/speedhs/skilled-manager/pkg/core"
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
	refreshGen    atomic.Uint64
}

// NewSkillsApp initializes the desktop application.
func NewSkillsApp(mgr *core.Manager) (*SkillsApp, error) {
	a := app.NewWithID("com.agent.skillsmanager")
	applyTheme(a, a.Preferences().StringWithFallback("theme", "system"))
	win := a.NewWindow("Skills Manager - AI Agent Skill Orchestrator")
	width := a.Preferences().FloatWithFallback("win_w", 1100)
	height := a.Preferences().FloatWithFallback("win_h", 720)
	if width < 640 {
		width = 1100
	}
	if height < 480 {
		height = 720
	}
	win.Resize(fyne.NewSize(float32(width), float32(height)))

	sa := &SkillsApp{
		fyneApp: a,
		window:  win,
		mgr:     mgr,
	}

	// Status bar at bottom
	sa.statusLabel = widget.NewLabel(fmt.Sprintf("Canonical Store: %s", mgr.CanonicalDir))

	// Skill list pane (left)
	sa.skillListPane = NewSkillListPane(func(skill core.Skill) {
		a.Preferences().SetString("last_skill", skill.Name)
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
	win.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("View",
			fyne.NewMenuItem("System theme", func() {
				a.Preferences().SetString("theme", "system")
				applyTheme(a, "system")
			}),
			fyne.NewMenuItem("Light", func() {
				a.Preferences().SetString("theme", "light")
				applyTheme(a, "light")
			}),
			fyne.NewMenuItem("Dark", func() {
				a.Preferences().SetString("theme", "dark")
				applyTheme(a, "dark")
			}),
		),
		fyne.NewMenu("Help",
			fyne.NewMenuItem("About", func() {
				dialog.ShowInformation("About Skills Manager", version.String()+"\n\nThis build is unsigned until release signing certificates are configured.\n\nSkills Manager never executes scripts inside a skill folder.", win)
			}),
		),
	))

	// Initialize File Watcher with debouncing
	watcher, err := core.NewFileWatcher(mgr, 250*time.Millisecond)
	if err == nil {
		sa.watcher = watcher
		go sa.watchLoop()
	}

	win.SetOnClosed(func() {
		size := win.Canvas().Size()
		a.Preferences().SetFloat("win_w", float64(size.Width))
		a.Preferences().SetFloat("win_h", float64(size.Height))
		if sel := sa.skillListPane.GetSelectedSkill(); sel != nil {
			a.Preferences().SetString("last_skill", sel.Name)
		}
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
// Disk work runs off the UI thread. A newer refresh supersedes an older one.
func (sa *SkillsApp) Refresh() {
	gen := sa.refreshGen.Add(1)
	sa.statusLabel.SetText("Refreshing…")
	go func() {
		err := sa.mgr.Reload()
		var skills []core.Skill
		statuses := map[string]core.CellStatus{}
		if err == nil {
			skills, err = core.ListSkills(sa.mgr.CanonicalDir)
		}
		if err == nil {
			for _, skill := range skills {
				for _, target := range sa.mgr.GetTargets() {
					st, stErr := sa.mgr.Status(context.Background(), skill.Name, target.ID)
					if stErr != nil {
						st = core.CellStatus{Code: core.StatusBroken, Message: stErr.Error()}
					}
					statuses[skill.Name+"\x00"+target.ID] = st
				}
			}
		}
		fyne.Do(func() {
			if gen != sa.refreshGen.Load() {
				return
			}
			if err != nil {
				showError(fmt.Errorf("failed to refresh: %w", err), sa.window)
				return
			}
			sa.skills = skills
			selectedName := ""
			if sel := sa.skillListPane.GetSelectedSkill(); sel != nil {
				selectedName = sel.Name
			}
			sa.skillListPane.SetSkills(skills)
			sa.matrixPane.SetStatuses(statuses)
			sa.matrixPane.SetData(skills, sa.mgr.GetTargets())
			if selectedName == "" {
				selectedName = sa.fyneApp.Preferences().String("last_skill")
			}
			var match *core.Skill
			for i := range skills {
				if skills[i].Name == selectedName {
					match = &skills[i]
					break
				}
			}
			if match == nil && selectedName == "" && len(skills) > 0 && !sa.editorPane.Dirty() {
				match = &skills[0]
			}
			if match != nil {
				sa.editorPane.LoadSkill(match)
				sa.skillListPane.SelectSkillByName(match.Name)
			} else if len(skills) == 0 {
				sa.editorPane.LoadSkill(nil)
			}
			sa.statusLabel.SetText(fmt.Sprintf("Canonical Store: %s | %d skills | Targets: %d",
				sa.mgr.CanonicalDir, len(skills), len(sa.mgr.GetTargets())))
			if sa.watcher != nil {
				sa.watcher.RefreshPaths()
			}
		})
	}()
}

func (sa *SkillsApp) watchLoop() {
	for range sa.watcher.NotifyChan() {
		// Run refresh on Fyne UI thread
		fyne.Do(func() {
			sa.Refresh()
		})
	}
}
