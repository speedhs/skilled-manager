package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type fixedVariantTheme struct {
	variant fyne.ThemeVariant
}

func (t fixedVariantTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(name, t.variant)
}

func (t fixedVariantTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t fixedVariantTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t fixedVariantTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}

func applyTheme(a fyne.App, name string) {
	switch name {
	case "light":
		a.Settings().SetTheme(fixedVariantTheme{variant: theme.VariantLight})
	case "dark":
		a.Settings().SetTheme(fixedVariantTheme{variant: theme.VariantDark})
	default:
		a.Settings().SetTheme(theme.DefaultTheme())
	}
}
