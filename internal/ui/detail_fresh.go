package ui

import "github.com/chenpu/k8s-monitor/internal/model"

// detail_fresh.go — keeps the detail views fresh across cluster refreshes.
//
// A detail view renders from a pointer captured when the user pressed Enter.
// Every refresh replaces m.clusterData with a new snapshot, so without this
// step the detail page would keep rendering the object captured at entry time
// and appear frozen for as long as the user stays on it.
//
// The refresh message handler is the single entry point: automatic and manual
// refreshes both arrive as clusterDataMsg, so re-pointing the selections there
// covers every refresh path.

// reresolveDetailSelections re-points the detail-view selections at their
// counterparts in the freshly received cluster data.
//
// For each live selection:
//   - a matching object in the new snapshot (same identity key) replaces the
//     pointer; nothing else about the detail view is touched (the scroll offset
//     is kept, and every renderer already clamps it to its own content length);
//   - an object that is gone from a non-empty collection is handled exactly as
//     pressing Esc would be: backFromDetail returns to the list the detail was
//     opened from;
//   - an empty collection is a transient API result, not a deletion: the
//     selection is left untouched so a blip does not throw the user out.
func (m *Model) reresolveDetailSelections() {
	if m.clusterData == nil {
		return
	}
	cd := m.clusterData

	// gone holds the detail views whose object disappeared from a non-empty
	// collection in this snapshot.
	gone := make(map[ViewType]bool)

	// Node detail: cluster-scoped, keyed by Name.
	if m.selectedNode != nil && len(cd.Nodes) > 0 {
		var found *model.NodeData
		for _, node := range cd.Nodes {
			if node != nil && node.Name == m.selectedNode.Name {
				found = node
				break
			}
		}
		if found != nil {
			m.selectedNode = found
		} else {
			gone[ViewNodeDetail] = true
		}
	}

	// Pod detail: keyed by Namespace+Name (the same name can exist in several
	// namespaces).
	if m.selectedPod != nil && len(cd.Pods) > 0 {
		var found *model.PodData
		for _, pod := range cd.Pods {
			if pod != nil && pod.Namespace == m.selectedPod.Namespace && pod.Name == m.selectedPod.Name {
				found = pod
				break
			}
		}
		if found != nil {
			m.selectedPod = found
		} else {
			gone[ViewPodDetail] = true
		}
	}

	// Event detail: events roll out of the window; Type+Reason+InvolvedObject+
	// InvolvedNamespace is what identifies one. A frozen event detail is worse
	// than leaving it, so a missing event exits like Esc.
	if m.selectedEvent != nil && len(cd.Events) > 0 {
		var found *model.EventData
		for _, event := range cd.Events {
			if event != nil &&
				event.Type == m.selectedEvent.Type &&
				event.Reason == m.selectedEvent.Reason &&
				event.InvolvedObject == m.selectedEvent.InvolvedObject &&
				event.InvolvedNamespace == m.selectedEvent.InvolvedNamespace {
				found = event
				break
			}
		}
		if found != nil {
			m.selectedEvent = found
		} else {
			gone[ViewEventDetail] = true
		}
	}

	// Job detail: Namespace+Name.
	if m.selectedJob != nil && len(cd.Jobs) > 0 {
		var found *model.JobData
		for _, job := range cd.Jobs {
			if job != nil && job.Namespace == m.selectedJob.Namespace && job.Name == m.selectedJob.Name {
				found = job
				break
			}
		}
		if found != nil {
			m.selectedJob = found
		} else {
			gone[ViewJobDetail] = true
		}
	}

	// Service detail: Namespace+Name.
	if m.selectedService != nil && len(cd.Services) > 0 {
		var found *model.ServiceData
		for _, svc := range cd.Services {
			if svc != nil && svc.Namespace == m.selectedService.Namespace && svc.Name == m.selectedService.Name {
				found = svc
				break
			}
		}
		if found != nil {
			m.selectedService = found
		} else {
			gone[ViewServiceDetail] = true
		}
	}

	// Deployment detail: Namespace+Name.
	if m.selectedDeployment != nil && len(cd.Deployments) > 0 {
		var found *model.DeploymentData
		for _, deploy := range cd.Deployments {
			if deploy != nil && deploy.Namespace == m.selectedDeployment.Namespace && deploy.Name == m.selectedDeployment.Name {
				found = deploy
				break
			}
		}
		if found != nil {
			m.selectedDeployment = found
		} else {
			gone[ViewDeploymentDetail] = true
		}
	}

	// StatefulSet detail: Namespace+Name.
	if m.selectedStatefulSet != nil && len(cd.StatefulSets) > 0 {
		var found *model.StatefulSetData
		for _, sts := range cd.StatefulSets {
			if sts != nil && sts.Namespace == m.selectedStatefulSet.Namespace && sts.Name == m.selectedStatefulSet.Name {
				found = sts
				break
			}
		}
		if found != nil {
			m.selectedStatefulSet = found
		} else {
			gone[ViewStatefulSetDetail] = true
		}
	}

	// DaemonSet detail: Namespace+Name.
	if m.selectedDaemonSet != nil && len(cd.DaemonSets) > 0 {
		var found *model.DaemonSetData
		for _, ds := range cd.DaemonSets {
			if ds != nil && ds.Namespace == m.selectedDaemonSet.Namespace && ds.Name == m.selectedDaemonSet.Name {
				found = ds
				break
			}
		}
		if found != nil {
			m.selectedDaemonSet = found
		} else {
			gone[ViewDaemonSetDetail] = true
		}
	}

	// CronJob detail: Namespace+Name.
	if m.selectedCronJob != nil && len(cd.CronJobs) > 0 {
		var found *model.CronJobData
		for _, cj := range cd.CronJobs {
			if cj != nil && cj.Namespace == m.selectedCronJob.Namespace && cj.Name == m.selectedCronJob.Name {
				found = cj
				break
			}
		}
		if found != nil {
			m.selectedCronJob = found
		} else {
			gone[ViewCronJobDetail] = true
		}
	}

	// PV detail: cluster-scoped, keyed by Name.
	if m.selectedPV != nil && len(cd.PVs) > 0 {
		var found *model.PVData
		for _, pv := range cd.PVs {
			if pv != nil && pv.Name == m.selectedPV.Name {
				found = pv
				break
			}
		}
		if found != nil {
			m.selectedPV = found
		} else {
			gone[ViewPVDetail] = true
		}
	}

	// PVC detail: Namespace+Name.
	if m.selectedPVC != nil && len(cd.PVCs) > 0 {
		var found *model.PVCData
		for _, pvc := range cd.PVCs {
			if pvc != nil && pvc.Namespace == m.selectedPVC.Namespace && pvc.Name == m.selectedPVC.Name {
				found = pvc
				break
			}
		}
		if found != nil {
			m.selectedPVC = found
		} else {
			gone[ViewPVCDetail] = true
		}
	}

	// Volcano Job detail: Namespace+Name.
	if m.selectedVolcanoJob != nil && len(cd.VolcanoJobs) > 0 {
		var found *model.VolcanoJobData
		for _, vj := range cd.VolcanoJobs {
			if vj != nil && vj.Namespace == m.selectedVolcanoJob.Namespace && vj.Name == m.selectedVolcanoJob.Name {
				found = vj
				break
			}
		}
		if found != nil {
			m.selectedVolcanoJob = found
		} else {
			gone[ViewVolcanoJobDetail] = true
		}
	}

	// Queue detail: cluster-scoped, keyed by Name.
	if m.selectedQueue != nil && len(cd.Queues) > 0 {
		var found *model.QueueData
		for _, queue := range cd.Queues {
			if queue != nil && queue.Name == m.selectedQueue.Name {
				found = queue
				break
			}
		}
		if found != nil {
			m.selectedQueue = found
		} else {
			gone[ViewQueueDetail] = true
		}
	}

	// SuperPod detail: the selection is an aggregated view derived from the
	// nodes, so it is rebuilt from the fresh node list and re-pointed by ID.
	if m.selectedSuperPod != nil {
		superPods := m.getSuperPodTopology()
		if len(superPods) > 0 {
			found := -1
			for i := range superPods {
				if superPods[i].ID == m.selectedSuperPod.ID {
					found = i
					break
				}
			}
			if found >= 0 {
				m.selectedSuperPod = &superPods[found]
			} else {
				gone[ViewTopologyDetail] = true
			}
		}
	}

	if len(gone) == 0 || !m.detailMode {
		return
	}
	m.leaveDetailAfterSelectionLost(gone)
}

// leaveDetailAfterSelectionLost backs the user out of every detail view whose
// object disappeared, using the Esc transition (backFromDetail) rather than a
// second exit path. A Pod detail opened from a Job / Volcano Job detail pops
// back to that parent first; if the parent's object is gone as well, the loop
// pops again, ending on the list view Esc would eventually reach.
func (m *Model) leaveDetailAfterSelectionLost(gone map[ViewType]bool) {
	// Transient overlays can sit on top of a detail page. Close them with the
	// same state resets the Esc handler applies when it pops them, so the
	// user is not left on a list with an orphaned overlay open.
	if m.commandOutputMode {
		m.commandOutputMode = false
		m.commandOutputTitle = ""
		m.commandOutputContent = ""
		m.commandOutputScroll = 0
	}
	if m.actionMenuMode {
		m.actionMenuMode = false
	}
	if m.logsMode {
		m.exitLogsViewer()
	}

	for m.detailMode && m.detailSelectionLost(gone) {
		m.backFromDetail()
	}
}

// detailSelectionLost reports whether the detail view on screen belongs to an
// object that disappeared from this snapshot. The Pod detail can be stacked on
// a Job / Volcano Job detail; that parent's object is part of the state the
// user is looking at, so a missing parent counts as lost too.
func (m *Model) detailSelectionLost(gone map[ViewType]bool) bool {
	if gone[m.currentView] {
		return true
	}
	if m.currentView == ViewPodDetail {
		if m.fromJobDetail && gone[ViewJobDetail] {
			return true
		}
		if m.fromVolcanoJobDetail && gone[ViewVolcanoJobDetail] {
			return true
		}
	}
	return false
}
