package datasource

// The kubelet access denial travels from the diagnostic status into the
// cluster summary: the raw reason (status.Message()) stays the display text
// everywhere (node.KubeletError, summary.KubeletError, hint classification),
// while summary.KubeletErrorKey/Args carry the i18n key so the overview can
// localize the denial. Raw per-node kubelet errors have no key and keep the
// stored-text fallback.

import (
	"errors"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/diagnostic"
	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

// kubeletEnabledSource builds a source whose kubelet client is non-nil, so the
// "metrics disabled (client not initialized)" branch in buildClusterSummary
// stays out of the way and the denial path is what gets asserted.
func kubeletEnabledSource() *AggregatedDataSource {
	agg := &AggregatedDataSource{logger: zap.NewNop()}
	kubeletClient, err := NewKubeletClient(&rest.Config{Host: "https://127.0.0.1:1"}, true, false, zap.NewNop())
	if err != nil {
		panic("NewKubeletClient: " + err.Error())
	}
	agg.kubeletClient = kubeletClient
	return agg
}

func denialStatus() *diagnostic.KubeletAccessStatus {
	// Built the same way shouldSkipKubeletEnrichment caches a failed access
	// review (diagnostic.AccessReviewFailedStatus).
	return diagnostic.AccessReviewFailedStatus(errors.New("sar create forbidden"))
}

// TestSummaryCarriesKubeletDenialKey: with the skip reason written into nodes,
// the summary shows the denial with its key/args while the raw text is
// untouched everywhere.
func TestSummaryCarriesKubeletDenialKey(t *testing.T) {
	agg := kubeletEnabledSource()
	status := denialStatus()
	reason := status.Message()

	nodes := []*model.NodeData{
		{Name: "node-a", Status: "Ready"},
		{Name: "node-b", Status: "Ready"},
	}
	agg.applyKubeletSkipReason(reason, status, nodes, nil)

	// Raw path: node errors carry the raw reason unchanged.
	for _, node := range nodes {
		if node.KubeletError != reason {
			t.Errorf("node %s KubeletError = %q, want raw reason %q", node.Name, node.KubeletError, reason)
		}
	}

	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.KubeletError != reason {
		t.Errorf("summary.KubeletError = %q, want raw reason %q", summary.KubeletError, reason)
	}
	if summary.KubeletErrorKey != status.MessageKey {
		t.Errorf("summary.KubeletErrorKey = %q, want %q", summary.KubeletErrorKey, status.MessageKey)
	}
	if summary.KubeletErrorArgs["Error"] != status.MessageArgs["Error"] {
		t.Errorf("summary.KubeletErrorArgs = %v, want %v", summary.KubeletErrorArgs, status.MessageArgs)
	}
}

// TestSummaryNodeErrorBeatsDenialKey: a genuine per-node kubelet error listed
// first keeps priority and stays keyless (raw fallback in the UI).
func TestSummaryNodeErrorBeatsDenialKey(t *testing.T) {
	agg := kubeletEnabledSource()
	status := denialStatus()
	reason := status.Message()

	nodeErr := "x509: certificate signed by unknown authority"
	nodes := []*model.NodeData{
		{Name: "node-a", Status: "Ready", KubeletError: nodeErr},
		{Name: "node-b", Status: "Ready"},
	}
	agg.applyKubeletSkipReason(reason, status, nodes, nil)

	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.KubeletError != nodeErr {
		t.Errorf("summary.KubeletError = %q, want node error %q", summary.KubeletError, nodeErr)
	}
	if summary.KubeletErrorKey != "" {
		t.Errorf("summary.KubeletErrorKey = %q, want empty (node errors are raw)", summary.KubeletErrorKey)
	}
	if summary.KubeletErrorArgs != nil {
		t.Errorf("summary.KubeletErrorArgs = %v, want nil", summary.KubeletErrorArgs)
	}
}

// TestSummaryDenialKeyWithoutNodes: with no nodes the skip reason lands on the
// summary through the fallback branch and still carries its key/args.
func TestSummaryDenialKeyWithoutNodes(t *testing.T) {
	agg := kubeletEnabledSource()
	status := denialStatus()
	reason := status.Message()
	agg.applyKubeletSkipReason(reason, status, nil, nil)

	summary := agg.buildClusterSummary(nil, nil, nil, nil, nil, nil, workloadLists{})
	if summary.KubeletError != reason {
		t.Errorf("summary.KubeletError = %q, want raw reason %q", summary.KubeletError, reason)
	}
	if summary.KubeletErrorKey != status.MessageKey {
		t.Errorf("summary.KubeletErrorKey = %q, want %q", summary.KubeletErrorKey, status.MessageKey)
	}
}

// TestSkipStatusWithoutKeyStaysRaw: a denial status without a message key (raw
// cluster-provided reason) must not fabricate one.
func TestSkipStatusWithoutKeyStaysRaw(t *testing.T) {
	agg := kubeletEnabledSource()
	status := &diagnostic.KubeletAccessStatus{
		ProxyAllowed: false,
		ProxyMessage: "raw cluster reason",
	}
	reason := status.Message()

	nodes := []*model.NodeData{{Name: "node-a", Status: "Ready"}}
	agg.applyKubeletSkipReason(reason, status, nodes, nil)

	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.KubeletError != reason {
		t.Errorf("summary.KubeletError = %q, want raw reason %q", summary.KubeletError, reason)
	}
	if summary.KubeletErrorKey != "" || summary.KubeletErrorArgs != nil {
		t.Errorf("keyless status produced key/args: %q %v", summary.KubeletErrorKey, summary.KubeletErrorArgs)
	}
}
