package ui

// b30v_probe_test.go — independent adversarial probes for batch 30 (verifier).
//
// These probes are written from the expected behaviour, not from the shipped
// assertions: percentage modes must be absolute and clamped, sensors stay
// node-local, the empty-HBM cell must not collide with the fault glyph, and the
// footer must advertise `h` only where the key actually works.

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b30vNode builds an n-chip single-node cluster piece: healthy chips with
// explicit metrics.
func b30vNode(name string, n int) *model.NodeData {
	chips := make([]model.NPUChipData, n)
	grades := make([]model.NPUChipGrade, n)
	for i := 0; i < n; i++ {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK"}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	return &model.NodeData{
		Name: name, Status: "Ready",
		NPUCapacity: int64(n), NPUAllocatable: int64(n),
		NPUChips: chips, NPUChipHealth: grades,
	}
}

func b30vModel(t *testing.T, locale string, nodes ...*model.NodeData) *Model {
	t.Helper()
	forcedAsciiProfile(t)
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width, m.height = 200, 60
	m.currentView = ViewNPU
	var cap int64
	for _, n := range nodes {
		cap += n.NPUCapacity
	}
	m.clusterData = &model.ClusterData{
		Nodes:   nodes,
		Summary: &model.ClusterSummary{NPUCapacity: cap, NPUAllocatable: cap, NPUNodesCount: len(nodes)},
	}
	return m
}

func b30vGlyphs(row string) []rune {
	var out []rune
	for _, r := range row {
		if strings.ContainsRune(npuHeatRamp, r) {
			out = append(out, r)
		}
	}
	return out
}

// --- B: normalization boundaries ------------------------------------------

// TestB30VPercentLevelBoundaries pins the absolute ramp at and around every
// floor boundary. A one-off in the formula or an off-by-one clamp shows here.
func TestB30VPercentLevelBoundaries(t *testing.T) {
	top := len([]rune(npuHeatRamp)) - 1
	if top != 6 {
		t.Fatalf("ramp has %d steps, fixture arithmetic assumes 7", top+1)
	}
	cases := []struct {
		v    float64
		want int
	}{
		{0, 0}, {15, 0}, {16, 0}, {100.0 / 6, 1}, // 16.666… %, first step
		{17, 1}, {33, 1}, {33.34, 2}, {50, 3}, {50.01, 3}, {66, 3},
		{83, 4}, {83.34, 5}, {99, 5}, {99.99, 5}, {100, 6}, {150, 6},
		{-0.5, 0}, {-5, 0}, {-1000, 0},
	}
	for _, c := range cases {
		got := npuHeatLevels([]float64{c.v}, npuMatrixModeAICore)[0]
		if got != c.want {
			t.Errorf("AICore %v%% -> level %d, want %d", c.v, got, c.want)
		}
	}

	// The HBM path runs through used/total*100; the same boundaries must hold,
	// including clamping dirty data at both ends.
	hbmCases := []struct {
		used, total int64
		want        int
	}{
		{0, 100, 0}, {15, 100, 0}, {17, 100, 1}, {50, 100, 3},
		{99, 100, 5}, {100, 100, 6}, {200, 100, 6}, {-50, 100, 0}, {10, 0, 0}, {0, 0, 0},
	}
	for _, c := range hbmCases {
		chip := model.NPUChipData{HBMUsed: c.used, HBMTotal: c.total}
		got := npuHeatLevels([]float64{npuHeatValue(chip, npuMatrixModeHBM)}, npuMatrixModeHBM)[0]
		if got != c.want {
			t.Errorf("HBM %d/%d -> level %d, want %d", c.used, c.total, got, c.want)
		}
	}
}

// TestB30VPercentSameRatioSameLevelAcrossCapacities asserts that capacity does
// not enter the scale: the same ratio on any total draws the same glyph.
func TestB30VPercentSameRatioSameLevelAcrossCapacities(t *testing.T) {
	totals := []int64{1, 8, 64, 1024, 65536, 1 << 40}
	for _, ratio := range []float64{0, 7, 25, 50, 90, 100} {
		want := npuHeatLevels([]float64{ratio}, npuMatrixModeHBM)[0]
		for _, total := range totals {
			used := int64(ratio / 100 * float64(total))
			chip := model.NPUChipData{HBMUsed: used, HBMTotal: total}
			got := npuHeatLevels([]float64{npuHeatValue(chip, npuMatrixModeHBM)}, npuMatrixModeHBM)[0]
			// Integer truncation of used can move a hair off the ratio for tiny
			// totals; only totals with enough resolution are held exact.
			if total >= 1024 && got != want {
				t.Errorf("%v%% of %d MB -> level %d, want %d (the level must not depend on capacity)",
					ratio, total, got, want)
			}
			if total >= 1024 && used > 0 && ratio < 100 {
				// non-vacuousness: the chip really does report a ratio
				if chip.HBMUsed == 0 {
					t.Fatalf("fixture bug: %v%% of %d is zero", ratio, total)
				}
			}
		}
	}
}

// TestB30VPercentIgnoresNeighbourChips: with two nodes at disjoint constant
// utilisation, each row is drawn solely from its own absolute percentages. A
// node-local scale (or a cluster-wide one) would move at least one row.
func TestB30VPercentIgnoresNeighbourChips(t *testing.T) {
	hot := b30vNode("hot", 8)
	for i := range hot.NPUChips {
		hot.NPUChips[i].AICore = 90
	}
	cold := b30vNode("cold", 8)
	for i := range cold.NPUChips {
		cold.NPUChips[i].AICore = 10
	}
	// floor(90/100*6)=5 -> ▇; floor(10/100*6)=0 -> ▂.
	wantHot := strings.Repeat("▇", 8)
	wantCold := strings.Repeat("▂", 8)

	hotAlone := stripANSI(b30vModel(t, "en", hot).npuMatrixRow(hot, false))
	coldAlone := stripANSI(b30vModel(t, "en", cold).npuMatrixRow(cold, false))
	if got := string(b30vGlyphs(hotAlone)); got != wantHot {
		t.Errorf("hot node alone draws %q, want %q", got, wantHot)
	}
	if got := string(b30vGlyphs(coldAlone)); got != wantCold {
		t.Errorf("cold node alone draws %q, want %q", got, wantCold)
	}

	together := b30vModel(t, "en", hot, cold)
	out := stripANSI(together.renderNPUView())
	if got := string(b30vGlyphs(lineContaining(out, "hot"))); got != wantHot {
		t.Errorf("hot node next to a cold node draws %q, want the same absolute %q", got, wantHot)
	}
	if got := string(b30vGlyphs(lineContaining(out, "cold"))); got != wantCold {
		t.Errorf("cold node next to a hot node draws %q, want the same absolute %q", got, wantCold)
	}
}

// TestB30VSensorFlatAndSingleChipMidLevel: a one-chip sensor node and a flat
// sensor node both land on the node-local middle.
func TestB30VSensorFlatAndSingleChipMidLevel(t *testing.T) {
	top := len([]rune(npuHeatRamp)) - 1
	one := b30vNode("one", 1)
	one.NPUChips[0].Temp = 46
	if got := npuHeatLevels(npuChipValues(one, npuMatrixModeTemp), npuMatrixModeTemp)[0]; got != top/2 {
		t.Errorf("single-chip temp node level %d, want middle %d", got, top/2)
	}
	flat := b30vNode("flat", 8)
	for i := range flat.NPUChips {
		flat.NPUChips[i].Power = 500
	}
	for i, lvl := range npuHeatLevels(npuChipValues(flat, npuMatrixModePower), npuMatrixModePower) {
		if lvl != top/2 {
			t.Errorf("flat power chip %d level %d, want middle %d", i, lvl, top/2)
		}
	}
	// Two flat nodes with different wattages both stay on the same middle.
	hotter := b30vNode("hotter", 8)
	for i := range hotter.NPUChips {
		hotter.NPUChips[i].Power = 900
	}
	for i, lvl := range npuHeatLevels(npuChipValues(hotter, npuMatrixModePower), npuMatrixModePower) {
		if lvl != top/2 {
			t.Errorf("flat 900 W chip %d level %d, want middle %d", i, lvl, top/2)
		}
	}
}

// --- C: HBM semantics and the glyph non-intersection contract --------------

// TestB30VEmptyHBMKeepsGradeGlyphs: the empty-HBM chip is a bottom-ramp ▂, not
// the fault ▁; degraded and faulted chips keep ▒/▁ even though their HBM is
// empty or their ratio is the highest in the row.
func TestB30VEmptyHBMKeepsGradeGlyphs(t *testing.T) {
	node := b30vNode("mixed", 4)
	node.NPUChips[0] = model.NPUChipData{NPUID: 0, Chip: 0, Health: "OK", HBMUsed: 0, HBMTotal: 0}          // empty, healthy
	node.NPUChips[1] = model.NPUChipData{NPUID: 0, Chip: 1, Health: "OK", HBMUsed: 0, HBMTotal: 1000}       // 0 %, healthy
	node.NPUChips[2] = model.NPUChipData{NPUID: 0, Chip: 0, Health: "Error", HBMUsed: 1000, HBMTotal: 1000} // degraded, full
	node.NPUChips[3] = model.NPUChipData{NPUID: 0, Chip: 1, Health: "Error", HBMUsed: 0, HBMTotal: 0}       // fault, empty
	node.NPUChipHealth = []model.NPUChipGrade{
		{NPUID: 0, Chip: 0, Grade: model.NPUChipGradeHealthy},
		{NPUID: 0, Chip: 1, Grade: model.NPUChipGradeHealthy},
		{NPUID: 0, Chip: 0, Grade: model.NPUChipGradeDegraded},
		{NPUID: 0, Chip: 1, Grade: model.NPUChipGradeFault},
	}

	levels := npuHeatLevels(npuChipValues(node, npuMatrixModeHBM), npuMatrixModeHBM)
	groups := npuChipCellRow(node.NPUChipHealth, levels)
	if len(groups) != 1 || len(groups[0]) != 4 {
		t.Fatalf("cell groups = %v, want one group of four", groups)
	}
	var cells []string
	for _, c := range groups[0] {
		cells = append(cells, stripANSI(c))
	}
	want := []string{"▂", "▂", "▒", "▁"}
	for i := range want {
		if cells[i] != want[i] {
			t.Errorf("cell %d = %q, want %q (cells %v)", i, cells[i], want[i], cells)
		}
	}
	// The healthy empty-HBM chip must not borrow the fault glyph.
	if cells[0] == "▁" {
		t.Errorf("the empty-HBM healthy chip renders the fault glyph")
	}

	// Ramp and grade glyphs must stay disjoint in both directions: that is what
	// keeps a bottom-of-ramp cell distinguishable from a fault in every mode.
	ramp := []rune(npuHeatRamp)
	for level := 0; level < len(ramp); level++ {
		g := stripANSI(npuHeatCell(level))
		for _, reserved := range []string{"▁", "▒", "▉"} {
			if g == reserved {
				t.Errorf("ramp level %d renders the grade glyph %q", level, reserved)
			}
		}
	}
	for _, grade := range []string{model.NPUChipGradeHealthy, model.NPUChipGradeDegraded, model.NPUChipGradeFault} {
		g := stripANSI(npuGradeCell(grade))
		if strings.ContainsRune(npuHeatRamp, []rune(g)[0]) {
			t.Errorf("the grade glyph for %q is a ramp glyph %q", grade, g)
		}
		// In a heat mode the healthy chips are shaded (that is the mode's
		// point), but a non-healthy grade must never be shaded: its glyph
		// stays the grade glyph whatever the level says.
		if grade != model.NPUChipGradeHealthy {
			for level := 0; level <= len(ramp)-1; level++ {
				cell := stripANSI(npuHeatModeCell(model.NPUChipGrade{Grade: grade}, level))
				if cell != g {
					t.Errorf("grade %q at level %d renders %q, want the grade glyph %q", grade, level, cell, g)
				}
			}
		}
	}
}

// TestB30VHBMEmptyMatchesAggregateOmission: the per-chip matrix draws an empty
// HBM as ▂ while the aggregate bar omits its HBM segment for the same data —
// two intentional readings of "nothing reported", asserted together so a later
// change has to face both.
func TestB30VHBMEmptyMatchesAggregateOmission(t *testing.T) {
	node := b30vNode("nohbm", 8) // HBMUsed 0, HBMTotal 0 everywhere
	m := b30vModel(t, "en", node)
	m.npuMatrixMode = npuMatrixModeHBM

	row := stripANSI(m.npuMatrixRow(node, false))
	if got := string(b30vGlyphs(row)); got != strings.Repeat("▂", 8) {
		t.Errorf("empty HBM row draws %q, want ▂×8", got)
	}
	bar := stripANSI(strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | "))
	if strings.Contains(strings.ToUpper(bar), "HBM") {
		t.Errorf("the aggregate bar shows an HBM segment without capacity: %q", bar)
	}
}

// --- D: footer / legend ----------------------------------------------------

// TestB30VFooterHKeyOnlyWhereItWorks walks the list modes and the blocked
// modes, in both locales.
func TestB30VFooterHKeyOnlyWhereItWorks(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := b30vModel(t, locale, b30vNode("npu-a", 4))
		m.width = 200
		desc := m.T("keys.npu_matrix")
		seg := stripANSI(RenderKeyBinding("h", desc))

		for _, view := range []ViewType{
			ViewOverview, ViewNodes, ViewPods, ViewWorkloads,
			ViewNetwork, ViewStorage, ViewEvents, ViewAlerts,
		} {
			m.currentView = view
			if f := stripANSI(m.renderFooter()); strings.Contains(f, seg) || strings.Contains(f, desc) {
				t.Errorf("%s: view %v advertises %q in the footer: %q", locale, view, seg, f)
			}
		}

		m.currentView = ViewNPU
		if f := stripANSI(m.renderFooter()); !strings.Contains(f, seg) {
			t.Errorf("%s: the NPU footer lacks %q: %q", locale, seg, f)
		}

		blocked := map[string]func(*Model){
			"detail":        func(m *Model) { m.detailMode = true },
			"search":        func(m *Model) { m.searchMode = true },
			"filter":        func(m *Model) { m.filterMode = true },
			"logs":          func(m *Model) { m.logsMode = true },
			"logsSearch":    func(m *Model) { m.logsMode = true; m.logsSearchMode = true },
			"commandOutput": func(m *Model) { m.commandOutputMode = true },
		}
		for name, set := range blocked {
			b := b30vModel(t, locale, b30vNode("npu-a", 4))
			b.width = 200
			set(b)
			if f := stripANSI(b.renderFooter()); strings.Contains(f, seg) {
				t.Errorf("%s: %s mode advertises the matrix key where h is blocked: %q", locale, name, f)
			}
		}
	}
}

// TestB30VFooterNarrowDoesNotWrap: at 38 columns the NPU footer stays one line
// inside the budget, whether or not the h segment survives the clip.
func TestB30VFooterNarrowDoesNotWrap(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := b30vModel(t, locale, b30vNode("npu-a", 4))
		m.width = 38
		f := stripANSI(m.renderFooter())
		if strings.Contains(f, "\n") {
			t.Errorf("%s: the 38-column footer wraps: %q", locale, f)
		}
		if visualLength(f) > 38 {
			t.Errorf("%s: the 38-column footer is %d cells wide: %q", locale, visualLength(f), f)
		}
	}
}

// TestB30VLegendLineEveryMode: every mode renders exactly one mode legend,
// above the static glyph legend and below the aggregate bar; only the health
// mode omits the ramp. This is the discoverability contract from the outside.
func TestB30VLegendLineEveryMode(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		for _, mode := range []int{
			npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp,
			npuMatrixModePower, npuMatrixModeHealth,
		} {
			node := b30vNode("npu-a", 4)
			for i := range node.NPUChips {
				node.NPUChips[i].AICore = i * 20
				node.NPUChips[i].HBMUsed = int64(i * 300)
				node.NPUChips[i].HBMTotal = 1000
				node.NPUChips[i].Temp = 40 + i
				node.NPUChips[i].Power = 100 + float64(i)*10
			}
			m := b30vModel(t, locale, node)
			m.npuMatrixMode = mode
			out := stripANSI(m.renderNPUView())
			legendMarker := "Mode:"
			if locale == "zh" {
				legendMarker = "模式："
			}
			var legendLines int
			var legendIdx, glyphIdx, aggIdx = -1, -1, -1
			glyphLegend := stripANSI(m.T("npuview.legend"))
			aggMarker := "Nodes "
			if locale == "zh" {
				aggMarker = "节点 "
			}
			for i, line := range strings.Split(out, "\n") {
				if strings.Contains(line, legendMarker) {
					legendLines++
					legendIdx = i
				}
				if strings.TrimSpace(line) == glyphLegend {
					glyphIdx = i
				}
				if strings.HasPrefix(line, aggMarker) {
					aggIdx = i
				}
			}
			if legendLines != 1 {
				t.Errorf("%s mode %d: %d mode-legend lines, want exactly 1:\n%s", locale, mode, legendLines, out)
			}
			if glyphIdx < 0 {
				t.Fatalf("%s mode %d: static glyph legend missing", locale, mode)
			}
			if legendIdx < 0 || legendIdx > glyphIdx {
				t.Errorf("%s mode %d: the mode legend (%d) is not above the glyph legend (%d)",
					locale, mode, legendIdx, glyphIdx)
			}
			if aggIdx >= 0 && legendIdx < aggIdx {
				t.Errorf("%s mode %d: the mode legend (%d) is above the aggregate bar (%d)",
					locale, mode, legendIdx, aggIdx)
			}
			legendText := ""
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, legendMarker) {
					legendText = line
				}
			}
			hasRamp := strings.Contains(legendText, npuHeatRamp)
			if mode == npuMatrixModeHealth && hasRamp {
				t.Errorf("%s: the health legend carries a ramp: %q", locale, legendText)
			}
			if mode != npuMatrixModeHealth && !hasRamp {
				t.Errorf("%s mode %d: legend lacks the ramp: %q", locale, mode, legendText)
			}
		}
	}
}

// TestB30VCycleWrapsFromHealth: from any start, one full cycle returns to the
// same mode, and health is followed by AI Core.
func TestB30VCycleWrapsFromHealth(t *testing.T) {
	m := b30vModel(t, "en", b30vNode("npu-a", 4))
	m.npuMatrixMode = npuMatrixModeHealth
	m.Update(keyMsg("h"))
	if m.npuMatrixMode != npuMatrixModeAICore {
		t.Fatalf("health -> h landed on %d, want AI Core 0", m.npuMatrixMode)
	}
	for _, start := range []int{npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp, npuMatrixModePower, npuMatrixModeHealth} {
		m.npuMatrixMode = start
		for i := 0; i < npuMatrixModeCount; i++ {
			m.Update(keyMsg("h"))
		}
		if m.npuMatrixMode != start {
			t.Errorf("a full cycle from mode %d ended at %d", start, m.npuMatrixMode)
		}
	}
}

// TestB30VHKeyNoEffectOnOtherViewsAndBlockedModes is a second, independent lock
// on the guard: the mode must never move on other views.
func TestB30VHKeyNoEffectOnOtherViewsAndBlockedModes(t *testing.T) {
	m := b30vModel(t, "en", b30vNode("npu-a", 4))
	m.currentView = ViewNodes
	m.Update(keyMsg("h"))
	if m.npuMatrixMode != npuMatrixModeAICore {
		t.Errorf("h on the nodes view changed the mode to %d", m.npuMatrixMode)
	}
	m.currentView = ViewNPU
	m.detailMode = true
	m.Update(keyMsg("h"))
	if m.npuMatrixMode != npuMatrixModeAICore {
		t.Errorf("h in detail mode changed the mode to %d", m.npuMatrixMode)
	}
}

// --- zero-drift cross-checks ----------------------------------------------

// TestB30VAggregateBarAndCardModeIndependent is an independent restatement of
// the zero-drift claim: the aggregate bar and the overview NPU card must not
// mention or move with the matrix mode.
func TestB30VAggregateBarAndCardModeIndependent(t *testing.T) {
	var firstBar, firstCard string
	for i, mode := range []int{
		npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp,
		npuMatrixModePower, npuMatrixModeHealth,
	} {
		m := b30vModel(t, "en", b30vNode("npu-a", 8))
		for j := range m.clusterData.Nodes[0].NPUChips {
			c := &m.clusterData.Nodes[0].NPUChips[j]
			c.AICore = 10 + j*10
			c.HBMUsed = int64(j * 500)
			c.HBMTotal = 4096
			c.Temp = 40 + j
			c.Power = 150 + float64(j)*5
		}
		m.npuMatrixMode = mode
		bar := stripANSI(strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | "))
		card := stripANSI(strings.Join(m.npuPanelLines(m.clusterData.Summary), "\n"))
		if i == 0 {
			firstBar, firstCard = bar, card
		} else if bar != firstBar || card != firstCard {
			t.Errorf("mode %d changed the aggregate bar or card\n got bar  %q\n got card %q\nwant bar  %q\nwant card %q",
				mode, bar, card, firstBar, firstCard)
		}
	}
}

// TestB30VActionMenuFooterStale documents (rather than endorses) a pre-existing
// footer limitation the new h segment inherits: the action-menu overlay does not
// switch renderFooter to a menu branch, so the list footer — every list key, not
// just h — stays on screen while viewSwitchBlocked() makes all of them no-ops.
// Batch 30 adds one more such binding; it is not a new class of defect.
func TestB30VActionMenuFooterStale(t *testing.T) {
	m := b30vModel(t, "en", b30vNode("npu-a", 4))
	m.width = 200
	m.actionMenuMode = true
	footer := stripANSI(m.renderFooter())
	if !strings.Contains(footer, stripANSI(RenderKeyBinding("h", m.T("keys.npu_matrix")))) {
		t.Fatalf("expected the documented stale-footer behaviour (h still advertised): %q", footer)
	}
	before := m.npuMatrixMode
	m.Update(keyMsg("h"))
	if m.npuMatrixMode != before {
		t.Errorf("h changed the mode while the action menu owns the keyboard: %d -> %d", before, m.npuMatrixMode)
	}
	// The same staleness applies to the older list keys, which is what makes
	// this a pre-existing footer limitation rather than an h-specific one.
	if !strings.Contains(footer, stripANSI(RenderKeyBinding("s", m.T("keys.sort")))) {
		t.Errorf("the stale footer does not show the other list keys either: %q", footer)
	}
}

// TestB30VHBMExactRationalBoundary documents the double-rounding asymmetry at
// exact k/6 boundaries: the AICore path feeds the percentage straight into the
// ramp, while the HBM path recomputes it as used/total*100 first, which lands a
// hair below the boundary for some k. The visible effect is at most one cell on
// data that reports exactly k/6; the probe records the levels and requires the
// sequence to stay monotone. Anything worse than the float hair would fail.
func TestB30VHBMExactRationalBoundary(t *testing.T) {
	for k := 1; k <= 6; k++ {
		aicore := npuHeatLevels([]float64{100.0 / 6 * float64(k)}, npuMatrixModeAICore)[0]
		chip := model.NPUChipData{HBMUsed: int64(k), HBMTotal: 6}
		hbm := npuHeatLevels([]float64{npuHeatValue(chip, npuMatrixModeHBM)}, npuMatrixModeHBM)[0]
		t.Logf("k=%d/6: AICore level %d, HBM level %d", k, aicore, hbm)
		if hbm < aicore-1 || hbm > aicore+1 {
			t.Errorf("k=%d/6: HBM level %d differs from the AICore level %d by more than the float hair",
				k, hbm, aicore)
		}
		if k > 1 {
			prev := npuHeatLevels([]float64{npuHeatValue(model.NPUChipData{HBMUsed: int64(k - 1), HBMTotal: 6}, npuMatrixModeHBM)}, npuMatrixModeHBM)[0]
			if hbm < prev {
				t.Errorf("k=%d/6: HBM level %d is below k=%d/6 level %d", k, hbm, k-1, prev)
			}
		}
	}
}

// TestB30VUnknownChipRendersAsIdleInDefaultMode documents an inherited semantic
// limitation that the new default view now puts on the first screen: a chip
// whose exporter reported nothing (Health "Unknown") is graded healthy and its
// zero AICore shades exactly like a genuinely idle chip, while the aggregate bar
// is careful to omit its utilisation mean for the same chips. Batch 30 did not
// introduce the loss (batch 29's heat modes and the health matrix both lose it),
// but it is now the default reading, so the batch report records it.
func TestB30VUnknownChipRendersAsIdleInDefaultMode(t *testing.T) {
	node := b30vNode("npu-a", 2)
	for i := range node.NPUChips {
		node.NPUChips[i].Health = "Unknown"
		node.NPUChips[i].AICore = 0
	}
	m := b30vModel(t, "en", node) // zero value = AI Core mode
	if m.npuMatrixMode != npuMatrixModeAICore {
		t.Fatalf("fixture expects the default AI Core mode, got %d", m.npuMatrixMode)
	}
	row := stripANSI(m.npuMatrixRow(node, false))
	if got := string(b30vGlyphs(row)); got != "▂▂" {
		t.Fatalf("unknown-health chips draw %q, want the documented idle-looking ▂▂", got)
	}
	bar := stripANSI(strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | "))
	if strings.Contains(bar, "Utilization") {
		t.Errorf("the aggregate bar shows a utilisation mean for all-Unknown chips: %q", bar)
	}
	t.Logf("documented: the matrix shows ▂▂ while the bar omits utilisation — %q", bar)
}
