package ui

// b25v_scale_test.go — reviewer round-5 probe.
//
// The batch's narrow-terminal contract is "38 columns must not overflow", and the
// summary bar's own comment states "the degraded and fault counts at the end may
// not be truncated away". Both were only ever verified on a one-node, eight-chip
// fixture. The health segment is not a fixed-width string: it carries the cluster
// total, so its width grows with the cluster. The "no fault" wording this batch
// introduced is one cell wider than the plain one in en, which moves the point at
// which the bar stops fitting the counts.
//
// Measured on this tree: at 38 columns, a cluster with at least one chip whose
// health is unknown and a chip total of 104 or more loses the fault count — the
// bar renders "102/104 no fault · 1 degraded · 1 f…" and truncates the single
// highest-severity signal away. The same cluster with every chip reporting a
// health status keeps both counts, and so does the pre-batch tree on every size
// up to 480 chips, so this is introduced by the batch, not inherited.

import (
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b25vScaleCluster builds a cluster of nodes x 8 chips, one degraded and one
// fault chip on the first node, and the first `unknown` chips reporting no
// health status (which selects the "no fault" wording).
func b25vScaleCluster(nodes, unknown int) *model.ClusterData {
	total := nodes * 8
	var ns []*model.NodeData
	for i := 0; i < nodes; i++ {
		chips := make([]model.NPUChipData, 8)
		grades := make([]model.NPUChipGrade, 8)
		for j := range chips {
			chips[j] = model.NPUChipData{NPUID: j / 2, Chip: j % 2, PhyID: j, Health: "OK"}
			grades[j] = model.NPUChipGrade{NPUID: j / 2, Chip: j % 2, Grade: model.NPUChipGradeHealthy}
		}
		ns = append(ns, &model.NodeData{
			Name: "node", Status: "Ready", NPUCapacity: 8, NPUAllocatable: 8,
			NPUChips: chips, NPUChipHealth: grades,
		})
	}
	for i := 0; i < unknown && i < total; i++ {
		ns[i/8].NPUChips[i%8].Health = "Unknown"
	}
	ns[0].NPUChipHealth[3].Grade = model.NPUChipGradeDegraded
	ns[0].NPUChipHealth[5].Grade = model.NPUChipGradeFault
	return &model.ClusterData{
		Nodes:   ns,
		Summary: &model.ClusterSummary{NPUCapacity: int64(total), NPUAllocatable: int64(total)},
	}
}

// TestB25VSummaryBarKeepsFaultCountAt38ColumnsOnLargeCluster pins the contract on
// a cluster whose chip total is wide enough that the health segment's own width
// matters. 13 nodes x 8 chips = 104 chips is the first size that fails; the sweep
// covers a realistic range either side of it.
//
// The judge is the highest-severity signal, not the layout: a bar that fits 38
// columns by dropping the fault count is exactly the failure the batch's own
// comment forbids, and a pod scheduled on such a cluster must not be able to hide
// a fault by making the bar one cell wider.
func TestB25VSummaryBarKeepsFaultCountAt38ColumnsOnLargeCluster(t *testing.T) {
	forcedAsciiProfile(t)

	for _, nodes := range []int{8, 13, 16, 32} {
		m := NewModel(nil, nil, 0, "en", "test", 200)
		m.width, m.height = 38, 40
		m.currentView = ViewNPU
		m.clusterData = b25vScaleCluster(nodes, 1)

		// The judge is the whole summary band, not one line: a correct fix may
		// wrap the counters onto their own line, and the "no fault" wording
		// would otherwise be mistaken for the fault counter itself.
		bar := b25vSummaryBand(stripANSI(m.renderNPUView()))
		if bar == "" {
			t.Fatalf("%d nodes (%d chips): no health summary band rendered:\n%s", nodes, nodes*8,
				m.renderNPUView())
		}
		if !strings.Contains(bar, "1 fault") {
			t.Errorf("%d nodes (%d chips): the 38-column bar lost the fault count: %q",
				nodes, nodes*8, bar)
		}
		if !strings.Contains(bar, "1 degraded") {
			t.Errorf("%d nodes (%d chips): the 38-column bar lost the degraded count: %q",
				nodes, nodes*8, bar)
		}
	}
}

// b25vSummaryBand returns the health-summary band: the lines between the view
// title and the legend, which is where the segment list wraps.
func b25vSummaryBand(view string) string {
	var band []string
	started := false
	for _, line := range strings.Split(view, "\n") {
		switch {
		case !started:
			if strings.Contains(line, "Overview") || strings.Contains(line, "总览") {
				started = true
			}
		case strings.Contains(line, "healthy  ") || strings.Contains(line, "健康  "):
			return strings.Join(band, "\n")
		default:
			band = append(band, line)
		}
	}
	return strings.Join(band, "\n")
}
