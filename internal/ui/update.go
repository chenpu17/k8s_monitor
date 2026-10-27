package ui

import (
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// printableRunes returns the printable runes typed in a key message.
// bubbletea merges burst-typed or pasted characters into a single multi-rune
// KeyMsg, so filtering on len(msg.String()) == 1 would drop that input
// entirely. Space is excluded; it arrives as KeySpace and is handled above.
func printableRunes(msg tea.KeyMsg) []rune {
	if msg.Type != tea.KeyRunes {
		return nil
	}
	out := make([]rune, 0, len(msg.Runes))
	for _, r := range msg.Runes {
		if unicode.IsPrint(r) && r != ' ' {
			out = append(out, r)
		}
	}
	return out
}

// isNameRune reports whether r is accepted in the list search text
// (alphanumerics, dash, underscore, dot).
func isNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	// 忽略鼠标事件，防止滚轮触发高频重绘
	case tea.MouseMsg:
		return m, nil

	case refreshTickMsg:
		if m.quitting {
			return m, nil
		}
		return m, tea.Batch(
			m.startFetchData(),
			m.scheduleRefresh(),
		)

	case tea.KeyMsg:
		// Ctrl+C quits from every mode. It is handled ahead of the search
		// interception block below (and every other mode block) because that
		// block treats every non-navigation key as text input, which swallowed
		// Ctrl+C in search mode. `q` keeps its per-mode meaning: in search mode
		// it is a character the user is typing.
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}

		// In search modes, treat most single-character keys as text input
		// Only allow navigation keys (arrows, page up/down, esc, backspace, space, enter)
		if m.logsSearchMode || m.searchMode {
			// Check if this is a special key that should work in search mode
			isSpecialKey := key.Matches(msg, m.keys.Back) || // Esc
				key.Matches(msg, m.keys.Up) ||
				key.Matches(msg, m.keys.Down) ||
				key.Matches(msg, m.keys.PageUp) ||
				key.Matches(msg, m.keys.PageDown) ||
				key.Matches(msg, m.keys.Enter) ||
				msg.Type == tea.KeyBackspace ||
				msg.Type == tea.KeyDelete ||
				msg.Type == tea.KeySpace

			// Handle backspace and space before checking other special keys
			if msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
				if m.logsSearchMode && len(m.logsSearchText) > 0 {
					m.logsSearchText = m.logsSearchText[:len(m.logsSearchText)-1]
					m.logsScrollOffset = 0
					return m, nil
				}
				if m.searchMode && len(m.searchText) > 0 {
					m.searchText = m.searchText[:len(m.searchText)-1]
					m.scrollOffset = 0
					m.selectedIndex = 0
					return m, nil
				}
				return m, nil
			}

			if msg.Type == tea.KeySpace {
				if m.logsSearchMode {
					m.logsSearchText += " "
					m.logsScrollOffset = 0
					return m, nil
				}
				if m.searchMode {
					m.searchText += " "
					m.scrollOffset = 0
					m.selectedIndex = 0
					return m, nil
				}
				return m, nil
			}

			// If not a special key, handle as text input directly
			if !isSpecialKey {
				// Handle text input for logs search mode
				if m.logsSearchMode {
					if chars := printableRunes(msg); len(chars) > 0 {
						m.logsSearchText += string(chars)
						m.logsScrollOffset = 0
						return m, nil
					}
				}
				// Handle text input for normal search mode
				if m.searchMode {
					accepted := ""
					for _, r := range printableRunes(msg) {
						if isNameRune(r) {
							accepted += string(r)
						}
					}
					if accepted != "" {
						m.searchText += accepted
						m.scrollOffset = 0
						m.selectedIndex = 0
						return m, nil
					}
				}
				// If we get here, it's not a printable character we handle
				return m, nil
			}
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Refresh):
			// Manual refresh: refresh logs if in logs mode, otherwise refresh cluster data
			if m.logsMode {
				return m, m.fetchLogs()
			}
			// Only call startFetchData() to avoid duplicate GetClusterData calls
			// The refresher cache will be updated automatically
			return m, m.startFetchData()

		// Number keys for quick view switching
		case msg.String() == "1":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewOverview
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "2":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewNodes
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "3":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewPods
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "4":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewWorkloads
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "5":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewNetwork
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "6":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewStorage
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "7":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewEvents
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "8":
			if !m.viewSwitchBlocked() {
				m.currentView = ViewAlerts
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "9":
			// Only switch to Queues view if Volcano is available
			if !m.viewSwitchBlocked() && m.hasVolcanoQueues() {
				m.currentView = ViewQueues
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "0":
			// Only switch to Topology view if SuperPod info is available
			if !m.viewSwitchBlocked() && m.hasSuperPodTopology() {
				m.currentView = ViewTopology
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "n":
			// Only switch to the NPU view if the cluster has NPU nodes. The
			// branch sits behind the search interception above, so in search
			// mode 'n' stays the character the user is typing.
			if !m.viewSwitchBlocked() && m.hasNPUNodes() {
				m.currentView = ViewNPU
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case key.Matches(msg, m.keys.Tab):
			// Tab key switches views in list mode
			if !m.viewSwitchBlocked() {
				// Build the list of currently available views; must stay in sync with renderViewTabs
				views := []ViewType{
					ViewOverview, ViewNodes, ViewPods, ViewWorkloads,
					ViewNetwork, ViewStorage, ViewEvents, ViewAlerts,
				}
				if m.hasNPUNodes() {
					views = append(views, ViewNPU)
				}
				if m.hasVolcanoQueues() {
					views = append(views, ViewQueues)
				}
				if m.hasSuperPodTopology() {
					views = append(views, ViewTopology)
				}

				// Advance to the next available view, wrapping around
				next := views[0]
				for i, v := range views {
					if v == m.currentView {
						next = views[(i+1)%len(views)]
						break
					}
				}
				m.currentView = next
				m.scrollOffset = 0 // Reset scroll when switching views
				m.selectedIndex = 0
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			// Handle action menu execution
			if m.actionMenuMode {
				items := m.getActionMenuItems()
				if m.actionMenuSelectedIndex < len(items) {
					action := items[m.actionMenuSelectedIndex].Action
					m.actionMenuMode = false
					return m, m.executeAction(action)
				}
				m.actionMenuMode = false
				return m, nil
			}
			// Enter key opens detail view or applies filter
			if m.filterMode {
				// Apply filter and exit filter mode
				m.filterMode = false
				return m, nil
			}
			if !m.detailMode && m.searchMode {
				// Enter from a searched list commits the search and opens the
				// detail: the input state is dropped so the detail view owns
				// the keyboard (Esc, q, the footer). The search text stays, so
				// the result the user was looking at is still filtered when
				// they come back.
				m.searchMode = false
			}
			if m.detailMode && m.currentView == ViewJobDetail && m.selectedJob != nil {
				// Navigate from Job detail to Pod detail
				jobPods := m.getJobPods(m.selectedJob)
				// Limit to max 50 pods displayed
				displayCount := len(jobPods)
				const maxDisplay = 50
				if displayCount > maxDisplay {
					displayCount = maxDisplay
				}
				if m.jobPodSelectedIndex < displayCount && m.jobPodSelectedIndex < len(jobPods) {
					m.selectedPod = jobPods[m.jobPodSelectedIndex]
					m.currentView = ViewPodDetail
					m.detailMode = true // BUGFIX: Must set detailMode to true for "l" key to work
					m.fromJobDetail = true
					m.detailScrollOffset = 0
				}
				return m, nil
			}
			if m.detailMode && m.currentView == ViewVolcanoJobDetail && m.selectedVolcanoJob != nil {
				// Navigate from Volcano Job detail to Pod detail
				volcanoJobPods := m.getVolcanoJobPods(m.selectedVolcanoJob)
				// Limit to max 50 pods displayed
				displayCount := len(volcanoJobPods)
				const maxDisplay = 50
				if displayCount > maxDisplay {
					displayCount = maxDisplay
				}
				if m.volcanoJobPodSelectedIndex < displayCount && m.volcanoJobPodSelectedIndex < len(volcanoJobPods) {
					m.selectedPod = volcanoJobPods[m.volcanoJobPodSelectedIndex]
					m.currentView = ViewPodDetail
					m.detailMode = true
					m.fromVolcanoJobDetail = true
					m.detailScrollOffset = 0
				}
				return m, nil
			}
			if !m.detailMode && m.clusterData != nil {
				switch m.currentView {
				case ViewNodes:
					// Use cached sorted nodes if available
					nodes := m.cachedSortedNodes
					if nodes == nil {
						nodes = m.clusterData.Nodes
					}
					if m.selectedIndex >= 0 && m.selectedIndex < len(nodes) {
						m.selectedNode = nodes[m.selectedIndex]
						m.currentView = ViewNodeDetail
						m.detailMode = true
						m.detailScrollOffset = 0 // Reset scroll when entering detail
					}
				case ViewPods:
					// Use cached sorted pods if available
					pods := m.cachedSortedPods
					if pods == nil {
						pods = m.getFilteredPods()
					}
					if m.selectedIndex >= 0 && m.selectedIndex < len(pods) {
						m.selectedPod = pods[m.selectedIndex]
						m.currentView = ViewPodDetail
						m.detailMode = true
						m.detailScrollOffset = 0 // Reset scroll when entering detail
					}
				case ViewEvents:
					// Use cached sorted events if available
					events := m.cachedSortedEvents
					if events == nil {
						events = m.getFilteredEvents()
					}
					if m.selectedIndex >= 0 && m.selectedIndex < len(events) {
						m.selectedEvent = events[m.selectedIndex]
						m.currentView = ViewEventDetail
						m.detailMode = true
						m.detailScrollOffset = 0 // Reset scroll when entering detail
					}
				case ViewNetwork:
					// Mirror the order the network table renders (endpoint-less first).
					services := m.sortedNetworkServices()
					if m.selectedIndex >= 0 && m.selectedIndex < len(services) {
						m.selectedService = services[m.selectedIndex]
						m.currentView = ViewServiceDetail
						m.detailMode = true
						m.detailScrollOffset = 0
						m.fromNetworkView = true
					}
				case ViewWorkloads:
					// Determine which workload section contains the selected index
					currentItemIndex := 0
					sectionOrder := []string{"volcanojob", "job", "service", "deployment", "statefulset", "daemonset", "cronjob"}

					for _, sectionType := range sectionOrder {
						section, exists := m.workloadSections[sectionType]
						if !exists || section.count == 0 {
							continue
						}

						// Check if selected index falls in this section
						if m.selectedIndex >= currentItemIndex && m.selectedIndex < currentItemIndex+section.count {
							itemIndexInSection := m.selectedIndex - currentItemIndex

							// Route to appropriate detail view based on section type
							switch sectionType {
							case "volcanojob":
								if itemIndexInSection < len(m.clusterData.VolcanoJobs) {
									m.selectedVolcanoJob = m.clusterData.VolcanoJobs[itemIndexInSection]
									m.currentView = ViewVolcanoJobDetail
									m.detailMode = true
									m.detailScrollOffset = 0
									m.volcanoJobPodSelectedIndex = 0
								}
							case "job":
								if itemIndexInSection < len(m.clusterData.Jobs) {
									m.selectedJob = m.clusterData.Jobs[itemIndexInSection]
									m.currentView = ViewJobDetail
									m.detailMode = true
									m.detailScrollOffset = 0
								}
							case "service":
								if itemIndexInSection < len(m.clusterData.Services) {
									m.selectedService = m.clusterData.Services[itemIndexInSection]
									m.currentView = ViewServiceDetail
									m.detailMode = true
									m.detailScrollOffset = 0
								}
							case "deployment":
								if itemIndexInSection < len(m.clusterData.Deployments) {
									m.selectedDeployment = m.clusterData.Deployments[itemIndexInSection]
									m.currentView = ViewDeploymentDetail
									m.detailMode = true
									m.detailScrollOffset = 0
								}
							case "statefulset":
								if itemIndexInSection < len(m.clusterData.StatefulSets) {
									m.selectedStatefulSet = m.clusterData.StatefulSets[itemIndexInSection]
									m.currentView = ViewStatefulSetDetail
									m.detailMode = true
									m.detailScrollOffset = 0
								}
							case "daemonset":
								if itemIndexInSection < len(m.clusterData.DaemonSets) {
									m.selectedDaemonSet = m.clusterData.DaemonSets[itemIndexInSection]
									m.currentView = ViewDaemonSetDetail
									m.detailMode = true
									m.detailScrollOffset = 0
								}
							case "cronjob":
								if itemIndexInSection < len(m.clusterData.CronJobs) {
									m.selectedCronJob = m.clusterData.CronJobs[itemIndexInSection]
									m.currentView = ViewCronJobDetail
									m.detailMode = true
									m.detailScrollOffset = 0
								}
							}
							break
						}

						currentItemIndex += section.count
					}
				case ViewStorage:
					// Storage view shows PVs first, then PVCs
					totalPVs := len(m.clusterData.PVs)
					totalPVCs := len(m.clusterData.PVCs)

					if m.selectedIndex >= 0 && m.selectedIndex < totalPVs {
						// Selected a PV
						m.selectedPV = m.clusterData.PVs[m.selectedIndex]
						m.currentView = ViewPVDetail
						m.detailMode = true
						m.detailScrollOffset = 0
					} else if m.selectedIndex < totalPVs+totalPVCs {
						// Selected a PVC
						pvcIndex := m.selectedIndex - totalPVs
						m.selectedPVC = m.clusterData.PVCs[pvcIndex]
						m.currentView = ViewPVCDetail
						m.detailMode = true
						m.detailScrollOffset = 0
					}
				case ViewQueues:
					// Queue view - select queue for detail
					if m.selectedIndex >= 0 && m.selectedIndex < len(m.clusterData.Queues) {
						m.selectedQueue = m.clusterData.Queues[m.selectedIndex]
						m.currentView = ViewQueueDetail
						m.detailMode = true
						m.detailScrollOffset = 0
					}
				case ViewTopology:
					// Topology view - select SuperPod for detail
					superPods := m.getSuperPodTopology()
					if m.selectedIndex >= 0 && m.selectedIndex < len(superPods) {
						m.selectedSuperPod = &superPods[m.selectedIndex]
						m.currentView = ViewTopologyDetail
						m.detailMode = true
						m.detailScrollOffset = 0
					}
				case ViewNPU:
					// NPU view - open the selected node's detail (a node without
					// chip data has a detail page too: it carries the node's other
					// NPU information, so Enter is never a dead key).
					nodes := m.npuNodes()
					if m.selectedIndex >= 0 && m.selectedIndex < len(nodes) {
						m.selectedNode = nodes[m.selectedIndex]
						m.currentView = ViewNodeDetail
						m.detailMode = true
						m.detailScrollOffset = 0
						m.fromNPUView = true
					}
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Back):
			// Esc key returns to list view or exits filter/search/logs/command output mode
			// Exit command output viewer if active
			if m.commandOutputMode {
				m.commandOutputMode = false
				m.commandOutputTitle = ""
				m.commandOutputContent = ""
				m.commandOutputScroll = 0
				return m, nil
			}
			if m.logsMode {
				// If in logs search mode, exit search mode first
				if m.logsSearchMode {
					m.logsSearchMode = false
					m.logsSearchText = ""
					m.logsScrollOffset = 0

					// Restart auto-refresh and fetch latest logs
					if !m.logsAutoRefresh {
						m.logsAutoRefresh = true
					}
					// Fetch logs immediately to show fresh content
					return m, m.fetchLogs()
				}
				// Exit logs mode entirely
				m.logsMode = false
				m.logsAutoRefresh = false // Stop auto-refresh
				m.logsAutoScroll = false  // Reset auto-scroll
				m.selectedContainer = ""
				m.containerLogs = ""
				m.logsFetched = false
				m.logsScrollOffset = 0
				m.logsError = ""
				return m, nil
			}
			// Exit action menu if active
			if m.actionMenuMode {
				m.actionMenuMode = false
				return m, nil
			}
			if m.searchMode {
				m.searchMode = false
				m.searchText = ""
				m.scrollOffset = 0
				m.selectedIndex = 0
				return m, nil
			}
			if m.filterMode {
				m.filterMode = false
				return m, nil
			}
			if m.detailMode {
				// Special handling for navigating back from Pod detail to Job detail
				if m.currentView == ViewPodDetail && m.fromJobDetail {
					m.currentView = ViewJobDetail
					m.fromJobDetail = false
					m.detailScrollOffset = 0
					m.selectedPod = nil
					// Keep m.selectedJob intact
					return m, nil
				}

				// Special handling for navigating back from Pod detail to Volcano Job detail
				if m.currentView == ViewPodDetail && m.fromVolcanoJobDetail {
					m.currentView = ViewVolcanoJobDetail
					m.fromVolcanoJobDetail = false
					m.detailScrollOffset = 0
					m.selectedPod = nil
					// Keep m.selectedVolcanoJob intact
					return m, nil
				}

				// Special handling for navigating back from Service detail to Network view
				if m.currentView == ViewServiceDetail && m.fromNetworkView {
					m.currentView = ViewNetwork
					m.fromNetworkView = false
					m.detailMode = false
					m.detailScrollOffset = 0
					m.selectedService = nil
					// Keep m.scrollOffset and m.selectedIndex intact
					return m, nil
				}

				// Special handling for navigating back from Node detail to the NPU
				// view. Returning early skips the main reset block below, so the
				// detail flag, the scroll offset and the selection are all handled
				// here: the list the user came from comes back as they left it.
				if m.currentView == ViewNodeDetail && m.fromNPUView {
					m.currentView = ViewNPU
					m.fromNPUView = false
					m.detailMode = false
					m.detailScrollOffset = 0
					m.selectedNode = nil
					// Keep m.scrollOffset and m.selectedIndex intact
					return m, nil
				}

				switch m.currentView {
				case ViewNodeDetail:
					m.currentView = ViewNodes
				case ViewPodDetail:
					m.currentView = ViewPods
				case ViewEventDetail:
					m.currentView = ViewEvents
				case ViewJobDetail:
					m.currentView = ViewWorkloads
					m.jobPodSelectedIndex = 0 // Reset selection
				case ViewVolcanoJobDetail:
					m.currentView = ViewWorkloads
					m.volcanoJobPodSelectedIndex = 0 // Reset selection
				case ViewServiceDetail:
					m.currentView = ViewWorkloads
				case ViewDeploymentDetail:
					m.currentView = ViewWorkloads
				case ViewStatefulSetDetail:
					m.currentView = ViewWorkloads
				case ViewDaemonSetDetail:
					m.currentView = ViewWorkloads
				case ViewCronJobDetail:
					m.currentView = ViewWorkloads
				case ViewPVDetail:
					m.currentView = ViewStorage
				case ViewPVCDetail:
					m.currentView = ViewStorage
				case ViewQueueDetail:
					m.currentView = ViewQueues
				case ViewTopologyDetail:
					m.currentView = ViewTopology
				}
				m.detailMode = false
				m.detailScrollOffset = 0 // Reset detail scroll offset
				m.selectedNode = nil
				m.selectedPod = nil
				m.selectedEvent = nil
				m.selectedJob = nil
				m.selectedService = nil
				m.selectedDeployment = nil
				m.selectedStatefulSet = nil
				m.selectedDaemonSet = nil
				m.selectedCronJob = nil
				m.selectedPV = nil
				m.selectedPVC = nil
				m.selectedVolcanoJob = nil
				m.selectedQueue = nil
				m.selectedSuperPod = nil
				m.scrollOffset = 0
				m.selectedIndex = 0 // Reset selected index when returning from detail view

				// Clamp selectedIndex to valid range after view switch
				maxIndex := m.getMaxIndex()
				if maxIndex > 0 && m.selectedIndex >= maxIndex {
					m.selectedIndex = maxIndex - 1
				}
				if m.selectedIndex < 0 {
					m.selectedIndex = 0
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Filter):
			// F key opens filter mode
			if !m.detailMode && !m.filterMode && !m.searchMode {
				// Only for Pods view for now (will extend to Nodes later)
				if m.currentView == ViewPods {
					m.filterMode = true
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.ClearFilter):
			// C key clears all filters
			if !m.detailMode {
				m.filterNamespace = ""
				m.filterStatus = ""
				m.filterRole = ""
				m.searchText = ""
				m.searchMode = false
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case key.Matches(msg, m.keys.Search):
			// / key enters search mode
			if m.logsMode && !m.logsSearchMode {
				// In logs mode, enter logs search mode
				m.logsSearchMode = true
				m.logsSearchText = ""
				return m, nil
			}
			if !m.detailMode && !m.filterMode && !m.searchMode {
				// Only list views whose filters actually consume the search text
				// (pods/nodes/events draw the search panel); elsewhere '/' would
				// open a panel that neither renders nor filters anything.
				switch m.currentView {
				case ViewPods, ViewNodes, ViewEvents:
					m.searchMode = true
					m.searchText = ""
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Logs):
			// L key opens logs viewer in Pod detail view or Job detail view
			if m.detailMode && m.currentView == ViewJobDetail && m.selectedJob != nil {
				// Get logs for selected pod in Job detail
				jobPods := m.getJobPods(m.selectedJob)
				// Limit to max 50 pods displayed
				displayCount := len(jobPods)
				const maxDisplay = 50
				if displayCount > maxDisplay {
					displayCount = maxDisplay
				}
				if m.jobPodSelectedIndex < displayCount && m.jobPodSelectedIndex < len(jobPods) {
					selectedPod := jobPods[m.jobPodSelectedIndex]
					if len(selectedPod.ContainerStates) == 0 {
						m.logsError = m.T("logs.no_containers")
						return m, nil
					}

					// If single container, fetch logs directly
					if len(selectedPod.ContainerStates) == 1 {
						m.selectedPod = selectedPod
						m.selectedContainer = selectedPod.ContainerStates[0].Name
						m.logsMode = true // Enter logs mode
						m.logsFetched = false
						return m, m.fetchLogs()
					}

					// For multiple containers, select the first container by default
					m.selectedPod = selectedPod
					m.selectedContainer = selectedPod.ContainerStates[0].Name
					m.logsMode = true // Enter logs mode
					m.logsFetched = false
					return m, m.fetchLogs()
				}
				return m, nil
			}
			if m.detailMode && m.currentView == ViewPodDetail && m.selectedPod != nil {
				// Get containers from the selected pod
				if len(m.selectedPod.ContainerStates) == 0 {
					m.logsError = m.T("logs.no_containers")
					return m, nil
				}

				// If single container, fetch logs directly
				if len(m.selectedPod.ContainerStates) == 1 {
					m.selectedContainer = m.selectedPod.ContainerStates[0].Name
					m.logsMode = true // Enter logs mode
					m.logsFetched = false
					return m, m.fetchLogs()
				}

				// For multiple containers, select the first container by default
				// TODO: In future, implement container selection UI
				m.selectedContainer = m.selectedPod.ContainerStates[0].Name
				m.logsMode = true // Enter logs mode
				m.logsFetched = false
				return m, m.fetchLogs()
			}
			return m, nil

		case key.Matches(msg, m.keys.Actions):
			// A key opens action menu in detail views. The menu is drawn below
			// the detail content, so it can only be opened from a detail layer
			// the user can actually see: under the command output overlay it
			// would be invisible and Enter would run the highlighted action
			// blind.
			if m.detailMode && !m.actionMenuMode && !m.commandOutputMode {
				// Check if current view supports actions
				if m.currentView == ViewPodDetail || m.currentView == ViewNodeDetail {
					m.actionMenuMode = true
					m.actionMenuSelectedIndex = 0
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Export):
			// E key exports current view data
			if !m.detailMode && !m.exportInProgress && !m.filterMode && !m.searchMode {
				// Check if current view supports export
				if m.currentView == ViewNodes || m.currentView == ViewPods ||
					m.currentView == ViewEvents || m.currentView == ViewNetwork {
					m.exportInProgress = true
					return m, m.exportData(ExportCSV) // Default to CSV format
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.Sort):
			// S key cycles through sort fields
			if !m.detailMode && !m.filterMode {
				switch m.currentView {
				case ViewNodes:
					// Cycle through: Name -> CPU -> Memory -> Pods -> Name
					switch m.sortField {
					case SortByName:
						m.sortField = SortByCPU
						m.sortOrder = SortDesc // Default to descending for numeric fields
					case SortByCPU:
						m.sortField = SortByMemory
						m.sortOrder = SortDesc
					case SortByMemory:
						m.sortField = SortByPods
						m.sortOrder = SortDesc
					case SortByPods:
						m.sortField = SortByName
						m.sortOrder = SortAsc // Default to ascending for name
					default:
						m.sortField = SortByName
						m.sortOrder = SortAsc
					}
				case ViewPods:
					// Cycle through: Name -> Namespace -> Restarts -> Name
					switch m.sortField {
					case SortByName:
						m.sortField = SortByNamespace
						m.sortOrder = SortAsc
					case SortByNamespace:
						m.sortField = SortByRestarts
						m.sortOrder = SortDesc
					case SortByRestarts:
						m.sortField = SortByName
						m.sortOrder = SortAsc
					default:
						m.sortField = SortByName
						m.sortOrder = SortAsc
					}
				}
				// Reset selection after sort
				m.selectedIndex = 0
				m.scrollOffset = 0
			}
			return m, nil

		case msg.String() == "h":
			// Cycle the NPU matrix encoding: AI Core utilisation (the default)
			// → HBM utilisation → temperature → power → health → AI Core. It is
			// NPU-view only, and it sits behind the search interception above,
			// so in search mode 'h' stays the character the user is typing. The
			// line-based scroll offset is kept: the mode legend is one line in
			// every mode, and the clamp in the renderer handles a shorter
			// document.
			if !m.viewSwitchBlocked() && m.currentView == ViewNPU && m.hasNPUNodes() {
				m.npuMatrixMode = (m.npuMatrixMode + 1) % npuMatrixModeCount
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			// Handle action menu navigation
			if m.actionMenuMode {
				if m.actionMenuSelectedIndex > 0 {
					m.actionMenuSelectedIndex--
				}
				return m, nil
			}
			if m.filterMode {
				// Handle filter navigation
				return m, m.handleFilterNavigation(-1)
			}
			if m.commandOutputMode {
				// Scroll up in command output view
				if m.commandOutputScroll > 0 {
					m.commandOutputScroll--
				}
				return m, nil
			}
			if m.logsMode {
				// Scroll up in logs view
				if m.logsScrollOffset > 0 {
					m.logsScrollOffset--
					m.logsAutoScroll = false // Disable auto-scroll when user scrolls up
				}
				return m, nil
			}
			if m.detailMode {
				// Navigate pods in Job detail view
				if m.currentView == ViewJobDetail && m.selectedJob != nil {
					jobPods := m.getJobPods(m.selectedJob)
					// Limit to max 50 pods displayed
					displayCount := len(jobPods)
					const maxDisplay = 50
					if displayCount > maxDisplay {
						displayCount = maxDisplay
					}
					if len(jobPods) > 0 && m.jobPodSelectedIndex > 0 {
						m.jobPodSelectedIndex--
					}
					return m, nil
				}
				// Navigate pods in Volcano Job detail view
				if m.currentView == ViewVolcanoJobDetail && m.selectedVolcanoJob != nil {
					volcanoJobPods := m.getVolcanoJobPods(m.selectedVolcanoJob)
					// Limit to max 50 pods displayed
					displayCount := len(volcanoJobPods)
					const maxDisplay = 50
					if displayCount > maxDisplay {
						displayCount = maxDisplay
					}
					if len(volcanoJobPods) > 0 && m.volcanoJobPodSelectedIndex > 0 {
						m.volcanoJobPodSelectedIndex--
					}
					return m, nil
				}
				// Scroll up in detail view
				m.detailScrollBy(-1)
				return m, nil
			}
			// Network view moves a selection across the service table rows when it
			// has services; without services it falls through to line-based
			// scrolling so the header and pod-network sections stay reachable.
			if m.currentView == ViewNetwork && m.clusterData != nil && len(m.clusterData.Services) > 0 {
				if m.selectedIndex > 0 {
					m.selectedIndex--
					m.networkScrollToSelection()
				}
				return m, nil
			}
			// The NPU view moves a selection across its node rows and keeps the
			// selected row on screen (the rows are the matrix, so the selection
			// is what the user navigates).
			if m.currentView == ViewNPU && m.clusterData != nil && len(m.npuNodes()) > 0 {
				if m.selectedIndex > 0 {
					m.selectedIndex--
					m.npuScrollToSelection()
				}
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.scrollLineViewBy(-1) {
				return m, nil
			}
			// Other list views use item-based selection
			if !m.detailMode {
				if m.selectedIndex > 0 {
					m.selectedIndex--
					if m.selectedIndex < m.scrollOffset {
						m.scrollOffset--
					}
				}
			}
			if m.currentView == ViewAlerts {
				// An alert row is several lines tall, so the window that keeps
				// the selection visible comes from the renderer.
				m.alertsScrollToSelection()
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			// Handle action menu navigation
			if m.actionMenuMode {
				items := m.getActionMenuItems()
				if m.actionMenuSelectedIndex < len(items)-1 {
					m.actionMenuSelectedIndex++
				}
				return m, nil
			}
			if m.filterMode {
				// Handle filter navigation
				return m, m.handleFilterNavigation(1)
			}
			if m.commandOutputMode {
				// Scroll down in command output view. The bound comes from the
				// renderer's own window so ↓ cannot scroll past the last row.
				if maxScroll := m.commandOutputMaxScroll(); m.commandOutputScroll < maxScroll {
					m.commandOutputScroll++
				}
				return m, nil
			}
			if m.logsMode {
				// Scroll down in logs view - use wrapped line count for correct bounds
				totalLines := m.getLogsDisplayLineCount()
				maxVisible := m.logsRows()
				maxScroll := totalLines - maxVisible
				if maxScroll < 0 {
					maxScroll = 0
				}

				if m.logsScrollOffset < maxScroll {
					m.logsScrollOffset++

					// Re-enable auto-scroll if user scrolls near bottom (within 5 lines)
					if m.logsScrollOffset >= maxScroll-5 {
						m.logsAutoScroll = true
					}
				}
				return m, nil
			}
			if m.detailMode {
				// Navigate pods in Job detail view
				if m.currentView == ViewJobDetail && m.selectedJob != nil {
					jobPods := m.getJobPods(m.selectedJob)
					// Limit to max 50 pods displayed
					displayCount := len(jobPods)
					const maxDisplay = 50
					if displayCount > maxDisplay {
						displayCount = maxDisplay
					}
					if len(jobPods) > 0 && m.jobPodSelectedIndex < displayCount-1 {
						m.jobPodSelectedIndex++
					}
					return m, nil
				}
				// Navigate pods in Volcano Job detail view
				if m.currentView == ViewVolcanoJobDetail && m.selectedVolcanoJob != nil {
					volcanoJobPods := m.getVolcanoJobPods(m.selectedVolcanoJob)
					// Limit to max 50 pods displayed
					displayCount := len(volcanoJobPods)
					const maxDisplay = 50
					if displayCount > maxDisplay {
						displayCount = maxDisplay
					}
					if len(volcanoJobPods) > 0 && m.volcanoJobPodSelectedIndex < displayCount-1 {
						m.volcanoJobPodSelectedIndex++
					}
					return m, nil
				}
				// Scroll down in detail view
				m.detailScrollBy(1)
				return m, nil
			}
			// Network view moves a selection across the service table rows when it
			// has services; without services it falls through to line-based
			// scrolling so the header and pod-network sections stay reachable.
			if m.currentView == ViewNetwork && m.clusterData != nil && len(m.clusterData.Services) > 0 {
				if m.selectedIndex < m.getMaxIndex()-1 {
					m.selectedIndex++
					m.networkScrollToSelection()
				}
				return m, nil
			}
			// The NPU view moves a selection across its node rows and keeps the
			// selected row on screen.
			if m.currentView == ViewNPU && m.clusterData != nil && len(m.npuNodes()) > 0 {
				if m.selectedIndex < m.getMaxIndex()-1 {
					m.selectedIndex++
					m.npuScrollToSelection()
				}
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.scrollLineViewBy(1) {
				return m, nil
			}
			// Other list views use item-based selection
			if !m.detailMode {
				maxIndex := m.getMaxIndex()
				if m.selectedIndex < maxIndex-1 {
					m.selectedIndex++
					maxVisible := m.listRowsFor()
					if m.selectedIndex >= m.scrollOffset+maxVisible {
						m.scrollOffset++
					}
				}
			}
			if m.currentView == ViewAlerts {
				// An alert row is several lines tall, so the window that keeps
				// the selection visible comes from the renderer.
				m.alertsScrollToSelection()
			}
			return m, nil

		case key.Matches(msg, m.keys.PageUp):
			// Page up: jump by a full page
			if m.logsMode {
				// Scroll up by a page in logs view
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				if m.logsScrollOffset >= pageSize {
					m.logsScrollOffset -= pageSize
				} else {
					m.logsScrollOffset = 0
				}
				m.logsAutoScroll = false // Disable auto-scroll when user pages up
				return m, nil
			}
			if m.commandOutputMode {
				// Page the command output, not the detail it covers: the
				// footer advertises PgUp/PgDn for the output window.
				pageSize := max(1, m.commandOutputRows())
				if m.commandOutputScroll -= pageSize; m.commandOutputScroll < 0 {
					m.commandOutputScroll = 0
				}
				return m, nil
			}
			if m.detailMode {
				// Scroll up by a page in detail view
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				m.detailScrollBy(-pageSize)
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.scrollLineViewBy(-max(1, m.height-10)) {
				return m, nil
			}
			// The alerts list pages by the alerts that fit on screen: an alert
			// row is several lines tall, so a page is not the terminal height.
			if m.currentView == ViewAlerts {
				m.alertsPageBy(-1)
				return m, nil
			}
			// Other list views use item-based selection. A page is what the
			// view actually shows: the renderers report it, because the rows a
			// view fits is not the terminal height minus a constant.
			if !m.detailMode {
				if m.getMaxIndex() == 0 {
					// Paging an empty list is a no-op: there is nothing to page
					// to, and writing the zero selection would be a side effect
					// with no visible result.
					return m, nil
				}
				pageSize := m.listRowsFor()
				if m.selectedIndex >= pageSize {
					m.selectedIndex -= pageSize
					m.scrollOffset -= pageSize
					if m.scrollOffset < 0 {
						m.scrollOffset = 0
					}
				} else {
					m.selectedIndex = 0
					m.scrollOffset = 0
				}
			}
			if m.currentView == ViewNPU {
				// The NPU view's rows are the matrix rows, which are taller than a
				// line; the window that keeps the selection visible comes from the
				// renderer.
				m.npuScrollToSelection()
			}
			return m, nil

		case key.Matches(msg, m.keys.PageDown):
			// Page down: jump by a full page
			if m.logsMode {
				// Scroll down by a page in logs view - use wrapped line count for correct bounds
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}

				// Calculate bounds using wrapped line count
				totalLines := m.getLogsDisplayLineCount()
				maxVisible := m.logsRows()
				maxScroll := totalLines - maxVisible
				if maxScroll < 0 {
					maxScroll = 0
				}

				m.logsScrollOffset += pageSize
				if m.logsScrollOffset > maxScroll {
					m.logsScrollOffset = maxScroll
				}

				// Re-enable auto-scroll if user pages near bottom (within 5 lines)
				if m.logsScrollOffset >= maxScroll-5 {
					m.logsAutoScroll = true
				}

				return m, nil
			}
			if m.commandOutputMode {
				// Page the command output, not the detail it covers.
				pageSize := max(1, m.commandOutputRows())
				if m.commandOutputScroll += pageSize; m.commandOutputScroll > m.commandOutputMaxScroll() {
					m.commandOutputScroll = m.commandOutputMaxScroll()
				}
				return m, nil
			}
			if m.detailMode {
				// Scroll down by a page in detail view
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				m.detailScrollBy(pageSize)
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.scrollLineViewBy(max(1, m.height-10)) {
				return m, nil
			}
			// The alerts list pages by the alerts that fit on screen.
			if m.currentView == ViewAlerts {
				m.alertsPageBy(1)
				return m, nil
			}
			// Other list views use item-based selection. A page is what the
			// view actually shows: the renderers report it, because the rows a
			// view fits is not the terminal height minus a constant.
			if !m.detailMode {
				if m.getMaxIndex() == 0 {
					// An empty list has nothing to page to. Without this the
					// else branch below would set selectedIndex to -1, which the
					// Enter guard used to accept (items[-1] panic).
					return m, nil
				}
				pageSize := m.listRowsFor()
				maxIndex := m.getMaxIndex()
				if m.selectedIndex+pageSize < maxIndex {
					m.selectedIndex += pageSize
					m.scrollOffset += pageSize
				} else {
					m.selectedIndex = maxIndex - 1
					m.scrollOffset = maxIndex - m.listRowsFor()
					if m.scrollOffset < 0 {
						m.scrollOffset = 0
					}
				}
			}
			if m.currentView == ViewNPU {
				// The NPU view's rows are the matrix rows, which are taller than a
				// line; the window that keeps the selection visible comes from the
				// renderer.
				m.npuScrollToSelection()
			}
			return m, nil

		// Handle text input in logs search mode and search mode
		default:
			if m.logsSearchMode {
				// Handle printable characters (allow more characters for log search)
				if chars := printableRunes(msg); len(chars) > 0 {
					m.logsSearchText += string(chars)
					m.logsScrollOffset = 0
					return m, nil
				}
				// Handle backspace
				if msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
					if len(m.logsSearchText) > 0 {
						m.logsSearchText = m.logsSearchText[:len(m.logsSearchText)-1]
						m.logsScrollOffset = 0
					}
					return m, nil
				}
				// Handle space
				if msg.Type == tea.KeySpace {
					m.logsSearchText += " "
					m.logsScrollOffset = 0
					return m, nil
				}
			}
			if m.searchMode {
				// Handle printable characters
				accepted := ""
				for _, r := range printableRunes(msg) {
					if isNameRune(r) {
						accepted += string(r)
					}
				}
				if accepted != "" {
					m.searchText += accepted
					m.scrollOffset = 0
					m.selectedIndex = 0
					return m, nil
				}
				// Handle backspace
				if msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
					if len(m.searchText) > 0 {
						m.searchText = m.searchText[:len(m.searchText)-1]
						m.scrollOffset = 0
						m.selectedIndex = 0
					}
					return m, nil
				}
				// Handle space
				if msg.Type == tea.KeySpace {
					m.searchText += " "
					m.scrollOffset = 0
					m.selectedIndex = 0
					return m, nil
				}
			}
		}

	case clusterDataMsg:
		m.refreshInFlight = false
		m.err = msg.err

		// Only update data and counters if successful
		if msg.err == nil && msg.data != nil {
			m.clusterData = msg.data
			m.lastUpdate = time.Now()
			m.refreshCounter++

			// Use the summary's LastRefreshTime (set by the refresher) to determine whether
			// this snapshot represents new metrics. This avoids both duplicate entries
			// and the missed-updates problem when cache returns the same pointer repeatedly.
			var snapshotTime time.Time
			if msg.data.Summary != nil && !msg.data.Summary.LastRefreshTime.IsZero() {
				snapshotTime = msg.data.Summary.LastRefreshTime
			} else {
				snapshotTime = time.Now()
			}

			if snapshotTime.After(m.lastSnapshotTime) {
				m.recordMetricSnapshot(msg.data)
				// Same guard, second ring: the sparkline series must not draw a
				// cached repeat as a new sample either.
				m.recordVizSample(msg.data, snapshotTime)
				m.lastSnapshotTime = snapshotTime
			}

			// Clamp selectedIndex to valid range after data refresh
			// This must run even in detailMode, because if data changes while viewing detail,
			// selectedIndex may point to non-existent items when user returns to list view
			maxIndex := m.getMaxIndex()
			if maxIndex > 0 && m.selectedIndex >= maxIndex {
				m.selectedIndex = maxIndex - 1
			}
			if m.selectedIndex < 0 {
				m.selectedIndex = 0
			}
		}

		return m, nil

	case logsMsg:
		// Only process log messages if still in logs mode
		// This prevents race conditions when user exits logs mode but async fetch completes
		if !m.logsMode {
			return m, nil
		}

		if msg.err != nil {
			m.logsError = msg.err.Error()
			m.containerLogs = ""
			m.logsFetched = false
			m.cachedLogLines = nil
		} else {
			m.logsError = ""
			m.logsFetched = true
			wasEmpty := m.containerLogs == ""
			m.containerLogs = msg.logs

			// Split once and cache the log lines to avoid repeated splits during rendering
			logLines := strings.Split(m.containerLogs, "\n")

			// Limit log size to prevent performance issues
			// Keep only the last 10000 lines
			const maxLogLines = 10000
			if len(logLines) > maxLogLines {
				// Keep only the last maxLogLines
				logLines = logLines[len(logLines)-maxLogLines:]
				m.containerLogs = strings.Join(logLines, "\n")
			}

			// Cache the split log lines for fast rendering
			m.cachedLogLines = logLines

			m.logsLastUpdate = time.Now() // Update refresh timestamp

			// Initialize scroll position when first receiving logs
			if wasEmpty && m.containerLogs != "" {
				m.initLogsScrollPosition()
			} else if m.logsAutoScroll {
				// Only auto-scroll if enabled and not first time
				// Calculate the new bottom position using wrapped line count
				maxVisible := m.logsRows()
				totalLines := m.getLogsDisplayLineCount()
				maxScroll := totalLines - maxVisible
				if maxScroll < 0 {
					maxScroll = 0
				}
				m.logsScrollOffset = maxScroll
			}

			// Start auto-refresh if not already running
			if !m.logsAutoRefresh {
				m.logsAutoRefresh = true
				return m, m.startLogsRefresh()
			}
		}
		return m, nil

	case logsRefreshTickMsg:
		// Auto-refresh logs if still in logs mode (but not in search mode)
		// Pause refresh during search to avoid performance issues with large logs
		if m.logsMode && m.logsAutoRefresh && !m.logsSearchMode {
			return m, tea.Batch(
				m.fetchLogs(),
				m.startLogsRefresh(), // Schedule next refresh
			)
		}
		// If in search mode, still schedule next tick but don't fetch
		if m.logsMode && m.logsAutoRefresh && m.logsSearchMode {
			return m, m.startLogsRefresh()
		}
		return m, nil

	case exportSuccessMsg:
		m.exportInProgress = false
		m.exportMessage = m.TF("export.success", map[string]interface{}{
			"Count": msg.count,
			"Path":  msg.filePath,
		})
		return m, tea.Tick(time.Second*3, func(time.Time) tea.Msg {
			return clearExportMessageMsg{}
		})

	case exportErrorMsg:
		m.exportInProgress = false
		m.exportMessage = m.TF("export.failed", map[string]interface{}{
			"Err": msg.err,
		})
		return m, tea.Tick(time.Second*3, func(time.Time) tea.Msg {
			return clearExportMessageMsg{}
		})

	case clipboardResultMsg:
		if msg.err != nil {
			m.exportMessage = m.TF("clipboard.failed", map[string]interface{}{
				"Err": msg.err,
			})
		} else {
			m.exportMessage = m.TF("clipboard.success", map[string]interface{}{
				"Text": msg.text,
			})
		}
		return m, tea.Tick(time.Second*2, func(time.Time) tea.Msg {
			return clearExportMessageMsg{}
		})

	case commandOutputMsg:
		// Display command output in viewer mode
		m.commandOutputMode = true
		m.commandOutputTitle = msg.title
		m.commandOutputContent = msg.content
		m.commandOutputScroll = 0
		return m, nil

	case clearExportMessageMsg:
		m.exportMessage = ""
		return m, nil

	case errMsg:
		m.refreshInFlight = false
		m.err = msg.err
		return m, nil
	}

	return m, nil
}
