package ui

// b31_fresh_test.go — batch 31 lockdown: detail views follow the data refresh.
//
// A detail view renders from the pointer captured when the user pressed Enter.
// Every refresh replaces m.clusterData wholesale, so without re-resolving the
// selections the page kept rendering the object captured at entry time (the
// reported defect: node detail froze while the header clock kept ticking).
//
// These tests pin, per class:
//   - the detail page shows the freshly received values after a refresh;
//   - identity keys are namespace-aware where they must be;
//   - an object that disappears from a non-empty collection exits via the Esc
//     transition (field-for-field identical state);
//   - an empty collection is treated as transient and does not evict the user;
//   - the scroll offset survives re-resolution and stays in range;
//   - the Esc transition itself is unchanged by the extraction;
//   - the logs viewer follows the re-pointed pod.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// b31Refresh delivers a successful refresh exactly the way the refresher does.
func b31Refresh(m *Model, data *model.ClusterData) {
	m.Update(clusterDataMsg{data: data})
}

// b31Rendered renders and strips the styling so assertions can look at text.
func b31Rendered(m *Model) string {
	return stripANSI(m.View())
}

// b31Node reads back a node detail fixture with enough fields for the detail
// renderer to draw its resource block.
func b31Node(name string, cpuMilli int64) *model.NodeData {
	return &model.NodeData{
		Name:              name,
		Status:            "Ready",
		CPUUsage:          cpuMilli,
		CPUAllocatable:    1000,
		MemoryUsage:       100,
		MemAllocatable:    1000,
		PodCount:          1,
		PodAllocatable:    10,
		CreationTimestamp: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

// b31OpenNodeDetail drives the real Enter path (list -> detail) so the tests
// exercise the same capture point the user does.
func b31OpenNodeDetail(t *testing.T, data *model.ClusterData) *Model {
	t.Helper()
	m := newTestModel(t)
	m.width = 200
	m.height = 40
	m.currentView = ViewNodes
	m.clusterData = data
	m.View() // populates cachedSortedNodes, which Enter reads
	m.selectedIndex = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewNodeDetail || !m.detailMode {
		t.Fatalf("Enter on the node list did not open the node detail (view=%v detailMode=%v)",
			m.currentView, m.detailMode)
	}
	return m
}

// b31OpenPodDetail drives the real Enter path for a pod.
func b31OpenPodDetail(t *testing.T, data *model.ClusterData, index int) *Model {
	t.Helper()
	m := newTestModel(t)
	m.width = 200
	m.height = 40
	m.currentView = ViewPods
	m.clusterData = data
	m.View() // populates cachedSortedPods, which Enter reads
	m.selectedIndex = index
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewPodDetail || !m.detailMode {
		t.Fatalf("Enter on the pod list did not open the pod detail (view=%v detailMode=%v)",
			m.currentView, m.detailMode)
	}
	return m
}

// b31Pod builds a pod fixture whose fixture values are visible in the detail.
func b31Pod(namespace, name string, restarts int32) *model.PodData {
	return &model.PodData{
		Name:              name,
		Namespace:         namespace,
		Node:              "node-a",
		Phase:             "Running",
		PodIP:             "10.0.0.10",
		HostIP:            "10.0.0.1",
		Containers:        1,
		ReadyContainers:   1,
		RestartCount:      restarts,
		CreationTimestamp: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

// b31DetailState is the slice of model state that the Esc transition owns.
// Two models with equal snapshots are indistinguishable to the user on the
// list they land on (the data they render may differ, the state does not).
type b31DetailState struct {
	view            ViewType
	detailMode      bool
	detailScroll    int
	scrollOffset    int
	selectedIndex   int
	jobPodIndex     int
	volcanoPodIndex int
	fromJob         bool
	fromVolcanoJob  bool
	fromNetwork     bool
	fromNPU         bool
	logsMode        bool

	selectedJobName string
	nodeNil         bool
	podNil          bool
	eventNil        bool
	jobNil          bool
	serviceNil      bool
	deploymentNil   bool
	statefulSetNil  bool
	daemonSetNil    bool
	cronJobNil      bool
	pvNil           bool
	pvcNil          bool
	volcanoJobNil   bool
	queueNil        bool
	superPodNil     bool
}

func b31Snapshot(m *Model) b31DetailState {
	s := b31DetailState{
		view:            m.currentView,
		detailMode:      m.detailMode,
		detailScroll:    m.detailScrollOffset,
		scrollOffset:    m.scrollOffset,
		selectedIndex:   m.selectedIndex,
		jobPodIndex:     m.jobPodSelectedIndex,
		volcanoPodIndex: m.volcanoJobPodSelectedIndex,
		fromJob:         m.fromJobDetail,
		fromVolcanoJob:  m.fromVolcanoJobDetail,
		fromNetwork:     m.fromNetworkView,
		fromNPU:         m.fromNPUView,
		logsMode:        m.logsMode,
		nodeNil:         m.selectedNode == nil,
		podNil:          m.selectedPod == nil,
		eventNil:        m.selectedEvent == nil,
		jobNil:          m.selectedJob == nil,
		serviceNil:      m.selectedService == nil,
		deploymentNil:   m.selectedDeployment == nil,
		statefulSetNil:  m.selectedStatefulSet == nil,
		daemonSetNil:    m.selectedDaemonSet == nil,
		cronJobNil:      m.selectedCronJob == nil,
		pvNil:           m.selectedPV == nil,
		pvcNil:          m.selectedPVC == nil,
		volcanoJobNil:   m.selectedVolcanoJob == nil,
		queueNil:        m.selectedQueue == nil,
		superPodNil:     m.selectedSuperPod == nil,
	}
	if m.selectedJob != nil {
		s.selectedJobName = m.selectedJob.Namespace + "/" + m.selectedJob.Name
	}
	return s
}

// allSelectionsNil reports whether every detail selection was cleared.
func (s b31DetailState) allSelectionsNil() bool {
	return s.nodeNil && s.podNil && s.eventNil && s.jobNil && s.serviceNil &&
		s.deploymentNil && s.statefulSetNil && s.daemonSetNil && s.cronJobNil &&
		s.pvNil && s.pvcNil && s.volcanoJobNil && s.queueNil && s.superPodNil
}

// ---------------------------------------------------------------------------
// Freshness per class
// ---------------------------------------------------------------------------

func TestB31NodeDetailFollowsRefresh(t *testing.T) {
	oldNode := b31Node("node-a", 200) // 20.0% of allocatable
	m := b31OpenNodeDetail(t, &model.ClusterData{Nodes: []*model.NodeData{oldNode}})

	if got := b31Rendered(m); !strings.Contains(got, "20.0%") {
		t.Fatalf("node detail before the refresh does not show the captured CPU usage:\n%s", got)
	}

	freshNode := b31Node("node-a", 900) // 90.0%
	b31Refresh(m, &model.ClusterData{Nodes: []*model.NodeData{freshNode}})

	if m.selectedNode != freshNode {
		t.Fatalf("after the refresh the node detail still points at the object captured at entry")
	}
	got := b31Rendered(m)
	if !strings.Contains(got, "90.0%") {
		t.Errorf("node detail does not show the refreshed CPU usage (90.0%%):\n%s", got)
	}
	if strings.Contains(got, "20.0%") {
		t.Errorf("node detail still shows the stale CPU usage (20.0%%):\n%s", got)
	}
}

func TestB31PodDetailFollowsRefresh(t *testing.T) {
	oldPod := b31Pod("default", "web-0", 1)
	m := b31OpenPodDetail(t, &model.ClusterData{Pods: []*model.PodData{oldPod}}, 0)

	restartsLabel := fmt.Sprintf("%s: 1", m.T("detail.field.restarts"))
	if got := b31Rendered(m); !strings.Contains(got, restartsLabel) {
		t.Fatalf("pod detail before the refresh does not show the captured restart count:\n%s", got)
	}

	freshPod := b31Pod("default", "web-0", 7)
	b31Refresh(m, &model.ClusterData{Pods: []*model.PodData{freshPod}})

	if m.selectedPod != freshPod {
		t.Fatalf("after the refresh the pod detail still points at the object captured at entry")
	}
	got := b31Rendered(m)
	want := fmt.Sprintf("%s: 7", m.T("detail.field.restarts"))
	if !strings.Contains(got, want) {
		t.Errorf("pod detail does not show the refreshed restart count (%q):\n%s", want, got)
	}
}

// TestB31PodIdentityIncludesNamespace pins the identity key: a pod with the
// same name in another namespace must not be picked as the refreshed object.
func TestB31PodIdentityIncludesNamespace(t *testing.T) {
	oldPod := b31Pod("default", "web-0", 1)
	m := b31OpenPodDetail(t, &model.ClusterData{Pods: []*model.PodData{oldPod}}, 0)

	sameNameOtherNS := b31Pod("other", "web-0", 9)
	sameNS := b31Pod("default", "web-0", 3)
	// The other-namespace pod deliberately comes first: a matcher that ignores
	// the namespace binds to it.
	b31Refresh(m, &model.ClusterData{Pods: []*model.PodData{sameNameOtherNS, sameNS}})

	if m.selectedPod != sameNS {
		t.Fatalf("the refresh matched the wrong pod: selected namespace = %q, want %q",
			m.selectedPod.Namespace, sameNS.Namespace)
	}
	if m.selectedPod.RestartCount != 3 {
		t.Errorf("selected pod restarts = %d, want the same-namespace pod's 3",
			m.selectedPod.RestartCount)
	}
}

func TestB31EventDetailFollowsRefresh(t *testing.T) {
	base := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	oldEvent := &model.EventData{
		Type:              "Warning",
		Reason:            "BackOff",
		Message:           "Back-off restarting failed container",
		Count:             1,
		FirstTimestamp:    base,
		LastTimestamp:     base,
		InvolvedObject:    "Pod/web-0",
		InvolvedNamespace: "default",
	}
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.currentView = ViewEvents
	m.clusterData = &model.ClusterData{Events: []*model.EventData{oldEvent}}
	m.View()
	m.selectedIndex = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewEventDetail {
		t.Fatalf("Enter on the event list did not open the event detail (view=%v)", m.currentView)
	}

	freshEvent := &model.EventData{
		Type:              "Warning",
		Reason:            "BackOff",
		Message:           "Back-off restarting failed container",
		Count:             5,
		FirstTimestamp:    base,
		LastTimestamp:     base.Add(10 * time.Second),
		InvolvedObject:    "Pod/web-0",
		InvolvedNamespace: "default",
	}
	b31Refresh(m, &model.ClusterData{Events: []*model.EventData{freshEvent}})

	if m.selectedEvent != freshEvent {
		t.Fatalf("after the refresh the event detail still points at the object captured at entry")
	}
	got := b31Rendered(m)
	wantCount := fmt.Sprintf("%s: 5", m.T("detail.event.count"))
	if !strings.Contains(got, wantCount) {
		t.Errorf("event detail does not show the refreshed count (%q):\n%s", wantCount, got)
	}
}

// TestB31EventGoneReturnsToEvents pins the event rule: a rolled-out event must
// not leave a frozen detail page behind.
func TestB31EventGoneReturnsToEvents(t *testing.T) {
	base := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	oldEvent := &model.EventData{
		Type: "Warning", Reason: "BackOff", Count: 1,
		FirstTimestamp: base, LastTimestamp: base,
		InvolvedObject: "Pod/web-0", InvolvedNamespace: "default",
	}
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.currentView = ViewEvents
	m.clusterData = &model.ClusterData{Events: []*model.EventData{oldEvent}}
	m.View()
	m.selectedIndex = 0
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	otherEvent := &model.EventData{
		Type: "Normal", Reason: "Scheduled", Count: 3,
		FirstTimestamp: base, LastTimestamp: base,
		InvolvedObject: "Pod/other-0", InvolvedNamespace: "default",
	}
	b31Refresh(m, &model.ClusterData{Events: []*model.EventData{otherEvent}})

	if m.currentView != ViewEvents || m.detailMode {
		t.Errorf("an event that rolled out of the window did not exit the detail (view=%v detailMode=%v)",
			m.currentView, m.detailMode)
	}
	if m.selectedEvent != nil {
		t.Errorf("the rolled-out event selection was not cleared")
	}
}

func TestB31WorkloadDetailFollowsRefresh(t *testing.T) {
	cases := []struct {
		name    string
		view    ViewType
		open    func(m *Model, d1 *model.ClusterData)
		refresh func(m *Model) *model.ClusterData
		check   func(t *testing.T, m *Model, fresh *model.ClusterData)
		want    string
	}{
		{
			name: "deployment",
			view: ViewDeploymentDetail,
			open: func(m *Model, d1 *model.ClusterData) {
				m.selectedDeployment = d1.Deployments[0]
			},
			refresh: func(m *Model) *model.ClusterData {
				return &model.ClusterData{Deployments: []*model.DeploymentData{{
					Name: "web", Namespace: "default", Replicas: 7, ReadyReplicas: 7,
				}}}
			},
			check: func(t *testing.T, m *Model, fresh *model.ClusterData) {
				if m.selectedDeployment != fresh.Deployments[0] {
					t.Errorf("deployment detail still points at the stale object")
				}
			},
			want: "7 / 7",
		},
		{
			name: "job",
			view: ViewJobDetail,
			open: func(m *Model, d1 *model.ClusterData) {
				m.selectedJob = d1.Jobs[0]
			},
			refresh: func(m *Model) *model.ClusterData {
				return &model.ClusterData{Jobs: []*model.JobData{{
					Name: "batch", Namespace: "default", Completions: 5, Succeeded: 2, Active: 3,
				}}}
			},
			check: func(t *testing.T, m *Model, fresh *model.ClusterData) {
				if m.selectedJob != fresh.Jobs[0] {
					t.Errorf("job detail still points at the stale object")
				}
			},
			want: "3 active, 2/5 succeeded",
		},
		{
			name: "statefulset",
			view: ViewStatefulSetDetail,
			open: func(m *Model, d1 *model.ClusterData) {
				m.selectedStatefulSet = d1.StatefulSets[0]
			},
			refresh: func(m *Model) *model.ClusterData {
				return &model.ClusterData{StatefulSets: []*model.StatefulSetData{{
					Name: "db", Namespace: "default", Replicas: 2, ReadyReplicas: 2,
				}}}
			},
			check: func(t *testing.T, m *Model, fresh *model.ClusterData) {
				if m.selectedStatefulSet != fresh.StatefulSets[0] {
					t.Errorf("statefulset detail still points at the stale object")
				}
			},
			want: "2 / 2",
		},
		{
			name: "daemonset",
			view: ViewDaemonSetDetail,
			open: func(m *Model, d1 *model.ClusterData) {
				m.selectedDaemonSet = d1.DaemonSets[0]
			},
			refresh: func(m *Model) *model.ClusterData {
				return &model.ClusterData{DaemonSets: []*model.DaemonSetData{{
					Name: "agent", Namespace: "kube-system",
					DesiredNumberScheduled: 6, CurrentNumberScheduled: 6, NumberReady: 6,
				}}}
			},
			check: func(t *testing.T, m *Model, fresh *model.ClusterData) {
				if m.selectedDaemonSet != fresh.DaemonSets[0] {
					t.Errorf("daemonset detail still points at the stale object")
				}
			},
			want: "6 / 6",
		},
		{
			name: "cronjob",
			view: ViewCronJobDetail,
			open: func(m *Model, d1 *model.ClusterData) {
				m.selectedCronJob = d1.CronJobs[0]
			},
			refresh: func(m *Model) *model.ClusterData {
				return &model.ClusterData{CronJobs: []*model.CronJobData{{
					Name: "nightly", Namespace: "default", Schedule: "0 2 * * *", Active: 2,
				}}}
			},
			check: func(t *testing.T, m *Model, fresh *model.ClusterData) {
				if m.selectedCronJob != fresh.CronJobs[0] {
					t.Errorf("cronjob detail still points at the stale object")
				}
			},
			want: "Active Jobs: 2",
		},
	}

	// Secret-free fixtures: the refresh data is built inside each case.
	initial := func(view ViewType) *model.ClusterData {
		return &model.ClusterData{
			Deployments:  []*model.DeploymentData{{Name: "web", Namespace: "default", Replicas: 2, ReadyReplicas: 2}},
			Jobs:         []*model.JobData{{Name: "batch", Namespace: "default", Completions: 5, Succeeded: 1, Active: 1}},
			StatefulSets: []*model.StatefulSetData{{Name: "db", Namespace: "default", Replicas: 2, ReadyReplicas: 1}},
			DaemonSets:   []*model.DaemonSetData{{Name: "agent", Namespace: "kube-system", DesiredNumberScheduled: 4, CurrentNumberScheduled: 4, NumberReady: 1}},
			CronJobs:     []*model.CronJobData{{Name: "nightly", Namespace: "default", Schedule: "0 2 * * *"}},
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width, m.height = 200, 40
			d1 := initial(tc.view)
			m.clusterData = d1
			m.currentView = tc.view
			m.detailMode = true
			tc.open(m, d1)

			fresh := tc.refresh(m)
			b31Refresh(m, fresh)

			tc.check(t, m, fresh)
			if got := b31Rendered(m); !strings.Contains(got, tc.want) {
				t.Errorf("%s detail does not show the refreshed value %q:\n%s", tc.name, tc.want, got)
			}
		})
	}
}

func TestB31StorageDetailFollowsRefresh(t *testing.T) {
	t.Run("pv", func(t *testing.T) {
		oldPV := &model.PVData{Name: "pv-a", Status: "Bound", StorageClass: "standard", Capacity: 1 << 30}
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.clusterData = &model.ClusterData{PVs: []*model.PVData{oldPV}}
		m.currentView = ViewPVDetail
		m.detailMode = true
		m.selectedPV = oldPV

		freshPV := &model.PVData{Name: "pv-a", Status: "Bound", StorageClass: "gold", Capacity: 1 << 30}
		b31Refresh(m, &model.ClusterData{PVs: []*model.PVData{freshPV}})

		if m.selectedPV != freshPV {
			t.Errorf("pv detail still points at the stale object")
		}
		want := fmt.Sprintf("%s: gold", m.T("detail.pv.storageclass"))
		if got := b31Rendered(m); !strings.Contains(got, want) {
			t.Errorf("pv detail does not show the refreshed storage class (%q):\n%s", want, got)
		}
	})

	t.Run("pvc", func(t *testing.T) {
		oldPVC := &model.PVCData{Name: "data", Namespace: "default", Status: "Pending"}
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.clusterData = &model.ClusterData{PVCs: []*model.PVCData{oldPVC}}
		m.currentView = ViewPVCDetail
		m.detailMode = true
		m.selectedPVC = oldPVC

		freshPVC := &model.PVCData{Name: "data", Namespace: "default", Status: "Bound"}
		b31Refresh(m, &model.ClusterData{PVCs: []*model.PVCData{freshPVC}})

		if m.selectedPVC != freshPVC {
			t.Errorf("pvc detail still points at the stale object")
		}
		want := fmt.Sprintf("%s: Bound", m.T("detail.pvc.status"))
		if got := b31Rendered(m); !strings.Contains(got, want) {
			t.Errorf("pvc detail does not show the refreshed status (%q):\n%s", want, got)
		}
	})

	t.Run("service", func(t *testing.T) {
		oldSvc := &model.ServiceData{Name: "api", Namespace: "default", Type: "ClusterIP", EndpointCount: 1}
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.clusterData = &model.ClusterData{Services: []*model.ServiceData{oldSvc}}
		m.currentView = ViewServiceDetail
		m.detailMode = true
		m.selectedService = oldSvc

		freshSvc := &model.ServiceData{Name: "api", Namespace: "default", Type: "ClusterIP", EndpointCount: 7}
		b31Refresh(m, &model.ClusterData{Services: []*model.ServiceData{freshSvc}})

		if m.selectedService != freshSvc {
			t.Errorf("service detail still points at the stale object")
		}
		want := m.TF("service.ready_endpoints", map[string]interface{}{"Count": 7})
		if got := b31Rendered(m); !strings.Contains(got, want) {
			t.Errorf("service detail does not show the refreshed endpoint count (%q):\n%s", want, got)
		}
	})
}

func TestB31VolcanoDetailFollowsRefresh(t *testing.T) {
	t.Run("queue", func(t *testing.T) {
		oldQueue := &model.QueueData{Name: "default", State: "Open", Weight: 1}
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.clusterData = &model.ClusterData{Queues: []*model.QueueData{oldQueue}}
		m.currentView = ViewQueueDetail
		m.detailMode = true
		m.selectedQueue = oldQueue

		freshQueue := &model.QueueData{Name: "default", State: "Open", Weight: 5}
		b31Refresh(m, &model.ClusterData{Queues: []*model.QueueData{freshQueue}})

		if m.selectedQueue != freshQueue {
			t.Errorf("queue detail still points at the stale object")
		}
		want := fmt.Sprintf("%s: 5", m.T("detail.queue.weight"))
		if got := b31Rendered(m); !strings.Contains(got, want) {
			t.Errorf("queue detail does not show the refreshed weight (%q):\n%s", want, got)
		}
	})

	t.Run("volcanojob", func(t *testing.T) {
		oldJob := &model.VolcanoJobData{Name: "vj", Namespace: "default", Status: "Running", Replicas: 4, Running: 1}
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.clusterData = &model.ClusterData{VolcanoJobs: []*model.VolcanoJobData{oldJob}}
		m.currentView = ViewVolcanoJobDetail
		m.detailMode = true
		m.selectedVolcanoJob = oldJob

		freshJob := &model.VolcanoJobData{Name: "vj", Namespace: "default", Status: "Running", Replicas: 4, Running: 3}
		b31Refresh(m, &model.ClusterData{VolcanoJobs: []*model.VolcanoJobData{freshJob}})

		if m.selectedVolcanoJob != freshJob {
			t.Errorf("volcano job detail still points at the stale object")
		}
		if got := b31Rendered(m); !strings.Contains(got, "3/4") {
			t.Errorf("volcano job detail does not show the refreshed running count (3/4):\n%s", got)
		}
	})

	t.Run("superpod", func(t *testing.T) {
		node := b31Node("node-a", 200)
		node.SuperPodID = "sp-1"
		node.NPUAllocatable = 8
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.clusterData = &model.ClusterData{Nodes: []*model.NodeData{node}}
		m.currentView = ViewTopology
		m.View()
		m.selectedIndex = 0
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.currentView != ViewTopologyDetail || m.selectedSuperPod == nil {
			t.Fatalf("Enter on the topology view did not open the SuperPod detail (view=%v)", m.currentView)
		}
		oldSelection := m.selectedSuperPod

		freshNode := b31Node("node-a", 200)
		freshNode.SuperPodID = "sp-1"
		freshNode.NPUAllocatable = 16
		b31Refresh(m, &model.ClusterData{Nodes: []*model.NodeData{freshNode}})

		if m.selectedSuperPod == nil || m.selectedSuperPod == oldSelection {
			t.Fatalf("the SuperPod selection was not re-pointed at the fresh aggregate")
		}
		if m.selectedSuperPod.ID != "sp-1" || m.selectedSuperPod.TotalNPU != 16 {
			t.Errorf("re-pointed SuperPod = ID %q TotalNPU %d, want sp-1 / 16",
				m.selectedSuperPod.ID, m.selectedSuperPod.TotalNPU)
		}
		if got := b31Rendered(m); !strings.Contains(got, "16 NPU") {
			t.Errorf("SuperPod detail does not show the refreshed NPU total (16 NPU):\n%s", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Disappearance, transient emptiness, scroll
// ---------------------------------------------------------------------------

// TestB31GoneNodeMatchesEscState pins that a node that disappears returns the
// model to exactly the state Esc produces.
func TestB31GoneNodeMatchesEscState(t *testing.T) {
	data := func() *model.ClusterData {
		return &model.ClusterData{Nodes: []*model.NodeData{b31Node("node-a", 200)}}
	}
	auto := b31OpenNodeDetail(t, data())
	manual := b31OpenNodeDetail(t, data())

	b31Refresh(auto, &model.ClusterData{Nodes: []*model.NodeData{b31Node("node-b", 100)}})
	manual.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if got, want := b31Snapshot(auto), b31Snapshot(manual); got != want {
		t.Errorf("auto-exit state differs from Esc state:\n auto = %+v\n   esc = %+v", got, want)
	}
	if auto.currentView != ViewNodes || auto.detailMode {
		t.Errorf("gone node did not return to the node list (view=%v detailMode=%v)",
			auto.currentView, auto.detailMode)
	}
}

// TestB31GonePodMatchesEscState is the pod analogue, including the parent-list
// selection index the Esc transition resets.
func TestB31GonePodMatchesEscState(t *testing.T) {
	data := func() *model.ClusterData {
		return &model.ClusterData{Pods: []*model.PodData{
			b31Pod("default", "web-0", 1),
			b31Pod("default", "web-1", 1),
		}}
	}
	auto := b31OpenPodDetail(t, data(), 1)
	manual := b31OpenPodDetail(t, data(), 1)

	b31Refresh(auto, &model.ClusterData{Pods: []*model.PodData{b31Pod("default", "web-9", 2)}})
	manual.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if got, want := b31Snapshot(auto), b31Snapshot(manual); got != want {
		t.Errorf("auto-exit state differs from Esc state:\n auto = %+v\n   esc = %+v", got, want)
	}
	if auto.currentView != ViewPods || auto.detailMode {
		t.Errorf("gone pod did not return to the pod list (view=%v detailMode=%v)",
			auto.currentView, auto.detailMode)
	}
}

// TestB31GonePodFromJobMatchesEscState covers the Job -> Pod chain: the exit
// must land back on the Job detail exactly as Esc does, keeping the Job.
func TestB31GonePodFromJobMatchesEscState(t *testing.T) {
	setup := func() *Model {
		m := newTestModel(t)
		m.width, m.height = 200, 40
		job := &model.JobData{Name: "batch", Namespace: "default", Completions: 5, Active: 1}
		m.clusterData = &model.ClusterData{
			Jobs: []*model.JobData{job},
			Pods: []*model.PodData{b31Pod("default", "batch-x1", 1)},
		}
		m.selectedJob = job
		m.selectedPod = m.clusterData.Pods[0]
		m.currentView = ViewPodDetail
		m.detailMode = true
		m.fromJobDetail = true
		return m
	}
	auto := setup()
	manual := setup()

	b31Refresh(auto, &model.ClusterData{
		Jobs: []*model.JobData{{Name: "batch", Namespace: "default", Completions: 5, Succeeded: 5}},
		Pods: []*model.PodData{b31Pod("default", "other-x1", 1)},
	})
	manual.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if got, want := b31Snapshot(auto), b31Snapshot(manual); got != want {
		t.Errorf("auto-exit state differs from Esc state:\n auto = %+v\n   esc = %+v", got, want)
	}
	if auto.currentView != ViewJobDetail {
		t.Errorf("a pod opened from the Job detail did not return to it (view=%v)", auto.currentView)
	}
	if auto.selectedJob == nil || auto.selectedJob.Name != "batch" {
		t.Errorf("the parent Job selection was dropped while backing out of the pod detail")
	}
}

// TestB31JobAndPodGoneBacksOutFully covers the broken chain: when the parent
// Job is gone too, the model must keep backing out to the Workloads list.
func TestB31JobAndPodGoneBacksOutFully(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 200, 40
	job := &model.JobData{Name: "batch", Namespace: "default", Completions: 5, Active: 1}
	m.clusterData = &model.ClusterData{
		Jobs: []*model.JobData{job},
		Pods: []*model.PodData{b31Pod("default", "batch-x1", 1)},
	}
	m.selectedJob = job
	m.selectedPod = m.clusterData.Pods[0]
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.fromJobDetail = true

	b31Refresh(m, &model.ClusterData{
		Jobs: []*model.JobData{{Name: "another", Namespace: "default"}},
		Pods: []*model.PodData{b31Pod("default", "another-x1", 1)},
	})

	if m.currentView != ViewWorkloads || m.detailMode {
		t.Errorf("both the pod and its parent job are gone but the view is %v (detailMode=%v)",
			m.currentView, m.detailMode)
	}
	if m.selectedPod != nil || m.selectedJob != nil {
		t.Errorf("selections survived the back-out: pod=%v job=%v", m.selectedPod, m.selectedJob)
	}
}

// TestB31EveryGoneDetailReturnsToItsList walks the remaining classes one by
// one: an object that disappears from a non-empty collection must land on the
// list that detail view is opened from.
func TestB31EveryGoneDetailReturnsToItsList(t *testing.T) {
	type goneCase struct {
		name        string
		view        ViewType
		setup       func(m *Model) *model.ClusterData
		fresh       *model.ClusterData
		want        ViewType
		fromNetwork bool
	}
	cases := []goneCase{
		{
			name: "job",
			view: ViewJobDetail,
			setup: func(m *Model) *model.ClusterData {
				job := &model.JobData{Name: "batch", Namespace: "default"}
				m.selectedJob = job
				return &model.ClusterData{Jobs: []*model.JobData{job}}
			},
			fresh: &model.ClusterData{Jobs: []*model.JobData{{Name: "other", Namespace: "default"}}},
			want:  ViewWorkloads,
		},
		{
			name: "service from workloads",
			view: ViewServiceDetail,
			setup: func(m *Model) *model.ClusterData {
				svc := &model.ServiceData{Name: "api", Namespace: "default"}
				m.selectedService = svc
				return &model.ClusterData{Services: []*model.ServiceData{svc}}
			},
			fresh: &model.ClusterData{Services: []*model.ServiceData{{Name: "other", Namespace: "default"}}},
			want:  ViewWorkloads,
		},
		{
			name: "service from network",
			view: ViewServiceDetail,
			setup: func(m *Model) *model.ClusterData {
				svc := &model.ServiceData{Name: "api", Namespace: "default"}
				m.selectedService = svc
				return &model.ClusterData{Services: []*model.ServiceData{svc}}
			},
			fresh:       &model.ClusterData{Services: []*model.ServiceData{{Name: "other", Namespace: "default"}}},
			want:        ViewNetwork,
			fromNetwork: true,
		},
		{
			name: "deployment",
			view: ViewDeploymentDetail,
			setup: func(m *Model) *model.ClusterData {
				deploy := &model.DeploymentData{Name: "web", Namespace: "default"}
				m.selectedDeployment = deploy
				return &model.ClusterData{Deployments: []*model.DeploymentData{deploy}}
			},
			fresh: &model.ClusterData{Deployments: []*model.DeploymentData{{Name: "other", Namespace: "default"}}},
			want:  ViewWorkloads,
		},
		{
			name: "statefulset",
			view: ViewStatefulSetDetail,
			setup: func(m *Model) *model.ClusterData {
				sts := &model.StatefulSetData{Name: "db", Namespace: "default"}
				m.selectedStatefulSet = sts
				return &model.ClusterData{StatefulSets: []*model.StatefulSetData{sts}}
			},
			fresh: &model.ClusterData{StatefulSets: []*model.StatefulSetData{{Name: "other", Namespace: "default"}}},
			want:  ViewWorkloads,
		},
		{
			name: "daemonset",
			view: ViewDaemonSetDetail,
			setup: func(m *Model) *model.ClusterData {
				ds := &model.DaemonSetData{Name: "agent", Namespace: "kube-system"}
				m.selectedDaemonSet = ds
				return &model.ClusterData{DaemonSets: []*model.DaemonSetData{ds}}
			},
			fresh: &model.ClusterData{DaemonSets: []*model.DaemonSetData{{Name: "other", Namespace: "kube-system"}}},
			want:  ViewWorkloads,
		},
		{
			name: "cronjob",
			view: ViewCronJobDetail,
			setup: func(m *Model) *model.ClusterData {
				cj := &model.CronJobData{Name: "nightly", Namespace: "default"}
				m.selectedCronJob = cj
				return &model.ClusterData{CronJobs: []*model.CronJobData{cj}}
			},
			fresh: &model.ClusterData{CronJobs: []*model.CronJobData{{Name: "other", Namespace: "default"}}},
			want:  ViewWorkloads,
		},
		{
			name: "pv",
			view: ViewPVDetail,
			setup: func(m *Model) *model.ClusterData {
				pv := &model.PVData{Name: "pv-a"}
				m.selectedPV = pv
				return &model.ClusterData{PVs: []*model.PVData{pv}}
			},
			fresh: &model.ClusterData{PVs: []*model.PVData{{Name: "pv-b"}}},
			want:  ViewStorage,
		},
		{
			name: "pvc",
			view: ViewPVCDetail,
			setup: func(m *Model) *model.ClusterData {
				pvc := &model.PVCData{Name: "data", Namespace: "default"}
				m.selectedPVC = pvc
				return &model.ClusterData{PVCs: []*model.PVCData{pvc}}
			},
			fresh: &model.ClusterData{PVCs: []*model.PVCData{{Name: "other", Namespace: "default"}}},
			want:  ViewStorage,
		},
		{
			name: "volcanojob",
			view: ViewVolcanoJobDetail,
			setup: func(m *Model) *model.ClusterData {
				vj := &model.VolcanoJobData{Name: "vj", Namespace: "default"}
				m.selectedVolcanoJob = vj
				return &model.ClusterData{VolcanoJobs: []*model.VolcanoJobData{vj}}
			},
			fresh: &model.ClusterData{VolcanoJobs: []*model.VolcanoJobData{{Name: "other", Namespace: "default"}}},
			want:  ViewWorkloads,
		},
		{
			name: "queue",
			view: ViewQueueDetail,
			setup: func(m *Model) *model.ClusterData {
				queue := &model.QueueData{Name: "default"}
				m.selectedQueue = queue
				return &model.ClusterData{Queues: []*model.QueueData{queue}}
			},
			fresh: &model.ClusterData{Queues: []*model.QueueData{{Name: "other"}}},
			want:  ViewQueues,
		},
		{
			name: "superpod",
			view: ViewTopologyDetail,
			setup: func(m *Model) *model.ClusterData {
				node := b31Node("node-a", 200)
				node.SuperPodID = "sp-1"
				node.NPUAllocatable = 8
				cd := &model.ClusterData{Nodes: []*model.NodeData{node}}
				// The SuperPod selection is an aggregate over the node list,
				// so it can only be built once that list is in place.
				m.clusterData = cd
				if sps := m.getSuperPodTopology(); len(sps) > 0 {
					m.selectedSuperPod = &sps[0]
				}
				return cd
			},
			fresh: func() *model.ClusterData {
				node := b31Node("node-a", 200)
				node.SuperPodID = "sp-2"
				return &model.ClusterData{Nodes: []*model.NodeData{node}}
			}(),
			want: ViewTopology,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width, m.height = 200, 40
			m.currentView = tc.view
			m.detailMode = true
			m.fromNetworkView = tc.fromNetwork
			m.clusterData = tc.setup(m)
			if tc.view == ViewTopologyDetail && m.selectedSuperPod == nil {
				t.Fatalf("the SuperPod fixture did not produce a selection")
			}

			b31Refresh(m, tc.fresh)

			if m.currentView != tc.want || m.detailMode {
				t.Errorf("a vanished object left the model in view=%v detailMode=%v, want view=%v on a list",
					m.currentView, m.detailMode, tc.want)
			}
			if snap := b31Snapshot(m); !snap.allSelectionsNil() {
				t.Errorf("a vanished object left a stale selection behind: %+v", snap)
			}
		})
	}
}

// TestB31TransientEmptyFreezesDetail pins the rule that an empty collection is
// a transient API result, not a deletion: the detail stays put.
func TestB31TransientEmptyFreezesDetail(t *testing.T) {
	t.Run("node", func(t *testing.T) {
		oldNode := b31Node("node-a", 200)
		m := b31OpenNodeDetail(t, &model.ClusterData{Nodes: []*model.NodeData{oldNode}})

		b31Refresh(m, &model.ClusterData{})

		if m.currentView != ViewNodeDetail || !m.detailMode {
			t.Fatalf("an empty node list evicted the user from the node detail (view=%v detailMode=%v)",
				m.currentView, m.detailMode)
		}
		if m.selectedNode != oldNode {
			t.Errorf("the frozen node selection was changed by a transient empty refresh")
		}
		if got := b31Rendered(m); !strings.Contains(got, "20.0%") {
			t.Errorf("the frozen node detail no longer renders its captured data:\n%s", got)
		}
	})

	t.Run("pod", func(t *testing.T) {
		oldPod := b31Pod("default", "web-0", 1)
		m := b31OpenPodDetail(t, &model.ClusterData{Pods: []*model.PodData{oldPod}}, 0)

		b31Refresh(m, &model.ClusterData{})

		if m.currentView != ViewPodDetail || !m.detailMode {
			t.Fatalf("an empty pod list evicted the user from the pod detail (view=%v detailMode=%v)",
				m.currentView, m.detailMode)
		}
		if m.selectedPod != oldPod {
			t.Errorf("the frozen pod selection was changed by a transient empty refresh")
		}
	})
}

// TestB31ScrollSurvivesReresolution pins that re-pointing does not reset the
// detail scroll offset and that a shorter refreshed page stays in range (the
// renderers clamp the offset they are handed).
func TestB31ScrollSurvivesReresolution(t *testing.T) {
	oldNode := b31Node("node-a", 200)
	oldNode.PodCount = 30
	// Thirty pods on the node make the first render long; the refreshed
	// snapshot carries none, so the content shrinks underneath the offset.
	d1 := &model.ClusterData{Nodes: []*model.NodeData{oldNode}}
	for i := 0; i < 30; i++ {
		d1.Pods = append(d1.Pods, b31Pod("default", fmt.Sprintf("web-%d", i), 0))
	}
	m := b31OpenNodeDetail(t, d1)

	m.detailScrollOffset = 0
	beforeLimit := m.detailScrollLimit(1000) // the last page of the long content
	m.detailScrollOffset = 100               // far past any page; the renderer clamps it
	if beforeLimit == 0 {
		t.Fatalf("the long node detail did not produce a scroll window to shrink from")
	}

	freshNode := b31Node("node-a", 900)
	b31Refresh(m, &model.ClusterData{Nodes: []*model.NodeData{freshNode}})

	if m.detailScrollOffset != 100 {
		t.Fatalf("re-resolution reset the detail scroll offset to %d, want 100", m.detailScrollOffset)
	}
	afterLimit := m.detailScrollLimit(1000)
	if afterLimit >= beforeLimit {
		t.Fatalf("the refreshed content did not shrink: last page %d -> %d", beforeLimit, afterLimit)
	}
	clamped := m.detailScrollLimit(100)
	if clamped >= 100 {
		t.Fatalf("the refreshed content did not shrink below the offset; limit = %d", clamped)
	}
	if got, want := m.renderDetailAt(100), m.renderDetailAt(clamped); got != want {
		t.Errorf("an oversized offset renders past the end of the refreshed content")
	}
	m.View() // must not panic on the dangling offset

	// The same page must render identically on a second pass (no map-order
	// flicker for the offset clamp to trip over).
	if first, second := m.View(), m.View(); first != second {
		t.Errorf("the refreshed detail renders differently on two consecutive frames")
	}
}

// ---------------------------------------------------------------------------
// Esc zero drift and the logs viewer
// ---------------------------------------------------------------------------

// TestB31EscZeroDriftAfterReresolution pins that extracting the Esc transition
// into a helper did not change what Esc does when a re-resolution preceded it.
func TestB31EscZeroDriftAfterReresolution(t *testing.T) {
	setup := func() *Model {
		return b31OpenNodeDetail(t, &model.ClusterData{Nodes: []*model.NodeData{b31Node("node-a", 200)}})
	}

	t.Run("after refresh", func(t *testing.T) {
		refreshed := setup()
		plain := setup()

		b31Refresh(refreshed, &model.ClusterData{Nodes: []*model.NodeData{b31Node("node-a", 900)}})
		refreshed.Update(tea.KeyMsg{Type: tea.KeyEsc})
		plain.Update(tea.KeyMsg{Type: tea.KeyEsc})

		if got, want := b31Snapshot(refreshed), b31Snapshot(plain); got != want {
			t.Errorf("Esc after a re-resolution differs from Esc without one:\n after = %+v\n plain = %+v", got, want)
		}
	})

	t.Run("from NPU view", func(t *testing.T) {
		setupNPU := func() *Model {
			m := newTestModel(t)
			m.width, m.height = 200, 40
			node := b31Node("node-a", 200)
			m.clusterData = &model.ClusterData{Nodes: []*model.NodeData{node}}
			m.currentView = ViewNodeDetail
			m.detailMode = true
			m.selectedNode = node
			m.fromNPUView = true
			m.selectedIndex = 2
			m.scrollOffset = 3
			return m
		}
		refreshed := setupNPU()
		plain := setupNPU()

		b31Refresh(refreshed, &model.ClusterData{Nodes: []*model.NodeData{b31Node("node-a", 900)}})
		refreshed.Update(tea.KeyMsg{Type: tea.KeyEsc})
		plain.Update(tea.KeyMsg{Type: tea.KeyEsc})

		if got, want := b31Snapshot(refreshed), b31Snapshot(plain); got != want {
			t.Errorf("Esc back to the NPU view changed after a re-resolution:\n after = %+v\n plain = %+v", got, want)
		}
		if refreshed.currentView != ViewNPU {
			t.Errorf("Esc from a node detail opened in the NPU view landed on %v, want ViewNPU", refreshed.currentView)
		}
	})
}

// TestB31LogsFollowReresolvedPod pins that the logs viewer keeps following the
// selected pod after the pointer is swapped.
func TestB31LogsFollowReresolvedPod(t *testing.T) {
	oldPod := b31Pod("default", "web-0", 1)
	oldPod.ContainerStates = []model.ContainerState{{Name: "app", Ready: true}}
	m := b31OpenPodDetail(t, &model.ClusterData{Pods: []*model.PodData{oldPod}}, 0)

	m.Update(keyMsg("l"))
	if !m.logsMode {
		t.Fatalf("'l' on the pod detail did not open the logs viewer")
	}

	freshPod := b31Pod("default", "web-0", 1)
	freshPod.ContainerStates = []model.ContainerState{{Name: "app", Ready: true}}
	b31Refresh(m, &model.ClusterData{Pods: []*model.PodData{freshPod}})

	if m.selectedPod != freshPod {
		t.Fatalf("the logs viewer's pod was not re-pointed at the fresh object")
	}
	if !m.logsMode {
		t.Errorf("the refresh closed the logs viewer")
	}
}

// TestB31LogsExitWhenPodDisappears pins that a pod disappearing under the logs
// viewer closes the viewer and backs out with Esc semantics.
func TestB31LogsExitWhenPodDisappears(t *testing.T) {
	oldPod := b31Pod("default", "web-0", 1)
	oldPod.ContainerStates = []model.ContainerState{{Name: "app", Ready: true}}
	m := b31OpenPodDetail(t, &model.ClusterData{Pods: []*model.PodData{oldPod}}, 0)
	m.Update(keyMsg("l"))
	if !m.logsMode {
		t.Fatalf("'l' on the pod detail did not open the logs viewer")
	}

	b31Refresh(m, &model.ClusterData{Pods: []*model.PodData{b31Pod("default", "web-1", 1)}})

	if m.logsMode || m.currentView != ViewPods || m.detailMode {
		t.Errorf("a vanished pod left the model in view=%v detailMode=%v logsMode=%v",
			m.currentView, m.detailMode, m.logsMode)
	}
}
