package ui

// b29v_heat_test.go — independent adversarial probe for batch 29's heat modes
// (written by the verifier, not the implementer). It deliberately re-derives
// the cells through the model types instead of calling the implementer's own
// tests, and compares graded cells byte for byte (styles included) against the
// health matrix rather than by substring.

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29vGradeNode builds an NPU node from chips and explicit grades. The grade
// slice is what the matrix renders; the chips are what the heat modes read.
func b29vGradeNode(name string, chips []model.NPUChipData, grades []model.NPUChipGrade) *model.NodeData {
	return &model.NodeData{
		Name: name, Status: "Ready",
		NPUCapacity: int64(len(chips)), NPUAllocatable: int64(len(chips)),
		NPUChips: chips, NPUChipHealth: grades,
	}
}

// b29vHealthyNode is n chips, two per NPU, all healthy.
func b29vHealthyNode(name string, n int) *model.NodeData {
	chips := make([]model.NPUChipData, n)
	grades := make([]model.NPUChipGrade, n)
	for i := 0; i < n; i++ {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK", Temp: 46, Power: 190}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	return b29vGradeNode(name, chips, grades)
}

// b29vHeatModel builds the NPU view over one cluster with a matrix mode.
func b29vHeatModel(data *model.ClusterData, mode int) *Model {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 200, 90
	m.currentView = ViewNPU
	m.clusterData = data
	m.npuMatrixMode = mode
	return m
}

// b29vFlattenCells flattens the grouped cell rows to canonical chip order.
func b29vFlattenCells(groups [][]string) []string {
	var cells []string
	for _, group := range groups {
		cells = append(cells, group...)
	}
	return cells
}

// TestB29VGradedCellsIdenticalInEveryMode is stronger than a "contains the
// degraded glyph" probe: for every non-healthy chip the heat-mode cell must be
// the exact string — glyph and ANSI style — the health matrix renders, at the
// same index. A heat mode that recoloured a degraded chip or moved it in the
// row fails here.
func TestB29VGradedCellsIdenticalInEveryMode(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })

	node := b29vHealthyNode("graded", 8)
	node.NPUChipHealth[1].Grade = model.NPUChipGradeDegraded
	node.NPUChipHealth[4].Grade = model.NPUChipGradeFault
	node.NPUChipHealth[6].Grade = model.NPUChipGradeDegraded
	for i := range node.NPUChips {
		node.NPUChips[i].AICore = i * 12
		node.NPUChips[i].Temp = 40 + i*3
		node.NPUChips[i].Power = 100 + float64(i)*30
	}

	health := b29vFlattenCells(npuChipCellRow(node.NPUChipHealth, nil))
	if len(health) != 8 {
		t.Fatalf("health matrix has %d cells, want 8", len(health))
	}

	for _, mode := range []int{npuMatrixModeAICore, npuMatrixModeTemp, npuMatrixModePower} {
		levels := npuHeatLevels(npuChipValues(node, mode))
		cells := b29vFlattenCells(npuChipCellRow(node.NPUChipHealth, levels))
		if len(cells) != 8 {
			t.Fatalf("mode %d has %d cells, want 8", mode, len(cells))
		}
		for i, grade := range node.NPUChipHealth {
			switch grade.Grade {
			case model.NPUChipGradeDegraded, model.NPUChipGradeFault:
				if cells[i] != health[i] {
					t.Errorf("mode %d chip %d (%s): heat cell %q, health cell %q — the grade semantics changed",
						mode, i, grade.Grade, cells[i], health[i])
				}
			default:
				if !strings.ContainsRune(npuHeatRamp, []rune(stripANSI(cells[i]))[0]) {
					t.Errorf("mode %d healthy chip %d: cell %q is not a heat glyph", mode, i, cells[i])
				}
				if stripANSI(cells[i]) == stripANSI(health[i]) {
					t.Errorf("mode %d healthy chip %d still draws the health glyph %q", mode, i, stripANSI(cells[i]))
				}
			}
		}
		// Non-vacuousness: the heat ramp really moves across the healthy chips.
		seen := map[string]bool{}
		for i, grade := range node.NPUChipHealth {
			if grade.Grade == model.NPUChipGradeHealthy {
				seen[stripANSI(cells[i])] = true
			}
		}
		if len(seen) < 2 {
			t.Errorf("mode %d drew %d distinct heat glyphs; the probe is vacuous", mode, len(seen))
		}
	}
}

// TestB29VHeatMonotoneTempAndPower pins the ordering property the AI-Core mode
// is tested for in the implementer's suite, but for the two other metrics and
// against a shuffled value set (so a hard-coded position mapping cannot pass).
func TestB29VHeatMonotoneTempAndPower(t *testing.T) {
	node := b29vHealthyNode("mono", 8)
	temps := []int{52, 41, 67, 46, 58, 44, 61, 49}
	powers := []float64{180, 240, 150, 210, 300, 170, 260, 195}
	for i := range node.NPUChips {
		node.NPUChips[i].Temp = temps[i]
		node.NPUChips[i].Power = powers[i]
	}

	for _, mode := range []int{npuMatrixModeTemp, npuMatrixModePower} {
		levels := npuHeatLevels(npuChipValues(node, mode))
		for i := range node.NPUChips {
			for j := range node.NPUChips {
				vi := npuHeatValue(node.NPUChips[i], mode)
				vj := npuHeatValue(node.NPUChips[j], mode)
				if vi < vj && levels[i] > levels[j] {
					t.Errorf("mode %d: chip %d (value %v) has level %d, above chip %d (value %v) level %d",
						mode, i, vi, levels[i], j, vj, levels[j])
				}
			}
		}
		minLevel, maxLevel := levels[0], levels[0]
		for _, l := range levels {
			if l < minLevel {
				minLevel = l
			}
			if l > maxLevel {
				maxLevel = l
			}
		}
		if maxLevel-minLevel < 1 {
			t.Errorf("mode %d: the ramp does not move across the shuffled chips (min %d, max %d)", mode, minLevel, maxLevel)
		}
	}
}

// TestB29VHeatAllEqualDrawsMidLevel pins the flat-node case: a node whose heat
// metric is identical on every chip renders the declared middle level (▄), not
// the top glyph and not a health glyph.
func TestB29VHeatAllEqualDrawsMidLevel(t *testing.T) {
	node := b29vHealthyNode("flat", 8) // all Temp 46
	m := b29vHeatModel(&model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}, npuMatrixModeTemp)
	forcedAsciiProfile(t)

	row := stripANSI(m.npuMatrixRow(node, false))
	if strings.Contains(row, "▉") {
		t.Errorf("the flat temp matrix still draws a health glyph: %q", row)
	}
	if got := strings.Count(row, "▅"); got != 8 {
		t.Errorf("the flat temp matrix has %d middle-level cells, want 8: %q", got, row)
	}
	levels := npuHeatLevels(npuChipValues(node, npuMatrixModeTemp))
	for i, level := range levels {
		if want := (len([]rune(npuHeatRamp)) - 1) / 2; level != want {
			t.Errorf("flat node chip %d level = %d, want the middle %d", i, level, want)
		}
	}
}

// TestB29VHeatIsNodeLocalForPower repeats the node-local normalization probe
// for the power metric: two nodes with disjoint wattage bands must both span
// the whole ramp (identical rows), because a cluster-wide scale would flatten
// one of them.
func TestB29VHeatIsNodeLocalForPower(t *testing.T) {
	cool := b29vHealthyNode("cool", 8)
	warm := b29vHealthyNode("warm", 8)
	for i := range cool.NPUChips {
		cool.NPUChips[i].Power = float64(i)       // 0..7 W
		warm.NPUChips[i].Power = 300 + float64(i) // 300..307 W
	}
	levelsCool := npuHeatLevels(npuChipValues(cool, npuMatrixModePower))
	levelsWarm := npuHeatLevels(npuChipValues(warm, npuMatrixModePower))
	if len(levelsCool) != 8 || len(levelsWarm) != 8 {
		t.Fatalf("levels %d/%d, want 8/8", len(levelsCool), len(levelsWarm))
	}
	for i := range levelsCool {
		if levelsCool[i] != levelsWarm[i] {
			t.Errorf("chip %d: cool level %d, warm level %d — the range is not node-local",
				i, levelsCool[i], levelsWarm[i])
		}
	}
	if levelsCool[0] != 0 || levelsCool[7] != len([]rune(npuHeatRamp))-1 {
		t.Errorf("the node-local ramp is %d..%d, want 0..%d",
			levelsCool[0], levelsCool[7], len([]rune(npuHeatRamp))-1)
	}
}

// TestB29VHeatNoColorCarriesTheValue pins the --no-color path through the real
// renderer: no escapes, at least two distinct ramp heights, and the graded
// chips still distinguishable from the ramp.
func TestB29VHeatNoColorCarriesTheValue(t *testing.T) {
	node := b29vHealthyNode("mono", 8)
	for i := range node.NPUChips {
		node.NPUChips[i].AICore = i * 10
	}
	node.NPUChipHealth[3].Grade = model.NPUChipGradeDegraded
	node.NPUChipHealth[5].Grade = model.NPUChipGradeFault
	m := b29vHeatModel(&model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}, npuMatrixModeAICore)
	forcedAsciiProfile(t)

	out := m.renderNPUView()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("the no-colour heat view carries ANSI escapes")
	}
	row := lineContaining(out, "mono")
	if row == "" {
		t.Fatalf("no node row in:\n%s", out)
	}
	distinct := map[rune]bool{}
	for _, r := range row {
		if strings.ContainsRune(npuHeatRamp, r) {
			distinct[r] = true
		}
	}
	if len(distinct) < 2 {
		t.Errorf("the no-colour heat row has %d distinct heights: %q", len(distinct), row)
	}
	if !strings.Contains(row, "▒") {
		t.Errorf("the degraded chip's glyph is gone in --no-color: %q", row)
	}
}

// TestB29VHeatCellIsOneCellWide pins the matrix alignment assumption: every
// heat cell must occupy exactly one display cell in both colour modes, or the
// NPU grouping spaces land on the wrong chips.
func TestB29VHeatCellIsOneCellWide(t *testing.T) {
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		saved := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(profile)
		for level := 0; level < len([]rune(npuHeatRamp)); level++ {
			cell := npuHeatCell(level)
			if got := runewidth.StringWidth(stripANSI(cell)); got != 1 {
				t.Errorf("profile %v level %d: cell %q is %d cells wide", profile, level, stripANSI(cell), got)
			}
		}
		lipgloss.SetColorProfile(saved)
	}
}

// TestB29VHeatLegendClipsOnDisplayWidth pins the narrow-terminal legend: it is
// clipped to the terminal, never wraps, and the clip never leaves an invalid or
// replacement rune (the legend carries wide CJK glyphs and an arrow).
func TestB29VHeatLegendClipsOnDisplayWidth(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
		m.width, m.height = 200, 90
		m.currentView = ViewNPU
		m.clusterData = &model.ClusterData{
			Nodes:   []*model.NodeData{b29vHealthyNode("n", 8)},
			Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
		}

		if got := m.npuHeatLegend(); got != "" {
			t.Errorf("%s: health mode rendered a heat legend: %q", locale, got)
		}
		for _, mode := range []int{npuMatrixModeAICore, npuMatrixModeTemp, npuMatrixModePower} {
			m.npuMatrixMode = mode
			for width := 1; width <= 40; width++ {
				m.width = width
				line := m.npuHeatLegend()
				if !utf8.ValidString(line) {
					t.Fatalf("%s mode %d width %d: legend is not valid UTF-8", locale, mode, width)
				}
				if strings.ContainsRune(line, utf8.RuneError) {
					t.Errorf("%s mode %d width %d: legend carries a replacement rune: %q", locale, mode, width, line)
				}
				if strings.Contains(line, "\n") {
					t.Errorf("%s mode %d width %d: legend wraps: %q", locale, mode, width, line)
				}
				if got := runewidth.StringWidth(stripANSI(line)); got > width {
					t.Errorf("%s mode %d width %d: legend is %d cells wide: %q", locale, mode, width, got, line)
				}
			}
		}
	}
}
