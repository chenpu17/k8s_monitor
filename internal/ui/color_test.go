package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestSetColorMode checks that SetColorMode drives the profile the package
// styles render with: "never" (the value --no-color writes into ui.color_mode)
// strips the ANSI escapes, "always" forces them, and "auto" leaves the profile
// the terminal detected untouched.
func TestSetColorMode(t *testing.T) {
	// The colour profile is global state on lipgloss's default renderer and
	// cannot be unset, so put back what the test found.
	saved := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(saved)

	// The test process has no terminal, so lipgloss would detect monochrome
	// anyway; force profiles to make the probes meaningful.
	t.Run("never strips colour", func(t *testing.T) {
		lipgloss.SetColorProfile(termenv.TrueColor)
		if !strings.Contains(StyleStatusReady.Render("Ready"), "\x1b[") {
			t.Fatal("the probe is vacuous: the style renders without colour on a true-colour profile")
		}

		SetColorMode("never")
		if got := StyleStatusReady.Render("Ready"); strings.Contains(got, "\x1b[") {
			t.Errorf("SetColorMode(\"never\") still emitted ANSI escapes: %q", got)
		}
	})

	t.Run("always forces colour", func(t *testing.T) {
		lipgloss.SetColorProfile(termenv.Ascii)

		SetColorMode("always")
		if got := StyleStatusReady.Render("Ready"); !strings.Contains(got, "\x1b[") {
			t.Errorf("SetColorMode(\"always\") did not colour the style: %q", got)
		}
	})

	t.Run("auto leaves the profile alone", func(t *testing.T) {
		lipgloss.SetColorProfile(termenv.TrueColor)

		SetColorMode("auto")
		if got := StyleStatusReady.Render("Ready"); !strings.Contains(got, "\x1b[") {
			t.Errorf("SetColorMode(\"auto\") cleared the colour profile instead of leaving it alone: %q", got)
		}
	})
}
