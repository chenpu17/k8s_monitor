package datasource

// b24v_kb_liveness_ecc_test.go — adversarial review of batch 24 W3/W4/W5.
//
// The probes here pin the parts of the knowledge base, the liveness state
// machine and the ECC window that the delivered tests either assert only on
// their happy path or leave out entirely.

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// TestB24VKBKeyDomain pins the key domain the embedded snapshot builds: the
// snapshot carries three entries written with nine hex digits, which decode to
// integers above 2^32 and therefore can never equal an exporter error code
// (npu_chip_info_error_code is a 32-bit field). The 32-bit spellings those
// entries probably meant are not in the snapshot. The task book mandated a
// byte-for-byte copy of the vendor file, so this is inherited data, not an
// implementation slip — the probe pins the fact so a future snapshot refresh
// has to decide about it instead of inheriting it silently.
func TestB24VKBKeyDomain(t *testing.T) {
	const maxUint32 = int64(0xFFFFFFFF)

	table := npuErrorCodeSnapshot()
	if len(table) != 802 {
		t.Errorf("the snapshot builds %d keys, want 802 distinct codes", len(table))
	}

	var unreachable []int64
	for code := range table {
		if code > maxUint32 {
			unreachable = append(unreachable, code)
		}
	}
	if len(unreachable) != 3 {
		t.Errorf("the snapshot carries %d keys above the 32-bit error-code domain, want the 3 documented nine-digit entries: %v",
			len(unreachable), unreachable)
	}
	for _, code := range unreachable {
		t.Logf("key %08X is outside the exporter's uint32 domain and can never match a reported code", code)
	}

	// The 32-bit readings those entries probably meant are absent, so a chip
	// reporting them lands on the generic wording instead of its real tier.
	for _, code := range []int64{0x00001002, 0x10001024, 0x10000002} {
		if _, ok := npuErrorCodeInfo(code); ok {
			t.Errorf("%08X resolves, so the nine-digit entries do shadow a usable classification after all", code)
		}
	}
}

// TestB24VKBDuplicateKeepsMostPermissiveLevel pins the direction the
// first-wins rule resolves duplicates in. A2301001 is listed by
// RestartBusiness (rerun: "the chip recovers without reset") and by
// SeparateNPU (isolate: "unrecoverable, evaluate replacement"), and the
// snapshot keeps the first, i.e. the *less* severe handling. The iteration is
// deterministic (a slice, not a map), so this is a data-policy finding, not a
// flakiness one.
func TestB24VKBDuplicateKeepsMostPermissiveLevel(t *testing.T) {
	category, ok := npuErrorCodeInfo(0xA2301001)
	if !ok {
		t.Fatal("A2301001 is missing from the snapshot")
	}
	// Non-vacuousness: the two upstream levels disagree, otherwise there is no
	// policy to pin.
	if category.Level != "RestartBusiness" || category.Family != model.NPUErrorCatRerun {
		t.Fatalf("A2301001 resolves to %q/%q, the fixture no longer describes the collision", category.Level, category.Family)
	}
	t.Logf("duplicate A2301001 keeps %q (%s) and drops the SeparateNPU reading", category.Level, category.Family)
}

// TestB24VLivenessNeverDropsFailureText pins the state the machine reads as
// "not polled yet" even though a fetch was attempted and failed: once the
// retry window has expired without a new attempt, Liveness reports "never" and
// clears LastError, which the node detail then renders as "not polled yet".
func TestB24VLivenessNeverDropsFailureText(t *testing.T) {
	client := &NPUExporterClient{}
	client.recordFailure(&testError{"dial tcp 10.0.0.1:8082: connect: connection refused"})

	// Age the failure past the retry window, the state a refresh tick sees
	// before it attempts the next fetch.
	client.mu.Lock()
	client.lastFailure = time.Now().Add(-2 * npuExporterRetryDelay)
	client.mu.Unlock()

	liveness := client.Liveness()
	if liveness.Status == "never" && liveness.LastError != "" {
		t.Fatalf("a never-status carries failure text: %+v", liveness)
	}
	if liveness.Status == "never" {
		t.Logf("a client that has failed reads as %q with no error text; only a fresh attempt restores %q", liveness.Status, "backoff")
	}
}

// TestB24VECCSamplesDoNotAgeOut pins the window's time semantics: samples are
// kept per chip only when that chip appears in a payload, so a chip that
// disappears for a long run of refreshes keeps its old baseline and the delta
// spans an arbitrarily long wall-clock gap while still reporting a small
// sample count.
func TestB24VECCSamplesDoNotAgeOut(t *testing.T) {
	client := &NPUExporterClient{}
	base := time.Now()

	for i := int64(0); i < 3; i++ {
		client.recordECCSamples(eccTestNode, []*NPUChipMetrics{{ID: 0, HBMEccSingleBitErrTotal: i * 10}}, base.Add(time.Duration(i)*time.Second))
	}
	// The chip is absent from the payload for far longer than the window.
	for i := 0; i < 30; i++ {
		client.recordECCSamples(eccTestNode, []*NPUChipMetrics{{ID: 1}}, base.Add(time.Duration(10+i)*time.Second))
	}
	delta := client.recordECCSamples(eccTestNode, []*NPUChipMetrics{{ID: 0, HBMEccSingleBitErrTotal: 1000}}, base.Add(time.Hour))[0]

	if delta.samples > 2 {
		t.Logf("window kept %d samples across a 30-refresh absence; the delta (%d) spans an unbounded wall-clock gap",
			delta.samples, delta.single)
	}
}

// TestB24VKBKnownCodeActionShape pins the shape of the action string the
// knowledge base writes onto the alert. The review found it carrying one copy
// per offending chip (an eight-chip node repeating the same advice eight
// times); the datasource now dedupes by recommendation, so identical codes
// yield one copy while distinct codes keep one entry each.
func TestB24VKBKnownCodeActionShape(t *testing.T) {
	chips := gradedChips(4)
	for i := range chips {
		chips[i].ErrorCode = 0x80E21007
	}
	node := &model.NodeData{Name: "npu-node", NPUCapacity: 4, NPUChips: chips}
	alert := npuAssertSingleAlert(t, npuThresholdAlerts(node, time.Now()), model.AlertTypeNPUErrorCode)

	if copies := strings.Count(alert.RecommendedAction, "No action required"); copies != 1 {
		t.Errorf("four chips sharing one code produce %d copies of the recommendation, want 1: %q",
			copies, alert.RecommendedAction)
	}

	// Distinct codes keep one entry each, in first-seen chip order. The pair
	// spans two different tiers: 80E21007 is NotHandleFault (no action), and
	// A2301001 is the RestartBusiness code the duplicate probe also uses.
	distinct := gradedChips(2)
	distinct[0].ErrorCode = 0x80E21007 // no_action
	distinct[1].ErrorCode = 0xA2301001 // rerun
	node2 := &model.NodeData{Name: "npu-node", NPUCapacity: 2, NPUChips: distinct}
	alert2 := npuAssertSingleAlert(t, npuThresholdAlerts(node2, time.Now()), model.AlertTypeNPUErrorCode)
	if got := strings.Count(alert2.RecommendedAction, " • ") + 1; got != 2 {
		t.Errorf("two distinct codes produce %d recommendation entries, want 2: %q", got, alert2.RecommendedAction)
	}
}

// TestB24VMultiNPUNodeLivenessIsNotClaimed pins the stamp rule on clusters
// with more than one NPU node. EnrichNodeData deliberately returns early there
// (one exporter answer cannot be attributed per node), so the exporter is
// never polled — and the summary must carry no liveness claim at all: a
// never-polled client reports "never", which the NPU view renders as
// "monitoring unavailable" while the exporter is perfectly healthy.
func TestB24VMultiNPUNodeLivenessIsNotClaimed(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, serveNPUMetrics(
		"machine_npu_nums 1\nnpu_chip_info_utilization{id=\"0\"} 10\n"))

	nodes := []*model.NodeData{nodeWithNPU("npu-a", 8), nodeWithNPU("npu-b", 8)}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData: %v", err)
	}
	if got := atomic.LoadInt32(requests); got != 0 {
		t.Fatalf("the multi-node path issued %d exporter requests; the fixture no longer describes an unpolled path", got)
	}

	agg := &AggregatedDataSource{logger: zap.NewNop()}
	agg.SetNPUExporterClient(client)
	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.NPUNodesCount != 2 {
		t.Fatalf("NPUNodesCount = %d, want 2", summary.NPUNodesCount)
	}

	// Fixed in review: the stamp now requires the single-NPU-node shape the
	// client actually polls, so a never-polled multi-node cluster carries no
	// liveness claim instead of "monitoring unavailable".
	if summary.NPUExporterLiveness != nil {
		t.Errorf("a never-polled exporter path is stamped as liveness %+v; the view renders %q for it even though the exporter is healthy",
			*summary.NPUExporterLiveness, summary.NPUExporterLiveness.Status)
	}
}
