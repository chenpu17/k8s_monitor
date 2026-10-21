package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// renderNPUDetailForTest renders the node detail view for a single node and
// strips ANSI codes so the assertions read the text the user sees. The model is
// tall enough that the scroll window never clips the NPU section.
func renderNPUDetailForTest(t *testing.T, node *model.NodeData) string {
	t.Helper()

	m := newTestModel(t)
	m.height = 200
	m.selectedNode = node
	return stripANSI(m.renderNodeDetail())
}

// indexOf returns the offset of substr in s, or -1.
func indexOf(s, substr string) int { return strings.Index(s, substr) }

// TestRenderNodeDetailChipHealth covers the per-chip health bitmap block: the
// cells follow NPUCapacity (not a hardcoded 8), faulty chips are marked ✗, and
// an unreported mask shows the muted "not reported" text instead of cells.
func TestRenderNodeDetailChipHealth(t *testing.T) {
	cases := []struct {
		name        string
		node        *model.NodeData
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name: "all chips healthy",
			node: &model.NodeData{
				Name: "node-ok", Status: "Ready",
				NPUCapacity: 3, NPUAllocatable: 3,
				NPUStatusKnown: true, NPUStatusMask: 0x0,
			},
			wantPresent: []string{"Chip Health", "[0]✓", "[1]✓", "[2]✓"},
			wantAbsent:  []string{"[3]", "✗", "Not reported"},
		},
		{
			name: "chip 0 faulty",
			node: &model.NodeData{
				Name: "node-fault", Status: "Ready",
				NPUCapacity: 4, NPUAllocatable: 4,
				NPUStatusKnown: true, NPUStatusMask: 0x01,
			},
			wantPresent: []string{"[0]✗", "[1]✓", "[2]✓", "[3]✓", "Fault: 1"},
			wantAbsent:  []string{"[0]✓", "[4]"},
		},
		{
			name: "chip 3 faulty on an 8-chip node",
			node: &model.NodeData{
				Name: "node-8", Status: "Ready",
				NPUCapacity: 8, NPUAllocatable: 8,
				NPUStatusKnown: true, NPUStatusMask: 0x08,
			},
			wantPresent: []string{"[3]✗", "[7]✓"},
			wantAbsent:  []string{"[8]", "[3]✓"},
		},
		{
			name: "status not reported",
			node: &model.NodeData{
				Name: "node-unknown", Status: "Ready",
				NPUCapacity: 8, NPUAllocatable: 8,
				NPUStatusKnown: false, NPUStatusMask: 0xff,
			},
			wantPresent: []string{"Chip Health", "Not reported"},
			wantAbsent:  []string{"[0]", "✓", "✗"},
		},
		{
			// Chips past the last bit the device plugin reports must not be
			// rendered as healthy: the mask says nothing about them.
			name: "capacity beyond reported bit width",
			node: &model.NodeData{
				Name: "node-wide", Status: "Ready",
				NPUCapacity: 10, NPUAllocatable: 10,
				NPUStatusKnown: true, NPUStatusMask: 0x01,
			},
			wantPresent: []string{"[0]✗", "[7]✓", "[8]?", "[9]?"},
			wantAbsent:  []string{"[8]✓", "[9]✓", "[10]"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderNPUDetailForTest(t, tc.node)

			for _, want := range tc.wantPresent {
				if !strings.Contains(out, want) {
					t.Errorf("rendered node detail is missing %q\n---\n%s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("rendered node detail unexpectedly contains %q\n---\n%s", absent, out)
				}
			}
		})
	}
}

// TestRenderNodeDetailInspection covers the NPU inspection table: only NT_NPU_*
// conditions are listed, the prefix is stripped, statuses map to ✓ / ✗ / –, the
// heartbeat falls back to the transition time, and faulted checks sort ahead of
// the rest.
func TestRenderNodeDetailInspection(t *testing.T) {
	now := time.Now()
	twoHoursAgo := metav1.NewTime(now.Add(-2 * time.Hour))

	node := &model.NodeData{
		Name: "node-inspect", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
		Conditions: []corev1.NodeCondition{
			{
				Type:               "NT_NPU_HEALTH",
				Status:             corev1.ConditionFalse, // passed
				Message:            "all chips healthy",
				LastHeartbeatTime:  twoHoursAgo,
				LastTransitionTime: twoHoursAgo,
			},
			{
				Type:              "NT_NPU_HBM_ECC",
				Status:            corev1.ConditionTrue, // fault detected
				Message:           "hbm ecc\n  double bit error",
				LastHeartbeatTime: metav1.NewTime(now.Add(-5 * time.Minute)),
			},
			{
				Type:   "NT_NPU_NETWORK",
				Status: corev1.ConditionUnknown, // not applicable
				// both timestamps unset -> heartbeat "-"
			},
			{
				Type:   "Ready", // non-inspection condition must be ignored
				Status: corev1.ConditionTrue,
			},
		},
	}

	out := renderNPUDetailForTest(t, node)

	// The prefix is stripped from the rendered check names.
	for _, want := range []string{"NPU_HEALTH", "NPU_HBM_ECC", "NPU_NETWORK"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered node detail is missing inspection check %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "NT_") {
		t.Errorf("rendered node detail still shows the NT_ prefix\n---\n%s", out)
	}

	// Statuses: False -> ✓, True -> ✗, Unknown -> –.
	if idx := indexOf(out, "NPU_HBM_ECC"); idx < 0 {
		t.Fatalf("inspection row NPU_HBM_ECC not rendered\n---\n%s", out)
	} else if line := lineContaining(out, "NPU_HBM_ECC"); !strings.Contains(line, "✗") {
		t.Errorf("faulted check row has no ✗ marker: %q", line)
	}
	if line := lineContaining(out, "NPU_HEALTH"); !strings.Contains(line, "✓") {
		t.Errorf("passing check row has no ✓ marker: %q", line)
	}
	if line := lineContaining(out, "NPU_NETWORK"); !strings.Contains(line, "–") {
		t.Errorf("not-applicable check row has no – marker: %q", line)
	}

	// Heartbeat: relative age from the heartbeat, "-" when nothing was reported.
	if line := lineContaining(out, "NPU_HEALTH"); !strings.Contains(line, "2h") {
		t.Errorf("check row does not show the 2h heartbeat age: %q", line)
	}
	if line := lineContaining(out, "NPU_NETWORK"); !strings.Contains(line, "-") {
		t.Errorf("check row with no timestamps does not show a placeholder: %q", line)
	}

	// Messages are collapsed onto one line.
	if line := lineContaining(out, "NPU_HBM_ECC"); !strings.Contains(line, "hbm ecc double bit error") {
		t.Errorf("check message was not collapsed onto a single line: %q", line)
	}

	// Faulted checks sort ahead of the rest.
	if i, j := indexOf(out, "NPU_HBM_ECC"), indexOf(out, "NPU_HEALTH"); i > j {
		t.Errorf("faulted check NPU_HBM_ECC (idx %d) did not sort before NPU_HEALTH (idx %d)\n---\n%s", i, j, out)
	}
	if i, j := indexOf(out, "NPU_HBM_ECC"), indexOf(out, "NPU_NETWORK"); i > j {
		t.Errorf("faulted check NPU_HBM_ECC (idx %d) did not sort before NPU_NETWORK (idx %d)\n---\n%s", i, j, out)
	}
}

// TestRenderNodeDetailInspectionIgnoresNonNPUChecks locks the prefix filter: a
// live healthy Ascend node carries ~40 NT_* conditions, and the non-NPU ones are
// not fault signals. NT_NODE_YUM_MANAGER_LAUNCHER in particular is permanently
// True on healthy nodes, so listing it would report a fault on a healthy node.
func TestRenderNodeDetailInspectionIgnoresNonNPUChecks(t *testing.T) {
	node := &model.NodeData{
		Name: "node-mixed", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
		Conditions: []corev1.NodeCondition{
			{Type: "NT_NODE_YUM_MANAGER_LAUNCHER", Status: corev1.ConditionTrue, Message: "launcher result text"},
			{Type: "NT_NPU_CARD_LOSE", Status: corev1.ConditionTrue, Message: "npu card lost"},
			{Type: "NT_NTP_SYNC", Status: corev1.ConditionTrue, Message: "ntp result text"},
			{Type: "NT_CABINET_DATA", Status: corev1.ConditionTrue, Message: "cabinet data result"},
			{Type: "NT_ASCEND_RANK_TABLE_DATA", Status: corev1.ConditionTrue, Message: "rank table result"},
			{Type: "NT_SFS_MOUNT", Status: corev1.ConditionTrue, Message: "sfs result text"},
		},
	}

	out := renderNPUDetailForTest(t, node)

	if !strings.Contains(out, "NPU Inspection") {
		t.Fatalf("NPU inspection section not rendered\n---\n%s", out)
	}
	if !strings.Contains(out, "NPU_CARD_LOSE") {
		t.Errorf("the NT_NPU_ check was not listed\n---\n%s", out)
	}

	for _, absent := range []string{
		"YUM_MANAGER",
		"NTP_SYNC",
		"CABINET_DATA",
		"RANK_TABLE_DATA",
		"SFS_MOUNT",
		"launcher result text",
	} {
		if strings.Contains(out, absent) {
			t.Errorf("non-NPU inspection check leaked into the NPU table: %q\n---\n%s", absent, out)
		}
	}

	// Exactly one inspection row: the header plus the single NT_NPU_ check.
	rows := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "NPU_CARD_LOSE") || strings.Contains(line, "NPU Inspection") {
			rows++
		}
	}
	if rows != 2 {
		t.Errorf("expected exactly one inspection row (section header + check), got %d\n---\n%s", rows, out)
	}
}

// TestRenderNodeDetailManagementFault covers the management-plane fault block:
// it appears only when a non-normal device fault level is reported, and carries
// the switch fault name when one is present.
func TestRenderNodeDetailManagementFault(t *testing.T) {
	cases := []struct {
		name        string
		fault       *model.NPUFaultInfo
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "no fault info",
			fault:       nil,
			wantAbsent:  []string{"Management Faults"},
			wantPresent: nil,
		},
		{
			name:        "normal fault level is hidden",
			fault:       &model.NPUFaultInfo{DeviceFaultLevel: "NormalNPU"},
			wantAbsent:  []string{"Management Faults", "NormalNPU"},
			wantPresent: nil,
		},
		{
			name: "device and switch fault",
			fault: &model.NPUFaultInfo{
				DeviceFaultLevel:  "RestartNPU",
				DeviceFaultEvents: []string{"device 3 restart\nrequested"},
				SwitchFaultLevel:  "SwitchFault",
				SwitchFaultName:   "switch-0/1",
			},
			wantPresent: []string{
				"Management Faults",
				"RestartNPU",
				"device 3 restart requested",
				"switch-0/1",
				"SwitchFault",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := &model.NodeData{
				Name: "node-fault", Status: "Ready",
				NPUCapacity: 8, NPUAllocatable: 8,
				NPUStatusKnown: true,
				NPUFault:       tc.fault,
			}
			out := renderNPUDetailForTest(t, node)

			for _, want := range tc.wantPresent {
				if !strings.Contains(out, want) {
					t.Errorf("rendered node detail is missing %q\n---\n%s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("rendered node detail unexpectedly contains %q\n---\n%s", absent, out)
				}
			}
		})
	}
}

// TestRenderNodeDetailNPUSectionsHiddenWithoutCapacity guards the precondition:
// a node with no NPU capacity must not grow any of the new blocks.
func TestRenderNodeDetailNPUSectionsHiddenWithoutCapacity(t *testing.T) {
	node := &model.NodeData{
		Name: "plain-node", Status: "Ready",
		CPUCapacity: 8000, CPUAllocatable: 8000,
		MemAllocatable: 16 * 1024 * 1024 * 1024,
		NPUCapacity:    0,
		NPUStatusKnown: true,
		NPUFault:       &model.NPUFaultInfo{DeviceFaultLevel: "RestartNPU"},
		Conditions: []corev1.NodeCondition{
			{Type: "NT_NPU_HEALTH", Status: corev1.ConditionTrue, Message: "boom"},
		},
	}

	out := renderNPUDetailForTest(t, node)

	for _, absent := range []string{"Chip Health", "Management Faults", "NPU Inspection", "NPU Details"} {
		if strings.Contains(out, absent) {
			t.Errorf("node without NPU capacity rendered the %q section\n---\n%s", absent, out)
		}
	}
}

// lineContaining returns the first rendered line containing substr.
func lineContaining(out, substr string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}
