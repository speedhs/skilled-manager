package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/user/skills-manager/internal/core"
)

// AppToolbar creates the main top toolbar for the application.
type AppToolbar struct {
	container *fyne.Container
}

// NewAppToolbar initializes the top toolbar.
func NewAppToolbar(
	mgr *core.Manager,
	win fyne.Window,
	onNew func(skill *core.Skill),
	onImport func(),
	onSync func(),
	onSettings func(),
	onRefresh func(),
) *AppToolbar {
	newBtn := widget.NewButtonWithIcon("New Skill", theme.ContentAddIcon(), func() {
		ShowNewSkillDialog(mgr, win, onNew)
	})
	newBtn.Importance = widget.HighImportance

	importBtn := widget.NewButtonWithIcon("Import Existing", theme.DownloadIcon(), func() {
		ShowImportDialog(mgr, win, onImport)
	})

	syncBtn := widget.NewButtonWithIcon("Sync All", theme.ViewRefreshIcon(), func() {
		ShowSyncReportDialog(mgr, win, onSync)
	})

	exportBtn := widget.NewButtonWithIcon("Export Zip", theme.StorageIcon(), func() {
		ShowExportZipDialog(mgr, win)
	})

	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		ShowSettingsDialog(mgr, win, onSettings)
	})

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.NavigateNextIcon(), func() {
		if onRefresh != nil {
			onRefresh()
		}
	})

	buttons := container.NewHBox(
		newBtn,
		importBtn,
		syncBtn,
		exportBtn,
		widget.NewSeparator(),
		settingsBtn,
		refreshBtn,
	)

	return &AppToolbar{
		container: container.NewVBox(
			buttons,
			widget.NewSeparator(),
		),
	}
}

// Container returns the Fyne canvas object for the toolbar.
func (t *AppToolbar) Container() fyne.CanvasObject {
	return t.container
}
