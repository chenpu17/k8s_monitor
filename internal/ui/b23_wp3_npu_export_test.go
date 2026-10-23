package ui

// b23_wp3_npu_export_test.go — batch 23 W3: the export shape the new field
// changes, and the i18n surface of the batch.
//
// NodeData gained NPUChipHealth, and exportNodesJSON encodes the whole NodeData
// slice, so a node with chip data now carries an "npu_chip_health" array. The
// task book accepts that increment explicitly; the probes pin that it is exactly
// the intended shape, that a node without chip data is byte-identical to before,
// and that the new keys exist in both locales.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// TestExportNodesJSONChipHealthShape pins the export increment: a node with chip
// data exports the per-chip array (npu, chip, grade, reason), and a node without
// it exports no field at all (omitempty).
func TestExportNodesJSONChipHealthShape(t *testing.T) {
	dir := t.TempDir()

	withChips := &model.NodeData{
		Name: "npu-node", Status: "Ready", NPUCapacity: 4,
		NPUChipHealth: []model.NPUChipGrade{
			{NPUID: 0, Chip: 0, Grade: model.NPUChipGradeHealthy},
			{NPUID: 0, Chip: 1, Grade: model.NPUChipGradeDegraded, Reason: "single-bit ECC"},
			{NPUID: 1, Chip: 0, Grade: model.NPUChipGradeFault, Reason: "double-bit ECC"},
			{NPUID: 1, Chip: 1, Grade: model.NPUChipGradeHealthy},
		},
	}
	withoutChips := &model.NodeData{Name: "plain-node", Status: "Ready"}

	m := NewModel(nil, nil, 0, "en", "test", 200)
	data := &model.ClusterData{Nodes: []*model.NodeData{withChips, withoutChips}}
	if err := m.exportNodesJSON(data, dir, "nodes.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)

	// The graded node carries the array, one entry per chip in chip order.
	for _, want := range []string{
		`"npu": 0,`,
		`"chip": 1,`,
		`"grade": "degraded",`,
		`"reason": "single-bit ECC"`,
		`"grade": "fault",`,
		`"reason": "double-bit ECC"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the export is missing %q:\n%s", want, out)
		}
	}

	// A healthy chip carries no reason: there is nothing to explain about it.
	var decoded []struct {
		Name          string           `json:"Name"`
		NPUChipHealth []map[string]any `json:"npu_chip_health"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the export is not valid JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("exported %d nodes, want 2", len(decoded))
	}
	if got := len(decoded[0].NPUChipHealth); got != 4 {
		t.Fatalf("the graded node exported %d chip entries, want 4", got)
	}
	if _, ok := decoded[0].NPUChipHealth[0]["reason"]; ok {
		t.Errorf("a healthy chip exported a reason: %v", decoded[0].NPUChipHealth[0])
	}

	// The node without chip data exports no field at all: the array is
	// omitempty, so every pre-batch node keeps its exact previous shape.
	if decoded[1].NPUChipHealth != nil {
		t.Errorf("a node without chip data exported npu_chip_health: %v", decoded[1].NPUChipHealth)
	}
	if strings.Count(out, "npu_chip_health") != 1 {
		t.Errorf("the field appears %d times, want once (only on the graded node):\n%s",
			strings.Count(out, "npu_chip_health"), out)
	}
}

// TestExportNodesJSONChipHealthRoundTrip pins that the exported array decodes
// back into the model type, so the field is a real contract and not just a
// string that happens to be there.
func TestExportNodesJSONChipHealthRoundTrip(t *testing.T) {
	dir := t.TempDir()

	want := []model.NPUChipGrade{
		{NPUID: 2, Chip: 0, Grade: model.NPUChipGradeDegraded, Reason: "temp 82>=80"},
		{NPUID: 2, Chip: 1, Grade: model.NPUChipGradeFault, Reason: "link down"},
	}
	m := NewModel(nil, nil, 0, "en", "test", 200)
	data := &model.ClusterData{Nodes: []*model.NodeData{{
		Name: "npu-node", NPUCapacity: 2, NPUChipHealth: want,
	}}}
	if err := m.exportNodesJSON(data, dir, "nodes.json"); err != nil {
		t.Fatalf("exportNodesJSON: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded []model.NodeData
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding the export back into NodeData: %v", err)
	}
	if len(decoded) != 1 || len(decoded[0].NPUChipHealth) != len(want) {
		t.Fatalf("round trip produced %+v", decoded)
	}
	for i, grade := range decoded[0].NPUChipHealth {
		if grade != want[i] {
			t.Errorf("chip %d round-tripped to %+v, want %+v", i, grade, want[i])
		}
	}
}

// TestNPUViewBundleKeys pins the batch's new keys in both bundles: every key the
// view renders exists in en and zh, the en/zh key sets still agree, and the zh
// copy carries no half-width colon.
func TestNPUViewBundleKeys(t *testing.T) {
	en := readBundleMessages(t, "../../internal/i18n/active.en.toml")
	zh := readBundleMessages(t, "../../internal/i18n/active.zh.toml")

	keys := []string{
		"views.npu.name",
		"npuview.title",
		"npuview.health_summary",
		"npuview.health_no_data",
		"npuview.alloc_summary",
		"npuview.alloc_summary_no_pct",
		"npuview.coverage",
		"npuview.legend",
		"npuview.row_degraded",
		"npuview.row_fault",
		"npuview.monitoring_unavailable",
		"npuview.no_nodes",
		"npuview.alerts_header",
		"npuview.type_chip_fault",
		"npuview.type_fault_level",
		"npuview.type_switch_fault",
		"npuview.type_check_failed",
		"npuview.type_temp_high",
		"npuview.type_ecc_double",
		"npuview.type_ecc_single",
		"npuview.type_link_down",
		"npuview.type_error_code",
		"npuview.type_monitor_degraded",
	}
	for _, id := range keys {
		if _, ok := en[id]; !ok {
			t.Errorf("the en bundle is missing %s", id)
		}
		if _, ok := zh[id]; !ok {
			t.Errorf("the zh bundle is missing %s", id)
		}
	}

	// zh colon convention: a zh value that carries a colon uses the full-width
	// one, like every other zh label.
	for _, id := range keys {
		value, ok := zh[id]
		if !ok {
			continue
		}
		if strings.Contains(value, ": ") && !strings.Contains(value, "：") {
			t.Errorf("zh %s still uses a half-width colon: %q", id, value)
		}
	}

	// Every NPU alert type the view counts has its own label key, and the list
	// stays in sync with the filtering set: a type added on one side only would
	// be counted but unlabelled (or labelled but never counted).
	if len(npuAlertTypeKeys) != len(npuAlertTypeSet) {
		t.Errorf("the alert type list has %d entries, the lookup set %d", len(npuAlertTypeKeys), len(npuAlertTypeSet))
	}
	for _, entry := range npuAlertTypeKeys {
		if _, ok := en[entry.key]; !ok {
			t.Errorf("the en bundle is missing the alert type label %s", entry.key)
		}
		if _, ok := zh[entry.key]; !ok {
			t.Errorf("the zh bundle is missing the alert type label %s", entry.key)
		}
		if !npuAlertTypeSet[entry.alertType] {
			t.Errorf("alert type %s has a label but is not in the filter set", entry.alertType)
		}
	}
}
