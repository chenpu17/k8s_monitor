package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// WP1 probes: the state machine and the key routing of Update. Every probe
// drives the model through the same messages Bubble Tea delivers and asserts
// on the resulting state, so a mode that keeps the keyboard (search after
// Enter, command output under the action menu) is observable without a
// terminal.

// emptyListModel returns a model on view with cluster data that has zero items
// in that view, so getMaxIndex() is 0.
func emptyListModel(t *testing.T, view ViewType) *Model {
	t.Helper()

	m := newTestModel(t)
	m.currentView = view
	m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{}}
	return m
}

// TestEmptyListPageKeysHaveNoEffect is the W1-1 regression guard: PgDn on an
// empty list used to set selectedIndex to maxIndex-1 == -1 (and scrollOffset
// negative), and the next Enter dereferenced items[-1]. Both page keys must
// leave an empty list alone instead of producing a negative selection.
func TestEmptyListPageKeysHaveNoEffect(t *testing.T) {
	views := []struct {
		name string
		view ViewType
	}{
		{"pods", ViewPods},
		{"nodes", ViewNodes},
		{"workloads", ViewWorkloads},
		{"events", ViewEvents},
		{"alerts", ViewAlerts},
	}

	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			m := emptyListModel(t, tc.view)

			m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
			if m.selectedIndex != 0 || m.scrollOffset != 0 {
				t.Fatalf("PgDn on an empty list moved the selection: selectedIndex=%d scrollOffset=%d, want 0/0",
					m.selectedIndex, m.scrollOffset)
			}
			if _, ok := tryUpdate(m, tea.KeyMsg{Type: tea.KeyEnter}); !ok {
				t.Fatalf("Enter after PgDn on an empty list panicked")
			}

			m.selectedIndex, m.scrollOffset = 0, 0
			m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
			if m.selectedIndex != 0 || m.scrollOffset != 0 {
				t.Fatalf("PgUp on an empty list moved the selection: selectedIndex=%d scrollOffset=%d, want 0/0",
					m.selectedIndex, m.scrollOffset)
			}
		})
	}
}

// tryUpdate runs one Update in a recover guard, reporting whether it returned
// normally. The probes want the positive behaviour (no panic); the recover only
// turns a panic into a readable failure instead of a crash of the test binary.
func tryUpdate(m *Model, msg tea.Msg) (cmd tea.Cmd, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	_, cmd = m.Update(msg)
	return cmd, true
}

// TestDetailEntryLeavesSearchMode is the W1-2 regression guard: opening a
// detail view from a searched list used to keep searchMode set, so the search
// interception block ran ahead of the detail keys — Esc was eaten, `q` typed a
// character and the footer advertised the search bindings.
func TestDetailEntryLeavesSearchMode(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewPods
	m.clusterData = &model.ClusterData{
		Summary: &model.ClusterSummary{},
		Pods: []*model.PodData{
			{Name: "web-0", Namespace: "default", Phase: "Running", Node: "node-1"},
			{Name: "web-1", Namespace: "default", Phase: "Running", Node: "node-1"},
		},
	}

	m.Update(keyMsg("/"))
	m.Update(keyMsg("w"))
	if m.searchText != "w" {
		t.Fatalf("search text = %q, want %q", m.searchText, "w")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.detailMode || m.currentView != ViewPodDetail {
		t.Fatalf("Enter in search mode did not open the pod detail: detailMode=%v view=%v", m.detailMode, m.currentView)
	}
	if m.searchMode {
		t.Errorf("searchMode is still set after entering the detail view; the search block keeps the keyboard")
	}

	footer := stripANSI(m.renderFooter())
	if strings.Contains(footer, m.T("keys.type_to_search")) {
		t.Errorf("footer still advertises the search bindings in the detail view: %q", footer)
	}

	// Esc must return to the list in one press: with searchMode set it only
	// left search mode and the detail stayed open.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.detailMode {
		t.Fatalf("Esc did not leave the detail view")
	}
	if m.currentView != ViewPods {
		t.Fatalf("Esc returned to %v, want the pods list", m.currentView)
	}

	// The filter text the user typed is kept, so returning to the list still
	// shows the filtered result.
	if m.searchText != "w" {
		t.Errorf("search text = %q after leaving the detail, want it preserved as %q", m.searchText, "w")
	}
	filtered := m.getFilteredPods()
	if len(filtered) != len(m.clusterData.Pods) {
		t.Errorf("the filter stopped applying: %d of %d pods, want all %d",
			len(filtered), len(m.clusterData.Pods), len(m.clusterData.Pods))
	}

	// `q` is a quit key again on the list.
	m.quitting = false
	if _, cmd := m.Update(keyMsg("q")); cmd == nil {
		t.Fatalf("q on the list did not quit after returning from the detail")
	}
}

// TestCtrlCQuitsFromEveryMode is the W1-3 regression guard: the search
// interception block ran before the quit binding, so Ctrl+C in search mode was
// swallowed — it neither quit nor cancelled the search. Ctrl+C must quit from
// every mode; `q` keeps its per-mode meaning (it types a character in search).
func TestCtrlCQuitsFromEveryMode(t *testing.T) {
	pod := &model.PodData{Name: "web-0", Namespace: "default", Phase: "Running",
		ContainerStates: []model.ContainerState{{Name: "app"}}}

	cases := []struct {
		name  string
		setup func(m *Model)
	}{
		{"list search", func(m *Model) {
			m.currentView = ViewPods
			m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{}, Pods: []*model.PodData{pod}}
			m.Update(keyMsg("/"))
		}},
		{"logs search", func(m *Model) {
			m.logsMode = true
			m.logsSearchMode = true
			m.selectedPod = pod
		}},
		{"action menu", func(m *Model) {
			m.detailMode = true
			m.currentView = ViewPodDetail
			m.selectedPod = pod
			m.actionMenuMode = true
		}},
		{"command output", func(m *Model) {
			m.detailMode = true
			m.currentView = ViewPodDetail
			m.selectedPod = pod
			m.commandOutputMode = true
			m.commandOutputContent = "yaml"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			tc.setup(m)

			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			if cmd == nil {
				t.Fatalf("Ctrl+C returned no command in %s mode", tc.name)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("Ctrl+C in %s mode did not quit (cmd produced %T)", tc.name, cmd())
			}
			if !m.quitting {
				t.Errorf("Ctrl+C in %s mode left quitting false", tc.name)
			}
		})
	}

	// `q` keeps typing a character in search mode.
	m := newTestModel(t)
	m.currentView = ViewPods
	m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{}, Pods: []*model.PodData{pod}}
	m.Update(keyMsg("/"))
	if _, cmd := m.Update(keyMsg("q")); cmd != nil {
		t.Errorf("q in search mode returned a command; it must stay a text input")
	}
	if m.searchText != "q" {
		t.Errorf("q in search mode was not typed: searchText = %q, want %q", m.searchText, "q")
	}
}

// TestCommandOutputOwnsActionsKey is the W1-4 regression guard: `a` opened the
// action menu while the command output overlay was on screen, so the menu was
// hidden behind the output and Enter executed the highlighted action blind.
func TestCommandOutputOwnsActionsKey(t *testing.T) {
	m := newTestModel(t)
	m.detailMode = true
	m.currentView = ViewPodDetail
	m.selectedPod = &model.PodData{Name: "web-0", Namespace: "default",
		ContainerStates: []model.ContainerState{{Name: "app"}}}
	m.Update(commandOutputMsg{title: "describe", content: "yaml"})
	if !m.commandOutputMode {
		t.Fatalf("commandOutputMsg did not enter the output mode")
	}

	m.Update(keyMsg("a"))
	if m.actionMenuMode {
		t.Fatalf("a opened the action menu under the command output overlay")
	}
	if len(m.getActionMenuItems()) == 0 {
		t.Fatalf("the pod detail has no actions, the probe would be vacuous")
	}
}

// TestCommandOutputPageKeysScrollOutput is the W1-5 regression guard: PgUp and
// PgDn fell through to the detail branch underneath the output overlay, while
// the footer advertised "PgUp/PgDn" for the output. They must move the output
// window and leave the detail's own scroll offset alone.
func TestCommandOutputPageKeysScrollOutput(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 24
	m.detailMode = true
	m.currentView = ViewPodDetail
	m.selectedPod = &model.PodData{Name: "web-0", Namespace: "default",
		ContainerStates: []model.ContainerState{{Name: "app"}}, Containers: 1}

	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "output line " + strings.Repeat("x", i%20)
	}
	m.Update(commandOutputMsg{title: "describe", content: strings.Join(lines, "\n")})
	m.commandOutputScroll = 0
	m.detailScrollOffset = 5

	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})

	if m.commandOutputScroll == 0 {
		t.Fatalf("PgDn in the command output did not scroll it (offset stayed 0)")
	}
	if m.detailScrollOffset != 5 {
		t.Errorf("PgDn in the command output moved the detail underneath: detailScrollOffset = %d, want 5",
			m.detailScrollOffset)
	}

	after := m.commandOutputScroll
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.commandOutputScroll >= after {
		t.Errorf("PgUp in the command output did not scroll it back: %d -> %d", after, m.commandOutputScroll)
	}
	if m.detailScrollOffset != 5 {
		t.Errorf("PgUp in the command output moved the detail underneath: detailScrollOffset = %d, want 5",
			m.detailScrollOffset)
	}
}

// TestShowEventsKeepsOriginClear is the W1-6 regression guard: the ShowEvents
// action left fromJobDetail/fromVolcanoJobDetail set, so a later pod detail
// opened from the plain pods list believed it came from a job and Esc jumped to
// the job detail instead of the pods list.
func TestShowEventsKeepsOriginClear(t *testing.T) {
	// A pod that belongs to job-0 and a job whose detail lists it.
	newModel := func(t *testing.T) *Model {
		t.Helper()
		m := newTestModel(t)
		m.currentView = ViewWorkloads
		m.clusterData = &model.ClusterData{
			Summary: &model.ClusterSummary{},
			Pods: []*model.PodData{{
				Name: "job-0-abc", Namespace: "default", Node: "node-1", Phase: "Running",
				ContainerStates: []model.ContainerState{{Name: "app"}},
			}},
			Events: []*model.EventData{{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/job-0-abc"}},
			Jobs:   []*model.JobData{{Name: "job-0", Namespace: "default"}},
		}
		return m
	}

	// showEventsFromPodDetail drives: workloads → job detail → pod detail →
	// action menu → Show Events.
	showEventsFromPodDetail := func(t *testing.T, m *Model) {
		t.Helper()
		m.View() // the renderer reports the workload sections Update navigates by
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.currentView != ViewJobDetail {
			t.Fatalf("Enter on the workloads list opened %v, want the job detail", m.currentView)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.currentView != ViewPodDetail || !m.fromJobDetail {
			t.Fatalf("Enter on the job detail opened %v (fromJobDetail=%v), want the pod detail from the job",
				m.currentView, m.fromJobDetail)
		}

		m.Update(keyMsg("a"))
		items := m.getActionMenuItems()
		idx := -1
		for i, item := range items {
			if item.Action == ActionShowEvents {
				idx = i
			}
		}
		if idx < 0 {
			t.Fatalf("the pod action menu has no Show Events item")
		}
		for i := 0; i < idx; i++ {
			m.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.currentView != ViewEvents {
			t.Fatalf("Show Events opened %v, want the events view", m.currentView)
		}
	}

	t.Run("fromJobDetail cleared", func(t *testing.T) {
		m := newModel(t)
		showEventsFromPodDetail(t, m)

		if m.fromJobDetail {
			t.Errorf("fromJobDetail is still set after Show Events; a pod detail opened from the pods list would Esc into the job detail")
		}

		// Enter a pod from the plain list and Esc back: it must return to the
		// pods list, not to the job detail.
		m.currentView = ViewPods
		m.selectedIndex = 0
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.currentView != ViewPodDetail {
			t.Fatalf("Enter on the pods list opened %v, want the pod detail", m.currentView)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.currentView != ViewPods {
			t.Fatalf("Esc from the pod detail returned to %v, want the pods list", m.currentView)
		}
	})

	// ShowEvents lands on the events top-level list with detailMode already
	// false, so an immediate Esc has no layer to pop: it must leave every bit
	// of navigation state untouched.
	t.Run("esc right after show events is a no-op", func(t *testing.T) {
		m := newModel(t)
		showEventsFromPodDetail(t, m)

		before := *m
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})

		if m.currentView != before.currentView {
			t.Errorf("Esc after Show Events moved to %v, want to stay on %v", m.currentView, before.currentView)
		}
		if m.detailMode != before.detailMode || m.fromJobDetail != before.fromJobDetail ||
			m.fromVolcanoJobDetail != before.fromVolcanoJobDetail || m.fromNetworkView != before.fromNetworkView {
			t.Errorf("Esc after Show Events changed mode flags: detailMode=%v fromJobDetail=%v fromVolcanoJobDetail=%v fromNetworkView=%v",
				m.detailMode, m.fromJobDetail, m.fromVolcanoJobDetail, m.fromNetworkView)
		}
		if m.selectedIndex != before.selectedIndex {
			t.Errorf("Esc after Show Events moved the selection %d → %d", before.selectedIndex, m.selectedIndex)
		}
	})
}

// TestShowEventsFromVolcanoJobKeepsOriginClear is the VolcanoJob half of W1-6:
// the volcano job detail sets fromVolcanoJobDetail on the way to the pod
// detail, and Show Events must clear it the same way.
func TestShowEventsFromVolcanoJobKeepsOriginClear(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewWorkloads
	m.clusterData = &model.ClusterData{
		Summary: &model.ClusterSummary{},
		Pods: []*model.PodData{{
			Name: "vj-0-worker-0", Namespace: "default", Node: "node-1", Phase: "Running",
			ContainerStates: []model.ContainerState{{Name: "app"}},
		}},
		Events:      []*model.EventData{{Type: "Warning", Reason: "Failed", InvolvedObject: "Pod/vj-0-worker-0"}},
		VolcanoJobs: []*model.VolcanoJobData{{Name: "vj-0", Namespace: "default"}},
	}

	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewVolcanoJobDetail {
		t.Fatalf("Enter on the workloads list opened %v, want the volcano job detail", m.currentView)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewPodDetail || !m.fromVolcanoJobDetail {
		t.Fatalf("Enter on the volcano job detail opened %v (fromVolcanoJobDetail=%v), want the pod detail from the volcano job",
			m.currentView, m.fromVolcanoJobDetail)
	}

	m.Update(keyMsg("a"))
	idx := -1
	for i, item := range m.getActionMenuItems() {
		if item.Action == ActionShowEvents {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("the pod action menu has no Show Events item")
	}
	for i := 0; i < idx; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewEvents {
		t.Fatalf("Show Events opened %v, want the events view", m.currentView)
	}
	if m.fromVolcanoJobDetail {
		t.Errorf("fromVolcanoJobDetail is still set after Show Events")
	}

	m.currentView = ViewPods
	m.selectedIndex = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewPodDetail {
		t.Fatalf("Enter on the pods list opened %v, want the pod detail", m.currentView)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.currentView != ViewPods {
		t.Fatalf("Esc from the pod detail returned to %v, want the pods list", m.currentView)
	}
}

// timeNow is shared by the probes that need a stable timestamp.
func timeNow() time.Time { return time.Now() }

// modelForTest builds a model directly, bypassing newTestModel, for probes that
// need a non-English locale next to the shared helpers.
func modelForTest(t *testing.T, locale string) *Model {
	t.Helper()
	return NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
}
