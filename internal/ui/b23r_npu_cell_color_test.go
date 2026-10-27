package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/muesli/termenv"
	"go.uber.org/zap"
)

// Review follow-up: the matrix cell colour must follow the grade. The batch
// glyph anchor only proves the glyph, and the colour end-to-end test only
// proves that some escape exists somewhere in the frame.
func TestNPUViewCellColorMatchesGrade(t *testing.T) {
	saved := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })
	lipgloss.SetColorProfile(termenv.TrueColor)

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 120, 40
	m.currentView = ViewNPU
	// Batch 30 made AI Core the default; this probe is about the grade matrix.
	m.npuMatrixMode = npuMatrixModeHealth
	n := &model.NodeData{Name: "npu-1", Status: "Ready", NPUCapacity: 3, NPUAllocatable: 3,
		NPUChipHealth: []model.NPUChipGrade{
			{NPUID: 0, Chip: 0, Grade: model.NPUChipGradeHealthy},
			{NPUID: 0, Chip: 1, Grade: model.NPUChipGradeDegraded},
			{NPUID: 1, Chip: 0, Grade: model.NPUChipGradeFault},
		}}
	m.clusterData = &model.ClusterData{Nodes: []*model.NodeData{n}, Summary: &model.ClusterSummary{NPUCapacity: 3, NPUAllocatable: 3}}

	out := m.renderNPUView()
	row := lineContaining(out, "npu-1")
	if row == "" {
		t.Fatal("no node row")
	}
	for _, tc := range []struct {
		glyph string
		style lipgloss.Style
	}{{"▉", StyleStatusReady}, {"▒", StyleWarning}, {"▁", StyleStatusNotReady}} {
		want := tc.style.Render(tc.glyph)
		if !strings.Contains(row, want) {
			t.Errorf("the %q cell is not rendered with its grade style; want %q in %q", tc.glyph, want, row)
		}
	}
	if StyleStatusReady.Render("x") == StyleStatusNotReady.Render("x") {
		t.Fatal("probe vacuous: profiles identical")
	}
}

// Review follow-up: when the matrix is cut to fit a narrow terminal, the
// truncation marker must be rendered so the loss of chips is not silent.
func TestNPUViewTruncationMarkerPresent(t *testing.T) {
	forcedAsciiProfile(t)
	m := npuViewModel(t, "en", 30, 40)
	out := m.renderNPUView()
	row := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(stripANSI(l), npuViewSelectionPrefix) {
			row = stripANSI(l)
		}
	}
	cells := strings.Count(row, "▉") + strings.Count(row, "▒") + strings.Count(row, "▁")
	if cells >= 16 {
		t.Fatalf("probe vacuous: all 16 chips fit at width 30 (%q)", row)
	}
	if !strings.Contains(row, "…") {
		t.Errorf("matrix cut to %d chips without the truncation marker: %q", cells, row)
	}
}

// Review follow-up: every summary segment must render in full at 38/40
// columns even when a long node name eats into the summary width.
func TestNPUViewSummarySegmentsSurviveNarrow(t *testing.T) {
	forcedAsciiProfile(t)
	for _, w := range []int{38, 40} {
		m := npuViewModel(t, "en", w, 40)
		out := stripANSI(m.renderNPUView())
		for _, seg := range m.npuSummarySegments(m.npuNodes()) {
			if !strings.Contains(out, stripANSI(seg)) {
				t.Errorf("w=%d: summary segment %q is not rendered in full:\n%s", w, stripANSI(seg), out)
			}
		}
	}
}
