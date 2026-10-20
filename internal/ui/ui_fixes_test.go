package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourusername/k8s-monitor/internal/i18n"
	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// newTestModel builds a model with the minimal wiring the view renderers need.
func newTestModel(t *testing.T) *Model {
	t.Helper()

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width = 120
	m.height = 24
	return m
}

// keyMsg builds the key message for a single character key.
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// TestDetailScrollUpRespondsAfterOverscroll reproduces the reported defect: after
// pressing ↓ far more often than the content allows, a single ↑ must scroll the
// view immediately. The old code kept incrementing detailScrollOffset while the
// renderers clamped a local copy, so the offset sat far past the end of the
// content and ↑ had to be pressed dozens of times before anything moved.
func TestDetailScrollUpRespondsAfterOverscroll(t *testing.T) {
	cases := []struct {
		name   string
		view   ViewType
		height int
		setup  func(m *Model)
	}{
		{
			name:   "pod detail",
			view:   ViewPodDetail,
			height: 12,
			setup: func(m *Model) {
				pod := &model.PodData{
					Name:            "web-0",
					Namespace:       "default",
					Node:            "node-1",
					Phase:           "Running",
					PodIP:           "10.0.0.1",
					HostIP:          "10.0.0.100",
					Containers:      5,
					ReadyContainers: 5,
				}
				for i := 0; i < 5; i++ {
					pod.ContainerStates = append(pod.ContainerStates, model.ContainerState{
						Name:  "container-" + string(rune('a'+i)),
						Image: "example.com/app:v1",
						Ready: true,
						State: "Running",
					})
				}
				m.selectedPod = pod
			},
		},
		{
			name:   "node detail",
			view:   ViewNodeDetail,
			height: 20,
			setup: func(m *Model) {
				m.selectedNode = &model.NodeData{
					Name:           "node-1",
					InternalIP:     "10.0.0.100",
					Status:         "Ready",
					CPUAllocatable: 8000,
					CPUUsage:       2000,
					MemAllocatable: 16 * 1024 * 1024 * 1024,
					MemoryUsage:    4 * 1024 * 1024 * 1024,
					PodAllocatable: 110,
					PodCount:       4,
				}
				pods := make([]*model.PodData, 0, 4)
				for i := 0; i < 4; i++ {
					pods = append(pods, &model.PodData{
						Name:      "web-" + string(rune('a'+i)),
						Namespace: "default",
						Node:      "node-1",
						Phase:     "Running",
					})
				}
				m.clusterData = &model.ClusterData{Pods: pods}
			},
		},
		{
			name:   "pvc detail",
			view:   ViewPVCDetail,
			height: 12,
			setup: func(m *Model) {
				labels := map[string]string{}
				annotations := map[string]string{}
				for i := 0; i < 6; i++ {
					key := "example.com/key-" + string(rune('a'+i))
					labels[key] = "value"
					annotations[key] = "value"
				}
				m.selectedPVC = &model.PVCData{
					Name:             "data-0",
					Namespace:        "default",
					Status:           "Bound",
					Volume:           "pvc-1234",
					Capacity:         8 * 1024 * 1024 * 1024,
					RequestedStorage: 8 * 1024 * 1024 * 1024,
					StorageClass:     "standard",
					AccessModes:      []string{"ReadWriteOnce"},
					VolumeMode:       "Filesystem",
					Labels:           labels,
					Annotations:      annotations,
				}
			},
		},
		{
			name:   "deployment detail",
			view:   ViewDeploymentDetail,
			height: 20,
			setup: func(m *Model) {
				m.selectedDeployment = &model.DeploymentData{
					Name:              "web",
					Namespace:         "default",
					Replicas:          4,
					ReadyReplicas:     4,
					AvailableReplicas: 4,
					UpdatedReplicas:   4,
					Strategy:          "RollingUpdate",
					Conditions:        []string{"Available", "Progressing"},
				}
				pods := make([]*model.PodData, 0, 4)
				for i := 0; i < 4; i++ {
					pods = append(pods, &model.PodData{
						Name:      "web-" + string(rune('a'+i)),
						Namespace: "default",
						Node:      "node-1",
						Phase:     "Running",
					})
				}
				m.clusterData = &model.ClusterData{Pods: pods}
			},
		},
		{
			// Queue detail is the one renderer that writes the scroll offset
			// back to the model while rendering.
			name:   "queue detail",
			view:   ViewQueueDetail,
			height: 12,
			setup: func(m *Model) {
				m.selectedQueue = &model.QueueData{
					Name:            "default-queue",
					State:           "Open",
					Weight:          1,
					CPUDeserved:     8000,
					CPUAllocated:    2000,
					MemoryDeserved:  16 * 1024 * 1024 * 1024,
					MemoryAllocated: 4 * 1024 * 1024 * 1024,
					NPUDeserved:     8,
					NPUAllocated:    2,
					PodDeserved:     100,
					PodAllocated:    10,
					RunningJobs:     2,
					PendingJobs:     3,
					CompletedJobs:   5,
					FailedJobs:      1,
					NPUResourceName: "huawei.com/ascend-1980",
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.height = tc.height
			m.currentView = tc.view
			m.detailMode = true
			tc.setup(m)

			// The probe is only meaningful if the content actually scrolls.
			m.Update(keyMsg("j"))
			if m.detailScrollOffset == 0 {
				t.Fatal("test content is not scrollable, the probe would be vacuous")
			}

			// The bottom page, measured by brute force on the rendered output.
			maxScroll := probeMaxScroll(m)
			bottomPage := probeKey(m.renderDetailAt(maxScroll))

			// Scroll down far past the end of the content.
			for i := 0; i < 200; i++ {
				m.Update(keyMsg("j"))
				m.View()
			}

			bottom := m.detailScrollOffset
			for i := 0; i < 5; i++ {
				m.Update(keyMsg("j"))
				m.View()
			}
			if m.detailScrollOffset != bottom {
				t.Fatalf("offset kept growing past the end of the content: %d -> %d", bottom, m.detailScrollOffset)
			}
			if got := probeKey(m.renderDetailAt(bottom)); got != bottomPage {
				t.Fatalf("scrolling down stopped at offset %d (maxScroll=%d): the last line of the content is unreachable",
					bottom, maxScroll)
			}

			// A single ↑ must move the view immediately, one line up from the
			// bottom page.
			before := m.View()
			m.Update(keyMsg("k"))
			after := m.View()

			if before == after {
				t.Fatalf("pressing ↑ once after scrolling past the end did not move the view (offset=%d)", m.detailScrollOffset)
			}
			if got, want := m.detailScrollOffset, bottom-1; got != want {
				t.Fatalf("offset after one ↑ = %d, want %d", got, want)
			}
			// The view must no longer show the bottom page: one ↑ moved it.
			if probeKey(m.renderDetailAt(m.detailScrollOffset)) == bottomPage {
				t.Fatalf("after one ↑ the view still shows the bottom page")
			}
		})
	}
}

// TestDetailScrollUpAfterContentShrinks covers the offset left dangling when the
// detail content gets shorter under the cursor (a refresh can drop rows). The
// first ↑ must still move the view instead of burning presses on offsets the
// renderer clamps away.
func TestDetailScrollUpAfterContentShrinks(t *testing.T) {
	m := newTestModel(t)
	m.height = 12
	m.currentView = ViewPodDetail
	m.detailMode = true

	pod := &model.PodData{Name: "web-0", Namespace: "default", Node: "node-1", Phase: "Running"}
	for i := 0; i < 10; i++ {
		pod.ContainerStates = append(pod.ContainerStates, model.ContainerState{
			Name:  "container-" + string(rune('a'+i)),
			Image: "example.com/app:v1",
			State: "Running",
		})
	}
	m.selectedPod = pod

	for i := 0; i < 200; i++ {
		m.Update(keyMsg("j"))
		m.View()
	}

	// The pod loses most of its containers, so the content gets much shorter.
	m.selectedPod = &model.PodData{
		Name:            pod.Name,
		Namespace:       pod.Namespace,
		Node:            pod.Node,
		Phase:           pod.Phase,
		ContainerStates: pod.ContainerStates[:2],
	}
	m.View()

	before := m.View()
	m.Update(keyMsg("k"))
	after := m.View()

	if before == after {
		t.Fatalf("pressing ↑ once after the content shrank did not move the view (offset=%d)", m.detailScrollOffset)
	}
}

// TestFilterModeBlocksViewSwitchKeys covers the number/Tab view switches being
// reachable while the pods filter panel is open. Switching views left filterMode
// set, so later ↑/↓ presses navigated an invisible panel.
func TestFilterModeBlocksViewSwitchKeys(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewPods
	m.filterMode = true

	m.Update(keyMsg("2"))
	if m.currentView != ViewPods {
		t.Fatalf("number key switched view while filtering: currentView = %v", m.currentView)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.currentView != ViewPods {
		t.Fatalf("tab switched view while filtering: currentView = %v", m.currentView)
	}

	// Leaving filter mode restores the shortcuts.
	m.filterMode = false
	m.Update(keyMsg("2"))
	if m.currentView != ViewNodes {
		t.Fatalf("number key did not switch view after leaving filter mode: currentView = %v", m.currentView)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.currentView != ViewPods {
		t.Fatalf("tab did not switch view after leaving filter mode: currentView = %v", m.currentView)
	}
}

// TestSearchModeConsumesViewSwitchKeys documents that search mode needs no extra
// guard: digits are taken as search text and tab is swallowed before the view
// switch handlers are reached.
func TestSearchModeConsumesViewSwitchKeys(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewPods
	m.searchMode = true

	m.Update(keyMsg("2"))
	if m.currentView != ViewPods {
		t.Fatalf("number key switched view while searching: currentView = %v", m.currentView)
	}
	if m.searchText != "2" {
		t.Fatalf("digit was not taken as search text: searchText = %q", m.searchText)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.currentView != ViewPods {
		t.Fatalf("tab switched view while searching: currentView = %v", m.currentView)
	}
}

// TestNetworkStatsDetailedRendersValues guards against the network header
// rendering network.stats_detailed through T() (which leaves the template
// variables as "<no value>").
func TestNetworkStatsDetailedRendersValues(t *testing.T) {
	services := []*model.ServiceData{
		{Name: "svc-a", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.1", EndpointCount: 2},
		{Name: "svc-b", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.2", EndpointCount: 1},
		{Name: "svc-c", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.3", EndpointCount: 1},
		{Name: "svc-d", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.4", EndpointCount: 0},
	}

	cases := []struct {
		locale string
		want   string
	}{
		{"en", "With Endpoints: 3 • Without: 1 • Total Endpoints: 4"},
		{"zh", "有端点：3 • 无端点：1 • 总端点数：4"},
	}

	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			m := newTestModel(t)
			m.localizer = i18n.NewLocalizer(tc.locale)
			m.currentView = ViewNetwork
			m.clusterData = &model.ClusterData{Services: services}

			rendered := stripANSI(m.renderNetwork())
			if strings.Contains(rendered, "<no value>") {
				t.Fatalf("network view rendered an unfilled template variable:\n%s", rendered)
			}
			if !strings.Contains(rendered, tc.want) {
				t.Fatalf("network view header missing %q:\n%s", tc.want, rendered)
			}
		})
	}
}

// TestCopyActionRunsOffTheUpdateGoroutine checks that the clipboard write is
// deferred to a command: Update must not block on it and the message must be set
// once the command reports back.
func TestCopyActionRunsOffTheUpdateGoroutine(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.selectedPod = &model.PodData{Name: "web-0", Namespace: "default"}

	cmd := m.executeAction(ActionCopyName)
	if cmd == nil {
		t.Fatal("ActionCopyName did not return a command")
	}
	if m.exportMessage != "" {
		t.Fatalf("clipboard write ran synchronously in Update: exportMessage = %q", m.exportMessage)
	}

	msg := cmd()
	result, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("clipboard command returned %T, want clipboardResultMsg", msg)
	}
	if result.text != "web-0" {
		t.Fatalf("clipboard command copied %q, want %q", result.text, "web-0")
	}

	_, tickCmd := m.Update(clipboardResultMsg{text: "web-0"})
	if !strings.Contains(m.exportMessage, "web-0") {
		t.Fatalf("clipboard result did not reach the status message: %q", m.exportMessage)
	}
	if tickCmd == nil {
		t.Fatal("clipboard result did not schedule the status message to be cleared")
	}
}

// TestExportDataWritesTheSnapshot checks that an export started from one model
// state still writes that state after the model has moved on.
func TestExportDataWritesTheSnapshot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := newTestModel(t)
	m.currentView = ViewPods
	m.clusterData = &model.ClusterData{
		Pods: []*model.PodData{{Name: "web-0", Namespace: "default", Phase: "Running"}},
	}

	cmd := m.exportData(ExportCSV)

	// The model moves on (a refresh replaces the data) before the command runs.
	m.clusterData = &model.ClusterData{
		Pods: []*model.PodData{{Name: "web-1", Namespace: "default", Phase: "Running"}},
	}

	msg, ok := cmd().(exportSuccessMsg)
	if !ok {
		t.Fatalf("export returned %T, want exportSuccessMsg", msg)
	}
	if msg.count != 1 {
		t.Fatalf("export reported %d items, want 1", msg.count)
	}

	content, err := os.ReadFile(msg.filePath)
	if err != nil {
		t.Fatalf("reading exported file: %v", err)
	}
	if !strings.Contains(string(content), "web-0") {
		t.Fatalf("export did not write the snapshot taken when the export started:\n%s", content)
	}
	if strings.Contains(string(content), "web-1") {
		t.Fatalf("export wrote data from after the export started:\n%s", content)
	}
}

// TestExportDataSnapshotsModelState drives the export command concurrently with
// the model mutations Update performs. Run with -race: reading m.currentView or
// m.clusterData from inside the command goroutine is a data race.
func TestExportDataSnapshotsModelState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := newTestModel(t)
	m.currentView = ViewPods
	m.clusterData = &model.ClusterData{
		Pods: []*model.PodData{{Name: "web-0", Namespace: "default", Phase: "Running"}},
	}

	cmd := m.exportData(ExportCSV)

	done := make(chan tea.Msg, 1)
	go func() {
		done <- cmd()
	}()

	for i := 0; i < 200; i++ {
		m.currentView = ViewPods
		m.clusterData = &model.ClusterData{
			Pods: []*model.PodData{{Name: "web-1", Namespace: "default", Phase: "Running"}},
		}
	}

	if msg := <-done; msg == nil {
		t.Fatal("export command returned no message")
	}
}
