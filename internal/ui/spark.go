package ui

import (
	"strings"
)

// sparkChars is the sparkline ramp, low to high, preceded by a space for a zero
// value: a zero in an otherwise non-zero series reads as "nothing happened
// here" instead of as the lowest non-zero level.
const sparkChars = " ▁▂▃▄▅▆▇█"

// sparkRamp is sparkChars without the zero cell: the glyphs a positive value can
// land on. Its top index is what floor(v/max*sparkTop) scales to.
const sparkRamp = "▁▂▃▄▅▆▇█"

// sparkTop is the highest index a positive value maps to, i.e. the bucket count
// minus one.
const sparkTop = 7

// sparkline renders vals as a one-line trend, newest value on the right, using
// at most width cells: only the last width values are drawn, so the rightmost
// cell is always the most recent sample.
//
// The height of a cell is the value's share of the series' maximum
// (bucket = floor(v/max*7)), which is what makes a hover around 3% CPU visible
// instead of flat: the ramp is scaled to the session, not to 100%. An empty
// series, a non-positive maximum (idle or missing data) and a width below 1
// render nothing — an all-blank chart would claim a trend that is not there.
//
// A series whose values are all equal and positive renders a flat ▄ row: the
// line is visible, and neither "all zero" nor "no data" is implied.
func sparkline(vals []float64, width int) string {
	if width < 1 || len(vals) == 0 {
		return ""
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}

	maxVal := vals[0]
	minVal := vals[0]
	for _, v := range vals {
		if v > maxVal {
			maxVal = v
		}
		if v < minVal {
			minVal = v
		}
	}
	if maxVal <= 0 {
		return ""
	}
	if maxVal == minVal {
		return strings.Repeat("▄", len(vals))
	}

	ramp := []rune(sparkRamp)
	var b strings.Builder
	for _, v := range vals {
		if v <= 0 {
			b.WriteByte(' ')
			continue
		}
		idx := int(v / maxVal * float64(sparkTop))
		if idx < 0 {
			idx = 0
		}
		if idx > sparkTop {
			idx = sparkTop
		}
		b.WriteRune(ramp[idx])
	}
	return b.String()
}
