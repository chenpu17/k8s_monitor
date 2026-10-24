package ui

// b24v_surfaces_test.go — adversarial review of batch 24's UI surfaces.
//
// The batch added five display surfaces. These probes cover the states its own
// tests do not reach: a chip Health value outside the exporter's three-value
// domain (the repo has a second producer), the summary bar's "N healthy /
// M unknown" co-existence, and the fact that the knowledge-base action the
// data layer stores is never rendered.

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/chenpu/k8s-monitor/internal/diagnostic"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// TestB24VChipHealthWarningCellStyled pins how the second producer's health
// value renders in the two chip tables after the batch narrowed their colour
// switches to OK / Error / Unknown. deploy/k8s-monitor-npu-collector.yaml copies
// the `npu-smi info` health string into k8s-monitor.io/npu-metrics and
// deploy/README.md documents it as OK/Warning/Error, so "Warning" reaches both
// tables from a real producer.
//
// The basic (non-extended) table's Health column has no case for "Warning" any
// more, so the value renders unstyled: in a colour terminal it is the only cell
// in the row with no colour at all, which reads as "less alarming than OK".
func TestB24VChipHealthWarningCellStyled(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	basic := &model.NodeData{
		Name: "node-basic", Status: "Ready", NPUCapacity: 2, NPUAllocatable: 2,
		NPUHealthStatus: "Healthy",
		NPUChips: []model.NPUChipData{
			{NPUID: 0, Chip: 0, PhyID: 0, Health: "Warning", Temp: 47, Power: 150, HBMUsed: 1, HBMTotal: 2},
			{NPUID: 0, Chip: 1, PhyID: 1, Health: "OK", Temp: 47, Power: 150, HBMUsed: 1, HBMTotal: 2},
			{NPUID: 1, Chip: 0, PhyID: 2, Health: "Error", Temp: 47, Power: 150, HBMUsed: 1, HBMTotal: 2},
		},
	}
	m := NewModel(nil, nil, 0, "en", "test", 200)
	m.width, m.height = 140, 200
	m.selectedNode = basic
	out := m.renderNodeDetail()

	warnCell := warnCellOf(t, out, "Warning")
	okCell := warnCellOf(t, out, "OK")
	errCell := warnCellOf(t, out, "Error")
	t.Logf("health cells: Warning=%q OK=%q Error=%q", warnCell, okCell, errCell)

	if !strings.Contains(errCell, "\x1b[") {
		t.Errorf("the Error health cell carries no style: %q", errCell)
	}
	if !strings.Contains(okCell, "\x1b[") {
		t.Errorf("the OK health cell carries no style: %q", okCell)
	}
	if !strings.Contains(warnCell, "\x1b[") {
		t.Errorf("the collector's Warning health cell renders unstyled (no colour at all, next to a coloured OK): %q", warnCell)
	}
}

// warnCellOf returns the Health-column cell of the basic chip table row that
// carries value, style escapes included.
func warnCellOf(t *testing.T, out, value string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, value) {
			continue
		}
		plain := stripANSI(line)
		// The Health column sits between the power cell ("150.0W") and the
		// HBM cell ("1/2MB").
		if !strings.Contains(plain, "150.0W") || !strings.Contains(plain, "1/2MB") {
			continue
		}
		startIdx := strings.Index(plain, "150.0W") + len("150.0W")
		endIdx := strings.Index(plain, "1/2MB")
		if endIdx <= startIdx {
			continue
		}
		// Slice the raw line by the same rune offsets the plain line uses.
		return rawSpan(line, startIdx, endIdx)
	}
	return ""
}

// rawSpan returns the raw (escape-bearing) substring of line that corresponds
// to plain-cell span [from, to) of its stripped form.
func rawSpan(line string, from, to int) string {
	var (
		b          strings.Builder
		plainIdx   int
		inEsc      bool
		escapeSeen int
	)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == 0x1b {
			inEsc = true
		}
		if inEsc {
			b.WriteByte(c)
			if c == 'm' {
				inEsc = false
			}
			continue
		}
		if plainIdx >= from && plainIdx < to {
			b.WriteByte(c)
			escapeSeen++
		}
		plainIdx++
	}
	_ = escapeSeen
	return b.String()
}

// TestB24VUnknownSegmentsCoexistWithHealthyCount pins the truthfulness of the
// summary bar for an all-unknown node. At HEAD=44c52af the bar reported "8/8
// healthy" and "unknown 8" on the same line for the same chips, which is what
// this probe pinned then.
//
// Batch 25 changed the wording on purpose: with unknown chips in the cluster the
// health segment says "no fault" instead of "healthy", because the matrix
// grade of an unknown chip is a statement about the absence of a fault signal,
// not a health signal. The counts stay identical to the matrix tally; only the
// word changes. The bytes below are the batch-25 expectation (wording shortened
// in the fix round after review: the first cut, "no known fault", overflowed
// the 38-column bar and truncated the trailing counters).
func TestB24VUnknownSegmentsCoexistWithHealthyCount(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 200, 40)
	chips := make([]model.NPUChipData, 8)
	grades := make([]model.NPUChipGrade, 8)
	for i := range chips {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "Unknown"}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	m.clusterData.Nodes[0].NPUChips = chips
	m.clusterData.Nodes[0].NPUChipHealth = grades

	out := stripANSI(m.renderNPUView())
	bar := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "no fault ·") {
			bar = line
			break
		}
	}
	if bar == "" {
		t.Fatalf("no summary bar rendered:\n%s", out)
	}
	if !strings.Contains(bar, "8/8 no fault") || !strings.Contains(bar, "unknown 8") {
		t.Fatalf("the bar no longer shows the no-fault/unknown pair: %q", bar)
	}
	// The old wording must be gone: "healthy" next to "unknown" for the same
	// chips is the contradiction this wording change exists to remove.
	if strings.Contains(bar, "8/8 healthy") {
		t.Errorf("the bar still claims health for chips it reports as unknown: %q", bar)
	}
	t.Logf("all-unknown node bar: %q", bar)
}

// TestB24VKBAlertActionRendered pins the W3-2 wiring at the display boundary.
// The review found the alerts view recomputing its "→ action" line from the
// alert type (alerts.go), never reading alert.RecommendedAction, so the
// knowledge-base action the datasource stored reached no screen; the view now
// prefers a stored action that is actually specific (different from the alert
// type's generic English action) and keeps the locale-aware lookup otherwise.
func TestB24VKBAlertActionRendered(t *testing.T) {
	forcedAsciiProfile(t)

	const kbAction = "No action required; the fault does not affect workloads"
	m := NewModel(nil, nil, 0, "en", "test", 200)
	m.width, m.height = 200, 50
	m.currentView = ViewAlerts
	m.clusterData = &model.ClusterData{
		Nodes: []*model.NodeData{{Name: "npu-node", Status: "Ready", NPUCapacity: 8}},
		Summary: &model.ClusterSummary{Alerts: []model.Alert{{
			Severity: model.AlertSeverityWarning, Category: "NPU", AlertType: model.AlertTypeNPUErrorCode,
			ResourceType: "Node", ResourceName: "npu-node",
			Message:     "NPU reported error code on chip(s) 1 (code 2162298887)",
			MessageKey:  "alert.npu_error_code",
			MessageArgs: map[string]interface{}{"Chips": "1 (code 2162298887)"},
			Value:       "chips 1", ValueKey: "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": "1"},
			RecommendedAction: kbAction, Timestamp: time.Now(),
		}}},
	}

	out := stripANSI(m.renderAlerts())
	if !strings.Contains(out, kbAction) {
		t.Errorf("the alerts view does not render the knowledge-base action:\n%s", out)
	}
}

// TestB24VGenericAlertKeepsLocaleAction pins the guard side of the stored-action
// preference: an alert whose stored action equals the generic English one (what
// every ordinary alert carries from the data layer) must still render through
// the locale-aware lookup, or a Chinese view would flip to English.
func TestB24VGenericAlertKeepsLocaleAction(t *testing.T) {
	forcedAsciiProfile(t)

	generic := diagnostic.GetRecommendedAction(model.AlertTypeNPUErrorCode, "", "npu-node")
	m := NewModel(nil, nil, 0, "zh", "test", 200)
	m.width, m.height = 200, 50
	m.currentView = ViewAlerts
	m.clusterData = &model.ClusterData{
		Nodes: []*model.NodeData{{Name: "npu-node", Status: "Ready", NPUCapacity: 8}},
		Summary: &model.ClusterSummary{Alerts: []model.Alert{{
			Severity: model.AlertSeverityWarning, Category: "NPU", AlertType: model.AlertTypeNPUErrorCode,
			ResourceType: "Node", ResourceName: "npu-node",
			Message:     "NPU reported error code on chip(s) 1",
			MessageKey:  "alert.npu_error_code",
			MessageArgs: map[string]interface{}{"Chips": "1"},
			Value:       "chips 1", ValueKey: "alert.value_chips",
			ValueArgs:         map[string]interface{}{"Chips": "1"},
			RecommendedAction: generic, Timestamp: time.Now(),
		}}},
	}

	out := stripANSI(m.renderAlerts())
	zh := diagnostic.GetRecommendedActionChinese(model.AlertTypeNPUErrorCode, "", "npu-node")
	if zh == "" {
		t.Fatal("no Chinese generic action; the probe is vacuous")
	}
	if !strings.Contains(out, zh) {
		t.Errorf("a stored-generic action shadowed the Chinese lookup (want %q):\n%s", zh, out)
	}
}

// TestB24VUnhealthyAggregateColour pins a regression the batch's colour switch
// rewrite introduced on the node aggregate. HEAD rendered
// "Unhealthy" with StyleStatusNotReady (red) and "Warning" with StyleWarning
// (yellow); the rewritten switch folds both onto StyleWarning, so the worst
// aggregate the annotation collector can produce (interface.go:259 sets
// "Unhealthy" when no chip reports OK) is now the same colour as a merely
// degraded one. "Unknown" is the only state the batch meant to re-colour.
func TestB24VUnhealthyAggregateColour(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	render := func(state string) string {
		node := extendedChipNode()
		node.NPUHealthStatus = state
		m := NewModel(nil, nil, 0, "en", "test", 200)
		m.width, m.height = 120, 200
		m.selectedNode = node
		for _, line := range strings.Split(m.renderNodeDetail(), "\n") {
			if strings.Contains(line, "Health Status") {
				return line
			}
		}
		return ""
	}

	unhealthy := render("Unhealthy")
	warning := render("Warning")

	if unhealthy == "" || warning == "" {
		t.Fatalf("the health-status line is missing (unhealthy %q, warning %q)", unhealthy, warning)
	}
	if unhealthy == warning {
		t.Errorf("Unhealthy and Warning render identically, so the worst aggregate is indistinguishable from a degraded one: %q", unhealthy)
	}
	if !strings.Contains(unhealthy, "\x1b[1;91m") {
		t.Errorf("Unhealthy lost StyleStatusNotReady (1;91) and renders like Warning: %q (warning: %q)", unhealthy, warning)
	}
}

// TestB24VUnknownAggregateIsMutedNotRed pins the half of the switch rewrite the
// batch intended: Unknown keeps a non-alarming style.
func TestB24VUnknownAggregateIsMutedNotRed(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	node := extendedChipNode()
	node.NPUHealthStatus = "Unknown"
	m := NewModel(nil, nil, 0, "en", "test", 200)
	m.width, m.height = 120, 200
	m.selectedNode = node
	for _, line := range strings.Split(m.renderNodeDetail(), "\n") {
		if !strings.Contains(line, "Health Status") {
			continue
		}
		if strings.Contains(line, "\x1b[1;91m") || strings.Contains(line, "\x1b[1;93m") {
			t.Errorf("Unknown is painted as a fault/degradation: %q", line)
		}
		return
	}
	t.Fatal("the health-status line is missing")
}

// TestB24VSparseRowsWidthFloor pins the width at which the new sparse rows stop
// fitting. The delivered probe asserts they "respect the terminal width like
// every other line of the detail" at 60 / 80 / 120 columns only; below 60 they
// are longer than the terminal and get clipped mid-token by the renderer (no
// ellipsis, so the number itself is cut), which is what a 38/40-column terminal
// shows. The chip *table* overflows narrow terminals at HEAD too, so this is not
// a regression — but the batch adds two more overflowing lines and its own test
// doc comment overstates the guarantee.
func TestB24VSparseRowsWidthFloor(t *testing.T) {
	forcedAsciiProfile(t)

	const eccRow = "ECC total single"
	floor := 0
	for width := 20; width <= 100; width++ {
		out := renderNPUDetailAtWidth(t, b24ChipNode(), width)
		line := lineContaining(out, eccRow)
		if line != "" && runewidth.StringWidth(line) <= width {
			floor = width
			break
		}
	}
	if floor == 0 {
		t.Fatal("the ECC sparse row never fits any width up to 100")
	}
	if floor != 60 {
		t.Errorf("the ECC sparse row starts fitting at %d columns, want 60 (the delivered probe's narrowest case)", floor)
	}

	// At the task book's 38-column smoke size the row is longer than the
	// terminal, i.e. the real renderer clips it mid-value.
	out := renderNPUDetailAtWidth(t, b24ChipNode(), 38)
	line := lineContaining(out, eccRow)
	if w := runewidth.StringWidth(line); w <= 38 {
		t.Fatalf("the ECC row fits 38 columns after all (%d cells): %q", w, line)
	} else {
		t.Logf("at 38 columns the ECC row is %d cells and gets clipped: %q", w, line)
	}
}

// TestB24VLivenessUnknownStatusIsNotOK pins the default branch of both new
// liveness surfaces. Neither switch originally had a case for a status outside
// ok/backoff/never, and both fell back to the "ok" styling and wording, so a
// producer that sends an unrecognised state (or an older cached payload with
// no status at all) made the summary bar claim "Monitoring OK" and the node
// detail render a green, empty value. The fixed surfaces render the muted
// "unavailable" wording for anything that is not a real status.
func TestB24VLivenessUnknownStatusIsNotOK(t *testing.T) {
	forcedAsciiProfile(t)

	m := npuViewModel(t, "en", 120, 40)
	m.clusterData.Summary.NPUExporterLiveness = &model.NPUExporterLiveness{}

	for _, line := range strings.Split(stripANSI(m.renderNPUView()), "\n") {
		if strings.Contains(line, "Monitoring OK") {
			t.Errorf("an empty liveness state still claims OK: %q", line)
		}
	}
	if bar := lineContaining(stripANSI(m.renderNPUView()), "Monitoring unavailable"); bar == "" {
		t.Errorf("the empty state does not render the muted unavailable wording:\n%s", m.renderNPUView())
	}

	m.selectedNode = m.clusterData.Nodes[0]
	var row string
	for _, line := range strings.Split(stripANSI(m.renderNodeDetail()), "\n") {
		if strings.Contains(line, m.T("npudetail.exporter_liveness")+":") {
			row = line
		}
	}
	if row == "" {
		t.Fatal("the node detail carries no exporter liveness row")
	}
	if strings.Contains(row, m.T("npu.liveness.ok")) || strings.TrimSpace(strings.SplitN(row, ":", 2)[1]) == "" {
		t.Errorf("the unknown-status detail row claims reachable or renders empty: %q", row)
	}
}
