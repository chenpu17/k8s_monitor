package ui

// b32v_zerodrift_test.go — verification-only differential probe for batch 32.
//
// It renders every view and every detail page from one fixed fixture and writes
// the stripped output to the file named by B32V_OUT. The same file is compiled
// into the pre-batch tree (git archive of HEAD) and the two outputs are diffed,
// so any view whose bytes moved is visible without trusting the batch's own
// golden tests. It uses no batch-32 symbol, so it builds in both trees.
//
// The probe is inert when B32V_OUT is unset: `go test ./...` runs it as a
// no-op.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

func b32vTime() time.Time {
	return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
}

func b32vNode(name string) *model.NodeData {
	n := &model.NodeData{
		Name: name, InternalIP: "10.0.0.10", ExternalIP: "1.2.3.4",
		Roles: []string{"worker"}, Status: "Ready",
		Labels: map[string]string{"zone": "a"}, Annotations: map[string]string{"k": "v"},
		CreationTimestamp: b32vTime(),
		CPUCapacity:       2000, MemoryCapacity: 8192, PodCapacity: 110,
		CPUAllocatable: 1000, MemAllocatable: 4096, PodAllocatable: 100,
		CPUUsage: 750, MemoryUsage: 2048, PodCount: 3,
		NetworkRxBytes: 1000, NetworkTxBytes: 2000, NetworkTimestamp: b32vTime(),
		HasKubeletMetrics: true,
		NPUCapacity:       16, NPUAllocatable: 16, NPUAllocated: 2,
		NPUChipType: "Ascend910",
		SuperPodID:  "sp-1",
	}
	for i := 0; i < 16; i++ {
		n.NPUChips = append(n.NPUChips, model.NPUChipData{
			NPUID: i / 8, Chip: i % 8, PhyID: i,
			Health: "OK", AICore: 10 + i, HBMUsed: 1000, HBMTotal: 4096,
			Power: 100, Temp: 40 + i,
		})
		grade := model.NPUChipGradeHealthy
		if i == 5 {
			grade = model.NPUChipGradeDegraded
		}
		if i == 11 {
			grade = model.NPUChipGradeFault
		}
		n.NPUChipHealth = append(n.NPUChipHealth, model.NPUChipGrade{NPUID: i / 8, Chip: i % 8, Grade: grade})
	}
	return n
}

func b32vData() *model.ClusterData {
	node := b32vNode("node-a")
	pod := &model.PodData{
		Name: "web-0", Namespace: "default", Node: "node-a", Phase: "Running",
		PodIP: "10.0.0.10", HostIP: "10.0.0.1", QOSClass: "Burstable",
		Containers: 1, ReadyContainers: 1, RestartCount: 2,
		CreationTimestamp: b32vTime(), StartTime: b32vTime(),
		CPURequest: 100, CPULimit: 200, MemoryRequest: 1024, MemoryLimit: 2048,
		CPUUsage: 50, MemoryUsage: 512,
		ContainerStates: []model.ContainerState{{Name: "app", Image: "nginx:1", Ready: true, State: "Running"}},
	}
	ev := &model.EventData{
		Type: "Warning", Reason: "BackOff", Message: "back-off restarting",
		Count: 3, FirstTimestamp: b32vTime(), LastTimestamp: b32vTime(),
		InvolvedObject: "Pod/web-0", InvolvedNamespace: "default", Source: "kubelet",
	}
	svc := &model.ServiceData{
		Name: "web", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.96.0.1",
		Ports:         []model.ServicePort{{Name: "http", Protocol: "TCP", Port: 80, TargetPort: "8080"}},
		EndpointCount: 1, CreationTimestamp: b32vTime(),
	}
	pv := &model.PVData{
		Name: "pv-1", Capacity: 1 << 30, StorageClass: "std", AccessModes: []string{"RWO"},
		ReclaimPolicy: "Delete", Status: "Bound", Claim: "default/pvc-1", VolumeType: "NFS",
		CreationTimestamp: b32vTime(),
	}
	pvc := &model.PVCData{
		Name: "pvc-1", Namespace: "default", Status: "Bound", Volume: "pv-1",
		Capacity: 1 << 30, RequestedStorage: 1 << 30, StorageClass: "std",
		AccessModes: []string{"RWO"}, CreationTimestamp: b32vTime(),
	}
	dep := &model.DeploymentData{Name: "web", Namespace: "default", Replicas: 2, ReadyReplicas: 2, AvailableReplicas: 2, UpdatedReplicas: 2, Strategy: "RollingUpdate", CreationTimestamp: b32vTime()}
	sts := &model.StatefulSetData{Name: "db", Namespace: "default", Replicas: 3, ReadyReplicas: 3, CurrentReplicas: 3, UpdatedReplicas: 3, CreationTimestamp: b32vTime()}
	ds := &model.DaemonSetData{Name: "agent", Namespace: "kube-system", DesiredNumberScheduled: 1, CurrentNumberScheduled: 1, NumberReady: 1, NumberAvailable: 1, CreationTimestamp: b32vTime()}
	job := &model.JobData{Name: "train", Namespace: "default", Completions: 1, Succeeded: 1, StartTime: b32vTime(), CompletionTime: b32vTime().Add(time.Minute), Duration: time.Minute, CreationTimestamp: b32vTime()}
	cj := &model.CronJobData{Name: "nightly", Namespace: "default", Schedule: "0 0 * * *", Active: 1, CreationTimestamp: b32vTime()}
	vj := &model.VolcanoJobData{Name: "vjob", Namespace: "default", Queue: "q1", CreationTimestamp: b32vTime()}
	q := &model.QueueData{Name: "q1", State: "Open", Weight: 1, Reclaimable: true, CPUDeserved: 1000, MemoryDeserved: 1 << 30, NPUDeserved: 8, CPUAllocated: 500, MemoryAllocated: 1 << 29, NPUAllocated: 2, RunningJobs: 1, TotalJobs: 1, CreationTimestamp: b32vTime()}
	hn := &model.HyperNodeData{Name: "sp-1", Tier: 1, NodeCount: 1, TotalNPU: 16, AllocatedNPU: 2, Members: []model.HyperNodeMember{{Type: "Node", Name: "node-a"}}}
	return &model.ClusterData{
		Nodes: []*model.NodeData{node, b32vNode("node-b")},
		Pods:  []*model.PodData{pod}, Events: []*model.EventData{ev},
		Services: []*model.ServiceData{svc}, PVs: []*model.PVData{pv}, PVCs: []*model.PVCData{pvc},
		Deployments: []*model.DeploymentData{dep}, StatefulSets: []*model.StatefulSetData{sts},
		DaemonSets: []*model.DaemonSetData{ds}, Jobs: []*model.JobData{job},
		CronJobs: []*model.CronJobData{cj}, VolcanoJobs: []*model.VolcanoJobData{vj},
		HyperNodes: []*model.HyperNodeData{hn}, Queues: []*model.QueueData{q},
		VolcanoSummary: &model.VolcanoSummary{},
		Summary: &model.ClusterSummary{
			TotalNodes: 2, ReadyNodes: 2, TotalPods: 1, RunningPods: 1, TotalEvents: 1, WarningEvents: 1,
			CPUCapacity: 4000, CPUAllocatable: 2000, CPURequested: 100, CPUUsed: 750,
			MemoryCapacity: 16384, MemoryAllocatable: 8192, MemoryRequested: 1024, MemoryUsed: 2048,
			PodCapacity: 220, PodAllocatable: 200,
			CPURequestUtilization: 5, CPUUsageUtilization: 18.75,
			MemRequestUtilization: 12.5, MemUsageUtilization: 12.5, PodUtilization: 0.5,
			TotalDeployments: 1, TotalStatefulSets: 1, TotalDaemonSets: 1, TotalJobs: 1, TotalCronJobs: 1,
			TotalServices: 1, ClusterIPServices: 1,
			TotalPVs: 1, BoundPVs: 1, TotalPVCs: 1, BoundPVCs: 1, TotalStorageSize: 1 << 30, UsedStorageSize: 1 << 30, StorageUsagePercent: 100,
			NetworkRxBytes: 1000, NetworkTxBytes: 2000, NetworkRxRate: 10, NetworkTxRate: 20,
			NodesWithMetrics: 2, KubeletMetricsAvailable: true,
			NPUCapacity: 16, NPUAllocatable: 16, NPUAllocated: 2, NPUUtilization: 12.5, NPUNodesCount: 1,
			NPUResourceName: "huawei.com/ascend-1980", NPUChipType: "Ascend910",
			HyperClusterID: "hc-1", HyperNodeCount: 1, SuperPodCount: 1,
			LastRefreshTime: b32vTime(),
		},
	}
}

func b32vModel(locale string, width int) *Model {
	m := NewModel(nil, zap.NewNop(), 2*time.Second, locale, "test", 200)
	m.width, m.height = width, 60
	m.clusterData = b32vData()
	m.lastUpdate = b32vTime()
	m.refreshCounter = 1
	return m
}

// b32vRenderAll emits every render surface, each under a stable header.
func b32vRenderAll(t *testing.T) string {
	t.Helper()
	d := b32vData()
	var b strings.Builder
	emit := func(name, out string) {
		fmt.Fprintf(&b, "===== %s =====\n%s\n", name, stripANSI(out))
	}

	for _, locale := range []string{"en", "zh"} {
		for _, width := range []int{38, 120} {
			tag := fmt.Sprintf("%s/w%d", locale, width)

			// Main views.
			views := []struct {
				name string
				view ViewType
			}{
				{"overview", ViewOverview}, {"nodes", ViewNodes}, {"pods", ViewPods},
				{"workloads", ViewWorkloads}, {"network", ViewNetwork}, {"storage", ViewStorage},
				{"events", ViewEvents}, {"alerts", ViewAlerts}, {"queues", ViewQueues},
				{"topology", ViewTopology}, {"npu", ViewNPU},
			}
			for _, v := range views {
				m := b32vModel(locale, width)
				m.currentView = v.view
				emit(tag+"/"+v.name, m.View())
			}

			// Detail pages, each on the fixed fixture's single object.
			details := []struct {
				name string
				view ViewType
				set  func(m *Model)
			}{
				{"node", ViewNodeDetail, func(m *Model) { m.selectedNode = d.Nodes[0] }},
				{"pod", ViewPodDetail, func(m *Model) { m.selectedPod = d.Pods[0] }},
				{"event", ViewEventDetail, func(m *Model) { m.selectedEvent = d.Events[0] }},
				{"service", ViewServiceDetail, func(m *Model) { m.selectedService = d.Services[0] }},
				{"deployment", ViewDeploymentDetail, func(m *Model) { m.selectedDeployment = d.Deployments[0] }},
				{"statefulset", ViewStatefulSetDetail, func(m *Model) { m.selectedStatefulSet = d.StatefulSets[0] }},
				{"daemonset", ViewDaemonSetDetail, func(m *Model) { m.selectedDaemonSet = d.DaemonSets[0] }},
				{"job", ViewJobDetail, func(m *Model) { m.selectedJob = d.Jobs[0] }},
				{"cronjob", ViewCronJobDetail, func(m *Model) { m.selectedCronJob = d.CronJobs[0] }},
				{"volcanojob", ViewVolcanoJobDetail, func(m *Model) { m.selectedVolcanoJob = d.VolcanoJobs[0] }},
				{"pv", ViewPVDetail, func(m *Model) { m.selectedPV = d.PVs[0] }},
				{"pvc", ViewPVCDetail, func(m *Model) { m.selectedPVC = d.PVCs[0] }},
				{"queue", ViewQueueDetail, func(m *Model) { m.selectedQueue = d.Queues[0] }},
				{"superpod", ViewTopologyDetail, func(m *Model) {
					m.selectedSuperPod = &SuperPodInfo{ID: "sp-1", NodeCount: 1, NodeIPs: []string{"10.0.0.10"}, TotalNPU: 16, NPUPerNode: 16}
				}},
			}
			for _, dv := range details {
				m := b32vModel(locale, width)
				m.currentView = dv.view
				m.detailMode = true
				dv.set(m)
				emit(tag+"/detail-"+dv.name, m.View())
			}
		}
	}
	return b.String()
}

// TestB32VZeroDriftDump writes the render dump when B32V_OUT is set.
func TestB32VZeroDriftDump(t *testing.T) {
	out := os.Getenv("B32V_OUT")
	if out == "" {
		t.Skip("B32V_OUT not set; differential probe is inert")
	}
	forcedAsciiProfile(t)
	data := b32vRenderAll(t)

	if err := os.WriteFile(out, []byte(data), 0o644); err != nil {
		t.Fatalf("write dump: %v", err)
	}
}
