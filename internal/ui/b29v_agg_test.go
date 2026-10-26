package ui

// b29v_agg_test.go — independent adversarial probe for batch 29's aggregate
// gauge bar and the session series behind it. The numbers below are computed by
// hand in the comments, independently of the implementer's fixtures.

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29vChipSet builds a node whose grades are all healthy, from explicit chips.
func b29vChipSet(name string, chips []model.NPUChipData) *model.NodeData {
	grades := make([]model.NPUChipGrade, len(chips))
	for i, chip := range chips {
		grades[i] = model.NPUChipGrade{NPUID: chip.NPUID, Chip: chip.Chip, Grade: model.NPUChipGradeHealthy}
	}
	return b29vGradeNode(name, chips, grades)
}

// b29vSegModel builds the NPU view over data with a session series of the given
// length, every sample built by the production aggregator.
func b29vSegModel(data *model.ClusterData, seriesLen int) *Model {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	m.width, m.height = 200, 90
	m.currentView = ViewNPU
	m.clusterData = data
	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 0; i < seriesLen; i++ {
		m.vizHistory = append(m.vizHistory, m.buildVizSample(data, base.Add(time.Duration(i)*2*time.Second)))
	}
	return m
}

// TestB29VAggregateExcludesUnknownFromUtilisationOnly pins the asymmetric rule
// with a fixture where the two excluded chips carry high utilisation: if the
// filter leaked, the mean would be 52.5 instead of 15.
//
//	utilisation reported: 10 and 20           → mean 15
//	temperature, all four: 40 50 60 70        → mean 55 (Unknown included)
//	power, all four: 4 × 100                  → 400 W
//	HBM: 1024 of 4096 used on each chip       → 1024 of 16384 MB
func TestB29VAggregateExcludesUnknownFromUtilisationOnly(t *testing.T) {
	forcedAsciiProfile(t)
	chips := []model.NPUChipData{
		{NPUID: 0, Chip: 0, Health: "OK", AICore: 10, Temp: 40, Power: 100, HBMUsed: 1024, HBMTotal: 4096},
		{NPUID: 0, Chip: 1, Health: "OK", AICore: 20, Temp: 50, Power: 100, HBMUsed: 0, HBMTotal: 4096},
		{NPUID: 1, Chip: 0, Health: "Unknown", AICore: 90, Temp: 60, Power: 100, HBMUsed: 0, HBMTotal: 4096},
		{NPUID: 1, Chip: 1, Health: "Unknown", AICore: 90, Temp: 70, Power: 100, HBMUsed: 0, HBMTotal: 4096},
	}
	node := b29vChipSet("node-a", chips)
	m := b29vSegModel(&model.ClusterData{
		Nodes:   []*model.NodeData{node},
		Summary: &model.ClusterSummary{NPUCapacity: 4, NPUAllocatable: 4},
	}, 1)

	sample := m.currentVizSample()
	if !sample.NPUUtilKnown {
		t.Fatal("NPUUtilKnown is false although two chips reported")
	}
	if got, want := sample.NPUUtilPct, 15.0; got != want {
		t.Errorf("NPUUtilPct = %v, want %v (the Unknown chips' 90%% must not be averaged in)", got, want)
	}
	if got, want := sample.NPUTempAvg, 55.0; got != want {
		t.Errorf("NPUTempAvg = %v, want %v (Unknown chips still have a temperature)", got, want)
	}
	if got, want := sample.NPUPowerW, 400.0; got != want {
		t.Errorf("NPUPowerW = %v, want %v", got, want)
	}
	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), sample), " | ")
	for _, want := range []string{"Power 400W", "Avg temp 55°C", "HBM 1/16G (6%)", "Utilization 15%"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the bar %q does not carry %q", bar, want)
		}
	}
}

// TestB29VAggregateHidesUnknownOnlySegments pins the per-segment presence rules
// on three fixtures: no chip data at all (no bar), all chips Unknown (no
// utilisation segment, but temperature/power exist), and chips that report HBM
// used but no capacity (no HBM segment).
func TestB29VAggregateHidesUnknownOnlySegments(t *testing.T) {
	forcedAsciiProfile(t)

	// No chip data: no bar at all.
	noChips := &model.ClusterData{
		Nodes:   []*model.NodeData{{Name: "n", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8}},
		Summary: &model.ClusterSummary{NPUCapacity: 8, NPUAllocatable: 8},
	}
	m := b29vSegModel(noChips, 4)
	if segs := m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()); len(segs) != 0 {
		t.Errorf("a cluster without chip data rendered a bar: %v", segs)
	}

	// All chips Unknown: utilisation gone, temp/power kept, HBM capacity absent.
	unknown := []model.NPUChipData{
		{NPUID: 0, Chip: 0, Health: "Unknown", AICore: 77, Temp: 52, Power: 150, HBMUsed: 512, HBMTotal: 0},
		{NPUID: 0, Chip: 1, Health: "Unknown", AICore: 77, Temp: 54, Power: 150, HBMUsed: 0, HBMTotal: 0},
	}
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{b29vChipSet("n", unknown)},
		Summary: &model.ClusterSummary{NPUCapacity: 2, NPUAllocatable: 2},
	}
	m = b29vSegModel(data, 4)
	bar := strings.Join(m.npuAggregateSegments(m.npuNodes(), m.currentVizSample()), " | ")
	if strings.Contains(bar, "Utilization") {
		t.Errorf("an all-Unknown cluster rendered a utilisation segment: %q", bar)
	}
	if strings.Contains(bar, "HBM") {
		t.Errorf("a zero HBM capacity rendered an HBM segment: %q", bar)
	}
	if !strings.Contains(bar, "Power 300W") || !strings.Contains(bar, "Avg temp 53°C") {
		t.Errorf("an all-Unknown cluster lost the measured power/temperature: %q", bar)
	}
}

// TestB29VAggregateBarKeepsWorstCaseValuesAt38 pins the content budget with
// values wider than the fixtures the implementer used: a five-digit wattage, a
// three-digit temperature, four-digit HBM, and 100% utilisation.
func TestB29VAggregateBarKeepsWorstCaseValuesAt38(t *testing.T) {
	forcedAsciiProfile(t)
	for _, locale := range []string{"en", "zh"} {
		chips := make([]model.NPUChipData, 4)
		for i := range chips {
			chips[i] = model.NPUChipData{
				NPUID: i / 2, Chip: i % 2, Health: "OK",
				AICore: 100, Temp: 105, Power: 3086.25,
				HBMUsed: 1023 * 1024, HBMTotal: 1023 * 1024,
			}
		}
		data := &model.ClusterData{
			Nodes:   []*model.NodeData{b29vChipSet("n", chips)},
			Summary: &model.ClusterSummary{NPUCapacity: 4, NPUAllocatable: 4},
		}
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)
		m.width, m.height = 38, 90
		m.currentView = ViewNPU
		m.clusterData = data
		m.vizHistory = []VizSample{m.buildVizSample(data, time.Time{}), m.buildVizSample(data, time.Time{})}

		segs := m.npuAggregateSegments(m.npuNodes(), m.currentVizSample())
		for _, seg := range segs {
			if got := visualLength(seg); got > 38 {
				t.Errorf("%s: segment %q is %d cells wide", locale, stripANSI(seg), got)
			}
		}
		out := stripANSI(m.renderNPUView())
		// 4 × 3086.25 = 12345 W; HBM used and capacity are both 1023 GiB per chip.
		for _, want := range []string{"12345", "105°C", "4092/4092", "100%"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the 38-column view lost %q:\n%s", locale, want, out)
			}
		}
	}
}

// TestB29VUtilisationSeriesSkipsUnknownSamples pins the series filter: samples
// recorded while no chip reported a utilisation must not become a zero point in
// the trend, and the rendered sparkline carries one cell per known sample.
func TestB29VUtilisationSeriesSkipsUnknownSamples(t *testing.T) {
	forcedAsciiProfile(t)
	chips := []model.NPUChipData{
		{NPUID: 0, Chip: 0, Health: "OK", AICore: 10, Temp: 46, Power: 100},
		{NPUID: 0, Chip: 1, Health: "OK", AICore: 20, Temp: 46, Power: 100},
	}
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{b29vChipSet("n", chips)},
		Summary: &model.ClusterSummary{NPUCapacity: 2, NPUAllocatable: 2},
	}
	m := b29vSegModel(data, 0)
	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	// Known, unknown, known, unknown, known: values 10, -, 20, -, 30.
	knownAt := map[int]float64{0: 10, 2: 20, 4: 30}
	for i := 0; i < 5; i++ {
		v, known := knownAt[i]
		m.vizHistory = append(m.vizHistory, VizSample{
			At:           base.Add(time.Duration(i) * 2 * time.Second),
			NPUUtilKnown: known,
			NPUUtilPct:   v,
		})
	}
	series := m.vizSeries(func(s VizSample) (float64, bool) { return s.NPUUtilPct, s.NPUUtilKnown })
	if len(series) != 3 {
		t.Fatalf("the utilisation series has %d points, want the 3 known ones: %v", len(series), series)
	}
	for i, want := range []float64{10, 20, 30} {
		if series[i] != want {
			t.Errorf("series point %d = %v, want %v", i, series[i], want)
		}
	}

	// The rendered segment's sparkline is three cells wide, not five.
	sample := m.currentVizSample()
	sample.NPUUtilKnown = true
	sample.NPUUtilPct = 30
	segs := m.npuAggregateSegments(m.npuNodes(), sample)
	var util string
	for _, seg := range segs {
		if strings.Contains(seg, "Utilization") {
			util = stripANSI(seg)
		}
	}
	if util == "" {
		t.Fatalf("no utilisation segment: %v", segs)
	}
	// The segment is "Utilization 50% " followed by the sparkline; the trailing
	// run of ramp/blank cells must be exactly three.
	tail := util[strings.LastIndex(util, " ")+1:]
	if got := len([]rune(tail)); got != 3 {
		t.Errorf("the utilisation sparkline is %d cells, want 3 (one per known sample): %q", got, util)
	}
}

// TestB29VAggregateSparkEndsOnThePrintedValue pins the same-source contract at
// the glyph level: with a strictly rising history whose last point is the live
// value, the trend must end on the top glyph and carry the printed number.
func TestB29VAggregateSparkEndsOnThePrintedValue(t *testing.T) {
	forcedAsciiProfile(t)
	chips := []model.NPUChipData{
		{NPUID: 0, Chip: 0, Health: "OK", Temp: 40, Power: 100},
		{NPUID: 0, Chip: 1, Health: "OK", Temp: 40, Power: 100},
	}
	data := &model.ClusterData{
		Nodes:   []*model.NodeData{b29vChipSet("n", chips)},
		Summary: &model.ClusterSummary{NPUCapacity: 2, NPUAllocatable: 2},
	}
	m := b29vSegModel(data, 0)
	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	for i := 1; i <= 6; i++ {
		m.vizHistory = append(m.vizHistory, VizSample{At: base.Add(time.Duration(i) * 2 * time.Second), NPUPowerW: float64(i) * 100})
	}
	sample := m.currentVizSample()
	if got, want := sample.NPUPowerW, 200.0; got != want {
		t.Fatalf("live power = %v, want %v", got, want)
	}
	segs := m.npuAggregateSegments(m.npuNodes(), sample)
	var power string
	for _, seg := range segs {
		if strings.Contains(seg, "Power") {
			power = stripANSI(seg)
		}
	}
	if power == "" {
		t.Fatalf("no power segment: %v", segs)
	}
	if !strings.HasPrefix(power, "Power 200W") {
		t.Errorf("the power segment %q does not start with the live value", power)
	}
	if !strings.HasSuffix(power, "█") {
		t.Errorf("the power sparkline %q does not end on the newest (largest) sample", power)
	}
}
