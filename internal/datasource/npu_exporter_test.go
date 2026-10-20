package datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

// npuExporterTestMetrics is a representative NPU-Exporter /metrics payload with
// two healthy chips (IDs 0 and 1) on a single node.
const npuExporterTestMetrics = `# HELP machine_npu_nums Number of NPUs on this machine
# TYPE machine_npu_nums gauge
machine_npu_nums 1
npu_chip_info_utilization{id="0",model_name="Ascend910B",pcie_bus_info="0000:01:00.0"} 50
npu_chip_info_utilization{id="1",model_name="Ascend910B",pcie_bus_info="0000:02:00.0"} 70
npu_chip_info_temperature{id="0"} 45
npu_chip_info_temperature{id="1"} 55
npu_chip_info_hbm_used_memory{id="0"} 100
npu_chip_info_hbm_used_memory{id="1"} 300
npu_chip_info_hbm_total_memory{id="0"} 1000
npu_chip_info_hbm_total_memory{id="1"} 1000
npu_chip_info_power{id="0"} 100.5
npu_chip_info_power{id="1"} 199.5
npu_chip_info_health_status{id="0"} 1
npu_chip_info_health_status{id="1"} 1
`

// newTestNPUExporterClient returns a client pointing at a fake NPU-Exporter
// endpoint, counting the requests that reach it.
func newTestNPUExporterClient(t *testing.T, handler http.HandlerFunc) (*NPUExporterClient, *int32) {
	t.Helper()

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	client, err := NewNPUExporterClient(&rest.Config{Host: "https://127.0.0.1:6443"}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewNPUExporterClient() error = %v", err)
	}
	client.SetEndpoint(srv.URL)

	return client, &requests
}

func serveNPUMetrics(payload string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(payload))
	}
}

// nodeWithNPU builds a node that looks like an Ascend node to the client.
func nodeWithNPU(name string, npuCapacity int64) *model.NodeData {
	return &model.NodeData{
		Name:           name,
		NPUCapacity:    npuCapacity,
		NPUAllocatable: npuCapacity,
	}
}

// hasNPUEnrichment reports whether EnrichNodeData wrote any NPU metric to the node.
func hasNPUEnrichment(node *model.NodeData) bool {
	return node.NPUChips != nil ||
		node.NPUUtilization != 0 ||
		node.NPUMemoryTotal != 0 ||
		node.NPUMemoryUsed != 0 ||
		node.NPUTemperature != 0 ||
		node.NPUPower != 0 ||
		node.NPUHealthStatus != "" ||
		node.NPUErrorCount != 0 ||
		!node.NPUMetricsTime.IsZero()
}

func TestEnrichNodeDataSingleNode(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, serveNPUMetrics(npuExporterTestMetrics))

	node := nodeWithNPU("npu-node-1", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if got := atomic.LoadInt32(requests); got != 1 {
		t.Errorf("request count = %d, want 1 (availability probe must reuse its response)", got)
	}

	if len(node.NPUChips) != 2 {
		t.Fatalf("len(NPUChips) = %d, want 2", len(node.NPUChips))
	}

	// Chip order is by chip ID, so chips[0] is id=0 and chips[1] is id=1.
	if node.NPUChips[0].PhyID != 0 || node.NPUChips[1].PhyID != 1 {
		t.Errorf("chip phy IDs = [%d %d], want [0 1]", node.NPUChips[0].PhyID, node.NPUChips[1].PhyID)
	}
	if node.NPUChips[0].NPUID != 0 || node.NPUChips[0].Chip != 0 {
		t.Errorf("chip 0 NPUID/Chip = %d/%d, want 0/0", node.NPUChips[0].NPUID, node.NPUChips[0].Chip)
	}
	if node.NPUChips[1].NPUID != 0 || node.NPUChips[1].Chip != 1 {
		t.Errorf("chip 1 NPUID/Chip = %d/%d, want 0/1", node.NPUChips[1].NPUID, node.NPUChips[1].Chip)
	}
	if node.NPUChips[0].BusID != "0000:01:00.0" {
		t.Errorf("chip 0 BusID = %q, want %q", node.NPUChips[0].BusID, "0000:01:00.0")
	}
	if node.NPUChips[0].AICore != 50 || node.NPUChips[1].AICore != 70 {
		t.Errorf("chip AICore = [%d %d], want [50 70]", node.NPUChips[0].AICore, node.NPUChips[1].AICore)
	}
	if node.NPUChips[0].Temp != 45 || node.NPUChips[1].Temp != 55 {
		t.Errorf("chip Temp = [%d %d], want [45 55]", node.NPUChips[0].Temp, node.NPUChips[1].Temp)
	}
	for _, chip := range node.NPUChips {
		if chip.Health != "OK" {
			t.Errorf("chip %d Health = %q, want %q", chip.PhyID, chip.Health, "OK")
		}
	}

	// Node-level aggregates: mean utilization (50+70)/2, mean temperature (45+55)/2,
	// summed power 100.5+199.5, HBM 400/2000 used of total.
	if node.NPUUtilization != 60 {
		t.Errorf("NPUUtilization = %v, want 60", node.NPUUtilization)
	}
	if node.NPUTemperature != 50 {
		t.Errorf("NPUTemperature = %v, want 50", node.NPUTemperature)
	}
	if node.NPUPower != 300 {
		t.Errorf("NPUPower = %v, want 300", node.NPUPower)
	}
	if node.NPUMemoryTotal != 2000*1024*1024 {
		t.Errorf("NPUMemoryTotal = %v, want %v", node.NPUMemoryTotal, int64(2000*1024*1024))
	}
	if node.NPUMemoryUsed != 400*1024*1024 {
		t.Errorf("NPUMemoryUsed = %v, want %v", node.NPUMemoryUsed, int64(400*1024*1024))
	}
	if node.NPUMemoryUtil != 20 {
		t.Errorf("NPUMemoryUtil = %v, want 20", node.NPUMemoryUtil)
	}
	if node.NPUHealthStatus != "Healthy" {
		t.Errorf("NPUHealthStatus = %q, want %q", node.NPUHealthStatus, "Healthy")
	}
	if node.NPUErrorCount != 0 {
		t.Errorf("NPUErrorCount = %d, want 0", node.NPUErrorCount)
	}
	if node.NPUMetricsTime.IsZero() {
		t.Error("NPUMetricsTime is zero, want the time of enrichment")
	}
}

func TestEnrichNodeDataUnhealthyChip(t *testing.T) {
	const payload = `machine_npu_nums 1
npu_chip_info_utilization{id="0"} 10
npu_chip_info_health_status{id="0"} 0
npu_chip_info_utilization{id="1"} 20
npu_chip_info_health_status{id="1"} 1
`
	client, _ := newTestNPUExporterClient(t, serveNPUMetrics(payload))

	node := nodeWithNPU("npu-node-1", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if node.NPUHealthStatus != "Warning" {
		t.Errorf("NPUHealthStatus = %q, want %q", node.NPUHealthStatus, "Warning")
	}
	if node.NPUErrorCount != 1 {
		t.Errorf("NPUErrorCount = %d, want 1", node.NPUErrorCount)
	}
	if node.NPUChips[0].Health != "Error" {
		t.Errorf("chip 0 Health = %q, want %q", node.NPUChips[0].Health, "Error")
	}
	if node.NPUChips[1].Health != "OK" {
		t.Errorf("chip 1 Health = %q, want %q", node.NPUChips[1].Health, "OK")
	}
}

// TestEnrichNodeDataMultiNodeSkipsEnrichment covers the fabricated-data defect:
// metrics obtained through the ClusterIP Service come from a single exporter
// pod, so they must not be attributed to several nodes.
func TestEnrichNodeDataMultiNodeSkipsEnrichment(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, serveNPUMetrics(npuExporterTestMetrics))

	nodes := []*model.NodeData{
		nodeWithNPU("npu-node-1", 1),
		nodeWithNPU("npu-node-2", 1),
		nodeWithNPU("npu-node-3", 2),
	}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	for _, node := range nodes {
		if hasNPUEnrichment(node) {
			t.Errorf("node %s was enriched in a multi-NPU-node cluster: chips=%d util=%v temp=%v health=%q",
				node.Name, len(node.NPUChips), node.NPUUtilization, node.NPUTemperature, node.NPUHealthStatus)
		}
	}

	if got := atomic.LoadInt32(requests); got != 0 {
		t.Errorf("request count = %d, want 0 (multi-node metrics cannot be attributed, so nothing to fetch)", got)
	}
}

// TestEnrichNodeDataNoNPUNodesSkipsRequest covers the wasted-request defect:
// clusters without NPU nodes must not query NPU-Exporter at all.
func TestEnrichNodeDataNoNPUNodesSkipsRequest(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, serveNPUMetrics(npuExporterTestMetrics))

	nodes := []*model.NodeData{
		{Name: "cpu-node-1"},
		{Name: "cpu-node-2"},
	}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if got := atomic.LoadInt32(requests); got != 0 {
		t.Errorf("request count = %d, want 0 for a cluster without NPU nodes", got)
	}
	for _, node := range nodes {
		if hasNPUEnrichment(node) {
			t.Errorf("node %s was enriched although it has no NPU capacity", node.Name)
		}
	}
}

// TestEnrichNodeDataSteadyStateRequests checks the request pattern: one request
// per refresh once the exporter is known to be up (no duplicated fetch).
func TestEnrichNodeDataSteadyStateRequests(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, serveNPUMetrics(npuExporterTestMetrics))

	for i := 0; i < 3; i++ {
		node := nodeWithNPU("npu-node-1", 1)
		if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
			t.Fatalf("EnrichNodeData() #%d error = %v", i, err)
		}
		if len(node.NPUChips) != 2 {
			t.Fatalf("refresh #%d: len(NPUChips) = %d, want 2", i, len(node.NPUChips))
		}
	}

	if got := atomic.LoadInt32(requests); got != 3 {
		t.Errorf("request count = %d, want 3 (one per refresh)", got)
	}
}

// TestCheckAvailabilityBackoff verifies that a failing exporter is retried with
// backoff instead of being hammered on every refresh.
func TestCheckAvailabilityBackoff(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	ctx := context.Background()
	if err := client.CheckAvailability(ctx); err == nil {
		t.Fatal("CheckAvailability() error = nil, want failure for HTTP 500")
	}
	if client.IsAvailable() {
		t.Error("IsAvailable() = true after a failed request, want false")
	}

	// Second call must be short-circuited by the retry backoff.
	if err := client.CheckAvailability(ctx); err == nil {
		t.Fatal("CheckAvailability() error = nil during backoff, want backoff error")
	}
	if got := atomic.LoadInt32(requests); got != 1 {
		t.Errorf("request count = %d, want 1 (backoff must suppress the retry)", got)
	}

	// EnrichNodeData must also stay quiet during the backoff window.
	node := nodeWithNPU("npu-node-1", 1)
	if err := client.EnrichNodeData(ctx, []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}
	if hasNPUEnrichment(node) {
		t.Error("node was enriched although the exporter is in backoff")
	}
	if got := atomic.LoadInt32(requests); got != 1 {
		t.Errorf("request count = %d, want 1 (EnrichNodeData must respect the backoff)", got)
	}
}

// TestNPUExporterClientConcurrentAccess exercises the client from several
// goroutines at once (refresh ticker, ForceRefresh and UI lookups may overlap).
// Run with -race to detect unsynchronized access to the client state.
func TestNPUExporterClientConcurrentAccess(t *testing.T) {
	client, _ := newTestNPUExporterClient(t, serveNPUMetrics(npuExporterTestMetrics))

	const goroutines = 8
	const iterations = 20

	var wg sync.WaitGroup
	start := make(chan struct{})
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				// Each goroutine owns its node slice, as every refresh cycle does.
				node := nodeWithNPU("npu-node-1", 1)
				_ = client.EnrichNodeData(context.Background(), []*model.NodeData{node})
				_ = client.IsAvailable()
				_ = client.CheckAvailability(context.Background())
			}
		}()
	}

	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent access did not finish within 30s")
	}
}
