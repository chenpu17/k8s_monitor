package datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

// kubeletSummaryFixture is a /stats/summary response in the exact wire shape
// kubelet uses (keys as frozen in kubelet_types.go). The node exposes both
// per-interface counters and top-level totals, and one pod has no CPU/memory/
// network sections at all, so the parsers' fallback and nil branches are
// exercised too.
const kubeletSummaryFixture = `{
  "node": {
    "nodeName": "node-a",
    "cpu": {"time": "2026-10-20T10:00:00Z", "usageNanoCores": 1500000000},
    "memory": {"time": "2026-10-20T10:00:00Z", "workingSetBytes": 2147483648},
    "network": {
      "time": "2026-10-20T10:00:05Z",
      "rxBytes": 999999,
      "txBytes": 999999,
      "interfaces": [
        {"name": "eth0", "rxBytes": 1000, "txBytes": 2000},
        {"name": "eth1", "rxBytes": 500, "txBytes": 700}
      ]
    }
  },
  "pods": [
    {
      "podRef": {"name": "web-0", "namespace": "default", "uid": "uid-1"},
      "startTime": "2026-10-20T09:00:00Z",
      "cpu": {"time": "2026-10-20T10:00:00Z", "usageNanoCores": 250000000},
      "memory": {"time": "2026-10-20T10:00:00Z", "workingSetBytes": 104857600},
      "network": {
        "time": "2026-10-20T10:00:05Z",
        "interfaces": [{"name": "eth0", "rxBytes": 300, "txBytes": 400}]
      },
      "containers": [
        {
          "name": "app",
          "cpu": {"time": "2026-10-20T10:00:00Z", "usageNanoCores": 250000000},
          "memory": {"time": "2026-10-20T10:00:00Z", "workingSetBytes": 104857600}
        }
      ]
    },
    {
      "podRef": {"name": "idle-0", "namespace": "kube-system", "uid": "uid-2"},
      "containers": [{"name": "app"}]
    }
  ]
}`

// newTestKubeletClient returns a proxy-mode client wired to a test server.
func newTestKubeletClient(server *httptest.Server) *KubeletClient {
	return &KubeletClient{
		httpClient: server.Client(),
		config:     &rest.Config{Host: server.URL},
		logger:     zap.NewNop(),
		useProxy:   true,
	}
}

func newSummaryServer(t *testing.T, requests *int64) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(requests, 1)

		if want := "/api/v1/nodes/node-a/proxy" + summaryAPIPath; r.URL.Path != want {
			t.Errorf("kubelet request path = %q, want %q", r.URL.Path, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(kubeletSummaryFixture))
	}))
}

// TestGetNodeAndPodsMetricsSingleFetch pins the whole point of the combined
// call: one kubelet summary request yields both the node metrics and the pod
// metrics of that node.
func TestGetNodeAndPodsMetricsSingleFetch(t *testing.T) {
	var requests int64
	server := newSummaryServer(t, &requests)
	defer server.Close()

	client := newTestKubeletClient(server)

	cpu, memory, rx, tx, timestamp, podMetrics, err := client.GetNodeAndPodsMetrics(context.Background(), "node-a")
	if err != nil {
		t.Fatalf("GetNodeAndPodsMetrics returned error: %v", err)
	}

	if got := atomic.LoadInt64(&requests); got != 1 {
		t.Errorf("kubelet summary requests = %d, want 1 (node and pod metrics must share one fetch)", got)
	}

	// Node metrics: interface counters win over the top-level totals.
	if cpu != 1500 {
		t.Errorf("cpu millicores = %d, want 1500", cpu)
	}
	if memory != 2147483648 {
		t.Errorf("memory bytes = %d, want 2147483648", memory)
	}
	if rx != 1500 {
		t.Errorf("network rx bytes = %d, want 1500 (sum of interfaces)", rx)
	}
	if tx != 2700 {
		t.Errorf("network tx bytes = %d, want 2700 (sum of interfaces)", tx)
	}
	wantTimestamp := time.Date(2026, 10, 20, 10, 0, 5, 0, time.UTC)
	if !timestamp.Equal(wantTimestamp) {
		t.Errorf("network timestamp = %s, want %s (kubelet-provided)", timestamp, wantTimestamp)
	}

	// Pod metrics: every pod in the response is present, keyed namespace/name.
	for _, key := range []string{"default/web-0", "kube-system/idle-0"} {
		if _, ok := podMetrics[key]; !ok {
			t.Errorf("pod metrics missing key %q (got %d entries)", key, len(podMetrics))
		}
	}
	if len(podMetrics) != 2 {
		t.Errorf("pod metrics entries = %d, want 2", len(podMetrics))
	}

	web := podMetrics["default/web-0"]
	if web == nil {
		t.Fatal("pod metrics for default/web-0 missing")
	}
	if web.Name != "web-0" || web.Namespace != "default" {
		t.Errorf("pod identity = %s/%s, want default/web-0", web.Namespace, web.Name)
	}
	if web.CPUUsage != 250 || web.MemoryUsage != 104857600 {
		t.Errorf("pod cpu/memory = %d/%d, want 250/104857600", web.CPUUsage, web.MemoryUsage)
	}
	if web.NetworkRxBytes != 300 || web.NetworkTxBytes != 400 {
		t.Errorf("pod network rx/tx = %d/%d, want 300/400", web.NetworkRxBytes, web.NetworkTxBytes)
	}
	if !web.NetworkTimestamp.Equal(wantTimestamp) {
		t.Errorf("pod network timestamp = %s, want %s", web.NetworkTimestamp, wantTimestamp)
	}
	if len(web.ContainerStates) != 1 {
		t.Fatalf("container states = %d, want 1", len(web.ContainerStates))
	}
	if web.ContainerStates[0].Name != "app" ||
		web.ContainerStates[0].CPUUsage != 250 ||
		web.ContainerStates[0].MemoryUsage != 104857600 {
		t.Errorf("container state = %+v, want app with 250 millicores / 104857600 bytes", web.ContainerStates[0])
	}

	// A pod without cpu/memory/network sections must yield zeros, not a missing entry.
	idle := podMetrics["kube-system/idle-0"]
	if idle == nil {
		t.Fatal("pod metrics for kube-system/idle-0 missing")
	}
	if idle.CPUUsage != 0 || idle.MemoryUsage != 0 || idle.NetworkRxBytes != 0 || idle.NetworkTxBytes != 0 {
		t.Errorf("pod without metrics sections = %+v, want all zero", idle)
	}
	if !idle.NetworkTimestamp.IsZero() {
		t.Errorf("pod without network section timestamp = %s, want zero time", idle.NetworkTimestamp)
	}
}

// TestExtractMetricsHandleMissingSections covers the nil-safe branches of the
// pure parsers shared by the combined call.
func TestExtractMetricsHandleMissingSections(t *testing.T) {
	summary := &KubeletSummary{}

	cpu, memory, rx, tx, timestamp := extractNodeMetrics(summary)
	if cpu != 0 || memory != 0 || rx != 0 || tx != 0 {
		t.Errorf("extractNodeMetrics on empty summary = %d/%d/%d/%d, want all zero", cpu, memory, rx, tx)
	}
	if !timestamp.IsZero() {
		t.Errorf("extractNodeMetrics timestamp = %s, want zero time", timestamp)
	}

	podMetrics := extractPodMetrics(summary)
	if podMetrics == nil {
		t.Fatal("extractPodMetrics on empty summary returned nil map, want empty map")
	}
	if len(podMetrics) != 0 {
		t.Errorf("extractPodMetrics on empty summary = %d entries, want 0", len(podMetrics))
	}
}

// TestEnrichWithKubeletMetricsFetchesSummaryOncePerNode is the defect probe for
// the doubled request volume: enrichment used to call the kubelet twice per
// node (once for the node metrics, once for the pods on it), even though both
// come from the same summary response. One node must now cost exactly one
// request, and that single response must feed both the node and its pods.
func TestEnrichWithKubeletMetricsFetchesSummaryOncePerNode(t *testing.T) {
	var requests int64
	server := newSummaryServer(t, &requests)
	defer server.Close()

	dataSource := NewAggregatedDataSource(nil, newTestKubeletClient(server), zap.NewNop(), 2)

	nodes := []*model.NodeData{{
		Name:           "node-a",
		CPUAllocatable: 4000,
		MemAllocatable: 8 * 1024 * 1024 * 1024,
		PodAllocatable: 110,
	}}
	pods := []*model.PodData{
		{
			Name:            "web-0",
			Namespace:       "default",
			Node:            "node-a",
			ContainerStates: []model.ContainerState{{Name: "app"}},
		},
		{
			Name:            "idle-0",
			Namespace:       "kube-system",
			Node:            "node-a",
			ContainerStates: []model.ContainerState{{Name: "app"}},
		},
	}

	dataSource.enrichWithKubeletMetrics(context.Background(), nodes, pods)

	if got := atomic.LoadInt64(&requests); got != 1 {
		t.Errorf("kubelet summary requests for 1 node = %d, want 1", got)
	}

	node := nodes[0]
	if !node.HasKubeletMetrics {
		t.Fatalf("node HasKubeletMetrics = false, kubelet error = %q", node.KubeletError)
	}
	if node.CPUUsage != 1500 || node.MemoryUsage != 2147483648 {
		t.Errorf("node cpu/memory = %d/%d, want 1500/2147483648", node.CPUUsage, node.MemoryUsage)
	}
	if node.NetworkRxBytes != 1500 || node.NetworkTxBytes != 2700 {
		t.Errorf("node network rx/tx = %d/%d, want 1500/2700", node.NetworkRxBytes, node.NetworkTxBytes)
	}
	if node.PodCount != 2 {
		t.Errorf("node pod count = %d, want 2", node.PodCount)
	}

	web := pods[0]
	if web.CPUUsage != 250 || web.MemoryUsage != 104857600 {
		t.Errorf("web-0 cpu/memory = %d/%d, want 250/104857600", web.CPUUsage, web.MemoryUsage)
	}
	if len(web.ContainerStates) != 1 || web.ContainerStates[0].CPUUsage != 250 {
		t.Errorf("web-0 container states = %+v, want app at 250 millicores", web.ContainerStates)
	}

	idle := pods[1]
	if idle.CPUUsage != 0 || idle.MemoryUsage != 0 {
		t.Errorf("idle-0 cpu/memory = %d/%d, want 0/0", idle.CPUUsage, idle.MemoryUsage)
	}
}

// TestEnrichWithKubeletMetricsMarksNodeOnFetchFailure keeps the failure path
// observable: a failing summary fetch must leave the node without kubelet
// metrics and record the error.
func TestEnrichWithKubeletMetricsMarksNodeOnFetchFailure(t *testing.T) {
	var requests int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		http.Error(w, "kubelet is down", http.StatusInternalServerError)
	}))
	defer server.Close()

	dataSource := NewAggregatedDataSource(nil, newTestKubeletClient(server), zap.NewNop(), 2)

	nodes := []*model.NodeData{{Name: "node-a"}}
	pods := []*model.PodData{{Name: "web-0", Namespace: "default", Node: "node-a"}}

	dataSource.enrichWithKubeletMetrics(context.Background(), nodes, pods)

	if got := atomic.LoadInt64(&requests); got != 1 {
		t.Errorf("kubelet summary requests on failure = %d, want 1", got)
	}
	if nodes[0].HasKubeletMetrics {
		t.Error("node HasKubeletMetrics = true after a failed summary fetch")
	}
	if nodes[0].KubeletError == "" {
		t.Error("node KubeletError is empty after a failed summary fetch")
	}
	if nodes[0].CPUUsage != 0 || nodes[0].MemoryUsage != 0 {
		t.Errorf("node usage = %d/%d after failure, want 0/0", nodes[0].CPUUsage, nodes[0].MemoryUsage)
	}
}
