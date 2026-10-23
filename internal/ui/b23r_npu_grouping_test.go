package ui

import (
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// Review follow-up: NPU grouping must follow NPUID for a chip layout that is not the
// exporter's two-chips-per-NPU default (the DaemonSet collector path reports
// chip.NPU verbatim, so 4 chips per NPU is a real shape).
func TestNPUViewGroupingFollowsNPUID(t *testing.T) {
	forcedAsciiProfile(t)

	grades := make([]model.NPUChipGrade, 16)
	for i := range grades {
		grades[i] = model.NPUChipGrade{NPUID: i / 4, Chip: i % 4, Grade: model.NPUChipGradeHealthy}
	}
	m := npuViewModel(t, "en", 120, 40)
	m.clusterData.Nodes = []*model.NodeData{{
		Name: "npu-1", Status: "Ready", NPUCapacity: 16, NPUAllocatable: 16, NPUChipHealth: grades,
	}}
	m.clusterData.Summary.NPUCapacity = 16
	m.clusterData.Summary.NPUAllocatable = 16

	row := lineContaining(m.renderNPUView(), "npu-1")
	row = stripANSI(row)
	if row == "" {
		t.Fatal("no node row")
	}
	if want := "▉ ▉ ▉ ▉  ▉ ▉ ▉ ▉  ▉ ▉ ▉ ▉  ▉ ▉ ▉ ▉"; !strings.Contains(row, want) {
		t.Errorf("4-chips-per-NPU layout is not grouped per NPU: %q (want %q)", row, want)
	}
	// non-vacuousness: the 2-per-NPU grouping would look different
	if strings.Contains(row, "▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉  ▉ ▉") {
		t.Fatalf("probe vacuous: the row is laid out 2 per NPU (%q)", row)
	}
}
