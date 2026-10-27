package ui

// b23_wp2_npu_view_test.go — batch 23 W2: the NPU overview view.
//
// The view is a top-level conditional view (key `n`, a tab only when the cluster
// has NPU nodes). The probes pin, in order: the view's reachability and gating,
// the byte-anchored rendering (colour and --no-color), the summary bar against
// the matrix (same source), the navigation round trip through the node detail,
// and the width/height budgets.

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"go.uber.org/zap"
)

// npuChipGrades builds a grade slice in the exporter's chip layout (two chips
// per NPU), with the graded chips given by a map from chip index to grade.
func npuChipGrades(total int, graded map[int]string) []model.NPUChipGrade {
	grades := make([]model.NPUChipGrade, total)
	for i := range grades {
		grade := model.NPUChipGradeHealthy
		if override, ok := graded[i]; ok {
			grade = override
		}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: grade}
	}
	return grades
}

// npuViewFixture is the two-node / 32-chip fixture the byte anchors are built
// on: node-a reports all 16 chips (2 degraded, 1 fault), node-b reports none
// (monitoring unavailable). The alert list mixes NPU alerts with a pod alert
// that must not appear in the NPU section.
func npuViewFixture() *model.ClusterData {
	nodeA := &model.NodeData{
		Name: "node-a", Status: "Ready", NPUCapacity: 16, NPUAllocatable: 16, NPUAllocated: 7,
		NPUChipHealth: npuChipGrades(16, map[int]string{
			3:  model.NPUChipGradeDegraded,
			7:  model.NPUChipGradeFault,
			10: model.NPUChipGradeDegraded,
		}),
	}
	nodeB := &model.NodeData{
		Name: "node-b", Status: "Ready", NPUCapacity: 16, NPUAllocatable: 16, NPUAllocated: 4,
	}

	return &model.ClusterData{
		Nodes: []*model.NodeData{nodeA, nodeB},
		Summary: &model.ClusterSummary{
			NPUCapacity: 32, NPUAllocatable: 32, NPUAllocated: 11, NPUNodesCount: 2,
			Alerts: []model.Alert{
				{
					Severity: model.AlertSeverityCritical, Category: "NPU",
					AlertType: model.AlertTypeNPULinkDown, ResourceType: "Node", ResourceName: "node-a",
					Message: "NPU link down on chip(s) 7", MessageKey: "alert.npu_link_down",
					MessageArgs: map[string]interface{}{"Chips": "7"}, Value: "chips 7",
					Timestamp: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
				},
				{
					Severity: model.AlertSeverityWarning, Category: "NPU",
					AlertType: model.AlertTypeNPUTempHigh, ResourceType: "Node", ResourceName: "node-a",
					Message: "NPU temperature high on chip(s) 10 (max 82°C)", MessageKey: "alert.npu_temp_high",
					MessageArgs: map[string]interface{}{"Chips": "10", "Max": 82}, Value: "chips 10",
					Timestamp: time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC),
				},
				{
					Severity: model.AlertSeverityWarning, Category: "Pod",
					AlertType: model.AlertTypePodHighRestarts, ResourceType: "Pod", ResourceName: "web-0",
					Namespace: "default", Message: "Pod restarted 7 times", Value: "7 restarts",
					Timestamp: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
				},
			},
		},
	}
}

// npuViewModel returns a model on the NPU view with the fixture loaded. The
// matrix is put in the health mode explicitly: batch 30 made AI Core the
// default, and the byte anchors in this file are about the health matrix.
func npuViewModel(t *testing.T, locale string, width, height int) *Model {
	t.Helper()

	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width = width
	m.height = height
	m.currentView = ViewNPU
	m.clusterData = npuViewFixture()
	m.npuMatrixMode = npuMatrixModeHealth
	return m
}

// TestNPUViewEnGolden pins the English bytes of the whole NPU view on the
// fixture: the summary bar, the legend, the two node rows (full matrix and
// "monitoring unavailable") and the alert section with its per-type counts.
func TestNPUViewEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	const want = " 🧮 NPU Overview \n" +
		"\n" +
		"13/16 healthy · 2 degraded · 1 fault  •  Allocation 11/32 (34.4%)  •  Monitoring 1/2 nodes\n" +
		"Mode: Chip health (h to switch)\n" +
		"▉ healthy  ▒ degraded  ▁ fault\n" +
		"\n" +
		"> node-a    ▉ ▉  ▉ ▒  ▉ ▉  ▉ ▁  ▉ ▉  ▒ ▉  ▉ ▉  ▉ ▉  2 degraded • 1 fault\n" +
		"  node-b    monitoring unavailable\n" +
		"\n" +
		"NPU Alerts\n" +
		"temp high: 1 • link down: 1\n" +
		"🔴 Critical  node-a  NPU link down on chip(s) 7\n" +
		"🟡 Warning  node-a  NPU temperature high on chip(s) 10 (max 82°C)"

	if got := npuViewModel(t, "en", 100, 40).renderNPUView(); got != want {
		t.Errorf("en NPU view bytes changed:\n got %q\nwant %q", got, want)
	}
}

// TestNPUViewNoColorGolden pins the same fixture under --no-color: the styles
// emit no escapes and the grade glyphs carry the information on their own, which
// is the property the glyph choice exists for.
func TestNPUViewNoColorGolden(t *testing.T) {
	forcedAsciiProfile(t)

	out := npuViewModel(t, "en", 100, 40).renderNPUView()

	if strings.Contains(out, "\x1b[") {
		t.Fatalf("the no-colour render still carries ANSI escapes:\n%q", out)
	}
	// One row per node, and the three grades are distinguishable by glyph alone.
	row := lineContaining(out, "node-a")
	if row == "" {
		t.Fatalf("the node-a row is missing:\n%s", out)
	}
	for _, glyph := range []string{"▉", "▒", "▁"} {
		if !strings.Contains(row, glyph) {
			t.Errorf("the matrix row lost the %q glyph: %q", glyph, row)
		}
	}
	if got, want := strings.Count(row, "▉"), 13; got != want {
		t.Errorf("healthy glyph count = %d, want %d (one per healthy chip): %q", got, want, row)
	}
	if got, want := strings.Count(row, "▒"), 2; got != want {
		t.Errorf("degraded glyph count = %d, want %d: %q", got, want, row)
	}
	if got, want := strings.Count(row, "▁"), 1; got != want {
		t.Errorf("fault glyph count = %d, want %d: %q", got, want, row)
	}
}

// TestNPUViewZhSwitch pins the zh render: the summary, the legend, the row
// suffix, the unavailable note and the alert section all follow the locale,
// while the alert messages come from their own keys.
func TestNPUViewZhSwitch(t *testing.T) {
	forcedAsciiProfile(t)

	out := npuViewModel(t, "zh", 100, 40).renderNPUView()

	for _, want := range []string{
		"🧮 NPU 总览",
		"13/16 健康 · 2 降级 · 1 故障",
		"分配 11/32（34.4%）",
		"监控 1/2 节点",
		"▉ 健康  ▒ 降级  ▁ 故障",
		"2 降级 • 1 故障",
		"监控不可用",
		"NPU 告警",
		"温度过高: 1 • 链路离线: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("zh NPU view is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"healthy", "degraded", "Monitoring", "monitoring unavailable", "NPU Alerts"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("zh NPU view still contains the English %q:\n%s", unwanted, out)
		}
	}
}

// TestNPUViewNoAlertMessageLeak pins that the alert section only lists NPU
// alerts: the fixture carries a pod alert, which must not be counted or shown.
func TestNPUViewNoAlertMessageLeak(t *testing.T) {
	forcedAsciiProfile(t)

	out := npuViewModel(t, "en", 120, 40).renderNPUView()

	if strings.Contains(out, "web-0") || strings.Contains(out, "restarted") {
		t.Errorf("the NPU alert section leaked a pod alert:\n%s", out)
	}
	if !strings.Contains(out, "temp high: 1 • link down: 1") {
		t.Errorf("the NPU alert counts are wrong:\n%s", out)
	}
}

// TestNPUViewEmptyAlerts pins the no-alert state: it reuses the existing
// no-alerts key rather than leaving the section blank.
func TestNPUViewEmptyAlerts(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 100, 40)
	m.clusterData.Summary.Alerts = nil
	out := m.renderNPUView()

	if !strings.Contains(out, m.T("msg.no_alerts_detected")) {
		t.Errorf("the empty NPU alert section does not show the no-alerts text:\n%s", out)
	}
}

// TestNPUViewMonitoringUnavailableNode pins the no-data node: it says so instead
// of drawing an empty matrix, and it is still selectable and openable (a node
// without chip data still has NPU information on its detail page).
func TestNPUViewMonitoringUnavailableNode(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 100, 40)
	m.View()

	// node-b is the second row in the fixture's render order.
	m.selectedIndex = 1
	m.View()
	if got := lineContaining(m.renderNPUView(), "node-b"); !strings.HasPrefix(strings.TrimPrefix(stripANSI(got), "> "), "node-b    monitoring unavailable") {
		t.Errorf("the no-data row does not read as monitoring unavailable: %q", stripANSI(got))
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewNodeDetail || m.selectedNode == nil || m.selectedNode.Name != "node-b" {
		t.Errorf("Enter on the no-data row opened %v (node %v), want the node-b detail", m.currentView, m.selectedNode)
	}
}

// TestNPUViewSummaryMatchesMatrix is the same-source probe: the summary counts
// must equal the glyphs the matrix draws, because both must come out of the same
// per-chip grades.
func TestNPUViewSummaryMatchesMatrix(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 200, 40)
	out := m.renderNPUView()

	healthy, degraded, fault := 0, 0, 0
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "node-") {
			continue
		}
		healthy += strings.Count(line, "▉")
		degraded += strings.Count(line, "▒")
		fault += strings.Count(line, "▁")
	}

	// The legend line carries one glyph of each grade; it is not a node row, so
	// it is excluded above by the "node-" filter.
	summaryHealthy, summaryDegraded, summaryFault := npuChipTally(m.npuNodes())
	if healthy != summaryHealthy || degraded != summaryDegraded || fault != summaryFault {
		t.Errorf("matrix glyphs (healthy %d, degraded %d, fault %d) disagree with the tally (healthy %d, degraded %d, fault %d)",
			healthy, degraded, fault, summaryHealthy, summaryDegraded, summaryFault)
	}
	if !strings.Contains(out, "13/16 healthy · 2 degraded · 1 fault") {
		t.Errorf("the summary bar does not carry the same numbers:\n%s", out)
	}
}

// TestNPUViewAllocationZeroAllocatable pins the divide-by-zero guard: a cluster
// whose allocatable NPU count is zero shows the allocation without a percentage
// instead of printing NaN.
func TestNPUViewAllocationZeroAllocatable(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 100, 40)
	m.clusterData.Summary.NPUAllocatable = 0
	m.clusterData.Summary.NPUAllocated = 0

	out := m.renderNPUView()
	if strings.Contains(out, "NaN") || strings.Contains(out, "Inf") {
		t.Fatalf("the allocation line printed a non-finite percentage:\n%s", out)
	}
	if !strings.Contains(out, "Allocation 0/0") {
		t.Errorf("the allocation line is missing its counts:\n%s", out)
	}
	if strings.Contains(out, "Allocation 0/0 (") {
		t.Errorf("the allocation line printed a percentage of zero allocatable:\n%s", out)
	}
}

// TestNPUViewAllChipsMissing pins the "no chip data at all" summary state: every
// NPU node is uncovered, and the health segment says so instead of claiming
// "0/0 healthy".
func TestNPUViewAllChipsMissing(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 100, 40)
	for _, node := range m.clusterData.Nodes {
		node.NPUChipHealth = nil
	}

	out := m.renderNPUView()
	if !strings.Contains(out, m.T("npuview.health_no_data")) {
		t.Errorf("the health segment does not report missing data:\n%s", out)
	}
	if !strings.Contains(out, "Monitoring 0/2 nodes") {
		t.Errorf("the coverage segment does not report the uncovered nodes:\n%s", out)
	}
	if strings.Contains(out, "0/0 healthy") {
		t.Errorf("the health segment claims a 0/0 tally:\n%s", out)
	}
}

// TestNPUViewColorModeEndToEnd drives the view through SetColorMode, the switch
// --no-color actually flips: "never" must leave the whole frame without a single
// escape, and "always" must colour the matrix cells (the glyphs do not depend on
// the colour, but the colour has to follow the switch rather than be hardcoded
// on).
func TestNPUViewColorModeEndToEnd(t *testing.T) {
	saved := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	t.Run("never renders plain text end to end", func(t *testing.T) {
		lipgloss.SetColorProfile(termenv.TrueColor)
		SetColorMode("never")

		frame := npuViewModel(t, "en", 100, 40).View()
		if strings.Contains(frame, "\x1b[") {
			t.Errorf("the --no-color frame carries ANSI escapes:\n%q", frame)
		}
		if !strings.Contains(frame, "▉") || !strings.Contains(frame, "▁") {
			t.Errorf("the --no-color frame lost the grade glyphs:\n%s", frame)
		}
	})

	t.Run("always colours the cells", func(t *testing.T) {
		lipgloss.SetColorProfile(termenv.Ascii)
		SetColorMode("always")

		out := npuViewModel(t, "en", 100, 40).renderNPUView()
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("the forced-colour render carries no escapes:\n%q", out)
		}
		for _, glyph := range []string{"▉", "▒", "▁"} {
			if !strings.Contains(out, glyph) {
				t.Errorf("the coloured render lost the %q glyph", glyph)
			}
		}
	})
}

// TestNPUViewReachability drives the view through the real key handling: the
// `n` key enters the view when the cluster has NPU nodes, does nothing when it
// has none, and the tab and Tab cycle agree with each other.
func TestNPUViewReachability(t *testing.T) {
	t.Run("n opens the view on an NPU cluster", func(t *testing.T) {
		m := newTestModel(t)
		m.currentView = ViewNodes
		m.clusterData = npuViewFixture()

		m.Update(keyMsg("n"))
		if m.currentView != ViewNPU {
			t.Fatalf("n on an NPU cluster opened %v, want the NPU view", m.currentView)
		}
		if m.selectedIndex != 0 || m.scrollOffset != 0 {
			t.Errorf("entering the view left selection/scroll at %d/%d, want 0/0", m.selectedIndex, m.scrollOffset)
		}
	})

	t.Run("n is a no-op without NPU nodes", func(t *testing.T) {
		m := newTestModel(t)
		m.currentView = ViewNodes
		m.clusterData = &model.ClusterData{
			Nodes:   []*model.NodeData{{Name: "plain", NPUCapacity: 0}},
			Summary: &model.ClusterSummary{},
		}

		m.Update(keyMsg("n"))
		if m.currentView != ViewNodes {
			t.Fatalf("n without NPU nodes switched to %v, want to stay on the nodes view", m.currentView)
		}
	})

	t.Run("the tab appears only on an NPU cluster", func(t *testing.T) {
		withNPU := newTestModel(t)
		withNPU.clusterData = npuViewFixture()
		if !strings.Contains(withNPU.renderViewTabs(), "n:"+withNPU.T("views.npu.name")) {
			t.Errorf("the NPU tab is missing on an NPU cluster: %q", stripANSI(withNPU.renderViewTabs()))
		}

		withoutNPU := newTestModel(t)
		withoutNPU.clusterData = &model.ClusterData{
			Nodes:   []*model.NodeData{{Name: "plain"}},
			Summary: &model.ClusterSummary{},
		}
		if strings.Contains(stripANSI(withoutNPU.renderViewTabs()), withNPU.T("views.npu.name")) {
			t.Errorf("the NPU tab appeared without NPU nodes: %q", stripANSI(withoutNPU.renderViewTabs()))
		}
	})

	t.Run("the tab order matches the Tab cycle", func(t *testing.T) {
		m := newTestModel(t)
		m.currentView = ViewAlerts
		m.clusterData = npuViewFixture()
		m.clusterData.Queues = []*model.QueueData{{Name: "q"}}
		m.clusterData.Summary.SuperPodCount = 1
		for _, node := range m.clusterData.Nodes {
			node.SuperPodID = "sp-1"
			node.InternalIP = "10.0.0.1"
		}

		// Tabs read 8 → n → 9 → 0, so the Tab cycle from the alerts view lands on
		// the NPU view, then Queues, then Topology.
		want := []ViewType{ViewNPU, ViewQueues, ViewTopology}
		for _, wantView := range want {
			m.Update(tea.KeyMsg{Type: tea.KeyTab})
			if m.currentView != wantView {
				t.Fatalf("Tab from %v landed on %v, want %v (tabs: %q)",
					m.currentView, m.currentView, wantView, stripANSI(m.renderViewTabs()))
			}
		}

		// And the cycle wraps back to Overview.
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if m.currentView != ViewOverview {
			t.Errorf("Tab past the last tab landed on %v, want the overview", m.currentView)
		}
	})

	t.Run("n is gated by the keyboard-owning modes", func(t *testing.T) {
		modes := []struct {
			name string
			set  func(*Model)
		}{
			{"detailMode", func(m *Model) { m.detailMode = true }},
			{"filterMode", func(m *Model) { m.filterMode = true }},
			{"logsMode", func(m *Model) { m.logsMode = true }},
			{"commandOutputMode", func(m *Model) { m.commandOutputMode = true }},
			{"actionMenuMode", func(m *Model) { m.actionMenuMode = true }},
		}
		for _, mode := range modes {
			t.Run(mode.name, func(t *testing.T) {
				m := newTestModel(t)
				m.currentView = ViewNodes
				m.clusterData = npuViewFixture()
				mode.set(m)

				m.Update(keyMsg("n"))
				if m.currentView != ViewNodes {
					t.Errorf("n while %s was active switched to %v, want the nodes view", mode.name, m.currentView)
				}
			})
		}
	})

	t.Run("n is a search character, not a view switch", func(t *testing.T) {
		m := newTestModel(t)
		m.currentView = ViewNodes
		m.clusterData = npuViewFixture()
		m.searchMode = true

		m.Update(keyMsg("n"))
		if m.currentView != ViewNodes {
			t.Fatalf("n in search mode switched to %v, want the nodes view", m.currentView)
		}
		if m.searchText != "n" {
			t.Errorf("n in search mode did not reach the search text: %q", m.searchText)
		}
	})
}

// TestNPUViewEnterAndEscRoundTrip pins the navigation to the node detail and
// back: the selection and the scroll offset survive, and the origin flag does
// not leak.
func TestNPUViewEnterAndEscRoundTrip(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 100, 40)
	m.View() // the renderer reports its row geometry

	m.selectedIndex = 1
	m.scrollOffset = 3
	m.View()

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewNodeDetail {
		t.Fatalf("Enter on the NPU view opened %v, want the node detail", m.currentView)
	}
	if m.selectedNode == nil || m.selectedNode.Name != "node-b" {
		t.Fatalf("Enter opened node %v, want node-b", m.selectedNode)
	}
	if !m.detailMode || !m.fromNPUView {
		t.Fatalf("Enter set detailMode=%v fromNPUView=%v, want both true", m.detailMode, m.fromNPUView)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.currentView != ViewNPU {
		t.Fatalf("Esc from the node detail returned to %v, want the NPU view", m.currentView)
	}
	if m.detailMode || m.fromNPUView {
		t.Errorf("Esc left detailMode=%v fromNPUView=%v, want both false", m.detailMode, m.fromNPUView)
	}
	if m.selectedNode != nil {
		t.Errorf("Esc left selectedNode = %v, want nil", m.selectedNode)
	}
	if m.selectedIndex != 1 || m.scrollOffset != 3 {
		t.Errorf("Esc moved the list state: selectedIndex=%d scrollOffset=%d, want 1/3", m.selectedIndex, m.scrollOffset)
	}
}

// TestNPUViewFlagClearedByActionMenu is the poison-chain probe in the shape of
// the batch-22 W1-6 tests: after entering a node detail from the NPU view, a
// "Show Events" action lands on the events list, and the origin flag must be
// cleared there - otherwise a later node detail on the node whatever path would
// believe it came from the NPU view and Esc would jump to the wrong list.
func TestNPUViewFlagClearedByActionMenu(t *testing.T) {
	m := npuViewModel(t, "en", 100, 40)
	m.View()

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewNodeDetail || !m.fromNPUView {
		t.Fatalf("precondition failed: currentView=%v fromNPUView=%v", m.currentView, m.fromNPUView)
	}

	m.Update(keyMsg("a"))
	idx := -1
	for i, item := range m.getActionMenuItems() {
		if item.Action == ActionShowEvents {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("the node action menu has no Show Events item")
	}
	for i := 0; i < idx; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewEvents {
		t.Fatalf("Show Events opened %v, want the events view", m.currentView)
	}
	if m.fromNPUView {
		t.Error("fromNPUView survived the Show Events action")
	}

	// A node detail opened from the nodes list must now Esc back to that list.
	m.currentView = ViewNodes
	m.selectedIndex = 0
	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.currentView != ViewNodes {
		t.Errorf("Esc after the action menu returned to %v, want the nodes view", m.currentView)
	}
}

// TestNPUViewEscFlagDoesNotLeak pins that the origin flag has exactly one
// consumer: a node detail opened from the nodes list must keep returning to the
// nodes list, also after a round trip through the NPU view.
func TestNPUViewEscFlagDoesNotLeak(t *testing.T) {
	m := npuViewModel(t, "en", 100, 40)
	m.View()

	// Round trip through the NPU view.
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.fromNPUView {
		t.Fatal("fromNPUView survived the round trip")
	}

	// Now open a node from the nodes list: Esc must land there.
	m.currentView = ViewNodes
	m.selectedIndex = 0
	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewNodeDetail {
		t.Fatalf("Enter on the nodes list opened %v, want the node detail", m.currentView)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.currentView != ViewNodes {
		t.Errorf("Esc from a node detail opened from the nodes list returned to %v, want the nodes view", m.currentView)
	}
}

// TestNPUViewEscOnListIsNoOp pins the list-level Esc: like the events list, the
// NPU view has no layer to pop, so Esc must change nothing.
func TestNPUViewEscOnListIsNoOp(t *testing.T) {
	m := npuViewModel(t, "en", 100, 40)
	m.View()
	m.selectedIndex = 1

	before := *m
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.currentView != before.currentView {
		t.Errorf("Esc on the NPU list moved to %v", m.currentView)
	}
	if m.selectedIndex != before.selectedIndex || m.scrollOffset != before.scrollOffset {
		t.Errorf("Esc on the NPU list moved the selection/scroll")
	}
	if m.detailMode || m.fromNPUView {
		t.Errorf("Esc on the NPU list set detailMode=%v fromNPUView=%v", m.detailMode, m.fromNPUView)
	}
}

// TestNPUViewExportIsInert pins the export key: the NPU view is not exportable in
// this batch, so `e` must fall through to the "view unsupported" path without
// panicking and without starting an export.
func TestNPUViewExportIsInert(t *testing.T) {
	m := npuViewModel(t, "en", 100, 40)

	m.Update(keyMsg("e"))
	if m.exportInProgress {
		t.Fatalf("e on the NPU view started an export")
	}
	if _, ok := tryUpdate(m, keyMsg("e")); !ok {
		t.Fatalf("e on the NPU view panicked")
	}
}

// TestNPUViewFitsWidths pins the width budget: every line the view renders fits
// the terminal, at the widths that historically broke the layout and below them
// (down to 10 cells, where the view has to give up content rather than wrap), in
// both locales. The fixture's node names are long enough to exercise the name
// column truncation.
func TestNPUViewFitsWidths(t *testing.T) {
	forcedAsciiProfile(t)

	for _, width := range []int{10, 20, 38, 40, 60, 80, 120} {
		for _, locale := range []string{"en", "zh"} {
			t.Run(strings.Join([]string{locale, itoa(width)}, "/"), func(t *testing.T) {
				m := npuViewModel(t, locale, width, 40)
				m.clusterData.Nodes[0].Name = "npu-worker-node-very-long-name-01"
				m.clusterData.Nodes[1].Name = "npu-worker-node-very-long-name-02"
				// A third node without chip data, so the "monitoring unavailable"
				// note is on screen at every width too.
				m.clusterData.Nodes = append(m.clusterData.Nodes, &model.NodeData{
					Name: "npu-worker-node-very-long-name-03", NPUCapacity: 8,
				})

				out := m.renderNPUView()
				for _, line := range strings.Split(out, "\n") {
					if w := runewidth.StringWidth(stripANSI(line)); w > width {
						t.Errorf("line is %d cells wide on a %d-cell terminal: %q", w, width, stripANSI(line))
					}
				}

				// The matrix itself must survive: from 38 cells up the selected
				// row (the first fixture node, the one with 16 chips) still shows
				// at least one chip glyph.
				if width < 38 {
					return
				}
				row := ""
				for _, line := range strings.Split(out, "\n") {
					if strings.HasPrefix(stripANSI(line), npuViewSelectionPrefix) {
						row = line
						break
					}
				}
				if row == "" {
					t.Fatalf("no selected node row rendered at width %d:\n%s", width, out)
				}
				if !strings.Contains(row, "▉") {
					t.Errorf("the node row lost its whole matrix at width %d: %q", width, stripANSI(row))
				}
			})
		}
	}
}

// itoa is a tiny helper so the subtest names read as "en/38".
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestNPUViewNarrowShrinksNameBeforeMatrix pins the shrink order that the width
// budget relies on: the name column gives way first, so the whole matrix survives
// as long as there is room for it. A row that kept the full name and cut chips
// instead would still fit the terminal, but it would hide the very thing the
// view is for.
func TestNPUViewNarrowShrinksNameBeforeMatrix(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 80, 40)
	const longName = "npu-worker-node-very-long-name-0123456789"
	m.clusterData.Nodes[0].Name = longName
	// The rows are sorted by name, so select the renamed node by its position in
	// that order.
	for i, node := range m.npuNodes() {
		if node.Name == longName {
			m.selectedIndex = i
		}
	}
	out := m.renderNPUView()

	row := lineContaining(out, npuViewSelectionPrefix)
	if row == "" {
		t.Fatalf("no selected node row rendered:\n%s", out)
	}
	// All 16 chips still fit at 80 cells once the name column is capped at
	// npuViewNameWidthMax: the column is a fixed-share column, not one that grows
	// with the name.
	if got, want := strings.Count(row, "▉")+strings.Count(row, "▒")+strings.Count(row, "▁"), 16; got != want {
		t.Errorf("the row drew %d cells at width 80, want all %d: %q", got, want, stripANSI(row))
	}
	nameField := strings.SplitN(strings.TrimPrefix(stripANSI(row), npuViewSelectionPrefix), "  ", 2)[0]
	if got := runewidth.StringWidth(nameField); got != npuViewNameWidthMax {
		t.Errorf("the name column is %d cells wide, want it capped at %d: %q",
			got, npuViewNameWidthMax, stripANSI(row))
	}
	if !strings.Contains(nameField, "...") {
		t.Errorf("the name column was not truncated: %q", nameField)
	}
}

// TestNPUViewRowGeometryMatchesRenderer pins the geometry the selection-follow
// logic indexes into: every reported document line must be the line that renders
// the corresponding node's row. Both sides read the same list, but a shift would
// leave the selection-follow bringing the wrong line on screen, which the
// frame-level probes cannot see because the clamp hides it.
func TestNPUViewRowGeometryMatchesRenderer(t *testing.T) {
	m := npuViewModel(t, "en", 120, 40)
	lines := m.npuContentLines()

	nodes := m.npuNodes()
	if len(m.npuNodeLines) != len(nodes) {
		t.Fatalf("npuNodeLines has %d entries, want %d", len(m.npuNodeLines), len(nodes))
	}
	for i, node := range nodes {
		line := m.npuNodeLines[i]
		if line < 0 || line >= len(lines) {
			t.Fatalf("npuNodeLines[%d] = %d is outside the %d content lines", i, line, len(lines))
		}
		if got := stripANSI(lines[line]); !strings.Contains(got, node.Name) {
			t.Errorf("npuNodeLines[%d] = %d points at %q, want the row of %s", i, line, got, node.Name)
		}
	}
}

// TestNPUViewFrameFitsHeight pins the frame budget: the view never renders more
// lines than the terminal, and the header and help footer stay on screen. The
// fixture is grown past the shortest terminal so the windowing is exercised.
func TestNPUViewFrameFitsHeight(t *testing.T) {
	for _, height := range []int{50, 35, 30, 20} {
		t.Run(itoa(height), func(t *testing.T) {
			m := npuViewModel(t, "en", 120, height)
			for i := 0; i < 12; i++ {
				m.clusterData.Nodes = append(m.clusterData.Nodes, &model.NodeData{
					Name:          "node-extra-" + itoa(i),
					NPUCapacity:   16,
					NPUChipHealth: npuChipGrades(16, nil),
				})
			}

			lines := strings.Split(m.View(), "\n")
			if len(lines) > height {
				t.Fatalf("frame is %d lines tall on a %d-line terminal:\n%s", len(lines), height, strings.Join(lines, "\n"))
			}
			if !strings.Contains(lines[0], m.T("app.title")) {
				t.Errorf("the first line is not the app header: %q", lines[0])
			}
			if !strings.Contains(lines[len(lines)-1], m.T("keys.quit")) {
				t.Errorf("the last line is not the help footer: %q", lines[len(lines)-1])
			}
		})
	}
}

// TestNPUViewSelectionFollowsScroll pins the selection-follow mechanism: walking
// down the rows with j keeps the selected row on screen, and the last row is
// reachable.
func TestNPUViewSelectionFollowsScroll(t *testing.T) {
	const height = 20

	m := npuViewModel(t, "en", 120, height)
	for i := 0; i < 12; i++ {
		m.clusterData.Nodes = append(m.clusterData.Nodes, &model.NodeData{
			Name:          "node-extra-" + itoa(i),
			NPUCapacity:   16,
			NPUChipHealth: npuChipGrades(16, nil),
		})
	}
	m.View()

	for i := 0; i < 40; i++ {
		m.Update(keyMsg("j"))
		m.View()

		lines := strings.Split(m.View(), "\n")
		if len(lines) > height {
			t.Fatalf("frame is %d lines tall on a %d-line terminal", len(lines), height)
		}
		selected := m.npuNodes()[m.selectedIndex]
		if !containsSubstring(lines, selected.Name) {
			t.Fatalf("the selected row %q is off screen after %d j presses:\n%s",
				selected.Name, i+1, strings.Join(lines, "\n"))
		}
	}

	if got, want := m.selectedIndex, len(m.npuNodes())-1; got != want {
		t.Errorf("after walking to the bottom the selection is %d, want the last row %d", got, want)
	}
}

// TestNPUViewChipOrderFollowsGrades pins the matrix layout: chips are drawn in
// the node's grade order, grouped by NPU with a wider gap between NPUs.
func TestNPUViewChipOrderFollowsGrades(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 120, 40)
	row := lineContaining(m.renderNPUView(), "node-a")

	// The grades mark chip 3 (degraded), chip 7 (fault) and chip 10 (degraded);
	// their position in the glyph sequence is the chip index, so the sequence
	// itself proves the order.
	var glyphs []string
	for _, r := range stripANSI(row) {
		switch r {
		case '▉', '▒', '▁':
			glyphs = append(glyphs, string(r))
		}
	}
	if len(glyphs) != 16 {
		t.Fatalf("the row drew %d cells, want 16: %q", len(glyphs), stripANSI(row))
	}
	want := map[int]string{3: "▒", 7: "▁", 10: "▒"}
	for i, glyph := range glyphs {
		expected := "▉"
		if override, ok := want[i]; ok {
			expected = override
		}
		if glyph != expected {
			t.Errorf("cell %d is %q, want %q (full row: %q)", i, glyph, expected, stripANSI(row))
		}
	}

	// The NPU grouping: two chips per NPU separated by a single space, NPUs by
	// two. The first NPU group therefore reads "▉ ▉" and the next starts after a
	// double space.
	if !strings.Contains(stripANSI(row), "▉ ▉  ▉ ") {
		t.Errorf("the matrix does not group chips per NPU: %q", stripANSI(row))
	}
}
