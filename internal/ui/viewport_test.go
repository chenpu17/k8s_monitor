package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// The probes in this file measure the frame against the rendered output rather
// than against the implementation: every view is rendered through View() at a
// given terminal height and the frame is measured as the terminal would see it.
// A view that renders one line too many makes the terminal scroll, which pushes
// the app header (and with it the view title) off the top of the screen - so the
// frame height is the defect, and it is observable without a terminal.

// frameLines returns the lines the terminal shows for the current model, i.e.
// what View() renders. A frame taller than m.height scrolls the terminal.
func frameLines(m *Model) []string {
	return strings.Split(m.View(), "\n")
}

// syntheticClusterData builds cluster data with enough items to overflow every
// list view, so the windowing of each view is actually exercised.
func syntheticClusterData(pods, nodes, events, alerts, pvs, pvcs, queues, services int) *model.ClusterData {
	data := &model.ClusterData{
		Summary: &model.ClusterSummary{
			TotalNodes:     nodes,
			ReadyNodes:     nodes,
			TotalPods:      pods,
			RunningPods:    pods,
			TotalEvents:    events,
			CPUCapacity:    192000,
			CPUUsed:        1800,
			MemoryCapacity: 1536 * 1024 * 1024 * 1024,
			MemoryUsed:     8 * 1024 * 1024 * 1024,
			TotalPVs:       pvs,
			TotalPVCs:      pvcs,
			TotalServices:  services,
			NPUCapacity:    8,
		},
	}

	for i := 0; i < nodes; i++ {
		data.Nodes = append(data.Nodes, &model.NodeData{
			Name: fmt.Sprintf("node-%02d", i), Status: "Ready",
			CPUCapacity: 192000, CPUAllocatable: 191400, CPUUsage: 1800,
			MemoryCapacity: 1536 * 1024 * 1024 * 1024, MemAllocatable: 1400 * 1024 * 1024 * 1024,
			MemoryUsage: 8 * 1024 * 1024 * 1024, PodCount: 30,
		})
	}

	for i := 0; i < pods; i++ {
		data.Pods = append(data.Pods, &model.PodData{
			Name: fmt.Sprintf("pod-%03d", i), Namespace: "default", Node: "node-00",
			Phase: "Running", PodIP: fmt.Sprintf("10.0.0.%d", i%250+1), HostIP: "10.0.0.100",
			CPUUsage: 10, MemoryUsage: 64 * 1024 * 1024,
		})
	}

	for i := 0; i < events; i++ {
		data.Events = append(data.Events, &model.EventData{
			Type: "Warning", Reason: "FailedScheduling", InvolvedObject: fmt.Sprintf("Pod/pod-%03d", i),
			Message: "no nodes available", Count: 3,
		})
	}

	for i := 0; i < alerts; i++ {
		severity := []model.AlertSeverity{model.AlertSeverityCritical, model.AlertSeverityWarning, model.AlertSeverityInfo}[i%3]
		data.Summary.Alerts = append(data.Summary.Alerts, model.Alert{
			Severity: severity, Category: "Pod", AlertType: model.AlertTypePodPendingTooLong,
			ResourceType: "Pod", ResourceName: fmt.Sprintf("pod-%03d", i), Namespace: "default",
			Message: "Pod pending for too long", Value: "12m", Threshold: "5m",
		})
	}

	for i := 0; i < pvs; i++ {
		data.PVs = append(data.PVs, &model.PVData{
			Name: fmt.Sprintf("pv-%03d", i), Status: "Bound", Claim: "default/pvc-0",
			Capacity: 10 * 1024 * 1024 * 1024, StorageClass: "csi-disk",
		})
	}
	for i := 0; i < pvcs; i++ {
		data.PVCs = append(data.PVCs, &model.PVCData{
			Name: fmt.Sprintf("pvc-%03d", i), Namespace: "default", Status: "Bound",
			Volume: "pv-000", Capacity: 10 * 1024 * 1024 * 1024, StorageClass: "csi-disk",
		})
	}

	for i := 0; i < queues; i++ {
		data.Queues = append(data.Queues, &model.QueueData{
			Name: fmt.Sprintf("queue-%02d", i), State: "Open", Weight: 1,
			NPUDeserved: 8, NPUAllocated: 2, CPUDeserved: 8000, CPUAllocated: 2000,
			MemoryDeserved: 16 * 1024 * 1024 * 1024, MemoryAllocated: 4 * 1024 * 1024 * 1024,
			RunningJobs: 2, PendingJobs: 3,
		})
	}
	data.VolcanoSummary = &model.VolcanoSummary{TotalQueues: queues, RunningJobs: 2 * queues, PendingJobs: 3 * queues}

	for i := 0; i < services; i++ {
		data.Services = append(data.Services, &model.ServiceData{
			Name: fmt.Sprintf("svc-%03d", i), Namespace: "default", Type: "ClusterIP",
			ClusterIP: fmt.Sprintf("10.96.0.%d", i%250+1), EndpointCount: i % 3,
			Ports: []model.ServicePort{{Protocol: "TCP", Port: 80}},
		})
	}

	return data
}

// listViews are the views whose content is longer than any terminal, plus the
// single-page views that must not overflow either.
var listViews = []struct {
	name string
	view ViewType
}{
	{"overview", ViewOverview},
	{"nodes", ViewNodes},
	{"pods", ViewPods},
	{"workloads", ViewWorkloads},
	{"network", ViewNetwork},
	{"storage", ViewStorage},
	{"events", ViewEvents},
	{"alerts", ViewAlerts},
	{"queues", ViewQueues},
	{"topology", ViewTopology},
	{"npu", ViewNPU},
}

// newFilledModel returns a model on the given view, filled with enough data for
// every list to overflow the terminal.
func newFilledModel(t *testing.T, view ViewType, height int) *Model {
	t.Helper()

	m := newTestModel(t)
	m.width = 220
	m.height = height
	m.currentView = view
	m.clusterData = syntheticClusterData(40, 30, 60, 12, 25, 25, 15, 30)
	m.clusterData.Summary.SuperPodCount = 20
	for i, node := range m.clusterData.Nodes {
		node.SuperPodID = fmt.Sprintf("superpod-%02d", i%20)
		node.NPUAllocatable = 8
		node.NPUCapacity = 8
		node.InternalIP = fmt.Sprintf("10.0.0.%d", i+1)
		// Two chips per NPU, one chip of every fourth node degraded, so the NPU
		// view has something to draw at every height.
		node.NPUChipHealth = make([]model.NPUChipGrade, 8)
		for chip := range node.NPUChipHealth {
			grade := model.NPUChipGradeHealthy
			if i%4 == 0 && chip == 3 {
				grade = model.NPUChipGradeDegraded
			}
			node.NPUChipHealth[chip] = model.NPUChipGrade{NPUID: chip / 2, Chip: chip % 2, Grade: grade}
		}
	}
	return m
}

// TestViewFrameFitsTerminalHeight is the regression guard for the views that
// rendered every line they had: the frame must never be taller than the
// terminal, and the header (first line) and help footer (last line) must stay on
// screen.
func TestViewFrameFitsTerminalHeight(t *testing.T) {
	for _, height := range []int{50, 35, 30, 20} {
		for _, tc := range listViews {
			t.Run(fmt.Sprintf("%s/%d", tc.name, height), func(t *testing.T) {
				m := newFilledModel(t, tc.view, height)

				lines := frameLines(m)
				if len(lines) > height {
					t.Fatalf("frame is %d lines tall on a %d-line terminal; the terminal scrolls and the header leaves the screen:\n%s",
						len(lines), height, strings.Join(lines, "\n"))
				}
				if !strings.Contains(lines[0], m.T("app.title")) {
					t.Fatalf("first line is not the app header: %q", lines[0])
				}
				if !strings.Contains(lines[len(lines)-1], m.T("keys.quit")) {
					t.Fatalf("last line is not the help footer: %q", lines[len(lines)-1])
				}
				if !containsSubstring(lines, m.T("views.overview.name")) {
					t.Fatalf("the view tabs are not on screen")
				}
			})
		}
	}
}

// TestNetworkScrollsToLastLine checks the network view can be scrolled to the
// end of its content: the last line it renders must be reachable with the keys a
// user has, at every height.
func TestNetworkScrollsToLastLine(t *testing.T) {
	for _, height := range []int{50, 35, 30} {
		t.Run(fmt.Sprintf("h%d", height), func(t *testing.T) {
			m := newFilledModel(t, ViewNetwork, height)

			content := m.networkContentLines()
			last := ""
			for i := len(content) - 1; i >= 0; i-- {
				if strings.TrimSpace(stripANSI(content[i])) != "" {
					last = content[i]
					break
				}
			}
			if last == "" {
				t.Fatal("the network content is empty, the probe would be vacuous")
			}

			for i := 0; i < 30; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				m.View()
			}

			lines := frameLines(m)
			if len(lines) > height {
				t.Fatalf("frame is %d lines tall on a %d-line terminal", len(lines), height)
			}
			if !containsSubstring(lines, stripANSI(last)) {
				t.Fatalf("the last line of the network content is unreachable:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
}

// TestListViewsReachLastItem scrolls every list to the bottom with the keys a
// user has and checks that the last item is on screen and the frame still fits.
// A view whose window is taller than its budget cannot be scrolled to the end
// without the frame overflowing.
func TestListViewsReachLastItem(t *testing.T) {
	const height = 30

	cases := []struct {
		name    string
		view    ViewType
		lastKey string // text that must be visible once scrolled to the bottom
	}{
		{"nodes", ViewNodes, "node-29"},
		{"pods", ViewPods, "pod-039"},
		{"events", ViewEvents, "pod-059"},
		{"alerts", ViewAlerts, "pod-011"},
		{"queues", ViewQueues, "queue-14"},
		{"storage", ViewStorage, "pvc-024"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newFilledModel(t, tc.view, height)

			// Scroll far past the end with the real key handlers. The frame is
			// rendered after every key, like the Bubble Tea loop does, so the
			// renderers report the rows they printed before the next key.
			for i := 0; i < 200; i++ {
				m.Update(keyMsg("j"))
				m.View()
			}
			for i := 0; i < 20; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				m.View()
			}

			lines := frameLines(m)
			if len(lines) > height {
				t.Fatalf("frame is %d lines tall on a %d-line terminal", len(lines), height)
			}
			if !containsSubstring(lines, tc.lastKey) {
				t.Fatalf("scrolled to the bottom but %q is not on screen:\n%s", tc.lastKey, strings.Join(lines, "\n"))
			}
		})
	}
}

// TestAlertSelectionStaysVisible checks the selection-follow logic against the
// renderer: an alert row is several lines tall, so a window computed from the
// terminal height would leave the selected alert off screen.
func TestAlertSelectionStaysVisible(t *testing.T) {
	const height = 30

	for _, key := range []tea.Msg{keyMsg("j"), tea.KeyMsg{Type: tea.KeyPgDown}} {
		t.Run(fmt.Sprintf("%v", key), func(t *testing.T) {
			m := newFilledModel(t, ViewAlerts, height)

			for i := 0; i < 30; i++ {
				m.Update(key)
				lines := frameLines(m)
				if len(lines) > height {
					t.Fatalf("frame is %d lines tall on a %d-line terminal", len(lines), height)
				}

				// The selected alert's resource must be inside the window the
				// list renders. The list shows the alerts grouped by severity,
				// so the selection index addresses that order.
				flatAlerts := alertsBySeverity(m.clusterData.Summary.Alerts)
				if m.selectedIndex >= len(flatAlerts) {
					t.Fatalf("selected index %d out of range", m.selectedIndex)
				}
				selected := flatAlerts[m.selectedIndex]
				want := fmt.Sprintf("• %s: %s/%s", selected.ResourceType, selected.Namespace, selected.ResourceName)
				if !containsSubstring(lines, want) {
					t.Fatalf("selected alert is off screen (index %d):\n%s", m.selectedIndex, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

// TestFitFrameTrimsContentKeepsChrome pins the safety net: content longer than
// the terminal is trimmed, while the header, the tabs and the help footer stay.
func TestFitFrameTrimsContentKeepsChrome(t *testing.T) {
	m := newTestModel(t)
	m.height = 12

	content := strings.Join([]string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9", "c10"}, "\n")
	frame := m.fitFrame("HEADER", content, "TABS", "FOOTER")
	lines := strings.Split(frame, "\n")

	if len(lines) > m.height {
		t.Fatalf("frame is %d lines tall on a %d-line terminal:\n%s", len(lines), m.height, frame)
	}
	if lines[0] != "HEADER" || lines[len(lines)-1] != "FOOTER" {
		t.Fatalf("the safety net dropped the header or the footer:\n%s", frame)
	}
	if !containsSubstring(lines, "TABS") {
		t.Fatalf("the safety net dropped the view tabs:\n%s", frame)
	}
	if !containsSubstring(lines, "c1") {
		t.Fatalf("the safety net dropped the first content line:\n%s", frame)
	}
	if containsSubstring(lines, "c10") {
		t.Fatalf("the safety net kept content past the terminal height:\n%s", frame)
	}
}

// TestScrollWindow covers the window arithmetic the views share: the slice, the
// clamping of a dangling offset and the lines reserved for the indicator.
func TestScrollWindow(t *testing.T) {
	cases := []struct {
		name          string
		total, offset int
		budget        int
		reserve       int
		wantStart     int
		wantEnd       int
	}{
		{"content fits", 10, 0, 20, 2, 0, 10},
		{"content fits, offset ignored", 10, 5, 20, 2, 0, 10},
		{"first page of overflowing content", 100, 0, 20, 2, 0, 18},
		{"middle page", 100, 40, 20, 2, 40, 58},
		{"last page", 100, 82, 20, 2, 82, 100},
		{"dangling offset shows the last page", 100, 500, 20, 2, 82, 100},
		{"negative offset clamps to the top", 100, -5, 20, 2, 0, 18},
		{"no reserve", 100, 0, 20, 0, 0, 20},
		{"budget below the reserve still shows a line", 100, 0, 2, 2, 0, 1},
		{"empty content", 0, 0, 20, 2, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := scrollWindow(tc.total, tc.offset, tc.budget, tc.reserve)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Fatalf("scrollWindow(%d, %d, %d, %d) = (%d, %d), want (%d, %d)",
					tc.total, tc.offset, tc.budget, tc.reserve, start, end, tc.wantStart, tc.wantEnd)
			}
			if end < start {
				t.Fatalf("window is inverted: [%d, %d)", start, end)
			}
			if end > tc.total {
				t.Fatalf("window reaches past the content: [%d, %d) of %d", start, end, tc.total)
			}
			if tc.total > tc.budget && end-start < 1 {
				t.Fatalf("window is empty although the content overflows")
			}
		})
	}
}

// TestMaxScrollOffsetReachesLastLine checks the offset that shows the last line
// is the one the views clamp to: scrolling there must show the final line, and
// one line further must not move the window.
func TestMaxScrollOffsetReachesLastLine(t *testing.T) {
	cases := []struct {
		total, budget, reserve int
	}{
		{100, 20, 2},
		{100, 20, 0},
		{10, 20, 2},
		{3, 2, 2},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d/%d/%d", tc.total, tc.budget, tc.reserve), func(t *testing.T) {
			maxOffset := maxScrollOffset(tc.total, tc.budget, tc.reserve)
			start, end := scrollWindow(tc.total, maxOffset, tc.budget, tc.reserve)
			if end != tc.total {
				t.Fatalf("offset %d shows [%d, %d), the last line of %d is unreachable", maxOffset, start, end, tc.total)
			}
			nextStart, nextEnd := scrollWindow(tc.total, maxOffset+1, tc.budget, tc.reserve)
			if nextStart != start || nextEnd != end {
				t.Fatalf("one line past the bottom page moved the window: [%d, %d) -> [%d, %d)", start, end, nextStart, nextEnd)
			}
		})
	}
}

// TestNetworkScrollClampsToContent pins that the network view cannot be scrolled
// past its content: a dangling offset must not cost the user several ↑ presses
// before the view moves again.
func TestNetworkScrollClampsToContent(t *testing.T) {
	m := newFilledModel(t, ViewNetwork, 30)

	for i := 0; i < 300; i++ {
		m.networkScrollBy(1)
	}
	if got, want := m.scrollOffset, m.networkMaxScroll(); got != want {
		t.Fatalf("offset after scrolling past the end = %d, want the bottom page %d", got, want)
	}

	before := m.View()
	m.networkScrollBy(-1)
	if m.View() == before {
		t.Fatalf("pressing ↑ once after scrolling past the end did not move the view (offset=%d)", m.scrollOffset)
	}

	for i := 0; i < 300; i++ {
		m.networkScrollBy(-1)
	}
	if m.scrollOffset != 0 {
		t.Fatalf("offset after scrolling past the top = %d, want 0", m.scrollOffset)
	}
}

// TestListIndicatorMatchesRenderedRows pins the scroll indicator against the
// rows the view actually printed: the window it reports must be exactly the
// rows on screen, and scrolling to the bottom must report the last item.
func TestListIndicatorMatchesRenderedRows(t *testing.T) {
	const height = 30

	m := newFilledModel(t, ViewPods, height)
	for i := 0; i < 200; i++ {
		m.Update(keyMsg("j"))
		m.View()
	}

	lines := frameLines(m)
	start, end, total := parseShowingIndicator(t, lines)
	if total != len(m.clusterData.Pods) {
		t.Fatalf("indicator reports %d items, the view has %d", total, len(m.clusterData.Pods))
	}
	if end != total {
		t.Fatalf("scrolled to the bottom but the indicator reports %d of %d", end, total)
	}

	// Count the pod rows on screen: everything between the table separator and
	// the stats line, minus the blank line before it.
	rows := 0
	for _, line := range lines {
		if strings.Contains(line, "pod-") && strings.Contains(line, "default") {
			rows++
		}
	}
	if rows != end-start+1 {
		t.Fatalf("the indicator reports %d rows (%d-%d) but %d rows are on screen:\n%s",
			end-start+1, start, end, rows, strings.Join(lines, "\n"))
	}
}

// parseShowingIndicator reads the "[Start-End of Total]" window the list views
// print in their stats line.
func parseShowingIndicator(t *testing.T, lines []string) (start, end, total int) {
	t.Helper()

	re := regexp.MustCompile(`\[(\d+)-(\d+)\s+\S+\s+(\d+)\]`)
	for _, line := range lines {
		match := re.FindStringSubmatch(stripANSI(line))
		if match == nil {
			continue
		}
		start, _ = strconv.Atoi(match[1])
		end, _ = strconv.Atoi(match[2])
		total, _ = strconv.Atoi(match[3])
		return start, end, total
	}
	t.Fatalf("no scroll indicator in the frame:\n%s", strings.Join(lines, "\n"))
	return 0, 0, 0
}

// containsSubstring reports whether any of the lines contains want.
func containsSubstring(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

// TestListViewContentFitsContentBudget guards the class of regression that
// TestViewFrameFitsTerminalHeight cannot see: fitFrame trims overflowing
// content silently, so a frame-height check stays green even when a view
// renders more lines than its content budget and the tail (a search panel, the
// scroll indicator) is cut off. This probe calls the renderers directly,
// bypassing View()/fitFrame, and requires the rendered content to fit the
// budget on its own.
//
// Heights below 25 are excluded on purpose: on such terminals the fixed chrome
// alone eats most of the budget, the window degrades to its floor by design and
// fitFrame trimming the remainder is the accepted behaviour (frame validity
// itself is covered by TestViewFrameFitsTerminalHeight at those heights).
func TestListViewContentFitsContentBudget(t *testing.T) {
	renderers := []struct {
		name   string
		view   ViewType
		render func(*Model) string
	}{
		{"nodes", ViewNodes, (*Model).renderNodes},
		{"pods", ViewPods, (*Model).renderPods},
		{"events", ViewEvents, (*Model).renderEvents},
		{"storage", ViewStorage, (*Model).renderStorage},
		{"queues", ViewQueues, (*Model).renderQueues},
		{"alerts", ViewAlerts, (*Model).renderAlerts},
		{"network", ViewNetwork, (*Model).renderNetwork},
		{"npu", ViewNPU, (*Model).renderNPUView},
	}

	for _, h := range []int{50, 35, 30, 25} {
		for _, tc := range renderers {
			for _, mode := range []string{"plain", "search", "filter"} {
				t.Run(fmt.Sprintf("%s/%s/%d", tc.name, mode, h), func(t *testing.T) {
					m := newFilledModel(t, tc.view, h)
					switch mode {
					case "search":
						m.searchMode = true
					case "filter":
						m.filterMode = true
					}

					content := tc.render(m)
					if n := lineCount(content); n > m.contentBudget() {
						t.Fatalf("%s rendered %d content lines in %s mode on a %d-line terminal, content budget is %d; the tail would be trimmed silently:\n%s",
							tc.name, n, mode, h, m.contentBudget(), content)
					}
				})
			}
		}
	}
}

// TestSearchPanelVisibleInListViews is the end-to-end companion of the budget
// probe: with the search panel open the input line, the placeholder and the key
// help must all survive in the rendered frame.
func TestSearchPanelVisibleInListViews(t *testing.T) {
	for _, tc := range []struct {
		name string
		view ViewType
	}{
		{"pods", ViewPods},
		{"nodes", ViewNodes},
		{"events", ViewEvents},
	} {
		for _, h := range []int{50, 35, 30, 25} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, h), func(t *testing.T) {
				m := newFilledModel(t, tc.view, h)
				m.searchMode = true

				frame := m.View()
				for _, key := range []string{"search.placeholder", "search.help"} {
					if !strings.Contains(frame, m.T(key)) {
						t.Fatalf("search panel is missing %q on %s at height %d; it would be trimmed silently:\n%s",
							key, tc.name, h, frame)
					}
				}
			})
		}
	}
}
