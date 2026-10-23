package datasource

// b24_wp4_liveness_summary_test.go — batch 24 W4: the exporter liveness state
// as the summary carries it.
//
// Liveness is summary-scoped on purpose: one ClusterIP service feeds a
// single-NPU-node cluster, and multi-NPU-node clusters are never enriched, so a
// per-node field would be a claim nobody can make.

import (
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// TestBuildClusterSummaryLivenessPresence pins when the liveness field is
// stamped: it needs both an exporter client and an NPU node, because without
// either the exporter path is never walked.
func TestBuildClusterSummaryLivenessPresence(t *testing.T) {
	npuNodes := []*model.NodeData{{Name: "npu-node", Status: "Ready", NPUCapacity: 8}}
	cpuNodes := []*model.NodeData{{Name: "cpu-node", Status: "Ready"}}

	t.Run("client and NPU node", func(t *testing.T) {
		agg := &AggregatedDataSource{logger: zap.NewNop()}
		client := &NPUExporterClient{}
		client.recordSuccess()
		agg.SetNPUExporterClient(client)

		summary := agg.buildClusterSummary(npuNodes, nil, nil, nil, nil, nil, workloadLists{})
		if summary.NPUExporterLiveness == nil {
			t.Fatal("the summary carries no liveness state")
		}
		if summary.NPUExporterLiveness.Status != "ok" {
			t.Errorf("Status = %q, want ok", summary.NPUExporterLiveness.Status)
		}
	})

	t.Run("no client", func(t *testing.T) {
		agg := &AggregatedDataSource{logger: zap.NewNop()}
		summary := agg.buildClusterSummary(npuNodes, nil, nil, nil, nil, nil, workloadLists{})
		if summary.NPUExporterLiveness != nil {
			t.Errorf("a summary without a client carries liveness %+v", summary.NPUExporterLiveness)
		}
	})

	t.Run("no NPU node", func(t *testing.T) {
		agg := &AggregatedDataSource{logger: zap.NewNop()}
		agg.SetNPUExporterClient(&NPUExporterClient{})
		summary := agg.buildClusterSummary(cpuNodes, nil, nil, nil, nil, nil, workloadLists{})
		if summary.NPUExporterLiveness != nil {
			t.Errorf("a cluster without NPU nodes carries liveness %+v", summary.NPUExporterLiveness)
		}
	})
}

// TestBuildClusterSummaryLivenessTracksBackoff pins that the stamped state is
// the live one: a failure surfaces as backoff with the raw error, and a later
// success clears it.
func TestBuildClusterSummaryLivenessTracksBackoff(t *testing.T) {
	agg := &AggregatedDataSource{logger: zap.NewNop()}
	client := &NPUExporterClient{}
	agg.SetNPUExporterClient(client)
	nodes := []*model.NodeData{{Name: "npu-node", Status: "Ready", NPUCapacity: 8}}

	client.recordFailure(errTestExporterDown)
	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.NPUExporterLiveness == nil || summary.NPUExporterLiveness.Status != "backoff" {
		t.Fatalf("liveness = %+v, want backoff", summary.NPUExporterLiveness)
	}
	if summary.NPUExporterLiveness.LastError == "" {
		t.Error("the backoff state carries no error text")
	}

	client.recordSuccess()
	summary = agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.NPUExporterLiveness.Status != "ok" {
		t.Errorf("liveness = %+v after recovery, want ok", summary.NPUExporterLiveness)
	}
}

// errTestExporterDown stands in for a transport error.
var errTestExporterDown = &testError{"dial tcp 10.0.0.1:8082: connect: connection refused"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
