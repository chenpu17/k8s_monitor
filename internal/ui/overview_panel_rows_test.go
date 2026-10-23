package ui

// The overview's summary panels wrap onto as few terminal rows as the width
// allows. A wide terminal (and an unset width) keeps the historical single-row
// JoinHorizontal layout byte for byte; narrower terminals group the panels per
// row so no row overflows the screen edge.

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// summaryPanelFixture renders the five row-2 panels through the real pipeline.
func summaryPanelFixture(m *Model) []string {
	summary := &model.ClusterSummary{
		TotalNodes: 3, ReadyNodes: 2, NotReadyNodes: 1,
		TotalEvents: 5, WarningEvents: 2, ErrorEvents: 1,
	}
	return []string{
		renderSummaryPanel(m.cpuDetailsLines(summary), 6),
		renderSummaryPanel(m.memoryDetailsLines(summary), 6),
		renderSummaryPanel(m.podDetailsLines(summary), 6),
		renderSummaryPanel(m.nodesAndPodsLines(summary), 6),
		renderSummaryPanel(m.eventSummaryLines(summary), 6),
	}
}

// TestJoinPanelRowsWideKeepsSingleRow pins the wide-terminal anchor: at a
// width that fits every panel the helper must produce exactly the historical
// one-shot JoinHorizontal output, byte for byte. An unset width (0) means
// unknown and keeps that layout too.
func TestJoinPanelRowsWideKeepsSingleRow(t *testing.T) {
	forcedAsciiProfile(t)

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	panels := summaryPanelFixture(m)
	want := lipgloss.JoinHorizontal(lipgloss.Top, panels...)

	for _, width := range []int{0, 110, 120, 400} {
		m.width = width
		if got := m.joinPanelRows(panels); got != want {
			t.Errorf("joinPanelRows at width %d differs from the single-row layout", width)
		}
	}
}

// TestJoinPanelRowsNarrowGroups checks the wrapped layouts against rows built
// with the same JoinHorizontal grouping the helper is specified to use.
func TestJoinPanelRowsNarrowGroups(t *testing.T) {
	forcedAsciiProfile(t)

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	panels := summaryPanelFixture(m)
	panelWidth := lipgloss.Width(panels[0])

	t.Run("two per row at width 45", func(t *testing.T) {
		m.width = 45
		want := strings.Join([]string{
			lipgloss.JoinHorizontal(lipgloss.Top, panels[0], panels[1]),
			lipgloss.JoinHorizontal(lipgloss.Top, panels[2], panels[3]),
			lipgloss.JoinHorizontal(lipgloss.Top, panels[4]),
		}, "\n")
		if got := m.joinPanelRows(panels); got != want {
			t.Errorf("grouped layout mismatch:\ngot:\n%s\nwant:\n%s", got, want)
		}
		for _, line := range strings.Split(want, "\n") {
			if w := lipgloss.Width(line); w > 45 {
				t.Errorf("wrapped row is %d wide, exceeds the 45-column terminal", w)
			}
		}
	})

	t.Run("one per row below a single panel width", func(t *testing.T) {
		m.width = panelWidth - 1
		want := strings.Join(panels, "\n")
		if got := m.joinPanelRows(panels); got != want {
			t.Errorf("single-column layout mismatch:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("one per row at an odd in-between width", func(t *testing.T) {
		// One panel short of two per row: perRow must floor to 1. A perRow
		// derived from a too-small panel width (say w1-2) would pair two
		// panelWidth-wide panels into a 2*panelWidth-wide row and overflow
		// this terminal by one column.
		m.width = 2*panelWidth - 1
		want := strings.Join(panels, "\n")
		if got := m.joinPanelRows(panels); got != want {
			t.Errorf("width %d must lay one panel per row", m.width)
		}
		for _, line := range strings.Split(want, "\n") {
			if w := lipgloss.Width(line); w > m.width {
				t.Errorf("wrapped row is %d wide, exceeds the %d-column terminal", w, m.width)
			}
		}
	})

	t.Run("empty panels", func(t *testing.T) {
		m.width = 45
		if got := m.joinPanelRows(nil); got != "" {
			t.Errorf("joinPanelRows(nil) = %q, want empty", got)
		}
	})
}

// TestOverviewPanelRowsWrap: through overviewContentLines, the row-2 headers
// share one line on a wide terminal and split into per-width groups as the
// terminal narrows. The probes match the localized panel titles ("[⚡ CPU]",
// "🔌 Services", ...) because the cluster-load box also prints "CPU Req:" on
// its own line.
func TestOverviewPanelRowsWrap(t *testing.T) {
	forcedAsciiProfile(t)

	cluster := &model.ClusterData{
		Summary: &model.ClusterSummary{
			TotalNodes: 3, ReadyNodes: 2, NotReadyNodes: 1,
			TotalEvents: 5, WarningEvents: 2, ErrorEvents: 1,
		},
	}

	linesAt := func(width int) []string {
		m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
		m.width = width
		m.height = 200
		m.clusterData = cluster
		return m.overviewContentLines(cluster.Summary)
	}

	// headersOnOneLine reports whether some rendered line carries all the
	// given panel titles together.
	headersOnOneLine := func(lines []string, headers ...string) bool {
		for _, l := range lines {
			all := true
			for _, h := range headers {
				if !strings.Contains(l, h) {
					all = false
					break
				}
			}
			if all {
				return true
			}
		}
		return false
	}

	row2All := []string{"[⚡ CPU]", "[🧠 Memory]", "[📦 Pods]", "[💻 Nodes]", "[🔔 Alerts]"}
	servicesRow := []string{"🔌 Services", "💾 Storage", "📋 Workloads"}

	wide := linesAt(120)
	if !headersOnOneLine(wide, row2All...) {
		t.Errorf("wide terminal did not keep the row-2 panels on one line")
	}
	if !headersOnOneLine(wide, servicesRow...) {
		t.Errorf("wide terminal did not keep the services row on one line")
	}

	// Width 45: two 22-wide panels per row — CPU pairs with Memory, Services
	// with Storage, and the trailing panels stand alone.
	mid := linesAt(45)
	if !headersOnOneLine(mid, "[⚡ CPU]", "[🧠 Memory]") {
		t.Errorf("width 45 did not pair CPU with Memory")
	}
	if headersOnOneLine(mid, "[⚡ CPU]", "[📦 Pods]") {
		t.Errorf("width 45 placed CPU and Pods on the same row")
	}
	if !headersOnOneLine(mid, "🔌 Services", "💾 Storage") {
		t.Errorf("width 45 did not pair Services with Storage")
	}
	if headersOnOneLine(mid, "🔌 Services", "📋 Workloads") {
		t.Errorf("width 45 placed Workloads next to Services")
	}
	if !(len(mid) > len(wide)) {
		t.Errorf("width 45 produced %d lines, not more than the wide %d", len(mid), len(wide))
	}

	// Width 21 (below one panel): every panel stands alone.
	narrow := linesAt(21)
	for _, h := range row2All {
		if !headersOnOneLine(narrow, h) {
			t.Errorf("width 21 lost panel %q", h)
		}
	}
	if headersOnOneLine(narrow, "[⚡ CPU]", "[🧠 Memory]") {
		t.Errorf("width 21 still pairs CPU with Memory")
	}
	if !(len(narrow) > len(mid)) {
		t.Errorf("width 21 produced %d lines, not more than width 45's %d", len(narrow), len(mid))
	}
}
