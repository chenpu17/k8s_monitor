package ui

// b30_matrix_test.go — batch 30: NPU matrix default mode, HBM heat view and the
// two normalization scales.
//
// The batch reorders the matrix modes so the first thing a user sees is AI Core
// utilisation (the number they asked for), adds an HBM utilisation mode, and
// splits the normalization in two: percentages have an absolute 0-100 scale,
// sensors keep the node-local min-max. The health matrix is still byte for byte
// what it always was, but it is now the last mode in the cycle.
//
// Every contract below has a RED mutation on record in the delivery report.

import (
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b30Node builds an NPU node whose chips carry every heat metric. The values
// are distinct so a heat row that accidentally renders the health matrix or a
// flat line is visibly wrong.
func b30Node(name string, n int) *model.NodeData {
	chips := make([]model.NPUChipData, n)
	grades := make([]model.NPUChipGrade, n)
	for i := 0; i < n; i++ {
		chips[i] = model.NPUChipData{
			NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK",
			AICore:   i * 12,              // 0..84 %
			HBMUsed:  int64(i * 10),       // 0..70 % of 100 MB
			HBMTotal: 100,                 //
			Temp:     40 + i,              // 40..47 °C
			Power:    100 + float64(i)*10, // 100..170 W
		}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	return &model.NodeData{
		Name: name, Status: "Ready",
		NPUCapacity: int64(n), NPUAllocatable: int64(n),
		NPUChips: chips, NPUChipHealth: grades,
	}
}

// b30Model returns the NPU view over a one-node cluster.
func b30Model(t *testing.T, locale string, mode int) *Model {
	t.Helper()
	forcedAsciiProfile(t)
	m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
	m.width, m.height = 160, 60
	m.currentView = ViewNPU
	m.clusterData = &model.ClusterData{
		Nodes:   []*model.NodeData{b30Node("npu-a", 8)},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8, NPUNodesCount: 1},
	}
	m.npuMatrixMode = mode
	return m
}

// b30RampGlyphs returns the heat-ramp glyphs of a row, in chip order.
func b30RampGlyphs(row string) []rune {
	var glyphs []rune
	for _, r := range row {
		if strings.ContainsRune(npuHeatRamp, r) {
			glyphs = append(glyphs, r)
		}
	}
	return glyphs
}

// T-default: a fresh model opens the NPU view in the AI Core heat mode, and the
// first render carries the heat glyphs plus the mode legend line. Both are part
// of the discoverability requirement: the user sees what the matrix means and
// that `h` changes it without having to find the key first.
func TestB30DefaultMatrixModeIsAICore(t *testing.T) {
	m := b30Model(t, "en", npuMatrixModeAICore)
	if npuMatrixModeAICore != 0 {
		t.Fatalf("npuMatrixModeAICore = %d, want the zero value so a fresh model defaults to it", npuMatrixModeAICore)
	}

	fresh := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	if fresh.npuMatrixMode != npuMatrixModeAICore {
		t.Fatalf("a fresh model starts in mode %d, want AI Core", fresh.npuMatrixMode)
	}

	out := stripANSI(m.renderNPUView())
	row := lineContaining(out, "npu-a")
	if row == "" {
		t.Fatalf("no node row:\n%s", out)
	}
	if strings.Contains(row, "▉") {
		t.Errorf("the default matrix still draws the health glyph: %q", row)
	}
	if got := len(b30RampGlyphs(row)); got != 8 {
		t.Errorf("the default matrix has %d heat glyphs, want one per chip (8): %q", got, row)
	}

	legend := lineContaining(out, "Mode:")
	if legend == "" {
		t.Fatalf("no mode legend line in the default view:\n%s", out)
	}
	if !strings.Contains(legend, "AI Core utilization") || !strings.Contains(legend, "h to switch") ||
		!strings.Contains(legend, npuHeatRamp) {
		t.Errorf("the default legend does not name AI Core, the key and the ramp: %q", legend)
	}

	// The matrix and the legend sit above the static health glyph legend, so a
	// reader meets the mode line first.
	if idxMode, idxGlyph := strings.Index(out, legend), strings.Index(out, m.T("npuview.legend")); idxMode < 0 || idxMode > idxGlyph {
		t.Errorf("the mode legend is not above the static glyph legend (%d vs %d)", idxMode, idxGlyph)
	}
}

// T-hbm: the HBM mode reads used/total, is normalized on the absolute 0-100
// scale (the same percentage draws the same glyph even when the capacities
// differ), and maps a node that reports no HBM capacity to the honest bottom of
// the ramp rather than inventing a ratio.
func TestB30HBMHeatMode(t *testing.T) {
	// Value extraction, including the no-capacity case.
	chip := model.NPUChipData{HBMUsed: 250, HBMTotal: 1000}
	if got, want := npuHeatValue(chip, npuMatrixModeHBM), 25.0; got != want {
		t.Errorf("HBM heat value = %v, want %v", got, want)
	}
	if got := npuHeatValue(model.NPUChipData{HBMUsed: 10, HBMTotal: 0}, npuMatrixModeHBM); got != 0 {
		t.Errorf("a chip with no HBM capacity has heat value %v, want 0", got)
	}

	// Monotone in the ratio, on a series that really spans 0-100 %.
	node := b30Node("mono", 8)
	for i := range node.NPUChips {
		node.NPUChips[i].HBMUsed = int64(i) * 100 / 7 // 0,14,…,100 %
		node.NPUChips[i].HBMTotal = 100
	}
	levels := npuHeatLevels(npuChipValues(node, npuMatrixModeHBM), npuMatrixModeHBM)
	for i := 1; i < len(levels); i++ {
		if levels[i] < levels[i-1] {
			t.Errorf("HBM chip %d (level %d) is below chip %d (level %d) on an ascending ratio",
				i, levels[i], i-1, levels[i-1])
		}
	}
	if levels[0] != 0 || levels[7] != len([]rune(npuHeatRamp))-1 {
		t.Errorf("the ascending HBM ratio spans levels %d..%d, want 0..%d",
			levels[0], levels[7], len([]rune(npuHeatRamp))-1)
	}

	// Absolute scale: the same 40 % on two nodes with different capacities
	// yields the same levels. floor(40/100*6) = 2 → ▄. A node-local scale would
	// put a flat 40 % node at the middle (▅).
	small := b30UniformHBM("small", 40, 100)
	large := b30UniformHBM("large", 400, 1000)
	smallLevels := npuHeatLevels(npuChipValues(small, npuMatrixModeHBM), npuMatrixModeHBM)
	largeLevels := npuHeatLevels(npuChipValues(large, npuMatrixModeHBM), npuMatrixModeHBM)
	mid := int(40.0 / 100 * float64(len([]rune(npuHeatRamp))-1))
	if mid != 2 {
		t.Fatalf("fixture arithmetic: floor(40%%*6) = %d, want the ▄ level 2", mid)
	}
	for i := range smallLevels {
		if smallLevels[i] != mid {
			t.Errorf("small node chip %d level = %d, want the absolute ▄ level %d", i, smallLevels[i], mid)
		}
		if smallLevels[i] != largeLevels[i] {
			t.Errorf("chip %d: 40%% of 100 MB is level %d, 40%% of 1000 MB is level %d — the scale is not absolute",
				i, smallLevels[i], largeLevels[i])
		}
	}

	// All-zero HBM is the honest zero: every chip draws the bottom level ▂.
	empty := b30UniformHBM("empty", 0, 65536)
	for i, level := range npuHeatLevels(npuChipValues(empty, npuMatrixModeHBM), npuMatrixModeHBM) {
		if level != 0 {
			t.Errorf("empty HBM chip %d level = %d, want 0 (▂)", i, level)
		}
	}

	// End to end: the rendered row is all ▄ for the flat 40 % node and carries
	// no health glyph.
	m := b30Model(t, "en", npuMatrixModeHBM)
	m.clusterData.Nodes = []*model.NodeData{small}
	out := stripANSI(m.renderNPUView())
	row := lineContaining(out, "small")
	if row == "" {
		t.Fatalf("no node row in the HBM mode:\n%s", out)
	}
	if got := strings.Count(row, "▄"); got != 8 {
		t.Errorf("the flat 40%% HBM row has %d ▄ cells, want 8: %q", got, row)
	}
	if strings.Contains(row, "▉") {
		t.Errorf("the HBM matrix still draws a health glyph: %q", row)
	}
}

// b30UniformHBM builds an n-chip node whose chips all report the given HBM
// used/total pair.
func b30UniformHBM(name string, used, total int64) *model.NodeData {
	node := b30Node(name, 8)
	for i := range node.NPUChips {
		node.NPUChips[i].HBMUsed = used
		node.NPUChips[i].HBMTotal = total
	}
	return node
}

// T-split: the percentage modes use the absolute 0-100 scale (all-zero → ▂, all
// 40 % → ▄), the sensor modes keep the node-local range (all equal → ▅). The
// two behaviours are asserted side by side so a change that collapses them into
// one scale fails in both directions.
func TestB30NormalizationSplitsPercentFromSensor(t *testing.T) {
	top := len([]rune(npuHeatRamp)) - 1

	zero := b30Node("zero", 8)
	for i := range zero.NPUChips {
		zero.NPUChips[i].AICore = 0
	}
	for i, level := range npuHeatLevels(npuChipValues(zero, npuMatrixModeAICore), npuMatrixModeAICore) {
		if level != 0 {
			t.Errorf("all-zero AI Core chip %d level = %d, want 0 (▂): 0 %% is an honest zero", i, level)
		}
	}

	forty := b30Node("forty", 8)
	for i := range forty.NPUChips {
		forty.NPUChips[i].AICore = 40
	}
	for i, level := range npuHeatLevels(npuChipValues(forty, npuMatrixModeAICore), npuMatrixModeAICore) {
		if level != 2 {
			t.Errorf("flat 40 %% AI Core chip %d level = %d, want the absolute ▄ level 2", i, level)
		}
	}

	flatTemp := b30Node("flat", 8)
	for i := range flatTemp.NPUChips {
		flatTemp.NPUChips[i].Temp = 46
	}
	for i, level := range npuHeatLevels(npuChipValues(flatTemp, npuMatrixModeTemp), npuMatrixModeTemp) {
		if level != top/2 {
			t.Errorf("flat 46 °C chip %d level = %d, want the node-local middle %d (▅)", i, level, top/2)
		}
	}
	if 2 == top/2 {
		t.Fatalf("probe vacuous: the absolute ▄ and the sensor middle are the same level")
	}

	// The sensor scale still moves on a real spread.
	spread := b30Node("spread", 8)
	levels := npuHeatLevels(npuChipValues(spread, npuMatrixModePower), npuMatrixModePower)
	if levels[0] != 0 || levels[len(levels)-1] != top {
		t.Errorf("a spread power series spans %d..%d, want 0..%d", levels[0], levels[len(levels)-1], top)
	}
}

// T-cycle: `h` walks AI Core → HBM → temperature → power → health → AI Core,
// and a full loop reproduces the starting bytes exactly (legend included).
func TestB30ModeCycleOrder(t *testing.T) {
	m := b30Model(t, "en", npuMatrixModeAICore)
	before := m.renderNPUView()

	want := []int{
		npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp,
		npuMatrixModePower, npuMatrixModeHealth, npuMatrixModeAICore,
	}
	var got []int
	for i := 0; i < npuMatrixModeCount; i++ {
		got = append(got, m.npuMatrixMode)
		m.Update(keyMsg("h"))
	}
	got = append(got, m.npuMatrixMode)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("press %d: mode %d, want %d (cycle %v)", i, got[i], want[i], got)
		}
	}
	if after := m.renderNPUView(); after != before {
		t.Errorf("a full cycle did not return to the starting bytes:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if npuMatrixModeCount != 5 {
		t.Errorf("npuMatrixModeCount = %d, want 5 (AI Core, HBM, temp, power, health)", npuMatrixModeCount)
	}
}

// T-legend: all five modes carry a mode legend naming the metric and the key;
// only the health mode omits the ramp; every locale's legend is clipped (never
// wrapped) at the 38-column budget.
func TestB30LegendInEveryMode(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		m := b30Model(t, locale, npuMatrixModeAICore)
		for _, mode := range []int{
			npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp,
			npuMatrixModePower, npuMatrixModeHealth,
		} {
			m.npuMatrixMode = mode
			line := stripANSI(m.npuHeatLegend())
			if line == "" {
				t.Fatalf("%s: mode %d has no legend", locale, mode)
			}
			if !strings.Contains(line, stripANSI(m.npuHeatModeName(mode))) {
				t.Errorf("%s: mode %d legend %q does not name the mode", locale, mode, line)
			}
			if !strings.Contains(line, "h") {
				t.Errorf("%s: mode %d legend %q does not point at the h key", locale, mode, line)
			}
			hasRamp := strings.Contains(line, npuHeatRamp)
			if mode == npuMatrixModeHealth && hasRamp {
				t.Errorf("%s: the health legend carries a ramp: %q", locale, line)
			}
			if mode != npuMatrixModeHealth && !hasRamp {
				t.Errorf("%s: mode %d legend does not carry the ramp: %q", locale, mode, line)
			}

			m.width = 38
			clipped := m.npuHeatLegend()
			if strings.Contains(clipped, "\n") || visualLength(clipped) > 38 {
				t.Errorf("%s: mode %d legend overflows or wraps at 38 columns: %q", locale, mode, clipped)
			}
			m.width = 160
		}
	}
}

// T-footer: the NPU view footer advertises `h`, and no other list view does.
func TestB30FooterAdvertisesHKeyOnNPUView(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		want := stripANSI(RenderKeyBinding("h", m30T(locale).T("keys.npu_matrix")))

		m := m30T(locale)
		m.width = 200
		for _, view := range []ViewType{ViewNodes, ViewPods, ViewEvents, ViewAlerts, ViewWorkloads, ViewNetwork, ViewStorage, ViewOverview} {
			m.currentView = view
			footer := stripANSI(m.renderFooter())
			if strings.Contains(footer, want) || strings.Contains(footer, m.T("keys.npu_matrix")) {
				t.Errorf("%s: view %v advertises the matrix key: %q", locale, view, footer)
			}
		}

		m.currentView = ViewNPU
		footer := stripANSI(m.renderFooter())
		if !strings.Contains(footer, want) {
			t.Errorf("%s: the NPU footer does not advertise %q: %q", locale, want, footer)
		}
	}
}

// m30T returns a bare model for translation lookups.
func m30T(locale string) *Model {
	return NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
}

// TestB30NewI18nKeysPlaceholderParity pins the batch's 4 new keys (review fix,
// P3-3): present in both bundles and carrying the same placeholder set, so one
// locale cannot silently drop the {{.Mode}} the other prints. Same shape as the
// batch-29 convention in b29v_width_test.go.
func TestB30NewI18nKeysPlaceholderParity(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	newKeys := []string{
		"npuview.mode_hbm", "npuview.mode_health", "npuview.health_legend", "keys.npu_matrix",
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
	// Non-vacuousness: the health legend really carries the {{.Mode}} placeholder
	// this test guards, in both locales.
	for locale, msg := range map[string]string{"en": en["npuview.health_legend"], "zh": zh["npuview.health_legend"]} {
		if !strings.Contains(msg, "{{.Mode}}") {
			t.Errorf("%s npuview.health_legend lost its {{.Mode}} placeholder: %q", locale, msg)
		}
	}
}

// T-width, zero-drift half: the aggregate bar and the overview NPU card do not
// depend on the matrix mode. Rendering them under all five modes must produce
// identical bytes; the batch-29 byte anchors in b29_agg_test.go, b29_card_test.go
// and b29v_zerodrift_test.go pin what those bytes are.
func TestB30AggregateBarAndCardIgnoreMatrixMode(t *testing.T) {
	forcedAsciiProfile(t)
	var bar, card string
	for i, mode := range []int{
		npuMatrixModeAICore, npuMatrixModeHBM, npuMatrixModeTemp,
		npuMatrixModePower, npuMatrixModeHealth,
	} {
		m := b30Model(t, "en", mode)
		m.clusterData = b29vZeroDriftData()
		barNow := stripANSI(strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | "))
		cardNow := stripANSI(strings.Join(m.npuPanelLines(m.clusterData.Summary), "\n"))
		if barNow == "" {
			t.Fatalf("mode %d: the aggregate bar is empty on a chip-bearing fixture", mode)
		}
		if i == 0 {
			bar, card = barNow, cardNow
			continue
		}
		if barNow != bar {
			t.Errorf("mode %d changed the aggregate bar:\n got %q\nwant %q", mode, barNow, bar)
		}
		if cardNow != card {
			t.Errorf("mode %d changed the overview NPU card:\n got %q\nwant %q", mode, cardNow, card)
		}
	}
}
