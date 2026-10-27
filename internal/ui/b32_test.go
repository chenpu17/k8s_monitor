package ui

// b32_test.go — batch 32 (node detail charts): the per-node session series,
// the resource-block CPU/memory sparklines, the chip heat strip and the NPU
// session trend rows.
//
// The contracts pinned here, per the delivery book:
//   - T-record: the recorder follows the cluster series' guard (a cached repeat
//     is not a new sample), keeps the 60-sample ring, starts a new session when
//     the user switches nodes, and measures the re-resolved node rather than
//     the pointer captured on entry (batch 31's rule applied to charts).
//   - T-denominator: CPU/memory percentages divide by capacity, the same
//     denominator the headline prints, and missing sources leave fields invalid
//     instead of fabricating a zero.
//   - T-spark-render: a trend appears from the second valid sample on, yields
//     whole when the line has no room, and is absent when the kubelet is.
//   - T-heat-strip: absolute 0-100 shading (identical across capacities), the
//     health glyphs kept for degraded/faulted chips, NPU grouping, and the same
//     glyph the NPU view's matrix draws for the same chip.
//   - T-width / T-nocolor / T-zerodrift: 38/40/60/100 columns in both locales,
//     distinguishable glyphs without ANSI, and no chart rows without data.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b32Node is a node whose capacity differs from its allocatable, so a test can
// tell the two denominators apart: 100m of 2000m CPU is 5%, of 1000m it is 10%;
// 500 of 2000 bytes is 25%, of 1000 it is 50%.
func b32Node(name string) *model.NodeData {
	return &model.NodeData{
		Name: name, Status: "Ready",
		CPUCapacity: 2000, CPUAllocatable: 1000, CPUUsage: 100,
		MemoryCapacity: 2000, MemAllocatable: 1000, MemoryUsage: 500,
		PodCount: 1, PodAllocatable: 10,
		HasKubeletMetrics: true,
	}
}

// b32ChipNode adds n chips to a node, aiCore supplies per-index utilisation and
// grades overrides the healthy grade of the given chip indexes.
func b32ChipNode(name string, n int, aiCore func(i int) int, grades map[int]string) *model.NodeData {
	node := b32Node(name)
	node.NPUCapacity = int64(n)
	node.NPUAllocatable = int64(n)
	chips := make([]model.NPUChipData, n)
	health := make([]model.NPUChipGrade, n)
	for i := 0; i < n; i++ {
		chips[i] = model.NPUChipData{
			NPUID: i / 8, Chip: i % 8, PhyID: i,
			Health: "OK", AICore: aiCore(i),
			HBMUsed: 50, HBMTotal: 100, Power: 100,
		}
		grade := model.NPUChipGradeHealthy
		if g, ok := grades[i]; ok {
			grade = g
		}
		health[i] = model.NPUChipGrade{NPUID: i / 8, Chip: i % 8, Grade: grade}
	}
	node.NPUChips = chips
	node.NPUChipHealth = health
	return node
}

// b32Flat returns an aiCore function yielding the same value on every chip.
func b32Flat(v int) func(int) int {
	return func(int) int { return v }
}

// b32DetailModel returns a model sitting on a node detail page.
func b32DetailModel(t *testing.T, locale string) *Model {
	t.Helper()
	m := modelForTest(t, locale)
	m.width, m.height = 120, 80
	m.currentView = ViewNodeDetail
	m.detailMode = true
	m.selectedNode = b32Node("node-a")
	return m
}

// b32Refresh sends a refresh carrying node at refresh, with the node list the
// re-resolver matches by name.
func b32Refresh(m *Model, node *model.NodeData, refresh time.Time) {
	m.Update(clusterDataMsg{data: &model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{LastRefreshTime: refresh},
	}})
}

// b32Base is a fixed session start.
func b32Base() time.Time {
	return time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// T-record
// ---------------------------------------------------------------------------

// TestB32NodeVizRecordGuard pins the shared guard: two refreshes carrying the
// same LastRefreshTime record one node sample, a new LastRefreshTime records
// another, and no sample is recorded when the node detail is not on screen.
func TestB32NodeVizRecordGuard(t *testing.T) {
	m := b32DetailModel(t, "en")
	node := b32Node("node-a")
	first := b32Base()

	b32Refresh(m, node, first)
	if got := len(m.nodeVizHistory); got != 1 {
		t.Fatalf("after one refresh the node series has %d samples, want 1", got)
	}
	if m.nodeVizNodeName != "node-a" {
		t.Errorf("recorded node name = %q, want node-a", m.nodeVizNodeName)
	}

	b32Refresh(m, node, first)
	if got := len(m.nodeVizHistory); got != 1 {
		t.Errorf("a repeated LastRefreshTime added a node sample: %d, want 1", got)
	}

	b32Refresh(m, node, first.Add(2*time.Second))
	if got := len(m.nodeVizHistory); got != 2 {
		t.Errorf("a new LastRefreshTime did not add a node sample: %d, want 2", got)
	}

	// Leaving the detail stops the recording: the chart belongs to the page.
	m.currentView = ViewNodes
	m.detailMode = false
	b32Refresh(m, node, first.Add(4*time.Second))
	if got := len(m.nodeVizHistory); got != 2 {
		t.Errorf("a refresh outside the node detail recorded a sample: %d, want 2", got)
	}

	// Another detail page must not feed the node series either.
	m.currentView = ViewPodDetail
	m.detailMode = true
	b32Refresh(m, node, first.Add(6*time.Second))
	if got := len(m.nodeVizHistory); got != 2 {
		t.Errorf("a refresh on a pod detail recorded a node sample: %d, want 2", got)
	}
}

// TestB32NodeVizRingKeepsDepth pins the 60-sample ring and that it keeps the
// newest samples.
func TestB32NodeVizRingKeepsDepth(t *testing.T) {
	m := b32DetailModel(t, "en")
	node := b32Node("node-a")
	m.selectedNode = node
	base := b32Base()

	for i := 0; i < m.maxNodeVizHistory+15; i++ {
		b32Refresh(m, node, base.Add(time.Duration(i)*time.Second))
	}
	if got := len(m.nodeVizHistory); got != m.maxNodeVizHistory {
		t.Fatalf("the node series holds %d samples, want the ring depth %d", got, m.maxNodeVizHistory)
	}
	last := base.Add(time.Duration(m.maxNodeVizHistory+14) * time.Second)
	if got := m.nodeVizHistory[len(m.nodeVizHistory)-1].At; !got.Equal(last) {
		t.Errorf("the node series ends at %v, want the newest refresh %v", got, last)
	}
	if m.nodeVizHistory[0].At.Before(base.Add(15 * time.Second)) {
		t.Errorf("the node series kept stale samples from %v", m.nodeVizHistory[0].At)
	}
}

// TestB32NodeVizNodeSwitchStartsNewSession pins the session boundary: switching
// to another node's detail clears the ring, so one node's trend is never drawn
// on top of another's.
func TestB32NodeVizNodeSwitchStartsNewSession(t *testing.T) {
	m := b32DetailModel(t, "en")
	nodeA := b32Node("node-a")
	nodeB := b32Node("node-b")
	m.selectedNode = nodeA
	base := b32Base()

	b32Refresh(m, nodeA, base)
	b32Refresh(m, nodeA, base.Add(2*time.Second))
	if got := len(m.nodeVizHistory); got != 2 {
		t.Fatalf("node-a recorded %d samples, want 2", got)
	}

	// Switch the detail to node-b the way the list does: the selection and the
	// refresh payload now name the other node.
	m.selectedNode = nodeB
	b32Refresh(m, nodeB, base.Add(4*time.Second))
	if got := len(m.nodeVizHistory); got != 1 {
		t.Fatalf("switching nodes left %d samples, want a fresh session with 1", got)
	}
	if m.nodeVizNodeName != "node-b" {
		t.Errorf("recorded node name = %q after the switch, want node-b", m.nodeVizNodeName)
	}
}

// TestB32NodeVizRecordsReresolvedNode pins that the sample is taken from the
// re-resolved selection, not the pointer captured when Enter was pressed. The
// refresh carries the same node name with fresh usage; a recorder that ran
// before reresolveDetailSelections would record the stale 5% instead of 50%.
func TestB32NodeVizRecordsReresolvedNode(t *testing.T) {
	m := b32DetailModel(t, "en")
	oldNode := b32Node("node-a")
	m.selectedNode = oldNode
	base := b32Base()

	b32Refresh(m, oldNode, base)

	freshNode := b32Node("node-a")
	freshNode.CPUUsage = 1000 // 50% of capacity 2000
	b32Refresh(m, freshNode, base.Add(2*time.Second))

	if m.selectedNode != freshNode {
		t.Fatalf("the detail still points at the object captured at entry")
	}
	last := m.nodeVizHistory[len(m.nodeVizHistory)-1]
	if want := 50.0; last.CPUPct != want {
		t.Errorf("the recorded sample used the stale node: CPUPct = %v, want %v", last.CPUPct, want)
	}
}

// ---------------------------------------------------------------------------
// T-denominator
// ---------------------------------------------------------------------------

// TestB32NodeVizDenominatorUsesCapacity pins the denominator: the sample and the
// headline percentage both divide by capacity, so with capacity != allocatable
// the two must agree on the capacity-based number and not fall back to
// allocatable.
func TestB32NodeVizDenominatorUsesCapacity(t *testing.T) {
	forcedAsciiProfile(t)

	node := b32Node("node-a")
	sample := buildNodeVizSample(node, b32Base())
	if want := 5.0; sample.CPUPct != want {
		t.Errorf("CPUPct = %v, want %v (100m / 2000m capacity)", sample.CPUPct, want)
	}
	if want := 25.0; sample.MemPct != want {
		t.Errorf("MemPct = %v, want %v (500 / 2000 capacity)", sample.MemPct, want)
	}
	if !sample.MemValid {
		t.Errorf("MemValid = false for an idle-safe kubelet sample")
	}

	m := b32DetailModel(t, "en")
	out := stripANSI(m.renderNodeResourceInfo(node))
	cpuLine := lineContaining(out, m.T("detail.field.cpu"))
	if !strings.Contains(cpuLine, "(5.0%)") || !strings.Contains(cpuLine, "2.00") {
		t.Errorf("CPU headline does not divide by capacity: %q", cpuLine)
	}
	if strings.Contains(cpuLine, "10.0%") {
		t.Errorf("CPU headline used allocatable as the denominator: %q", cpuLine)
	}
	memLine := lineContaining(out, m.T("detail.field.memory"))
	if !strings.Contains(memLine, "(25.0%)") || !strings.Contains(memLine, "2.0 KiB") {
		t.Errorf("memory headline does not divide by capacity: %q", memLine)
	}
	if strings.Contains(memLine, "50.0%") {
		t.Errorf("memory headline used allocatable as the denominator: %q", memLine)
	}
}

// TestB32NodeVizInvalidFieldsNotFaked pins the "invalid, not zero" rule: a
// missing kubelet summary, a non-positive capacity and absent chip/HBM data all
// leave their validity flags false instead of recording a measured zero.
func TestB32NodeVizInvalidFieldsNotFaked(t *testing.T) {
	at := b32Base()

	noKubelet := &model.NodeData{
		Name: "n", CPUCapacity: 2000, MemoryCapacity: 2000,
		MemoryUsage: 500, // nonzero usage alone is accepted as evidence...
	}
	if got := buildNodeVizSample(noKubelet, at); !got.MemValid {
		t.Errorf("a payload with usage but no kubelet flag was treated as missing")
	}

	// ...but a failed fetch zeroes the usage and clears the flag.
	failed := &model.NodeData{Name: "n", CPUCapacity: 2000, MemoryCapacity: 2000}
	got := buildNodeVizSample(failed, at)
	if got.MemValid || got.CPUPct != 0 || got.MemPct != 0 {
		t.Errorf("a missing kubelet summary fabricated a sample: %+v", got)
	}
	if got.NPUUtilValid || got.HBMValid {
		t.Errorf("a node without chips reported NPU/HBM validity: %+v", got)
	}

	// Non-positive capacities have no denominator to divide by.
	noCap := &model.NodeData{Name: "n", HasKubeletMetrics: true, CPUUsage: 100, MemoryUsage: 100}
	got = buildNodeVizSample(noCap, at)
	if got.MemValid {
		t.Errorf("MemValid = true with MemoryCapacity 0")
	}
	if got.CPUPct != 0 || got.MemPct != 0 {
		t.Errorf("zero capacities produced percentages: %+v", got)
	}
}

// TestB32NodeVizNPUAggregates pins the NPU side of the sample: utilisation is
// the mean over chips that reported (an "Unknown" chip is not averaged in as
// idle), power is the sum over every chip, and HBM is used/total with a zero
// total invalid.
func TestB32NodeVizNPUAggregates(t *testing.T) {
	node := b32Node("n")
	node.NPUChips = []model.NPUChipData{
		{NPUID: 0, Chip: 0, Health: "OK", AICore: 40, Power: 300, HBMUsed: 100, HBMTotal: 1000},
		{NPUID: 0, Chip: 1, Health: "Unknown", AICore: 0, Power: 200, HBMUsed: 100, HBMTotal: 1000},
	}
	sample := buildNodeVizSample(node, b32Base())
	if !sample.NPUUtilValid {
		t.Fatalf("NPUUtilValid = false on a chip-bearing node")
	}
	if want := 40.0; sample.NPUUtilPct != want {
		t.Errorf("NPUUtilPct = %v, want %v (the Unknown chip is not an idle chip)", sample.NPUUtilPct, want)
	}
	if want := 500.0; sample.NPUPowerW != want {
		t.Errorf("NPUPowerW = %v, want %v (sum over every chip)", sample.NPUPowerW, want)
	}
	if !sample.HBMValid {
		t.Fatalf("HBMValid = false with a positive total")
	}
	if want := 10.0; sample.HBMPct != want {
		t.Errorf("HBMPct = %v, want %v (200 used / 2000 total)", sample.HBMPct, want)
	}

	// No chip reported utilisation: unknown, not an idle node.
	unknown := b32Node("n")
	unknown.NPUChips = []model.NPUChipData{{NPUID: 0, Chip: 0, Health: "Unknown", Power: 50}}
	if s := buildNodeVizSample(unknown, b32Base()); s.NPUUtilValid {
		t.Errorf("NPUUtilValid = true when no chip reported utilisation")
	}
}

// b32History installs a per-node session with the given CPU percentages.
func b32History(m *Model, cpu ...float64) {
	m.nodeVizHistory = m.nodeVizHistory[:0]
	// The render side only borrows the session of the node it is named after,
	// so the helper records the fixture name the tests render ("node-a").
	m.nodeVizNodeName = "node-a"
	for i, v := range cpu {
		m.nodeVizHistory = append(m.nodeVizHistory, NodeVizSample{
			At:     b32Base().Add(time.Duration(i) * time.Second),
			CPUPct: v, MemPct: v / 5, MemValid: true,
			NPUUtilPct: v * 8, NPUUtilValid: true,
			NPUPowerW: v * 32, HBMPct: v, HBMValid: true,
		})
	}
}

// ---------------------------------------------------------------------------
// T-spark-render
// ---------------------------------------------------------------------------

// TestB32ResourceSparkFromSecondSample pins the appearance rule: the CPU and
// memory rows carry no trend below two samples and exactly the sparkline of the
// series from two on.
func TestB32ResourceSparkFromSecondSample(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32DetailModel(t, "en")
	node := b32Node("node-a")
	m.selectedNode = node
	cpuBase := "  CPU: 100m / 2.00 (5.0%)"
	memBase := "  Memory: 500 B / 2.0 KiB (25.0%)"

	render := func() (string, string) {
		out := stripANSI(m.renderNodeResourceInfo(node))
		return lineContaining(out, m.T("detail.field.cpu")),
			lineContaining(out, m.T("detail.field.memory"))
	}

	b32History(m)
	cpu, mem := render()
	if cpu != cpuBase || mem != memBase {
		t.Errorf("with no samples the rows already changed:\n cpu %q\n mem %q", cpu, mem)
	}

	b32History(m, 5)
	cpu, mem = render()
	if cpu != cpuBase || mem != memBase {
		t.Errorf("one sample drew a trend:\n cpu %q\n mem %q", cpu, mem)
	}

	b32History(m, 5, 2)
	cpu, mem = render()
	wantCPU := cpuBase + " " + sparkline([]float64{5, 2}, nodeDetailSparkWidth)
	if cpu != wantCPU {
		t.Errorf("CPU row at two samples:\n got %q\nwant %q", cpu, wantCPU)
	}
	// At width 120 the memory line (36 cells) leaves room for the reserved
	// 10-cell sparkline.
	wantMem := memBase + " " + sparkline([]float64{1, 0.4}, nodeDetailSparkWidth)
	if mem != wantMem {
		t.Errorf("memory row at two samples:\n got %q\nwant %q", mem, wantMem)
	}
}

// TestB32ResourceSparkAbsentWithoutKubelet pins honest absence: a node whose
// kubelet summary is missing draws no CPU/memory trend even when the session
// holds earlier valid samples, and a node without NPU chip health draws no heat
// strip or NPU trend rows.
func TestB32ResourceSparkAbsentWithoutKubelet(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32DetailModel(t, "en")
	node := b32Node("node-a")
	node.HasKubeletMetrics = false
	node.CPUUsage = 0
	node.MemoryUsage = 0
	node.NPUCapacity = 8 // capacity but no chip health data
	m.selectedNode = node
	b32History(m, 5, 2, 8)

	out := stripANSI(m.renderNodeResourceInfo(node))
	if line := lineContaining(out, m.T("detail.field.cpu")); strings.ContainsAny(line, sparkRamp) {
		t.Errorf("a missing kubelet still drew a CPU trend: %q", line)
	}
	if line := lineContaining(out, m.T("detail.field.memory")); strings.ContainsAny(line, sparkRamp) {
		t.Errorf("a missing kubelet still drew a memory trend: %q", line)
	}
	if strings.Contains(out, m.T("nodedetail.heat_strip")) {
		t.Errorf("a node without chip health drew the heat strip")
	}
	if strings.Contains(out, m.T("nodedetail.npu_util_trend")) ||
		strings.Contains(out, m.T("nodedetail.npu_power_trend")) {
		t.Errorf("a node without chip data drew NPU trend rows")
	}
}

// TestB32SparkYieldsWhenNarrow pins the value-first rule: when the reserved
// sparkline width does not fit the line, the whole sparkline yields and the
// value stays untouched.
func TestB32SparkYieldsWhenNarrow(t *testing.T) {
	forcedAsciiProfile(t)

	// 25 cells of CPU row + 1 separator + 10 reserved spark cells = 36 fits at
	// 38 and not at 30.
	for _, tc := range []struct {
		width int
		spark bool
	}{
		{30, false},
		{38, true},
	} {
		m := b32DetailModel(t, "en")
		m.width = tc.width
		node := b32Node("node-a")
		m.selectedNode = node
		b32History(m, 5, 2)

		line := lineContaining(stripANSI(m.renderNodeResourceInfo(node)), m.T("detail.field.cpu"))
		hasSpark := strings.ContainsAny(line, sparkRamp)
		if hasSpark != tc.spark {
			t.Errorf("width %d: spark present = %v, want %v (line %q)", tc.width, hasSpark, tc.spark, line)
		}
		if w := runewidth.StringWidth(line); w > tc.width {
			t.Errorf("width %d: CPU row is %d cells wide: %q", tc.width, w, line)
		}
	}
}

// ---------------------------------------------------------------------------
// T-heat-strip
// ---------------------------------------------------------------------------

// b32Strip extracts the chip-cell part of the heat strip line (label removed).
// The legend repeats the ramp glyphs, so tests that inspect the cells take
// b32WantStrip — the exact cell string — rather than Fields over the tail.
func b32Strip(t *testing.T, m *Model, node *model.NodeData) string {
	t.Helper()
	line := m.renderNodeChipHeatStrip(node)
	if line == "" {
		return ""
	}
	prefix := "  " + m.T("nodedetail.heat_strip") + " "
	if !strings.HasPrefix(stripANSI(line), prefix) {
		t.Fatalf("heat strip line does not start with its label: %q", line)
	}
	return strings.TrimPrefix(stripANSI(line), prefix)
}

// b32WantStrip is the cell string the shared heat primitives produce for node,
// i.e. what the strip must start with.
func b32WantStrip(node *model.NodeData) string {
	levels := npuHeatLevels(npuChipValues(node, npuMatrixModeAICore), npuMatrixModeAICore)
	return stripANSI(npuMatrixText(npuChipCellRow(node.NPUChipHealth, levels), -1))
}

// TestB32HeatStripAbsoluteScale pins the absolute 0-100 scale: 25% and 75% map
// to the ramp's 25% and 75% buckets whatever the node's other chips say. A
// node-local normalization would stretch the same two chips across the whole
// ramp (▂ █) and is exactly the defect this refuses.
func TestB32HeatStripAbsoluteScale(t *testing.T) {
	m := b32DetailModel(t, "en")
	m.width = 200

	// Two chips, 25% and 75%, no other chip to widen the range.
	node := b32ChipNode("node-a", 2, func(i int) int { return 25 + 50*i }, nil)
	strip := b32Strip(t, m, node)
	ramp := []rune(npuHeatRamp)
	want := string(ramp[1]) + " " + string(ramp[4])
	if strip == "" || !strings.HasPrefix(strip, want) {
		t.Fatalf("25/75 on one node rendered %q, want %q (absolute 0-100 scale)", strip, want)
	}
	if got := b32WantStrip(node); got != want {
		t.Fatalf("the shared heat primitives produced %q, want %q (absolute 0-100 scale)", got, want)
	}

	// The same 50% cell on a 1-chip node and on a 16-chip node draws the same
	// glyph: capacity does not change the scale.
	smallNode := b32ChipNode("n1", 1, b32Flat(50), nil)
	largeNode := b32ChipNode("n16", 16, b32Flat(50), nil)
	small := b32WantStrip(smallNode)
	large := b32WantStrip(largeNode)
	mid := string(ramp[3])
	if !strings.HasPrefix(small, mid) {
		t.Errorf("50%% on a 1-chip node rendered %q, want %q", small, mid)
	}
	for _, cell := range strings.Fields(large) {
		if cell != mid {
			t.Errorf("50%% on a 16-chip node rendered %q, want every cell %q", large, mid)
		}
	}
	if !strings.HasPrefix(b32Strip(t, m, largeNode), large) {
		t.Errorf("the 16-chip strip does not start with the primitives' cell string")
	}
}

// TestB32HeatStripKeepsHealthGlyphs pins that a degraded/faulted chip keeps its
// health cell instead of taking the heat glyph, even at 100% utilisation.
func TestB32HeatStripKeepsHealthGlyphs(t *testing.T) {
	m := b32DetailModel(t, "en")
	m.width = 200

	node := b32ChipNode("node-a", 16, func(i int) int {
		if i == 5 || i == 10 {
			return 100 // a broken chip's utilisation must not win the cell
		}
		return 50
	}, map[int]string{
		5:  model.NPUChipGradeDegraded,
		10: model.NPUChipGradeFault,
	})
	strip := b32WantStrip(node)
	if !strings.Contains(strip, "▒") {
		t.Errorf("degraded chip lost its ▒ cell: %q", strip)
	}
	if !strings.Contains(strip, "▁") {
		t.Errorf("faulted chip lost its ▁ cell: %q", strip)
	}
	if strings.Contains(strip, "█") {
		t.Errorf("a chip cell took the top heat glyph instead of a health glyph: %q", strip)
	}
}

// TestB32HeatStripGroupingAndMatrixConsistency pins the NPU grouping (two-space
// gap, one-space inside an NPU) and that the strip's cells are exactly the NPU
// view's default-mode matrix cells for the same chips.
func TestB32HeatStripGroupingAndMatrixConsistency(t *testing.T) {
	m := b32DetailModel(t, "en")
	m.width = 200
	m.npuMatrixMode = npuMatrixModeAICore

	node := b32ChipNode("node-a", 16, func(i int) int { return i * 5 }, nil)
	strip := b32Strip(t, m, node)

	want := b32WantStrip(node)
	if !strings.HasPrefix(strip, want) {
		t.Fatalf("heat strip cells %q do not match the matrix cells %q", strip, want)
	}
	if got := strings.Count(want, "  "); got != 1 {
		t.Errorf("NPU grouping lost its two-space gap (%d found): %q", got, want)
	}

	row := stripANSI(m.npuMatrixRow(node, false))
	if !strings.Contains(row, want) {
		t.Errorf("the NPU view matrix and the node detail heat strip disagree:\n row %q\nstrip %q", row, want)
	}
}

// TestB32HeatStripAbsentWithoutChips pins the precondition: no chip grades, no
// strip.
func TestB32HeatStripAbsentWithoutChips(t *testing.T) {
	m := b32DetailModel(t, "en")
	if got := m.renderNodeChipHeatStrip(b32Node("node-a")); got != "" {
		t.Errorf("a node without chip grades rendered a strip: %q", got)
	}
}

// ---------------------------------------------------------------------------
// T-width
// ---------------------------------------------------------------------------

// TestB32NewLinesFitWidth pins the width budget of every line this batch adds:
// at 38, 40, 60 and 100 columns in both locales no new line exceeds the
// terminal, and the sparklines step down as the terminal narrows.
func TestB32NewLinesFitWidth(t *testing.T) {
	forcedAsciiProfile(t)

	// The memory row is 36 cells before the sparkline; appendSpark reserves the
	// full 10-cell spark width, so it fits from 47 cells on.
	const memSparkMin = 47

	for _, locale := range []string{"en", "zh"} {
		for _, width := range []int{38, 40, 60, 100} {
			t.Run(fmt.Sprintf("%s/w%d", locale, width), func(t *testing.T) {
				m := b32DetailModel(t, locale)
				m.width = width
				node := b32ChipNode("node-a", 16, func(i int) int { return 10 + i }, map[int]string{
					4:  model.NPUChipGradeDegraded,
					12: model.NPUChipGradeFault,
				})
				m.selectedNode = node
				b32History(m, 5, 2, 8)

				out := stripANSI(m.renderNodeResourceInfo(node))
				lines := map[string]string{
					"cpu":       lineContaining(out, m.T("detail.field.cpu")),
					"memory":    lineContaining(out, m.T("detail.field.memory")),
					"heat":      lineContaining(out, m.T("nodedetail.heat_strip")),
					"npu_util":  lineContaining(out, m.T("nodedetail.npu_util_trend")),
					"npu_power": lineContaining(out, m.T("nodedetail.npu_power_trend")),
				}
				for name, line := range lines {
					if line == "" {
						t.Fatalf("%s row is missing at width %d", name, width)
					}
					if strings.Contains(line, "\n") {
						t.Errorf("%s row wrapped at width %d: %q", name, width, line)
					}
					if w := runewidth.StringWidth(line); w > width {
						t.Errorf("%s row is %d cells wide on a %d-cell terminal: %q", name, w, width, line)
					}
				}

				// The sparkline yields before the value does: CPU fits at 38,
				// the longer memory row only from memSparkMin on.
				if !strings.ContainsAny(lines["cpu"], sparkRamp) {
					t.Errorf("width %d: CPU trend unexpectedly absent: %q", width, lines["cpu"])
				}
				memHasSpark := strings.ContainsAny(lines["memory"], sparkRamp)
				if width < memSparkMin && memHasSpark {
					t.Errorf("width %d: memory trend did not yield: %q", width, lines["memory"])
				}
				if width >= memSparkMin && !memHasSpark {
					t.Errorf("width %d: memory trend missing where it fits: %q", width, lines["memory"])
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// T-nocolor
// ---------------------------------------------------------------------------

// TestB32NoColorGlyphs pins that the new rows carry no ANSI under --no-color and
// that the glyphs still distinguish heat from health.
func TestB32NoColorGlyphs(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32DetailModel(t, "en")
	m.width = 200
	node := b32ChipNode("node-a", 16, func(i int) int { return 10 + i }, map[int]string{
		4:  model.NPUChipGradeDegraded,
		12: model.NPUChipGradeFault,
	})
	m.selectedNode = node
	b32History(m, 5, 2, 8)

	out := m.renderNodeResourceInfo(node)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("new node-detail rows emit ANSI in --no-color mode:\n%q", out)
	}

	strip := b32WantStrip(node)
	if !strings.Contains(strip, "▒") || !strings.Contains(strip, "▁") {
		t.Errorf("health glyphs not distinguishable without colour: %q", strip)
	}
	heatCells := 0
	for _, r := range strip {
		if strings.ContainsRune(npuHeatRamp, r) {
			heatCells++
		}
	}
	if heatCells == 0 {
		t.Errorf("no heat glyphs in the strip: %q", strip)
	}
}

// ---------------------------------------------------------------------------
// T-zerodrift
// ---------------------------------------------------------------------------

// TestB32NoSamplesNoChartRows pins the zero-drift half: without a session the
// resource block keeps its pre-batch shape and a chip-bearing NPU block grows
// only the stateless heat strip, never a fabricated trend.
func TestB32NoSamplesNoChartRows(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32DetailModel(t, "en")
	node := b32ChipNode("node-a", 16, b32Flat(50), nil)
	m.selectedNode = node

	out := stripANSI(m.renderNodeResourceInfo(node))
	if line := lineContaining(out, m.T("detail.field.cpu")); strings.ContainsAny(line, sparkRamp) {
		t.Errorf("CPU row grew a trend without samples: %q", line)
	}
	if strings.Contains(out, m.T("nodedetail.npu_util_trend")) ||
		strings.Contains(out, m.T("nodedetail.npu_power_trend")) {
		t.Errorf("NPU trend rows rendered without samples")
	}
	// The heat strip is a snapshot, not a trend: chip data alone is enough.
	if !strings.Contains(out, m.T("nodedetail.heat_strip")) {
		t.Errorf("the heat strip is missing on a chip-bearing node")
	}
}

// TestB32DetailScrollOffsetSurvivesRefresh pins that chart rows arriving with a
// refresh do not move the user's scroll position (the renderer clamps a local
// copy; the stored offset is theirs).
func TestB32DetailScrollOffsetSurvivesRefresh(t *testing.T) {
	m := b32DetailModel(t, "en")
	node := b32Node("node-a")
	m.selectedNode = node
	m.detailScrollOffset = 7

	for i := 1; i <= 3; i++ {
		b32Refresh(m, node, b32Base().Add(time.Duration(i)*time.Second))
	}
	if m.detailScrollOffset != 7 {
		t.Errorf("a refresh changed the detail scroll offset to %d, want 7", m.detailScrollOffset)
	}
}

// ---------------------------------------------------------------------------
// D5: i18n
// ---------------------------------------------------------------------------

// TestB32NewI18nKeysPlaceholderParity pins the batch's 4 new keys: present in
// both bundles with the same placeholder set.
func TestB32NewI18nKeysPlaceholderParity(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	newKeys := []string{
		"nodedetail.heat_strip", "nodedetail.heat_legend",
		"nodedetail.npu_util_trend", "nodedetail.npu_power_trend",
	}
	placeholder := regexp.MustCompile(`\{\{\.[A-Za-z]+\}\}`)
	placeholders := func(s string) []string {
		out := placeholder.FindAllString(s, -1)
		sort.Strings(out)
		return out
	}
	for _, key := range newKeys {
		enMsg, okEn := en[key]
		zhMsg, okZh := zh[key]
		if !okEn || !okZh {
			t.Errorf("%s missing (en=%v zh=%v)", key, okEn, okZh)
			continue
		}
		if strings.TrimSpace(enMsg) == "" || strings.TrimSpace(zhMsg) == "" {
			t.Errorf("%s has an empty message (%q / %q)", key, enMsg, zhMsg)
		}
		if got, want := placeholders(zhMsg), placeholders(enMsg); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s placeholder mismatch: en %v, zh %v", key, want, got)
		}
	}

	// Non-vacuousness: the legend really carries the ramp placeholder the test
	// guards in both locales.
	for locale, msg := range map[string]string{"en": en["nodedetail.heat_legend"], "zh": zh["nodedetail.heat_legend"]} {
		if !strings.Contains(msg, "{{.Ramp}}") {
			t.Errorf("%s nodedetail.heat_legend lost its {{.Ramp}} placeholder: %q", locale, msg)
		}
	}
}

// TestB32LabelsResolve pins that every new key resolves in both locales (no
// raw key leaking into the page).
func TestB32LabelsResolve(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
		for _, key := range []string{
			"nodedetail.heat_strip", "nodedetail.npu_util_trend", "nodedetail.npu_power_trend",
		} {
			if got := m.T(key); got == "" || strings.Contains(got, "nodedetail.") {
				t.Errorf("%s: %s did not resolve (%q)", locale, key, got)
			}
		}
		if got := m.TF("nodedetail.heat_legend", map[string]interface{}{"Ramp": npuHeatRamp}); !strings.Contains(got, npuHeatRamp) {
			t.Errorf("%s: heat legend did not interpolate the ramp (%q)", locale, got)
		}
	}
}

// TestB32AllocatableReferenceLine pins the overview load box's transparency
// rule on the node detail: capacity is the headline denominator, and the
// schedulable budget stays visible as a muted reference line whenever the two
// differ — they do on real nodes that carry a system reserve. Equal
// denominators, the capacity-fallback path and an N/A usage render no
// reference line.
func TestB32AllocatableReferenceLine(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32DetailModel(t, "en")
	alloc := m.T("metrics.allocatable")

	out := stripANSI(m.renderNodeResourceInfo(b32Node("node-a")))
	cpuRef := lineContaining(out, alloc)
	if !strings.Contains(cpuRef, "1.00") || !strings.Contains(cpuRef, "(10.0%)") {
		t.Errorf("CPU allocatable reference missing or wrong: %q", cpuRef)
	}
	if !strings.Contains(out, "1000 B") || !strings.Contains(out, "(50.0%)") {
		t.Errorf("memory allocatable reference missing: %q", out)
	}

	// The reference line also respects the terminal width.
	m.width = 38
	out = stripANSI(m.renderNodeResourceInfo(b32Node("node-a")))
	for _, ref := range strings.Split(out, "\n") {
		if !strings.Contains(ref, alloc) {
			continue
		}
		if w := runewidth.StringWidth(ref); w > 38 {
			t.Errorf("reference line is %d cells wide on a 38-cell terminal: %q", w, ref)
		}
	}

	// Equal denominators: the reference would just repeat the headline.
	node := b32Node("node-b")
	node.CPUCapacity, node.MemoryCapacity = 1000, 1000
	if out := stripANSI(m.renderNodeResourceInfo(node)); strings.Contains(out, alloc) {
		t.Errorf("reference line rendered with equal denominators: %q", out)
	}

	// Capacity fallback: allocatable is already the headline denominator.
	node = b32Node("node-c")
	node.CPUCapacity, node.MemoryCapacity = 0, 0
	if out := stripANSI(m.renderNodeResourceInfo(node)); strings.Contains(out, alloc) {
		t.Errorf("reference line rendered on the capacity fallback path: %q", out)
	}

	// No usage: the headline is N/A, and a percent beside it would be fake.
	node = b32Node("node-d")
	node.CPUUsage, node.MemoryUsage = 0, 0
	if out := stripANSI(m.renderNodeResourceInfo(node)); strings.Contains(out, alloc) {
		t.Errorf("reference line rendered without usage: %q", out)
	}
}

// TestB32CrossNodeSeriesGuard pins the render-side node identity check:
// entering another node's detail must not draw the previous node's session.
// The ring is emptied only by the recorder on the next advancing snapshot, so
// without the check the new page borrows the old node's trend until then —
// indefinitely, while refreshes keep failing.
func TestB32CrossNodeSeriesGuard(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32DetailModel(t, "en")
	b32History(m, 5, 2, 8)

	// node-a owns the session: its rows render.
	nodeA := b32Node("node-a")
	if line := lineContaining(stripANSI(m.renderNodeResourceInfo(nodeA)), m.T("detail.field.cpu")); !strings.ContainsAny(line, sparkRamp) {
		t.Errorf("node-a's own session did not render: %q", line)
	}

	// node-b entered before any refresh: no sparklines, no NPU trend rows.
	nodeB := b32Node("node-b")
	out := stripANSI(m.renderNodeResourceInfo(nodeB))
	if cpuLine := lineContaining(out, m.T("detail.field.cpu")); strings.ContainsAny(cpuLine, sparkRamp) {
		t.Errorf("node-b's CPU row drew node-a's session: %q", cpuLine)
	}
	if memLine := lineContaining(out, m.T("detail.field.memory")); strings.ContainsAny(memLine, sparkRamp) {
		t.Errorf("node-b's memory row drew node-a's session: %q", memLine)
	}
	if rows := stripANSI(strings.Join(m.renderNodeNPUTrends(nodeB), "\n")); strings.Contains(rows, m.T("nodedetail.npu_util_trend")) || strings.Contains(rows, m.T("nodedetail.npu_power_trend")) {
		t.Errorf("node-b's NPU block drew node-a's trends: %q", rows)
	}
}

// TestB32HeatStripLegendWholeOrAbsent pins the legend rule: the legend is
// decorative, so it is appended only when the whole line fits — a narrow
// terminal loses the legend entire, never keeps a truncated "lo..." fragment
// that reads as a broken word. The ramp glyphs appear contiguously only in the
// legend (strip cells are always space-separated).
func TestB32HeatStripLegendWholeOrAbsent(t *testing.T) {
	forcedAsciiProfile(t)

	for _, locale := range []string{"en", "zh"} {
		for _, width := range []int{38, 40, 60, 100} {
			m := b32DetailModel(t, locale)
			m.width = width
			node := b32ChipNode("node-a", 16, func(i int) int { return 10 + i }, nil)

			line := stripANSI(m.renderNodeChipHeatStrip(node))
			if w := runewidth.StringWidth(line); w > width {
				t.Errorf("%s w%d: strip line is %d cells wide: %q", locale, width, w, line)
			}
			if strings.Contains(line, npuHeatRamp+" ") && !strings.Contains(line, m.TF("nodedetail.heat_legend", map[string]interface{}{"Ramp": npuHeatRamp})) {
				t.Errorf("%s w%d: legend truncated, want whole or absent: %q", locale, width, line)
			}
		}
	}
}
