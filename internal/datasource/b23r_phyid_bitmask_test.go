package datasource

import (
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// Review follow-up: the device-plugin bitmask names the chip by PhyID, not by the
// chip's position in the exporter's enumeration.
func TestNPUChipHealthBitmaskUsesPhyID(t *testing.T) {
	chips := []model.NPUChipData{
		{NPUID: 1, Chip: 1, PhyID: 3, Health: "OK"},
		{NPUID: 0, Chip: 0, PhyID: 0, Health: "OK"},
		{NPUID: 1, Chip: 0, PhyID: 2, Health: "OK"},
		{NPUID: 0, Chip: 1, PhyID: 1, Health: "OK"},
	}
	node := &model.NodeData{Name: "n", NPUCapacity: 4, NPUChips: chips,
		NPUStatusKnown: true, NPUStatusMask: 0xffffffffffffff01}
	npuThresholdAlerts(node, time.Now())
	for i, g := range node.NPUChipHealth {
		wantFault := chips[i].PhyID == 0
		if wantFault != (g.Grade == model.NPUChipGradeFault) {
			t.Errorf("slice index %d (PhyID %d) graded %q, want fault only for PhyID 0",
				i, chips[i].PhyID, g.Grade)
		}
	}
}
