package ui

// b25v_fixround_test.go — reviewer round-3 probes against the fix commit
// 95e8a8c. These target the new degradation path the three P1 fixes added,
// not the batch's original surfaces.

import (
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b25vHealthyOccupiedRow builds a fully healthy 16-chip node with n chips
// attributed to a pod: the shape where the grade counters are empty and the
// occupancy segment is the only suffix.
func b25vHealthyOccupiedRow(occupied int) *model.NodeData {
	chips := make([]model.NPUChipData, 16)
	grades := make([]model.NPUChipGrade, 16)
	for i := range chips {
		chips[i] = model.NPUChipData{NPUID: i / 2, Chip: i % 2, PhyID: i, Health: "OK"}
		grades[i] = model.NPUChipGrade{NPUID: i / 2, Chip: i % 2, Grade: model.NPUChipGradeHealthy}
	}
	for i := 0; i < occupied; i++ {
		chips[i].OwnerNamespace, chips[i].OwnerPod = "ml", "train-0"
	}
	return &model.NodeData{
		Name: "node-a", Status: "Ready", NPUCapacity: 16, NPUAllocatable: 16,
		NPUChips: chips, NPUChipHealth: grades,
	}
}

// TestB25VFixRoundRowHasNoTrailingPadding pins the byte hygiene of the suffix
// degradation added in the fix round. On a fully healthy node with attributed
// chips, the grade counters are empty, so the degradation path swaps the suffix
// to "" and then reports the row as having a suffix (`suffix != ""` is false but
// the recomputed `fits` is true), which appends the two-cell gap before an empty
// suffix — the row ends in two trailing spaces.
//
// A HEAD row of the same geometry (healthy, no attribution) has no suffix and
// therefore no trailing padding, so this is introduced by the fix. The bytes are
// invisible in a terminal but they are not the bytes a reader- or golden-based
// assertion sees, and any future golden of the healthy+occupied shape would bake
// them in.
func TestB25VFixRoundRowHasNoTrailingPadding(t *testing.T) {
	forcedAsciiProfile(t)

	for _, width := range []int{50, 55, 60, 62} {
		m := NewModel(nil, nil, 0, "en", "test", 200)
		m.width, m.height = width, 40
		row := m.npuMatrixRow(b25vHealthyOccupiedRow(2), false)

		if strings.HasSuffix(row, " ") {
			t.Errorf("width %d: the row ends in %d trailing spaces: %q",
				width, len(row)-len(strings.TrimRight(row, " ")), row)
		}
		if strings.Contains(stripANSI(row), "occupied") {
			continue // the suffix fit; no degradation happened at this width
		}
		// Degraded to no suffix at all: the row must end on the last matrix cell.
		plain := stripANSI(row)
		if strings.HasSuffix(plain, "  ") {
			t.Errorf("width %d: the degraded row keeps the suffix gap: %q", width, plain)
		}
	}
}
