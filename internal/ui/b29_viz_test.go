package ui

// b29_viz_test.go — batch 29 (overview visualisation), the session-series base.
//
// The four surfaces of the batch all read one series. Its two contracts are
// here rather than in the renderer tests: the series follows the same
// last-refresh guard metricHistory follows (a cached payload must not be drawn
// twice), and the ring keeps its declared depth so the sparklines stay inside
// the session they claim to show.

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b29VizData is a refresh payload whose aggregate values are all distinct.
func b29VizData(refresh time.Time) *model.ClusterData {
	chips := []model.NPUChipData{
		{NPUID: 0, Chip: 0, Health: "OK", AICore: 40, Temp: 50, Power: 300, HBMUsed: 100, HBMTotal: 1000},
		{NPUID: 0, Chip: 1, Health: "OK", AICore: 20, Temp: 40, Power: 200, HBMUsed: 100, HBMTotal: 1000},
	}
	return &model.ClusterData{
		Nodes: []*model.NodeData{{
			Name: "npu-1", Status: "Ready", NPUCapacity: 2, NPUAllocatable: 2,
			CPUAllocatable: 1000, CPUUsage: 100, CPUCapacity: 1000,
			MemAllocatable: 1000, MemoryUsage: 500, MemoryCapacity: 1000,
			NPUChips: chips,
			NPUChipHealth: []model.NPUChipGrade{
				{NPUID: 0, Chip: 0, Grade: model.NPUChipGradeHealthy},
				{NPUID: 0, Chip: 1, Grade: model.NPUChipGradeHealthy},
			},
		}},
		Summary: &model.ClusterSummary{
			CPUCapacity: 1000, CPUAllocatable: 1000, CPUUsed: 100,
			MemoryCapacity: 1000, MemoryAllocatable: 1000, MemoryUsed: 500,
			NPUCapacity: 2, NPUAllocatable: 2,
			LastRefreshTime: refresh,
		},
	}
}

// TestB29VizSampleRecordedWithRefreshGuard pins the shared guard: two
// clusterDataMsg carrying the same LastRefreshTime record one sample, and a new
// LastRefreshTime records another. This is the same rule metricHistory follows,
// and the sparkline series has to follow it or a cached repeat would draw a flat
// segment the cluster never had.
func TestB29VizSampleRecordedWithRefreshGuard(t *testing.T) {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	first := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
	second := first.Add(2 * time.Second)

	m.Update(clusterDataMsg{data: b29VizData(first)})
	if got := len(m.vizHistory); got != 1 {
		t.Fatalf("after one refresh the series has %d samples, want 1", got)
	}
	m.Update(clusterDataMsg{data: b29VizData(first)})
	if got := len(m.vizHistory); got != 1 {
		t.Errorf("a repeated LastRefreshTime added a sample: %d samples, want 1", got)
	}
	m.Update(clusterDataMsg{data: b29VizData(second)})
	if got := len(m.vizHistory); got != 2 {
		t.Errorf("a new LastRefreshTime did not add a sample: %d samples, want 2", got)
	}

	// The guard is the same one: when the series grew, metricHistory did too.
	if len(m.metricHistory) != len(m.vizHistory) {
		t.Errorf("the series and metricHistory disagree on the sample count: %d vs %d",
			len(m.vizHistory), len(m.metricHistory))
	}
}

// TestB29VizRingKeepsDepth pins the ring depth: the series never exceeds
// maxVizHistory and always keeps the newest samples.
func TestB29VizRingKeepsDepth(t *testing.T) {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	base := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)

	for i := 0; i < m.maxVizHistory+20; i++ {
		m.Update(clusterDataMsg{data: b29VizData(base.Add(time.Duration(i) * time.Second))})
	}
	if got := len(m.vizHistory); got != m.maxVizHistory {
		t.Fatalf("the series holds %d samples, want the ring depth %d", got, m.maxVizHistory)
	}
	// The newest sample is the one that survived, the oldest are gone.
	last := base.Add(time.Duration(m.maxVizHistory+19) * time.Second)
	if got := m.vizHistory[len(m.vizHistory)-1].At; !got.Equal(last) {
		t.Errorf("the series ends at %v, want the newest refresh %v", got, last)
	}
	if first := base.Add(19 * time.Second); m.vizHistory[0].At.Before(first) {
		t.Errorf("the series still starts before %v: %v", first, m.vizHistory[0].At)
	}
}

// TestB29VizAggregatesFollowTheSummary pins the CPU/Memory source: the sample's
// percentages are against allocatable and equal what the cluster-load box
// divides by, so the number and its sparkline cannot come from different
// denominators.
func TestB29VizAggregatesFollowTheSummary(t *testing.T) {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	data := b29VizData(time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC))
	m.clusterData = data

	sample := m.currentVizSample()
	if sample == nil {
		t.Fatal("no sample for the fixture")
	}
	if want := 10.0; sample.CPUPct != want {
		t.Errorf("CPUPct = %v, want %v (100m / 1000m)", sample.CPUPct, want)
	}
	if want := 50.0; sample.MemPct != want {
		t.Errorf("MemPct = %v, want %v (500 / 1000)", sample.MemPct, want)
	}

	// A summary with no allocatable denominator yields 0, not NaN or +Inf.
	empty := &model.ClusterData{Summary: &model.ClusterSummary{CPUUsed: 100, MemoryUsed: 100}}
	emptyModel := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	emptyModel.clusterData = empty
	if got := emptyModel.currentVizSample().CPUPct; got != 0 {
		t.Errorf("CPUPct without an allocatable denominator = %v, want 0", got)
	}
}

// TestB29VizHistorySurvivesNoSummary pins the node-summing fallback: a payload
// without a summary still produces a sample from the node fields, which is what
// the tests and the first refresh of a session see.
func TestB29VizHistorySurvivesNoSummary(t *testing.T) {
	m := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	data := b29VizData(time.Time{})
	data.Summary = nil
	m.clusterData = data

	sample := m.currentVizSample()
	if sample == nil {
		t.Fatal("no sample for a payload without a summary")
	}
	if want := 10.0; sample.CPUPct != want {
		t.Errorf("CPUPct from the node fallback = %v, want %v", sample.CPUPct, want)
	}
	if want := 30.0; sample.NPUUtilPct != want {
		t.Errorf("NPUUtilPct = %v, want %v (40 and 20 over two chips)", sample.NPUUtilPct, want)
	}
}
