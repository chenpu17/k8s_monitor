package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// Regression: bubbletea merges burst-typed or pasted characters into a single
// multi-rune KeyMsg. The old `len(msg.String()) == 1` checks dropped such
// input entirely, so pasting into a search box did nothing.

func TestSearchAcceptsPastedText(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewPods
	m.searchMode = true

	m.Update(keyMsg("node-exporter"))

	if m.searchText != "node-exporter" {
		t.Fatalf("searchText = %q, want %q", m.searchText, "node-exporter")
	}
}

func TestSearchKeepsNameCharsetOnly(t *testing.T) {
	m := newTestModel(t)
	m.currentView = ViewPods
	m.searchMode = true

	m.Update(keyMsg("kube-system_99.a+b c"))

	if m.searchText != "kube-system_99.abc" {
		t.Fatalf("searchText = %q, want %q", m.searchText, "kube-system_99.abc")
	}
}

func TestLogsSearchAcceptsPastedText(t *testing.T) {
	m := newTestModel(t)
	m.logsSearchMode = true

	m.Update(keyMsg("ERROR"))
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(keyMsg("timeout"))

	if m.logsSearchText != "ERROR timeout" {
		t.Fatalf("logsSearchText = %q, want %q", m.logsSearchText, "ERROR timeout")
	}
}

// Regression: a container with no output used to render the loading state
// forever, because an empty result was indistinguishable from "not fetched
// yet".

func newLogsModel(t *testing.T) *Model {
	t.Helper()
	m := newTestModel(t)
	m.logsMode = true
	m.selectedPod = &model.PodData{Namespace: "ns", Name: "pod"}
	m.selectedContainer = "c"
	return m
}

func TestLogsEmptyFetchShowsEmptyState(t *testing.T) {
	m := newLogsModel(t)

	m.Update(logsMsg{logs: ""})

	if !m.logsFetched {
		t.Fatal("logsFetched = false after a successful fetch, want true")
	}
	view := m.View()
	if !strings.Contains(view, "No logs for this container") {
		t.Fatalf("view does not show the empty state:\n%s", view)
	}
	if strings.Contains(view, "Loading logs...") {
		t.Fatal("view still shows the loading state after a successful fetch")
	}
}

func TestLogsFetchErrorKeepsLoadingStateOff(t *testing.T) {
	m := newLogsModel(t)

	m.Update(logsMsg{err: errors.New("boom")})

	if m.logsFetched {
		t.Fatal("logsFetched = true after a failed fetch, want false")
	}
	if view := m.View(); !strings.Contains(view, "Error fetching logs: boom") {
		t.Fatalf("view does not show the error:\n%s", view)
	}
}

func TestLogsBeforeFirstFetchShowsLoading(t *testing.T) {
	m := newLogsModel(t)

	if view := m.View(); !strings.Contains(view, "Loading logs...") {
		t.Fatalf("view does not show the loading state before the first fetch:\n%s", view)
	}
}
