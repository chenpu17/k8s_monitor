package datasource

// b25_wp1_owner_test.go — batch 25 W1: the chip ownership attribution.
//
// NPU-Exporter is the only source that maps a chip to a workload: every
// npu_chip_info_* line carries pod_name / namespace / container_name labels
// (empty for an unallocated chip). The parse layer already read them into
// NPUChipMetrics; these probes pin that the conversion now carries them into
// model.NPUChipData and that an unallocated chip exports no ownership at all.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b25AssignedPayload is a synthetic exporter payload with two allocated chips of
// the same pod and one idle chip. The reference cluster has 0/8 allocated, so
// this shape only exists in tests.
const b25AssignedPayload = `machine_npu_nums 2
npu_chip_info_utilization{id="0",pod_name="train-0",namespace="ml",container_name="trainer"} 87
npu_chip_info_hbm_used_memory{id="0",pod_name="train-0",namespace="ml",container_name="trainer"} 1024
npu_chip_info_utilization{id="1",pod_name="train-0",namespace="ml",container_name="trainer"} 85
npu_chip_info_utilization{id="2",pod_name="",namespace="",container_name=""} 0
`

// TestConvertChipMetricsCopiesOwner pins the conversion step: the exporter's
// ownership labels land on NPUChipData, and an idle chip keeps the empty value.
func TestConvertChipMetricsCopiesOwner(t *testing.T) {
	client := &NPUExporterClient{}

	assigned := client.convertChipMetrics(&NPUChipMetrics{
		ID: 0, PodName: "train-0", Namespace: "ml", ContainerName: "trainer",
	})
	if assigned.OwnerPod != "train-0" || assigned.OwnerNamespace != "ml" {
		t.Errorf("assigned chip owner = %q/%q, want train-0/ml", assigned.OwnerNamespace, assigned.OwnerPod)
	}

	idle := client.convertChipMetrics(&NPUChipMetrics{ID: 2})
	if idle.OwnerPod != "" || idle.OwnerNamespace != "" {
		t.Errorf("idle chip owner = %q/%q, want empty", idle.OwnerNamespace, idle.OwnerPod)
	}
}

// TestParseConvertOwnerEndToEnd drives the whole chain — Prometheus payload →
// parseMetrics → convertChipMetrics → EnrichNodeData — because the field is
// read in one function and copied in another, and a break in either leaves the
// UI silently unattributed.
func TestParseConvertOwnerEndToEnd(t *testing.T) {
	client, _ := newTestNPUExporterClient(t, serveNPUMetrics(b25AssignedPayload))

	node := nodeWithNPU("npu-node-1", 2)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData: %v", err)
	}

	want := map[int]struct{ ns, pod string }{
		0: {"ml", "train-0"},
		1: {"ml", "train-0"},
		2: {"", ""},
	}
	if len(node.NPUChips) != len(want) {
		t.Fatalf("enriched %d chips, want %d: %+v", len(node.NPUChips), len(want), node.NPUChips)
	}
	for _, chip := range node.NPUChips {
		w := want[chip.PhyID]
		if chip.OwnerNamespace != w.ns || chip.OwnerPod != w.pod {
			t.Errorf("chip %d owner = %q/%q, want %q/%q", chip.PhyID, chip.OwnerNamespace, chip.OwnerPod, w.ns, w.pod)
		}
	}
}

// TestOwnerOmittedWhenIdle pins the sparse shape of the model field: an idle
// chip marshals no owner keys at all, so the JSON export (which encodes the
// whole NodeData) grows keys only for chips the exporter actually attributed.
func TestOwnerOmittedWhenIdle(t *testing.T) {
	idle, err := json.Marshal(model.NPUChipData{NPUID: 0, Chip: 0, PhyID: 0})
	if err != nil {
		t.Fatalf("marshal idle chip: %v", err)
	}
	for _, absent := range []string{"owner_pod", "owner_namespace"} {
		if strings.Contains(string(idle), absent) {
			t.Errorf("an idle chip marshalled the %q key: %s", absent, idle)
		}
	}

	assigned, err := json.Marshal(model.NPUChipData{NPUID: 0, Chip: 0, PhyID: 0, OwnerPod: "train-0", OwnerNamespace: "ml"})
	if err != nil {
		t.Fatalf("marshal assigned chip: %v", err)
	}
	for _, want := range []string{`"owner_pod":"train-0"`, `"owner_namespace":"ml"`} {
		if !strings.Contains(string(assigned), want) {
			t.Errorf("an assigned chip marshalled without %s: %s", want, assigned)
		}
	}
}
