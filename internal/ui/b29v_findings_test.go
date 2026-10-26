package ui

// b29v_findings_test.go — the verifier's three reproducible defects in batch
// 29, plus the follow-up contract the adjudicator's fix added. These probes
// FAILED against the batch as delivered; after the fixes they pin the fixed
// behaviour, and each has a RED mutation on record (revert the fix, the probe
// goes red again — see the delivery report for the mutation log).
//
// F1 (P2): the aggregate bar printed "Chips N/N healthy" when the exporter
// reported no health status, directly under the summary bar's deliberately
// neutral "no fault" wording. Batch 25 established that rule for the same
// view. The fix switches the segment to "no fault" wording and makes the
// overview card state the unknown count instead of an unqualified tally.
//
// F2 (P2): the zh NPU card's utilisation row was 19 cells wide when the mean
// is a three-digit value (100%), one over the panel's 18-cell content budget;
// the rendered panel silently clipped the percentage off the row. The fix
// makes the bar yield width to the value.
//
// F3 (P2): the heat ramp started with the very glyph the fault grade owns, so
// in --no-color a cold healthy chip and a faulted chip were the same byte.
// The fix starts the ramp at ▂.

import (
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// TestB29VAggregateHealthWordingStaysHonestAboutUnknown pins the wording rule
// batch 25 established for the summary bar: when chips did not report a health
// status, the healthy count must not be presented as "healthy", because the
// same panel prints "unknown N" beside it. The aggregate bar's health segment
// is the same claim in the same view.
func TestB29VAggregateHealthWordingStaysHonestAboutUnknown(t *testing.T) {
	forcedAsciiProfile(t)
	chips := make([]model.NPUChipData, 8)
	for i := range chips {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, Health: "Unknown", Temp: 46, Power: 190}
	}
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{b29vChipSet("n", chips)},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}
	m := b29vSegModel(data, 4)

	// The summary bar above says "no fault", never "healthy", on this fixture.
	summary := m.npuSummarySegments(m.npuNodes())
	if len(summary) == 0 {
		t.Fatal("no summary segments")
	}
	if strings.Contains(summary[0], "healthy") || strings.Contains(summary[0], "健康") {
		t.Fatalf("fixture invalid: the summary bar itself claims health: %q", stripANSI(summary[0]))
	}

	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | ")
	if strings.Contains(bar, "healthy") || strings.Contains(bar, "健康") {
		t.Errorf("the aggregate bar claims health for chips whose exporter said nothing: %q", stripANSI(bar))
	}
	if !strings.Contains(bar, "no fault") && !strings.Contains(bar, "无故障") {
		t.Errorf("the aggregate bar dropped the health tally instead of rewording it: %q", stripANSI(bar))
	}

	// The same fixture on the overview card: the card states the unknown count
	// outright instead of an unqualified ratio that reads all-healthy.
	for _, locale := range []string{"en", "zh"} {
		cardModel := b29vCardModel(locale, data)
		card := stripANSI(strings.Join(cardModel.npuPanelLines(data.Summary), "\n"))
		qualifier := "unk"
		if locale == "zh" {
			qualifier = "未知"
		}
		if !strings.Contains(card, qualifier) {
			t.Errorf("%s: the overview card claims an unqualified chip tally for an all-Unknown cluster:\n%s", locale, card)
		}
	}
}

// TestB29VCardFitsContentBudgetAtHundredPercent is the width case the
// implementer's card fixture misses: at a 20% utilisation the zh "芯片率：" row
// is exactly 18 cells; a three-digit mean makes it 19, which lipgloss clips,
// and the percentage disappears from the rendered card.
func TestB29VCardFitsContentBudgetAtHundredPercent(t *testing.T) {
	forcedAsciiProfile(t)
	for _, locale := range []string{"en", "zh"} {
		data := b29vCardData(100, 60, 100)
		m := b29vCardModel(locale, data)
		for i, line := range m.npuPanelLines(data.Summary) {
			if got := visualLength(line); got > summaryPanelContent {
				t.Errorf("%s: card line %d is %d cells wide, the content budget is %d: %q",
					locale, i, got, summaryPanelContent, stripANSI(line))
			}
		}
		// And the rendered panel must still show the value: the content budget is
		// a budget because lipgloss clips silently past it.
		panel := stripANSI(renderSummaryPanel(m.npuPanelLines(data.Summary), summaryPanelMinContentLine))
		if !strings.Contains(panel, "100%") {
			t.Errorf("%s: the rendered NPU card lost the utilisation value:\n%s", locale, panel)
		}
	}
}

// TestB29VHeatRampDoesNotCollideWithTheFaultGlyph is F3 (P2): the heat ramp
// starts with the very glyph the fault grade owns. In --no-color mode a healthy
// chip at the node's metric minimum and a faulted chip are the same byte, and
// the view prints two legends that contradict each other ("▁ fault" next to
// "▁▂▃▄▅▆▇█ low→high"). The batch's own comment claims the two glyph sets are
// disjoint so a heat cell can never be mistaken for a graded one.
func TestB29VHeatRampDoesNotCollideWithTheFaultGlyph(t *testing.T) {
	forcedAsciiProfile(t)

	fault := stripANSI(npuGradeCell(model.NPUChipGradeFault))
	cold := stripANSI(npuHeatCell(0))
	if fault == cold {
		t.Errorf("the heat ramp's lowest glyph %q is the fault glyph", cold)
	}

	// End to end: a node whose faulted chip is the hottest and whose healthy chip
	// is the coldest renders two identical cells, and the legend says that glyph
	// means "fault".
	node := b29vHealthyNode("collide", 4)
	node.NPUChips[0].AICore = 0   // healthy, node minimum
	node.NPUChips[1].AICore = 100 // faulted, node maximum
	node.NPUChipHealth[1].Grade = model.NPUChipGradeFault
	m := b29vHeatModel(&model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 4, NPUAllocatable: 4},
	}, npuMatrixModeAICore)

	out := stripANSI(m.renderNPUView())
	row := lineContaining(out, "collide")
	if row == "" {
		t.Fatalf("no node row in:\n%s", out)
	}
	glyphs := strings.Fields(row)
	nameIdx := -1
	for i, f := range glyphs {
		if f == "collide" {
			nameIdx = i
			break
		}
	}
	if nameIdx < 0 || len(glyphs) < nameIdx+3 {
		t.Fatalf("cannot locate the two chips in %q", row)
	}
	coldCell, faultCell := glyphs[nameIdx+1], glyphs[nameIdx+2]
	if coldCell == faultCell {
		t.Errorf("the cold healthy chip and the faulted chip render the same cell %q in --no-color: %q",
			coldCell, row)
	}
	if !strings.Contains(out, stripANSI(m.T("npuview.legend"))) {
		t.Errorf("the health legend is gone, so the collision argument is moot")
	}
}

// TestB29VVizPercentageDenominatorMatchesTheHeadline (P3): the CPU/memory
// series feeds the sparkline printed on the same load-box line as a
// capacity-based headline percentage. The series must use that same
// denominator — with a capacity/allocatable gap the trend and its headline
// number would answer different questions, and a missing allocatable would
// silence the sparkline under a live percentage.
func TestB29VVizPercentageDenominatorMatchesTheHeadline(t *testing.T) {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.clusterData = &model.ClusterData{
		Summary: &model.ClusterSummary{
			CPUUsed: 100, CPUCapacity: 2000, CPUAllocatable: 1000,
			MemoryUsed: 500, MemoryCapacity: 2000, MemoryAllocatable: 1000,
		},
	}

	sample := m.currentVizSample()
	if sample == nil {
		t.Fatal("no sample for a payload with a summary")
	}
	if want := 5.0; sample.CPUPct != want {
		t.Errorf("CPUPct = %v, want the capacity share %v (100/2000)", sample.CPUPct, want)
	}
	if want := 25.0; sample.MemPct != want {
		t.Errorf("MemPct = %v, want the capacity share %v (500/2000)", sample.MemPct, want)
	}
}
