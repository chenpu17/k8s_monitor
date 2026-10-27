package ui

// b29_heat_test.go — batch 29 (overview visualisation), contract T-heat.
//
// The chip matrix gains heat modes behind the `h` key. Batch 30 reordered the
// modes so the default is AI Core utilisation and added the HBM mode; the
// health matrix is now the last mode, selected explicitly. The hard line this
// file still defends is that the health matrix itself is byte for byte what it
// was before the modes existed, and that heat is an addition to the health
// semantics, never a replacement for them.

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29IdleNode is the real-cluster idle shape captured on the A3 cluster: 16
// healthy chips, two per NPU, zero AI Core utilisation, 46 °C, one wattage per
// chip and an empty HBM. It is the fixture the health-mode byte anchor and the
// heat-mode monotonicity probes are both built on.
func b29IdleNode(name string) *model.NodeData {
	chips := make([]model.NPUChipData, 16)
	grades := make([]model.NPUChipGrade, 16)
	for i := range chips {
		chips[i] = model.NPUChipData{
			NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK",
			AICore: 0, Temp: 46, Power: 190.5, HBMUsed: 0, HBMTotal: 65536,
		}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	return &model.NodeData{
		Name: name, Status: "Ready", NPUCapacity: 16, NPUAllocatable: 16,
		NPUChips: chips, NPUChipHealth: grades,
	}
}

// b29HeatModel is the NPU view over the idle fixture, at a width wide enough for
// every chip and the suffix.
func b29HeatModel(t *testing.T, locale string) *Model {
	t.Helper()
	forcedAsciiProfile(t)
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width, m.height = 120, 60
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{b29IdleNode("192.168.2.27"), b29IdleNode("192.168.2.77")},
		Summary: &model.ClusterSummary{NPUCapacity: 32, NPUAllocatable: 32, NPUNodesCount: 2},
	}
	return m
}

// b29MatrixRows returns the node rows of the view, in order.
func b29MatrixRows(m *Model) []string {
	var rows []string
	for _, node := range m.npuNodes() {
		rows = append(rows, stripANSI(m.npuMatrixRow(node, false)))
	}
	return rows
}

// TestB29HealthMatrixBytesUnchanged is the zero-drift anchor for the health
// matrix: both locale fixtures at four widths, compared against the bytes the
// pre-batch tree produced for the same fixture (captured with a detached-HEAD
// dump and frozen here). Batch 30 made AI Core the default, so the health mode
// is selected explicitly here; the matrix bytes are still the pre-batch ones.
//
// The rows below are the whole matrix as the user sees it. They are asserted
// individually rather than as one blob so a failure names the width that broke.
func TestB29HealthMatrixBytesUnchanged(t *testing.T) {
	want := map[int][]string{
		38: {
			"  192.1...  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉ …",
			"  192.1...  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉ …",
		},
		60: {
			"  192.168.2.27  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
			"  192.168.2.77  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
		},
		100: {
			"  192.168.2.27  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
			"  192.168.2.77  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
		},
		160: {
			"  192.168.2.27  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
			"  192.168.2.77  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉",
		},
	}

	for _, locale := range []string{"en", "zh"} {
		for width, rows := range want {
			m := b29HeatModel(t, locale)
			m.width = width
			// Batch 30: AI Core is the default; this anchor is about the
			// health matrix's own bytes, so select it explicitly.
			m.npuMatrixMode = npuMatrixModeHealth
			got := b29MatrixRows(m)
			if len(got) != len(rows) {
				t.Fatalf("%s/%d: %d rows, want %d", locale, width, len(got), len(rows))
			}
			for i := range rows {
				if got[i] != rows[i] {
					t.Errorf("%s/%d row %d changed:\n got %q\nwant %q", locale, width, i, got[i], rows[i])
				}
			}
		}
	}
}

// TestB29HealthModeIgnoresChipMetrics pins the same rule from the other side:
// filling every chip with a distinct utilisation, temperature and power must not
// move a single byte of the health matrix. A heat leak into the health mode
// would show up here even if the idle anchor above somehow matched.
func TestB29HealthModeIgnoresChipMetrics(t *testing.T) {
	base := b29HeatModel(t, "en")
	base.npuMatrixMode = npuMatrixModeHealth
	before := b29MatrixRows(base)

	moved := b29HeatModel(t, "en")
	moved.npuMatrixMode = npuMatrixModeHealth
	for _, node := range moved.clusterData.Nodes {
		for i := range node.NPUChips {
			node.NPUChips[i].AICore = i * 6
			node.NPUChips[i].Temp = 40 + i
			node.NPUChips[i].Power = 100 + float64(i)*13
		}
	}
	after := b29MatrixRows(moved)

	for i := range before {
		if before[i] != after[i] {
			t.Errorf("row %d changed when only the chip metrics moved:\n idle %q\nbusy %q", i, before[i], after[i])
		}
	}
	// Non-vacuousness: the fixture really does differ under a heat mode.
	moved.npuMatrixMode = npuMatrixModeAICore
	if heat := b29MatrixRows(moved); heat[0] == before[0] {
		t.Fatalf("probe vacuous: the heat matrix equals the health matrix")
	}
}

// TestB29HeatModeMonotoneInMetric pins the heat encoding's ordering: on one
// node, a chip with a higher metric never draws a lower glyph than a chip with a
// lower one. The comparison is on the gap ramp, which is what a reader decodes.
func TestB29HeatModeMonotoneInMetric(t *testing.T) {
	// A node whose chips carry a strict ascending utilisation.
	node := b29IdleNode("node-heat")
	for i := range node.NPUChips {
		node.NPUChips[i].AICore = i * 6
	}

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 160, 60
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 16, NPUAllocatable: 16},
	}
	m.npuMatrixMode = npuMatrixModeAICore

	row := stripANSI(m.npuMatrixRow(node, false))
	// Pull the heat glyphs out of the matrix, in chip order. The row is
	// "name  gap  cell cell ..."; the heat ramp is the only glyph set present.
	var glyphs []rune
	for _, r := range row {
		if strings.ContainsRune(npuHeatRamp, r) {
			glyphs = append(glyphs, r)
		}
	}
	if len(glyphs) != len(node.NPUChips) {
		t.Fatalf("row carries %d heat glyphs for %d chips: %q", len(glyphs), len(node.NPUChips), row)
	}

	prevIdx := -1
	for i, g := range glyphs {
		idx := strings.IndexRune(npuHeatRamp, g)
		if idx < prevIdx {
			t.Errorf("chip %d (AICore %d) draws %q (index %d), lower than chip %d's index %d",
				i, node.NPUChips[i].AICore, string(g), idx, i-1, prevIdx)
		}
		prevIdx = idx
	}
	// Non-vacuousness: the ascending utilisation really spans the ramp.
	if strings.Count(string(glyphs), string(glyphs[0])) == len(glyphs) {
		t.Fatalf("probe vacuous: every chip drew the same glyph %q", string(glyphs[0]))
	}
}

// TestB29HeatModeKeepsGradeSemantics is the "heat is not health" contract: in
// every heat mode a degraded chip keeps the degraded glyph and its warning
// style, and a faulted chip keeps the fault glyph and its danger style. Heat
// only ever recolours the healthy cells.
func TestB29HeatModeKeepsGradeSemantics(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	node := b29IdleNode("node-graded")
	node.NPUChipHealth[3].Grade = model.NPUChipGradeDegraded
	node.NPUChipHealth[7].Grade = model.NPUChipGradeFault
	node.NPUChipHealth[10].Grade = model.NPUChipGradeDegraded
	for i := range node.NPUChips {
		node.NPUChips[i].AICore = i * 6
		node.NPUChips[i].Temp = 40 + i
		node.NPUChips[i].Power = 100 + float64(i)*13
	}

	for _, mode := range []int{npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp, npuMatrixModePower} {
		m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
		m.width, m.height = 160, 60
		m.currentView = ViewNPU
		m.clusterData = &model.ClusterData{
			Nodes:   []*model.NodeData{node},
			Summary: &model.ClusterSummary{NPUCapacity: 16, NPUAllocatable: 16},
		}
		m.npuMatrixMode = mode

		row := m.npuMatrixRow(node, false)
		if !strings.Contains(row, StyleWarning.Render("▒")) {
			t.Errorf("mode %d lost the degraded chip's glyph and style: %q", mode, row)
		}
		if !strings.Contains(row, StyleStatusNotReady.Render("▁")) {
			t.Errorf("mode %d lost the faulted chip's glyph and style: %q", mode, row)
		}
		// Non-vacuousness: the healthy cells really are heat-encoded, so the
		// graded cells above are an override rather than the whole row.
		stripped := stripANSI(row)
		if strings.Contains(stripped, "▉") {
			t.Errorf("mode %d still draws a health glyph for a healthy chip: %q", mode, stripped)
		}
	}
}

// TestB29HeatModeCycle pins the `h` key: from the AI Core default it steps
// through HBM, temperature, power and health back to AI Core, only on the NPU
// view, and never while a mode that owns the keyboard is open.
func TestB29HeatModeCycle(t *testing.T) {
	m := b29HeatModel(t, "en")

	var got []int
	for i := 0; i < 5; i++ {
		got = append(got, m.npuMatrixMode)
		m.Update(keyMsg("h"))
	}
	want := []int{npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp, npuMatrixModePower, npuMatrixModeHealth}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("press %d: mode %d, want %d (cycle %v)", i, got[i], want[i], got)
		}
	}

	// Off the NPU view the key does nothing.
	other := b29HeatModel(t, "en")
	other.currentView = ViewOverview
	other.Update(keyMsg("h"))
	if other.npuMatrixMode != npuMatrixModeAICore {
		t.Errorf("'h' on the overview changed the matrix mode to %d", other.npuMatrixMode)
	}

	// In search mode it stays a typed character.
	searching := b29HeatModel(t, "en")
	searching.searchMode = true
	searching.Update(keyMsg("h"))
	if searching.npuMatrixMode != npuMatrixModeAICore {
		t.Errorf("'h' in search mode changed the matrix mode to %d", searching.npuMatrixMode)
	}
	if !strings.Contains(searching.searchText, "h") {
		t.Errorf("'h' in search mode was not typed into the search text: %q", searching.searchText)
	}
}

// TestB29HeatModeNoColorStaysReadable pins the --no-color contract the batch
// inherits from batch 25: with the colour channel gone the glyph height alone
// must still separate a hot chip from a cold one, and the graded cells must
// still be distinguishable from both.
func TestB29HeatModeNoColorStaysReadable(t *testing.T) {
	node := b29IdleNode("node-heat")
	for i := range node.NPUChips {
		node.NPUChips[i].AICore = i * 6
	}
	node.NPUChipHealth[7].Grade = model.NPUChipGradeFault

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 160, 60
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 16, NPUAllocatable: 16},
	}
	m.npuMatrixMode = npuMatrixModeAICore

	out := m.renderNPUView()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("the no-colour render carries ANSI escapes:\n%q", out)
	}
	row := lineContaining(out, "node-heat")
	if row == "" {
		t.Fatalf("no node row:\n%s", out)
	}
	// At least two distinct ramp heights survive, plus the fault glyph.
	distinct := map[rune]bool{}
	for _, r := range row {
		if strings.ContainsRune(npuHeatRamp, r) {
			distinct[r] = true
		}
	}
	if len(distinct) < 2 {
		t.Errorf("the no-colour heat row has %d distinct heights, want at least 2: %q", len(distinct), row)
	}
	if !strings.Contains(row, "▁") {
		t.Errorf("the faulted chip's glyph is gone in --no-color: %q", row)
	}
}

// TestB29HeatModeKeepsNPUGrouping pins that the heat encoding does not disturb
// the matrix layout: the NPU boundary (the two-space group separator) sits where
// the health matrix puts it, so a heat cell never reads as belonging to the
// wrong NPU.
func TestB29HeatModeKeepsNPUGrouping(t *testing.T) {
	node := b29IdleNode("node-heat")
	// Four chips per NPU, the collector path's non-default shape.
	for i := range node.NPUChips {
		node.NPUChips[i].NPUID = i / 4
		node.NPUChips[i].Chip = i % 4
		node.NPUChips[i].AICore = i * 6
		node.NPUChipHealth[i].NPUID = i / 4
		node.NPUChipHealth[i].Chip = i % 4
	}

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 200, 60
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 16, NPUAllocatable: 16},
	}
	m.npuMatrixMode = npuMatrixModeAICore

	// The same grouping the health matrix produces, with the cells shaded: four
	// per NPU, a double gap between groups, one space between cells. Batch 30
	// made AI Core absolute, so the levels are floor(AICore/100*6) for the
	// 0,6,…,90 fixture: 0,0,0,1 1,1,2,2 2,3,3,3 4,4,5,5.
	groups := npuChipCellRow(node.NPUChipHealth, npuHeatLevels(npuChipValues(node, npuMatrixModeAICore), npuMatrixModeAICore))
	if len(groups) != 4 {
		t.Fatalf("the heat matrix has %d NPU groups, want 4", len(groups))
	}
	if got, want := stripANSI(npuMatrixText(groups, -1)),
		"▂ ▂ ▂ ▃  ▃ ▃ ▄ ▄  ▄ ▅ ▅ ▅  ▆ ▆ ▇ ▇"; got != want {
		t.Errorf("the heat matrix text = %q, want the health matrix's layout with shaded cells %q", got, want)
	}
	// And the same layout is what the health matrix produces for the same
	// grades, proving the grouping is shared rather than re-implemented.
	health := npuMatrixText(npuChipCellGroups(node.NPUChipHealth), -1)
	if strings.Count(health, " ") != strings.Count(stripANSI(npuMatrixText(groups, -1)), " ") {
		t.Errorf("the heat matrix's spacing (%q) differs from the health matrix's (%q)",
			stripANSI(npuMatrixText(groups, -1)), health)
	}
}

// TestB29HeatCellsCarryDistinctColors pins the second channel: in a colour
// terminal the ramp levels are also coloured, so a reader who cannot resolve the
// glyph heights still sees a gradient. The styles must be distinct per level and
// monotone in brightness is not asserted (that is a design choice, not a
// contract) — only that hotter levels are not the same colour as colder ones.
func TestB29HeatCellsCarryDistinctColors(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	seen := map[string]int{}
	sgr := map[string]int{}
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for level := 0; level < len([]rune(npuHeatRamp)); level++ {
		cell := npuHeatCell(level)
		if !strings.Contains(cell, "\x1b[") {
			t.Fatalf("level %d cell %q carries no colour", level, cell)
		}
		if prev, dup := seen[cell]; dup {
			t.Errorf("levels %d and %d render the same cell %q", prev, level, cell)
		}
		seen[cell] = level

		// The colour itself must move with the level: a fixed colour would leave
		// the gradient in the glyph only, which is what the --no-color path
		// already covers.
		colour := re.FindString(cell)
		if prev, dup := sgr[colour]; dup {
			t.Errorf("levels %d and %d share the colour %q", prev, level, colour)
		}
		sgr[colour] = level
	}
	if len(seen) != len([]rune(npuHeatRamp)) {
		t.Errorf("%d distinct heat cells for %d levels", len(seen), len([]rune(npuHeatRamp)))
	}
	if len(sgr) != len([]rune(npuHeatRamp)) {
		t.Errorf("%d distinct heat colours for %d levels", len(sgr), len([]rune(npuHeatRamp)))
	}
}

// TestB29HeatNormalizationSplitsByMetricKind pins the two scales batch 30
// established. The percentage modes (AI Core, HBM) have an absolute 0-100
// scale: the same percentage draws the same glyph on every node, whatever the
// node's own range is. The sensor modes (temperature, power) keep the
// node-local min-max scale, because a wattage only ranks within its own board.
// The non-vacuousness clause keeps the probe honest: the sensor fixture really
// does span the full ramp.
func TestB29HeatNormalizationSplitsByMetricKind(t *testing.T) {
	cool := b29IdleNode("node-cool")
	warm := b29IdleNode("node-warm")
	for i := range cool.NPUChips {
		cool.NPUChips[i].AICore = i      // 0..15 %
		warm.NPUChips[i].AICore = 80 + i // 80..95 %
		cool.NPUChips[i].Power = float64(i)
		warm.NPUChips[i].Power = 300 + float64(i)
	}

	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 200, 60
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{cool, warm},
		Summary: &model.ClusterSummary{NPUCapacity: 32, NPUAllocatable: 32},
	}
	m.npuMatrixMode = npuMatrixModeAICore

	// AI Core: absolute. The cool node's 0..15 % is the bottom of the ramp on
	// every chip; the warm node's 80..95 % is the top two levels. A node-local
	// scale would stretch both nodes across the whole ramp.
	coolGlyphs := heatGlyphs(stripANSI(m.npuMatrixRow(cool, false)))
	warmGlyphs := heatGlyphs(stripANSI(m.npuMatrixRow(warm, false)))
	if len(coolGlyphs) != 16 || len(warmGlyphs) != 16 {
		t.Fatalf("glyph counts %d/%d, want 16/16", len(coolGlyphs), len(warmGlyphs))
	}
	if got := strings.Count(string(coolGlyphs), "▂"); got != 16 {
		t.Errorf("the 0..15%% node draws %q; 15%% is still below one ramp step, want all ▂", string(coolGlyphs))
	}
	for i, g := range warmGlyphs {
		if g != '▆' && g != '▇' {
			t.Errorf("the 80..95%% node chip %d draws %q, want ▆/▇ from the absolute scale", i, string(g))
		}
	}

	// Power: still node-local. Two disjoint wattage bands draw identical rows.
	levelsCool := npuHeatLevels(npuChipValues(cool, npuMatrixModePower), npuMatrixModePower)
	levelsWarm := npuHeatLevels(npuChipValues(warm, npuMatrixModePower), npuMatrixModePower)
	for i := range levelsCool {
		if levelsCool[i] != levelsWarm[i] {
			t.Errorf("power chip %d: cool level %d, warm level %d — the sensor range is not node-local",
				i, levelsCool[i], levelsWarm[i])
		}
	}
	if levelsCool[0] != 0 || levelsCool[15] != len([]rune(npuHeatRamp))-1 {
		t.Errorf("the node-local power ramp is %d..%d, want 0..%d",
			levelsCool[0], levelsCool[15], len([]rune(npuHeatRamp))-1)
	}
}

// heatGlyphs extracts the heat-ramp glyphs of a row, in chip order.
func heatGlyphs(row string) []rune {
	var glyphs []rune
	for _, r := range row {
		if strings.ContainsRune(npuHeatRamp, r) {
			glyphs = append(glyphs, r)
		}
	}
	return glyphs
}

// TestB29HeatLegendInEveryMode pins the legend line: it appears in every mode
// (batch 30 made the health legend part of the h-key discoverability work), it
// names the metric and the key, fits the terminal, and only the heat modes
// carry the ramp. The health legend must not show a ramp: the health matrix has
// no gradient, and two legends reading the same glyphs differently is exactly
// the collision batch 29 fixed.
func TestB29HeatLegendInEveryMode(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := b29HeatModel(t, locale)
		for _, mode := range []int{npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp, npuMatrixModePower, npuMatrixModeHealth} {
			m.npuMatrixMode = mode
			line := stripANSI(m.npuHeatLegend())
			if line == "" {
				t.Fatalf("%s: mode %d rendered no mode legend", locale, mode)
			}
			if !strings.Contains(line, stripANSI(m.npuHeatModeName(mode))) {
				t.Errorf("%s: mode %d legend %q does not name the metric %q",
					locale, mode, line, stripANSI(m.npuHeatModeName(mode)))
			}
			if !strings.Contains(line, "h") {
				t.Errorf("%s: mode %d legend %q does not name the h key", locale, mode, line)
			}
			if mode == npuMatrixModeHealth {
				if strings.Contains(line, npuHeatRamp) {
					t.Errorf("%s: the health legend carries the heat ramp: %q", locale, line)
				}
			} else if !strings.Contains(line, npuHeatRamp) {
				t.Errorf("%s: mode %d legend %q does not show the ramp", locale, mode, line)
			}
			// Bounded by the width, never wrapped. The check is done at the
			// fixture width, where the whole legend is on screen.
			if w := visualLength(m.npuHeatLegend()); w > m.width {
				t.Errorf("%s: mode %d legend is %d cells wide on a %d-cell terminal: %q",
					locale, mode, w, m.width, m.npuHeatLegend())
			}
			// At a width narrower than the legend the line is clipped, not
			// wrapped: one line out, no overflow.
			savedWidth := m.width
			m.width = 38
			if got := m.npuHeatLegend(); visualLength(got) > 38 || strings.Contains(got, "\n") {
				t.Errorf("%s: mode %d legend overflows or wraps at 38 columns: %q", locale, mode, got)
			}
			m.width = savedWidth
		}
	}
}
