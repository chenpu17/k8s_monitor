package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
	"go.uber.org/zap"
)

// WP2 probes: the render budgets of the scrolling viewers. fitFrame trims
// content that overflows the content area silently, so a frame-height check
// cannot see a view that renders one line too many — the symptom is the tail
// (a status line, the scroll indicator) disappearing. Every probe here asserts
// on the tail through the full frame, at several terminal heights.

// numberedLines returns n single-line log/output rows.
func numberedLines(prefix string, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s %03d", prefix, i+1)
	}
	return lines
}

// commandOutputModel returns a model showing n lines of command output.
func commandOutputModel(t *testing.T, height, n int) *Model {
	t.Helper()

	m := modelForTest(t, "en")
	m.width = 120
	m.height = height
	m.commandOutputMode = true
	m.commandOutputTitle = "describe"
	m.commandOutputContent = strings.Join(numberedLines("output line", n), "\n")
	return m
}

// TestCommandOutputBottomPageShowsLastLineAndStatus is the W2-1 regression
// guard: the viewer reserved only six lines for chrome that actually occupies
// ten, so its content was always four lines taller than the content area and
// fitFrame trimmed the bottom — the position line and the last output rows
// never appeared, at any terminal height.
func TestCommandOutputBottomPageShowsLastLineAndStatus(t *testing.T) {
	for _, height := range []int{10, 24, 40, 60} {
		t.Run(fmt.Sprintf("h%d", height), func(t *testing.T) {
			m := commandOutputModel(t, height, 200)

			// Scroll to the bottom with the key a user has, rendering after
			// each press like the Bubble Tea loop does.
			for i := 0; i < 400; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				m.View()
			}
			for i := 0; i < 20; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m.View()
			}

			lines := frameLines(m)
			if len(lines) > height {
				t.Fatalf("frame is %d lines tall on a %d-line terminal:\n%s",
					len(lines), height, strings.Join(lines, "\n"))
			}
			if !containsSubstring(lines, "output line 200") {
				t.Errorf("the last output line is not on screen after scrolling to the bottom:\n%s",
					strings.Join(lines, "\n"))
			}
			// The position line is the tail the renderer appends; its window
			// must end at the last line.
			if !containsSubstring(lines, "200 of 200") {
				t.Errorf("the position line is not on screen after scrolling to the bottom:\n%s",
					strings.Join(lines, "\n"))
			}
		})
	}
}

// TestCommandOutputShortContentStillFits pins the other side of the budget: a
// short output has no position line, and it must not be padded or trimmed
// either.
func TestCommandOutputShortContentStillFits(t *testing.T) {
	// 10 and 24 are the heights where the chrome leaves room for the whole
	// three-line output; a 1-row content area truncating it would make the
	// assertion below vacuous.
	for _, height := range []int{24, 40, 60} {
		t.Run(fmt.Sprintf("h%d", height), func(t *testing.T) {
			m := commandOutputModel(t, height, 3)

			content := m.renderCommandOutput()
			if n := lineCount(content); n > m.contentBudget() {
				t.Fatalf("content is %d lines, the budget is %d:\n%s", n, m.contentBudget(), content)
			}
			for _, line := range numberedLines("output line", 3) {
				if !strings.Contains(content, line) {
					t.Errorf("short output lost line %q:\n%s", line, content)
				}
			}
		})
	}
}

// workloadsPerfModel builds a workloads list long enough to overflow every
// height under test. Jobs with a duration and a completion raise the job
// performance summary, which is what makes the workloads header two physical
// lines tall.
func workloadsPerfModel(t *testing.T, height int) *Model {
	t.Helper()

	m := modelForTest(t, "en")
	m.width = 200
	m.height = height
	m.currentView = ViewWorkloads
	m.clusterData = &model.ClusterData{Summary: &model.ClusterSummary{}}
	for i := 0; i < 40; i++ {
		m.clusterData.Jobs = append(m.clusterData.Jobs, &model.JobData{
			Name: fmt.Sprintf("job-%02d", i), Namespace: "default",
			Completions: 1, Succeeded: 1, Duration: 2 * time.Minute,
		})
	}
	return m
}

// TestWorkloadsScrollIndicatorVisible is the W2-2 regression guard: the header
// is appended as one element but renders two physical lines when the job
// performance summary is present, so the window was one line too tall and
// fitFrame trimmed the scroll indicator off the bottom of every workloads
// sub-view.
func TestWorkloadsScrollIndicatorVisible(t *testing.T) {
	for _, height := range []int{20, 24, 30, 40} {
		t.Run(fmt.Sprintf("h%d", height), func(t *testing.T) {
			m := workloadsPerfModel(t, height)

			header := m.renderWorkloadsHeader()
			if lineCount(header) < 2 {
				t.Fatalf("the workloads header is %d line(s) tall; the probe needs the two-line form to exercise the budget:\n%s",
					lineCount(header), header)
			}

			content := m.renderWorkloads()
			if n := lineCount(content); n > m.contentBudget() {
				t.Fatalf("content is %d lines, the budget is %d — the tail would be trimmed:\n%s",
					n, m.contentBudget(), content)
			}
			if !strings.Contains(stripANSI(content), m.T("keys.scroll")) {
				t.Fatalf("the scroll indicator is missing from the content:\n%s", stripANSI(content))
			}

			lines := frameLines(m)
			if len(lines) > height {
				t.Fatalf("frame is %d lines tall on a %d-line terminal", len(lines), height)
			}
			if !containsSubstring(lines, "showing") {
				t.Fatalf("the scroll indicator was trimmed off the frame:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
}

// TestWorkloadsShortListUnchanged pins the byte output of a workloads list that
// fits: no scroll indicator, so the budget change must not move a single
// character.
func TestWorkloadsShortListUnchanged(t *testing.T) {
	m := modelForTest(t, "en")
	m.width = 200
	m.height = 40
	m.currentView = ViewWorkloads
	m.clusterData = &model.ClusterData{
		Summary:     &model.ClusterSummary{},
		Deployments: []*model.DeploymentData{{Name: "d1", Namespace: "default", Replicas: 1, ReadyReplicas: 1}},
	}

	const want = " ⚙️  Workloads   Deployments: 1 • StatefulSets: 0 • DaemonSets: 0 • Jobs: 0 • CronJobs: 0\n" +
		"\n" +
		"Deployments  (Total: 1)\n" +
		"\n" +
		"NAME                                 NAMESPACE        READY            UP-TO-DATE    AVAILABLE   \n" +
		"> d1                                   default          1/1              0             0           \n"
	if got := m.renderWorkloads(); got != want {
		t.Errorf("short workloads list bytes changed:\n got %q\nwant %q", got, want)
	}
}

// logsBulkModel returns a model in logs mode with n log lines.
func logsBulkModel(t *testing.T, height, n int) *Model {
	t.Helper()

	m := modelForTest(t, "en")
	m.width = 120
	m.height = height
	m.logsMode = true
	m.selectedPod = &model.PodData{Name: "web-0", Namespace: "default"}
	m.selectedContainer = "app"
	lines := numberedLines("log line", n)
	m.containerLogs = strings.Join(lines, "\n")
	m.cachedLogLines = lines
	m.logsFetched = true
	return m
}

// TestLogsBottomPageShowsLastLineAndStatus is the W2-3 regression guard: the
// log window was sized height-8 while the renderer spends eight content lines
// on the title, the separators and the two-line status tail, so the rendered
// content was always two lines too tall and fitFrame trimmed the status line
// carrying the position and the auto-follow state.
func TestLogsBottomPageShowsLastLineAndStatus(t *testing.T) {
	for _, height := range []int{10, 24, 40} {
		t.Run(fmt.Sprintf("h%d", height), func(t *testing.T) {
			m := logsBulkModel(t, height, 200)

			for i := 0; i < 400; i++ {
				m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
				m.View()
			}

			content := m.renderLogs()
			if n := lineCount(content); n > m.contentBudget() {
				t.Fatalf("logs content is %d lines, the budget is %d:\n%s",
					n, m.contentBudget(), stripANSI(content))
			}
			lines := frameLines(m)
			if len(lines) > height {
				t.Fatalf("frame is %d lines tall on a %d-line terminal", len(lines), height)
			}
			if !containsSubstring(lines, "log line 200") {
				t.Errorf("the last log line is not on screen at the bottom:\n%s", strings.Join(lines, "\n"))
			}
			// Both halves of the status tail: the position window and the
			// auto-follow state.
			if !containsSubstring(lines, "200 of 200") {
				t.Errorf("the log position line is missing at the bottom:\n%s", strings.Join(lines, "\n"))
			}
			if !containsSubstring(lines, m.T("logs.paused")) && !containsSubstring(lines, m.T("logs.auto_follow")) {
				t.Errorf("the auto-follow state is missing from the status line:\n%s", strings.Join(lines, "\n"))
			}
		})
	}
}

// TestLogsSearchModePausesAutoRefresh is the behaviour the budget fix must not
// disturb: entering search freezes the auto-refresh loop.
func TestLogsSearchModePausesAutoRefresh(t *testing.T) {
	m := logsBulkModel(t, 24, 100)
	m.logsAutoRefresh = true
	m.logsSearchMode = true

	_, cmd := m.Update(logsRefreshTickMsg{})
	if cmd == nil {
		// The tick reschedules itself; there is no fetch to expect.
		return
	}
	msg := cmd()
	if _, isLogs := msg.(logsMsg); isLogs {
		t.Fatalf("a refresh tick in logs search mode fetched logs")
	}
}

// hyperNodeModel returns a node detail model whose HyperNodeID is long enough
// to overflow a narrow terminal.
func hyperNodeModel(t *testing.T, width int) *Model {
	t.Helper()

	m := modelForTest(t, "en")
	m.width = width
	m.height = 400
	m.selectedNode = &model.NodeData{
		Name: "node-a", Status: "Ready",
		CPUAllocatable: 4000, CPUUsage: 100,
		MemAllocatable: 8 << 30, MemoryUsage: 1 << 30,
		NPUCapacity: 8, NPUAllocatable: 8, NPUAllocated: 2,
		HyperNodeID: "hypernode-very-long-identifier-0123456789abcdef",
		SuperPodID:  "superpod-01",
		CabinetInfo: "cabinet-1",
	}
	return m
}

// TestNodeDetailHyperNodeIDFitsWidth is the W2-4 regression guard: the
// HyperNode ID line was capped at a fixed 36-cell value, which pushed the line
// past the right edge on terminals 38-53 cells wide.
func TestNodeDetailHyperNodeIDFitsWidth(t *testing.T) {
	for _, width := range []int{38, 44, 53, 54, 80} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			m := hyperNodeModel(t, width)
			out := stripANSI(m.renderNodeDetail())

			line := lineContaining(out, m.T("npudetail.hypernode_id"))
			if line == "" {
				t.Fatalf("the HyperNode ID line is not rendered at width %d:\n%s", width, out)
			}
			if w := runewidth.StringWidth(line); w > width {
				t.Errorf("the HyperNode ID line is %d cells wide on a %d-cell terminal: %q", w, width, line)
			}
			if !strings.Contains(line, "hypernode-") {
				t.Errorf("the HyperNode ID value is gone entirely: %q", line)
			}
		})
	}
}

// TestNodeDetailWideHyperNodeIDUnchanged pins the wide-terminal bytes: at 54
// cells and above the fixed 36-cell truncation already fit, so the adaptive
// width must not change them.
func TestNodeDetailWideHyperNodeIDUnchanged(t *testing.T) {
	const want = "    HyperNode ID: hypernode-very-long-identifier-01..."
	for _, width := range []int{54, 80, 120} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			out := stripANSI(hyperNodeModel(t, width).renderNodeDetail())
			if got := lineContaining(out, "HyperNode ID"); got != want {
				t.Errorf("wide-terminal HyperNode ID line changed at width %d:\n got %q\nwant %q", width, got, want)
			}
		})
	}
}

// modelForTest quiesces the unused zap import in files that build models
// directly; it keeps the locale wiring of NewModel visible to the probes.
var _ = zap.NewNop
