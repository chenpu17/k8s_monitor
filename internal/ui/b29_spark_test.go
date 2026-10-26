package ui

// b29_spark_test.go — batch 29 (overview visualisation), contract T-spark.
//
// sparkline is the pure function behind three of the batch's four surfaces:
// the CPU/Memory trend beside the cluster-load bars, the NPU aggregate gauges
// and the overview NPU card. Its properties are pinned here rather than through
// a renderer, because every one of them is a rule about the series and not
// about a view.

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestSparklineEmptyInputs pins that nothing is drawn when there is nothing to
// draw: no samples, a width below one cell, or a series whose maximum is zero.
// The last one is the honest-data rule — a row of spaces would claim "no
// activity", which is not what "no data" means.
func TestSparklineEmptyInputs(t *testing.T) {
	cases := []struct {
		name  string
		vals  []float64
		width int
	}{
		{"no values", nil, 10},
		{"empty slice", []float64{}, 10},
		{"zero width", []float64{1, 2, 3}, 0},
		{"negative width", []float64{1, 2, 3}, -3},
		{"all zero (no data)", []float64{0, 0, 0}, 10},
		{"negative values only", []float64{-1, -2}, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sparkline(tc.vals, tc.width); got != "" {
				t.Errorf("sparkline(%v, %d) = %q, want the empty string", tc.vals, tc.width, got)
			}
		})
	}
}

// TestSparklineKeepsNewestOnTheRight pins the reading direction: with a short
// series the cells are laid out left to right in time order, and with a series
// longer than the width only its newest values survive.
func TestSparklineKeepsNewestOnTheRight(t *testing.T) {
	// A rising series ends with its maximum, so the tallest glyph must be last.
	rising := []float64{1, 2, 3, 4, 5}
	got := sparkline(rising, 5)
	if runes := []rune(got); len(runes) != 5 || string(runes[4]) != "█" {
		t.Errorf("sparkline(%v, 5) = %q, want the tallest cell last", rising, got)
	}

	// Width-bounded: the newest value (the global maximum) must still be there,
	// and the cells dropped are the oldest ones.
	long := []float64{1, 1, 1, 1, 1, 1, 1, 9}
	got = sparkline(long, 3)
	if !strings.HasSuffix(got, "█") {
		t.Errorf("sparkline(%v, 3) = %q, want it to end with the newest value", long, got)
	}
	if w := runewidth.StringWidth(got); w != 3 {
		t.Errorf("sparkline(%v, 3) is %d cells wide, want 3", long, w)
	}

	// A short series is not padded to the width: the cells are one per sample.
	if got, want := sparkline([]float64{1, 2}, 10), 2; runewidth.StringWidth(got) != want {
		t.Errorf("sparkline of 2 samples at width 10 = %q (%d cells), want %d cells", got, runewidth.StringWidth(got), want)
	}
}

// TestSparklineFlatSeries pins the all-equal case: a flat but non-zero series
// draws a visible flat line at mid-height, and an all-zero series draws nothing
// (that is TestSparklineEmptyInputs). "Flat" must not be confused with "empty".
func TestSparklineFlatSeries(t *testing.T) {
	for _, vals := range [][]float64{{5, 5, 5}, {0.5, 0.5}, {42, 42, 42, 42}} {
		got := sparkline(vals, len(vals))
		if got == "" {
			t.Errorf("sparkline(%v) drew nothing; a flat non-zero series must be visible", vals)
			continue
		}
		if strings.Trim(got, "▄") != "" {
			t.Errorf("sparkline(%v) = %q, want the mid-height flat glyph", vals, got)
		}
		if w, want := runewidth.StringWidth(got), len(vals); w != want {
			t.Errorf("sparkline(%v) is %d cells wide, want %d", vals, w, want)
		}
	}
}

// TestSparklineBucketBoundaries pins the bucket mapping at its edges: the
// maximum always maps to the top glyph, a value at half the maximum maps to the
// middle glyph, and the ramp is monotone in the value.
func TestSparklineBucketBoundaries(t *testing.T) {
	ramp := []rune(sparkRamp)

	// Minimum -> the lowest glyph; maximum -> the highest.
	got := []rune(sparkline([]float64{0, 8}, 2))
	if len(got) != 2 {
		t.Fatalf("sparkline([0 8], 2) = %q", string(got))
	}
	if got[0] != ' ' {
		t.Errorf("a zero value maps to %q, want the blank cell", string(got[0]))
	}
	if got[1] != ramp[len(ramp)-1] {
		t.Errorf("the series maximum maps to %q, want %q", string(got[1]), string(ramp[len(ramp)-1]))
	}

	// Half the maximum -> the middle of the ramp (floor(0.5*7) = 3).
	if mid := []rune(sparkline([]float64{0, 4, 8}, 3))[1]; mid != ramp[3] {
		t.Errorf("half the maximum maps to %q, want %q", string(mid), string(ramp[3]))
	}

	// Monotone: a higher value never draws a lower glyph. The sweep holds the
	// series maximum fixed so the swept value is the one that moves; this is the
	// property the readers rely on rather than the exact bucket of every value.
	prev := -1
	for v := 1.0; v <= 8; v++ {
		r := []rune(sparkline([]float64{0, v, 8}, 3))[1]
		idx := strings.IndexRune(sparkRamp, r)
		if idx < prev {
			t.Errorf("value %v draws %q (index %d), lower than the previous index %d", v, string(r), idx, prev)
		}
		prev = idx
	}
	// Non-vacuousness: the sweep above spans several distinct buckets.
	first := strings.IndexRune(sparkRamp, []rune(sparkline([]float64{0, 1, 8}, 3))[1])
	last := strings.IndexRune(sparkRamp, []rune(sparkline([]float64{0, 8, 8}, 3))[1])
	if last-first < 1 {
		t.Errorf("the ramp does not move across the sweep (first %d, last %d)", first, last)
	}
}

// TestSparklineIsScaledToTheSeries pins the normalisation choice: the ramp spans
// the series' own maximum, not a fixed 0-100. A cluster hovering between 0.5%
// and 1.5% CPU must show a shape, or the sparkline would be a flat line on every
// real cluster.
func TestSparklineIsScaledToTheSeries(t *testing.T) {
	low := sparkline([]float64{0.5, 1.5}, 2)
	high := sparkline([]float64{50, 150}, 2)
	if low != high {
		t.Errorf("the same shape at a different scale renders differently: %q vs %q", low, high)
	}
	// Non-vacuousness: the shape is a rise, not a blank row.
	if len([]rune(low)) != 2 || strings.TrimSpace(low) == "" || low[0] == low[1] {
		t.Errorf("a 3x rise rendered as %q, want two distinct non-blank cells", low)
	}
}
