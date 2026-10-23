package datasource

// b23_wp1_chip_health_test.go — batch 23 W1: the per-chip health grades the
// datasource now produces alongside the NPU alerts.
//
// The grades exist because the chip attribution of an alert only survives as
// free text in Value ("chips 0, 3"), and the UI cannot recover it reliably from
// there. The rules are pinned here: which signal grades a chip how, that the
// most severe signal wins, that the reason is the one of the winning signal,
// that the order matches the node's chip order, and that a node without
// exporter data carries no grades at all (nil, not an empty slice).

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// gradedChips returns a node with n chips, all healthy and distinguishable by
// their NPU/Chip/PhyID triple (two chips per NPU, the exporter's layout).
func gradedChips(n int) []model.NPUChipData {
	chips := make([]model.NPUChipData, n)
	for i := range chips {
		chips[i] = model.NPUChipData{
			NPUID:  i / 2,
			Chip:   i % 2,
			PhyID:  i,
			Health: "OK",
		}
	}
	return chips
}

// gradeOf returns the grade entry for the chip with the given PhyID.
func gradeOf(t *testing.T, grades []model.NPUChipGrade, phyID int) model.NPUChipGrade {
	t.Helper()
	for _, grade := range grades {
		if grade.NPUID == phyID/2 && grade.Chip == phyID%2 {
			return grade
		}
	}
	t.Fatalf("no grade for chip %d in %+v", phyID, grades)
	return model.NPUChipGrade{}
}

// TestNPUChipHealthGrades pins one representative of every grading rule, plus
// the "healthy is the default" case.
func TestNPUChipHealthGrades(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func([]model.NPUChipData)
		wantChip   int
		wantGrade  string
		wantReason string
	}{
		{
			name:      "an untouched chip stays healthy with no reason",
			mutate:    func([]model.NPUChipData) {},
			wantChip:  0,
			wantGrade: model.NPUChipGradeHealthy,
		},
		{
			name: "temperature above the warning threshold degrades",
			mutate: func(chips []model.NPUChipData) {
				chips[1].Temp = npuTempWarningCelsius
			},
			wantChip:   1,
			wantGrade:  model.NPUChipGradeDegraded,
			wantReason: "temp 80 ge 80",
		},
		{
			name: "temperature above the critical threshold faults",
			mutate: func(chips []model.NPUChipData) {
				chips[1].Temp = npuTempCriticalCelsius + 1
			},
			wantChip:   1,
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "temp 91 ge 90",
		},
		{
			name: "single-bit ECC degrades",
			mutate: func(chips []model.NPUChipData) {
				chips[2].HBMEccSingleErr = 3
			},
			wantChip:   2,
			wantGrade:  model.NPUChipGradeDegraded,
			wantReason: "single-bit ECC",
		},
		{
			name: "double-bit ECC faults",
			mutate: func(chips []model.NPUChipData) {
				chips[2].HBMEccDoubleErr = 1
			},
			wantChip:   2,
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "double-bit ECC",
		},
		{
			name: "a reported link down faults",
			mutate: func(chips []model.NPUChipData) {
				chips[3].LinkStatusKnown = true
				chips[3].LinkStatus = 0
			},
			wantChip:   3,
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "link down",
		},
		{
			name: "a missing link status says nothing",
			mutate: func(chips []model.NPUChipData) {
				chips[3].LinkStatusKnown = false
				chips[3].LinkStatus = 0
			},
			wantChip:  3,
			wantGrade: model.NPUChipGradeHealthy,
		},
		{
			name: "an error code faults",
			mutate: func(chips []model.NPUChipData) {
				chips[4].ErrorCode = 42
			},
			wantChip:   4,
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "error code 42",
		},
		{
			// Batch 24: the exporter's health_status is three-valued upstream
			// (1 healthy, 0 unhealthy, -1 DCMI failure), so the converted chip
			// never carries "Warning". A chip whose health is not known must not
			// be graded on it: absence of a signal is not a signal.
			name: "the exporter health Unknown says nothing",
			mutate: func(chips []model.NPUChipData) {
				chips[5].Health = "Unknown"
			},
			wantChip:  5,
			wantGrade: model.NPUChipGradeHealthy,
		},
		{
			name: "the exporter health Error faults",
			mutate: func(chips []model.NPUChipData) {
				chips[5].Health = "Error"
			},
			wantChip:   5,
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "chip health Error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chips := gradedChips(8)
			tc.mutate(chips)
			node := &model.NodeData{Name: "npu-node", NPUCapacity: 8, NPUChips: chips}

			npuThresholdAlerts(node, time.Now())

			if len(node.NPUChipHealth) != len(chips) {
				t.Fatalf("got %d grades for %d chips: %+v", len(node.NPUChipHealth), len(chips), node.NPUChipHealth)
			}

			got := gradeOf(t, node.NPUChipHealth, tc.wantChip)
			if got.Grade != tc.wantGrade {
				t.Errorf("chip %d grade = %q, want %q", tc.wantChip, got.Grade, tc.wantGrade)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("chip %d reason = %q, want %q", tc.wantChip, got.Reason, tc.wantReason)
			}

			// Every other chip must be untouched by the mutation.
			for i := range chips {
				if i == tc.wantChip {
					continue
				}
				if other := gradeOf(t, node.NPUChipHealth, i); other.Grade != model.NPUChipGradeHealthy {
					t.Errorf("chip %d was graded %q by a mutation on chip %d", i, other.Grade, tc.wantChip)
				}
			}
		})
	}
}

// TestNPUChipHealthTakesHighestGrade pins that a chip hit by several signals
// keeps the most severe one, with the reason of that signal.
func TestNPUChipHealthTakesHighestGrade(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*model.NPUChipData)
		wantGrade  string
		wantReason string
	}{
		{
			name: "degrading signals stack to degraded",
			mutate: func(chip *model.NPUChipData) {
				chip.Temp = npuTempWarningCelsius
				chip.HBMEccSingleErr = 2
			},
			wantGrade: model.NPUChipGradeDegraded,
			// Temperature is evaluated first, so it is the reason that survives.
			wantReason: "temp 80 ge 80",
		},
		{
			name: "a faulting signal outranks the degrading ones",
			mutate: func(chip *model.NPUChipData) {
				chip.Temp = npuTempWarningCelsius
				chip.HBMEccSingleErr = 2
				chip.ErrorCode = 7
			},
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "error code 7",
		},
		{
			name: "a faulting signal cannot be lowered by a later degrading one",
			mutate: func(chip *model.NPUChipData) {
				chip.HBMEccDoubleErr = 1
				chip.Health = "Unknown"
			},
			wantGrade:  model.NPUChipGradeFault,
			wantReason: "double-bit ECC",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chips := gradedChips(4)
			tc.mutate(&chips[1])
			node := &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: chips}

			npuThresholdAlerts(node, time.Now())

			got := gradeOf(t, node.NPUChipHealth, 1)
			if got.Grade != tc.wantGrade {
				t.Errorf("grade = %q, want %q", got.Grade, tc.wantGrade)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}

// TestNPUChipHealthKeepsChipOrder pins that the grades follow the node's own
// chip order (the order npu_health.go reports chips in), including a node whose
// exporter enumeration is not sorted.
func TestNPUChipHealthKeepsChipOrder(t *testing.T) {
	chips := []model.NPUChipData{
		{NPUID: 1, Chip: 1, PhyID: 3, Health: "OK"},
		{NPUID: 0, Chip: 0, PhyID: 0, Health: "OK"},
		{NPUID: 1, Chip: 0, PhyID: 2, Health: "OK"},
		{NPUID: 0, Chip: 1, PhyID: 1, Health: "OK"},
	}
	chips[2].ErrorCode = 9

	node := &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: chips}
	npuThresholdAlerts(node, time.Now())

	want := []model.NPUChipGrade{
		{NPUID: 1, Chip: 1, Grade: model.NPUChipGradeHealthy},
		{NPUID: 0, Chip: 0, Grade: model.NPUChipGradeHealthy},
		{NPUID: 1, Chip: 0, Grade: model.NPUChipGradeFault, Reason: "error code 9"},
		{NPUID: 0, Chip: 1, Grade: model.NPUChipGradeHealthy},
	}
	if !reflect.DeepEqual(node.NPUChipHealth, want) {
		t.Errorf("grades do not follow the node's chip order:\n got %+v\nwant %+v", node.NPUChipHealth, want)
	}
}

// TestNPUChipHealthNilWithoutExporterData pins the presence rule: a node with no
// exporter chip metrics carries nil grades, so the view can tell "no data" from
// "all healthy" (an empty non-nil slice would read as the latter).
func TestNPUChipHealthNilWithoutExporterData(t *testing.T) {
	node := &model.NodeData{Name: "npu-node", NPUCapacity: 8}
	npuThresholdAlerts(node, time.Now())
	if node.NPUChipHealth != nil {
		t.Errorf("a node without chip metrics carries %+v, want nil", node.NPUChipHealth)
	}

	// Chips present but no signal at all: every chip is healthy, not nil.
	node = &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: gradedChips(4)}
	npuThresholdAlerts(node, time.Now())
	if len(node.NPUChipHealth) != 4 {
		t.Fatalf("got %d grades, want 4", len(node.NPUChipHealth))
	}
	for _, grade := range node.NPUChipHealth {
		if grade.Grade != model.NPUChipGradeHealthy {
			t.Errorf("chip %d/%d = %q, want healthy", grade.NPUID, grade.Chip, grade.Grade)
		}
	}
}

// TestNPUChipHealthDevicePluginBitmask pins the device-plugin signal: a chip the
// bitmask flags is faulty even though the exporter metrics said nothing about
// it, and a chip outside the bitmap stays as the metrics left it.
func TestNPUChipHealthDevicePluginBitmask(t *testing.T) {
	node := &model.NodeData{
		Name:           "npu-node",
		NPUCapacity:    16,
		NPUChips:       gradedChips(16),
		NPUStatusMask:  0xffffffffffffff01, // chip 0 faulty, the rest healthy
		NPUStatusKnown: true,
	}
	npuThresholdAlerts(node, time.Now())

	if got := gradeOf(t, node.NPUChipHealth, 0); got.Grade != model.NPUChipGradeFault || got.Reason != "device plugin fault bit" {
		t.Errorf("chip 0 = %+v, want fault from the device plugin bit", got)
	}
	for _, phyID := range []int{1, 2, 15} {
		if got := gradeOf(t, node.NPUChipHealth, phyID); got.Grade != model.NPUChipGradeHealthy {
			t.Errorf("chip %d = %+v, want healthy", phyID, got)
		}
	}

	// An unreported bitmask (all ones) is "unknown", never "every chip faulty".
	node = &model.NodeData{
		Name:           "npu-node",
		NPUCapacity:    4,
		NPUChips:       gradedChips(4),
		NPUStatusMask:  0xffffffffffffffff,
		NPUStatusKnown: false,
	}
	npuThresholdAlerts(node, time.Now())
	for _, grade := range node.NPUChipHealth {
		if grade.Grade != model.NPUChipGradeHealthy {
			t.Errorf("chip %d/%d = %q after an unreported bitmask, want healthy", grade.NPUID, grade.Chip, grade.Grade)
		}
	}
}

// TestNPUChipHealthDeviceFaultLevel pins the management-plane signal: a device
// fault level grades every chip of the node (it is not attributable to one
// chip), a switch fault grades none (a faulty switch is not a faulty chip).
func TestNPUChipHealthDeviceFaultLevel(t *testing.T) {
	chips := gradedChips(4)

	node := &model.NodeData{
		Name: "npu-node", NPUCapacity: 4, NPUChips: chips,
		NPUFault: &model.NPUFaultInfo{DeviceFaultLevel: "UrgentNPU"},
	}
	npuThresholdAlerts(node, time.Now())
	for _, grade := range node.NPUChipHealth {
		if grade.Grade != model.NPUChipGradeFault {
			t.Errorf("chip %d/%d = %q under an urgent device fault, want fault", grade.NPUID, grade.Chip, grade.Grade)
		}
		if grade.Reason != "device fault level UrgentNPU" {
			t.Errorf("chip %d/%d reason = %q", grade.NPUID, grade.Chip, grade.Reason)
		}
	}

	// An ImportantNPU level is a warning on the alerts side, so it degrades.
	node = &model.NodeData{
		Name: "npu-node", NPUCapacity: 4, NPUChips: gradedChips(4),
		NPUFault: &model.NPUFaultInfo{DeviceFaultLevel: "ImportantNPU"},
	}
	npuThresholdAlerts(node, time.Now())
	for _, grade := range node.NPUChipHealth {
		if grade.Grade != model.NPUChipGradeDegraded {
			t.Errorf("chip %d/%d = %q under an important device fault, want degraded", grade.NPUID, grade.Chip, grade.Grade)
		}
	}

	// NormalNPU is no fault, and a switch fault alone leaves the chips healthy.
	node = &model.NodeData{
		Name: "npu-node", NPUCapacity: 4, NPUChips: gradedChips(4),
		NPUFault: &model.NPUFaultInfo{DeviceFaultLevel: "NormalNPU", SwitchFaultLevel: "Warning"},
	}
	npuThresholdAlerts(node, time.Now())
	for _, grade := range node.NPUChipHealth {
		if grade.Grade != model.NPUChipGradeHealthy {
			t.Errorf("chip %d/%d = %q without a device fault, want healthy", grade.NPUID, grade.Chip, grade.Grade)
		}
	}
}

// chipsNamedIn parses the chip ids out of an alert Value ("chips 0, 3").
func chipsNamedIn(value string) []int {
	ids := make([]int, 0, 4)
	for _, field := range strings.Split(strings.TrimPrefix(value, "chips "), ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(field)); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// TestNPUChipHealthMatchesAlertedChips is the consistency probe: the set of
// chips graded degraded-or-worse must be exactly the set the per-chip alerts
// name. The two come out of the same detection loop, and this pins that they
// cannot drift apart.
func TestNPUChipHealthMatchesAlertedChips(t *testing.T) {
	chips := gradedChips(8)
	chips[0].Temp = npuTempWarningCelsius + 2 // temperature warning
	chips[1].Temp = npuTempCriticalCelsius    // temperature critical
	chips[2].HBMEccSingleErr = 1              // single-bit ECC
	chips[3].HBMEccDoubleErr = 1              // double-bit ECC
	chips[4].LinkStatusKnown = true           // link down
	chips[4].LinkStatus = 0
	chips[5].ErrorCode = 11 // error code

	node := &model.NodeData{Name: "npu-node", NPUCapacity: 8, NPUChips: chips}
	alerts := npuThresholdAlerts(node, time.Now())

	alerted := map[int]bool{}
	for _, alert := range alerts {
		for _, chip := range chipsNamedIn(alert.Value) {
			alerted[chip] = true
		}
	}

	for i, grade := range node.NPUChipHealth {
		if alerted[i] != (grade.Grade != model.NPUChipGradeHealthy) {
			t.Errorf("chip %d: graded %q but alerted=%v (alerts: %+v)", i, grade.Grade, alerted[i], alerts)
		}
	}
	// Non-vacuousness: the fixture must actually have alerted on some chips.
	if len(alerted) == 0 {
		t.Fatal("the probe is vacuous: no chip was alerted on")
	}
}
