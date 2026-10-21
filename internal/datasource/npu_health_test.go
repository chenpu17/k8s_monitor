package datasource

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Real values observed on the Ascend910B1 node (8 chips, all healthy).
const (
	realMaskAllHealthy  = "0xffffffffffffff00"
	realMaskChip0       = "0xffffffffffffff01"
	realMaskNotReported = "0xffffffffffffffff"
	realFaultInfo       = `{"SwitchFault":{"SwitchName":"","FaultCode":null,"FaultLevel":"","FaultEvent":null},` +
		`"DeviceFault":{"DevType":"Ascend910","FaultLevel":"NormalNPU","FaultEvent":[]},` +
		`"CustomFault":{"DevType":"Ascend910","FaultLevel":"NotHandle","FaultEvent":[]}}`
)

func TestApplyNPUHealthStatusMask(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		wantRaw     string
		wantKnown   bool
		wantMask    uint64
		wantFaulty  []int
	}{
		{
			name:        "all chips healthy",
			annotations: map[string]string{npuStatusAnnotation: realMaskAllHealthy},
			wantRaw:     realMaskAllHealthy,
			wantKnown:   true,
			wantMask:    0xffffffffffffff00,
			wantFaulty:  nil,
		},
		{
			name:        "chip 0 faulty",
			annotations: map[string]string{npuStatusAnnotation: realMaskChip0},
			wantRaw:     realMaskChip0,
			wantKnown:   true,
			wantMask:    0xffffffffffffff01,
			wantFaulty:  []int{0},
		},
		{
			name:        "chips 1 and 3 faulty",
			annotations: map[string]string{npuStatusAnnotation: "0xffffffffffffff0a"},
			wantRaw:     "0xffffffffffffff0a",
			wantKnown:   true,
			wantMask:    0xffffffffffffff0a,
			wantFaulty:  []int{1, 3},
		},
		{
			name:        "all ones means not reported yet",
			annotations: map[string]string{npuStatusAnnotation: realMaskNotReported},
			wantRaw:     realMaskNotReported,
			wantKnown:   false,
			wantMask:    0,
		},
		{
			name:        "all ones uppercase is also not reported",
			annotations: map[string]string{npuStatusAnnotation: "0xFFFFFFFFFFFFFFFF"},
			wantRaw:     "0xFFFFFFFFFFFFFFFF",
			wantKnown:   false,
			wantMask:    0,
		},
		{
			name:        "short form with 0x prefix",
			annotations: map[string]string{npuStatusAnnotation: "0x1"},
			wantRaw:     "0x1",
			wantKnown:   true,
			wantMask:    1,
			wantFaulty:  []int{0},
		},
		{
			name:        "decimal value without prefix",
			annotations: map[string]string{npuStatusAnnotation: "1"},
			wantRaw:     "1",
			wantKnown:   true,
			wantMask:    1,
			wantFaulty:  []int{0},
		},
		{
			name:        "garbage value leaves the mask unknown but keeps the raw text",
			annotations: map[string]string{npuStatusAnnotation: "not-a-mask"},
			wantRaw:     "not-a-mask",
			wantKnown:   false,
			wantMask:    0,
		},
		{
			name:        "bare hex without prefix is rejected",
			annotations: map[string]string{npuStatusAnnotation: "ffffffffffffff01"},
			wantRaw:     "ffffffffffffff01",
			wantKnown:   false,
			wantMask:    0,
		},
		{
			name:        "empty value is treated as absent",
			annotations: map[string]string{npuStatusAnnotation: "  "},
			wantRaw:     "",
			wantKnown:   false,
			wantMask:    0,
		},
		{
			name: "generic annotation wins over the ascend-1980 annotation",
			annotations: map[string]string{
				npuStatusAnnotation:     realMaskChip0,
				npuStatus1980Annotation: "0xffffffffffffff02",
			},
			wantRaw:    realMaskChip0,
			wantKnown:  true,
			wantMask:   0xffffffffffffff01,
			wantFaulty: []int{0},
		},
		{
			name:        "ascend-1980 annotation is used as fallback",
			annotations: map[string]string{npuStatus1980Annotation: "0xffffffffffffff02"},
			wantRaw:     "0xffffffffffffff02",
			wantKnown:   true,
			wantMask:    0xffffffffffffff02,
			wantFaulty:  []int{1},
		},
		{
			name:       "labels are used when annotations are missing",
			labels:     map[string]string{npuStatusAnnotation: realMaskChip0},
			wantRaw:    realMaskChip0,
			wantKnown:  true,
			wantMask:   0xffffffffffffff01,
			wantFaulty: []int{0},
		},
		{
			name: "annotations win over labels",
			annotations: map[string]string{
				npuStatus1980Annotation: "0xffffffffffffff02",
			},
			labels:     map[string]string{npuStatusAnnotation: realMaskChip0},
			wantRaw:    "0xffffffffffffff02",
			wantKnown:  true,
			wantMask:   0xffffffffffffff02,
			wantFaulty: []int{1},
		},
		{
			name:       "generic label wins over the ascend-1980 label",
			labels:     map[string]string{npuStatusAnnotation: realMaskChip0, npuStatus1980Annotation: "0xffffffffffffff02"},
			wantRaw:    realMaskChip0,
			wantKnown:  true,
			wantMask:   0xffffffffffffff01,
			wantFaulty: []int{0},
		},
		{
			name: "no status metadata at all",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := &model.NodeData{}
			applyNPUHealth(node, tc.annotations, tc.labels)

			if node.NPUStatusMaskRaw != tc.wantRaw {
				t.Errorf("NPUStatusMaskRaw = %q, want %q", node.NPUStatusMaskRaw, tc.wantRaw)
			}
			if node.NPUStatusKnown != tc.wantKnown {
				t.Errorf("NPUStatusKnown = %v, want %v", node.NPUStatusKnown, tc.wantKnown)
			}
			if node.NPUStatusMask != tc.wantMask {
				t.Errorf("NPUStatusMask = %#x, want %#x", node.NPUStatusMask, tc.wantMask)
			}

			var faulty []int
			for chip := 0; chip < model.NPUStatusChipBits; chip++ {
				if node.NPUChipFaulty(chip) {
					faulty = append(faulty, chip)
				}
			}
			if !reflect.DeepEqual(faulty, tc.wantFaulty) {
				t.Errorf("faulty chips = %v, want %v", faulty, tc.wantFaulty)
			}
		})
	}
}

// TestNPUChipFaultyIgnoresPaddingAndUnknownMask pins the two ways a chip can be
// wrongly reported faulty: reading the always-set padding bits above the low
// byte, and reading a mask that was never reported.
func TestNPUChipFaultyIgnoresPaddingAndUnknownMask(t *testing.T) {
	known := &model.NodeData{NPUStatusMask: 0xffffffffffffff01, NPUStatusKnown: true}
	for chip := model.NPUStatusChipBits; chip < 64; chip++ {
		if known.NPUChipFaulty(chip) {
			t.Errorf("chip %d reported faulty from a padding bit", chip)
		}
	}
	if known.NPUChipFaulty(-1) {
		t.Error("chip -1 reported faulty")
	}

	unknown := &model.NodeData{NPUStatusMask: 0xffffffffffffff01, NPUStatusKnown: false}
	if unknown.NPUChipFaulty(0) {
		t.Error("chip 0 reported faulty while the bitmask is unknown")
	}
}

func TestParseNPUFaultInfo(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        *model.NPUFaultInfo
	}{
		{
			name:        "real cluster sample",
			annotations: map[string]string{npuFaultInfoAnnotation: realFaultInfo},
			want: &model.NPUFaultInfo{
				DeviceFaultLevel: "NormalNPU",
				CustomFaultLevel: "NotHandle",
			},
		},
		{
			name: "urgent device fault with string events",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":{"DevType":"Ascend910",` +
				`"FaultLevel":"UrgentNPU","FaultEvent":["npu chip 0 is unhealthy","aicore reset"]}}`},
			want: &model.NPUFaultInfo{
				DeviceFaultLevel:  "UrgentNPU",
				DeviceFaultEvents: []string{"npu chip 0 is unhealthy", "aicore reset"},
			},
		},
		{
			name: "event elements may be objects",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":{"FaultLevel":"ImportantNPU",` +
				`"FaultEvent":[{"subType":"npu-ecc","detail":"count 12"}]}}`},
			want: &model.NPUFaultInfo{
				DeviceFaultLevel:  "ImportantNPU",
				DeviceFaultEvents: []string{`{"detail":"count 12","subType":"npu-ecc"}`},
			},
		},
		{
			name: "switch fault fields",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"SwitchFault":{"SwitchName":"switch-1",` +
				`"FaultLevel":"Warning","FaultEvent":null}}`},
			want: &model.NPUFaultInfo{
				SwitchFaultLevel: "Warning",
				SwitchFaultName:  "switch-1",
			},
		},
		{
			name:        "fields missing",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":{"FaultLevel":"UrgentNPU"}}`},
			want:        &model.NPUFaultInfo{DeviceFaultLevel: "UrgentNPU"},
		},
		{
			name:        "empty object",
			annotations: map[string]string{npuFaultInfoAnnotation: `{}`},
			want:        &model.NPUFaultInfo{},
		},
		{
			name: "numeric fault level degrades to an unknown level string",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":{"FaultLevel":3,` +
				`"FaultEvent":[7,true]}}`},
			want: &model.NPUFaultInfo{
				DeviceFaultLevel:  "3",
				DeviceFaultEvents: []string{"7", "true"},
			},
		},
		{
			name:        "FaultEvent as a bare string is a single event",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":{"FaultEvent":"chip 2 fault"}}`},
			want:        &model.NPUFaultInfo{DeviceFaultEvents: []string{"chip 2 fault"}},
		},
		{
			name:        "empty event strings are dropped",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":{"FaultEvent":["","ok"]}}`},
			want:        &model.NPUFaultInfo{DeviceFaultEvents: []string{"ok"}},
		},
		{
			name:        "invalid json",
			annotations: map[string]string{npuFaultInfoAnnotation: `{"DeviceFault":`},
			want:        nil,
		},
		{
			name:        "json array instead of object",
			annotations: map[string]string{npuFaultInfoAnnotation: `[1,2]`},
			want:        nil,
		},
		{
			name:        "empty string",
			annotations: map[string]string{npuFaultInfoAnnotation: "  "},
			want:        nil,
		},
		{
			name: "annotation absent",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseNPUFaultInfo(tc.annotations)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseNPUFaultInfo() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSanitizeNPUText(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		maxLen int
		want   string
	}{
		{name: "newlines collapsed", text: "line one\nline two\r\nline three", maxLen: 100, want: "line one line two line three"},
		{name: "short text unchanged", text: "ok", maxLen: 10, want: "ok"},
		{name: "truncated on rune boundary", text: "故障芯片数量超过阈值", maxLen: 3, want: "故障芯..."},
		{name: "empty", text: "", maxLen: 10, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeNPUText(tc.text, tc.maxLen); got != tc.want {
				t.Errorf("sanitizeNPUText(%q, %d) = %q, want %q", tc.text, tc.maxLen, got, tc.want)
			}
		})
	}
}

// TestConvertNodeAppliesNPUHealth verifies the wiring: a node object carrying
// the annotations observed on the cluster must end up with parsed health data.
func TestConvertNodeAppliesNPUHealth(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "npu-node",
			Annotations: map[string]string{
				npuStatusAnnotation:    realMaskChip0,
				npuFaultInfoAnnotation: realFaultInfo,
			},
			Labels: map[string]string{"huawei.com/ascend-1980-status": realMaskAllHealthy},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   "NT_NPU_DRIVER",
				Status: corev1.ConditionFalse,
				Reason: "os_task_name:npu-driver",
			}},
		},
	}

	nodeData := ConvertNode(node)

	if !nodeData.NPUStatusKnown {
		t.Fatal("NPUStatusKnown = false, want true")
	}
	if nodeData.NPUStatusMaskRaw != realMaskChip0 {
		t.Errorf("NPUStatusMaskRaw = %q, want %q", nodeData.NPUStatusMaskRaw, realMaskChip0)
	}
	if !nodeData.NPUChipFaulty(0) {
		t.Error("chip 0 should be faulty")
	}
	if nodeData.NPUChipFaulty(1) {
		t.Error("chip 1 should be healthy")
	}
	if nodeData.NPUFault == nil {
		t.Fatal("NPUFault = nil, want parsed fault info")
	}
	if nodeData.NPUFault.DeviceFaultLevel != "NormalNPU" {
		t.Errorf("DeviceFaultLevel = %q, want NormalNPU", nodeData.NPUFault.DeviceFaultLevel)
	}
}

// npuTestNodes builds nodes whose only NPU-relevant input is the given data.
func npuTestNodes(nodes ...*model.NodeData) []*model.NodeData {
	for _, node := range nodes {
		if node.Status == "" {
			node.Status = "Ready"
		}
	}
	return nodes
}

func npuAlertsOfType(alerts []model.Alert, alertType model.AlertType) []model.Alert {
	matched := make([]model.Alert, 0)
	for _, alert := range alerts {
		if alert.AlertType == alertType {
			matched = append(matched, alert)
		}
	}
	return matched
}

func countNPUAlerts(alerts []model.Alert) int {
	count := 0
	for _, alert := range alerts {
		if alert.Category == "NPU" {
			count++
		}
	}
	return count
}

func TestCollectAlertsNPUChipFault(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name        string
		mask        uint64
		known       bool
		wantValue   string
		wantMessage []string
		wantAlerts  int
	}{
		{
			name:       "no faulty chip",
			mask:       0xffffffffffffff00,
			known:      true,
			wantAlerts: 0,
		},
		{
			name:        "chip 0 faulty",
			mask:        0xffffffffffffff01,
			known:       true,
			wantValue:   "chips 0",
			wantMessage: []string{"chip(s) 0", "device plugin"},
			wantAlerts:  1,
		},
		{
			name:        "chips 0 and 7 faulty",
			mask:        0xffffffffffffff81,
			known:       true,
			wantValue:   "chips 0, 7",
			wantMessage: []string{"0, 7"},
			wantAlerts:  1,
		},
		{
			name:       "mask not reported yet",
			mask:       0xffffffffffffffff,
			known:      false,
			wantAlerts: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes := npuTestNodes(&model.NodeData{
				Name:           "npu-node",
				NPUStatusMask:  tc.mask,
				NPUStatusKnown: tc.known,
			})
			alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUChipFault)

			if len(alerts) != tc.wantAlerts {
				t.Fatalf("got %d chip fault alerts, want %d: %+v", len(alerts), tc.wantAlerts, alerts)
			}
			if tc.wantAlerts == 0 {
				return
			}

			alert := alerts[0]
			if alert.Severity != model.AlertSeverityCritical {
				t.Errorf("severity = %s, want Critical", alert.Severity)
			}
			if alert.Category != "NPU" {
				t.Errorf("category = %q, want NPU", alert.Category)
			}
			if alert.ResourceType != "Node" || alert.ResourceName != "npu-node" {
				t.Errorf("resource = %s/%s, want Node/npu-node", alert.ResourceType, alert.ResourceName)
			}
			if alert.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", alert.Value, tc.wantValue)
			}
			for _, fragment := range tc.wantMessage {
				if !strings.Contains(alert.Message, fragment) {
					t.Errorf("message %q does not contain %q", alert.Message, fragment)
				}
			}
			if alert.RecommendedAction == "" {
				t.Error("recommended action is empty")
			}
			if alert.Timestamp.IsZero() {
				t.Error("timestamp is zero")
			}
		})
	}
}

func TestCollectAlertsNPUFaultLevel(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name          string
		fault         *model.NPUFaultInfo
		wantSeverity  model.AlertSeverity
		wantReported  bool
		wantInMessage []string
	}{
		{
			name:         "normal device fault is silent",
			fault:        &model.NPUFaultInfo{DeviceFaultLevel: "NormalNPU", CustomFaultLevel: "NotHandle"},
			wantReported: false,
		},
		{
			name:         "missing device fault level is silent",
			fault:        &model.NPUFaultInfo{},
			wantReported: false,
		},
		{
			name:          "urgent is critical",
			fault:         &model.NPUFaultInfo{DeviceFaultLevel: "UrgentNPU"},
			wantSeverity:  model.AlertSeverityCritical,
			wantReported:  true,
			wantInMessage: []string{"UrgentNPU"},
		},
		{
			name:          "important is a warning",
			fault:         &model.NPUFaultInfo{DeviceFaultLevel: "ImportantNPU"},
			wantSeverity:  model.AlertSeverityWarning,
			wantReported:  true,
			wantInMessage: []string{"ImportantNPU"},
		},
		{
			name:          "warning level is a warning",
			fault:         &model.NPUFaultInfo{DeviceFaultLevel: "WarningNPU"},
			wantSeverity:  model.AlertSeverityWarning,
			wantReported:  true,
			wantInMessage: []string{"WarningNPU"},
		},
		{
			name:          "not handled is a warning",
			fault:         &model.NPUFaultInfo{DeviceFaultLevel: "NotHandle"},
			wantSeverity:  model.AlertSeverityWarning,
			wantReported:  true,
			wantInMessage: []string{"NotHandle"},
		},
		{
			name:          "unknown level is reported rather than swallowed",
			fault:         &model.NPUFaultInfo{DeviceFaultLevel: "SomeFutureLevel"},
			wantSeverity:  model.AlertSeverityWarning,
			wantReported:  true,
			wantInMessage: []string{"SomeFutureLevel"},
		},
		{
			name: "events are appended to the message",
			fault: &model.NPUFaultInfo{
				DeviceFaultLevel:  "UrgentNPU",
				DeviceFaultEvents: []string{"chip 0 unhealthy", "aicore reset"},
			},
			wantSeverity:  model.AlertSeverityCritical,
			wantReported:  true,
			wantInMessage: []string{"chip 0 unhealthy", "aicore reset"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUFault: tc.fault})
			alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUFaultLevel)

			if !tc.wantReported {
				if len(alerts) != 0 {
					t.Fatalf("got %d fault level alerts, want none: %+v", len(alerts), alerts)
				}
				return
			}
			if len(alerts) != 1 {
				t.Fatalf("got %d fault level alerts, want 1: %+v", len(alerts), alerts)
			}

			alert := alerts[0]
			if alert.Severity != tc.wantSeverity {
				t.Errorf("severity = %s, want %s", alert.Severity, tc.wantSeverity)
			}
			if alert.Category != "NPU" {
				t.Errorf("category = %q, want NPU", alert.Category)
			}
			if alert.Value != tc.fault.DeviceFaultLevel {
				t.Errorf("value = %q, want %q", alert.Value, tc.fault.DeviceFaultLevel)
			}
			for _, fragment := range tc.wantInMessage {
				if !strings.Contains(alert.Message, fragment) {
					t.Errorf("message %q does not contain %q", alert.Message, fragment)
				}
			}
			if alert.RecommendedAction == "" {
				t.Error("recommended action is empty")
			}
		})
	}
}

func TestCollectAlertsNPUSwitchFault(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name          string
		fault         *model.NPUFaultInfo
		wantReported  bool
		wantInMessage []string
	}{
		{
			name:         "no switch fault",
			fault:        &model.NPUFaultInfo{DeviceFaultLevel: "NormalNPU"},
			wantReported: false,
		},
		{
			name:         "normal switch level is silent",
			fault:        &model.NPUFaultInfo{SwitchFaultLevel: "Normal"},
			wantReported: false,
		},
		{
			name:          "switch warning is reported",
			fault:         &model.NPUFaultInfo{SwitchFaultLevel: "Warning", SwitchFaultName: "switch-1"},
			wantReported:  true,
			wantInMessage: []string{"Warning", "switch-1"},
		},
		{
			name:          "switch fault without a name",
			fault:         &model.NPUFaultInfo{SwitchFaultLevel: "Urgent"},
			wantReported:  true,
			wantInMessage: []string{"Urgent"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUFault: tc.fault})
			alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUSwitchFault)

			if !tc.wantReported {
				if len(alerts) != 0 {
					t.Fatalf("got %d switch fault alerts, want none: %+v", len(alerts), alerts)
				}
				return
			}
			if len(alerts) != 1 {
				t.Fatalf("got %d switch fault alerts, want 1: %+v", len(alerts), alerts)
			}

			alert := alerts[0]
			if alert.Severity != model.AlertSeverityWarning {
				t.Errorf("severity = %s, want Warning", alert.Severity)
			}
			if alert.Category != "NPU" {
				t.Errorf("category = %q, want NPU", alert.Category)
			}
			for _, fragment := range tc.wantInMessage {
				if !strings.Contains(alert.Message, fragment) {
					t.Errorf("message %q does not contain %q", alert.Message, fragment)
				}
			}
		})
	}
}

func TestCollectAlertsNPUCheckFailed(t *testing.T) {
	agg := &AggregatedDataSource{}

	// Condition shapes taken from the cluster: False covers "passed" and
	// "skipped", True is a detected fault, Unknown is not applicable.
	conditions := []corev1.NodeCondition{
		{Type: "NT_NPU_DRIVER", Status: corev1.ConditionFalse, Message: "ok"},
		{Type: "NT_NPU_ECC_COUNT", Status: corev1.ConditionUnknown, Message: "skip"},
		{Type: "NT_NPU_CARD_LOSE", Status: corev1.ConditionTrue, Message: "{\"subType\": \"npu-card-lost\"}"},
		{Type: "NT_NPU_OTHER", Status: corev1.ConditionTrue, Message: "first line\nsecond line"},
		{Type: "Ready", Status: corev1.ConditionTrue, Message: "kubelet ready"},
	}

	nodes := npuTestNodes(&model.NodeData{Name: "npu-node", Conditions: conditions})
	alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUCheckFailed)

	if len(alerts) != 2 {
		t.Fatalf("got %d check failed alerts, want 2: %+v", len(alerts), alerts)
	}

	byValue := make(map[string]model.Alert, len(alerts))
	for _, alert := range alerts {
		byValue[alert.Value] = alert
		if alert.Severity != model.AlertSeverityWarning {
			t.Errorf("%s: severity = %s, want Warning", alert.Value, alert.Severity)
		}
		if alert.Category != "NPU" {
			t.Errorf("%s: category = %q, want NPU", alert.Value, alert.Category)
		}
		if alert.ResourceName != "npu-node" {
			t.Errorf("%s: resource = %q, want npu-node", alert.Value, alert.ResourceName)
		}
		if !strings.Contains(alert.Message, alert.Value) {
			t.Errorf("%s: message %q does not name the condition", alert.Value, alert.Message)
		}
		if strings.Contains(alert.Message, "\n") {
			t.Errorf("%s: message %q still contains a newline", alert.Value, alert.Message)
		}
	}

	cardLose, ok := byValue["NT_NPU_CARD_LOSE"]
	if !ok {
		t.Fatalf("missing alert for NT_NPU_CARD_LOSE, got %+v", byValue)
	}
	if !strings.Contains(cardLose.Message, "npu-card-lost") {
		t.Errorf("NT_NPU_CARD_LOSE message %q does not carry the condition message", cardLose.Message)
	}

	other, ok := byValue["NT_NPU_OTHER"]
	if !ok {
		t.Fatalf("missing alert for NT_NPU_OTHER, got %+v", byValue)
	}
	if !strings.Contains(other.Message, "first line second line") {
		t.Errorf("NT_NPU_OTHER message %q did not collapse the newline", other.Message)
	}
}

func TestCollectAlertsNPUCheckMessageTruncated(t *testing.T) {
	agg := &AggregatedDataSource{}

	long := strings.Repeat("x", 1000)
	nodes := npuTestNodes(&model.NodeData{
		Name: "npu-node",
		Conditions: []corev1.NodeCondition{
			{Type: "NT_NPU_NET", Status: corev1.ConditionTrue, Message: long},
		},
	})

	alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUCheckFailed)
	if len(alerts) != 1 {
		t.Fatalf("got %d check failed alerts, want 1", len(alerts))
	}
	if len([]rune(alerts[0].Message)) > npuAlertMessageMaxLen+64 {
		t.Errorf("message was not truncated: %d runes", len([]rune(alerts[0].Message)))
	}
	if !strings.Contains(alerts[0].Message, "...") {
		t.Error("truncated message is not marked with an ellipsis")
	}
}

// TestCollectAlertsNPUCheckPrefixNarrowing pins the NT_NPU_ prefix rule: the
// non-NPU NT_* conditions observed on a healthy node must not alert, even when
// they are True, while a real NPU hardware check that is True must alert.
func TestCollectAlertsNPUCheckPrefixNarrowing(t *testing.T) {
	agg := &AggregatedDataSource{}

	nodes := npuTestNodes(&model.NodeData{
		Name: "npu-node",
		Conditions: []corev1.NodeCondition{
			// Permanently True on the live healthy node; informational text.
			{Type: "NT_NODE_YUM_MANAGER_LAUNCHER", Status: corev1.ConditionTrue,
				Message: "scriptOfEcs: result-YumInstall: it is not euler os,return."},
			// Data-pipeline conditions (MsgDumpToLabel / MsgToAnnotation).
			{Type: "NT_CABINET_DATA", Status: corev1.ConditionTrue, Message: "MsgDumpToLabel-cce.kubectl.kubernetes.io/cabinet"},
			{Type: "NT_ASCEND_RANK_TABLE_DATA", Status: corev1.ConditionTrue, Message: "MsgToAnnotation-cce.kubectl.kubernetes.io/ascend-rank-table"},
			// Non-NPU inspection families that exist on Ascend nodes.
			{Type: "NT_DISK_SLOW_IO_CHECK", Status: corev1.ConditionTrue, Message: "all disk io is normal"},
			{Type: "NT_GPU_CARD_LOSE", Status: corev1.ConditionTrue, Message: "skip"},
			// The real NPU hardware check under test.
			{Type: "NT_NPU_CARD_LOSE", Status: corev1.ConditionTrue, Message: "{\"subType\": \"npu-card-lost\", \"detail\": \"\"}"},
		},
	})

	alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUCheckFailed)

	if len(alerts) != 1 {
		t.Fatalf("got %d check failed alerts, want only NT_NPU_CARD_LOSE: %+v", len(alerts), alerts)
	}
	if alerts[0].Value != "NT_NPU_CARD_LOSE" {
		t.Errorf("alert value = %q, want NT_NPU_CARD_LOSE", alerts[0].Value)
	}
	if !strings.Contains(alerts[0].Message, "npu-card-lost") {
		t.Errorf("message %q does not carry the condition message", alerts[0].Message)
	}
}

// TestCollectAlertsRealClusterNodeHealthy feeds the values observed on the live
// node into the alert collector: a healthy NPU node must produce no NPU alerts,
// including the NT_NODE_YUM_MANAGER_LAUNCHER condition that is True there.
func TestCollectAlertsRealClusterNodeHealthy(t *testing.T) {
	agg := &AggregatedDataSource{}

	nodeData := &model.NodeData{
		Name:   "192.168.2.188",
		Status: "Ready",
		Annotations: map[string]string{
			npuStatusAnnotation:    realMaskAllHealthy,
			npuFaultInfoAnnotation: realFaultInfo,
		},
		Conditions: []corev1.NodeCondition{
			{Type: "NT_NPU_DRIVER", Status: corev1.ConditionFalse, Message: "ok"},
			{Type: "NT_NPU_ECC_COUNT", Status: corev1.ConditionFalse, Message: "ok"},
			{Type: "NT_NPU_CARD_LOSE", Status: corev1.ConditionFalse, Message: "{\"subType\": \"npu-card-lost\", \"detail\": \"\"}"},
			{Type: "NT_NPU_NET", Status: corev1.ConditionFalse, Message: "{\"subType\": \"npu-check-net\", \"detail\": \"\"}"},
			{Type: "NT_NPU_NET_FLICKER_CHECK", Status: corev1.ConditionFalse, Message: "ok"},
			{Type: "NT_NPU_CHECK_RESIDUAL_PROCESS", Status: corev1.ConditionFalse, Message: "{\"subType\": \"npu-check-residual-process-nothandle\", \"detail\": \"[]\"}"},
			{Type: "NT_NPU_OPTICAL_MEDIA_SNR_CHECK", Status: corev1.ConditionFalse, Message: "[2026-10-21 10:15:30] [] not 910c machine, skip optical media snr check."},
			{Type: "NT_GPU_CARD_LOSE", Status: corev1.ConditionUnknown, Message: "skip"},
			{Type: "NT_NPU_CHECK_HIGH_FREQUENCY_FAULT", Status: corev1.ConditionUnknown, Message: "{\"subType\": \"npu-check-high-frequency-fault\", \"detail\": \"\"}"},
			// Observed True on the live healthy node.
			{Type: "NT_NODE_YUM_MANAGER_LAUNCHER", Status: corev1.ConditionTrue,
				Message: "scriptOfEcs: result-YumInstall: it is not euler os,return."},
		},
	}
	applyNPUHealth(nodeData, nodeData.Annotations, nil)

	if !nodeData.NPUStatusKnown {
		t.Fatal("NPUStatusKnown = false, want true")
	}

	alerts := agg.collectAlerts(npuTestNodes(nodeData), nil, nil, nil, nil)
	if got := countNPUAlerts(alerts); got != 0 {
		t.Fatalf("got %d NPU alerts on a healthy node, want 0: %+v", got, alerts)
	}
}

// TestCollectAlertsNPUCombined pins that the independent NPU signals each
// produce their own alert and do not shadow one another.
func TestCollectAlertsNPUCombined(t *testing.T) {
	agg := &AggregatedDataSource{}

	nodes := npuTestNodes(&model.NodeData{
		Name:             "npu-node",
		NPUStatusMask:    0xffffffffffffff01,
		NPUStatusKnown:   true,
		NPUStatusMaskRaw: realMaskChip0,
		NPUFault: &model.NPUFaultInfo{
			DeviceFaultLevel: "UrgentNPU",
			SwitchFaultLevel: "Warning",
		},
		Conditions: []corev1.NodeCondition{
			{Type: "NT_NPU_NET", Status: corev1.ConditionTrue, Message: "link down"},
		},
	})

	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)

	for _, alertType := range []model.AlertType{
		model.AlertTypeNPUChipFault,
		model.AlertTypeNPUFaultLevel,
		model.AlertTypeNPUSwitchFault,
		model.AlertTypeNPUCheckFailed,
	} {
		if got := len(npuAlertsOfType(alerts, alertType)); got != 1 {
			t.Errorf("%s: got %d alerts, want 1", alertType, got)
		}
	}
	if got := countNPUAlerts(alerts); got != 4 {
		t.Errorf("got %d NPU alerts, want 4: %+v", got, alerts)
	}
}

// npuThresholdAlertTypes is the per-chip threshold family added on top of the
// annotation-driven NPU alerts.
var npuThresholdAlertTypes = []model.AlertType{
	model.AlertTypeNPUTempHigh,
	model.AlertTypeNPUEccDouble,
	model.AlertTypeNPUEccSingle,
	model.AlertTypeNPULinkDown,
	model.AlertTypeNPUErrorCode,
}

// realClusterChips is the per-chip snapshot read from the live NPU-Exporter on
// the 910B1 node: eight idle chips (temps 46-49°C, links up, no ECC or error
// code reported).
func realClusterChips() []model.NPUChipData {
	temps := []int{47, 49, 47, 47, 47, 46, 48, 48}
	chips := make([]model.NPUChipData, len(temps))
	for i, temp := range temps {
		chips[i] = model.NPUChipData{
			NPUID:           i / chipsPerNPU,
			Chip:            i % chipsPerNPU,
			PhyID:           i,
			Temp:            temp,
			LinkStatus:      1,
			LinkStatusKnown: true,
		}
	}
	return chips
}

// npuChipsFromTemps builds chips 0..n-1 with the given temperatures and an
// otherwise healthy profile.
func npuChipsFromTemps(temps ...int) []model.NPUChipData {
	chips := make([]model.NPUChipData, len(temps))
	for i, temp := range temps {
		chips[i] = model.NPUChipData{PhyID: i, Temp: temp, LinkStatus: 1, LinkStatusKnown: true}
	}
	return chips
}

// npuAssertSingleAlert returns the only alert of the given type, failing the
// test when the node produced none or several.
func npuAssertSingleAlert(t *testing.T, alerts []model.Alert, alertType model.AlertType) model.Alert {
	t.Helper()
	matched := npuAlertsOfType(alerts, alertType)
	if len(matched) != 1 {
		t.Fatalf("got %d %s alerts, want exactly 1: %+v", len(matched), alertType, alerts)
	}
	return matched[0]
}

// npuAssertThresholdAlertShape checks the fields every per-chip threshold alert
// must carry regardless of type.
func npuAssertThresholdAlertShape(t *testing.T, alert model.Alert, nodeName string) {
	t.Helper()
	if alert.Category != "NPU" {
		t.Errorf("category = %q, want NPU", alert.Category)
	}
	if alert.ResourceType != "Node" || alert.ResourceName != nodeName {
		t.Errorf("resource = %s/%s, want Node/%s", alert.ResourceType, alert.ResourceName, nodeName)
	}
	if alert.Message == "" {
		t.Error("message is empty")
	}
	if alert.RecommendedAction == "" {
		t.Error("recommended action is empty")
	}
	if alert.Timestamp.IsZero() {
		t.Error("timestamp is zero")
	}
}

func TestCollectAlertsNPUTempHigh(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name         string
		temps        []int
		wantAlerts   int
		wantSeverity model.AlertSeverity
		wantValue    string
		wantMessage  []string
	}{
		{
			name:       "real idle cluster temperatures are silent",
			temps:      []int{47, 49, 47, 47, 47, 46, 48, 48},
			wantAlerts: 0,
		},
		{
			name:       "79°C is below the warning threshold",
			temps:      []int{47, 79},
			wantAlerts: 0,
		},
		{
			name:         "80°C warns",
			temps:        []int{47, 80},
			wantAlerts:   1,
			wantSeverity: model.AlertSeverityWarning,
			wantValue:    "chips 1",
			wantMessage:  []string{"chip(s) 1", "80°C"},
		},
		{
			name:         "89°C is still a warning",
			temps:        []int{89},
			wantAlerts:   1,
			wantSeverity: model.AlertSeverityWarning,
			wantValue:    "chips 0",
			wantMessage:  []string{"89°C"},
		},
		{
			name:         "90°C is critical",
			temps:        []int{90},
			wantAlerts:   1,
			wantSeverity: model.AlertSeverityCritical,
			wantValue:    "chips 0",
			wantMessage:  []string{"90°C"},
		},
		{
			name:         "the hottest chip decides the severity and every hot chip is listed",
			temps:        []int{85, 95},
			wantAlerts:   1,
			wantSeverity: model.AlertSeverityCritical,
			wantValue:    "chips 0, 1",
			wantMessage:  []string{"chip(s) 0, 1", "95°C"},
		},
		{
			name:         "eight hot chips still raise a single alert",
			temps:        []int{81, 82, 83, 84, 85, 86, 87, 88},
			wantAlerts:   1,
			wantSeverity: model.AlertSeverityWarning,
			wantValue:    "chips 0, 1, 2, 3, 4, 5, 6, 7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: npuChipsFromTemps(tc.temps...)})
			alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUTempHigh)

			if len(alerts) != tc.wantAlerts {
				t.Fatalf("got %d temperature alerts, want %d: %+v", len(alerts), tc.wantAlerts, alerts)
			}
			if tc.wantAlerts == 0 {
				return
			}

			alert := alerts[0]
			npuAssertThresholdAlertShape(t, alert, "npu-node")
			if alert.Severity != tc.wantSeverity {
				t.Errorf("severity = %s, want %s", alert.Severity, tc.wantSeverity)
			}
			if alert.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", alert.Value, tc.wantValue)
			}
			// Threshold reports the line matching the severity: the warning
			// line while every offending chip is below the critical line, the
			// critical line once severity escalates.
			wantThreshold := "80°C"
			if tc.wantSeverity == model.AlertSeverityCritical {
				wantThreshold = "90°C"
			}
			if alert.Threshold != wantThreshold {
				t.Errorf("threshold = %q, want %q", alert.Threshold, wantThreshold)
			}
			for _, fragment := range tc.wantMessage {
				if !strings.Contains(alert.Message, fragment) {
					t.Errorf("message %q does not contain %q", alert.Message, fragment)
				}
			}
		})
	}
}

func TestCollectAlertsNPUEccErrors(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name          string
		single        []int64
		double        []int64
		wantDouble    int
		wantSingle    int
		wantValue     string
		wantSeverity  model.AlertSeverity
		wantInMessage []string
	}{
		{
			name:       "clean chips are silent",
			single:     []int64{0, 0},
			double:     []int64{0, 0},
			wantDouble: 0,
			wantSingle: 0,
		},
		{
			name:          "a single-bit error warns",
			single:        []int64{0, 0, 3},
			double:        []int64{0, 0, 0},
			wantDouble:    0,
			wantSingle:    1,
			wantValue:     "chips 2",
			wantSeverity:  model.AlertSeverityWarning,
			wantInMessage: []string{"single-bit", "correctable"},
		},
		{
			name:          "a double-bit error is critical",
			single:        []int64{0},
			double:        []int64{1},
			wantDouble:    1,
			wantSingle:    0,
			wantValue:     "chips 0",
			wantSeverity:  model.AlertSeverityCritical,
			wantInMessage: []string{"double-bit", "uncorrectable"},
		},
		{
			name:          "a double-bit error suppresses the single-bit alert on the same chip",
			single:        []int64{5},
			double:        []int64{2},
			wantDouble:    1,
			wantSingle:    0,
			wantValue:     "chips 0",
			wantSeverity:  model.AlertSeverityCritical,
			wantInMessage: []string{"chip(s) 0"},
		},
		{
			name:          "different chips raise both alerts",
			single:        []int64{0, 0, 0, 4},
			double:        []int64{1, 0, 0, 0},
			wantDouble:    1,
			wantSingle:    1,
			wantValue:     "chips 0",
			wantSeverity:  model.AlertSeverityCritical,
			wantInMessage: []string{"chip(s) 0"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chips := make([]model.NPUChipData, len(tc.single))
			for i := range tc.single {
				chips[i] = model.NPUChipData{
					PhyID:           i,
					Temp:            47,
					LinkStatus:      1,
					LinkStatusKnown: true,
					HBMEccSingleErr: tc.single[i],
					HBMEccDoubleErr: tc.double[i],
				}
			}
			nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: chips})
			alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)

			doubleAlerts := npuAlertsOfType(alerts, model.AlertTypeNPUEccDouble)
			singleAlerts := npuAlertsOfType(alerts, model.AlertTypeNPUEccSingle)
			if len(doubleAlerts) != tc.wantDouble {
				t.Fatalf("got %d double-bit alerts, want %d: %+v", len(doubleAlerts), tc.wantDouble, doubleAlerts)
			}
			if len(singleAlerts) != tc.wantSingle {
				t.Fatalf("got %d single-bit alerts, want %d: %+v", len(singleAlerts), tc.wantSingle, singleAlerts)
			}

			// Whatever fired must carry the shape and the expected severity; the
			// expected value and message are asserted on the double-bit alert when
			// present, otherwise on the single-bit one.
			alert := model.Alert{}
			switch {
			case len(doubleAlerts) == 1:
				alert = doubleAlerts[0]
			case len(singleAlerts) == 1:
				alert = singleAlerts[0]
			default:
				if got := countNPUAlerts(alerts); got != 0 {
					t.Fatalf("got %d NPU alerts on clean chips, want 0: %+v", got, alerts)
				}
				return
			}

			npuAssertThresholdAlertShape(t, alert, "npu-node")
			if alert.Severity != tc.wantSeverity {
				t.Errorf("severity = %s, want %s", alert.Severity, tc.wantSeverity)
			}
			if alert.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", alert.Value, tc.wantValue)
			}
			for _, fragment := range tc.wantInMessage {
				if !strings.Contains(alert.Message, fragment) {
					t.Errorf("message %q does not contain %q", alert.Message, fragment)
				}
			}

			// The single-bit alert on the "different chips" case must point at its
			// own chip rather than repeating the double-bit one.
			if tc.wantSingle == 1 && len(singleAlerts) == 1 && tc.wantDouble == 1 {
				if singleAlerts[0].Value != "chips 3" {
					t.Errorf("single-bit value = %q, want %q", singleAlerts[0].Value, "chips 3")
				}
			}
		})
	}
}

// TestCollectAlertsNPUEccCounterChoice pins the ECC counter choice behind the
// threshold alerts: alerts read the per-window npu_chip_info_hbm_ecc_*_cnt
// family, never the cumulative npu_chip_info_hbm_ecc_total_*_bit_error_cnt
// family. Shape taken from the reference node (verified live 2026-10-21):
// chip 2 reports a six-digit cumulative single-bit total with a quiet
// per-window counter, and must stay silent.
func TestCollectAlertsNPUEccCounterChoice(t *testing.T) {
	agg := &AggregatedDataSource{}

	chips := []model.NPUChipData{
		{PhyID: 0, Temp: 47, LinkStatus: 1, LinkStatusKnown: true, HBMEccSingleErrTotal: 0},
		{PhyID: 2, Temp: 47, LinkStatus: 1, LinkStatusKnown: true, HBMEccSingleErrTotal: 99992},
	}
	nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: chips})
	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)

	if got := npuAlertsOfType(alerts, model.AlertTypeNPUEccSingle); len(got) != 0 {
		t.Fatalf("cumulative single-bit total raised an alert: %+v", got)
	}
	if got := npuAlertsOfType(alerts, model.AlertTypeNPUEccDouble); len(got) != 0 {
		t.Fatalf("cumulative total raised a double-bit alert: %+v", got)
	}

	// The same chip reporting through the per-window counter does alert.
	chips[1].HBMEccSingleErr = 2
	nodes = npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: chips})
	alerts = agg.collectAlerts(nodes, nil, nil, nil, nil)
	got := npuAlertsOfType(alerts, model.AlertTypeNPUEccSingle)
	if len(got) != 1 || got[0].Value != "chips 2" {
		t.Fatalf("per-window counter did not warn on chips 2: %+v", got)
	}
}

func TestCollectAlertsNPULinkDown(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name       string
		chips      []model.NPUChipData
		wantAlerts int
		wantValue  string
	}{
		{
			name:       "all links up are silent",
			chips:      realClusterChips(),
			wantAlerts: 0,
		},
		{
			name: "an unreported link status is not a link down",
			chips: []model.NPUChipData{
				{PhyID: 0, Temp: 47},
				{PhyID: 1, Temp: 47},
			},
			wantAlerts: 0,
		},
		{
			name: "a single link down is critical",
			chips: []model.NPUChipData{
				{PhyID: 0, Temp: 47, LinkStatus: 1, LinkStatusKnown: true},
				{PhyID: 1, Temp: 47, LinkStatus: 0, LinkStatusKnown: true},
			},
			wantAlerts: 1,
			wantValue:  "chips 1",
		},
		{
			name: "several links down aggregate into one alert",
			chips: []model.NPUChipData{
				{PhyID: 0, Temp: 47, LinkStatus: 0, LinkStatusKnown: true},
				{PhyID: 1, Temp: 47, LinkStatus: 1, LinkStatusKnown: true},
				{PhyID: 2, Temp: 47, LinkStatus: 0, LinkStatusKnown: true},
			},
			wantAlerts: 1,
			wantValue:  "chips 0, 2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: tc.chips})
			alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPULinkDown)

			if len(alerts) != tc.wantAlerts {
				t.Fatalf("got %d link alerts, want %d: %+v", len(alerts), tc.wantAlerts, alerts)
			}
			if tc.wantAlerts == 0 {
				return
			}

			alert := alerts[0]
			npuAssertThresholdAlertShape(t, alert, "npu-node")
			if alert.Severity != model.AlertSeverityCritical {
				t.Errorf("severity = %s, want Critical", alert.Severity)
			}
			if alert.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", alert.Value, tc.wantValue)
			}
		})
	}
}

func TestCollectAlertsNPUErrorCode(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name          string
		codes         []int
		wantAlerts    int
		wantValue     string
		wantInMessage []string
	}{
		{
			name:       "zero error code is silent",
			codes:      []int{0, 0, 0},
			wantAlerts: 0,
		},
		{
			name:          "a non-zero error code warns and keeps the raw code",
			codes:         []int{0, 0, 0, 0, 0, 7},
			wantAlerts:    1,
			wantValue:     "chips 5",
			wantInMessage: []string{"chip(s) 5 (code 7)"},
		},
		{
			name:          "several error codes aggregate into one alert",
			codes:         []int{3, 0, 1},
			wantAlerts:    1,
			wantValue:     "chips 0, 2",
			wantInMessage: []string{"0 (code 3)", "2 (code 1)"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chips := make([]model.NPUChipData, len(tc.codes))
			for i, code := range tc.codes {
				chips[i] = model.NPUChipData{PhyID: i, Temp: 47, LinkStatus: 1, LinkStatusKnown: true, ErrorCode: code}
			}
			nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: chips})
			alerts := npuAlertsOfType(agg.collectAlerts(nodes, nil, nil, nil, nil), model.AlertTypeNPUErrorCode)

			if len(alerts) != tc.wantAlerts {
				t.Fatalf("got %d error code alerts, want %d: %+v", len(alerts), tc.wantAlerts, alerts)
			}
			if tc.wantAlerts == 0 {
				return
			}

			alert := alerts[0]
			npuAssertThresholdAlertShape(t, alert, "npu-node")
			if alert.Severity != model.AlertSeverityWarning {
				t.Errorf("severity = %s, want Warning", alert.Severity)
			}
			if alert.Value != tc.wantValue {
				t.Errorf("value = %q, want %q", alert.Value, tc.wantValue)
			}
			for _, fragment := range tc.wantInMessage {
				if !strings.Contains(alert.Message, fragment) {
					t.Errorf("message %q does not contain %q", alert.Message, fragment)
				}
			}
		})
	}
}

// TestCollectAlertsNPUThresholdsRequireChips pins that the whole threshold
// family is gated on per-chip metrics: with the NPU-Exporter unavailable the
// node still carries the annotation-driven signals but must not produce chip
// threshold alerts out of zero values.
func TestCollectAlertsNPUThresholdsRequireChips(t *testing.T) {
	agg := &AggregatedDataSource{}

	chips := []model.NPUChipData{
		{PhyID: 0, Temp: 95, LinkStatus: 0, LinkStatusKnown: true, HBMEccDoubleErr: 1, ErrorCode: 5},
		{PhyID: 1, Temp: 95, LinkStatus: 0, LinkStatusKnown: true, HBMEccSingleErr: 2, ErrorCode: 5},
	}

	withChips := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: chips})
	withoutChips := npuTestNodes(&model.NodeData{Name: "npu-node"})

	for _, alertType := range npuThresholdAlertTypes {
		if got := len(npuAlertsOfType(agg.collectAlerts(withoutChips, nil, nil, nil, nil), alertType)); got != 0 {
			t.Errorf("%s: got %d alerts without chip metrics, want 0", alertType, got)
		}
		if got := len(npuAlertsOfType(agg.collectAlerts(withChips, nil, nil, nil, nil), alertType)); got != 1 {
			t.Errorf("%s: got %d alerts with chip metrics, want 1", alertType, got)
		}
	}
}

// TestCollectAlertsNPUThresholdsOnePerNode pins that every threshold type is
// emitted at most once per node even when all eight chips are in violation, so
// a fully degraded node produces five alerts rather than forty.
func TestCollectAlertsNPUThresholdsOnePerNode(t *testing.T) {
	agg := &AggregatedDataSource{}

	chips := make([]model.NPUChipData, 8)
	for i := range chips {
		chips[i] = model.NPUChipData{
			PhyID:           i,
			Temp:            95,
			LinkStatus:      0,
			LinkStatusKnown: true,
			ErrorCode:       9,
		}
		if i < 4 {
			chips[i].HBMEccDoubleErr = 1
		} else {
			// Single-bit only: a double-bit error on the same chip would
			// suppress the single-bit alert by design.
			chips[i].HBMEccSingleErr = 1
		}
	}
	nodes := npuTestNodes(&model.NodeData{Name: "npu-node", NPUChips: chips})

	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)

	for _, alertType := range npuThresholdAlertTypes {
		if got := len(npuAlertsOfType(alerts, alertType)); got != 1 {
			t.Errorf("%s: got %d alerts, want exactly 1", alertType, got)
		}
	}
	if got := countNPUAlerts(alerts); got != len(npuThresholdAlertTypes) {
		t.Errorf("got %d NPU alerts, want %d: %+v", got, len(npuThresholdAlertTypes), alerts)
	}
}

// TestCollectAlertsNPUThresholdsDoNotShadowAnnotationAlerts pins that the two
// NPU alert families are independent: a node that trips both produces all nine
// alert types.
func TestCollectAlertsNPUThresholdsDoNotShadowAnnotationAlerts(t *testing.T) {
	agg := &AggregatedDataSource{}

	chips := []model.NPUChipData{
		{PhyID: 0, Temp: 95, LinkStatus: 0, LinkStatusKnown: true, HBMEccDoubleErr: 1, ErrorCode: 9},
		{PhyID: 1, Temp: 95, LinkStatus: 0, LinkStatusKnown: true, HBMEccSingleErr: 1, ErrorCode: 9},
	}

	nodes := npuTestNodes(&model.NodeData{
		Name:           "npu-node",
		NPUStatusMask:  0xffffffffffffff01,
		NPUStatusKnown: true,
		NPUFault: &model.NPUFaultInfo{
			DeviceFaultLevel: "UrgentNPU",
			SwitchFaultLevel: "Warning",
		},
		Conditions: []corev1.NodeCondition{
			{Type: "NT_NPU_NET", Status: corev1.ConditionTrue, Message: "link down"},
		},
		NPUChips: chips,
	})

	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)

	for _, alertType := range append([]model.AlertType{
		model.AlertTypeNPUChipFault,
		model.AlertTypeNPUFaultLevel,
		model.AlertTypeNPUSwitchFault,
		model.AlertTypeNPUCheckFailed,
	}, npuThresholdAlertTypes...) {
		if got := len(npuAlertsOfType(alerts, alertType)); got != 1 {
			t.Errorf("%s: got %d alerts, want 1", alertType, got)
		}
	}
	if got := countNPUAlerts(alerts); got != 9 {
		t.Errorf("got %d NPU alerts, want 9: %+v", got, alerts)
	}
}

// TestCollectAlertsNPUThresholdsRealClusterHealthy is the live-data harness
// check: the real per-chip snapshot of the healthy 910B1 node must produce no
// threshold alerts.
func TestCollectAlertsNPUThresholdsRealClusterHealthy(t *testing.T) {
	agg := &AggregatedDataSource{}

	nodes := npuTestNodes(&model.NodeData{
		Name:     "192.168.2.188",
		NPUChips: realClusterChips(),
	})

	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)
	if got := countNPUAlerts(alerts); got != 0 {
		t.Fatalf("got %d NPU alerts on a healthy node, want 0: %+v", got, alerts)
	}
}

// TestCollectAlertsNPUMonitorDegraded covers the degraded-monitoring alert
// that fills the silent window while the NPU-Exporter client is in retry
// backoff: no client or a never-attempted client stays quiet, an armed
// backoff yields exactly one Info alert anchored to the NPU node, and a
// recorded success clears it again.
func TestCollectAlertsNPUMonitorDegraded(t *testing.T) {
	newNodes := func() []*model.NodeData {
		return npuTestNodes(
			&model.NodeData{Name: "192.168.2.188", NPUCapacity: 8},
			&model.NodeData{Name: "cpu-node"},
		)
	}

	t.Run("no client attached", func(t *testing.T) {
		agg := &AggregatedDataSource{}
		alerts := agg.collectAlerts(newNodes(), nil, nil, nil, nil)
		if got := len(npuAlertsOfType(alerts, model.AlertTypeNPUMonitorDegraded)); got != 0 {
			t.Errorf("got %d degraded alerts without a client, want 0", got)
		}
	})

	t.Run("never attempted is not degraded", func(t *testing.T) {
		agg := &AggregatedDataSource{}
		agg.SetNPUExporterClient(&NPUExporterClient{})
		alerts := agg.collectAlerts(newNodes(), nil, nil, nil, nil)
		if got := len(npuAlertsOfType(alerts, model.AlertTypeNPUMonitorDegraded)); got != 0 {
			t.Errorf("got %d degraded alerts before the first attempt, want 0", got)
		}
	})

	t.Run("backoff arms one Info alert on the NPU node", func(t *testing.T) {
		client := &NPUExporterClient{}
		agg := &AggregatedDataSource{}
		agg.SetNPUExporterClient(client)
		client.recordFailure()

		alerts := agg.collectAlerts(newNodes(), nil, nil, nil, nil)
		matched := npuAlertsOfType(alerts, model.AlertTypeNPUMonitorDegraded)
		if len(matched) != 1 {
			t.Fatalf("got %d degraded alerts during backoff, want 1: %+v", len(matched), alerts)
		}
		alert := matched[0]
		if alert.Severity != model.AlertSeverityInfo {
			t.Errorf("severity = %v, want Info", alert.Severity)
		}
		if alert.ResourceName != "192.168.2.188" {
			t.Errorf("ResourceName = %q, want the NPU node", alert.ResourceName)
		}
		if alert.Category != "NPU" || alert.ResourceType != "Node" {
			t.Errorf("Category/ResourceType = %q/%q, want NPU/Node", alert.Category, alert.ResourceType)
		}
		if alert.RecommendedAction == "" {
			t.Error("RecommendedAction is empty, want a hint for the exporter path")
		}
	})

	t.Run("success clears the alert", func(t *testing.T) {
		client := &NPUExporterClient{}
		agg := &AggregatedDataSource{}
		agg.SetNPUExporterClient(client)
		client.recordFailure()
		client.recordSuccess()

		alerts := agg.collectAlerts(newNodes(), nil, nil, nil, nil)
		if got := len(npuAlertsOfType(alerts, model.AlertTypeNPUMonitorDegraded)); got != 0 {
			t.Errorf("got %d degraded alerts after recovery, want 0", got)
		}
	})
}
