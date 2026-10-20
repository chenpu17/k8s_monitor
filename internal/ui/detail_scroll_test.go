package ui

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// The probes in this file measure the detail scroll behaviour against the
// rendered output instead of against the implementation: probeMaxScroll brute
// forces the renderers to find where the content actually ends, so the tests
// stay valid whatever formula a renderer uses to clamp its window.

// probeKey canonicalises a rendered window for comparison. NOTE: it is the same
// algorithm as the implementation's detailWindowKey (split + sort + join), so on
// its own it does NOT give the probes an independent oracle - a bug that changed
// both would be invisible here. What keeps the probes honest is that they never
// call detailScrollLimit/detailScrollTarget: the reachable offset is derived by
// brute-force scanning the renderer's own output (probeMaxScroll /
// probeDisplayedOffset), and TestDetailScrollPVCMatchesRendererIndicator parses
// the footer the renderer itself prints. The line sort makes the comparison
// order-insensitive; since the D4 fix sorted every rendered map iteration, it is
// no longer load-bearing, but it is kept so the probes cannot go flaky if an
// unsorted map iteration is reintroduced.
func probeKey(content string) string {
	lines := strings.Split(content, "\n")
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// probeMaxScroll returns the largest offset the current detail renderer honours,
// measured by brute force on the rendered output: the first k whose window stops
// changing is the end of the scrollable content.
func probeMaxScroll(m *Model) int {
	for k := 0; k < 5000; k++ {
		if probeKey(m.renderDetailAt(k)) == probeKey(m.renderDetailAt(k+1)) {
			return k
		}
	}
	return -1
}

// probeDisplayedOffset returns the offset the renderer actually displays for the
// requested offset. render is injective below the end of the content, so the
// displayed offset is the unique k in [0,maxScroll] whose window equals the
// window rendered for the requested offset.
func probeDisplayedOffset(m *Model, offset, maxScroll int) int {
	want := probeKey(m.renderDetailAt(offset))
	for k := 0; k <= maxScroll; k++ {
		if probeKey(m.renderDetailAt(k)) == want {
			return k
		}
	}
	return -1
}

// longPodDetail fills the model with a pod detail view long enough to scroll.
func longPodDetail(m *Model, containers int) *model.PodData {
	pod := &model.PodData{Name: "web-0", Namespace: "default", Node: "node-1", Phase: "Running"}
	for i := 0; i < containers; i++ {
		pod.ContainerStates = append(pod.ContainerStates, model.ContainerState{
			Name:  "container-" + string(rune('a'+i)),
			Image: "example.com/app:v1",
			State: "Running",
		})
	}
	m.selectedPod = pod
	return pod
}

// TestDetailScrollReachesLastLine checks that scrolling down can actually reach
// the bottom page of the content. The renderer clamps any offset past the end to
// maxScroll, so maxScroll is the window a user reaches with the old unbounded
// `detailScrollOffset++`; the probe-based bound must not stop one line short of
// it, which would leave the last line of the content unreadable.
func TestDetailScrollReachesLastLine(t *testing.T) {
	cases := []struct {
		name   string
		height int
		setup  func(m *Model)
	}{
		{
			name:   "pod detail",
			height: 12,
			setup: func(m *Model) {
				longPodDetail(m, 12)
			},
		},
		{
			name:   "node detail",
			height: 20,
			setup: func(m *Model) {
				m.selectedNode = &model.NodeData{
					Name: "node-1", InternalIP: "10.0.0.100", Status: "Ready",
					CPUAllocatable: 8000, CPUUsage: 2000,
					MemAllocatable: 16 * 1024 * 1024 * 1024, MemoryUsage: 4 * 1024 * 1024 * 1024,
					PodAllocatable: 110, PodCount: 8,
				}
				pods := make([]*model.PodData, 0, 8)
				for i := 0; i < 8; i++ {
					pods = append(pods, &model.PodData{
						Name: "web-" + string(rune('a'+i)), Namespace: "default",
						Node: "node-1", Phase: "Running",
					})
				}
				m.clusterData = &model.ClusterData{Pods: pods}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.height = tc.height
			m.currentView = map[string]ViewType{"pod detail": ViewPodDetail, "node detail": ViewNodeDetail}[tc.name]
			m.detailMode = true
			tc.setup(m)

			maxScroll := probeMaxScroll(m)
			if maxScroll < 3 {
				t.Fatalf("probe content only scrolls to %d, need a longer view", maxScroll)
			}

			for i := 0; i < 300; i++ {
				m.Update(keyMsg("j"))
				m.View()
			}
			// Also try paging, which takes a different branch of detailScrollBy.
			for i := 0; i < 20; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				m.View()
			}

			bottomPage := probeKey(m.renderDetailAt(maxScroll))
			current := probeKey(m.renderDetailAt(m.detailScrollOffset))
			if current != bottomPage {
				t.Errorf("after scrolling down the model sits at offset %d (maxScroll=%d): the bottom page is unreachable by ↓/PgDn",
					m.detailScrollOffset, maxScroll)
			}
		})
	}
}

// TestDetailScrollLimitMatchesDisplayedOffset checks the contract stated in the
// doc comment of detailScrollLimit: it must return the offset the detail
// renderer actually displays for the given (possibly dangling) offset. The
// dangling case is the one that matters: a refresh can shorten the content while
// detailScrollOffset still points past the new end.
func TestDetailScrollLimitMatchesDisplayedOffset(t *testing.T) {
	m := newTestModel(t)
	m.height = 12
	m.currentView = ViewPodDetail
	m.detailMode = true
	longPodDetail(m, 12)

	maxScroll := probeMaxScroll(m)
	if maxScroll < 3 {
		t.Fatalf("probe content only scrolls to %d, need a longer view to test the boundary", maxScroll)
	}

	for _, offset := range []int{0, 1, maxScroll - 1, maxScroll, maxScroll + 1, maxScroll + 2, maxScroll + 50, maxScroll + 5000} {
		got := m.detailScrollLimit(offset)
		want := probeDisplayedOffset(m, offset, maxScroll)
		if want < 0 {
			t.Fatalf("offset %d: renderer window does not match any offset in [0,%d]", offset, maxScroll)
		}
		if got != want {
			t.Errorf("detailScrollLimit(%d) = %d, but the renderer displays offset %d (maxScroll=%d)",
				offset, got, want, maxScroll)
		}
	}
}

// TestDetailScrollUpStepAfterShrink measures how far a single ↑ moves the view
// once the content has shrunk under a dangling offset. The intended behaviour is
// a one-line step; anything larger is a jump the user did not ask for.
func TestDetailScrollUpStepAfterShrink(t *testing.T) {
	m := newTestModel(t)
	m.height = 12
	m.currentView = ViewPodDetail
	m.detailMode = true
	pod := longPodDetail(m, 12)

	// Scroll to the end of the long content.
	for i := 0; i < 300; i++ {
		m.Update(keyMsg("j"))
	}
	longMax := probeMaxScroll(m)
	longDisplayed := probeDisplayedOffset(m, m.detailScrollOffset, longMax)

	// The pod loses most of its containers: the content gets much shorter, so the
	// model offset now dangles past the new end.
	m.selectedPod = &model.PodData{
		Name: pod.Name, Namespace: pod.Namespace, Node: pod.Node, Phase: pod.Phase,
		ContainerStates: pod.ContainerStates[:3],
	}
	shortMax := probeMaxScroll(m)
	if shortMax >= longMax {
		t.Fatalf("content did not shrink: long maxScroll=%d, short maxScroll=%d", longMax, shortMax)
	}

	displayedBefore := probeDisplayedOffset(m, m.detailScrollOffset, shortMax)

	m.Update(keyMsg("k"))
	displayedAfter := probeDisplayedOffset(m, m.detailScrollOffset, shortMax)

	step := displayedBefore - displayedAfter
	if step < 1 {
		t.Errorf("one ↑ moved the view by %d lines (from %d to %d), want a forward move",
			step, displayedBefore, displayedAfter)
	}
	if step > 1 {
		t.Errorf("one ↑ moved the view by %d lines (from %d to %d), want exactly 1",
			step, displayedBefore, displayedAfter)
	}
	t.Logf("longMaxScroll=%d (displayed %d) shortMaxScroll=%d: one ↑ stepped %d line",
		longMax, longDisplayed, shortMax, step)
}

// TestDetailScrollDoesNotLockDuplicatedLines checks the conservative direction of
// the probe-based bottom detection: a view whose first rendered windows are
// identical must not lock content that is still scrollable. The probe may stop
// early, but it must never make reachable content unreachable.
func TestDetailScrollDoesNotLockDuplicatedLines(t *testing.T) {
	m := newTestModel(t)
	m.height = 12
	m.currentView = ViewPodDetail
	m.detailMode = true

	pod := &model.PodData{Name: "web-0", Namespace: "default", Node: "node-1", Phase: "Running"}
	for i := 0; i < 40; i++ {
		pod.ContainerStates = append(pod.ContainerStates, model.ContainerState{
			Name:  "container-" + string(rune('a'+i%26)),
			Image: "example.com/app:v1",
			State: "Running",
		})
	}
	m.selectedPod = pod

	maxScroll := probeMaxScroll(m)

	// Walk down one line at a time and record how far the view actually travelled.
	reached := 0
	for i := 0; i < maxScroll+20; i++ {
		before := probeKey(m.renderDetailAt(m.detailScrollOffset))
		m.Update(keyMsg("j"))
		if probeKey(m.renderDetailAt(m.detailScrollOffset)) == before {
			break
		}
		reached = probeDisplayedOffset(m, m.detailScrollOffset, maxScroll)
	}

	if reached == 0 {
		t.Fatalf("repeated ↓ never moved the view although maxScroll=%d", maxScroll)
	}
	if reached != maxScroll {
		t.Errorf("↓ reached offset %d of maxScroll %d", reached, maxScroll)
	}
}

// TestDetailScrollMapOrderedContent covers the detail views whose content is
// built from maps (labels, annotations, selectors). Those renderers used to walk
// the map directly, so every render produced a different window - the labels
// block reshuffled on every refresh, and the scroll clamp had no deterministic
// view to compare against, which left the offset drifting past the end of the
// content and a single ↑ doing nothing. The renderers now sort their keys; this
// test pins both halves of that: the window is stable across renders, and the
// clamp lands exactly on the bottom page.
func TestDetailScrollMapOrderedContent(t *testing.T) {
	cases := []struct {
		name   string
		view   ViewType
		height int
		setup  func(m *Model)
	}{
		{
			name: "pvc detail", view: ViewPVCDetail, height: 20,
			setup: func(m *Model) {
				m.selectedPVC = &model.PVCData{
					Name: "data-0", Namespace: "default", Status: "Bound", Volume: "pvc-1234",
					Capacity: 8 << 30, RequestedStorage: 8 << 30, StorageClass: "standard",
					AccessModes: []string{"ReadWriteOnce"}, VolumeMode: "Filesystem",
					Labels: mapEntries(6, "example.com/label-key-"), Annotations: mapEntries(6, "example.com/anno-key-"),
				}
			},
		},
		{
			name: "pv detail", view: ViewPVDetail, height: 20,
			setup: func(m *Model) {
				m.selectedPV = &model.PVData{
					Name: "pv-0", Capacity: 8 << 30, StorageClass: "standard",
					AccessModes: []string{"ReadWriteOnce"}, Status: "Bound",
					Labels: mapEntries(6, "example.com/label-key-"), Annotations: mapEntries(6, "example.com/anno-key-"),
				}
			},
		},
		{
			name: "service detail", view: ViewServiceDetail, height: 12,
			setup: func(m *Model) {
				m.selectedService = &model.ServiceData{
					Name: "svc-0", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.0.0.1",
					Selector: mapEntries(6, "app-"), Labels: mapEntries(6, "example.com/label-key-"),
					Annotations: mapEntries(6, "example.com/anno-key-"),
				}
			},
		},
		{
			name: "deployment detail", view: ViewDeploymentDetail, height: 20,
			setup: func(m *Model) {
				m.selectedDeployment = &model.DeploymentData{
					Name: "dep-0", Namespace: "default", Replicas: 3, ReadyReplicas: 3, AvailableReplicas: 3,
					Selector: mapEntries(6, "app-"), Labels: mapEntries(6, "example.com/label-key-"),
					Annotations: mapEntries(6, "example.com/anno-key-"),
				}
			},
		},
		{
			name: "statefulset detail", view: ViewStatefulSetDetail, height: 20,
			setup: func(m *Model) {
				m.selectedStatefulSet = &model.StatefulSetData{
					Name: "sts-0", Namespace: "default", Replicas: 3, ReadyReplicas: 3,
					Selector: mapEntries(6, "app-"), Labels: mapEntries(6, "example.com/label-key-"),
				}
			},
		},
		{
			name: "daemonset detail", view: ViewDaemonSetDetail, height: 20,
			setup: func(m *Model) {
				m.selectedDaemonSet = &model.DaemonSetData{
					Name: "ds-0", Namespace: "default", DesiredNumberScheduled: 3, CurrentNumberScheduled: 3,
					Selector: mapEntries(6, "app-"), Labels: mapEntries(6, "example.com/label-key-"),
				}
			},
		},
		{
			name: "cronjob detail", view: ViewCronJobDetail, height: 20,
			setup: func(m *Model) {
				m.selectedCronJob = &model.CronJobData{
					Name: "cj-0", Namespace: "default", Schedule: "*/5 * * * *",
					Labels: mapEntries(6, "example.com/label-key-"),
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

			maxScroll := probeMaxScroll(m)
			if maxScroll < 3 {
				t.Fatalf("fixture does not scroll (maxScroll=%d), the test would be vacuous", maxScroll)
			}

			// Every offset must render the same window twice in a row: that is
			// what the clamp compares, and what the user sees on every refresh.
			for offset := 0; offset <= maxScroll+3; offset++ {
				if m.renderDetailAt(offset) != m.renderDetailAt(offset) {
					t.Fatalf("offset %d renders differently on every render: the view is non-deterministic", offset)
				}
			}

			for i := 0; i < 300; i++ {
				m.Update(keyMsg("j"))
			}
			if m.detailScrollOffset != maxScroll {
				t.Fatalf("offset after 300 ↓ = %d, want the bottom page %d", m.detailScrollOffset, maxScroll)
			}

			bottomPage := probeKey(m.renderDetailAt(maxScroll))
			if got := probeKey(m.renderDetailAt(m.detailScrollOffset)); got != bottomPage {
				t.Fatalf("the view does not show the bottom page after scrolling to the end")
			}

			m.Update(keyMsg("k"))
			if got := probeKey(m.renderDetailAt(m.detailScrollOffset)); got == bottomPage {
				t.Fatalf("one ↑ did not move the view off the bottom page (offset=%d)", m.detailScrollOffset)
			}
		})
	}
}

// TestDetailScrollPVCMatchesRendererIndicator cross-checks the clamp against an
// oracle the probe does not use: the pvc renderer's own scroll indicator prints
// the offset it clamped to, so a probe that stops short of (or past) the real
// end shows up as a disagreement.
func TestDetailScrollPVCMatchesRendererIndicator(t *testing.T) {
	m := newTestModel(t)
	m.height = 20
	m.currentView = ViewPVCDetail
	m.detailMode = true
	m.selectedPVC = &model.PVCData{
		Name: "data-0", Namespace: "default", Status: "Bound", Volume: "pvc-1234",
		Capacity: 8 << 30, RequestedStorage: 8 << 30, StorageClass: "standard",
		AccessModes: []string{"ReadWriteOnce"}, VolumeMode: "Filesystem",
		Labels: mapEntries(6, "example.com/label-key-"), Annotations: mapEntries(6, "example.com/anno-key-"),
	}

	// The indicator reports "viewing <start>-<end> of <total> lines", so the
	// offset the renderer clamped to is start-1.
	var start, end, total int
	for _, line := range strings.Split(m.renderDetailAt(1<<20), "\n") {
		if n, _ := fmt.Sscanf(stripANSI(line), "(viewing %d-%d of %d lines", &start, &end, &total); n == 3 {
			break
		}
	}
	if start == 0 {
		t.Fatalf("could not read the scroll indicator out of the pvc detail window")
	}
	rendererMaxScroll := start - 1

	if got := probeMaxScroll(m); got != rendererMaxScroll {
		t.Errorf("probe says maxScroll=%d, the renderer reports %d (window %d-%d of %d lines)",
			got, rendererMaxScroll, start, end, total)
	}

	for i := 0; i < 200; i++ {
		m.Update(keyMsg("j"))
	}
	if m.detailScrollOffset != rendererMaxScroll {
		t.Errorf("offset after 200 ↓ = %d, want the renderer's bottom page %d", m.detailScrollOffset, rendererMaxScroll)
	}
}

// mapEntries builds a labels-style map with n entries.
func mapEntries(n int, prefix string) map[string]string {
	entries := make(map[string]string, n)
	for i := 0; i < n; i++ {
		entries[prefix+string(rune('a'+i))] = "value"
	}
	return entries
}

// TestDetailScrollBoundaries covers the edges of the valid-offset prefix: offset
// 0 is always valid, the bottom page maxScroll is the last valid offset, and
// content that fits in one screen (maxScroll 0) must not scroll at all.
func TestDetailScrollBoundaries(t *testing.T) {
	t.Run("content fits one screen", func(t *testing.T) {
		m := newTestModel(t)
		m.currentView = ViewPodDetail
		m.detailMode = true
		m.selectedPod = &model.PodData{Name: "web-0", Namespace: "default", Node: "node-1", Phase: "Running"}

		if maxScroll := probeMaxScroll(m); maxScroll != 0 {
			t.Fatalf("test content scrolls to %d, want a view that fits one screen", maxScroll)
		}
		for i := 0; i < 3; i++ {
			m.Update(keyMsg("j"))
		}
		if m.detailScrollOffset != 0 {
			t.Errorf("↓ moved a view that fits one screen to offset %d", m.detailScrollOffset)
		}
		// A dangling offset with no content to scroll to resolves to 0.
		if got := m.detailScrollLimit(5); got != 0 {
			t.Errorf("detailScrollLimit(5) = %d on a single-screen view, want 0", got)
		}
		m.detailScrollOffset = 5
		m.Update(keyMsg("k"))
		if m.detailScrollOffset != 0 {
			t.Errorf("↑ from a dangling offset landed on %d, want 0", m.detailScrollOffset)
		}
	})

	t.Run("bottom page and back", func(t *testing.T) {
		m := newTestModel(t)
		m.height = 12
		m.currentView = ViewPodDetail
		m.detailMode = true
		longPodDetail(m, 12)

		maxScroll := probeMaxScroll(m)
		if maxScroll < 2 {
			t.Fatalf("probe content only scrolls to %d, need a longer view", maxScroll)
		}

		for i := 0; i < maxScroll; i++ {
			m.Update(keyMsg("j"))
		}
		if m.detailScrollOffset != maxScroll {
			t.Fatalf("offset after %d ↓ = %d, want the bottom page %d", maxScroll, m.detailScrollOffset, maxScroll)
		}

		for i := 0; i < maxScroll; i++ {
			m.Update(keyMsg("k"))
		}
		if m.detailScrollOffset != 0 {
			t.Fatalf("offset after %d ↑ = %d, want 0", maxScroll, m.detailScrollOffset)
		}
		m.Update(keyMsg("k"))
		if m.detailScrollOffset != 0 {
			t.Fatalf("↑ scrolled past the first line: offset = %d", m.detailScrollOffset)
		}
	})
}

// TestProbeViewSwitchKeysInOtherModes records what the digit/Tab view-switch keys
// do in the modes that were NOT given a filterMode-style guard. detailMode is
// left false so that only the mode under test can block the shortcut.
func TestProbeViewSwitchKeysInOtherModes(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Model)
	}{
		{"filterMode (guarded)", func(m *Model) { m.filterMode = true }},
		{"searchMode (guarded by search handler)", func(m *Model) { m.searchMode = true }},
		{"logsMode", func(m *Model) { m.logsMode = true }},
		{"commandOutputMode", func(m *Model) { m.commandOutputMode = true }},
		{"actionMenuMode", func(m *Model) { m.actionMenuMode = true }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.currentView = ViewPods
			m.detailMode = false
			tc.setup(m)

			m.Update(keyMsg("2"))
			switchedByDigit := m.currentView != ViewPods

			m2 := newTestModel(t)
			m2.currentView = ViewPods
			m2.detailMode = false
			tc.setup(m2)
			m2.Update(tea.KeyMsg{Type: tea.KeyTab})
			switchedByTab := m2.currentView != ViewPods

			t.Logf("%s: digit switched view = %v, tab switched view = %v (view after digit=%v)",
				tc.name, switchedByDigit, switchedByTab, m.currentView)
		})
	}
}
