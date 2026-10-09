package ui

import (
	"image/color"

	"github.com/skilled-manager/skills-manager/pkg/core"
)

var (
	ColorEnabled  = color.NRGBA{R: 34, G: 197, B: 94, A: 255}  // Green
	ColorDisabled = color.NRGBA{R: 148, G: 163, B: 184, A: 255} // Slate/Gray
	ColorConflict = color.NRGBA{R: 239, G: 68, B: 68, A: 255}  // Red
	ColorDrifted  = color.NRGBA{R: 234, G: 179, B: 8, A: 255}   // Amber/Yellow
	ColorBroken   = color.NRGBA{R: 168, G: 85, B: 247, A: 255} // Purple
)

// GetStatusColor returns the badge color for a given status code.
func GetStatusColor(code core.StatusCode) color.Color {
	switch code {
	case core.StatusEnabled:
		return ColorEnabled
	case core.StatusDrifted:
		return ColorDrifted
	case core.StatusConflict:
		return ColorConflict
	case core.StatusBroken:
		return ColorBroken
	default:
		return ColorDisabled
	}
}

// GetStatusLabel returns a human-friendly string for a status code.
func GetStatusLabel(code core.StatusCode) string {
	switch code {
	case core.StatusEnabled:
		return "Enabled"
	case core.StatusDrifted:
		return "Drifted"
	case core.StatusConflict:
		return "Conflict"
	case core.StatusBroken:
		return "Broken"
	default:
		return "Disabled"
	}
}
