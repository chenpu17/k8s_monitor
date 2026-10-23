package datasource

// b24v_health_domain_test.go — adversarial review of batch 24 W1.
//
// The batch removed `case "Warning"` from the per-chip health grading switch on
// the grounds that npu_chip_info_health_status is only 1 / 0 / -1. That value
// domain is true for the NPU-Exporter producer, but NPUChipData.Health has a
// second producer in this repository: the k8s-monitor NPU collector DaemonSet,
// which copies the `npu-smi info` health string verbatim into the
// k8s-monitor.io/npu-metrics annotation (deploy/k8s-monitor-npu-collector.yaml,
// documented in deploy/README.md as "OK/Warning/Error"), and ConvertNode copies
// it into NPUChipData.Health unchanged (internal/datasource/interface.go:226).
//
// TestB24VCollectorHealthWarningStillGrades is therefore RED against the batch
// as delivered: a chip the collector reports as Warning is graded healthy and
// raises nothing.

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// b24vCollectorNode builds the node the deploy/ collector produces: one NPU
// with two chips, health strings straight out of `npu-smi info`.
func b24vCollectorNode(t *testing.T, healths ...string) *model.NodeData {
	t.Helper()

	chips := ""
	for i, health := range healths {
		if i > 0 {
			chips += ","
		}
		chips += `{"npu":0,"chip":` + itoa(i) + `,"phy_id":` + itoa(i) +
			`,"bus_id":"0000:9D:00.0","health":"` + health +
			`","power":150,"temp":47,"aicore":10,"hbm_used":1024,"hbm_total":65536}`
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "192.168.2.188",
			Annotations: map[string]string{
				"k8s-monitor.io/npu-metrics": `{"timestamp":"2026-10-23T10:00:00Z","chips":[` + chips + `]}`,
			},
		},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			"huawei.com/ascend-1980": resource.MustParse(itoa(len(healths))),
		}},
	}
	converted := ConvertNode(node)
	if len(converted.NPUChips) != len(healths) {
		t.Fatalf("ConvertNode produced %d chips for %d health strings", len(converted.NPUChips), len(healths))
	}
	return converted
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestB24VCollectorHealthWarningStillGrades pins the second producer: the
// collector path's "Warning" must degrade the chip (HEAD=44c52af did; the
// batch's first cut dropped it and was fixed in review).
func TestB24VCollectorHealthWarningStillGrades(t *testing.T) {
	node := b24vCollectorNode(t, "OK", "Warning")

	// The producer value survives conversion unchanged, so the grading switch
	// really does see it.
	if got := node.NPUChips[1].Health; got != "Warning" {
		t.Fatalf("ConvertNode rewrote the collector health %q; the probe no longer reaches the switch", got)
	}

	npuThresholdAlerts(node, time.Now())

	grade := gradeOf(t, node.NPUChipHealth, 1)
	if grade.Grade != model.NPUChipGradeDegraded {
		t.Errorf("collector health Warning graded %q, want %q (defect: the removed \"Warning\" case was live on this producer path)",
			grade.Grade, model.NPUChipGradeDegraded)
	}
	if grade.Reason != "chip health Warning" {
		t.Errorf("collector health Warning reason = %q, want %q", grade.Reason, "chip health Warning")
	}
}

// TestB24VCollectorHealthErrorStillFaults pins the half of the domain that did
// survive the change, so the defect above is the only divergence.
func TestB24VCollectorHealthErrorStillFaults(t *testing.T) {
	node := b24vCollectorNode(t, "OK", "Error")
	npuThresholdAlerts(node, time.Now())

	if grade := gradeOf(t, node.NPUChipHealth, 1); grade.Grade != model.NPUChipGradeFault || grade.Reason != "chip health Error" {
		t.Errorf("collector health Error graded %q/%q, want fault/\"chip health Error\"", grade.Grade, grade.Reason)
	}
	if got := countNPUAlerts(npuThresholdAlerts(node, time.Now())); got != 1 {
		t.Errorf("collector health Error raised %d NPU alerts, want 1 (the chip-health alert)", got)
	}
}

// TestB24VHealthDomainUnion documents the two producers side by side: the
// exporter's three values and the collector's three values overlap on OK/Error
// while the collector adds Warning. Every non-OK value must move the grade off
// healthy on its producing path.
func TestB24VHealthDomainUnion(t *testing.T) {
	exporter := []string{"OK", "Error", "Unknown"}  // npu_exporter.go convertChipMetrics
	collector := []string{"OK", "Warning", "Error"} // deploy/README.md, interface.go:226

	for _, state := range append(append([]string{}, exporter...), collector...) {
		chips := gradedChips(2)
		chips[1].Health = state
		node := &model.NodeData{Name: "npu-node", NPUCapacity: 2, NPUChips: chips}
		npuThresholdAlerts(node, time.Now())
		grade := gradeOf(t, node.NPUChipHealth, 1)
		t.Logf("Health=%-8s -> grade=%-9s reason=%q", state, grade.Grade, grade.Reason)
		if state == "OK" {
			if grade.Grade != model.NPUChipGradeHealthy {
				t.Errorf("Health=OK graded %q, want healthy", grade.Grade)
			}
			continue
		}
		if state == "Unknown" {
			// Deliberately not a hit: absence of a signal is not a signal.
			if grade.Grade != model.NPUChipGradeHealthy {
				t.Errorf("Health=Unknown graded %q, want the untouched healthy grade", grade.Grade)
			}
			continue
		}
		if grade.Grade == model.NPUChipGradeHealthy {
			t.Errorf("Health=%s fell through to healthy; the grading switch lost its case", state)
		}
	}
}

// TestB24VFaultIsNotLoweredByANodeWideDegradation pins the grade-ordering guard
// after the batch removed its only real fixture. The b23 case named "a faulting
// signal cannot be lowered by a later degrading one" used Health="Warning" as
// the later degrading signal; the batch re-pointed it at "Unknown", which no
// longer hits at all, so the case passes with or without the guard (removing
// even the whole assignment leaves it green). This probe uses a path that is
// still live: a chip with a faulting signal, on a node whose management-plane
// device fault level is a degradation, is hit by the node-wide call AFTER the
// per-chip loop — i.e. a lower grade arrives last.
func TestB24VFaultIsNotLoweredByANodeWideDegradation(t *testing.T) {
	chips := gradedChips(4)
	chips[1].HBMEccDoubleErr = 1 // fault, set in the per-chip loop

	node := &model.NodeData{
		Name: "npu-node", NPUCapacity: 4, NPUChips: chips,
		// ImportantNPU grades every chip degraded, and hitNodeWide runs last.
		NPUFault: &model.NPUFaultInfo{DeviceFaultLevel: "ImportantNPU"},
	}
	npuThresholdAlerts(node, time.Now())

	grade := gradeOf(t, node.NPUChipHealth, 1)
	if grade.Grade != model.NPUChipGradeFault {
		t.Errorf("a later node-wide degradation lowered the chip from fault to %q", grade.Grade)
	}
	if grade.Reason != "double-bit ECC" {
		t.Errorf("reason = %q, want the faulting trigger %q", grade.Reason, "double-bit ECC")
	}

	// Non-vacuousness: the same node-wide signal alone really does degrade a
	// clean chip, so the assertion above is not satisfied by the signal being
	// ignored.
	if other := gradeOf(t, node.NPUChipHealth, 0); other.Grade != model.NPUChipGradeDegraded {
		t.Fatalf("the node-wide degradation did not reach a clean chip (%q); the probe is vacuous", other.Grade)
	}
}
