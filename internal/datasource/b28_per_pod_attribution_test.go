package datasource

// b28_per_pod_attribution_test.go — batch 28: per-exporter-pod scraping and
// per-node attribution.
//
// NPU-Exporter runs as a DaemonSet whose pods listen on the pod network, so the
// ClusterIP answer comes from one node and carries no node identity. Scraping
// each pod through its own proxy makes every answer attributable to the node
// the pod runs on. These probes pin the attribution, the ECC window isolation
// that chip IDs alone cannot provide, the fallback to the service path and the
// partial-failure semantics.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b28FakeExporter replaces pod discovery and the per-pod scrape, the two seams
// the client offers, so the attribution logic is exercised without an API
// server. Payloads may be rewritten between refreshes to model counter growth.
type b28FakeExporter struct {
	pods     []npuExporterPod
	listErr  error
	payloads map[string]string // pod name -> metrics body
	fetchErr map[string]error  // pod name -> scrape failure

	mu      sync.Mutex
	fetched []string
}

func (f *b28FakeExporter) install(c *NPUExporterClient) {
	c.podLister = func(ctx context.Context) ([]npuExporterPod, error) {
		if f.listErr != nil {
			return nil, f.listErr
		}
		pods := make([]npuExporterPod, len(f.pods))
		copy(pods, f.pods)
		return pods, nil
	}
	c.podFetcher = func(ctx context.Context, pod npuExporterPod) (io.ReadCloser, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.fetched = append(f.fetched, pod.Name)
		if err, ok := f.fetchErr[pod.Name]; ok {
			return nil, err
		}
		body, ok := f.payloads[pod.Name]
		if !ok {
			return nil, fmt.Errorf("no fixture payload for pod %s", pod.Name)
		}
		return io.NopCloser(strings.NewReader(body)), nil
	}
}

func (f *b28FakeExporter) fetchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.fetched)
}

// newB28PerPodClient returns a client that has the API-server shape (proxy mode
// with the default seams) but never performs real I/O: the tests overwrite both
// seams before enriching.
func newB28PerPodClient(t *testing.T) *NPUExporterClient {
	t.Helper()

	client, err := NewNPUExporterClient(&rest.Config{Host: "https://127.0.0.1:6443"}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewNPUExporterClient() error = %v", err)
	}
	return client
}

// newB28ServiceClient returns a client whose REST client points at a fake API
// server, so the service fallback path can be observed end to end. It injects an
// empty pod list, which is the condition the fallback exists for.
func newB28ServiceClient(t *testing.T, handler http.HandlerFunc) (*NPUExporterClient, *int32) {
	t.Helper()

	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	client, err := NewNPUExporterClient(&rest.Config{Host: srv.URL}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewNPUExporterClient() error = %v", err)
	}
	client.podLister = func(ctx context.Context) ([]npuExporterPod, error) { return nil, nil }

	return client, &requests
}

// b28ChipPayload builds a one-chip exporter body. Each value is a parameter so
// a probe can give two nodes the same chip ID and different readings, the shape
// that exposes cross-node contamination.
func b28ChipPayload(chipID, temp int, podName string, eccSingleTotal int64) string {
	return fmt.Sprintf(`machine_npu_nums 1
npu_chip_info_utilization{id="%d",pod_name="%s",namespace="ml",container_name="c"} 30
npu_chip_info_temperature{id="%d"} %d
npu_chip_info_hbm_used_memory{id="%d"} 100
npu_chip_info_hbm_total_memory{id="%d"} 1000
npu_chip_info_health_status{id="%d"} 1
npu_chip_info_hbm_ecc_total_single_bit_error_cnt{id="%d"} %d
`,
		chipID, podName,
		chipID, temp,
		chipID, chipID,
		chipID,
		chipID, eccSingleTotal)
}

// TestB28PerPodAttribution pins the attribution itself: each pod's answer lands
// on the node that pod runs on. Both payloads describe chip 4 with different
// temperatures and owners, so a cross-attribution or a merge of the two answers
// cannot pass.
func TestB28PerPodAttribution(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "npu-exporter-docker-4qtp5", NodeName: "192.168.2.77"},
			{Name: "npu-exporter-docker-5lzxv", NodeName: "192.168.2.27"},
		},
		payloads: map[string]string{
			"npu-exporter-docker-4qtp5": b28ChipPayload(4, 70, "pod-b", 0),
			"npu-exporter-docker-5lzxv": b28ChipPayload(4, 50, "pod-a", 0),
		},
	}
	fake.install(client)

	// Non-vacuousness: the two payloads must differ, otherwise a swap is invisible.
	if fake.payloads["npu-exporter-docker-4qtp5"] == fake.payloads["npu-exporter-docker-5lzxv"] {
		t.Fatal("the fixture payloads are identical, so misattribution cannot be detected")
	}

	nodeA := nodeWithNPU("192.168.2.27", 8)
	nodeB := nodeWithNPU("192.168.2.77", 8)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{nodeA, nodeB}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	for _, tc := range []struct {
		node    *model.NodeData
		owner   string
		temp    int
		nodeWhy string
	}{
		{nodeA, "pod-a", 50, "192.168.2.27 runs npu-exporter-docker-5lzxv"},
		{nodeB, "pod-b", 70, "192.168.2.77 runs npu-exporter-docker-4qtp5"},
	} {
		if len(tc.node.NPUChips) != 1 {
			t.Fatalf("node %s carries %d chips, want 1 (%s)", tc.node.Name, len(tc.node.NPUChips), tc.nodeWhy)
		}
		chip := tc.node.NPUChips[0]
		if chip.PhyID != 4 {
			t.Errorf("node %s chip PhyID = %d, want 4", tc.node.Name, chip.PhyID)
		}
		if chip.Temp != tc.temp || tc.node.NPUTemperature != tc.temp {
			t.Errorf("node %s temp = %d (aggregate %d), want %d (%s)",
				tc.node.Name, chip.Temp, tc.node.NPUTemperature, tc.temp, tc.nodeWhy)
		}
		if chip.OwnerPod != tc.owner {
			t.Errorf("node %s chip owner = %q, want %q (%s)", tc.node.Name, chip.OwnerPod, tc.owner, tc.nodeWhy)
		}
	}

	if nodeA.NPUChips[0].Temp == nodeB.NPUChips[0].Temp {
		t.Error("both nodes report the same temperature, the two answers were merged")
	}
}

// TestB28ECCSampleWindowsArePerNode is the collision probe: every node numbers
// its chips from 0, so a window keyed by chip ID alone mixes two nodes' counters.
// Both nodes report chip 4 with different cumulative totals; after two refreshes
// each node's delta must be its own growth, and the first refresh must leave both
// at one sample and zero delta.
func TestB28ECCSampleWindowsArePerNode(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "exporter-on-node-a", NodeName: "node-a"},
			{Name: "exporter-on-node-b", NodeName: "node-b"},
		},
		payloads: map[string]string{
			"exporter-on-node-a": b28ChipPayload(4, 50, "pod-a", 100),
			"exporter-on-node-b": b28ChipPayload(4, 60, "pod-b", 1000),
		},
	}
	fake.install(client)

	enrich := func() (a, b *model.NodeData) {
		nodeA := nodeWithNPU("node-a", 8)
		nodeB := nodeWithNPU("node-b", 8)
		if err := client.EnrichNodeData(context.Background(), []*model.NodeData{nodeA, nodeB}); err != nil {
			t.Fatalf("EnrichNodeData() error = %v", err)
		}
		if len(nodeA.NPUChips) != 1 || len(nodeB.NPUChips) != 1 {
			t.Fatalf("chips = %d/%d, want 1/1", len(nodeA.NPUChips), len(nodeB.NPUChips))
		}
		return nodeA, nodeB
	}

	// First refresh: one sample per (node, chip), so neither may report a delta.
	// A chip-ID-only window sees node B's 1000 appended to node A's 100 and
	// reports the difference as a session increment.
	a1, b1 := enrich()
	for _, tc := range []struct {
		node *model.NodeData
		want int64
	}{{a1, 100}, {b1, 1000}} {
		chip := tc.node.NPUChips[0]
		if chip.HBMEccDeltaSamples != 1 {
			t.Errorf("node %s first refresh: window holds %d samples, want 1 (the other node's counter leaked in)",
				tc.node.Name, chip.HBMEccDeltaSamples)
		}
		if chip.HBMEccSingleErrDelta != 0 {
			t.Errorf("node %s first refresh: delta = %d, want 0 (baseline for %d)",
				tc.node.Name, chip.HBMEccSingleErrDelta, tc.want)
		}
		if chip.HBMEccSingleErrTotal != tc.want {
			t.Errorf("node %s cumulative total = %d, want %d (passthrough)", tc.node.Name, chip.HBMEccSingleErrTotal, tc.want)
		}
	}

	// Second refresh: each node moved by its own amount.
	fake.payloads["exporter-on-node-a"] = b28ChipPayload(4, 50, "pod-a", 150)
	fake.payloads["exporter-on-node-b"] = b28ChipPayload(4, 60, "pod-b", 1100)

	a2, b2 := enrich()
	if got := a2.NPUChips[0].HBMEccSingleErrDelta; got != 50 {
		t.Errorf("node-a delta = %d, want 50 (150 - 100)", got)
	}
	if got := b2.NPUChips[0].HBMEccSingleErrDelta; got != 100 {
		t.Errorf("node-b delta = %d, want 100 (1100 - 1000)", got)
	}
	for _, node := range []*model.NodeData{a2, b2} {
		if got := node.NPUChips[0].HBMEccDeltaSamples; got != 2 {
			t.Errorf("node %s second refresh: window holds %d samples, want 2", node.Name, got)
		}
	}
}

// TestB28ServiceFallbackWhenNoPods pins design decision 1: an empty pod list
// keeps the service path, which is today's single-NPU-node behavior.
func TestB28ServiceFallbackWhenNoPods(t *testing.T) {
	client, requests := newB28ServiceClient(t, serveNPUMetrics(npuExporterTestMetrics))

	node := nodeWithNPU("npu-node-1", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if got := atomic.LoadInt32(requests); got != 1 {
		t.Errorf("service endpoint requests = %d, want 1 (the fallback path)", got)
	}
	if len(node.NPUChips) != 2 {
		t.Fatalf("len(NPUChips) = %d, want 2 via the service fallback", len(node.NPUChips))
	}
	if node.NPUUtilization != 60 || node.NPUTemperature != 50 {
		t.Errorf("service fallback aggregates = util %v temp %v, want 60/50", node.NPUUtilization, node.NPUTemperature)
	}
}

// TestB28PodListFailureFallsBack pins the other half of decision 1: a failing
// pod list is not fatal, the cycle continues on the service path.
func TestB28PodListFailureFallsBack(t *testing.T) {
	client, requests := newB28ServiceClient(t, serveNPUMetrics(npuExporterTestMetrics))
	client.podLister = func(ctx context.Context) ([]npuExporterPod, error) {
		return nil, errors.New("forbidden: pods is forbidden")
	}

	node := nodeWithNPU("npu-node-1", 1)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if got := atomic.LoadInt32(requests); got != 1 {
		t.Errorf("service endpoint requests = %d, want 1 after the pod list failed", got)
	}
	if len(node.NPUChips) != 2 {
		t.Errorf("len(NPUChips) = %d, want 2 via the service fallback", len(node.NPUChips))
	}
}

// TestB28PartialFailureKeepsServingCluster pins design decision 5: one node's
// scrape failing must not silence the others and must not arm the cluster-wide
// backoff.
func TestB28PartialFailureKeepsServingCluster(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "exporter-on-node-a", NodeName: "node-a"},
			{Name: "exporter-on-node-b", NodeName: "node-b"},
		},
		payloads: map[string]string{
			"exporter-on-node-b": b28ChipPayload(4, 60, "pod-b", 0),
		},
		fetchErr: map[string]error{
			"exporter-on-node-a": errors.New("proxy error: dial tcp 10.0.0.1:8082: connect: connection refused"),
		},
	}
	fake.install(client)

	nodeA := nodeWithNPU("node-a", 8)
	nodeB := nodeWithNPU("node-b", 8)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{nodeA, nodeB}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if hasNPUEnrichment(nodeA) {
		t.Errorf("the failing node was enriched: chips=%d temp=%v", len(nodeA.NPUChips), nodeA.NPUTemperature)
	}
	if len(nodeB.NPUChips) != 1 || nodeB.NPUTemperature != 60 {
		t.Fatalf("the healthy node = chips %d temp %v, want 1 chip at 60", len(nodeB.NPUChips), nodeB.NPUTemperature)
	}

	if client.Degraded() {
		t.Error("Degraded() = true after a partial success, want false (one node must not silence the cluster)")
	}
	if client.Liveness().Status != "ok" {
		t.Errorf("liveness = %q, want ok after a partial success", client.Liveness().Status)
	}

	// Non-vacuousness: no backoff means the failing node is retried next cycle.
	fake.fetchErr = nil
	fake.payloads["exporter-on-node-a"] = b28ChipPayload(4, 50, "pod-a", 0)
	next := nodeWithNPU("node-a", 8)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{next}); err != nil {
		t.Fatalf("EnrichNodeData() #2 error = %v", err)
	}
	if len(next.NPUChips) != 1 || next.NPUTemperature != 50 {
		t.Errorf("the retried node = chips %d temp %v, want 1 chip at 50", len(next.NPUChips), next.NPUTemperature)
	}
}

// TestB28AllScrapesFailingArmsBackoff pins the other side of decision 5: a cycle
// where every scrape failed does arm the backoff, and the existing
// npu_monitor_degraded alert path reports it untouched.
func TestB28AllScrapesFailingArmsBackoff(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "exporter-on-node-a", NodeName: "node-a"},
			{Name: "exporter-on-node-b", NodeName: "node-b"},
		},
		fetchErr: map[string]error{
			"exporter-on-node-a": errors.New("dial tcp 10.0.0.1:8082: connect: connection refused"),
			"exporter-on-node-b": errors.New("dial tcp 10.0.0.2:8082: connect: connection refused"),
		},
	}
	fake.install(client)

	nodeA := nodeWithNPU("node-a", 8)
	nodeB := nodeWithNPU("node-b", 8)
	nodes := []*model.NodeData{nodeA, nodeB}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if !client.Degraded() {
		t.Fatal("Degraded() = false after every scrape failed, want the retry backoff")
	}
	if got := fake.fetchCount(); got != 2 {
		t.Errorf("scrapes = %d, want 2 (one attempt per node before backing off)", got)
	}
	if liveness := client.Liveness(); liveness.Status != "backoff" || liveness.LastError == "" {
		t.Errorf("liveness = %+v, want backoff with the failure text", liveness)
	}

	// The existing degradation alert still fires, unchanged.
	agg := &AggregatedDataSource{logger: zap.NewNop()}
	agg.SetNPUExporterClient(client)
	alerts := agg.collectAlerts(nodes, nil, nil, nil, nil)
	matched := npuAlertsOfType(alerts, model.AlertTypeNPUMonitorDegraded)
	if len(matched) != 1 {
		t.Fatalf("degraded alerts = %d, want 1: %+v", len(matched), alerts)
	}
	if matched[0].Severity != model.AlertSeverityInfo {
		t.Errorf("degraded alert severity = %v, want Info", matched[0].Severity)
	}

	// The backoff suppresses the next cycle's scrapes.
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{nodeWithNPU("node-a", 8)}); err != nil {
		t.Fatalf("EnrichNodeData() during backoff error = %v", err)
	}
	if got := fake.fetchCount(); got != 2 {
		t.Errorf("scrapes = %d after a backoff cycle, want 2 (suppressed)", got)
	}
}

// TestB28LivenessStampedForMultiNodePerPodPath pins design decision 6: once the
// metrics are attributable per node, the summary may carry the liveness state
// even for a multi-NPU-node cluster. Before the per-pod path this case produced
// a nil stamp, which the view renders as "monitoring unavailable".
func TestB28LivenessStampedForMultiNodePerPodPath(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "exporter-on-node-a", NodeName: "node-a"},
			{Name: "exporter-on-node-b", NodeName: "node-b"},
		},
		payloads: map[string]string{
			"exporter-on-node-a": b28ChipPayload(4, 50, "pod-a", 0),
			"exporter-on-node-b": b28ChipPayload(4, 60, "pod-b", 0),
		},
	}
	fake.install(client)

	nodes := []*model.NodeData{nodeWithNPU("node-a", 8), nodeWithNPU("node-b", 8)}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	agg := &AggregatedDataSource{logger: zap.NewNop()}
	agg.SetNPUExporterClient(client)
	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.NPUNodesCount != 2 {
		t.Fatalf("NPUNodesCount = %d, want 2", summary.NPUNodesCount)
	}
	if summary.NPUExporterLiveness == nil {
		t.Fatal("a multi-NPU-node cluster enriched per pod carries no liveness state")
	}
	if summary.NPUExporterLiveness.Status != "ok" {
		t.Errorf("liveness status = %q, want ok", summary.NPUExporterLiveness.Status)
	}

	// The stamp follows the path, not the node count: a multi-node cluster that
	// stayed on the single-answer service path still makes no claim.
	serviceClient, _ := newB28ServiceClient(t, serveNPUMetrics(npuExporterTestMetrics))
	agg2 := &AggregatedDataSource{logger: zap.NewNop()}
	agg2.SetNPUExporterClient(serviceClient)
	if err := serviceClient.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() #2 error = %v", err)
	}
	summary2 := agg2.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary2.NPUExporterLiveness != nil {
		t.Errorf("a multi-node cluster on the single-answer path is stamped %+v", summary2.NPUExporterLiveness)
	}
}

// TestB28AllScrapesFailingStillClaimsLiveness pins the honest side of decision
// 6: per-node attribution is a property of the path, so a cycle in which every
// scrape failed must still report "backoff" instead of falling back to "never".
func TestB28AllScrapesFailingStillClaimsLiveness(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{{Name: "exporter-on-node-a", NodeName: "node-a"}},
		fetchErr: map[string]error{
			"exporter-on-node-a": errors.New("connection refused"),
		},
	}
	fake.install(client)

	nodes := []*model.NodeData{nodeWithNPU("node-a", 8), nodeWithNPU("node-b", 8)}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	agg := &AggregatedDataSource{logger: zap.NewNop()}
	agg.SetNPUExporterClient(client)
	summary := agg.buildClusterSummary(nodes, nil, nil, nil, nil, nil, workloadLists{})
	if summary.NPUExporterLiveness == nil || summary.NPUExporterLiveness.Status != "backoff" {
		t.Fatalf("liveness = %+v, want a backoff claim", summary.NPUExporterLiveness)
	}
}

// TestB28DefaultSeamsBuildAPIServerRequests pins the default implementation of
// the two seams, which the fake-seam probes above cannot cover: discovery lists
// kube-system pods filtered by the exporter label, and each scrape goes through
// that pod's own :8082 proxy. The URL shapes are what makes the answer belong to
// one node, so they are asserted literally.
func TestB28DefaultSeamsBuildAPIServerRequests(t *testing.T) {
	var mu sync.Mutex
	var urls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		urls = append(urls, r.URL.String())
		mu.Unlock()

		if r.URL.Path == "/api/v1/namespaces/kube-system/pods" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[
				{"metadata":{"name":"npu-exporter-docker-4qtp5"},"spec":{"nodeName":"192.168.2.77"}},
				{"metadata":{"name":"npu-exporter-docker-5lzxv"},"spec":{"nodeName":"192.168.2.27"}}
			]}`))
			return
		}
		_, _ = w.Write([]byte(b28ChipPayload(4, 50, "pod-a", 0)))
	}))
	t.Cleanup(srv.Close)

	client, err := NewNPUExporterClient(&rest.Config{Host: srv.URL}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewNPUExporterClient() error = %v", err)
	}

	pods, err := client.podLister(context.Background())
	if err != nil {
		t.Fatalf("podLister() error = %v", err)
	}
	want := []npuExporterPod{
		{Name: "npu-exporter-docker-4qtp5", NodeName: "192.168.2.77"},
		{Name: "npu-exporter-docker-5lzxv", NodeName: "192.168.2.27"},
	}
	if !reflect.DeepEqual(pods, want) {
		t.Errorf("podLister() = %+v, want %+v", pods, want)
	}

	body, err := client.podFetcher(context.Background(), want[0])
	if err != nil {
		t.Fatalf("podFetcher() error = %v", err)
	}
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(raw), `npu_chip_info_temperature{id="4"} 50`) {
		t.Errorf("podFetcher() body = %q, want the fake pod's metrics", raw)
	}

	mu.Lock()
	defer mu.Unlock()
	// The label values are pinned as raw literals rather than read back from
	// npuExporterPodSelectors: the whole point of the dual selector is that
	// both wild label shapes are discovered, so a constant edited into a wrong
	// value must fail this assertion — reading the constant here would make
	// the check a tautology that stays green while discovery goes blind.
	wantURLs := []string{
		"/api/v1/namespaces/kube-system/pods?labelSelector=app.kubernetes.io%2Fname%3Dnpu-exporter",
		"/api/v1/namespaces/kube-system/pods?labelSelector=app%3Dnpu-exporter",
		"/api/v1/namespaces/kube-system/pods/npu-exporter-docker-4qtp5:8082/proxy/metrics",
	}
	if len(urls) != len(wantURLs) {
		t.Fatalf("requests = %v, want %v", urls, wantURLs)
	}
	for i, want := range wantURLs {
		if urls[i] != want {
			t.Errorf("request #%d = %q, want %q", i, urls[i], want)
		}
	}
}

// TestB28CustomEndpointKeepsSingleAnswerPath pins design decision 7: a custom
// endpoint (--npu-exporter / SetEndpoint) is a single direct HTTP target with no
// node identity, so the per-pod path must not be taken even when the pod list
// would support it. Multi-NPU-node clusters stay unenriched there, exactly as
// before this batch.
func TestB28CustomEndpointKeepsSingleAnswerPath(t *testing.T) {
	client, requests := newTestNPUExporterClient(t, serveNPUMetrics(npuExporterTestMetrics))

	// A pod list that would cover both nodes: only the endpoint mode keeps the
	// per-pod path away.
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "exporter-on-node-a", NodeName: "node-a"},
			{Name: "exporter-on-node-b", NodeName: "node-b"},
		},
		payloads: map[string]string{
			"exporter-on-node-a": b28ChipPayload(4, 50, "pod-a", 0),
			"exporter-on-node-b": b28ChipPayload(4, 60, "pod-b", 0),
		},
	}
	fake.install(client)

	nodes := []*model.NodeData{nodeWithNPU("node-a", 8), nodeWithNPU("node-b", 8)}
	if err := client.EnrichNodeData(context.Background(), nodes); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if got := fake.fetchCount(); got != 0 {
		t.Errorf("per-pod scrapes = %d, want 0 (a custom endpoint has no node identity)", got)
	}
	if got := atomic.LoadInt32(requests); got != 0 {
		t.Errorf("endpoint requests = %d, want 0 (two NPU nodes cannot be served by one answer)", got)
	}
	for _, node := range nodes {
		if hasNPUEnrichment(node) {
			t.Errorf("node %s was enriched from a single unattributable answer", node.Name)
		}
	}

	// Single NPU node: the endpoint answer is used, as today.
	single := nodeWithNPU("node-a", 8)
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{single}); err != nil {
		t.Fatalf("EnrichNodeData() #2 error = %v", err)
	}
	if got := fake.fetchCount(); got != 0 {
		t.Errorf("per-pod scrapes = %d in custom-endpoint mode, want 0", got)
	}
	if len(single.NPUChips) != 2 || single.NPUUtilization != 60 {
		t.Errorf("custom endpoint single node = chips %d util %v, want 2 chips at 60",
			len(single.NPUChips), single.NPUUtilization)
	}
}

// TestB28AttributePods pins the edge cases of the pairing rule (decision 3): a
// pod without a node assignment and a pod on a node without NPU capacity are
// skipped, and several pods on one node resolve deterministically to the first
// by name.
func TestB28AttributePods(t *testing.T) {
	client := newB28PerPodClient(t)
	// attributePods receives the already-filtered NPU node list, as
	// EnrichNodeData passes it; the CPU-only node is part of the pod fixture
	// below through its node name.
	nodes := []*model.NodeData{nodeWithNPU("node-a", 8)}

	pods := []npuExporterPod{
		{Name: "exporter-unscheduled", NodeName: ""},
		{Name: "exporter-on-cpu", NodeName: "cpu-node"},
		{Name: "exporter-zzz", NodeName: "node-a"},
		{Name: "exporter-aaa", NodeName: "node-a"},
		{Name: "exporter-elsewhere", NodeName: "node-not-in-list"},
	}

	targets := client.attributePods(pods, nodes)
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1: %+v", len(targets), targets)
	}
	if targets[0].node.Name != "node-a" {
		t.Errorf("target node = %q, want node-a", targets[0].node.Name)
	}
	if targets[0].pod.Name != "exporter-aaa" {
		t.Errorf("chosen pod = %q, want the first by name (exporter-aaa)", targets[0].pod.Name)
	}

	if got := client.attributePods(nil, nodes); len(got) != 0 {
		t.Errorf("no pods produced %d targets", len(got))
	}
}

// TestB28PerPodPathMatchesServicePath pins decision 9: for a single NPU node
// with one discovered exporter pod, the per-pod answer is converted exactly as
// the service answer was — every chip field and every node aggregate, deltas
// included.
//
// The payload lists its chips out of ID order on purpose: the parse path stores
// them in a map, so only an explicit sort yields a stable NPUChips order.
func TestB28PerPodPathMatchesServicePath(t *testing.T) {
	const payload = `machine_npu_nums 1
npu_chip_info_utilization{id="3",model_name="Ascend910B"} 70
npu_chip_info_utilization{id="0",model_name="Ascend910B",pcie_bus_info="0000:01:00.0",pod_name="train-0",namespace="ml",container_name="trainer"} 50
npu_chip_info_utilization{id="5"} 10
npu_chip_info_utilization{id="1",model_name="Ascend910B",pcie_bus_info="0000:02:00.0"} 30
npu_chip_info_utilization{id="4"} 20
npu_chip_info_utilization{id="2"} 40
npu_chip_info_temperature{id="0"} 45
npu_chip_info_temperature{id="1"} 55
npu_chip_info_temperature{id="2"} 46
npu_chip_info_temperature{id="3"} 56
npu_chip_info_temperature{id="4"} 47
npu_chip_info_temperature{id="5"} 57
npu_chip_info_hbm_used_memory{id="0"} 100
npu_chip_info_hbm_used_memory{id="1"} 300
npu_chip_info_hbm_total_memory{id="0"} 1000
npu_chip_info_hbm_total_memory{id="1"} 1000
npu_chip_info_hbm_ecc_total_single_bit_error_cnt{id="0"} 99992
npu_chip_info_health_status{id="0"} 1
npu_chip_info_health_status{id="1"} 0
`

	perPod := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods:     []npuExporterPod{{Name: "exporter-on-node-a", NodeName: "node-a"}},
		payloads: map[string]string{"exporter-on-node-a": payload},
	}
	fake.install(perPod)

	viaService, _ := newB28ServiceClient(t, serveNPUMetrics(payload))

	nodePerPod := nodeWithNPU("node-a", 8)
	if err := perPod.EnrichNodeData(context.Background(), []*model.NodeData{nodePerPod}); err != nil {
		t.Fatalf("EnrichNodeData() per pod error = %v", err)
	}
	nodeViaService := nodeWithNPU("node-a", 8)
	if err := viaService.EnrichNodeData(context.Background(), []*model.NodeData{nodeViaService}); err != nil {
		t.Fatalf("EnrichNodeData() via service error = %v", err)
	}

	if fake.fetchCount() != 1 {
		t.Errorf("per-pod scrapes = %d, want 1", fake.fetchCount())
	}

	// Chip order is part of the output: the UI indexes chips by position.
	if len(nodePerPod.NPUChips) != 6 {
		t.Fatalf("chips = %d, want 6", len(nodePerPod.NPUChips))
	}
	for i, chip := range nodePerPod.NPUChips {
		if chip.PhyID != i {
			t.Errorf("chip %d has PhyID %d, want the slice ordered by chip ID", i, chip.PhyID)
		}
	}

	// The timestamp is the one field that legitimately differs (two clock reads).
	nodePerPod.NPUMetricsTime = nodeViaService.NPUMetricsTime
	if !reflect.DeepEqual(nodePerPod, nodeViaService) {
		t.Errorf("the per-pod path diverged from the service path:\n per pod: %+v\n service: %+v",
			nodePerPod, nodeViaService)
	}
}

// TestB28DuplicateExportersPerNode pins the last-refresh-stability rule of
// decision 3 end to end: two exporter variants on the same node (docker and
// containerd side by side) must yield one scrape, always of the same pod, so the
// node's chip set cannot flip between refreshes.
func TestB28DuplicateExportersPerNode(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			// The rule is "first pod name wins", so the fixture makes the
			// alphabetically first one distinguishable by its payload.
			{Name: "npu-exporter-containerd-9wxyz", NodeName: "node-a"},
			{Name: "npu-exporter-docker-4qtp5", NodeName: "node-a"},
		},
		payloads: map[string]string{
			"npu-exporter-docker-4qtp5":     b28ChipPayload(4, 50, "pod-docker", 0),
			"npu-exporter-containerd-9wxyz": b28ChipPayload(4, 99, "pod-containerd", 0),
		},
	}
	fake.install(client)

	// Several refreshes: the choice has to be stable, not just once lucky.
	for i := 0; i < 3; i++ {
		node := nodeWithNPU("node-a", 8)
		if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
			t.Fatalf("EnrichNodeData() #%d error = %v", i, err)
		}
		if len(node.NPUChips) != 1 || node.NPUTemperature != 99 {
			t.Fatalf("refresh #%d: chips %d temp %v, want the first-by-name pod's answer (1 chip at 99)",
				i, len(node.NPUChips), node.NPUTemperature)
		}
		if node.NPUChips[0].OwnerPod != "pod-containerd" {
			t.Errorf("refresh #%d: owner = %q, want pod-containerd (first pod by name)", i, node.NPUChips[0].OwnerPod)
		}
	}

	if got := fake.fetchCount(); got != 3 {
		t.Errorf("scrapes = %d over 3 refreshes, want 3 (one pod per node per refresh)", got)
	}
}

// TestB28MultiNodeEnrichmentSkipsNonNPUPods checks the observable side of the
// per-pod path: exporter pods on nodes without NPU capacity contribute nothing
// and the NPU nodes are still covered.
func TestB28MultiNodeEnrichmentSkipsNonNPUPods(t *testing.T) {
	client := newB28PerPodClient(t)
	fake := &b28FakeExporter{
		pods: []npuExporterPod{
			{Name: "exporter-on-cpu", NodeName: "cpu-node"},
			{Name: "exporter-on-node-a", NodeName: "node-a"},
		},
		payloads: map[string]string{
			"exporter-on-cpu":    b28ChipPayload(4, 99, "pod-cpu", 0),
			"exporter-on-node-a": b28ChipPayload(4, 50, "pod-a", 0),
		},
	}
	fake.install(client)

	npuNode := nodeWithNPU("node-a", 8)
	cpuNode := &model.NodeData{Name: "cpu-node"}
	if err := client.EnrichNodeData(context.Background(), []*model.NodeData{npuNode, cpuNode}); err != nil {
		t.Fatalf("EnrichNodeData() error = %v", err)
	}

	if fake.fetchCount() != 1 {
		t.Errorf("scrapes = %d, want 1 (the exporter pod on a non-NPU node must not be scraped)", fake.fetchCount())
	}
	if npuNode.NPUTemperature != 50 {
		t.Errorf("NPU node temperature = %v, want 50", npuNode.NPUTemperature)
	}
	if hasNPUEnrichment(cpuNode) {
		t.Error("the CPU-only node was enriched")
	}
}

// TestB28SingleNodeAllScrapesForbiddenFallsBackToService pins the credential
// shape behind the single-node service fallback: a kubeconfig that may list
// pods but may not use pods/proxy fails every per-pod scrape, and without the
// fallback that left a single-NPU-node cluster dark even though the service
// proxy — the verb such credentials have always carried — still worked. The
// cycle must hand over to the service path instead of arming the backoff, so
// the fallback keeps working on every refresh rather than only the first.
func TestB28SingleNodeAllScrapesForbiddenFallsBackToService(t *testing.T) {
	var mu sync.Mutex
	var proxyURLs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/namespaces/kube-system/pods":
			// Both label variants list here; the same pod answers twice and
			// discovery must keep it once.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"exporter-aaa"},"spec":{"nodeName":"node-a"}}]}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/kube-system/pods/"):
			proxyURLs = append(proxyURLs, r.URL.String())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"pods/proxy is forbidden","reason":"Forbidden","code":403}`))
		case strings.Contains(r.URL.Path, "/services/npu-exporter:8082/proxy/"):
			proxyURLs = append(proxyURLs, r.URL.String())
			_, _ = w.Write([]byte(b28ChipPayload(4, 50, "pod-a", 0)))
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := NewNPUExporterClient(&rest.Config{Host: srv.URL}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewNPUExporterClient() error = %v", err)
	}

	refresh := func() *model.NodeData {
		node := nodeWithNPU("node-a", 8)
		t.Helper()
		if err := client.EnrichNodeData(context.Background(), []*model.NodeData{node}); err != nil {
			t.Fatalf("EnrichNodeData() error = %v", err)
		}
		return node
	}

	node := refresh()
	if len(node.NPUChips) != 1 || node.NPUChips[0].Temp != 50 || node.NPUChips[0].OwnerPod != "pod-a" {
		t.Fatalf("chips after the refused per-pod path = %+v, want one chip 4 at 50°C owned by pod-a via the service", node.NPUChips)
	}
	if client.Degraded() {
		t.Error("Degraded() = true although the service fallback answered")
	}
	if liveness := client.Liveness(); liveness.Status != "ok" {
		t.Errorf("liveness status = %q, want ok", liveness.Status)
	}

	// The per-pod path is refused on the next refresh too; a backoff armed on
	// those refusals would silence the chip data from the second cycle on.
	node = refresh()
	if len(node.NPUChips) != 1 {
		t.Fatalf("second refresh carries %d chips, want 1 — the fallback stopped serving", len(node.NPUChips))
	}
	if client.Degraded() {
		t.Error("Degraded() = true on the second refresh, the refusals armed the backoff")
	}

	mu.Lock()
	defer mu.Unlock()
	podProxy, serviceProxy := 0, 0
	for _, u := range proxyURLs {
		switch {
		case strings.Contains(u, "/pods/"):
			podProxy++
		case strings.Contains(u, "/services/"):
			serviceProxy++
		}
	}
	if podProxy != 2 || serviceProxy != 2 {
		t.Errorf("pod proxy requests = %d, service proxy requests = %d, want 2 and 2 (each cycle: refuse, then fall back)", podProxy, serviceProxy)
	}
}
