package datasource

// b25v_collector_test.go — reviewer probe for batch 25.
//
// Batch R1 states the annotation collector path (deploy/k8s-monitor-npu-collector.yaml)
// has no ownership field at all, so chips produced from it must stay
// unattributed. That is the reason the batch only wires the exporter path; if a
// later change invented ownership for the collector payload, workload
// attribution would be fabricated from a source that has no idea who holds the
// chip. This probe is what makes that statement executable.

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// b25vCollectorNode builds a node exactly as the deploy/ collector publishes it:
// the annotation JSON carries only the npu-smi fields, and the ownership labels
// that NPU-Exporter emits are absent by construction.
func b25vCollectorNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "npu-collector",
			Annotations: map[string]string{
				"k8s-monitor.io/npu-metrics": `{"timestamp":"2026-10-23T10:00:00Z","chips":[` +
					`{"npu":0,"chip":0,"phy_id":0,"bus_id":"0000:9D:00.0","health":"OK","power":95.5,"temp":45,"aicore":10,"hbm_used":1024,"hbm_total":65536},` +
					`{"npu":0,"chip":1,"phy_id":1,"bus_id":"0000:9E:00.0","health":"OK","power":96.5,"temp":46,"aicore":20,"hbm_used":2048,"hbm_total":65536}]}`,
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceName("huawei.com/ascend-1980"): resource.MustParse("2"),
			},
		},
	}
}

// TestB25VCollectorPathCarriesNoOwner pins R1 at the conversion boundary: a chip
// from the annotation collector has no ownership, so every surface keyed on
// ownership treats it as idle. A change that starts populating OwnerPod from
// that payload would fabricate attribution, and this probe turns red.
func TestB25VCollectorPathCarriesNoOwner(t *testing.T) {
	node := ConvertNode(b25vCollectorNode())
	if len(node.NPUChips) != 2 {
		t.Fatalf("the collector path produced %d chips, want 2: %+v", len(node.NPUChips), node.NPUChips)
	}
	for _, chip := range node.NPUChips {
		if chip.OwnerPod != "" || chip.OwnerNamespace != "" {
			t.Errorf("collector chip %d carries ownership %q/%q; that payload has no such field",
				chip.PhyID, chip.OwnerNamespace, chip.OwnerPod)
		}
	}
}

// TestB25VCollectorPathUnknownOwnerFieldsIgnored documents the inverse risk: the
// annotation JSON is decoded into an anonymous struct, so owner_* keys in the
// payload are dropped on the floor rather than half-trusted. The collector is
// updated independently of this binary, so a payload that grew those keys must
// not smuggle attribution in through the back door.
func TestB25VCollectorPathUnknownOwnerFieldsIgnored(t *testing.T) {
	node := b25vCollectorNode()
	node.Annotations["k8s-monitor.io/npu-metrics"] =
		`{"timestamp":"2026-10-23T10:00:00Z","chips":[` +
			`{"npu":0,"chip":0,"phy_id":0,"health":"OK","power":95.5,"temp":45,"aicore":10,"hbm_used":1024,"hbm_total":65536,` +
			`"pod_name":"train-0","namespace":"ml","owner_pod":"train-0","owner_namespace":"ml"}]}`

	converted := ConvertNode(node)
	if len(converted.NPUChips) != 1 {
		t.Fatalf("converted %d chips, want 1", len(converted.NPUChips))
	}
	chip := converted.NPUChips[0]
	if chip.OwnerPod != "" || chip.OwnerNamespace != "" {
		t.Errorf("owner keys in the annotation payload were trusted: %q/%q", chip.OwnerNamespace, chip.OwnerPod)
	}
}
