package ui

// b29_card_test.go — batch 29 (overview visualisation), contract T-card.
//
// The overview NPU card gains chip-level lines. Two rules are pinned here, both
// learned the hard way in this repo: the assertions are on whole lines (a
// superset Contains probe passes on a card that renders the right words in the
// wrong place), and a cluster without chip data must render the card it
// rendered before, byte for byte.

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29CardModel builds an overview model over a cluster.
func b29CardModel(t *testing.T, locale string, data *model.ClusterData, seriesLen int) *Model {
	t.Helper()
	forcedAsciiProfile(t)
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width, m.height = 200, 80
	m.currentView = ViewOverview
	m.clusterData = data

	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 0; i < seriesLen; i++ {
		m.vizHistory = append(m.vizHistory, m.buildVizSample(data, base.Add(time.Duration(i)*2*time.Second)))
	}
	return m
}

// b29CardCluster is b29AggCluster with the summary fields the card reads.
func b29CardCluster() *model.ClusterData {
	data := b29AggCluster()
	data.Summary = &model.ClusterSummary{
		NPUCapacity: 16, NPUAllocatable: 16, NPUAllocated: 4, NPUUtilization: 25,
		NPUNodesCount: 2, NPUChipType: "Ascend910B",
	}
	return data
}

// b29CardLines returns the NPU card's content lines, unstyled.
func b29CardLines(m *Model) []string {
	lines := m.npuPanelLines(m.clusterData.Summary)
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = stripANSI(line)
	}
	return out
}

// TestB29OverviewCardChipLines pins the exact lines the card adds and, just as
// importantly, their order: the chip rows sit directly under the allocation bar
// and above the type/topology block, so a reader scanning the card finds them
// where the resource rows end.
func TestB29OverviewCardChipLines(t *testing.T) {
	m := b29CardModel(t, "en", b29CardCluster(), 6)
	lines := b29CardLines(m)

	// The pre-batch head of the card, unchanged.
	wantHead := []string{" 🧮 NPU ", "", "Total:   16", "Alloc:   4", "Usage:   25.0%", "  ███░░░░░░░░░░░"}
	if len(lines) < len(wantHead) {
		t.Fatalf("the card has %d lines, want at least %d: %q", len(lines), len(wantHead), lines)
	}
	for i, want := range wantHead {
		if lines[i] != want {
			t.Errorf("line %d = %q, want the unchanged %q", i, lines[i], want)
		}
	}

	// The chip rows, exact.
	// 16 of 16 chips healthy → "Chips:16/16"; utilisation mean is 20%.
	if got, want := lines[6], "Chips:16/16"; got != want {
		t.Errorf("chip health line = %q, want %q", got, want)
	}
	utilLine := lines[7]
	if !strings.HasPrefix(utilLine, "Util: ") {
		t.Errorf("chip utilisation line = %q, want it to start with %q", utilLine, "Util: ")
	}
	if !strings.HasSuffix(utilLine, " 20%") {
		t.Errorf("chip utilisation line = %q, want it to end with the mean %q", utilLine, " 20%")
	}
	if got, want := lines[8], "44°C · 1.9kW"; got != want {
		t.Errorf("temperature/power line = %q, want %q", got, want)
	}

	// The type block still follows, after the chip rows.
	chipTypeIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "Chip: Ascend910B") {
			chipTypeIdx = i
		}
	}
	if chipTypeIdx < 0 {
		t.Fatalf("the chip-type line is gone: %q", lines)
	}
	if chipTypeIdx <= 8 {
		t.Errorf("the chip-type line moved above the chip rows (index %d)", chipTypeIdx)
	}
}

// TestB29OverviewCardNoChipDataUnchanged is the zero-drift anchor for the card:
// an NPU cluster with no chip data must render exactly what it rendered before
// the batch, and the chip lines must not appear as zeros.
func TestB29OverviewCardNoChipDataUnchanged(t *testing.T) {
	data := &model.ClusterData{
		Nodes: []*model.NodeData{
			{Name: "node-a", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8},
			{Name: "node-b", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8},
		},
		Summary: &model.ClusterSummary{
			NPUCapacity: 16, NPUAllocatable: 16, NPUAllocated: 4, NPUUtilization: 25,
			NPUNodesCount: 2, NPUChipType: "Ascend910B", SuperPodCount: 1,
		},
	}
	m := b29CardModel(t, "en", data, 6)
	got := b29CardLines(m)

	want := []string{
		" 🧮 NPU ",
		"",
		"Total:   16",
		"Alloc:   4",
		"Usage:   25.0%",
		"  ███░░░░░░░░░░░",
		"",
		"Chip: Ascend910B",
		"Pods: 2 nodes",
		"",
		"Deploy collector",
		"for chip metrics",
	}
	if len(got) != len(want) {
		t.Fatalf("the no-chip-data card has %d lines, want the pre-batch %d:\n got %q\nwant %q",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want the pre-batch %q", i, got[i], want[i])
		}
	}
}

// TestB29OverviewCardNonHealthyChips pins the degraded/fault rows: they appear
// only when there is something to report, they carry the right count, and the
// healthy total in the line above counts every chip.
func TestB29OverviewCardNonHealthyChips(t *testing.T) {
	data := b29CardCluster()
	data.Nodes[0].NPUChipHealth[3].Grade = model.NPUChipGradeDegraded
	data.Nodes[0].NPUChipHealth[7].Grade = model.NPUChipGradeFault
	data.Nodes[1].NPUChipHealth[3].Grade = model.NPUChipGradeDegraded

	m := b29CardModel(t, "en", data, 6)
	lines := b29CardLines(m)

	// Locate the chip block by its first line and assert the block exactly.
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "Chips:") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("the chip block is missing: %q", lines)
	}
	want := []string{"Chips:13/16", "2 degraded", "1 fault", "Util: █░░░░░ 20%", "44°C · 1.9kW"}
	if len(lines)-start < len(want) {
		t.Fatalf("the chip block has %d lines, want %d: %q", len(lines)-start, len(want), lines[start:])
	}
	for i, wantLine := range want {
		if got := lines[start+i]; got != wantLine {
			t.Errorf("chip block line %d = %q, want %q", i, got, wantLine)
		}
	}

	// A cluster with no non-healthy chip has neither row.
	clean := b29CardModel(t, "en", b29CardCluster(), 6)
	joined := strings.Join(b29CardLines(clean), "\n")
	for _, absent := range []string{" degraded", " fault"} {
		if strings.Contains(joined, absent) {
			t.Errorf("a fully healthy cluster rendered %q:\n%s", absent, joined)
		}
	}
}

// TestB29OverviewCardZhSwitches pins the zh render of the new lines and that no
// key name leaks. The numeric tail is locale-invariant; the labels are not.
func TestB29OverviewCardZhSwitches(t *testing.T) {
	zh := b29CardModel(t, "zh", b29CardCluster(), 6)
	en := b29CardModel(t, "en", b29CardCluster(), 6)

	zhLines := b29CardLines(zh)
	enLines := b29CardLines(en)

	found := false
	for _, line := range zhLines {
		if strings.HasPrefix(line, "芯片：16/16") {
			found = true
		}
	}
	if !found {
		t.Errorf("the zh card has no %q line: %q", "芯片：16/16", zhLines)
	}

	joined := strings.Join(zhLines, "\n")
	for _, bad := range []string{"Chips:", "Util:", "overview.", "{{", "}}"} {
		if strings.Contains(joined, bad) {
			t.Errorf("the zh card carries %q:\n%s", bad, joined)
		}
	}
	// Non-vacuousness: en and zh really differ on the labelled lines.
	diff := 0
	for i := range zhLines {
		if i < len(enLines) && zhLines[i] != enLines[i] {
			diff++
		}
	}
	if diff < 2 {
		t.Errorf("only %d zh lines differ from en; the locale did not switch: zh %q en %q", diff, zhLines, enLines)
	}
}

// TestB29OverviewCardFitsPanelWidth pins the width budget: every line the card
// produces fits the panel's content width, so nothing is clipped off the right
// edge on a narrow terminal. This is the "measure before you print" rule the
// card's own comment states.
func TestB29OverviewCardFitsPanelWidth(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	for _, locale := range []string{"en", "zh"} {
		for _, data := range []*model.ClusterData{
			b29CardCluster(),
			func() *model.ClusterData { // the widest values the fixtures allow
				d := b29CardCluster()
				for _, node := range d.Nodes {
					for i := range node.NPUChips {
						node.NPUChips[i].Power = 999.9
						node.NPUChips[i].Temp = 108
					}
				}
				return d
			}(),
		} {
			m := b29CardModel(t, locale, data, 6)
			for i, line := range m.npuPanelLines(data.Summary) {
				if w := visualLength(line); w > summaryPanelContent {
					t.Errorf("%s: card line %d is %d cells wide, the panel content is %d: %q",
						locale, i, w, summaryPanelContent, stripANSI(line))
				}
			}
			// And the rendered panel is still exactly one panel wide, so the row
			// of panels keeps its layout.
			panel := renderSummaryPanel(m.npuPanelLines(data.Summary), summaryPanelMinContentLine)
			if w := lipgloss.Width(panel); w != summaryPanelWidth+2 {
				t.Errorf("%s: the rendered NPU panel is %d cells wide, want %d", locale, w, summaryPanelWidth+2)
			}
		}
	}
}
