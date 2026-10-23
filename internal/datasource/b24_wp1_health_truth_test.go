package datasource

// b24_wp1_health_truth_test.go — batch 24 W1/W4: the exporter health status is
// three-valued (1 healthy, 0 unhealthy, -1 DCMI failure) and may be absent, and
// the exporter's own fault signal now produces an alert next to the device
// plugin's bitmask alert instead of silently colouring the matrix.
//
// Real-cluster reference: 192.168.2.188, 8 chips, all health_status=1 and
// error_code=0 (/tmp/b24_metrics.txt).

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b24RealMetrics is the shape of a live NPU-Exporter payload for the reference
// node, trimmed to the lines these probes need.
func b24MetricsPayload(healthLines string) string {
	return `machine_npu_nums 1
npu_chip_info_utilization{id="0"} 10
npu_chip_info_temperature{id="0"} 47
` + healthLines
}

// TestParseMetricsHealthStatusKnown pins the parse layer: a chip whose
// health_status metric is present is marked known; a chip without the metric
// keeps Known=false so the missing value cannot be read as "unhealthy".
func TestParseMetricsHealthStatusKnown(t *testing.T) {
	client, _ := newTestNPUExporterClient(t, serveNPUMetrics(b24MetricsPayload(
		"npu_chip_info_health_status{id=\"0\"} 1\n")))

	chips, _, err := client.GetMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if chip := chips[0]; !chip.HealthStatusKnown || chip.HealthStatus != 1 {
		t.Errorf("reported chip = %+v, want HealthStatusKnown with status 1", chip)
	}
}

// TestConvertChipMetricsHealthThreeStates pins the conversion: only a reported
// 1 / 0 maps to OK / Error; -1 and a missing metric both map to Unknown, which
// must never paint a chip as faulty.
func TestConvertChipMetricsHealthThreeStates(t *testing.T) {
	client := &NPUExporterClient{}

	cases := []struct {
		name    string
		chip    NPUChipMetrics
		want    string
		wantWhy string
	}{
		{"reported healthy", NPUChipMetrics{HealthStatus: 1, HealthStatusKnown: true}, "OK", "1 means healthy"},
		{"reported unhealthy", NPUChipMetrics{HealthStatus: 0, HealthStatusKnown: true}, "Error", "0 means unhealthy"},
		{"reported unknown", NPUChipMetrics{HealthStatus: -1, HealthStatusKnown: true}, "Unknown", "-1 is a DCMI failure, not a fault"},
		{"metric absent", NPUChipMetrics{}, "Unknown", "a missing metric says nothing about the chip"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := client.convertChipMetrics(&tc.chip).Health; got != tc.want {
				t.Errorf("Health = %q, want %q (%s)", got, tc.want, tc.wantWhy)
			}
		})
	}
}

// TestEnrichNodeDataHealthStates lays the four states over one node and checks
// both the per-chip strings and the node aggregate: only explicitly unhealthy
// chips count against the node, and a node whose every chip is unknown reports
// "Unknown" rather than claiming health nobody reported.
func TestEnrichNodeDataHealthStates(t *testing.T) {
	const payload = `machine_npu_nums 1
npu_chip_info_utilization{id="0"} 10
npu_chip_info_health_status{id="0"} 1
npu_chip_info_utilization{id="1"} 10
npu_chip_info_health_status{id="1"} 0
npu_chip_info_utilization{id="2"} 10
npu_chip_info_health_status{id="2"} -1
npu_chip_info_utilization{id="3"} 10
`
	client, _ := newTestNPUExporterClient(t, serveNPUMetrics(payload))

	node := nodeWithNPU("npu-node-1", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData: %v", err)
	}

	want := map[int]string{0: "OK", 1: "Error", 2: "Unknown", 3: "Unknown"}
	for _, chip := range node.NPUChips {
		if chip.Health != want[chip.PhyID] {
			t.Errorf("chip %d Health = %q, want %q", chip.PhyID, chip.Health, want[chip.PhyID])
		}
	}
	if node.NPUErrorCount != 1 {
		t.Errorf("NPUErrorCount = %d, want 1 (only the explicitly unhealthy chip)", node.NPUErrorCount)
	}
	if node.NPUHealthStatus != "Warning" {
		t.Errorf("NPUHealthStatus = %q, want Warning", node.NPUHealthStatus)
	}

	// Every chip unknown: the aggregate must not claim healthy.
	const unknownOnly = `machine_npu_nums 1
npu_chip_info_utilization{id="0"} 10
npu_chip_info_health_status{id="0"} -1
npu_chip_info_utilization{id="1"} 10
`
	client2, _ := newTestNPUExporterClient(t, serveNPUMetrics(unknownOnly))
	node2 := nodeWithNPU("npu-node-2", 1)
	if err := client2.EnrichNodeData(context.Background(), []*model.NodeData{node2}); err != nil {
		t.Fatalf("EnrichNodeData: %v", err)
	}
	if node2.NPUHealthStatus != "Unknown" {
		t.Errorf("NPUHealthStatus = %q for an all-unknown node, want Unknown", node2.NPUHealthStatus)
	}
	if node2.NPUErrorCount != 0 {
		t.Errorf("NPUErrorCount = %d for unknown chips, want 0 (unknown is not a fault)", node2.NPUErrorCount)
	}
}

// TestNPUHealthFaultAlertClosesContradiction is the W1-4 probe: a chip the
// exporter reports unhealthy must produce exactly one chip-fault alert even
// when the device plugin bitmask says nothing, so the NPU view can no longer
// show "1 fault" above "No alerts detected".
func TestNPUHealthFaultAlertClosesContradiction(t *testing.T) {
	chips := gradedChips(8)
	chips[2].Health = "Error"

	node := &model.NodeData{Name: "192.168.2.188", NPUCapacity: 8, NPUChips: chips}
	alerts := npuThresholdAlerts(node, time.Now())

	matched := npuAssertSingleAlert(t, alerts, model.AlertTypeNPUChipFault)
	if matched.MessageKey != "alert.npu_chip_health" {
		t.Errorf("MessageKey = %q, want alert.npu_chip_health (the health-status site, not the bitmask site)", matched.MessageKey)
	}
	if matched.Severity != model.AlertSeverityCritical {
		t.Errorf("Severity = %v, want Critical", matched.Severity)
	}
	if matched.Message != "NPU chip(s) 2 unhealthy per NPU-Exporter health status" {
		t.Errorf("Message = %q", matched.Message)
	}
	if matched.Value != "chips 2" || matched.ValueKey != "alert.value_chips" {
		t.Errorf("Value/ValueKey = %q/%q, want chips 2/alert.value_chips", matched.Value, matched.ValueKey)
	}
	if matched.RecommendedAction == "" {
		t.Error("RecommendedAction is empty")
	}
	// The grade and the alert must agree on the chip.
	if got := gradeOf(t, node.NPUChipHealth, 2); got.Grade != model.NPUChipGradeFault {
		t.Errorf("chip 2 grade = %q, want fault", got.Grade)
	}
}

// TestNPUHealthFaultAlertCoexistsWithBitmask pins the deliberate double signal:
// the device plugin bitmask and the exporter health status are independent
// sources, so a node where both point at chips yields two alerts of the same
// type, each naming its own chips.
func TestNPUHealthFaultAlertCoexistsWithBitmask(t *testing.T) {
	chips := gradedChips(8)
	chips[5].Health = "Error"

	node := &model.NodeData{
		Name: "npu-node", NPUCapacity: 8, NPUChips: chips,
		NPUStatusMask: 0xffffffffffffff01, NPUStatusKnown: true, // chip 0 faulty
	}
	// The bitmask alert is raised by the cluster-level collector, the exporter
	// alert by the per-chip detection: run both over the same node so the
	// coexistence is the real one.
	agg := &AggregatedDataSource{}
	alerts := agg.collectAlerts(npuTestNodes(node), nil, nil, nil, nil)

	var byKey = map[string]string{}
	for _, alert := range alerts {
		if alert.AlertType == model.AlertTypeNPUChipFault {
			byKey[alert.MessageKey] = alert.Value
		}
	}
	if len(byKey) != 2 {
		t.Fatalf("got %d chip fault alerts, want 2 (one per source): %+v", len(byKey), alerts)
	}
	if byKey["alert.npu_chip_fault"] != "chips 0" {
		t.Errorf("bitmask alert value = %q, want chips 0", byKey["alert.npu_chip_fault"])
	}
	if byKey["alert.npu_chip_health"] != "chips 5" {
		t.Errorf("exporter alert value = %q, want chips 5", byKey["alert.npu_chip_health"])
	}
}

// TestNPUHealthUnknownDoesNotAlert pins the other half of the three-state
// truth: a chip whose health is unknown is neither graded fault nor alerted on.
func TestNPUHealthUnknownDoesNotAlert(t *testing.T) {
	chips := gradedChips(8)
	for i := range chips {
		chips[i].Health = "Unknown"
	}

	node := &model.NodeData{Name: "npu-node", NPUCapacity: 8, NPUChips: chips}
	alerts := npuThresholdAlerts(node, time.Now())

	if got := countNPUAlerts(alerts); got != 0 {
		t.Errorf("got %d NPU alerts for all-unknown health, want 0: %+v", got, alerts)
	}
	for _, grade := range node.NPUChipHealth {
		if grade.Grade != model.NPUChipGradeHealthy {
			t.Errorf("chip %d/%d = %q for unknown health, want healthy", grade.NPUID, grade.Chip, grade.Grade)
		}
	}
}

// TestNPUErrorCodeAlertUsesKnowledgeBase pins the W3 alert wiring: a code the
// knowledge base classifies replaces the generic recommendation, an
// unclassified code keeps it, and the message keeps the raw decimal code.
func TestNPUErrorCodeAlertUsesKnowledgeBase(t *testing.T) {
	known, ok := npuErrorCodeInfo(0x80E21007)
	if !ok {
		t.Fatalf("0x80E21007 is not in the knowledge base")
	}

	chips := gradedChips(4)
	chips[1].ErrorCode = 0x80E21007
	node := &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: chips}
	alert := npuAssertSingleAlert(t, npuThresholdAlerts(node, time.Now()), model.AlertTypeNPUErrorCode)

	if alert.RecommendedAction != known.Action {
		t.Errorf("RecommendedAction = %q, want the knowledge-base action %q", alert.RecommendedAction, known.Action)
	}
	if !strings.Contains(alert.Message, "code 2162298887") {
		t.Errorf("Message lost the raw code: %q", alert.Message)
	}
	if got := gradeOf(t, node.NPUChipHealth, 1); got.Reason != "error code 80E21007 (no action needed)" {
		t.Errorf("grade reason = %q", got.Reason)
	}

	// An unclassified code keeps the generic recommendation and the old reason.
	chips = gradedChips(4)
	chips[1].ErrorCode = 0xDEADBEEF
	node = &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: chips}
	alert = npuAssertSingleAlert(t, npuThresholdAlerts(node, time.Now()), model.AlertTypeNPUErrorCode)

	wantGeneric := "npu-smi info -t errorcode -i 0 # Look up the reported error code for npu-node and open a hardware support case if it persists"
	if alert.RecommendedAction != wantGeneric {
		t.Errorf("RecommendedAction = %q, want the generic hint", alert.RecommendedAction)
	}
	if got := gradeOf(t, node.NPUChipHealth, 1); got.Reason != "error code 3735928559" {
		t.Errorf("grade reason = %q", got.Reason)
	}
}

// TestNPUErrorCodeAlertMixedCodesKeepsGeneric pins the no-half-truth rule: when
// one offending code is classified and another is not, the alert keeps the
// generic recommendation instead of advertising a tier for the unknown one.
func TestNPUErrorCodeAlertMixedCodesKeepsGeneric(t *testing.T) {
	chips := gradedChips(4)
	chips[1].ErrorCode = 0x80E21007 // known
	chips[2].ErrorCode = 0xDEADBEEF // unknown

	node := &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: chips}
	alert := npuAssertSingleAlert(t, npuThresholdAlerts(node, time.Now()), model.AlertTypeNPUErrorCode)

	if strings.Contains(alert.RecommendedAction, "No action required") {
		t.Errorf("RecommendedAction advertises a knowledge-base tier for a mixed code set: %q", alert.RecommendedAction)
	}
}

// TestLivenessStates pins the W4 state machine: never attempted, ok, backoff,
// and the recovery that clears the failure text again.
func TestLivenessStates(t *testing.T) {
	t.Run("never attempted", func(t *testing.T) {
		client := &NPUExporterClient{}
		got := client.Liveness()
		if got.Status != "never" || got.MessageKey != "npu.liveness.never" {
			t.Errorf("Liveness = %+v, want never/npu.liveness.never", got)
		}
		if got.LastError != "" || got.MessageArgs != nil {
			t.Errorf("a never-attempted client carries failure text: %+v", got)
		}
	})

	t.Run("ok after a success", func(t *testing.T) {
		client := &NPUExporterClient{}
		client.recordSuccess()
		got := client.Liveness()
		if got.Status != "ok" || got.MessageKey != "npu.liveness.ok" {
			t.Errorf("Liveness = %+v, want ok/npu.liveness.ok", got)
		}
		if got.LastSuccess.IsZero() {
			t.Error("LastSuccess is zero after a success")
		}
	})

	t.Run("backoff after a failure", func(t *testing.T) {
		client := &NPUExporterClient{}
		client.recordSuccess()
		success := client.Liveness().LastSuccess
		client.recordFailure(http.ErrAbortHandler)
		got := client.Liveness()
		if got.Status != "backoff" || got.MessageKey != "npu.liveness.backoff" {
			t.Errorf("Liveness = %+v, want backoff/npu.liveness.backoff", got)
		}
		if got.LastError == "" {
			t.Error("LastError is empty during backoff")
		}
		if got.MessageArgs["Error"] != got.LastError {
			t.Errorf("MessageArgs[Error] = %v, want the raw error %q", got.MessageArgs["Error"], got.LastError)
		}
		if !got.LastSuccess.Equal(success) {
			t.Errorf("LastSuccess = %v, want the last successful fetch %v", got.LastSuccess, success)
		}
	})

	t.Run("recovery clears the failure text", func(t *testing.T) {
		client := &NPUExporterClient{}
		client.recordFailure(http.ErrAbortHandler)
		client.recordSuccess()
		got := client.Liveness()
		if got.Status != "ok" || got.LastError != "" {
			t.Errorf("Liveness = %+v, want ok without failure text", got)
		}
	})
}

// TestLivenessReportsFetchFailure drives the real fetch path against a dead
// endpoint: the raw transport error must reach Liveness instead of being
// reduced to a boolean.
func TestLivenessReportsFetchFailure(t *testing.T) {
	client, _ := newTestNPUExporterClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	if err := client.CheckAvailability(context.Background()); err == nil {
		t.Fatal("CheckAvailability succeeded against an erroring endpoint")
	}
	liveness := client.Liveness()
	if liveness.Status != "backoff" || liveness.LastError == "" {
		t.Fatalf("Liveness = %+v, want backoff with the transport error", liveness)
	}
	if !strings.Contains(liveness.LastError, "500") {
		t.Errorf("LastError = %q, want the HTTP status text", liveness.LastError)
	}
}
