package datasource

import (
	"context"
	"testing"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

func TestBuildClusterSummary(t *testing.T) {
	logger := zap.NewNop()

	// Create test data
	nodes := []*model.NodeData{
		{Name: "node1", Status: "Ready"},
		{Name: "node2", Status: "Ready"},
		{Name: "node3", Status: "NotReady"},
	}

	pods := []*model.PodData{
		{Name: "pod1", Phase: "Running"},
		{Name: "pod2", Phase: "Running"},
		{Name: "pod3", Phase: "Pending"},
		{Name: "pod4", Phase: "Failed"},
	}

	events := []*model.EventData{
		{Type: "Warning", Reason: "Test1"},
		{Type: "Error", Reason: "Test2"},
		{Type: "Normal", Reason: "Test3"},
	}

	// Create aggregated data source (without actual clients)
	agg := &AggregatedDataSource{
		logger: logger,
	}

	// Build summary
	summary := agg.buildClusterSummary(nodes, pods, events, nil, nil, nil)

	// Verify node counts
	if summary.TotalNodes != 3 {
		t.Errorf("Expected 3 total nodes, got %d", summary.TotalNodes)
	}
	if summary.ReadyNodes != 2 {
		t.Errorf("Expected 2 ready nodes, got %d", summary.ReadyNodes)
	}
	if summary.NotReadyNodes != 1 {
		t.Errorf("Expected 1 not ready node, got %d", summary.NotReadyNodes)
	}

	// Verify pod counts
	if summary.TotalPods != 4 {
		t.Errorf("Expected 4 total pods, got %d", summary.TotalPods)
	}
	if summary.RunningPods != 2 {
		t.Errorf("Expected 2 running pods, got %d", summary.RunningPods)
	}
	if summary.PendingPods != 1 {
		t.Errorf("Expected 1 pending pod, got %d", summary.PendingPods)
	}
	if summary.FailedPods != 1 {
		t.Errorf("Expected 1 failed pod, got %d", summary.FailedPods)
	}

	// Verify event counts
	if summary.TotalEvents != 3 {
		t.Errorf("Expected 3 total events, got %d", summary.TotalEvents)
	}
	if summary.WarningEvents != 1 {
		t.Errorf("Expected 1 warning event, got %d", summary.WarningEvents)
	}
	if summary.ErrorEvents != 1 {
		t.Errorf("Expected 1 error event, got %d", summary.ErrorEvents)
	}
}

func TestKubeletSummaryParsing(t *testing.T) {
	// Test that kubelet summary types are properly defined
	var summary KubeletSummary

	// Should have Node and Pods fields
	summary.Node = Node{
		NodeName: "test-node",
	}

	summary.Pods = []Pod{
		{
			PodRef: PodReference{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	if summary.Node.NodeName != "test-node" {
		t.Errorf("Expected node name 'test-node', got '%s'", summary.Node.NodeName)
	}

	if len(summary.Pods) != 1 {
		t.Errorf("Expected 1 pod, got %d", len(summary.Pods))
	}

	if summary.Pods[0].PodRef.Name != "test-pod" {
		t.Errorf("Expected pod name 'test-pod', got '%s'", summary.Pods[0].PodRef.Name)
	}
}

func TestAggregatedDataSourceCreation(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock API Server client
	apiServer := &mockDataSource{}

	// Create aggregated data source
	agg := NewAggregatedDataSource(apiServer, nil, logger, 10)

	if agg == nil {
		t.Error("Expected non-nil aggregated data source")
	}

	if agg.Name() != "Aggregated" {
		t.Errorf("Expected name 'Aggregated', got '%s'", agg.Name())
	}
}

// mockDataSource is a simple mock for testing
type mockDataSource struct{}

func (m *mockDataSource) GetNodes(ctx context.Context) ([]*model.NodeData, error) {
	return []*model.NodeData{}, nil
}

func (m *mockDataSource) GetPods(ctx context.Context, namespace string) ([]*model.PodData, error) {
	return []*model.PodData{}, nil
}

func (m *mockDataSource) GetEvents(ctx context.Context, namespace string, eventTypes []string, limit int) ([]*model.EventData, error) {
	return []*model.EventData{}, nil
}

func (m *mockDataSource) Name() string {
	return "Mock"
}

func (m *mockDataSource) Close() error {
	return nil
}
