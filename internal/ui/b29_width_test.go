package ui

// b29_width_test.go — batch 29 (overview visualisation), contract T-width.
//
// Every line this batch adds is measured against the 38-column budget the
// narrow-terminal probes of batches 22-26 established. The lesson those batches
// recorded is specific: a new wording is one or two cells wider than the one it
// replaces, and the 38-column case is where that difference lands on a truncated
// value. So each new line is checked both for width and for the values it must
// still carry at that width.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29NarrowModel is the NPU view on a two-node chip cluster at a given width,
// with a session series long enough for every sparkline to want its full width.
func b29NarrowModel(t *testing.T, locale string, width int) *Model {
	t.Helper()
	forcedAsciiProfile(t)
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width, m.height = width, 60
	m.currentView = ViewNPU
	m.clusterData = b29AggCluster()
	m.clusterData.Summary = &model.ClusterSummary{
		NPUCapacity: 16, NPUAllocatable: 16, NPUAllocated: 4, NPUUtilization: 25,
		NPUNodesCount: 2,
	}

	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		m.vizHistory = append(m.vizHistory, m.buildVizSample(m.clusterData, base.Add(time.Duration(i)*2*time.Second)))
	}
	return m
}

// TestB29NPUViewFitsThirtyEightColumns pins the hard width budget for the whole
// NPU view in every matrix mode: no rendered line exceeds the terminal, in both
// locales. Every mode — including health since batch 30 — carries a mode
// legend line, so all five are exercised.
func TestB29NPUViewFitsThirtyEightColumns(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		for _, width := range []int{38, 40, 60, 100} {
			for _, mode := range []int{npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp, npuMatrixModePower, npuMatrixModeHealth} {
				t.Run(fmt.Sprintf("%s/%d/mode%d", locale, width, mode), func(t *testing.T) {
					m := b29NarrowModel(t, locale, width)
					m.npuMatrixMode = mode

					out := stripANSI(m.renderNPUView())
					for _, line := range strings.Split(out, "\n") {
						if w := runewidth.StringWidth(line); w > width {
							t.Errorf("line is %d cells wide on a %d-cell terminal: %q", w, width, line)
						}
					}
				})
			}
		}
	}
}

// TestB29AggregateBarKeepsItsValuesAt38Columns pins the content side of the same
// budget: at 38 columns the bar may wrap onto several lines (that is the wrap
// path's job) but it must still print every number in full. A bar that fits by
// truncating a gauge is the failure mode batches 25/26 recorded.
func TestB29AggregateBarKeepsItsValuesAt38Columns(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := b29NarrowModel(t, locale, 38)
		out := stripANSI(m.renderNPUView())

		// The numbers, not the labels: labels are localized, values are not.
		for _, want := range []string{"1900", "44", "32/1024", "20%"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the 38-column view lost the value %q:\n%s", locale, want, out)
			}
		}
		// And the summary bar's own high-severity counts are untouched by the
		// lines this batch added (batch 25's contract, re-checked because the
		// view grew two lines above the matrix).
		for _, want := range []string{"16/16", "0"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the 38-column view lost %q:\n%s", locale, want, out)
			}
		}
	}
}

// TestB29SparklinesYieldBeforeValues pins the yield order the overview follows:
// the sparkline is the last thing on the line and the first thing to go. At a
// width where it does not fit, the horizontal-budget helper returns the line
// untouched rather than shortening the series.
func TestB29SparklinesYieldBeforeValues(t *testing.T) {
	series := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	// A line that leaves no room: the sparkline is not appended.
	base := strings.Repeat("x", 90)
	if got, added := appendSpark(base, series, overviewSparkWidth, clusterLoadContent); added || got != base {
		t.Errorf("the sparkline was appended past the budget: %q (added=%v)", got, added)
	}

	// A line that leaves exactly room: it is appended whole, ten cells.
	base = strings.Repeat("x", clusterLoadContent-overviewSparkWidth-1)
	got, added := appendSpark(base, series, overviewSparkWidth, clusterLoadContent)
	if !added {
		t.Fatalf("the sparkline was not appended although it fits: %q", got)
	}
	if w, want := visualLength(got), clusterLoadContent; w != want {
		t.Errorf("the line is %d cells wide, want exactly the budget %d", w, want)
	}
	if strings.Count(got, "█") != 1 {
		t.Errorf("the appended sparkline is not the ten-cell series: %q", got)
	}

	// An empty series appends nothing and reports it.
	if got, added := appendSpark("line", nil, overviewSparkWidth, clusterLoadContent); added || got != "line" {
		t.Errorf("an empty series changed the line: %q (added=%v)", got, added)
	}
}

// TestB29OverviewLoadBoxSparkFits pins the overview side: the CPU/Memory lines
// with their sparkline stay inside the cluster-load box's content width, so the
// box does not grow a second line and shift every panel below it.
func TestB29OverviewLoadBoxSparkFits(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := b29CardModel(t, locale, b29CardCluster(), 12)
		m.clusterData.Summary = &model.ClusterSummary{
			TotalNodes: 2, ReadyNodes: 2,
			CPUCapacity: 319130, CPUAllocatable: 319130, CPUUsed: 1600, CPUUsageUtilization: 0.5,
			MemoryCapacity: 2 << 40, MemoryAllocatable: 2 << 40, MemoryUsed: 4 << 30, MemUsageUtilization: 0.2,
			NetworkRxBytes: 100, NetworkTxBytes: 100,
			KubeletMetricsAvailable: true, NodesWithMetrics: 2,
		}
		// Re-record the series against this summary so the CPU/Mem series exist.
		m.vizHistory = nil
		base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
		for i := 0; i < 12; i++ {
			m.vizHistory = append(m.vizHistory, m.buildVizSample(m.clusterData, base.Add(time.Duration(i)*2*time.Second)))
		}

		box := stripANSI(m.renderClusterLoadCompact(m.clusterData.Summary))
		lines := strings.Split(box, "\n")
		// The box is 9 lines: border, padding, title, padding, two load rows,
		// the network row, padding, border. A tenth means a load row wrapped.
		if len(lines) > 9 {
			t.Errorf("%s: the cluster-load box grew to %d lines, a load row wrapped:\n%s", locale, len(lines), box)
		}
		for _, line := range lines {
			if w := runewidth.StringWidth(line); w > clusterLoadBoxWidth+2 {
				t.Errorf("%s: a box line is %d cells wide: %q", locale, w, line)
			}
		}
		// Non-vacuousness: the sparkline really is on screen, one per load row.
		if strings.Count(box, "█")+strings.Count(box, "▅")+strings.Count(box, "▄") == 0 {
			t.Fatalf("%s: no sparkline rendered in the load box:\n%s", locale, box)
		}
	}
}
