package ui

// b29_agg_test.go — batch 29 (overview visualisation), contract T-agg.
//
// The aggregate gauge bar sits between the summary bar and the chip matrix. Its
// contract has three parts: every number is an exact sum or mean over the chips
// that carry data (not over the nodes, not over the grades), a cluster with no
// chip data has no bar at all, and the value beside a sparkline is the same
// sample the series is built from.

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29AggCluster is a two-node, sixteen-chip fixture whose aggregates are exact
// round numbers, so the bar's arithmetic can be checked by hand:
//
//	power:        100*4 + 200*2 + 300 + 400 (node-a) + 50*8 (node-b) = 1900 W
//	temperature:  (40*4 + 50*2 + 60 + 70) / 16 … node-b adds 40*8
//	              → (390 + 320) / 16 = 44.375 °C, printed as 44
//	HBM:          32768 of 16 * 65536 MB = 3.125 %, printed as 32/1024G (3%)
//	utilisation:  (0*2 + 40*2 + 80*2 + 40*2) / 16 = 320 / 16 = 20 %
//
// Every chip reports a health status ("OK"), so all sixteen count toward the
// utilisation mean — including the ones reporting a real 0.
func b29AggCluster() *model.ClusterData {
	chip := func(npu, indexInNPU, aiCore, temp int, power float64, used, total int64) model.NPUChipData {
		return model.NPUChipData{
			NPUID: npu, Chip: indexInNPU, PhyID: npu*2 + indexInNPU, Health: "OK",
			AICore: aiCore, Temp: temp, Power: power, HBMUsed: used, HBMTotal: total,
		}
	}
	nodeA := b29NodeWith("node-a", []model.NPUChipData{
		chip(0, 0, 0, 40, 100, 0, 65536),
		chip(0, 1, 0, 40, 100, 0, 65536),
		chip(1, 0, 40, 40, 100, 0, 65536),
		chip(1, 1, 40, 40, 100, 0, 65536),
		chip(2, 0, 80, 50, 200, 32768, 65536),
		chip(2, 1, 80, 50, 200, 0, 65536),
		chip(3, 0, 40, 60, 300, 0, 65536),
		chip(3, 1, 40, 70, 400, 0, 65536),
	})
	nodeB := b29NodeWith("node-b", []model.NPUChipData{
		chip(0, 0, 0, 40, 50, 0, 65536),
		chip(0, 1, 0, 40, 50, 0, 65536),
		chip(1, 0, 0, 40, 50, 0, 65536),
		chip(1, 1, 0, 40, 50, 0, 65536),
		chip(2, 0, 0, 40, 50, 0, 65536),
		chip(2, 1, 0, 40, 50, 0, 65536),
		chip(3, 0, 0, 40, 50, 0, 65536),
		chip(3, 1, 0, 40, 50, 0, 65536),
	})
	return &model.ClusterData{
		Nodes:   []*model.NodeData{nodeA, nodeB},
		Summary: &model.ClusterSummary{NPUCapacity: 16, NPUAllocatable: 16, NPUNodesCount: 2},
	}
}

// b29NodeWith wraps chips into an NPU node whose grades are all healthy.
func b29NodeWith(name string, chips []model.NPUChipData) *model.NodeData {
	grades := make([]model.NPUChipGrade, len(chips))
	for i, chip := range chips {
		grades[i] = model.NPUChipGrade{NPUID: chip.NPUID, Chip: chip.Chip, Grade: model.NPUChipGradeHealthy}
	}
	return &model.NodeData{
		Name: name, Status: "Ready", NPUCapacity: int64(len(chips)),
		NPUAllocatable: int64(len(chips)),
		NPUChips:       chips, NPUChipHealth: grades,
	}
}

// b29ChipNode builds an 8-chip node (two chips per NPU) whose first four chips
// carry the given values and whose last four are zeroed but present.
func b29ChipNode(name string, aiCore, temps []int, power []float64, hbm []int64) *model.NodeData {
	chips := make([]model.NPUChipData, 8)
	for i := range chips {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK"}
	}
	for i := range aiCore {
		chips[i].AICore = aiCore[i]
		chips[i].Temp = temps[i]
		chips[i].Power = power[i]
	}
	chips[0].HBMUsed, chips[0].HBMTotal = hbm[0], hbm[1]
	chips[4].HBMUsed, chips[4].HBMTotal = hbm[0], hbm[1]
	return b29NodeWith(name, chips)
}

// b29AggModel builds the NPU view over a cluster, with a session series of the
// given length ending at the cluster's own aggregate values.
func b29AggModel(t *testing.T, data *model.ClusterData, seriesLen int) *Model {
	t.Helper()
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 200, 60
	m.currentView = ViewNPU
	m.clusterData = data

	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 0; i < seriesLen; i++ {
		sample := m.buildVizSample(data, base.Add(time.Duration(i)*2*time.Second))
		// Vary the series so the sparkline has a shape, but keep the last value
		// the live one: that is the "same source, same value" pairing.
		if i < seriesLen-1 {
			sample.NPUPowerW += float64(i) * 10
		}
		m.vizHistory = append(m.vizHistory, sample)
	}
	return m
}

// TestB29AggregateSumsTwoNodes pins the arithmetic: the bar's power, average
// temperature, HBM ratio and utilisation mean are exact over every chip of both
// nodes — including the chips that carry zeros, which are chips the exporter
// answered about.
func TestB29AggregateSumsTwoNodes(t *testing.T) {
	forcedAsciiProfile(t)

	m := b29AggModel(t, b29AggCluster(), 1)
	sample := m.currentVizSample()
	if sample == nil {
		t.Fatal("no aggregate sample for the fixture")
	}

	// Power: 1500 W on node-a, 8 x 50 W on node-b.
	if want := 1900.0; sample.NPUPowerW != want {
		t.Errorf("NPUPowerW = %v, want %v", sample.NPUPowerW, want)
	}
	// Temperature mean over all 16 chips: 710 / 16 = 44.375.
	if want := 44.375; sample.NPUTempAvg != want {
		t.Errorf("NPUTempAvg = %v, want %v", sample.NPUTempAvg, want)
	}
	// HBM: one 32 GiB allocation out of 16 x 64 GiB.
	if sample.HBMUsedMB != 32768 || sample.HBMTotalMB != 16*65536 {
		t.Errorf("HBM = %d/%d MB, want %d/%d", sample.HBMUsedMB, sample.HBMTotalMB, 32768, 16*65536)
	}
	// Utilisation mean over all 16 reported chips: 320 / 16 = 20.
	if !sample.NPUUtilKnown {
		t.Fatal("NPUUtilKnown is false on a fixture where every chip reported")
	}
	if want := 20.0; sample.NPUUtilPct != want {
		t.Errorf("NPUUtilPct = %v, want %v", sample.NPUUtilPct, want)
	}

	// And the same numbers reach the bar's segments.
	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), sample), " | ")
	for _, want := range []string{"Power 1900W", "Avg temp 44°C", "HBM 32/1024G (3%)", "Utilization 20%"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the aggregate bar %q does not carry %q", bar, want)
		}
	}
	// The chip-health segment keeps the degraded/fault counts out of the bar but
	// keeps the healthy ratio exact, including the non-healthy chips in the
	// denominator.
	for _, want := range []string{"Nodes 2", "Chips 16/16 healthy"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the aggregate bar %q does not carry %q", bar, want)
		}
	}
}

// TestB29AggregateSkipsClusterWithoutChipData pins the whole-or-nothing rule: an
// NPU cluster whose exporter never answered has no gauge bar. The summary bar's
// coverage segment already says the monitoring is missing, and a bar of zeros
// would contradict it by reading as a measured idle cluster.
func TestB29AggregateSkipsClusterWithoutChipData(t *testing.T) {
	forcedAsciiProfile(t)

	m := b29AggModel(t, &model.ClusterData{
		Nodes: []*model.NodeData{
			{Name: "node-a", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8},
			{Name: "node-b", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8},
		},
		Summary: &model.ClusterSummary{NPUCapacity: 16, NPUAllocatable: 16, NPUNodesCount: 2},
	}, 4)

	if segs := m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()); len(segs) != 0 {
		t.Errorf("a cluster with no chip data rendered an aggregate bar: %v", segs)
	}

	// Non-vacuousness: the same model with chip data does render one.
	withData := b29AggModel(t, b29AggCluster(), 4)
	if segs := withData.npuAggregateSegments(withData.npuNodes(), withData.currentVizSample()); len(segs) == 0 {
		t.Fatal("probe vacuous: the fixture with chip data rendered no bar either")
	}
}

// TestB29AggregateHidesHBMWithoutCapacity pins the ratio guard: a chip set that
// reports no HBM capacity has no HBM segment, because "0/0 (NaN%)" is worse than
// saying nothing.
func TestB29AggregateHidesHBMWithoutCapacity(t *testing.T) {
	forcedAsciiProfile(t)

	node := b29ChipNode("node-a", []int{10, 20, 30, 40}, []int{40, 40, 40, 40},
		[]float64{50, 50, 50, 50}, []int64{0, 0})
	m := b29AggModel(t, &model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}, 1)

	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | ")
	if strings.Contains(bar, "HBM") {
		t.Errorf("the bar rendered an HBM segment without a reported capacity: %q", bar)
	}
	if !strings.Contains(bar, "Power") || !strings.Contains(bar, "Utilization") {
		t.Errorf("the bar dropped more than the HBM segment: %q", bar)
	}
}

// TestB29AggregateHidesUnknownUtilisation is the "idle is not unknown" rule: a
// chip that reported no health status (the exporter's Unknown) does not count
// toward the utilisation mean, and a cluster where no chip reported one has no
// utilisation segment at all. A zero mean would read as an idle cluster.
func TestB29AggregateHidesUnknownUtilisation(t *testing.T) {
	forcedAsciiProfile(t)

	unknown := b29ChipNode("node-a", []int{0, 0, 0, 0}, []int{40, 40, 40, 40},
		[]float64{50, 50, 50, 50}, []int64{0, 8000})
	for i := range unknown.NPUChips {
		unknown.NPUChips[i].Health = "Unknown"
	}
	m := b29AggModel(t, &model.ClusterData{
		Nodes:   []*model.NodeData{unknown},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}, 1)

	sample := m.currentVizSample()
	if sample.NPUUtilKnown {
		t.Errorf("NPUUtilKnown is true on a fixture where no chip reported a utilisation")
	}
	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), sample), " | ")
	if strings.Contains(bar, "Utilization") {
		t.Errorf("the bar rendered a utilisation segment without a reported utilisation: %q", bar)
	}

	// An idle cluster that does report (AICore 0 with a health status) keeps its
	// utilisation segment: 0% is a measurement.
	idle := b29ChipNode("node-a", []int{0, 0, 0, 0}, []int{40, 40, 40, 40},
		[]float64{50, 50, 50, 50}, []int64{0, 8000})
	idleModel := b29AggModel(t, &model.ClusterData{
		Nodes:   []*model.NodeData{idle},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}, 1)
	if !idleModel.currentVizSample().NPUUtilKnown {
		t.Fatal("an idle but reporting cluster lost its utilisation sample")
	}
	if bar := strings.Join(idleModel.npuAggregateSegments(idleModel.npuNodes(), idleModel.currentVizSample()), " | "); !strings.Contains(bar, "Utilization 0%") {
		t.Errorf("an idle reporting cluster lost its utilisation segment: %q", bar)
	}
}

// TestB29AggregateSparkMatchesThePrintedValue pins source consistency: the last
// point of every sparkline in the bar is the same sample the number beside it is
// formatted from. A series built from a stale copy of the cluster data would
// print a trend that does not end where the number does.
func TestB29AggregateSparkMatchesThePrintedValue(t *testing.T) {
	forcedAsciiProfile(t)

	const seriesLen = 6
	m := b29AggModel(t, b29AggCluster(), seriesLen)

	// The series the bar draws from must end on the live sample.
	live := m.currentVizSample()
	powerSeries := m.vizSeries(func(s VizSample) (float64, bool) { return s.NPUPowerW, true })
	if len(powerSeries) != seriesLen {
		t.Fatalf("the power series has %d points, want %d", len(powerSeries), seriesLen)
	}
	if powerSeries[len(powerSeries)-1] != live.NPUPowerW {
		t.Errorf("the series ends at %v W, the bar prints %v W", powerSeries[len(powerSeries)-1], live.NPUPowerW)
	}

	// And the recorded sample of the live payload is the live payload: the two
	// entry points share one aggregation function.
	if got := m.buildVizSample(m.clusterData, time.Time{}); got != *live {
		t.Errorf("buildVizSample(live) = %+v, currentVizSample() = %+v", got, *live)
	}

	// The bar's power segment carries a sparkline: at six samples and a wide
	// terminal there is room for it.
	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), live), " | ")
	if !strings.ContainsAny(bar, "▁▂▃▄▅▆▇█") {
		t.Errorf("the bar carries no sparkline at six samples: %q", bar)
	}
}

// TestB29AggregateSparkYieldsOnNarrowTerminals pins the width rule: a segment
// never exceeds the terminal, with or without its sparkline. The sparkline is
// shortened to whatever the segment leaves, and when nothing is left it is
// dropped whole rather than pushed past the edge.
func TestB29AggregateSparkYieldsOnNarrowTerminals(t *testing.T) {
	forcedAsciiProfile(t)

	// The narrow widths are the ones that exercise the clamp: at 38 columns every
	// segment above already fits with its full ten-cell sparkline.
	for _, width := range []int{20, 24, 28, 38, 40, 60, 80} {
		m := b29AggModel(t, b29AggCluster(), 12)
		m.width = width

		segs := m.npuAggregateSegments(m.npuNodes(), m.currentVizSample())
		for _, seg := range segs {
			if w := visualLength(seg); w > width {
				t.Errorf("width %d: segment %q is %d cells wide", width, stripANSI(seg), w)
			}
		}
		// The wrapped bar as the view renders it fits too, and the numbers it
		// carries survive.
		lines := wrapSummarySegments(segs, width, "  •  ")
		for _, line := range lines {
			if w := visualLength(line); w > width {
				t.Errorf("width %d: bar line %q is %d cells wide", width, stripANSI(line), w)
			}
		}
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "Power 1900W") {
			t.Errorf("width %d: the bar lost the power value: %q", width, joined)
		}
	}
}

// TestB29AggregateSparkShortensBeforeItOverflows pins the helper itself: the
// sparkline takes the room the segment leaves over, and disappears when there is
// none. Without the clamp the segment would be handed to the wrap path already
// wider than the terminal, which is how a narrow terminal loses a value.
func TestB29AggregateSparkShortensBeforeItOverflows(t *testing.T) {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	series := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	for _, width := range []int{13, 16, 20, 22} {
		m.width = width
		seg := "Power 1900W" // 11 cells
		got := m.npuAggregateSpark(seg, series, 10)
		if w := visualLength(got); w > width {
			t.Errorf("width %d: the segment with its sparkline is %d cells wide: %q", width, w, got)
		}
		if !strings.Contains(got, "1900W") {
			t.Errorf("width %d: the value was lost: %q", width, got)
		}
		// The sparkline is shortened, not dropped, while there is room for at
		// least one cell beside the value.
		if !strings.ContainsAny(got, "▁▂▃▄▅▆▇█") {
			t.Errorf("width %d: the sparkline was dropped although room remained: %q", width, got)
		}
	}

	// No room at all: the segment survives alone.
	m.width = 12
	if got := m.npuAggregateSpark(segLong, series, 10); got != segLong {
		t.Errorf("a segment that fills the terminal kept a sparkline: %q", got)
	}
}

// segLong is eleven cells wide, the width of "Power 1900W".
const segLong = "Power 1900W"
