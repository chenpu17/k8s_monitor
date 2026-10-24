package ui

// b25v_row_suffix_test.go — reviewer probe for a within-batch regression on the
// NPU matrix row.

import (
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b25vGradedRow builds a 16-chip node with one degraded and one fault chip, and
// occupied chips attributed to a workload.
func b25vGradedRow(occupied int) *model.NodeData {
	grades := make([]model.NPUChipGrade, 16)
	chips := make([]model.NPUChipData, 16)
	for i := range grades {
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK"}
	}
	grades[3].Grade = model.NPUChipGradeDegraded
	grades[7].Grade = model.NPUChipGradeFault
	for i := 0; i < occupied; i++ {
		chips[i].OwnerNamespace, chips[i].OwnerPod = "ml", "train-0"
	}
	return &model.NodeData{
		Name: "node-a", Status: "Ready", NPUCapacity: 16, NPUAllocatable: 16,
		NPUChips: chips, NPUChipHealth: grades,
	}
}

// TestB25VRowSuffixKeepsGradeCountersWhenOccupied pins the priority order the
// batch's own comment states ("the matrix is the content of this view, so the
// summary is the first thing to go when the terminal is narrow") — but the
// suffix is dropped as one unit, so the *informational* "N occupied" segment
// appended by W2-2 evicts the degraded/fault counters that were rendered before
// it existed.
//
// At HEAD the 16-chip row shows "1 degraded • 1 fault" from 72 columns. After
// this batch, a node with two attributed chips loses both counters at 72 and 80
// columns (they come back only at 90+), because the row's suffix grew by 13
// cells and npuMatrixRow drops the whole suffix instead of the last segment.
// The fault count is the highest-severity signal the row carries; a scheduled
// pod must not be able to hide it.
func TestB25VRowSuffixKeepsGradeCountersWhenOccupied(t *testing.T) {
	forcedAsciiProfile(t)

	for _, occupied := range []int{1, 2, 4, 8} {
		for _, width := range []int{72, 80} {
			m := NewModel(nil, nil, 0, "en", "test", 200)
			m.width, m.height = width, 40
			row := stripANSI(m.npuMatrixRow(b25vGradedRow(occupied), false))

			if !strings.Contains(row, "1 fault") {
				t.Errorf("width %d, %d occupied: the row lost its fault counter: %q", width, occupied, row)
			}
			if !strings.Contains(row, "1 degraded") {
				t.Errorf("width %d, %d occupied: the row lost its degraded counter: %q", width, occupied, row)
			}
		}
	}
}
