package datasource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WP3 probes: the Volcano queue resource conversion and the kubelet failure
// path of the aggregated data source.

// convertQueueForTest runs convertQueue on a Queue unstructured object built
// from the given spec/status maps.
func convertQueueForTest(t *testing.T, spec, status map[string]interface{}) *model.QueueData {
	t.Helper()

	obj := map[string]interface{}{
		"apiVersion": "scheduling.volcano.sh/v1beta1",
		"kind":       "Queue",
		"metadata":   map[string]interface{}{"name": "q-test"},
	}
	if spec != nil {
		obj["spec"] = spec
	}
	if status != nil {
		obj["status"] = status
	}

	client := &VolcanoClient{}
	queue := client.convertQueue(&unstructured.Unstructured{Object: obj})
	if queue == nil {
		t.Fatalf("convertQueue returned nil")
	}
	return queue
}

// TestConvertQueueGuaranteeNestedResource is the W3-1 regression guard: the
// Volcano Queue spec nests the guarantee resources one level deeper than
// deserved/allocated (`spec.guarantee.resource`, the ResourceList field of
// QueueSpec.Guarantee), so parsing the guarantee map directly found a map value
// under "resource", skipped it, and every guarantee stayed 0 — which the queues
// view then hid behind its `> 0` guard.
func TestConvertQueueGuaranteeNestedResource(t *testing.T) {
	queue := convertQueueForTest(t, map[string]interface{}{
		"guarantee": map[string]interface{}{
			"resource": map[string]interface{}{
				"cpu":    "2",
				"memory": "4Gi",
				"pods":   "7",
			},
		},
	}, nil)

	if queue.CPUGuarantee != 2000 {
		t.Errorf("CPUGuarantee = %d millicores, want 2000", queue.CPUGuarantee)
	}
	if want := int64(4) * 1024 * 1024 * 1024; queue.MemoryGuarantee != want {
		t.Errorf("MemoryGuarantee = %d bytes, want %d", queue.MemoryGuarantee, want)
	}
}

// TestConvertQueueGuaranteeNestedNPU pins the NPU resource inside the nested
// form: the chip count and the resource name both come from guarantee.resource.
func TestConvertQueueGuaranteeNestedNPU(t *testing.T) {
	queue := convertQueueForTest(t, map[string]interface{}{
		"guarantee": map[string]interface{}{
			"resource": map[string]interface{}{
				"huawei.com/ascend910": "8",
			},
		},
	}, nil)

	if queue.NPUGuarantee != 8 {
		t.Errorf("NPUGuarantee = %d, want 8", queue.NPUGuarantee)
	}
}

// TestConvertQueueGuaranteeFlatFallback keeps the tolerance for a non-standard
// CRD that puts the resources directly under guarantee: a flat guarantee is
// still parsed, so the fix does not regress a cluster that worked before.
func TestConvertQueueGuaranteeFlatFallback(t *testing.T) {
	queue := convertQueueForTest(t, map[string]interface{}{
		"guarantee": map[string]interface{}{
			"cpu":    "1",
			"memory": "2Gi",
		},
	}, nil)

	if queue.CPUGuarantee != 1000 {
		t.Errorf("CPUGuarantee = %d millicores, want 1000 (flat fallback)", queue.CPUGuarantee)
	}
	if want := int64(2) * 1024 * 1024 * 1024; queue.MemoryGuarantee != want {
		t.Errorf("MemoryGuarantee = %d bytes, want %d (flat fallback)", queue.MemoryGuarantee, want)
	}
}

// TestConvertQueueGuaranteeNestedWins pins the precedence when both shapes are
// present: the upstream nested form must win over a stray flat key.
func TestConvertQueueGuaranteeNestedWins(t *testing.T) {
	queue := convertQueueForTest(t, map[string]interface{}{
		"guarantee": map[string]interface{}{
			"cpu": "9",
			"resource": map[string]interface{}{
				"cpu": "3",
			},
		},
	}, nil)

	if queue.CPUGuarantee != 3000 {
		t.Errorf("CPUGuarantee = %d millicores, want 3000 (the nested form must win over the flat fallback)",
			queue.CPUGuarantee)
	}
}

// TestConvertQueueDeservedAndAllocatedUnchanged pins the two maps that were
// already flat: they must keep parsing exactly as before.
func TestConvertQueueDeservedAndAllocatedUnchanged(t *testing.T) {
	queue := convertQueueForTest(t, map[string]interface{}{
		"deserved": map[string]interface{}{"cpu": "4", "memory": "8Gi", "pods": "20"},
	}, map[string]interface{}{
		"allocated": map[string]interface{}{"cpu": "2", "memory": "1Gi", "pods": "5"},
	})

	if queue.CPUDeserved != 4000 {
		t.Errorf("CPUDeserved = %d, want 4000", queue.CPUDeserved)
	}
	if queue.PodDeserved != 20 {
		t.Errorf("PodDeserved = %d, want 20", queue.PodDeserved)
	}
	if queue.CPUAllocated != 2000 {
		t.Errorf("CPUAllocated = %d, want 2000", queue.CPUAllocated)
	}
	if queue.PodAllocated != 5 {
		t.Errorf("PodAllocated = %d, want 5", queue.PodAllocated)
	}
}

// failingKubeletServer returns a test server that rejects every summary request,
// so the kubelet client's error branch is the one under test.
func failingKubeletServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kubelet is down", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestKubeletFailureKeepsPodCount is the W3-2 regression guard: PodCount comes
// from the API Server pod list, which does not depend on the kubelet at all.
// The failure branch returned before setting it, so every node whose kubelet
// was unreachable reported 0 pods while the pods view listed them.
func TestKubeletFailureKeepsPodCount(t *testing.T) {
	server := failingKubeletServer(t)

	dataSource := NewAggregatedDataSource(nil, newTestKubeletClient(server), zap.NewNop(), 2)

	nodes := []*model.NodeData{{Name: "node-a", PodAllocatable: 10}}
	pods := []*model.PodData{
		{Name: "web-0", Namespace: "default", Node: "node-a"},
		{Name: "web-1", Namespace: "default", Node: "node-a"},
		{Name: "other-0", Namespace: "default", Node: "node-b"},
	}

	dataSource.enrichWithKubeletMetrics(context.Background(), nodes, pods)

	node := nodes[0]
	if node.HasKubeletMetrics {
		t.Fatalf("HasKubeletMetrics = true after a failed kubelet fetch")
	}
	if node.PodCount != 2 {
		t.Errorf("PodCount = %d after a failed kubelet fetch, want 2 (the API Server pod list is unaffected)", node.PodCount)
	}
	if want := 20.0; node.PodUsagePercent != want {
		t.Errorf("PodUsagePercent = %v, want %v", node.PodUsagePercent, want)
	}
}

// TestKubeletFailureWithoutPodAllocatable keeps the percent at zero when the
// node reports no pod capacity: the count is still recorded, the division is
// skipped.
func TestKubeletFailureWithoutPodAllocatable(t *testing.T) {
	server := failingKubeletServer(t)

	dataSource := NewAggregatedDataSource(nil, newTestKubeletClient(server), zap.NewNop(), 2)

	nodes := []*model.NodeData{{Name: "node-a"}}
	pods := []*model.PodData{{Name: "web-0", Namespace: "default", Node: "node-a"}}

	dataSource.enrichWithKubeletMetrics(context.Background(), nodes, pods)

	if nodes[0].PodCount != 1 {
		t.Errorf("PodCount = %d, want 1", nodes[0].PodCount)
	}
	if nodes[0].PodUsagePercent != 0 {
		t.Errorf("PodUsagePercent = %v with no allocatable pods, want 0", nodes[0].PodUsagePercent)
	}
}
