package datasource

// b24_wp5_ecc_delta_test.go — batch 24 W5: the session-scoped ECC delta.
//
// The exporter publishes two counter families per chip: the per-window _cnt
// family (the alert signal) and the cumulative _total_ family. The reference
// node has chip 2 at 99992 cumulative single-bit corrections while its
// per-window value is 0 — history, not an ongoing fault. The delta answers the
// third question ("did the total move since monitoring started") without
// touching the alert semantics.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// eccPayload builds an exporter payload whose chip 0 carries the given
// cumulative single-bit total and chip 1 the given cumulative double-bit total.
func eccPayload(singleTotal, doubleTotal int64) string {
	return fmt.Sprintf(`machine_npu_nums 1
npu_chip_info_utilization{id="0"} 10
npu_chip_info_health_status{id="0"} 1
npu_chip_info_hbm_ecc_total_single_bit_error_cnt{id="0"} %d
npu_chip_info_utilization{id="1"} 10
npu_chip_info_health_status{id="1"} 1
npu_chip_info_hbm_ecc_total_double_bit_error_cnt{id="1"} %d
`, singleTotal, doubleTotal)
}

// TestECCDeltaSlidingWindowMath pins the arithmetic and the window bound: with
// a payload per refresh, the delta is newest-minus-oldest, stays at 0 until the
// second sample exists, and the window never grows past its depth.
func TestECCDeltaSlidingWindowMath(t *testing.T) {
	totals := []int64{100, 120, 150, 200, 210, 220, 230, 240, 250, 260, 300}

	// Drive the client directly: the window logic is what is under test, and the
	// HTTP path is covered by TestECCDeltaThroughEnrichment.
	c := &NPUExporterClient{}
	base := time.Now()

	if got := c.recordECCSamples([]*NPUChipMetrics{{ID: 0, HBMEccSingleBitErrTotal: totals[0]}}, base); got[0].single != 0 || got[0].samples != 1 {
		t.Fatalf("first sample = %+v, want zero delta over 1 sample", got[0])
	}

	for i := 1; i < len(totals); i++ {
		got := c.recordECCSamples([]*NPUChipMetrics{{ID: 0, HBMEccSingleBitErrTotal: totals[i]}}, base.Add(time.Duration(i)*time.Second))[0]

		wantSamples := i + 1
		if wantSamples > eccSampleWindow {
			wantSamples = eccSampleWindow
		}
		if got.samples != wantSamples {
			t.Fatalf("refresh %d: window holds %d samples, want %d", i, got.samples, wantSamples)
		}

		oldest := totals[max(0, i-(eccSampleWindow-1))]
		if want := totals[i] - oldest; got.single != want {
			t.Errorf("refresh %d: delta = %d, want %d (newest %d - oldest %d)", i, got.single, want, totals[i], oldest)
		}
	}

	// Non-vacuousness: the last delta must not be the whole history (the 1st
	// sample has already scrolled out of the window).
	last := c.recordECCSamples([]*NPUChipMetrics{{ID: 0, HBMEccSingleBitErrTotal: 310}}, base.Add(time.Minute))[0]
	if last.single == 310-totals[0] {
		t.Fatalf("the window still holds the first sample: delta %d", last.single)
	}
}

// TestECCDeltaCounterReset pins the rollback handling: a cumulative counter that
// went backwards (chip reset, driver reload) clears the window and keeps only
// the new sample, so the delta never goes negative and does not compare across
// the reset.
func TestECCDeltaCounterReset(t *testing.T) {
	c := &NPUExporterClient{}
	now := time.Now()

	c.recordECCSamples([]*NPUChipMetrics{{ID: 3, HBMEccSingleBitErrTotal: 500}}, now)
	before := c.recordECCSamples([]*NPUChipMetrics{{ID: 3, HBMEccSingleBitErrTotal: 700}}, now.Add(time.Second))[3]
	if before.single != 200 {
		t.Fatalf("delta before the reset = %d, want 200", before.single)
	}

	after := c.recordECCSamples([]*NPUChipMetrics{{ID: 3, HBMEccSingleBitErrTotal: 5}}, now.Add(2*time.Second))[3]
	if after.single != 0 {
		t.Errorf("delta across a counter reset = %d, want 0", after.single)
	}
	if after.samples != 1 {
		t.Errorf("window after a reset holds %d samples, want 1", after.samples)
	}

	next := c.recordECCSamples([]*NPUChipMetrics{{ID: 3, HBMEccSingleBitErrTotal: 12}}, now.Add(3*time.Second))[3]
	if next.single != 7 {
		t.Errorf("delta after the reset = %d, want 7 (12 - 5)", next.single)
	}
}

// TestECCDeltaPerChipIsolation pins that the windows are per chip: one chip
// resetting must not disturb another chip's delta.
func TestECCDeltaPerChipIsolation(t *testing.T) {
	c := &NPUExporterClient{}
	now := time.Now()

	c.recordECCSamples([]*NPUChipMetrics{
		{ID: 0, HBMEccSingleBitErrTotal: 10},
		{ID: 1, HBMEccSingleBitErrTotal: 40},
	}, now)
	got := c.recordECCSamples([]*NPUChipMetrics{
		{ID: 0, HBMEccSingleBitErrTotal: 2}, // reset
		{ID: 1, HBMEccSingleBitErrTotal: 55},
	}, now.Add(time.Second))

	if got[0].single != 0 || got[0].samples != 1 {
		t.Errorf("reset chip = %+v, want zero delta over one sample", got[0])
	}
	if got[1].single != 15 || got[1].samples != 2 {
		t.Errorf("steady chip = %+v, want delta 15 over two samples", got[1])
	}
}

// TestECCDeltaThroughEnrichment drives the real enrichment path: two refreshes
// against the same exporter attach the delta to the chip data, and the reference
// node's shape (a six-digit cumulative total with a silent per-window counter)
// stays delta-free on the first refresh.
func TestECCDeltaThroughEnrichment(t *testing.T) {
	var total int64 = 99992
	client, _ := newTestNPUExporterClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(eccPayload(total, 0)))
	})

	first := nodeWithNPU("192.168.2.188", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{first}); err != nil {
		t.Fatalf("EnrichNodeData: %v", err)
	}
	for _, chip := range first.NPUChips {
		if chip.HBMEccSingleErrDelta != 0 || chip.HBMEccDoubleErrDelta != 0 {
			t.Errorf("chip %d carries a delta after one refresh: %+v", chip.PhyID, chip)
		}
	}
	if got := first.NPUChips[0].HBMEccSingleErrTotal; got != 99992 {
		t.Errorf("cumulative total = %d, want the passthrough 99992", got)
	}

	total = 100002
	second := nodeWithNPU("192.168.2.188", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{second}); err != nil {
		t.Fatalf("EnrichNodeData: %v", err)
	}
	if got := second.NPUChips[0].HBMEccSingleErrDelta; got != 10 {
		t.Errorf("delta = %d, want 10", got)
	}
	if got := second.NPUChips[0].HBMEccDeltaSamples; got != 2 {
		t.Errorf("delta samples = %d, want 2", got)
	}
	if got := second.NPUChips[0].HBMEccSingleErr; got != 0 {
		t.Errorf("the per-window counter changed: %d", got)
	}
	// No alert: the delta is display-only (the reference node's 99992
	// cumulative corrections must never page anyone).
	node := &model.NodeData{Name: "192.168.2.188", NPUCapacity: 8, NPUChips: second.NPUChips}
	if alerts := npuThresholdAlerts(node, time.Now()); countNPUAlerts(alerts) != 0 {
		t.Errorf("the ECC delta raised alerts: %+v", alerts)
	}
}

// TestECCFieldsJSON pins the export shape: the delta fields are omitempty (a
// cluster that never saw an increment exports nothing new), and the sample
// count describes the collector's window, so it never leaves the process.
func TestECCFieldsJSON(t *testing.T) {
	plain := model.NPUChipData{NPUID: 0, Chip: 0, PhyID: 0}
	raw, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"hbm_ecc_single_err_delta", "hbm_ecc_double_err_delta", "HBMEccDeltaSamples"} {
		if strings.Contains(string(raw), absent) {
			t.Errorf("a zero-delta chip exports %q: %s", absent, raw)
		}
	}

	withDelta := model.NPUChipData{NPUID: 1, Chip: 0, PhyID: 2, HBMEccSingleErrDelta: 4, HBMEccDeltaSamples: 3}
	raw, err = json.Marshal(withDelta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"hbm_ecc_single_err_delta":4`) {
		t.Errorf("the single-bit delta is missing from the export: %s", raw)
	}
	if strings.Contains(string(raw), "hbm_ecc_double_err_delta") {
		t.Errorf("an empty double-bit delta was exported: %s", raw)
	}
	if strings.Contains(string(raw), "HBMEccDeltaSamples") || strings.Contains(string(raw), "delta_samples") {
		t.Errorf("the sample count leaked into the export: %s", raw)
	}
}
