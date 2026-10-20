package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// SetColorMode forces the colour profile that every style in this package
// renders with. mode follows the ui.color_mode setting:
//
//	"never"  render monochrome, whatever the terminal supports (--no-color)
//	"always" force true colour, also when the output is not a terminal
//	"auto"   leave the profile lipgloss detected for the terminal
//
// The styles are package-level values bound to lipgloss's default renderer, so
// this must be called before the UI is rendered for the first time; afterwards
// the profile is cached on the renderer. Setting a profile is one-way - lipgloss
// offers no way back to detection - so "auto" is a no-op and must be the last
// word only when nothing forced a profile earlier.
func SetColorMode(mode string) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	case "always":
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}
