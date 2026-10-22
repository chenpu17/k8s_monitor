package ui

// network_navigation_test.go — batch 20 lockdown for the Network view service
// table navigation.
//
// The network view reserves a selection across the services it renders
// (getMaxIndex returns the service count), but ↑/↓ used to be swallowed by the
// line-based scrolling and Enter had no branch. These tests pin the completed
// wiring: ↑/↓ move the selection over the rows in the order the table renders
// them (endpoint-less first), the window follows the selection, Enter opens the
// service detail of the rendered row, and Esc returns to the Network view —
// while the Workloads path into the same detail view keeps returning to
// Workloads.

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// networkServicesFixture returns three services whose endpoint counts make the
// table's endpoint-less-first sort observable: the zero-endpoint service
// (input position 1) must render first.
func networkServicesFixture() []*model.ServiceData {
	return []*model.ServiceData{
		{Name: "svc-busy", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.96.0.1", EndpointCount: 5, Ports: []model.ServicePort{{Protocol: "TCP", Port: 80}}},
		{Name: "svc-broken", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.96.0.2", EndpointCount: 0, Ports: []model.ServicePort{{Protocol: "TCP", Port: 80}}},
		{Name: "svc-quiet", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.96.0.3", EndpointCount: 2, Ports: []model.ServicePort{{Protocol: "TCP", Port: 80}}},
	}
}

// newNetworkModel builds a model on the network view with the given services,
// renders once so the renderer-reported geometry is in place, and returns it.
func newNetworkModel(t *testing.T, services []*model.ServiceData, height int) *Model {
	t.Helper()

	m := newTestModel(t)
	m.width = 220
	m.height = height
	m.currentView = ViewNetwork
	m.clusterData = &model.ClusterData{Services: services}
	m.View()
	return m
}

// highlightedRows returns the stripped lines of the network content that carry
// the selection marker.
func highlightedRows(m *Model) []string {
	var rows []string
	for _, line := range m.networkContentLines() {
		stripped := stripANSI(line)
		if strings.HasPrefix(stripped, "> ") {
			rows = append(rows, stripped)
		}
	}
	return rows
}

// TestNetworkDownMovesSelectionAlongRenderedRows drives ↓ once and checks the
// selection lands on the row the table renders second.
func TestNetworkDownMovesSelectionAlongRenderedRows(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)

	if m.selectedIndex != 0 {
		t.Fatalf("fresh model starts with selectedIndex = %d, want 0", m.selectedIndex)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.View()

	if m.selectedIndex != 1 {
		t.Fatalf("after ↓ selectedIndex = %d, want 1", m.selectedIndex)
	}

	// Render order: svc-broken (0 endpoints) first, then svc-quiet (2), then
	// svc-busy (5). The highlighted row must be the rendered second row.
	if got := m.sortedNetworkServices()[1]; got != services[2] {
		t.Fatalf("sorted[1] = %s, want svc-quiet", got.Name)
	}

	rows := highlightedRows(m)
	if len(rows) != 1 {
		t.Fatalf("exactly one row must carry the selection marker, got %d: %q", len(rows), rows)
	}
	if !strings.Contains(rows[0], "svc-quiet") {
		t.Fatalf("highlighted row %q is not the rendered second row (svc-quiet)", rows[0])
	}

	// ↑ walks back to the first rendered row.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.View()
	if m.selectedIndex != 0 {
		t.Fatalf("after ↑ selectedIndex = %d, want 0", m.selectedIndex)
	}
	rows = highlightedRows(m)
	if len(rows) != 1 || !strings.Contains(rows[0], "svc-broken") {
		t.Fatalf("after ↑ the highlighted row must be svc-broken, got %q", rows)
	}
}

// TestNetworkEnterOpensServiceDetail checks Enter opens the detail of the
// rendered row the selection points at.
func TestNetworkEnterOpensServiceDetail(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)
	m.selectedIndex = 1

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.currentView != ViewServiceDetail {
		t.Fatalf("after Enter currentView = %v, want ViewServiceDetail", m.currentView)
	}
	if m.selectedService != services[2] {
		t.Fatalf("selectedService = %v, want the rendered second row (svc-quiet)", m.selectedService)
	}
	if !m.detailMode {
		t.Fatal("Enter must enter detail mode")
	}
	if m.detailScrollOffset != 0 {
		t.Fatalf("detailScrollOffset = %d, want 0", m.detailScrollOffset)
	}
	if !m.fromNetworkView {
		t.Fatal("the network path must set fromNetworkView")
	}
}

// TestNetworkEnterFollowsRenderedOrder proves the Enter order is the render
// order: with the selection on the first row, Enter must open the
// zero-endpoint service even though it sits second in the input.
func TestNetworkEnterFollowsRenderedOrder(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.selectedService != services[1] {
		t.Fatalf("selectedService = %v, want the zero-endpoint svc-broken that renders first", m.selectedService)
	}
}

// TestNetworkServiceLinesGeometryMatchesTable pins the renderer-reported
// geometry: every networkServiceLines entry must point at the row that renders
// the corresponding service.
func TestNetworkServiceLinesGeometryMatchesTable(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)

	lines := m.networkContentLines()
	if len(m.networkServiceLines) != len(services) {
		t.Fatalf("networkServiceLines has %d entries, want %d", len(m.networkServiceLines), len(services))
	}
	for i, svc := range m.sortedNetworkServices() {
		line := stripANSI(lines[m.networkServiceLines[i]])
		if !strings.Contains(line, svc.Name) {
			t.Fatalf("networkServiceLines[%d] = %d points at %q, want the row of %s",
				i, m.networkServiceLines[i], line, svc.Name)
		}
	}
}

// TestNetworkServiceDetailEscReturnsToNetwork checks Esc from a network-opened
// service detail returns to the Network view with the list state intact.
func TestNetworkServiceDetailEscReturnsToNetwork(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)
	m.selectedIndex = 1
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewServiceDetail {
		t.Fatalf("precondition failed: currentView = %v", m.currentView)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m.currentView != ViewNetwork {
		t.Fatalf("after Esc currentView = %v, want ViewNetwork", m.currentView)
	}
	if m.fromNetworkView {
		t.Fatal("fromNetworkView must be cleared after returning")
	}
	if m.selectedService != nil {
		t.Fatalf("selectedService = %v, want nil", m.selectedService)
	}
	if m.detailMode {
		t.Fatal("detailMode must be cleared when returning to the list view")
	}
	if m.selectedIndex != 1 {
		t.Fatalf("selectedIndex = %d, want 1 (the list selection must survive the round trip)", m.selectedIndex)
	}
}

// TestWorkloadsServiceDetailEscStillReturnsToWorkloads pins the Workloads
// regression: entering a service detail from the Workloads view does not set
// fromNetworkView, so Esc keeps returning to Workloads.
func TestWorkloadsServiceDetailEscStillReturnsToWorkloads(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)
	m.currentView = ViewWorkloads
	m.selectedIndex = 0
	m.View() // populates m.workloadSections the way the frame render does

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewServiceDetail || m.selectedService != services[0] {
		t.Fatalf("precondition failed: currentView = %v, selectedService = %v", m.currentView, m.selectedService)
	}
	if m.fromNetworkView {
		t.Fatal("the Workloads path must not set fromNetworkView")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m.currentView != ViewWorkloads {
		t.Fatalf("after Esc currentView = %v, want ViewWorkloads", m.currentView)
	}
	if m.detailMode {
		t.Fatal("detailMode must be cleared when returning to Workloads")
	}
}

// TestNetworkWithoutServicesKeepsLineScrolling checks the fallback: with no
// services the ↑/↓ keys keep scrolling the document and Enter does nothing.
func TestNetworkWithoutServicesKeepsLineScrolling(t *testing.T) {
	pods := make([]*model.PodData, 0, 30)
	for i := 0; i < 30; i++ {
		pods = append(pods, &model.PodData{
			Name:      fmt.Sprintf("web-%02d", i),
			Namespace: "default",
			Node:      "node-1",
			Phase:     "Running",
			PodIP:     fmt.Sprintf("10.0.0.%d", i+1),
		})
	}
	m := newTestModel(t)
	m.width = 220
	m.height = 20
	m.currentView = ViewNetwork
	m.clusterData = &model.ClusterData{Pods: pods}
	m.View()

	if m.networkMaxScroll() == 0 {
		t.Fatalf("fixture does not scroll (content=%d, budget=%d); the probe would be vacuous",
			len(m.networkContentLines()), m.contentBudget())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.View()
	if m.selectedIndex != 0 {
		t.Fatalf("without services selectedIndex moved to %d, want 0", m.selectedIndex)
	}
	if m.scrollOffset != 1 {
		t.Fatalf("↓ must scroll the document, scrollOffset = %d, want 1", m.scrollOffset)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.View()
	if m.scrollOffset != 0 {
		t.Fatalf("↑ must scroll the document back, scrollOffset = %d, want 0", m.scrollOffset)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewNetwork || m.detailMode {
		t.Fatalf("Enter without services must be a no-op, got view=%v detailMode=%v", m.currentView, m.detailMode)
	}
}

// TestNetworkSelectionFollowKeepsRowInWindow shrinks the terminal so the table
// cannot fit, then walks the selection past the bottom of the window: the
// scroll offset must move and the selected row's document line must land inside
// the visible window (visibleLines arithmetic, not just rendered text).
func TestNetworkSelectionFollowKeepsRowInWindow(t *testing.T) {
	services := make([]*model.ServiceData, 0, 12)
	for i := 0; i < 12; i++ {
		services = append(services, &model.ServiceData{
			Name:          fmt.Sprintf("svc-%02d", i),
			Namespace:     "default",
			Type:          "ClusterIP",
			ClusterIP:     fmt.Sprintf("10.96.0.%d", i+1),
			EndpointCount: i,
			Ports:         []model.ServicePort{{Protocol: "TCP", Port: 80}},
		})
	}
	m := newNetworkModel(t, services, 20)

	if len(m.networkServiceLines) != len(services) {
		t.Fatalf("networkServiceLines has %d entries, want %d", len(m.networkServiceLines), len(services))
	}

	for i := 0; i < 12; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m.View()
	}

	if m.scrollOffset <= 0 {
		t.Fatalf("walking the selection past the window bottom never scrolled (scrollOffset = %d)", m.scrollOffset)
	}

	total := len(m.networkContentLines())
	visible := visibleLines(total, m.contentBudget(), scrollIndicatorLines)
	line := m.networkServiceLines[m.selectedIndex]
	if line < m.scrollOffset || line >= m.scrollOffset+visible {
		t.Fatalf("selected row %d (index %d) outside the visible window [%d, %d) of %d lines",
			line, m.selectedIndex, m.scrollOffset, m.scrollOffset+visible, total)
	}
}

// TestNetworkRefreshClampsSelection pins the generic refresh clamp on the
// network view: a snapshot with fewer services pulls the selection back in
// range.
func TestNetworkRefreshClampsSelection(t *testing.T) {
	services := networkServicesFixture()
	m := newNetworkModel(t, services, 50)
	m.selectedIndex = 2

	m.Update(clusterDataMsg{data: &model.ClusterData{Services: services[:1]}})
	m.View()

	if m.selectedIndex != 0 {
		t.Fatalf("after the refresh selectedIndex = %d, want 0", m.selectedIndex)
	}
}
