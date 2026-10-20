package datasource

import (
	"context"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
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
		// Pods carrying the legacy label heuristics must not influence the
		// workload counts anymore.
		{
			Name:      "pod5",
			Namespace: "default",
			Phase:     "Running",
			Labels: map[string]string{
				"app.kubernetes.io/component": "deployment",
				"app.kubernetes.io/name":      "ghost",
				"job-name":                    "ghost-job",
			},
		},
	}

	events := []*model.EventData{
		{Type: "Warning", Reason: "Test1"},
		{Type: "Error", Reason: "Test2"},
		{Type: "Normal", Reason: "Test3"},
	}

	workloads := workloadLists{
		deployments: []*model.DeploymentData{
			{Name: "deploy1", Namespace: "default"},
			{Name: "deploy2", Namespace: "kube-system"},
		},
		statefulsets: []*model.StatefulSetData{
			{Name: "sts1", Namespace: "default"},
		},
		daemonsets: []*model.DaemonSetData{
			{Name: "ds1", Namespace: "kube-system"},
		},
		jobs: []*model.JobData{
			{Name: "job1", Namespace: "default"},
			{Name: "job2", Namespace: "default"},
			{Name: "job3", Namespace: "default"},
		},
		cronjobs: []*model.CronJobData{
			{Name: "cj1", Namespace: "default"},
		},
	}

	// Create aggregated data source (without actual clients)
	agg := &AggregatedDataSource{
		logger: logger,
	}

	// Build summary
	summary := agg.buildClusterSummary(nodes, pods, events, nil, nil, nil, workloads)

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
	if summary.TotalPods != 5 {
		t.Errorf("Expected 5 total pods, got %d", summary.TotalPods)
	}
	if summary.RunningPods != 3 {
		t.Errorf("Expected 3 running pods, got %d", summary.RunningPods)
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

	// Verify workload counts come from the real lists, not from pod labels
	if summary.TotalDeployments != 2 {
		t.Errorf("Expected 2 deployments, got %d", summary.TotalDeployments)
	}
	if summary.TotalStatefulSets != 1 {
		t.Errorf("Expected 1 statefulset, got %d", summary.TotalStatefulSets)
	}
	if summary.TotalDaemonSets != 1 {
		t.Errorf("Expected 1 daemonset, got %d", summary.TotalDaemonSets)
	}
	if summary.TotalJobs != 3 {
		t.Errorf("Expected 3 jobs, got %d", summary.TotalJobs)
	}
	if summary.TotalCronJobs != 1 {
		t.Errorf("Expected 1 cronjob, got %d", summary.TotalCronJobs)
	}
}

func TestApplyWorkloadCounts(t *testing.T) {
	tests := []struct {
		name     string
		workload workloadLists
		want     model.ClusterSummary
	}{
		{
			name:     "empty lists",
			workload: workloadLists{},
			want:     model.ClusterSummary{},
		},
		{
			name: "mixed workload types",
			workload: workloadLists{
				deployments: []*model.DeploymentData{
					{Name: "d1", Namespace: "default"},
					{Name: "d2", Namespace: "default"},
				},
				statefulsets: []*model.StatefulSetData{
					{Name: "s1", Namespace: "default"},
					{Name: "s2", Namespace: "default"},
					{Name: "s3", Namespace: "default"},
				},
				daemonsets: []*model.DaemonSetData{
					{Name: "ds1", Namespace: "default"},
				},
				jobs: []*model.JobData{
					{Name: "j1", Namespace: "default"},
				},
				cronjobs: []*model.CronJobData{
					{Name: "c1", Namespace: "default"},
					{Name: "c2", Namespace: "default"},
				},
			},
			want: model.ClusterSummary{
				TotalDeployments:  2,
				TotalStatefulSets: 3,
				TotalDaemonSets:   1,
				TotalJobs:         1,
				TotalCronJobs:     2,
			},
		},
		{
			name: "lists scoped to a single namespace",
			workload: workloadLists{
				deployments: []*model.DeploymentData{
					{Name: "d1", Namespace: "team-a"},
					{Name: "d2", Namespace: "team-a"},
				},
				jobs: []*model.JobData{
					{Name: "j1", Namespace: "team-a"},
				},
			},
			want: model.ClusterSummary{
				TotalDeployments: 2,
				TotalJobs:        1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := &model.ClusterSummary{}
			applyWorkloadCounts(summary, tt.workload)

			if summary.TotalDeployments != tt.want.TotalDeployments {
				t.Errorf("TotalDeployments = %d, want %d", summary.TotalDeployments, tt.want.TotalDeployments)
			}
			if summary.TotalStatefulSets != tt.want.TotalStatefulSets {
				t.Errorf("TotalStatefulSets = %d, want %d", summary.TotalStatefulSets, tt.want.TotalStatefulSets)
			}
			if summary.TotalDaemonSets != tt.want.TotalDaemonSets {
				t.Errorf("TotalDaemonSets = %d, want %d", summary.TotalDaemonSets, tt.want.TotalDaemonSets)
			}
			if summary.TotalJobs != tt.want.TotalJobs {
				t.Errorf("TotalJobs = %d, want %d", summary.TotalJobs, tt.want.TotalJobs)
			}
			if summary.TotalCronJobs != tt.want.TotalCronJobs {
				t.Errorf("TotalCronJobs = %d, want %d", summary.TotalCronJobs, tt.want.TotalCronJobs)
			}
		})
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
