package ui

// b32v_probe_test.go — batch-32 adversarial probes written by the verification
// pass. They are deliberately independent of b32_test.go's helpers: each one
// pins a defect shape the delivery tests could pass while the product is wrong.
//
// Covered:
//   - C: a session gap (kubelet dropped mid-session) must draw a blank cell,
//     never the lowest bar; an all-zero series has no row at all.
//   - F: an all-zero AI Core series drops the "mandatory" util trend row while
//     the power row survives — pinned and characterised.
//   - D: the heat strip's per-width chip table, the truncation marker, the
//     legend-before-chips clipping order, and an independent cell-math check.
//   - B: the allocatable reference line across widths and locales, plus the
//     four-state guard.
//   - A: the node list keeps allocatable as its denominator.
//   - E: the sample comes from the re-resolved object (record-order evidence).

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b32vNodeLike is a capacity!=allocatable node, the shape the denominator
// change is about: 100m/2000m is 5%, 500B/2000B is 25%.
func b32vNodeLike(name string) *model.NodeData {
	return &model.NodeData{
		Name: name, Status: "Ready",
		CPUCapacity: 2000, CPUAllocatable: 1000, CPUUsage: 100,
		MemoryCapacity: 2000, MemAllocatable: 1000, MemoryUsage: 500,
		PodCount: 1, PodAllocatable: 10, HasKubeletMetrics: true,
	}
}

// b32vChipNode is n chips in NPU pairs of eight with per-chip AICore.
func b32vChipNode(name string, n int, aiCore func(int) int, grades map[int]string) *model.NodeData {
	node := b32vNodeLike(name)
	node.NPUCapacity = int64(n)
	node.NPUAllocatable = int64(n)
	for i := 0; i < n; i++ {
		node.NPUChips = append(node.NPUChips, model.NPUChipData{
			NPUID: i / 8, Chip: i % 8, PhyID: i, Health: "OK",
			AICore: aiCore(i), HBMUsed: 50, HBMTotal: 100, Power: 100,
		})
		grade := model.NPUChipGradeHealthy
		if g, ok := grades[i]; ok {
			grade = g
		}
		node.NPUChipHealth = append(node.NPUChipHealth, model.NPUChipGrade{NPUID: i / 8, Chip: i % 8, Grade: grade})
	}
	return node
}

func b32vDetailModel(t *testing.T, locale string) *Model {
	t.Helper()
	m := modelForTest(t, locale)
	m.width, m.height = 120, 80
	m.currentView = ViewNodeDetail
	m.detailMode = true
	m.selectedNode = b32vNodeLike("node-a")
	return m
}

func b32vBase() time.Time { return time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC) }

// b32vSetHistory installs a CPU series directly on the ring. The ring is named
// for the fixture node the probes render ("node-a"): the render side only
// borrows the session of the node it is named after.
func b32vSetHistory(m *Model, cpu []float64, valid []bool) {
	m.nodeVizHistory = m.nodeVizHistory[:0]
	m.nodeVizNodeName = "node-a"
	for i, v := range cpu {
		s := NodeVizSample{At: b32vBase().Add(time.Duration(i) * time.Second), CPUPct: v}
		if i < len(valid) {
			s.MemPct, s.MemValid = v/5, valid[i]
		}
		m.nodeVizHistory = append(m.nodeVizHistory, s)
	}
}

// ---------------------------------------------------------------------------
// C: no fake low bar on a session gap
// ---------------------------------------------------------------------------

// TestB32VCPUGapIsBlankNotLowBar drives the real sparkline and the rendered CPU
// row with a mid-session gap: the recording path writes CPUPct=0 when the
// kubelet drops out (there is no CPUValid bit), and the sparkline must render
// that zero as a blank cell, not as ▁. A ▁ would claim "the node idled" where
// the truth is "the node stopped reporting".
func TestB32VCPUGapIsBlankNotLowBar(t *testing.T) {
	forcedAsciiProfile(t)

	// Primitives first: [50, 0, 50] at the node detail's width.
	got := sparkline([]float64{50, 0, 50}, nodeDetailSparkWidth)
	runes := []rune(got)
	if len(runes) != 3 {
		t.Fatalf("sparkline([50,0,50]) = %q, want three cells", got)
	}
	if runes[1] != ' ' {
		t.Errorf("the gap sample drew %q, want a blank cell (▁ would fabricate a low reading)", string(runes[1]))
	}
	if strings.ContainsRune(got, '▁') {
		t.Errorf("the gap sample produced the lowest bar: %q", got)
	}

	// End to end: the rendered CPU row carries the same blank for the gap. The
	// current node reports again, so the render-side gate is open; the gap is
	// only in the ring.
	m := b32vDetailModel(t, "en")
	node := b32vNodeLike("node-a")
	m.selectedNode = node
	b32vSetHistory(m, []float64{50, 0, 50}, []bool{true, true, true})

	row := lineContaining(stripANSI(m.renderNodeResourceInfo(node)), m.T("detail.field.cpu"))
	if !strings.ContainsAny(row, sparkRamp) {
		t.Fatalf("the CPU row has no trend at all: %q", row)
	}
	// The renderer appends exactly the sparkline of the ring.
	spark := sparkline([]float64{50, 0, 50}, nodeDetailSparkWidth)
	if !strings.HasSuffix(row, " "+spark) {
		t.Errorf("CPU row = %q, want it to end with the series sparkline %q", row, spark)
	}
	if !strings.Contains(spark, " ") {
		t.Errorf("the gap cell is not blank in %q", spark)
	}
	if strings.Contains(row, "▁") {
		t.Errorf("the rendered CPU row drew a fake low bar for the gap: %q", row)
	}
}

// TestB32VAllZeroCPUSeriesDropsRow characterises the other edge: an all-zero
// (real idle) series makes sparkline return "", so the row carries no trend
// even though the ring has samples. This is the same semantic the overview's
// load box uses; pinned so a future change cannot make it silently different.
func TestB32VAllZeroCPUSeriesDropsRow(t *testing.T) {
	forcedAsciiProfile(t)

	if got := sparkline([]float64{0, 0, 0}, nodeDetailSparkWidth); got != "" {
		t.Errorf("sparkline of an all-zero series = %q, want empty", got)
	}

	m := b32vDetailModel(t, "en")
	node := b32vNodeLike("node-a")
	m.selectedNode = node
	// Ring has three samples, all zero; the current node reports usage.
	b32vSetHistory(m, []float64{0, 0, 0}, []bool{true, true, true})

	row := lineContaining(stripANSI(m.renderNodeResourceInfo(node)), m.T("detail.field.cpu"))
	if strings.ContainsAny(row, sparkRamp) {
		t.Errorf("an all-zero series drew a trend row: %q", row)
	}
	if len(m.nodeVizHistory) != 3 {
		t.Fatalf("the probe's ring holds %d samples, want 3", len(m.nodeVizHistory))
	}
}

// ---------------------------------------------------------------------------
// F: all-zero AI Core trend semantics
// ---------------------------------------------------------------------------

// TestB32VNPUUtilAllZeroTrendDrop pins the real .27 shape: every chip reports
// 0% AI Core (an inference gap) but draws power. The util trend row disappears
// because sparkline refuses an all-zero series, while the power row survives.
// Both halves are asserted so the disappearance cannot be mistaken for "no
// data" at the sample level: NPUUtilValid is true.
func TestB32VNPUUtilAllZeroTrendDrop(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32vDetailModel(t, "en")
	m.width = 200
	node := b32vChipNode("node-a", 16, func(int) int { return 0 }, nil)
	m.selectedNode = node
	m.nodeVizHistory = m.nodeVizHistory[:0]
	m.nodeVizNodeName = "node-a"
	for i := 0; i < 3; i++ {
		m.nodeVizHistory = append(m.nodeVizHistory, NodeVizSample{
			At:         b32vBase().Add(time.Duration(i) * time.Second),
			NPUUtilPct: 0, NPUUtilValid: true, NPUPowerW: 1500, HBMPct: 4, HBMValid: true,
		})
	}
	if !m.nodeVizHistory[2].NPUUtilValid {
		t.Fatalf("fixture: the samples must be valid zeros, not gaps")
	}

	out := stripANSI(strings.Join(m.renderNodeNPUTrends(node), "\n"))
	if strings.Contains(out, m.T("nodedetail.npu_util_trend")) {
		t.Errorf("an all-zero AI Core series drew the util trend row (sparkline should refuse):\n%s", out)
	}
	if !strings.Contains(out, m.T("nodedetail.npu_power_trend")) {
		t.Errorf("the power trend row vanished with the util row:\n%s", out)
	}

	// Control: a non-zero util series draws the util row.
	nonZero := b32vChipNode("node-a", 16, func(i int) int { return 20 + i }, nil)
	m.selectedNode = nonZero
	m.nodeVizHistory = m.nodeVizHistory[:0]
	m.nodeVizNodeName = "node-a"
	for i := 0; i < 3; i++ {
		m.nodeVizHistory = append(m.nodeVizHistory, NodeVizSample{
			At:         b32vBase().Add(time.Duration(i) * time.Second),
			NPUUtilPct: float64(20 + i), NPUUtilValid: true, NPUPowerW: 1500,
		})
	}
	rows := strings.Join(m.renderNodeNPUTrends(nonZero), "\n")
	if !strings.Contains(stripANSI(rows), m.T("nodedetail.npu_util_trend")) {
		t.Errorf("a non-zero util series did not draw the util row:\n%s", stripANSI(rows))
	}
}

// ---------------------------------------------------------------------------
// D: heat strip width / clipping order / cell math
// ---------------------------------------------------------------------------

// b32vStripParts splits the rendered strip into the chip body and the legend.
func b32vStripParts(t *testing.T, m *Model, node *model.NodeData) (body, legend string, truncated bool) {
	t.Helper()
	line := stripANSI(m.renderNodeChipHeatStrip(node))
	if line == "" {
		t.Fatalf("no strip rendered")
	}
	prefix := "  " + m.T("nodedetail.heat_strip") + " "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("strip does not start with its label: %q", line)
	}
	rest := strings.TrimPrefix(line, prefix)
	if idx := strings.Index(rest, npuHeatRamp); idx >= 0 {
		// The ramp only occurs contiguously in the legend (chips are 1-space
		// separated), so everything before it is the chip body.
		body, legend = strings.TrimSuffix(rest[:idx], "  "), rest[idx:]
	} else {
		body = rest
	}
	truncated = strings.HasSuffix(strings.TrimRight(line, " "), "...")
	return body, legend, truncated
}

// b32vCountChips counts chip glyphs in a strip body (heat, degraded, fault).
func b32vCountChips(body string) int {
	n := 0
	for _, r := range body {
		if strings.ContainsRune(npuHeatRamp, r) || r == '▒' || r == '▁' {
			n++
		}
	}
	return n
}

// TestB32VHeatStripWidthTable renders the strip at the required widths in both
// locales, records the retained-chip table, and pins the clipping contract: the
// line never exceeds the terminal, truncation is marked with "...", and the
// legend (the tail) disappears before chips are cut.
func TestB32VHeatStripWidthTable(t *testing.T) {
	forcedAsciiProfile(t)

	for _, locale := range []string{"en", "zh"} {
		for _, width := range []int{38, 40, 60, 100} {
			m := b32vDetailModel(t, locale)
			m.width = width
			node := b32vChipNode("node-a", 16, func(i int) int { return 10 + i }, map[int]string{
				5: model.NPUChipGradeDegraded, 11: model.NPUChipGradeFault,
			})

			line := stripANSI(m.renderNodeChipHeatStrip(node))
			body, legend, truncated := b32vStripParts(t, m, node)
			chips := b32vCountChips(body)
			w := runewidth.StringWidth(line)
			t.Logf("locale=%s width=%d lineWidth=%d chips=%d truncated=%v legend=%q",
				locale, width, w, chips, truncated, legend)

			if w > width {
				t.Errorf("%s/%d: strip is %d cells wide: %q", locale, width, w, line)
			}
			if strings.Contains(line, "\n") {
				t.Errorf("%s/%d: strip wrapped: %q", locale, width, line)
			}
			// Clipping order: the legend (the tail) is consumed completely
			// before the first chip is cut.
			if chips < 16 {
				if !truncated {
					t.Errorf("%s/%d: a strip that does not fit has no truncation marker: %q", locale, width, line)
				}
				if legend != "" {
					t.Errorf("%s/%d: a chip was cut while the legend survived: %q", locale, width, line)
				}
			}
			if width >= 100 {
				if chips != 16 {
					t.Errorf("%s/%d: the strip fits but shows %d of 16 chips", locale, width, chips)
				}
				if !strings.Contains(legend, "low→high") && !strings.Contains(legend, "低→高") {
					t.Errorf("%s/%d: the strip fits but the full legend is missing: %q", locale, width, legend)
				}
			}
			if width <= 40 && chips >= 16 {
				t.Errorf("%s/%d: no chip was cut at a width narrower than the strip", locale, width)
			}
		}
	}
}

// TestB32VChipCellMath is an independent recomputation of the strip's glyphs:
// level = floor(aicore/100*6) on the absolute scale, ▒ for degraded, ▁ for
// fault, with a two-space gap at the NPU boundary and single spaces within.
func TestB32VChipCellMath(t *testing.T) {
	forcedAsciiProfile(t)

	ramp := []rune(npuHeatRamp)
	values := []int{0, 16, 33, 50, 66, 83, 100, 100}
	want := make([]string, len(values))
	for i, v := range values {
		want[i] = string(ramp[int(float64(v)/100*float64(len(ramp)-1))])
	}
	want[3] = "▒" // degraded chip keeps its health glyph
	want[7] = "▁" // faulted chip keeps its health glyph

	node := b32vChipNode("node-a", 16, func(i int) int { return values[i%len(values)] }, map[int]string{
		3: model.NPUChipGradeDegraded, 7: model.NPUChipGradeFault,
	})
	m := b32vDetailModel(t, "en")
	m.width = 200
	body, _, _ := b32vStripParts(t, m, node)

	// First NPU's eight cells, in order.
	first := strings.Split(body, npuViewGroupSep)[0]
	got := strings.Fields(first)
	if len(got) != 8 {
		t.Fatalf("first NPU group has %d cells: %q", len(got), first)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("chip %d: strip drew %q, want %q (aicore %d)", i, got[i], want[i], values[i])
		}
	}
	// The boundary is exactly two spaces, inside the group exactly one.
	if !strings.Contains(body, want[7]+"  "+want[0]) {
		t.Errorf("NPU boundary is not a two-space gap: %q", body)
	}
	if !strings.Contains(body, want[0]+" "+want[0]) && !strings.Contains(body, want[0]+" "+want[1]) {
		t.Errorf("chips inside one NPU are not single-space separated: %q", body)
	}
}

// TestB32VHeatStripMatchesNPUView is an independent consistency variant: with
// the NPU view in its default AI Core mode, the same chips draw the same glyph
// string in both surfaces.
func TestB32VHeatStripMatchesNPUView(t *testing.T) {
	forcedAsciiProfile(t)

	node := b32vChipNode("node-d", 16, func(i int) int { return (i * 13) % 100 }, map[int]string{
		2: model.NPUChipGradeDegraded, 14: model.NPUChipGradeFault,
	})
	m := b32vDetailModel(t, "en")
	m.width = 200

	body, _, _ := b32vStripParts(t, m, node)

	// Recompute the matrix cells through the public NPU-view primitives.
	levels := npuHeatLevels(npuChipValues(node, npuMatrixModeAICore), npuMatrixModeAICore)
	matrix := stripANSI(npuMatrixText(npuChipCellRow(node.NPUChipHealth, levels), -1))
	if body != matrix {
		t.Errorf("node-detail strip and NPU view matrix disagree:\n strip  %q\n matrix %q", body, matrix)
	}
	// And the rendered NPU view row contains that exact cell string.
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{Nodes: []*model.NodeData{node}, Summary: &model.ClusterSummary{
		NPUCapacity: 16, NPUAllocatable: 16, NPUNodesCount: 1, LastRefreshTime: b32vBase(),
	}}
	view := stripANSI(m.View())
	if !strings.Contains(view, matrix) {
		t.Errorf("the NPU view does not draw the same chip cells:\n%s", view)
	}
}

// ---------------------------------------------------------------------------
// B: allocatable reference line — widths, locales, four states
// ---------------------------------------------------------------------------

// b32vOverflow returns the node-detail lines wider than width (stripped).
func b32vOverflow(out string, width int) []string {
	var bad []string
	for _, line := range strings.Split(out, "\n") {
		if runewidth.StringWidth(line) > width {
			bad = append(bad, fmt.Sprintf("%d cells: %q", runewidth.StringWidth(line), line))
		}
	}
	return bad
}

// TestB32VReferenceLineWidths renders the resource block across widths and
// locales and reports every line that overflows, so a reference line added
// without a width budget is visible. It pins the 38-column budget (the batch's
// stated floor) in both locales.
func TestB32VReferenceLineWidths(t *testing.T) {
	forcedAsciiProfile(t)

	for _, locale := range []string{"en", "zh"} {
		for _, width := range []int{38, 40, 60, 100} {
			m := b32vDetailModel(t, locale)
			m.width = width
			node := b32vNodeLike("node-a")
			out := stripANSI(m.renderNodeResourceInfo(node))
			alloc := m.T("metrics.allocatable")
			if !strings.Contains(out, alloc) {
				t.Fatalf("%s/%d: reference line missing", locale, width)
			}
			for _, bad := range b32vOverflow(out, width) {
				t.Errorf("%s/%d: node resource block line overflows: %s", locale, width, bad)
			}
		}
	}

	// Below the stated floor, report what overflows (not a failure of the
	// stated contract, but the caller wants the shape).
	for _, width := range []int{30, 20} {
		m := b32vDetailModel(t, "en")
		m.width = width
		out := stripANSI(m.renderNodeResourceInfo(b32vNodeLike("node-a")))
		for _, bad := range b32vOverflow(out, width) {
			t.Logf("width=%d OVERFLOW: %s", width, bad)
		}
	}
}

// TestB32VReferenceLineFourStates independently drives the guard's four states
// and checks the reference percentage is the allocatable ratio (not the
// headline's capacity ratio).
func TestB32VReferenceLineFourStates(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32vDetailModel(t, "en")
	m.width = 100
	alloc := m.T("metrics.allocatable")

	// State 1: denominators differ and usage exists -> line with usage/alloc.
	node := b32vNodeLike("node-a")
	out := stripANSI(m.renderNodeResourceInfo(node))
	cpuRef := lineContaining(out, alloc)
	if !strings.Contains(cpuRef, "1.00") || !strings.Contains(cpuRef, "(10.0%)") {
		t.Errorf("CPU reference = %q, want 1.00 allocatable (10.0%% = 100m/1000m)", cpuRef)
	}
	memRef := lineContainingNext(out, alloc, cpuRef)
	if !strings.Contains(memRef, "1000 B") || !strings.Contains(memRef, "(50.0%)") {
		t.Errorf("memory reference = %q, want 1000 B allocatable (50.0%% = 500/1000)", memRef)
	}
	// The headline still uses capacity: proving the two denominators coexist.
	if cpuLine := lineContaining(out, m.T("detail.field.cpu")); !strings.Contains(cpuLine, "(5.0%)") {
		t.Errorf("CPU headline = %q, want the capacity denominator (5.0%%)", cpuLine)
	}

	// State 2: equal denominators -> no reference.
	equal := b32vNodeLike("node-b")
	equal.CPUCapacity, equal.MemoryCapacity = 1000, 1000
	if out := stripANSI(m.renderNodeResourceInfo(equal)); strings.Contains(out, alloc) {
		t.Errorf("equal denominators still drew a reference line: %q", out)
	}

	// State 3: capacity=0 fallback -> allocatable is already the headline.
	fallback := b32vNodeLike("node-c")
	fallback.CPUCapacity, fallback.MemoryCapacity = 0, 0
	out = stripANSI(m.renderNodeResourceInfo(fallback))
	if strings.Contains(out, alloc) {
		t.Errorf("the capacity fallback still drew a reference line: %q", out)
	}
	if cpuLine := lineContaining(out, m.T("detail.field.cpu")); !strings.Contains(cpuLine, "(10.0%)") {
		t.Errorf("fallback headline = %q, want the allocatable denominator (10.0%%)", cpuLine)
	}

	// State 4: usage 0 -> headline N/A, no fake percentage.
	idle := b32vNodeLike("node-d")
	idle.CPUUsage, idle.MemoryUsage = 0, 0
	if out := stripANSI(m.renderNodeResourceInfo(idle)); strings.Contains(out, alloc) {
		t.Errorf("an N/A usage still drew a reference line: %q", out)
	}
}

// lineContainingNext returns the first line containing substr after the line
// equal to prev (used to reach the memory reference when both share a label).
func lineContainingNext(out, substr, prev string) string {
	seen := false
	for _, line := range strings.Split(out, "\n") {
		if line == prev {
			seen = true
			continue
		}
		if seen && strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// A: node list keeps allocatable
// ---------------------------------------------------------------------------

// TestB32VNodeListStillAllocatable pins the other half of the denominator
// change: the node list's CPU/memory cells keep dividing by allocatable, so a
// user comparing list and detail sees the documented (and labelled) difference
// rather than an unexplained jump.
func TestB32VNodeListStillAllocatable(t *testing.T) {
	forcedAsciiProfile(t)

	m := modelForTest(t, "en")
	m.width, m.height = 200, 40
	m.currentView = ViewNodes
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{b32vNodeLike("node-a")},
		Summary: &model.ClusterSummary{TotalNodes: 1, ReadyNodes: 1, LastRefreshTime: b32vBase()},
	}
	out := stripANSI(m.View())
	// The list header names the columns; the row must carry allocatable values
	// (100m/1.00 CPU = 10%, 500B/1000B = 50%) not capacity.
	row := lineContaining(out, "node-a")
	if !strings.Contains(row, "1.00") {
		t.Errorf("the node list row does not show allocatable CPU: %q", row)
	}
	if !strings.Contains(row, "1000 B") {
		t.Errorf("the node list row does not show allocatable memory: %q", row)
	}
}

// ---------------------------------------------------------------------------
// E: record order — the sample belongs to the re-resolved object
// ---------------------------------------------------------------------------

// TestB32VRecordsFreshObject pins that the recorder runs after the re-resolve
// pass: a refresh that replaces node-a's object (same name, new usage) must
// record the new usage. A recorder placed before reresolveDetailSelections
// would keep reading the pointer captured on entry.
func TestB32VRecordsFreshObject(t *testing.T) {
	m := b32vDetailModel(t, "en")
	stale := b32vNodeLike("node-a")
	m.selectedNode = stale

	// First refresh installs the fresh object.
	fresh := b32vNodeLike("node-a")
	fresh.CPUUsage = 1000 // 50% of capacity
	m.Update(clusterDataMsg{data: &model.ClusterData{
		Nodes:   []*model.NodeData{fresh},
		Summary: &model.ClusterSummary{LastRefreshTime: b32vBase()},
	}})
	if m.selectedNode != fresh {
		t.Fatalf("the detail was not re-pointed at the fresh object")
	}
	last := m.nodeVizHistory[len(m.nodeVizHistory)-1]
	if last.CPUPct != 50 {
		t.Errorf("recorded CPUPct = %v, want 50 (fresh usage / capacity)", last.CPUPct)
	}
}

// ---------------------------------------------------------------------------
// P2 candidate: cross-node trend bleed before the next refresh
// ---------------------------------------------------------------------------

// TestB32VCrossNodeTrendBleed pins the review finding that motivated the
// render-side node identity check: recordNodeVizSample empties the ring only
// when the next advancing snapshot arrives, so entering another node's detail
// before that must not draw the previous node's sparklines (nodeVizSeriesFor
// guards this). The probe found the bleed as delivered; it is now the
// regression test for the fix.
func TestB32VCrossNodeTrendBleed(t *testing.T) {
	forcedAsciiProfile(t)

	m := b32vDetailModel(t, "en")
	nodeA := b32vChipNode("node-a", 16, func(i int) int { return 40 + i }, nil)
	m.selectedNode = nodeA
	// Fill node-a's session ring through the real guarded path.
	for i := 0; i < 3; i++ {
		m.Update(clusterDataMsg{data: &model.ClusterData{
			Nodes:   []*model.NodeData{nodeA},
			Summary: &model.ClusterSummary{LastRefreshTime: b32vBase().Add(time.Duration(i) * time.Second)},
		}})
	}
	if len(m.nodeVizHistory) != 3 || m.nodeVizNodeName != "node-a" {
		t.Fatalf("fixture: ring has %d samples for %q", len(m.nodeVizHistory), m.nodeVizNodeName)
	}

	// Enter node-b's detail the way the node list does: selection and view
	// change, no refresh yet.
	nodeB := b32vChipNode("node-b", 16, func(i int) int { return 10 }, nil)
	m.selectedNode = nodeB
	m.currentView = ViewNodeDetail
	m.detailMode = true
	m.detailScrollOffset = 0

	out := stripANSI(m.renderNodeResourceInfo(nodeB))
	cpuLine := lineContaining(out, m.T("detail.field.cpu"))
	if strings.ContainsAny(cpuLine, sparkRamp) {
		t.Errorf("node-b's CPU row drew node-a's session before any refresh of node-b: %q", cpuLine)
	}
	npuRows := stripANSI(strings.Join(m.renderNodeNPUTrends(nodeB), "\n"))
	if strings.Contains(npuRows, m.T("nodedetail.npu_util_trend")) {
		t.Errorf("node-b's NPU block drew node-a's AI Core trend before any refresh: %q", npuRows)
	}

	// After one successful refresh of node-b the ring is node-b's alone.
	m.Update(clusterDataMsg{data: &model.ClusterData{
		Nodes:   []*model.NodeData{nodeB},
		Summary: &model.ClusterSummary{LastRefreshTime: b32vBase().Add(10 * time.Second)},
	}})
	if m.nodeVizNodeName != "node-b" || len(m.nodeVizHistory) != 1 {
		t.Errorf("after the refresh the ring is %q/%d, want node-b/1", m.nodeVizNodeName, len(m.nodeVizHistory))
	}
}
