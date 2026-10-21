package datasource

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// Tests for alert message i18n: translatable alert sites must dual-write the
// English Message and a MessageKey + MessageArgs pair, while sites that quote
// cluster-provided text verbatim (NT inspection output, fault-info annotation
// text) keep an empty MessageKey so the alerts view falls back to Message.

func TestCollectAlertsMessageKeys(t *testing.T) {
	agg := &AggregatedDataSource{}

	tests := []struct {
		name        string
		alertType   model.AlertType
		nodes       []*model.NodeData
		pods        []*model.PodData
		wantKey     string
		wantArgs    map[string]interface{}
		wantMessage string
	}{
		{
			name:        "node not ready",
			alertType:   model.AlertTypeNodeNotReady,
			nodes:       []*model.NodeData{{Name: "node-1", Status: "NotReady"}},
			wantKey:     "alert.node_not_ready",
			wantArgs:    map[string]interface{}{"Status": "NotReady"},
			wantMessage: fmt.Sprintf("Node is %s", "NotReady"),
		},
		{
			name:        "node cpu critical has no template args",
			alertType:   model.AlertTypeNodeCPUCritical,
			nodes:       []*model.NodeData{{Name: "node-1", Status: "Ready", CPUUsagePercent: 95.0}},
			wantKey:     "alert.node_cpu_critical",
			wantArgs:    nil,
			wantMessage: "Node CPU usage critical",
		},
		{
			name:      "pod oom killed",
			alertType: model.AlertTypePodOOMKilled,
			pods: []*model.PodData{{
				Name: "app", Namespace: "default",
				ContainerStates: []model.ContainerState{{Name: "worker", Reason: "OOMKilled"}},
			}},
			wantKey:     "alert.pod_oom_killed",
			wantArgs:    map[string]interface{}{"Container": "worker"},
			wantMessage: fmt.Sprintf("Container %s was OOMKilled", "worker"),
		},
		{
			name:      "npu temperature high",
			alertType: model.AlertTypeNPUTempHigh,
			nodes: []*model.NodeData{{
				Name:     "npu-node",
				NPUChips: []model.NPUChipData{{PhyID: 3, Temp: 95}},
			}},
			wantKey:     "alert.npu_temp_high",
			wantArgs:    map[string]interface{}{"Chips": "3", "Max": 95},
			wantMessage: fmt.Sprintf("NPU temperature high on chip(s) %s (max %d°C)", "3", 95),
		},
		{
			name:      "npu link down",
			alertType: model.AlertTypeNPULinkDown,
			nodes: []*model.NodeData{{
				Name:     "npu-node",
				NPUChips: []model.NPUChipData{{PhyID: 2, LinkStatusKnown: true, LinkStatus: 0}},
			}},
			wantKey:     "alert.npu_link_down",
			wantArgs:    map[string]interface{}{"Chips": "2"},
			wantMessage: fmt.Sprintf("NPU link down on chip(s) %s", "2"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			alerts := npuAlertsOfType(
				agg.collectAlerts(npuTestNodes(tc.nodes...), tc.pods, nil, nil, nil),
				tc.alertType,
			)
			if len(alerts) != 1 {
				t.Fatalf("got %d %s alerts, want 1: %+v", len(alerts), tc.alertType, alerts)
			}
			alert := alerts[0]

			if alert.MessageKey != tc.wantKey {
				t.Errorf("MessageKey = %q, want %q", alert.MessageKey, tc.wantKey)
			}
			if alert.Message != tc.wantMessage {
				t.Errorf("Message = %q, want %q", alert.Message, tc.wantMessage)
			}
			if tc.wantArgs == nil {
				if alert.MessageArgs != nil {
					t.Errorf("MessageArgs = %v, want nil", alert.MessageArgs)
				}
				return
			}
			if len(alert.MessageArgs) != len(tc.wantArgs) {
				t.Fatalf("MessageArgs = %v, want %v", alert.MessageArgs, tc.wantArgs)
			}
			for k, want := range tc.wantArgs {
				if got := alert.MessageArgs[k]; got != want {
					t.Errorf("MessageArgs[%q] = %v (%T), want %v (%T)", k, got, got, want, want)
				}
			}
		})
	}
}

func TestCollectAlertsRawMessagesHaveNoKey(t *testing.T) {
	agg := &AggregatedDataSource{}

	nodes := npuTestNodes(
		&model.NodeData{
			Name: "fault-node",
			NPUFault: &model.NPUFaultInfo{
				DeviceFaultLevel:  "UrgentNPU",
				DeviceFaultEvents: []string{"chip 0 unhealthy"},
			},
		},
		&model.NodeData{
			Name:     "switch-node",
			NPUFault: &model.NPUFaultInfo{SwitchFaultLevel: "Warning", SwitchFaultName: "switch-1"},
		},
		&model.NodeData{
			Name: "check-node",
			Conditions: []corev1.NodeCondition{
				{Type: "NT_NPU_ECC_COUNT", Status: corev1.ConditionTrue, Message: "ecc count exceed"},
			},
		},
	)
	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)

	rawTypes := map[model.AlertType]string{
		model.AlertTypeNPUFaultLevel:  "NPU device fault level UrgentNPU",
		model.AlertTypeNPUSwitchFault: "NPU switch fault level Warning",
		model.AlertTypeNPUCheckFailed: "NPU inspection check NT_NPU_ECC_COUNT failed",
	}
	for alertType, wantPrefix := range rawTypes {
		matched := npuAlertsOfType(alerts, alertType)
		if len(matched) != 1 {
			t.Fatalf("got %d %s alerts, want 1: %+v", len(matched), alertType, alerts)
		}
		if matched[0].MessageKey != "" {
			t.Errorf("%s: MessageKey = %q, want empty (raw cluster text)", alertType, matched[0].MessageKey)
		}
		if matched[0].MessageArgs != nil {
			t.Errorf("%s: MessageArgs = %v, want nil", alertType, matched[0].MessageArgs)
		}
		if !strings.HasPrefix(matched[0].Message, wantPrefix) {
			t.Errorf("%s: Message = %q, want prefix %q", alertType, matched[0].Message, wantPrefix)
		}
	}
}
