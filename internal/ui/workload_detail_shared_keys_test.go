package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// The three workload detail views (deployment / statefulset / daemonset) are
// template siblings: their isomorphic strings share one detail.workload.*
// bundle key per literal instead of one copy per family. This test pins the
// sharing both ways:
//   - each family must render the byte-identical shared composition (a forked
//     per-family copy with drifted wording turns exactly this test red while
//     every per-family golden stays green);
//   - the compositions are pinned by hardcoded literals, so editing a shared
//     key's value in the bundle turns this test AND the golden tests of all
//     three family files red at once — the drift-protection property the
//     shared namespace exists for.

func TestWorkloadDetailSharedKeysLockedAcrossFamilies(t *testing.T) {
	forcedAsciiProfile(t)

	compositions := map[string]struct {
		selector, noCluster, noSelector string
		replicaLabels                   []string
		morePodsTail                    string
	}{
		"en": {
			selector:      "Selector\n\n  No selector configured\n\nLabels\n\n  No labels",
			noCluster:     "Managed Pods\n\n  No pod information available",
			noSelector:    "Managed Pods\n\n  No selector configured (matchLabels is empty)",
			replicaLabels: []string{"Desired Replicas", "Ready Replicas", "Updated Replicas"},
			morePodsTail:  "  ... and 2 more pods",
		},
		"zh": {
			selector:      "选择器\n\n  未配置选择器\n\n标签\n\n  无标签",
			noCluster:     "托管 Pod\n\n  暂无 Pod 信息",
			noSelector:    "托管 Pod\n\n  未配置选择器（matchLabels 为空）",
			replicaLabels: []string{"期望副本数", "就绪副本数", "已更新副本数"},
			morePodsTail:  "  ... 还有 2 个 Pod",
		},
	}

	for locale, want := range compositions {
		m := NewModel(nil, zap.NewNop(), time.Second, locale, "test", 200)

		dep := &model.DeploymentData{Name: "w-x", Namespace: "default"}
		sts := &model.StatefulSetData{Name: "w-x", Namespace: "default"}
		dss := &model.DaemonSetData{Name: "w-x", Namespace: "default"}

		// Selector + labels section: all three families byte-identical.
		got := map[string]string{
			"deployment":  m.renderDeploymentSelector(dep),
			"statefulset": m.renderStatefulSetSelector(sts),
			"daemonset":   m.renderDaemonSetSelector(dss),
		}
		for family, out := range got {
			if out != want.selector {
				t.Errorf("[%s] %s selector section:\n got %q\nwant %q", locale, family, out, want.selector)
			}
		}

		// Pods section, no cluster data: all three families byte-identical.
		m.clusterData = nil
		got = map[string]string{
			"deployment":  m.renderDeploymentPods(dep),
			"statefulset": m.renderStatefulSetPods(sts),
			"daemonset":   m.renderDaemonSetPods(dss),
		}
		for family, out := range got {
			if out != want.noCluster {
				t.Errorf("[%s] %s no-cluster-data pods section:\n got %q\nwant %q", locale, family, out, want.noCluster)
			}
		}

		// Pods section, pods exist but the workload has no selector: all
		// three families byte-identical.
		m.clusterData = &model.ClusterData{Pods: []*model.PodData{
			{Name: "pod-a", Namespace: "default", Phase: "Running"},
		}}
		got = map[string]string{
			"deployment":  m.renderDeploymentPods(dep),
			"statefulset": m.renderStatefulSetPods(sts),
			"daemonset":   m.renderDaemonSetPods(dss),
		}
		for family, out := range got {
			if out != want.noSelector {
				t.Errorf("[%s] %s no-selector pods section:\n got %q\nwant %q", locale, family, out, want.noSelector)
			}
		}

		// Replica labels shared by the deployment and statefulset replica
		// panels (the daemonset status panel uses family keys instead).
		depReplica := m.renderDeploymentReplicaStatus(&model.DeploymentData{Name: "w-x", Replicas: 1, ReadyReplicas: 1})
		stsReplica := m.renderStatefulSetReplicaStatus(&model.StatefulSetData{Name: "w-x", Replicas: 1, ReadyReplicas: 1})
		for _, label := range want.replicaLabels {
			if !strings.Contains(depReplica, label) {
				t.Errorf("[%s] deployment replica panel %q does not contain the shared label %q", locale, depReplica, label)
			}
			if !strings.Contains(stsReplica, label) {
				t.Errorf("[%s] statefulset replica panel %q does not contain the shared label %q", locale, stsReplica, label)
			}
		}

		// The "... and N more pods" tail is shared by the deployment and
		// statefulset pods tables (the daemonset table has no tail).
		var many []*model.PodData
		for i := 0; i < 22; i++ {
			many = append(many, &model.PodData{
				Name: "p-" + strconv.Itoa(i), Namespace: "default", Phase: "Running",
				Labels: map[string]string{"app": "w-x"},
			})
		}
		m.clusterData = &model.ClusterData{Pods: many}
		dep.Selector = map[string]string{"app": "w-x"}
		sts.Selector = map[string]string{"app": "w-x"}
		depPods := m.renderDeploymentPods(dep)
		stsPods := m.renderStatefulSetPods(sts)
		if !strings.HasSuffix(depPods, want.morePodsTail) {
			t.Errorf("[%s] deployment pods table must end with the shared tail %q:\n%s", locale, want.morePodsTail, depPods)
		}
		if !strings.HasSuffix(stsPods, want.morePodsTail) {
			t.Errorf("[%s] statefulset pods table must end with the shared tail %q:\n%s", locale, want.morePodsTail, stsPods)
		}
	}
}
