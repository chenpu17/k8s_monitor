package datasource

// Alert message localisation lockdown. The en bundle must render
// TF(MessageKey, MessageArgs) byte-identical to the stored Message for every
// translatable alert site, and raw sites (fault-info / NT_ inspection text
// quoted from the cluster) must keep an empty MessageKey. The existing
// site-sample tests cannot catch en-bundle drift: a mutated bundle value
// still passes them, only this TF==Message identity fails (batch-9 verify
// finding). Fixtures trigger ALL 26 alert sites in one collectAlerts pass
// and fail if any expected type is missing, so a silently untriggered site
// can never read as green.

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/chenpu/k8s-monitor/internal/model"
)

var alertMessageKeys = map[model.AlertType]string{
	model.AlertTypeNodeNotReady:        "alert.node_not_ready",
	model.AlertTypeNodeMemoryPressure:  "alert.node_memory_pressure",
	model.AlertTypeNodeDiskPressure:    "alert.node_disk_pressure",
	model.AlertTypeNodePIDPressure:     "alert.node_pid_pressure",
	model.AlertTypeNodeCPUCritical:     "alert.node_cpu_critical",
	model.AlertTypeNodeCPUHigh:         "alert.node_cpu_high",
	model.AlertTypeNodeMemoryCritical:  "alert.node_memory_critical",
	model.AlertTypeNodeMemoryHigh:      "alert.node_memory_high",
	model.AlertTypeNPUChipFault:        "alert.npu_chip_fault",
	model.AlertTypeNPUMonitorDegraded:  "alert.npu_monitor_degraded",
	model.AlertTypePodOOMKilled:        "alert.pod_oom_killed",
	model.AlertTypePodCrashLoopBackOff: "alert.pod_crash_loop",
	model.AlertTypePodImagePullBackOff: "alert.pod_image_pull",
	model.AlertTypePodHighRestarts:     "alert.pod_high_restarts",
	model.AlertTypePodPendingTooLong:   "alert.pod_pending_long",
	model.AlertTypePodFailed:           "alert.pod_failed",
	model.AlertTypeServiceNoEndpoints:  "alert.service_no_endpoints",
	model.AlertTypePVCPendingTooLong:   "alert.pvc_pending_long",
	model.AlertTypeNPUTempHigh:         "alert.npu_temp_high",
	model.AlertTypeNPUEccDouble:        "alert.npu_ecc_double_bit",
	model.AlertTypeNPUEccSingle:        "alert.npu_ecc_single_bit",
	model.AlertTypeNPULinkDown:         "alert.npu_link_down",
	model.AlertTypeNPUErrorCode:        "alert.npu_error_code",
}

var rawAlertMessageTypes = map[model.AlertType]bool{
	model.AlertTypeNPUFaultLevel:  true,
	model.AlertTypeNPUSwitchFault: true,
	model.AlertTypeNPUCheckFailed: true,
}

func allAlertSitesNodes() []*model.NodeData {
	nodes := []*model.NodeData{
		{Name: "n-notready", Status: "NotReady"},
		{Name: "n-press", Status: "Ready", MemoryPressure: true, DiskPressure: true, PIDPressure: true},
		{Name: "n-crit", Status: "Ready", CPUUsagePercent: 95.0, MemoryUsagePercent: 95.0},
		{Name: "n-high", Status: "Ready", CPUUsagePercent: 85.0, MemoryUsagePercent: 85.0},
		{Name: "n-npu", Status: "Ready", NPUStatusKnown: true, NPUStatusMask: 1 << 2},
		{Name: "n-thresh", Status: "Ready", NPUChips: []model.NPUChipData{
			{PhyID: 3, Temp: 95},
			{PhyID: 4, HBMEccDoubleErr: 1},
			{PhyID: 5, HBMEccSingleErr: 1},
			{PhyID: 6, LinkStatusKnown: true, LinkStatus: 0},
			{PhyID: 7, ErrorCode: 42},
		}},
		{Name: "n-degraded", Status: "Ready", NPUCapacity: 8},
		{Name: "n-devfault", Status: "Ready", NPUFault: &model.NPUFaultInfo{
			DeviceFaultLevel:  "UrgentNPU",
			DeviceFaultEvents: []string{"chip 0 unhealthy"},
		}},
		{Name: "n-switch", Status: "Ready", NPUFault: &model.NPUFaultInfo{
			SwitchFaultLevel: "Warning", SwitchFaultName: "switch-1",
		}},
		{Name: "n-check", Status: "Ready", Conditions: []corev1.NodeCondition{
			{Type: "NT_NPU_ECC_COUNT", Status: corev1.ConditionTrue, Message: "ecc count exceed"},
		}},
	}
	return nodes
}

func TestAlertMessagesEnIdentityAllSites(t *testing.T) {
	agg := &AggregatedDataSource{}
	agg.SetNPUExporterClient(&NPUExporterClient{lastFailure: time.Now()})

	minutesAgo := time.Now().Add(-10 * time.Minute)
	pods := []*model.PodData{
		{Name: "p-oom", Namespace: "default", ContainerStates: []model.ContainerState{{Name: "oom-worker", Reason: "OOMKilled"}}},
		{Name: "p-crash", Namespace: "default", ContainerStates: []model.ContainerState{{Name: "crash-worker", Reason: "CrashLoopBackOff"}}},
		{Name: "p-pull", Namespace: "default", ContainerStates: []model.ContainerState{{Name: "pull-worker", Reason: "ImagePullBackOff"}}},
		{Name: "p-restarts", Namespace: "default", RestartCount: 6},
		{Name: "p-pending", Namespace: "default", Phase: "Pending", CreationTimestamp: minutesAgo},
		{Name: "p-failed", Namespace: "default", Phase: "Failed"},
	}
	services := []*model.ServiceData{
		{Name: "svc-0", Namespace: "default", EndpointCount: 0},
	}
	pvcs := []*model.PVCData{
		{Name: "pvc-0", Namespace: "default", Status: "Pending", CreationTimestamp: minutesAgo},
	}

	alerts := agg.collectAlerts(allAlertSitesNodes(), pods, services, pvcs, nil)

	seen := make(map[model.AlertType]int)
	for _, a := range alerts {
		seen[a.AlertType]++
		key, keyed := alertMessageKeys[a.AlertType]
		switch {
		case keyed:
			if a.MessageKey != key {
				t.Errorf("%s: MessageKey = %q, want %q", a.AlertType, a.MessageKey, key)
			}
			got := i18n.NewLocalizer("en").TF(a.MessageKey, a.MessageArgs)
			if got != a.Message {
				t.Errorf("%s: en TF(%q, %v) = %q, Message = %q (not byte-identical)",
					a.AlertType, a.MessageKey, a.MessageArgs, got, a.Message)
			}
		case rawAlertMessageTypes[a.AlertType]:
			if a.MessageKey != "" {
				t.Errorf("%s: raw site MessageKey = %q, want empty", a.AlertType, a.MessageKey)
			}
			if a.MessageArgs != nil {
				t.Errorf("%s: raw site MessageArgs = %v, want nil", a.AlertType, a.MessageArgs)
			}
		default:
			t.Errorf("unexpected alert type %s (Message=%q) — fixture or new site unaccounted", a.AlertType, a.Message)
		}
	}

	// Every one of the 26 sites must have actually fired exactly once.
	for at, key := range alertMessageKeys {
		if seen[at] != 1 {
			t.Errorf("keyed site %s (key %s) fired %d times, want 1 — probe fixture broken", at, key, seen[at])
		}
	}
	for at := range rawAlertMessageTypes {
		if seen[at] != 1 {
			t.Errorf("raw site %s fired %d times, want 1 — probe fixture broken", at, seen[at])
		}
	}
}

// TestAlertMessagesZhRenderSanity: every keyed alert must render in zh without key-name
// leakage, unresolved placeholders, or fmt-style errors.
func TestAlertMessagesZhRenderSanity(t *testing.T) {
	agg := &AggregatedDataSource{}
	agg.SetNPUExporterClient(&NPUExporterClient{lastFailure: time.Now()})
	alerts := agg.collectAlerts(allAlertSitesNodes(), nil, nil, nil, nil)

	zh := i18n.NewLocalizer("zh")
	for _, a := range alerts {
		if _, keyed := alertMessageKeys[a.AlertType]; !keyed {
			continue
		}
		got := zh.TF(a.MessageKey, a.MessageArgs)
		for _, bad := range []string{"alert.", "{{", "}}", "<no value>", "%!"} {
			if strings.Contains(got, bad) {
				t.Errorf("%s: zh render %q contains %q", a.AlertType, got, bad)
			}
		}
	}
}

// Args spot-checks on sites whose named arguments are load-bearing: the
// identity check above catches drift only when args feed a placeholder, so
// these pin the actual arg values the render path will pass to TF.
func TestAlertMessageArgsValues(t *testing.T) {
	agg := &AggregatedDataSource{}
	alerts := agg.collectAlerts(allAlertSitesNodes(), nil, nil, nil, nil)

	type want struct {
		alertType model.AlertType
		args      map[string]interface{}
	}
	wants := []want{
		{model.AlertTypeNodeNotReady, map[string]interface{}{"Status": "NotReady"}},
		{model.AlertTypeNPUChipFault, map[string]interface{}{"Chips": "2"}},
		{model.AlertTypeNPUTempHigh, map[string]interface{}{"Chips": "3", "Max": 95}},
		{model.AlertTypeNPULinkDown, map[string]interface{}{"Chips": "6"}},
		{model.AlertTypeNPUErrorCode, map[string]interface{}{"Chips": "7 (code 42)"}},
	}
	for _, w := range wants {
		matched := npuAlertsOfType(alerts, w.alertType)
		if len(matched) != 1 {
			t.Fatalf("%s: got %d alerts, want 1", w.alertType, len(matched))
		}
		if len(matched[0].MessageArgs) != len(w.args) {
			t.Fatalf("%s: MessageArgs = %v, want %v", w.alertType, matched[0].MessageArgs, w.args)
		}
		for k, v := range w.args {
			if matched[0].MessageArgs[k] != v {
				t.Errorf("%s: MessageArgs[%q] = %v (%T), want %v (%T)", w.alertType, k, matched[0].MessageArgs[k], matched[0].MessageArgs[k], v, v)
			}
		}
	}
}
