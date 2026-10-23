package ui

// The help footer is one " • "-joined line; on terminals narrower than the
// joined bindings it used to wrap and push the layout. It is now clipped to
// the terminal width (truncate semantics: ANSI stripped, "..." suffix). A
// footer that fits — and an unset width (0), which means unknown — must stay
// byte for byte.

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestFooterWidth pins the footer clipping: wide and unset widths keep the
// historical joined line, a narrow terminal clips it to fit.
func TestFooterWidth(t *testing.T) {
	forcedAsciiProfile(t)

	// The overview footer's bindings, frozen as the golden inner text.
	const wantFooter = "q quit • r refresh • 1-8 views • tab next"

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	t.Run("wide terminal keeps the joined line", func(t *testing.T) {
		m.width = 200
		if got := stripANSI(m.renderFooter()); got != wantFooter {
			t.Errorf("wide footer = %q, want %q", got, wantFooter)
		}
	})

	t.Run("unset width does not clip", func(t *testing.T) {
		m.width = 0
		got := stripANSI(m.renderFooter())
		if got != wantFooter {
			t.Errorf("unset-width footer = %q, want %q", got, wantFooter)
		}
	})

	t.Run("narrow terminal clips to the width", func(t *testing.T) {
		const width = 30
		m.width = width
		got := stripANSI(m.renderFooter())
		if w := visualLength(got); w > width {
			t.Errorf("footer is %d cells wide, exceeds the %d-column terminal: %q", w, width, got)
		}
		if want := truncate(wantFooter, width); got != want {
			t.Errorf("clipped footer = %q, want %q", got, want)
		}
	})

	t.Run("list view footer clips on a narrow terminal", func(t *testing.T) {
		m.width = 60
		m.currentView = ViewPods
		got := stripANSI(m.renderFooter())
		if w := visualLength(got); w > 60 {
			t.Errorf("list-view footer is %d cells wide, exceeds 60: %q", w, got)
		}
	})
}
