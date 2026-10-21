package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
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

// TestRenderNodeDetailNPUBlocksBilingual locks the new NPU keys in both
// bundles: go-i18n hands back the message id unchanged when a key is missing,
// so an unlocalised key shows up in the view as "npudetail.<key>".
func TestRenderNodeDetailNPUBlocksBilingual(t *testing.T) {
	node := &model.NodeData{
		Name: "node-i18n", Status: "Ready",
		NPUCapacity: 10, NPUAllocatable: 10,
		NPUStatusKnown: true, NPUStatusMask: 0x01,
		NPUHealthStatus: "Healthy",
		NPUChips:        []model.NPUChipData{{PhyID: 0, Health: "OK", AICore: 10}},
		NPUTopology:     []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}}},
		NPUDeviceNet: []model.NPUDeviceNetEntry{
			{DeviceID: 0, DeviceIP: "29.11.0.10", TORIP: "29.79.161.163", TORPort: "17:1"},
		},
	}

	cases := []struct {
		locale string
		want   []string
		hint   []string // lines of the no-exporter hint, split over two rows
	}{
		{
			locale: "en",
			want: []string{
				"Chip Health  (device plugin bitmap)",
				"(npu-exporter)",
				"(2 not reported)",
				"HCCS Affinity: [0-7]",
				"NPU Network Mapping",
				"DEVICE IP", "TOR SWITCH", "TOR PORT",
			},
			hint: []string{"NPU-Exporter not reachable", "or not deployed in kube-system namespace"},
		},
		{
			locale: "zh",
			want: []string{
				"芯片健康  (设备插件位图)",
				"(npu-exporter)",
				"(2 未上报)",
				"HCCS 亲和: [0-7]",
				"NPU 网络映射",
				"设备IP", "TOR交换机", "TOR端口",
			},
			hint: []string{"NPU-Exporter 不可达", "或未部署在 kube-system 命名空间"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			m := newTestModel(t)
			m.localizer = i18n.NewLocalizer(tc.locale)
			m.height = 200
			m.selectedNode = node
			out := stripANSI(m.renderNodeDetail())

			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("rendered node detail is missing %q\n---\n%s", want, out)
				}
			}
			if strings.Contains(out, "npudetail.") {
				t.Errorf("an NPU translation key was not resolved\n---\n%s", out)
			}

			// A node with NPU capacity but no chip metrics renders the
			// no-exporter hint; it must be split over two rows (the old single
			// line was 71 cells and overflowed narrow terminals).
			m.selectedNode = &model.NodeData{
				Name: "node-hint", Status: "Ready",
				NPUCapacity: 8, NPUAllocatable: 8, NPUStatusKnown: true,
			}
			hintOut := stripANSI(m.renderNodeDetail())
			for _, want := range tc.hint {
				line := lineContaining(hintOut, want)
				if line == "" {
					t.Errorf("no-exporter hint is missing %q\n---\n%s", want, hintOut)
					continue
				}
				if w := runewidth.StringWidth(line); w > 64 {
					t.Errorf("no-exporter hint line is %d cells wide: %q", w, line)
				}
			}
			if strings.Contains(hintOut, "npudetail.") {
				t.Errorf("an NPU translation key was not resolved\n---\n%s", hintOut)
			}
		})
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

// blockAfter returns the rendered block introduced by header: the header line
// and every following line up to the next blank line. It returns nil when the
// header is not rendered.
func blockAfter(out, header string) []string {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.Contains(line, header) {
			continue
		}
		block := []string{line}
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				break
			}
			block = append(block, next)
		}
		return block
	}
	return nil
}

// renderNPUDetailAtWidth renders the node detail view at a given terminal width.
func renderNPUDetailAtWidth(t *testing.T, node *model.NodeData, width int) string {
	t.Helper()

	m := newTestModel(t)
	m.width = width
	m.height = 200
	m.selectedNode = node
	return stripANSI(m.renderNodeDetail())
}

// inspectionNode builds a node whose NPU inspection table has the widest
// realistic cells: a full-length check name and a long message.
func inspectionNode() *model.NodeData {
	now := metav1.NewTime(time.Now().Add(-90 * time.Minute))
	return &model.NodeData{
		Name: "node-inspect", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
		Conditions: []corev1.NodeCondition{
			{
				Type:              "NT_NPU_CARD_LOSE",
				Status:            corev1.ConditionTrue,
				Message:           "npu card lost on device 3, check the cable and the device plugin registration",
				LastHeartbeatTime: now,
			},
			{
				Type:              "NT_NPU_HBM_ECC",
				Status:            corev1.ConditionFalse,
				Message:           "hbm ecc counters are clean on all chips",
				LastHeartbeatTime: now,
			},
		},
	}
}

// TestRenderNodeDetailInspectionTableFitsTerminal locks the narrow-terminal
// fix: the inspection table used to keep a 20-cell MESSAGE column no matter how
// narrow the terminal was, so it overflowed by 3 columns at width 74-76. The
// MESSAGE column is now dropped when the table cannot fit, and no line of the
// NPU tables may exceed the terminal width from 60 columns up.
func TestRenderNodeDetailInspectionTableFitsTerminal(t *testing.T) {
	// A network mapping is added so both new NPU tables are width-checked.
	node := inspectionNode()
	node.NPUDeviceNet = []model.NPUDeviceNetEntry{
		{DeviceID: 0, DeviceIP: "29.11.0.10", TORIP: "29.79.161.163", TORPort: "17:1"},
		{DeviceID: 7, DeviceIP: "29.11.0.17", TORIP: "29.79.161.169", TORPort: "18:2"},
	}

	for _, width := range []int{60, 74, 80} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			out := renderNPUDetailAtWidth(t, node, width)

			inspection := blockAfter(out, "NPU Inspection")
			if len(inspection) == 0 {
				t.Fatalf("inspection table not rendered at width %d\n---\n%s", width, out)
			}
			network := blockAfter(out, "NPU Network Mapping")
			if len(network) == 0 {
				t.Fatalf("network table not rendered at width %d\n---\n%s", width, out)
			}

			for _, block := range [][]string{inspection, network} {
				for _, line := range block {
					if w := runewidth.StringWidth(line); w > width {
						t.Errorf("table line is %d cells wide, terminal is %d: %q\n---\n%s",
							w, width, line, out)
					}
				}
			}

			// The rows themselves must still be there at every width.
			for _, want := range []string{"NPU_CARD_LOSE", "NPU_HBM_ECC"} {
				if !strings.Contains(out, want) {
					t.Errorf("inspection row %q missing at width %d\n---\n%s", want, width, out)
				}
			}
		})
	}

	// Wide terminals keep the full table, message included.
	wide := renderNPUDetailAtWidth(t, node, 120)
	if line := lineContaining(wide, "MESSAGE"); line == "" {
		t.Errorf("MESSAGE column dropped on a wide terminal\n---\n%s", wide)
	}
	if line := lineContaining(wide, "NPU_CARD_LOSE"); !strings.Contains(line, "npu card lost on device 3") {
		t.Errorf("wide-terminal inspection row lost its message: %q", line)
	}

	// The message column only comes back once the terminal can hold it.
	if line := lineContaining(renderNPUDetailAtWidth(t, node, 76), "NPU Inspection"); line == "" {
		t.Fatal("inspection section missing at width 76")
	}
	if strings.Contains(renderNPUDetailAtWidth(t, node, 76), "MESSAGE") {
		t.Errorf("MESSAGE column kept at width 76 where the table cannot fit")
	}
	if !strings.Contains(renderNPUDetailAtWidth(t, node, 77), "MESSAGE") {
		t.Errorf("MESSAGE column dropped at width 77 where the table does fit")
	}
}

// TestRenderNodeDetailChipHealthLegendCoversReportedChipsOnly locks the legend
// count: the bitmap covers 8 chips, so a 10-chip node must not have its 2
// unreported chips counted as healthy (which produced "✓ OK: 9" next to 10
// cells). Unreported chips are called out separately.
func TestRenderNodeDetailChipHealthLegendCoversReportedChipsOnly(t *testing.T) {
	cases := []struct {
		name        string
		node        *model.NodeData
		wantLegend  string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name: "10 chips, chip 0 faulty",
			node: &model.NodeData{
				Name: "node-wide", Status: "Ready",
				NPUCapacity: 10, NPUAllocatable: 10,
				NPUStatusKnown: true, NPUStatusMask: 0x01,
			},
			wantLegend:  "      ✓ OK: 7   ✗ Fault: 1   (2 not reported)",
			wantPresent: []string{"[8]?", "[9]?"},
			wantAbsent:  []string{"✓ OK: 8", "✓ OK: 9", "[8]✓", "[9]✓"},
		},
		{
			name: "10 chips, no fault",
			node: &model.NodeData{
				Name: "node-wide-ok", Status: "Ready",
				NPUCapacity: 10, NPUAllocatable: 10,
				NPUStatusKnown: true, NPUStatusMask: 0x0,
			},
			wantLegend:  "      ✓ OK: 8   ✗ Fault: 0   (2 not reported)",
			wantPresent: []string{"[8]?", "[9]?"},
			wantAbsent:  []string{"✓ OK: 9", "✓ OK: 10", "[8]✓", "[9]✓"},
		},
		{
			name: "capacity inside the bitmask, no fault",
			node: &model.NodeData{
				Name: "node-8", Status: "Ready",
				NPUCapacity: 8, NPUAllocatable: 8,
				NPUStatusKnown: true, NPUStatusMask: 0x0,
			},
			// Every chip is covered, so there is no legend at all.
			wantLegend:  "",
			wantAbsent:  []string{"✓ OK:", "Fault:", "not reported"},
			wantPresent: []string{"[0]✓", "[7]✓"},
		},
		{
			name: "capacity far beyond the bitmask is capped",
			node: &model.NodeData{
				Name: "node-huge", Status: "Ready",
				NPUCapacity: 100, NPUAllocatable: 100,
				NPUStatusKnown: true, NPUStatusMask: 0x0,
			},
			// 64 cells are drawn, 8 of them covered by the bitmask.
			wantLegend: "      ✓ OK: 8   ✗ Fault: 0   (56 not reported)",
			wantAbsent: []string{"✓ OK: 9"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderNPUDetailForTest(t, tc.node)

			if tc.wantLegend == "" {
				if line := lineContaining(out, "✓ OK:"); line != "" {
					t.Errorf("unexpected chip health legend: %q\n---\n%s", line, out)
				}
			} else {
				line := lineContaining(out, "✓ OK:")
				if line != tc.wantLegend {
					t.Errorf("chip health legend = %q, want %q\n---\n%s", line, tc.wantLegend, out)
				}
			}

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

// TestRenderNodeDetailSignalSources locks the source annotations: the Chip
// Health bitmap and the runtime Health Status come from different producers
// (device plugin vs npu-exporter) and must be labelled as such.
func TestRenderNodeDetailSignalSources(t *testing.T) {
	node := &model.NodeData{
		Name: "node-src", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true, NPUStatusMask: 0x0,
		NPUHealthStatus: "Healthy",
		NPUChips:        []model.NPUChipData{{PhyID: 0, Health: "OK", AICore: 10}},
	}

	out := renderNPUDetailForTest(t, node)

	if !strings.Contains(out, "Chip Health  (device plugin bitmap)") {
		t.Errorf("Chip Health section is not labelled with its source\n---\n%s", out)
	}
	if !strings.Contains(out, "Health Status (npu-exporter)") {
		t.Errorf("runtime Health Status row is not labelled with its source\n---\n%s", out)
	}
}

// TestFormatSwitchGroups covers the HCCS affinity rendering: chip ids are
// compressed into ranges, groups of another interconnect type keep their own
// name, and empty groups are skipped.
func TestFormatSwitchGroups(t *testing.T) {
	cases := []struct {
		name   string
		groups []model.NPUSwitchGroup
		want   string
	}{
		{name: "no topology", groups: nil, want: ""},
		{
			name:   "single hccs group",
			groups: []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}}},
			want:   "[0-7]",
		},
		{
			name:   "two hccs groups",
			groups: []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}}},
			want:   "[0-3] [4-7]",
		},
		{
			name:   "non hccs type keeps its name",
			groups: []model.NPUSwitchGroup{{Type: "SIO", Groups: [][]int{{0, 1, 2, 3}}}},
			want:   "SIO [0-3]",
		},
		{
			name: "mixed types",
			groups: []model.NPUSwitchGroup{
				{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}},
				{Type: "SIO", Groups: [][]int{{8, 9}}},
			},
			want: "[0-7]  SIO [8-9]",
		},
		{
			name:   "empty group is skipped",
			groups: []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{}}}},
			want:   "",
		},
		{
			name:   "empty group next to a populated one",
			groups: []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{nil, {2, 3}}}},
			want:   "[2-3]",
		},
		{
			name:   "untyped group",
			groups: []model.NPUSwitchGroup{{Groups: [][]int{{5}}}},
			want:   "[5]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatSwitchGroups(tc.groups); got != tc.want {
				t.Errorf("formatSwitchGroups(%+v) = %q, want %q", tc.groups, got, tc.want)
			}
		})
	}
}

// TestCompressChipRanges covers the chip id range compression itself.
func TestCompressChipRanges(t *testing.T) {
	cases := []struct {
		name  string
		chips []int
		want  string
	}{
		{name: "empty", chips: nil, want: ""},
		{name: "single", chips: []int{3}, want: "3"},
		{name: "full run", chips: []int{0, 1, 2, 3, 4, 5, 6, 7}, want: "0-7"},
		{name: "run plus single", chips: []int{0, 1, 5}, want: "0-1,5"},
		{name: "unsorted input", chips: []int{5, 0, 1}, want: "0-1,5"},
		{name: "duplicates collapse", chips: []int{2, 2, 3}, want: "2-3"},
		{name: "separate runs", chips: []int{0, 1, 4, 5, 8}, want: "0-1,4-5,8"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compressChipRanges(tc.chips); got != tc.want {
				t.Errorf("compressChipRanges(%v) = %q, want %q", tc.chips, got, tc.want)
			}
		})
	}
}

// TestRenderNodeDetailHCCSAffinity covers the affinity line in the view: the
// real node reports one HCCS group of chips 0-7.
func TestRenderNodeDetailHCCSAffinity(t *testing.T) {
	node := &model.NodeData{
		Name: "node-hccs", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
		NPUTopology:    []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{0, 1, 2, 3, 4, 5, 6, 7}}}},
	}

	out := renderNPUDetailForTest(t, node)

	if line := lineContaining(out, "HCCS Affinity"); line != "    HCCS Affinity: [0-7]" {
		t.Errorf("HCCS affinity line = %q, want %q\n---\n%s", line, "    HCCS Affinity: [0-7]", out)
	}

	// No topology annotation -> no line at all.
	plain := &model.NodeData{
		Name: "node-plain", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
	}
	if strings.Contains(renderNPUDetailForTest(t, plain), "HCCS Affinity") {
		t.Errorf("HCCS affinity line rendered without a topology annotation")
	}

	// An empty group is not an affinity group.
	empty := &model.NodeData{
		Name: "node-empty", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
		NPUTopology:    []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{}}}},
	}
	if strings.Contains(renderNPUDetailForTest(t, empty), "HCCS Affinity") {
		t.Errorf("HCCS affinity line rendered for an empty group")
	}

	// Without NPU capacity the whole NPU section stays hidden, topology or not.
	noNPU := &model.NodeData{
		Name: "node-cpu", Status: "Ready",
		NPUTopology: []model.NPUSwitchGroup{{Type: "HCCS", Groups: [][]int{{0, 1}}}},
	}
	if strings.Contains(renderNPUDetailForTest(t, noNPU), "HCCS Affinity") {
		t.Errorf("HCCS affinity line rendered on a node without NPU capacity")
	}
}

// TestRenderNodeDetailNPUNetworkMapping covers the network mapping table: one
// row per device with its TOR switch and port, and no table at all when the
// rank-table annotation is absent.
func TestRenderNodeDetailNPUNetworkMapping(t *testing.T) {
	node := &model.NodeData{
		Name: "node-net", Status: "Ready",
		NPUCapacity: 8, NPUAllocatable: 8,
		NPUStatusKnown: true,
	}
	// The real node reports 8 devices on two TOR switches.
	for i := 0; i < 8; i++ {
		tor, port := "29.79.161.163", fmt.Sprintf("17:%d", i%2+1)
		if i >= 4 {
			tor, port = "29.79.161.169", fmt.Sprintf("18:%d", i%2+1)
		}
		node.NPUDeviceNet = append(node.NPUDeviceNet, model.NPUDeviceNetEntry{
			DeviceID: i,
			DeviceIP: fmt.Sprintf("29.11.0.%d", 10+i),
			TORIP:    tor,
			TORPort:  port,
		})
	}

	out := renderNPUDetailForTest(t, node)

	block := blockAfter(out, "NPU Network Mapping")
	if len(block) == 0 {
		t.Fatalf("network mapping section not rendered\n---\n%s", out)
	}
	if len(block) != 11 { // section title + header + separator + 8 device rows
		t.Errorf("network mapping block has %d lines, want 11 (title, header, separator, 8 rows)\n---\n%s", len(block), out)
	}

	header := block[1]
	for _, want := range []string{"ID", "DEVICE IP", "TOR SWITCH", "TOR PORT"} {
		if !strings.Contains(header, want) {
			t.Errorf("network mapping header is missing %q: %q", want, header)
		}
	}

	for i := 0; i < 8; i++ {
		line := lineContaining(out, fmt.Sprintf("29.11.0.%d", 10+i))
		if line == "" {
			t.Fatalf("device %d row not rendered\n---\n%s", i, out)
		}
		if !strings.Contains(line, fmt.Sprintf("%d", i)) {
			t.Errorf("device %d row lost its id: %q", i, line)
		}
		if !strings.Contains(line, "29.79.161.16") {
			t.Errorf("device %d row lost its TOR switch: %q", i, line)
		}
	}

	// No annotation -> no section.
	node.NPUDeviceNet = nil
	if strings.Contains(renderNPUDetailForTest(t, node), "NPU Network Mapping") {
		t.Errorf("network mapping section rendered without the rank-table annotation")
	}

	// Without NPU capacity the section stays hidden even with data.
	noNPU := &model.NodeData{
		Name: "node-cpu", Status: "Ready",
		NPUDeviceNet: []model.NPUDeviceNetEntry{{DeviceID: 0, DeviceIP: "29.11.0.10", TORIP: "29.79.161.163", TORPort: "17:1"}},
	}
	if strings.Contains(renderNPUDetailForTest(t, noNPU), "NPU Network Mapping") {
		t.Errorf("network mapping section rendered on a node without NPU capacity")
	}
}

// extendedChipNode builds a node whose NPU chips carry extended metrics, so the
// nine-column chip table (not the basic one) is rendered.
func extendedChipNode() *model.NodeData {
	return &model.NodeData{
		Name: "node-ext", Status: "Ready",
		NPUCapacity: 2, NPUAllocatable: 2,
		NPUStatusKnown:  true,
		NPUHealthStatus: "Healthy",
		NPUChips: []model.NPUChipData{
			{PhyID: 0, Health: "OK", AICore: 10, VectorUtil: 5.5, HBMUsed: 1024, HBMTotal: 32768,
				Temp: 60, Power: 150.5, Voltage: 0.85, AICoreFreq: 1800, LinkStatus: 1},
			{PhyID: 1, Health: "OK", AICore: 20, VectorUtil: 6.5, HBMUsed: 2048, HBMTotal: 32768,
				Temp: 71, Power: 160.5, Voltage: 0.86, AICoreFreq: 1900, LinkStatus: 0},
		},
	}
}

// TestRenderNodeDetailExtendedTableResponsive locks the narrow-terminal
// degradation of the extended NPU chip table: the nine-column table is 72 cells
// wide, so on narrower terminals its rightmost columns (Freq, Link) used to be
// clipped at the terminal edge. Whole columns are now dropped from the right in
// two tiers, and the header, the separator and the data rows must always agree
// on the surviving column set.
func TestRenderNodeDetailExtendedTableResponsive(t *testing.T) {
	// Row width = indent (4) + the surviving column widths + one separator per
	// gap between them.
	cases := []struct {
		width        int
		columns      []string
		wantAbsent   []string // column names that must leave the header
		blockAbsent  []string // cell values of dropped columns
		blockPresent []string // cell values of columns that survive the tier
		rowWidth     int
	}{
		{
			width:        120,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt", "Freq", "Link"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"1800MHz", "0.85V", "Up"},
			rowWidth:     72,
		},
		{
			width:        76,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt", "Freq", "Link"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"1800MHz", "0.85V", "Up"},
			rowWidth:     72,
		},
		{
			// 72 is the exact full-tier boundary: the nine-column table is
			// itself 72 cells, so it must survive un-degraded here.
			width:        72,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt", "Freq", "Link"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"1800MHz", "0.85V", "Up"},
			rowWidth:     72,
		},
		{
			// The nine-column table is exactly 72 cells, so 75 keeps every
			// column.
			width:        75,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt", "Freq", "Link"},
			wantAbsent:   []string{},
			blockAbsent:  []string{},
			blockPresent: []string{"1800MHz", "0.85V", "Up"},
			rowWidth:     72,
		},
		{
			width:        71,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt"},
			wantAbsent:   []string{"Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "Up", "Down"},
			blockPresent: []string{"0.85V", "0.86V", "60°C", "150W"},
			rowWidth:     59,
		},
		{
			width:        62,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power", "Volt"},
			wantAbsent:   []string{"Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "Up", "Down"},
			blockPresent: []string{"0.85V", "0.86V", "60°C", "150W"},
			rowWidth:     59,
		},
		{
			width:        61,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power"},
			wantAbsent:   []string{"Volt", "Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "0.85V", "0.86V", "Up", "Down"},
			blockPresent: []string{"60°C", "71°C", "150W", "160W"},
			rowWidth:     52,
		},
		{
			width:        60,
			columns:      []string{"ID", "AICore", "Vector", "HBM", "Temp", "Power"},
			wantAbsent:   []string{"Volt", "Freq", "Link"},
			blockAbsent:  []string{"1800MHz", "1900MHz", "0.85V", "0.86V", "Up", "Down"},
			blockPresent: []string{"60°C", "71°C", "150W", "160W"},
			rowWidth:     52,
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("width_%d", tc.width), func(t *testing.T) {
			out := renderNPUDetailAtWidth(t, extendedChipNode(), tc.width)

			block := blockAfter(out, "NPU Chip Details")
			if len(block) != 5 { // section title + header + separator + 2 chip rows
				t.Fatalf("extended chip table has %d lines, want 5\n---\n%s", len(block), out)
			}

			header := block[1]
			for _, col := range tc.columns {
				if !strings.Contains(header, col) {
					t.Errorf("header is missing column %q at width %d: %q", col, tc.width, header)
				}
			}
			for _, col := range tc.wantAbsent {
				if strings.Contains(header, col) {
					t.Errorf("header kept dropped column %q at width %d: %q", col, tc.width, header)
				}
			}

			// Every rendered line - header, separator and data rows alike - must
			// use the same column set and must fit the terminal.
			for _, line := range block[1:] {
				if w := runewidth.StringWidth(line); w != tc.rowWidth {
					t.Errorf("table line is %d cells wide, want %d at terminal width %d: %q",
						w, tc.rowWidth, tc.width, line)
				}
			}

			// Dropped columns must take their cell values with them, and the
			// surviving cells must keep their own values. Checking the whole
			// block (not just the header, and at every tier) is what catches a
			// mutation that writes a dropped column's value into a surviving
			// cell.
			blockText := strings.Join(block, "\n")
			for _, gone := range tc.blockAbsent {
				if strings.Contains(blockText, gone) {
					t.Errorf("dropped-column value %q still rendered at width %d:\n%s", gone, tc.width, blockText)
				}
			}
			for _, want := range tc.blockPresent {
				if !strings.Contains(blockText, want) {
					t.Errorf("surviving-column value %q missing at width %d:\n%s", want, tc.width, blockText)
				}
			}
		})
	}

	// The widest tier is the pre-existing layout: nine columns, unchanged width.
	wide := renderNPUDetailAtWidth(t, extendedChipNode(), 120)
	if line := lineContaining(wide, "1800MHz"); line == "" {
		t.Errorf("Freq column value missing on a wide terminal\n---\n%s", wide)
	}
}

// visualIndexOf returns the display column at which substr starts in s, or -1.
func visualIndexOf(s, substr string) int {
	idx := strings.Index(s, substr)
	if idx < 0 {
		return -1
	}
	return runewidth.StringWidth(s[:idx])
}

// TestRenderNodePodsInfoColumnAlignment reproduces the pods table misalignment:
// the STATUS cell carries ANSI codes from RenderStatus, and fmt's %-Ns padded it
// by byte count, so RESTARTS started eight cells early on the data rows. Header,
// separator and rows must now share one cell layout, and only the node's own
// pods may appear.
func TestRenderNodePodsInfoColumnAlignment(t *testing.T) {
	m := newTestModel(t)
	m.height = 200
	node := &model.NodeData{Name: "node-pods", Status: "Ready", PodCount: 2}
	m.clusterData = &model.ClusterData{Pods: []*model.PodData{
		{Name: "alpha-pod", Namespace: "default", Node: "node-pods", Phase: "Running", RestartCount: 3},
		{Name: "beta-pod", Namespace: "kube-system", Node: "node-pods", Phase: "Running", RestartCount: 0},
		{Name: "other-node-pod", Namespace: "default", Node: "other-node", Phase: "Running", RestartCount: 7},
	}}

	out := stripANSI(m.renderNodePodsInfo(node))
	lines := strings.Split(out, "\n")
	if len(lines) < 5 {
		t.Fatalf("pods block has %d lines, want at least 5\n---\n%s", len(lines), out)
	}

	if want := "📦 Pods on this Node (2)"; !strings.Contains(lines[0], want) {
		t.Errorf("pods title = %q, want it to contain %q", lines[0], want)
	}
	header, separator, rows := lines[2], lines[3], lines[4:]
	if len(rows) != 2 {
		t.Fatalf("rendered %d pod rows, want 2 (pods from other nodes must not appear)\n---\n%s", len(rows), out)
	}
	if strings.Contains(out, "other-node-pod") {
		t.Errorf("a pod from another node leaked into the table\n---\n%s", out)
	}

	// Column starts: NAME 2, NAMESPACE 38, STATUS 59, RESTARTS 75.
	wantStarts := []struct {
		column string
		start  int
	}{
		{"NAME", 2},
		{"NAMESPACE", 38},
		{"STATUS", 59},
		{"RESTARTS", 75},
	}
	for _, want := range wantStarts {
		if got := visualIndexOf(header, want.column); got != want.start {
			t.Errorf("header column %q starts at cell %d, want %d: %q", want.column, got, want.start, header)
		}
	}

	restartsCells := []string{"3", "0"}
	for i, row := range rows {
		if got := visualIndexOf(row, restartsCells[i]); got != 75 {
			t.Errorf("row %d restarts value starts at cell %d, want 75: %q", i, got, row)
		}
	}

	for _, line := range append([]string{header, separator}, rows...) {
		if w := runewidth.StringWidth(line); w != 85 {
			t.Errorf("pods table line is %d cells wide, want 85: %q", w, line)
		}
	}
}

// TestRenderNodePodsInfoBilingual locks the four detail.node.* pods keys in
// both bundles: go-i18n echoes the message id for a missing key, so any of
// these strings going unlocalised would surface as "detail.node.<key>".
func TestRenderNodePodsInfoBilingual(t *testing.T) {
	node := &model.NodeData{Name: "node-pods", Status: "Ready", PodCount: 3}
	manyPods := &model.ClusterData{Pods: []*model.PodData{
		{Name: "pod-a", Namespace: "default", Node: "node-pods", Phase: "Running"},
		{Name: "pod-b", Namespace: "default", Node: "node-pods", Phase: "Running"},
		{Name: "pod-c", Namespace: "default", Node: "node-pods", Phase: "Running"},
	}}

	cases := []struct {
		locale string
		title  string
		noInfo string
		noPods string
		more   string
	}{
		{
			locale: "en",
			title:  "📦 Pods on this Node (3)",
			noInfo: "No pod information available",
			noPods: "No pods running on this node",
			more:   "... and 2 more pods",
		},
		{
			locale: "zh",
			title:  "📦 本节点上的 Pod (3)",
			noInfo: "暂无 Pod 信息",
			noPods: "本节点上没有运行中的 Pod",
			more:   "... 还有 2 个 Pod",
		},
	}

	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			m := newTestModel(t)
			m.localizer = i18n.NewLocalizer(tc.locale)
			m.height = 20 // maxVisible clamps to 1, so the overflow line renders

			m.clusterData = nil
			if got := stripANSI(m.renderNodePodsInfo(node)); !strings.Contains(got, tc.noInfo) {
				t.Errorf("no-info state = %q, want it to contain %q", got, tc.noInfo)
			}

			m.clusterData = &model.ClusterData{Pods: []*model.PodData{
				{Name: "elsewhere", Namespace: "default", Node: "other-node", Phase: "Running"},
			}}
			if got := stripANSI(m.renderNodePodsInfo(node)); !strings.Contains(got, tc.noPods) {
				t.Errorf("no-pods state = %q, want it to contain %q", got, tc.noPods)
			}

			m.clusterData = manyPods
			got := stripANSI(m.renderNodePodsInfo(node))
			if !strings.Contains(got, tc.title) {
				t.Errorf("title = %q, want it to contain %q", got, tc.title)
			}
			if !strings.Contains(got, tc.more) {
				t.Errorf("overflow line missing: %q, want it to contain %q", got, tc.more)
			}
		})
	}
}

// TestRenderNodeDetailBasicChipTableWidth locks the basic chip table against
// overflowing its own header. The seed uses the real Ascend 910B1 geometry
// (64 GiB HBM per chip): the widest value "65536/65536MB (100%)" is 20 cells
// and once overflowed the old 15-cell column, so every data row was wider than
// the header and separator.
func TestRenderNodeDetailBasicChipTableWidth(t *testing.T) {
	node := &model.NodeData{
		Name: "node-basic", Status: "Ready",
		NPUCapacity: 2, NPUAllocatable: 2,
		NPUStatusKnown:  true,
		NPUHealthStatus: "Healthy",
		NPUChips: []model.NPUChipData{
			{PhyID: 0, Health: "OK", AICore: 10, HBMUsed: 10240, HBMTotal: 65536, Temp: 60, Power: 150.5},
			{PhyID: 1, Health: "OK", AICore: 20, HBMUsed: 65536, HBMTotal: 65536, Temp: 71, Power: 160.5},
		},
	}

	out := renderNPUDetailAtWidth(t, node, 120)
	block := blockAfter(out, "NPU Chip Details")
	if len(block) != 5 { // title + header + separator + 2 chip rows
		t.Fatalf("basic chip table has %d lines, want 5\n---\n%s", len(block), out)
	}
	if !strings.Contains(block[4], "65536/65536MB (100%)") {
		t.Errorf("basic table row lost the widest HBM value: %q", block[4])
	}
	for _, line := range block[1:] {
		if w := runewidth.StringWidth(line); w != 65 {
			t.Errorf("basic chip table line is %d cells wide, want 65: %q", w, line)
		}
	}
}

// eccChipNode builds an extended-metrics node whose ECC counters are non-zero,
// so the ECC summary row renders.
func eccChipNode() *model.NodeData {
	node := extendedChipNode()
	node.NPUChips[0].HBMEccSingleErr = 2
	node.NPUChips[1].HBMEccDoubleErr = 1
	return node
}

// TestRenderNodeDetailECCResponsive locks the ECC summary degradation: the full
// line is 62 cells and overflows below 64 columns, where the parenthesised
// qualifiers are dropped but the counts stay visible.
func TestRenderNodeDetailECCResponsive(t *testing.T) {
	cases := []struct {
		width   int
		want    string
		absent  []string
		maxLine int
	}{
		{
			width:   60,
			want:    "Single-bit: 2  Double-bit: 1",
			absent:  []string{"(correctable)", "(uncorrectable)"},
			maxLine: 60,
		},
		{
			// 63/64 bracket the qualifier threshold: compact strictly below
			// 64, the full line from 64 up.
			width:   63,
			want:    "Single-bit: 2  Double-bit: 1",
			absent:  []string{"(correctable)", "(uncorrectable)"},
			maxLine: 63,
		},
		{
			width:   64,
			want:    "Single-bit: 2 (correctable)  Double-bit: 1 (uncorrectable)",
			maxLine: 64,
		},
		{
			width:   120,
			want:    "Single-bit: 2 (correctable)  Double-bit: 1 (uncorrectable)",
			maxLine: 120,
		},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("width_%d", tc.width), func(t *testing.T) {
			out := renderNPUDetailAtWidth(t, eccChipNode(), tc.width)
			line := lineContaining(out, "Single-bit")
			if line == "" {
				t.Fatalf("ECC summary row not rendered\n---\n%s", out)
			}
			if !strings.Contains(line, tc.want) {
				t.Errorf("ECC row = %q, want it to contain %q", line, tc.want)
			}
			for _, gone := range tc.absent {
				if strings.Contains(line, gone) {
					t.Errorf("ECC row unexpectedly contains %q at width %d: %q", gone, tc.width, line)
				}
			}
			if w := runewidth.StringWidth(line); w > tc.maxLine {
				t.Errorf("ECC row is %d cells wide, exceeds terminal width %d: %q", w, tc.maxLine, line)
			}
		})
	}
}
