package ui

// b31v_probe_test.go — independent adversarial probes for batch 31
// (detail views re-resolve their selection on every refresh).
//
// These probes are written from the contract, not from the implementation:
//   - the Esc transition must not have changed shape (esc-dump differential);
//   - identity keys must be complete (events) and namespace-aware (pods);
//   - "empty collection = transient" must hold for all 14 detail classes;
//   - the logs viewer's search state must not survive an auto-exit;
//   - a node detail opened from the NPU view must land on a renderable NPU view.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

func b31vPod(ns, name string, restarts int32) *model.PodData {
	return &model.PodData{
		Name: name, Namespace: ns, Node: "node-a", Phase: "Running",
		PodIP: "10.0.0.10", HostIP: "10.0.0.1", Containers: 1, ReadyContainers: 1,
		RestartCount:      restarts,
		CreationTimestamp: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

func b31vNode(name string) *model.NodeData {
	return &model.NodeData{
		Name: name, Status: "Ready", CPUUsage: 200, CPUAllocatable: 1000,
		MemoryUsage: 100, MemAllocatable: 1000, PodCount: 1, PodAllocatable: 10,
		CreationTimestamp: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

// b31vOpenPod enters the pod detail through the real Enter path.
func b31vOpenPod(t *testing.T, cd *model.ClusterData, index int) *Model {
	t.Helper()
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.currentView = ViewPods
	m.clusterData = cd
	m.View()
	m.selectedIndex = index
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.currentView != ViewPodDetail || !m.detailMode {
		t.Fatalf("Enter did not open the pod detail (view=%v detailMode=%v)", m.currentView, m.detailMode)
	}
	return m
}

// ---------------------------------------------------------------------------
// 1. The logs search state must be gone after an auto-exit, not just the
//    logs mode: a surviving logsSearchMode swallows every single-key binding
//    (including q) with no visible search bar to explain it.
// ---------------------------------------------------------------------------

func TestB31VLogsSearchStateClearedOnAutoExit(t *testing.T) {
	pod := b31vPod("default", "web-0", 1)
	pod.ContainerStates = []model.ContainerState{{Name: "app", Ready: true}}
	m := b31vOpenPod(t, &model.ClusterData{Pods: []*model.PodData{pod}}, 0)

	m.Update(keyMsg("l"))
	if !m.logsMode {
		t.Fatalf("'l' did not open the logs viewer")
	}
	m.Update(keyMsg("/"))
	if !m.logsSearchMode {
		t.Fatalf("'/' did not open the logs search")
	}
	for _, r := range "err" {
		m.Update(keyMsg(string(r)))
	}
	if m.logsSearchText != "err" {
		t.Fatalf("logs search text = %q, want err", m.logsSearchText)
	}

	// The pod vanishes while the user is typing in the logs search.
	b31RefreshProbe(m, &model.ClusterData{Pods: []*model.PodData{b31vPod("default", "web-1", 1)}})

	if m.logsMode {
		t.Errorf("the logs viewer survived the pod's disappearance")
	}
	if m.logsSearchMode {
		t.Errorf("logsSearchMode survived the auto-exit; every single-key binding is now swallowed")
	}
	if m.logsSearchText != "" {
		t.Errorf("logsSearchText survived the auto-exit: %q", m.logsSearchText)
	}
	if m.currentView != ViewPods || m.detailMode {
		t.Errorf("auto-exit landed on view=%v detailMode=%v, want ViewPods", m.currentView, m.detailMode)
	}

	// Observable consequence: with no overlay open, q must quit.
	m.Update(keyMsg("q"))
	if !m.quitting {
		t.Errorf("'q' did not quit after the auto-exit: a phantom logs search swallowed it")
	}
}

// b31RefreshProbe feeds a successful refresh through the real message handler.
func b31RefreshProbe(m *Model, data *model.ClusterData) {
	m.Update(clusterDataMsg{data: data})
}

// ---------------------------------------------------------------------------
// 2. Event identity: Type+Reason+InvolvedObject+InvolvedNamespace. Same key
//    with a new message must follow; same Type+Reason on another object must
//    NOT be bound to.
// ---------------------------------------------------------------------------

func TestB31VEventIdentityKey(t *testing.T) {
	base := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	mk := func(typ, reason, obj, ns, msg string, count int32) *model.EventData {
		return &model.EventData{
			Type: typ, Reason: reason, InvolvedObject: obj, InvolvedNamespace: ns,
			Message: msg, Count: count, FirstTimestamp: base, LastTimestamp: base,
		}
	}
	openWith := func(ev *model.EventData) *Model {
		m := newTestModel(t)
		m.width, m.height = 200, 40
		m.currentView = ViewEvents
		m.clusterData = &model.ClusterData{Events: []*model.EventData{ev}}
		m.View()
		m.selectedIndex = 0
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.currentView != ViewEventDetail || !m.detailMode {
			t.Fatalf("Enter did not open the event detail (view=%v)", m.currentView)
		}
		return m
	}

	t.Run("message change follows", func(t *testing.T) {
		m := openWith(mk("Warning", "BackOff", "Pod/web-0", "default", "old text", 1))
		fresh := mk("Warning", "BackOff", "Pod/web-0", "default", "new text", 4)
		b31RefreshProbe(m, &model.ClusterData{Events: []*model.EventData{fresh}})
		if m.selectedEvent != fresh {
			t.Fatalf("the event was not re-pointed although its identity key matched")
		}
	})

	t.Run("same type and reason on another object is not bound", func(t *testing.T) {
		m := openWith(mk("Warning", "BackOff", "Pod/web-0", "default", "text", 1))
		other := mk("Warning", "BackOff", "Pod/web-9", "default", "text", 9)
		b31RefreshProbe(m, &model.ClusterData{Events: []*model.EventData{other}})
		if m.detailMode || m.currentView != ViewEvents {
			t.Errorf("an event with the same type+reason but another involved object kept the detail open (view=%v)", m.currentView)
		}
		if m.selectedEvent != nil {
			t.Errorf("the stale event selection survived")
		}
	})

	t.Run("same object in another namespace is not bound", func(t *testing.T) {
		m := openWith(mk("Warning", "BackOff", "Pod/web-0", "default", "text", 1))
		other := mk("Warning", "BackOff", "Pod/web-0", "other", "text", 9)
		b31RefreshProbe(m, &model.ClusterData{Events: []*model.EventData{other}})
		if m.detailMode {
			t.Errorf("an event with the same object name in another namespace kept the detail open")
		}
	})
}

// ---------------------------------------------------------------------------
// 3. "Empty collection = transient, freeze" must be uniform across all 14.
// ---------------------------------------------------------------------------

func TestB31VTransientEmptyFreezesAllClasses(t *testing.T) {
	type tc struct {
		name  string
		view  ViewType
		setup func(m *Model) (*model.ClusterData, interface{})
	}
	cases := []tc{
		{"node", ViewNodeDetail, func(m *Model) (*model.ClusterData, interface{}) {
			n := b31vNode("node-a")
			m.selectedNode = n
			return &model.ClusterData{Nodes: []*model.NodeData{n}}, n
		}},
		{"pod", ViewPodDetail, func(m *Model) (*model.ClusterData, interface{}) {
			p := b31vPod("default", "web-0", 1)
			m.selectedPod = p
			return &model.ClusterData{Pods: []*model.PodData{p}}, p
		}},
		{"event", ViewEventDetail, func(m *Model) (*model.ClusterData, interface{}) {
			e := &model.EventData{Type: "Warning", Reason: "BackOff", InvolvedObject: "Pod/web-0", InvolvedNamespace: "default"}
			m.selectedEvent = e
			return &model.ClusterData{Events: []*model.EventData{e}}, e
		}},
		{"job", ViewJobDetail, func(m *Model) (*model.ClusterData, interface{}) {
			j := &model.JobData{Name: "batch", Namespace: "default"}
			m.selectedJob = j
			return &model.ClusterData{Jobs: []*model.JobData{j}}, j
		}},
		{"service", ViewServiceDetail, func(m *Model) (*model.ClusterData, interface{}) {
			s := &model.ServiceData{Name: "api", Namespace: "default"}
			m.selectedService = s
			return &model.ClusterData{Services: []*model.ServiceData{s}}, s
		}},
		{"deployment", ViewDeploymentDetail, func(m *Model) (*model.ClusterData, interface{}) {
			d := &model.DeploymentData{Name: "web", Namespace: "default"}
			m.selectedDeployment = d
			return &model.ClusterData{Deployments: []*model.DeploymentData{d}}, d
		}},
		{"statefulset", ViewStatefulSetDetail, func(m *Model) (*model.ClusterData, interface{}) {
			s := &model.StatefulSetData{Name: "db", Namespace: "default"}
			m.selectedStatefulSet = s
			return &model.ClusterData{StatefulSets: []*model.StatefulSetData{s}}, s
		}},
		{"daemonset", ViewDaemonSetDetail, func(m *Model) (*model.ClusterData, interface{}) {
			d := &model.DaemonSetData{Name: "agent", Namespace: "kube-system"}
			m.selectedDaemonSet = d
			return &model.ClusterData{DaemonSets: []*model.DaemonSetData{d}}, d
		}},
		{"cronjob", ViewCronJobDetail, func(m *Model) (*model.ClusterData, interface{}) {
			c := &model.CronJobData{Name: "nightly", Namespace: "default"}
			m.selectedCronJob = c
			return &model.ClusterData{CronJobs: []*model.CronJobData{c}}, c
		}},
		{"pv", ViewPVDetail, func(m *Model) (*model.ClusterData, interface{}) {
			p := &model.PVData{Name: "pv-a"}
			m.selectedPV = p
			return &model.ClusterData{PVs: []*model.PVData{p}}, p
		}},
		{"pvc", ViewPVCDetail, func(m *Model) (*model.ClusterData, interface{}) {
			p := &model.PVCData{Name: "data", Namespace: "default"}
			m.selectedPVC = p
			return &model.ClusterData{PVCs: []*model.PVCData{p}}, p
		}},
		{"volcanojob", ViewVolcanoJobDetail, func(m *Model) (*model.ClusterData, interface{}) {
			v := &model.VolcanoJobData{Name: "vj", Namespace: "default"}
			m.selectedVolcanoJob = v
			return &model.ClusterData{VolcanoJobs: []*model.VolcanoJobData{v}}, v
		}},
		{"queue", ViewQueueDetail, func(m *Model) (*model.ClusterData, interface{}) {
			q := &model.QueueData{Name: "default"}
			m.selectedQueue = q
			return &model.ClusterData{Queues: []*model.QueueData{q}}, q
		}},
		{"superpod", ViewTopologyDetail, func(m *Model) (*model.ClusterData, interface{}) {
			n := b31vNode("node-a")
			n.SuperPodID, n.NPUAllocatable = "sp-1", 8
			cd := &model.ClusterData{Nodes: []*model.NodeData{n}}
			m.clusterData = cd
			sps := m.getSuperPodTopology()
			if len(sps) == 0 {
				t.Fatalf("superpod fixture produced no topology")
			}
			m.selectedSuperPod = &sps[0]
			return cd, m.selectedSuperPod
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestModel(t)
			m.width, m.height = 200, 40
			m.currentView = c.view
			m.detailMode = true
			cd, ptr := c.setup(m)
			if m.clusterData == nil {
				m.clusterData = cd
			}

			b31RefreshProbe(m, &model.ClusterData{})

			if m.currentView != c.view || !m.detailMode {
				t.Fatalf("an empty collection evicted the user: view=%v detailMode=%v (want %v)", m.currentView, m.detailMode, c.view)
			}
			switch v := ptr.(type) {
			case *model.NodeData:
				if m.selectedNode != v {
					t.Errorf("node selection changed on a transient empty refresh")
				}
			case *model.PodData:
				if m.selectedPod != v {
					t.Errorf("pod selection changed on a transient empty refresh")
				}
			case *model.EventData:
				if m.selectedEvent != v {
					t.Errorf("event selection changed on a transient empty refresh")
				}
			case *model.JobData:
				if m.selectedJob != v {
					t.Errorf("job selection changed on a transient empty refresh")
				}
			case *model.ServiceData:
				if m.selectedService != v {
					t.Errorf("service selection changed on a transient empty refresh")
				}
			case *model.DeploymentData:
				if m.selectedDeployment != v {
					t.Errorf("deployment selection changed on a transient empty refresh")
				}
			case *model.StatefulSetData:
				if m.selectedStatefulSet != v {
					t.Errorf("statefulset selection changed on a transient empty refresh")
				}
			case *model.DaemonSetData:
				if m.selectedDaemonSet != v {
					t.Errorf("daemonset selection changed on a transient empty refresh")
				}
			case *model.CronJobData:
				if m.selectedCronJob != v {
					t.Errorf("cronjob selection changed on a transient empty refresh")
				}
			case *model.PVData:
				if m.selectedPV != v {
					t.Errorf("pv selection changed on a transient empty refresh")
				}
			case *model.PVCData:
				if m.selectedPVC != v {
					t.Errorf("pvc selection changed on a transient empty refresh")
				}
			case *model.VolcanoJobData:
				if m.selectedVolcanoJob != v {
					t.Errorf("volcanojob selection changed on a transient empty refresh")
				}
			case *model.QueueData:
				if m.selectedQueue != v {
					t.Errorf("queue selection changed on a transient empty refresh")
				}
			case *SuperPodInfo:
				if m.selectedSuperPod != v {
					t.Errorf("superpod selection changed on a transient empty refresh")
				}
			default:
				t.Fatalf("unexpected fixture type %T", ptr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4. Node detail opened from the NPU view, node gone from a non-empty list:
//    must land on the NPU view, keep the list position, and render without a
//    panic even when no NPU node is left.
// ---------------------------------------------------------------------------

func TestB31VNodeGoneFromNPULandsRenderable(t *testing.T) {
	nodeA := b31vNode("node-a")
	nodeA.NPUCapacity = 8
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.clusterData = &model.ClusterData{Nodes: []*model.NodeData{nodeA}}
	m.currentView = ViewNodeDetail
	m.detailMode = true
	m.fromNPUView = true
	m.selectedNode = nodeA
	m.selectedIndex = 1
	m.scrollOffset = 2

	noNPUNode := b31vNode("node-b") // no NPU capacity
	b31RefreshProbe(m, &model.ClusterData{Nodes: []*model.NodeData{noNPUNode}})

	if m.currentView != ViewNPU || m.detailMode {
		t.Fatalf("gone NPU node landed on view=%v detailMode=%v, want ViewNPU", m.currentView, m.detailMode)
	}
	if m.fromNPUView {
		t.Errorf("fromNPUView survived the auto-exit")
	}
	if m.selectedNode != nil {
		t.Errorf("selectedNode survived the auto-exit")
	}
	if m.selectedIndex != 1 || m.scrollOffset != 2 {
		t.Errorf("the NPU list position was reset: selectedIndex=%d scrollOffset=%d", m.selectedIndex, m.scrollOffset)
	}

	// Rendering must not panic and must say there is no NPU node.
	view := stripANSI(m.View())
	if !strings.Contains(view, m.T("npuview.no_nodes")) {
		t.Errorf("empty NPU view does not render its empty state:\n%s", view)
	}
}

// ---------------------------------------------------------------------------
// 5. Parent Job gone while the pod is alive: the chain is broken, so the
//    detail must not stay on top of an object that no longer exists.
// ---------------------------------------------------------------------------

func TestB31VJobGonePodAliveBacksOut(t *testing.T) {
	job := &model.JobData{Name: "batch", Namespace: "default"}
	pod := b31vPod("default", "batch-x1", 1)
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.clusterData = &model.ClusterData{Jobs: []*model.JobData{job}, Pods: []*model.PodData{pod}}
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.fromJobDetail = true
	m.selectedJob = job
	m.selectedPod = pod

	b31RefreshProbe(m, &model.ClusterData{
		Jobs: []*model.JobData{{Name: "another", Namespace: "default"}},
		Pods: []*model.PodData{b31vPod("default", "batch-x1", 2)}, // same identity, fresh values
	})

	if m.currentView != ViewWorkloads || m.detailMode {
		t.Errorf("a deleted parent job left the model on view=%v detailMode=%v, want ViewWorkloads",
			m.currentView, m.detailMode)
	}
	if m.selectedPod != nil || m.selectedJob != nil {
		t.Errorf("selections survived: pod=%v job=%v", m.selectedPod, m.selectedJob)
	}
}

// ---------------------------------------------------------------------------
// 6. A refresh outside any detail view must not disturb the overlay state,
//    even when a leftover selection pointer no longer resolves.
// ---------------------------------------------------------------------------

func TestB31VNoSideEffectOutsideDetail(t *testing.T) {
	stale := b31vPod("default", "web-0", 1)
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.currentView = ViewPods
	m.clusterData = &model.ClusterData{Pods: []*model.PodData{stale}}
	m.selectedPod = stale // leftover, not shown anywhere

	b31RefreshProbe(m, &model.ClusterData{Pods: []*model.PodData{b31vPod("default", "web-1", 1)}})

	if m.currentView != ViewPods || m.detailMode || m.logsMode {
		t.Errorf("a refresh outside a detail view changed the view state: view=%v detailMode=%v logsMode=%v",
			m.currentView, m.detailMode, m.logsMode)
	}
}

// ---------------------------------------------------------------------------
// 7. Logs opened from the Job detail (the second logs entry point): the pod
//    disappearing must close the viewer but leave the still-valid Job detail.
// ---------------------------------------------------------------------------

func TestB31VLogsFromJobDetailPodGone(t *testing.T) {
	job := &model.JobData{Name: "batch", Namespace: "default"}
	pod := b31vPod("default", "batch-x1", 1)
	pod.ContainerStates = []model.ContainerState{{Name: "app", Ready: true}}
	freshJob := &model.JobData{Name: "batch", Namespace: "default", Active: 4}
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.clusterData = &model.ClusterData{Jobs: []*model.JobData{job}, Pods: []*model.PodData{pod}}
	m.currentView = ViewJobDetail
	m.detailMode = true
	m.selectedJob = job
	m.View()
	m.Update(keyMsg("l"))
	if !m.logsMode || m.selectedPod == nil {
		t.Fatalf("'l' on the job detail did not open its pod's logs (logsMode=%v selectedPod=%v)", m.logsMode, m.selectedPod)
	}

	b31RefreshProbe(m, &model.ClusterData{
		Jobs: []*model.JobData{freshJob},
		Pods: []*model.PodData{b31vPod("default", "other-x1", 1)},
	})

	if m.logsMode {
		t.Errorf("the logs viewer stayed open over a pod that no longer exists")
	}
	if m.currentView != ViewJobDetail || !m.detailMode {
		t.Errorf("the still-live Job detail was left: view=%v detailMode=%v", m.currentView, m.detailMode)
	}
	if m.selectedJob != freshJob {
		t.Errorf("the Job selection was not re-pointed while its pod disappeared")
	}
}

// ---------------------------------------------------------------------------
// 8. Escape-chain equivalence: an auto-exit must land in the state the user
//    reaches by pressing Esc the same number of times.
// ---------------------------------------------------------------------------

func b31vDump(m *Model) string {
	args := []interface{}{m.currentView, m.detailMode, m.detailScrollOffset, m.scrollOffset, m.selectedIndex,
		m.jobPodSelectedIndex, m.volcanoJobPodSelectedIndex,
		m.fromJobDetail, m.fromVolcanoJobDetail, m.fromNetworkView, m.fromNPUView,
		m.logsMode, m.logsSearchMode, m.logsSearchText, m.logsAutoRefresh, m.logsAutoScroll,
		m.selectedContainer, m.containerLogs, m.logsFetched, m.logsScrollOffset, m.logsError,
		m.commandOutputMode, m.commandOutputTitle, m.commandOutputScroll, m.actionMenuMode,
		m.actionMenuSelectedIndex, m.searchMode, m.searchText, m.filterMode,
		m.selectedNode != nil, m.selectedPod != nil, m.selectedEvent != nil, m.selectedJob != nil,
		m.selectedService != nil, m.selectedDeployment != nil, m.selectedStatefulSet != nil,
		m.selectedDaemonSet != nil, m.selectedCronJob != nil, m.selectedPV != nil,
		m.selectedPVC != nil, m.selectedVolcanoJob != nil, m.selectedQueue != nil, m.selectedSuperPod != nil}
	return fmt.Sprintf(strings.Repeat("%v|", len(args)-1)+"%v", args...)
}

func TestB31VAutoExitMatchesEscChain(t *testing.T) {
	setupLogsSearch := func(t *testing.T) *Model {
		pod := b31vPod("default", "web-0", 1)
		pod.ContainerStates = []model.ContainerState{{Name: "app", Ready: true}}
		m := b31vOpenPod(t, &model.ClusterData{Pods: []*model.PodData{pod}}, 0)
		m.Update(keyMsg("l"))
		m.Update(keyMsg("/"))
		m.Update(keyMsg("e"))
		if !m.logsMode || !m.logsSearchMode {
			t.Fatalf("precondition failed: logsMode=%v logsSearchMode=%v", m.logsMode, m.logsSearchMode)
		}
		return m
	}
	gone := &model.ClusterData{Pods: []*model.PodData{b31vPod("default", "web-1", 1)}}

	auto := setupLogsSearch(t)
	b31RefreshProbe(auto, gone)

	manual := setupLogsSearch(t)
	for i := 0; i < 3; i++ { // cancel search, close logs, leave detail
		manual.Update(tea.KeyMsg{Type: tea.KeyEsc})
	}

	if got, want := b31vDump(auto), b31vDump(manual); got != want {
		t.Errorf("auto-exit state differs from the three-Esc chain:\n auto   = %s\n manual = %s", got, want)
	}
}

func TestB31VOverlayClosureMatchesEscChain(t *testing.T) {
	mk := func(t *testing.T) *Model {
		pod := b31vPod("default", "web-0", 1)
		m := b31vOpenPod(t, &model.ClusterData{Pods: []*model.PodData{pod}}, 0)
		return m
	}
	gone := &model.ClusterData{Pods: []*model.PodData{b31vPod("default", "web-1", 1)}}

	t.Run("command output", func(t *testing.T) {
		auto, manual := mk(t), mk(t)
		for _, m := range []*Model{auto, manual} {
			m.commandOutputMode = true
			m.commandOutputTitle = "describe"
			m.commandOutputContent = "output"
			m.commandOutputScroll = 5
		}
		b31RefreshProbe(auto, gone)
		manual.Update(tea.KeyMsg{Type: tea.KeyEsc})
		manual.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if got, want := b31vDump(auto), b31vDump(manual); got != want {
			t.Errorf("command-output auto-close state differs from Esc Esc:\n auto   = %s\n manual = %s", got, want)
		}
	})

	t.Run("action menu", func(t *testing.T) {
		auto, manual := mk(t), mk(t)
		for _, m := range []*Model{auto, manual} {
			m.actionMenuMode = true
			m.actionMenuSelectedIndex = 2
		}
		b31RefreshProbe(auto, gone)
		manual.Update(tea.KeyMsg{Type: tea.KeyEsc})
		manual.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if got, want := b31vDump(auto), b31vDump(manual); got != want {
			t.Errorf("action-menu auto-close state differs from Esc Esc:\n auto   = %s\n manual = %s", got, want)
		}
	})
}

// ---------------------------------------------------------------------------
// 9. A transiently empty parent collection must not break the child detail:
//    an empty Jobs list is a blip, not a deletion of the Job the Pod hangs off.
// ---------------------------------------------------------------------------

func TestB31VTransientEmptyParentKeepsPodDetail(t *testing.T) {
	job := &model.JobData{Name: "batch", Namespace: "default"}
	pod := b31vPod("default", "batch-x1", 1)
	m := newTestModel(t)
	m.width, m.height = 200, 40
	m.clusterData = &model.ClusterData{Jobs: []*model.JobData{job}, Pods: []*model.PodData{pod}}
	m.currentView = ViewPodDetail
	m.detailMode = true
	m.fromJobDetail = true
	m.selectedJob = job
	m.selectedPod = pod

	freshPod := b31vPod("default", "batch-x1", 6)
	b31RefreshProbe(m, &model.ClusterData{Pods: []*model.PodData{freshPod}}) // Jobs list empty

	if m.currentView != ViewPodDetail || !m.detailMode || !m.fromJobDetail {
		t.Fatalf("an empty Jobs blip evicted the pod detail: view=%v detailMode=%v fromJob=%v",
			m.currentView, m.detailMode, m.fromJobDetail)
	}
	if m.selectedPod != freshPod {
		t.Errorf("the pod was not re-pointed; the page would stay frozen")
	}
	if m.selectedJob != job {
		t.Errorf("the parent Job selection was altered by a transiently empty Jobs list")
	}
}
