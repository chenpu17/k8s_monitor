package ui

// b29v_state_test.go — independent adversarial probe for batch 29's `h` key and
// matrix-mode state. The implementer's cycle test covers the happy path; this
// file covers every keyboard-owning mode, the typed-character path, persistence
// across views, and the no-NPU / no-data guards.

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29vStateModel is the NPU view over one eight-chip node.
func b29vStateModel() *Model {
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{b29vHealthyNode("npu-a", 8)},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8, NPUNodesCount: 1},
	}
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 120, 60
	m.currentView = ViewNPU
	m.clusterData = data
	return m
}

// TestB29VHeatKeyBlockedInEveryOwningMode walks every mode viewSwitchBlocked
// knows about plus the logs search overlay, and requires `h` to leave the
// matrix mode untouched.
func TestB29VHeatKeyBlockedInEveryOwningMode(t *testing.T) {
	blocked := map[string]func(*Model){
		"detail":        func(m *Model) { m.detailMode = true },
		"filter":        func(m *Model) { m.filterMode = true },
		"logs":          func(m *Model) { m.logsMode = true },
		"commandOutput": func(m *Model) { m.commandOutputMode = true },
		"actionMenu":    func(m *Model) { m.actionMenuMode = true },
	}
	for name, block := range blocked {
		m := b29vStateModel()
		m.npuMatrixMode = npuMatrixModeTemp // start somewhere non-zero
		block(m)
		m.Update(keyMsg("h"))
		if m.npuMatrixMode != npuMatrixModeTemp {
			t.Errorf("%s: 'h' cycled the matrix to %d while the mode owns the keyboard",
				name, m.npuMatrixMode)
		}
	}

	// The logs search overlay types the character instead.
	m := b29vStateModel()
	m.logsMode = true
	m.logsSearchMode = true
	m.Update(keyMsg("h"))
	if m.npuMatrixMode != npuMatrixModeAICore {
		t.Errorf("logs search: 'h' cycled the matrix to %d", m.npuMatrixMode)
	}
	if !strings.Contains(m.logsSearchText, "h") {
		t.Errorf("logs search: 'h' was not typed into the filter text: %q", m.logsSearchText)
	}
}

// TestB29VHeatModePersistsAcrossViewSwitch pins the state lifetime: the chosen
// mode is a model field, so leaving the NPU view and coming back must not reset
// it (and the renderer must still be in that mode).
func TestB29VHeatModePersistsAcrossViewSwitch(t *testing.T) {
	m := b29vStateModel()
	m.Update(keyMsg("h"))
	m.Update(keyMsg("h"))
	if m.npuMatrixMode != npuMatrixModeTemp {
		t.Fatalf("two presses reached mode %d, want temp", m.npuMatrixMode)
	}
	m.currentView = ViewOverview
	m.Update(keyMsg("h")) // ignored off the NPU view
	if m.npuMatrixMode != npuMatrixModeTemp {
		t.Fatalf("'h' on another view changed the mode to %d", m.npuMatrixMode)
	}
	m.currentView = ViewNPU
	if m.npuMatrixMode != npuMatrixModeTemp {
		t.Errorf("the mode was reset by the view round-trip: %d", m.npuMatrixMode)
	}
	// And the renderer reflects it: the heat legend is on screen.
	if !strings.Contains(stripANSI(m.renderNPUView()), stripANSI(m.npuHeatLegend())) {
		t.Errorf("the heat legend is missing after the view round-trip")
	}
}

// TestB29VHeatKeyNoOpWithoutNPUNodes pins both guards: a CPU-only cluster and a
// model without data must ignore `h` (no panic, no mode change).
func TestB29VHeatKeyNoOpWithoutNPUNodes(t *testing.T) {
	cases := map[string]*model.ClusterData{
		"cpu-only": {
			Nodes:   []*model.NodeData{{Name: "cpu-a", Status: "Ready", CPUAllocatable: 8000}},
			Summary: &model.ClusterSummary{TotalNodes: 1, ReadyNodes: 1},
		},
		"nil-data": nil,
	}
	for name, data := range cases {
		m := b29vStateModel()
		m.clusterData = data
		m.Update(keyMsg("h"))
		if m.npuMatrixMode != npuMatrixModeAICore {
			t.Errorf("%s: 'h' cycled the matrix to %d without NPU nodes", name, m.npuMatrixMode)
		}
	}
}

// TestB29VHeatCycleReturnsToTheStartingBytes pins the loop end to end: after a
// full cycle the view must be byte-identical to the starting (AI Core) view
// again, with the same legend and the same heat glyphs.
func TestB29VHeatCycleReturnsToTheStartingBytes(t *testing.T) {
	forcedAsciiProfile(t)
	m := b29vStateModel()
	before := m.renderNPUView()
	for i := 0; i < npuMatrixModeCount; i++ {
		m.Update(keyMsg("h"))
	}
	if m.npuMatrixMode != npuMatrixModeAICore {
		t.Fatalf("a full cycle ended at mode %d, want the AI Core default", m.npuMatrixMode)
	}
	if after := m.renderNPUView(); after != before {
		t.Errorf("the view did not return to the starting bytes after a full cycle:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestB29VHeatModeRendersWithoutSummary pins the nil-summary path the aggregate
// bar and the heat matrix share: the NPU view must render (no panic, no lost
// legend) from node-level data alone, which is the first-refresh shape.
func TestB29VHeatModeRendersWithoutSummary(t *testing.T) {
	forcedAsciiProfile(t)
	m := b29vStateModel()
	m.clusterData = &model.ClusterData{Nodes: []*model.NodeData{b29vHealthyNode("npu-x", 4)}}
	m.npuMatrixMode = npuMatrixModeAICore

	out := stripANSI(m.renderNPUView())
	if strings.Contains(out, "panic") || strings.Contains(out, "%!") {
		t.Fatalf("the nil-summary render is malformed:\n%s", out)
	}
	if !strings.Contains(out, stripANSI(m.npuHeatLegend())) {
		t.Errorf("the heat legend is missing in the nil-summary render:\n%s", out)
	}
	if !strings.Contains(out, "npu-x") {
		t.Errorf("the node row is missing in the nil-summary render:\n%s", out)
	}
}
