package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showError presents err with the message visible and the full text behind a details expander.
func showError(err error, win fyne.Window) {
	if err == nil || win == nil {
		return
	}
	summary := widget.NewLabel(err.Error())
	summary.Wrapping = fyne.TextWrapWord
	details := widget.NewMultiLineEntry()
	details.SetText(fmt.Sprintf("%v", err))
	details.Disable()
	body := container.NewVBox(
		summary,
		widget.NewAccordion(widget.NewAccordionItem("Details", details)),
	)
	d := dialog.NewCustom("Could not complete that action", "Close", body, win)
	d.Resize(fyne.NewSize(520, 280))
	d.Show()
}
