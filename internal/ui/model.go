package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourusername/k8s-monitor/internal/i18n"
	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// DataProvider defines the interface for getting cluster data
type DataProvider interface {
	GetClusterData() (*model.ClusterData, error)
	ForceRefresh() error
}

// ViewType represents different view types
type ViewType int

const (
	ViewOverview ViewType = iota
	ViewNodes
	ViewPods
	ViewWorkloads
	ViewNetwork
	ViewStorage
	ViewEvents
	ViewAlerts
	ViewQueues   // Volcano Queues view (AI/HPC)
	ViewTopology // SuperPod Topology view (Ascend NPU)
	ViewNodeDetail
	ViewPodDetail
	ViewEventDetail
	ViewJobDetail
	ViewServiceDetail
	ViewDeploymentDetail
	ViewStatefulSetDetail
	ViewDaemonSetDetail
	ViewCronJobDetail
	ViewPVDetail
	ViewPVCDetail
	ViewVolcanoJobDetail
	ViewQueueDetail
	ViewTopologyDetail // SuperPod detail view
)

// SortField represents the field to sort by
type SortField int

const (
	SortByName SortField = iota
	SortByStatus
	SortByCPU
	SortByMemory
	SortByPods      // For Nodes view
	SortByRestarts  // For Pods view
	SortByNamespace // For Pods view
	SortByNode      // For Pods view
)

// SortOrder represents sort direction
type SortOrder int

const (
	SortAsc SortOrder = iota
	SortDesc
)

// MetricSnapshot represents a snapshot of metrics at a point in time
type MetricSnapshot struct {
	NodeMetrics map[string]*NodeMetric // key: node name
	PodMetrics  map[string]*PodMetric  // key: namespace/name
	Timestamp   time.Time

	// Cluster-wide NPU summary
	ClusterNPUCapacity   int64 // Total NPU capacity across all nodes
	ClusterNPUAllocated  int64 // Total NPUs allocated to pods
	ClusterNPUAllocatable int64 // Total allocatable NPUs
}

// NodeMetric stores historical metrics for a node
type NodeMetric struct {
	CPUUsage       int64
	MemoryUsage    int64
	NetworkRxBytes int64
	NetworkTxBytes int64
	Timestamp      time.Time // Kubelet-provided timestamp for accurate rate calculation

	// NPU metrics (Ascend AI accelerators)
	NPUCapacity   int64 // Total NPU capacity on this node
	NPUAllocated  int64 // NPUs allocated to pods on this node
	NPUAllocatable int64 // Allocatable NPUs on this node
}

// PodMetric stores historical metrics for a pod
type PodMetric struct {
	CPUUsage       int64
	MemoryUsage    int64
	NetworkRxBytes int64
	NetworkTxBytes int64
	Timestamp      time.Time // Kubelet-provided timestamp for accurate rate calculation
}

// Trend represents the trend direction
type Trend int

const (
	TrendStable Trend = iota
	TrendUp
	TrendDown
)

// Model is the main UI model
type Model struct {
	dataProvider        DataProvider
	logger              *zap.Logger
	localizer           *i18n.Localizer // Translator for i18n support
	locale              string          // Current locale (en, zh, etc.)
	version             string          // Application version
	refreshInterval     time.Duration
	logTailLines        int // Number of log lines to fetch
	refreshCounter      int
	width               int
	height              int
	currentView         ViewType
	clusterData         *model.ClusterData
	lastUpdate          time.Time
	err                 error
	keys                KeyMap
	quitting            bool
	scrollOffset        int                    // Scroll offset for lists
	selectedIndex       int                    // Selected item index in lists
	detailMode          bool                   // True when viewing node/pod/event/job detail
	detailScrollOffset  int                    // Scroll offset for detail view content
	selectedNode        *model.NodeData        // Currently selected node for detail view
	selectedPod         *model.PodData         // Currently selected pod for detail view
	selectedEvent       *model.EventData       // Currently selected event for detail view
	selectedJob         *model.JobData         // Currently selected job for detail view
	selectedService     *model.ServiceData     // Currently selected service for detail view
	selectedDeployment  *model.DeploymentData  // Currently selected deployment for detail view
	selectedStatefulSet *model.StatefulSetData // Currently selected statefulset for detail view
	selectedDaemonSet   *model.DaemonSetData   // Currently selected daemonset for detail view
	selectedCronJob     *model.CronJobData     // Currently selected cronjob for detail view
	selectedPV          *model.PVData          // Currently selected PV for detail view
	selectedPVC         *model.PVCData         // Currently selected PVC for detail view
	selectedVolcanoJob  *model.VolcanoJobData  // Currently selected Volcano job for detail view
	selectedQueue       *model.QueueData       // Currently selected Volcano queue for detail view
	selectedSuperPod    *SuperPodInfo          // Currently selected SuperPod for detail view

	// Job pod selection state
	jobPodSelectedIndex         int  // Selected pod index in job detail view
	volcanoJobPodSelectedIndex  int  // Selected pod index in volcano job detail view
	fromJobDetail               bool // True when navigating from job detail to pod detail
	fromVolcanoJobDetail        bool // True when navigating from volcano job detail to pod detail

	// Filter state
	filterMode      bool   // True when in filter mode
	filterNamespace string // Current namespace filter (pods only)
	filterStatus    string // Current status filter (nodes: Ready/NotReady, pods: Running/Pending/Failed)
	filterRole      string // Current role filter (nodes only)
	filterEventType string // Current event type filter (Warning/Normal)
	searchMode      bool   // True when in search mode
	searchText      string // Current search text (filter by name)

	// Sort state
	sortField SortField // Current sort field
	sortOrder SortOrder // Current sort order

	// Cached sorted data (to ensure selection consistency)
	cachedSortedNodes  []*model.NodeData
	cachedSortedPods   []*model.PodData
	cachedSortedEvents []*model.EventData

	// Metric history for trend calculation (keep last 10 snapshots)
	metricHistory    []MetricSnapshot
	maxHistory       int       // Maximum history snapshots to keep
	lastSnapshotTime time.Time // Timestamp of last recorded metric snapshot

	// Logs viewer state
	logsMode          bool      // True when viewing logs
	logsAutoRefresh   bool      // True to enable auto-refresh of logs
	logsAutoScroll    bool      // True to auto-scroll to bottom on new logs
	logsLastUpdate    time.Time // Last time logs were refreshed
	selectedContainer string    // Selected container name for logs
	containerLogs     string    // Fetched logs content
	logsScrollOffset  int       // Scroll offset for logs
	logsError         string    // Error message if logs fetch failed

	// Logs search state
	logsSearchMode bool   // True when in logs search mode
	logsSearchText string // Current search text for logs filtering

	// Logs cache for performance (avoid re-splitting on every render)
	cachedLogLines       []string // Cached split log lines
	cachedLogLinesSource string   // Source string that was cached (for invalidation)

	// Action menu state
	actionMenuMode          bool // True when action menu is visible
	actionMenuSelectedIndex int  // Selected item in action menu

	// Export state
	exportInProgress bool   // True when export is in progress
	exportMessage    string // Export success/error message

	// Workloads view state
	workloadSections map[string]workloadSection // Track each workload type's position

	// Command output viewer state
	commandOutputMode    bool   // True when viewing command output
	commandOutputTitle   string // Title of the command output
	commandOutputContent string // Content to display
	commandOutputScroll  int    // Scroll offset for command output
}

// workloadSection tracks the position and count of a workload type in the view
type workloadSection struct {
	startLine int
	count     int
	itemType  string // "service", "job", "deployment", "statefulset", "daemonset", "cronjob"
}

// KeyMap defines key bindings
type KeyMap struct {
	Quit        key.Binding
	Refresh     key.Binding
	Help        key.Binding
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Tab         key.Binding
	Enter       key.Binding
	Back        key.Binding
	Filter      key.Binding
	ClearFilter key.Binding
	Sort        key.Binding
	Search      key.Binding
	Logs        key.Binding
	Actions     key.Binding // Open action menu
	Export      key.Binding // Export current view data
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("PgUp", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("PgDn", "page down"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch view"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "detail"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc", "backspace"),
			key.WithHelp("esc", "back"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter"),
		),
		ClearFilter: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clear filter"),
		),
		Sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sort"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
		),
		Actions: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "actions"),
		),
		Export: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "export"),
		),
	}
}

// NewModel creates a new UI model
func NewModel(dataProvider DataProvider, logger *zap.Logger, refreshInterval time.Duration, locale string, version string, logTailLines int) *Model {
	return &Model{
		dataProvider:     dataProvider,
		logger:           logger,
		localizer:        i18n.NewLocalizer(locale),
		locale:           locale,
		version:          version,
		refreshInterval:  refreshInterval,
		logTailLines:     logTailLines,
		currentView:      ViewOverview,
		keys:             DefaultKeyMap(),
		metricHistory:    make([]MetricSnapshot, 0, 10),
		maxHistory:       10, // Keep last 10 snapshots for trend calculation
		workloadSections: make(map[string]workloadSection),
	}
}

// T translates a message by its ID
func (m *Model) T(messageID string) string {
	return m.localizer.T(messageID)
}

// TP translates a message with pluralization
func (m *Model) TP(messageID string, count int) string {
	return m.localizer.TP(messageID, count)
}

// TF translates a message with template data
func (m *Model) TF(messageID string, templateData map[string]interface{}) string {
	return m.localizer.TF(messageID, templateData)
}

// isChinese returns true if the current locale is Chinese
func (m *Model) isChinese() bool {
	return strings.HasPrefix(m.locale, "zh")
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchData(),
		tea.EnterAltScreen,
		m.scheduleRefresh(),
	)
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
			m.fetchData(),
			m.scheduleRefresh(),
		)

	case tea.KeyMsg:
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
					if len(msg.String()) == 1 {
						char := msg.String()
						// Allow most printable characters for log search
						if char >= " " && char <= "~" {
							m.logsSearchText += char
							m.logsScrollOffset = 0
							return m, nil
						}
					}
				}
				// Handle text input for normal search mode
				if m.searchMode {
					if len(msg.String()) == 1 {
						char := msg.String()
						// Allow alphanumeric, dash, underscore, dot
						if (char >= "a" && char <= "z") || (char >= "A" && char <= "Z") ||
							(char >= "0" && char <= "9") || char == "-" || char == "_" || char == "." {
							m.searchText += char
							m.scrollOffset = 0
							m.selectedIndex = 0
							return m, nil
						}
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
			// Only call fetchData() to avoid duplicate GetClusterData calls
			// The refresher cache will be updated automatically
			return m, m.fetchData()

		// Number keys for quick view switching
		case msg.String() == "1":
			if !m.detailMode {
				m.currentView = ViewOverview
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "2":
			if !m.detailMode {
				m.currentView = ViewNodes
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "3":
			if !m.detailMode {
				m.currentView = ViewPods
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "4":
			if !m.detailMode {
				m.currentView = ViewWorkloads
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "5":
			if !m.detailMode {
				m.currentView = ViewNetwork
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "6":
			if !m.detailMode {
				m.currentView = ViewStorage
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "7":
			if !m.detailMode {
				m.currentView = ViewEvents
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "8":
			if !m.detailMode {
				m.currentView = ViewAlerts
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "9":
			// Only switch to Queues view if Volcano is available
			if !m.detailMode && m.hasVolcanoQueues() {
				m.currentView = ViewQueues
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case msg.String() == "0":
			// Only switch to Topology view if SuperPod info is available
			if !m.detailMode && m.hasSuperPodTopology() {
				m.currentView = ViewTopology
				m.scrollOffset = 0
				m.selectedIndex = 0
			}
			return m, nil

		case key.Matches(msg, m.keys.Tab):
			// Tab key switches views in list mode
			if !m.detailMode {
				// Determine max views based on available features
				maxViews := 8 // Base: Overview, Nodes, Pods, Workloads, Network, Storage, Events, Alerts
				if m.hasVolcanoQueues() {
					maxViews = 9 // Include ViewQueues
				}
				if m.hasSuperPodTopology() {
					maxViews = 10 // Include ViewTopology
				}
				m.currentView = (m.currentView + 1) % ViewType(maxViews)
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
					m.detailMode = true  // BUGFIX: Must set detailMode to true for "l" key to work
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
					if m.selectedIndex < len(nodes) {
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
					if m.selectedIndex < len(pods) {
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
					if m.selectedIndex < len(events) {
						m.selectedEvent = events[m.selectedIndex]
						m.currentView = ViewEventDetail
						m.detailMode = true
						m.detailScrollOffset = 0 // Reset scroll when entering detail
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

					if m.selectedIndex < totalPVs {
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
					if m.selectedIndex < len(m.clusterData.Queues) {
						m.selectedQueue = m.clusterData.Queues[m.selectedIndex]
						m.currentView = ViewQueueDetail
						m.detailMode = true
						m.detailScrollOffset = 0
					}
				case ViewTopology:
					// Topology view - select SuperPod for detail
					superPods := m.getSuperPodTopology()
					if m.selectedIndex < len(superPods) {
						m.selectedSuperPod = &superPods[m.selectedIndex]
						m.currentView = ViewTopologyDetail
						m.detailMode = true
						m.detailScrollOffset = 0
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
				m.searchMode = true
				m.searchText = ""
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
						m.logsError = "No containers available"
						return m, nil
					}

					// If single container, fetch logs directly
					if len(selectedPod.ContainerStates) == 1 {
						m.selectedPod = selectedPod
						m.selectedContainer = selectedPod.ContainerStates[0].Name
						m.logsMode = true // Enter logs mode
						return m, m.fetchLogs()
					}

					// For multiple containers, select the first container by default
					m.selectedPod = selectedPod
					m.selectedContainer = selectedPod.ContainerStates[0].Name
					m.logsMode = true // Enter logs mode
						return m, m.fetchLogs()
				}
				return m, nil
			}
			if m.detailMode && m.currentView == ViewPodDetail && m.selectedPod != nil {
				// Get containers from the selected pod
				if len(m.selectedPod.ContainerStates) == 0 {
					m.logsError = "No containers available"
					return m, nil
				}

				// If single container, fetch logs directly
				if len(m.selectedPod.ContainerStates) == 1 {
					m.selectedContainer = m.selectedPod.ContainerStates[0].Name
					m.logsMode = true // Enter logs mode
					return m, m.fetchLogs()
				}

				// For multiple containers, select the first container by default
				// TODO: In future, implement container selection UI
				m.selectedContainer = m.selectedPod.ContainerStates[0].Name
				m.logsMode = true // Enter logs mode
					return m, m.fetchLogs()
			}
			return m, nil

		case key.Matches(msg, m.keys.Actions):
			// A key opens action menu in detail views
			if m.detailMode && !m.actionMenuMode {
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
				if m.detailScrollOffset > 0 {
					m.detailScrollOffset--
				}
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.currentView == ViewOverview || m.currentView == ViewNetwork {
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
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
				// Scroll down in command output view
				lines := strings.Split(m.commandOutputContent, "\n")
				maxVisible := m.height - 6
				if maxVisible < 1 {
					maxVisible = 1
				}
				totalLines := len(lines)
				maxScroll := totalLines - maxVisible
				if maxScroll < 0 {
					maxScroll = 0
				}
				if m.commandOutputScroll < maxScroll {
					m.commandOutputScroll++
				}
				return m, nil
			}
			if m.logsMode {
				// Scroll down in logs view - use cached log lines for performance
				logLines := m.cachedLogLines
				if len(logLines) == 0 {
					// Fallback if cache not populated
					logLines = strings.Split(m.containerLogs, "\n")
				}
				maxVisible := m.height - 8
				if maxVisible < 1 {
					maxVisible = 1
				}
				totalLines := len(logLines)
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
				m.detailScrollOffset++
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.currentView == ViewOverview || m.currentView == ViewNetwork {
				m.scrollOffset++
				return m, nil
			}
			// Other list views use item-based selection
			if !m.detailMode {
				maxIndex := m.getMaxIndex()
				if m.selectedIndex < maxIndex-1 {
					m.selectedIndex++
					maxVisible := m.height - 10
					if m.selectedIndex >= m.scrollOffset+maxVisible {
						m.scrollOffset++
					}
				}
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
			if m.detailMode {
				// Scroll up by a page in detail view
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				if m.detailScrollOffset >= pageSize {
					m.detailScrollOffset -= pageSize
				} else {
					m.detailScrollOffset = 0
				}
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.currentView == ViewOverview || m.currentView == ViewNetwork {
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				if m.scrollOffset >= pageSize {
					m.scrollOffset -= pageSize
				} else {
					m.scrollOffset = 0
				}
				return m, nil
			}
			// Other list views use item-based selection
			if !m.detailMode {
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
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
			return m, nil

		case key.Matches(msg, m.keys.PageDown):
			// Page down: jump by a full page
			if m.logsMode {
				// Scroll down by a page in logs view
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}

				// Calculate bounds using cached log lines for performance
				logLines := m.cachedLogLines
				if len(logLines) == 0 {
					logLines = strings.Split(m.containerLogs, "\n")
				}
				maxVisible := m.height - 8
				if maxVisible < 1 {
					maxVisible = 1
				}
				totalLines := len(logLines)
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
			if m.detailMode {
				// Scroll down by a page in detail view
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				m.detailScrollOffset += pageSize
				return m, nil
			}
			// Overview and Network views use line-based scrolling
			if m.currentView == ViewOverview || m.currentView == ViewNetwork {
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				m.scrollOffset += pageSize
				return m, nil
			}
			// Other list views use item-based selection
			if !m.detailMode {
				pageSize := m.height - 10
				if pageSize < 1 {
					pageSize = 1
				}
				maxIndex := m.getMaxIndex()
				if m.selectedIndex+pageSize < maxIndex {
					m.selectedIndex += pageSize
					m.scrollOffset += pageSize
				} else {
					m.selectedIndex = maxIndex - 1
					maxVisible := m.height - 10
					m.scrollOffset = maxIndex - maxVisible
					if m.scrollOffset < 0 {
						m.scrollOffset = 0
					}
				}
			}
			return m, nil

		// Handle text input in logs search mode and search mode
		default:
			if m.logsSearchMode {
				// Handle printable characters (allow more characters for log search)
				if len(msg.String()) == 1 {
					char := msg.String()
					// Allow most printable characters for log search
					if char >= " " && char <= "~" {
						m.logsSearchText += char
						m.logsScrollOffset = 0
						return m, nil
					}
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
				if len(msg.String()) == 1 {
					char := msg.String()
					// Allow alphanumeric, dash, underscore, dot
					if (char >= "a" && char <= "z") || (char >= "A" && char <= "Z") ||
						(char >= "0" && char <= "9") || char == "-" || char == "_" || char == "." {
						m.searchText += char
						m.scrollOffset = 0
						m.selectedIndex = 0
						return m, nil
					}
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
			m.cachedLogLines = nil
			m.cachedLogLinesSource = ""
		} else {
			m.logsError = ""
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
			m.cachedLogLinesSource = m.containerLogs

			m.logsLastUpdate = time.Now() // Update refresh timestamp

			// Initialize scroll position when first receiving logs
			if wasEmpty && m.containerLogs != "" {
				m.initLogsScrollPosition()
			} else if m.logsAutoScroll {
				// Only auto-scroll if enabled and not first time
				// Calculate the new bottom position using cached lines
				maxVisible := m.height - 8
				if maxVisible < 1 {
					maxVisible = 1
				}
				totalLines := len(m.cachedLogLines)
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
		m.exportMessage = fmt.Sprintf("✅ Exported %d items to: %s", msg.count, msg.filePath)
		return m, tea.Tick(time.Second*3, func(time.Time) tea.Msg {
			return clearExportMessageMsg{}
		})

	case exportErrorMsg:
		m.exportInProgress = false
		m.exportMessage = fmt.Sprintf("❌ Export failed: %v", msg.err)
		return m, tea.Tick(time.Second*3, func(time.Time) tea.Msg {
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
		m.err = msg.err
		return m, nil
	}

	return m, nil
}

// View renders the UI
func (m *Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if m.width == 0 {
		return "Loading..."
	}

	// Render header
	header := m.renderHeader()

	// Render command output view if in command output mode
	if m.commandOutputMode {
		content := m.renderCommandOutput()
		footer := m.renderFooter()
		return fmt.Sprintf("%s\n\n%s\n\n%s", header, content, footer)
	}

	// Render logs view if in logs mode
	if m.logsMode {
		content := m.renderLogs()
		footer := m.renderFooter()
		return fmt.Sprintf("%s\n\n%s\n\n%s", header, content, footer)
	}

	// Render current view
	var content string
	switch m.currentView {
	case ViewOverview:
		content = m.renderOverview()
	case ViewNodes:
		content = m.renderNodes()
	case ViewPods:
		content = m.renderPods()
	case ViewEvents:
		content = m.renderEvents()
	case ViewAlerts:
		content = m.renderAlerts()
	case ViewWorkloads:
		content = m.renderWorkloads()
	case ViewNetwork:
		content = m.renderNetwork()
	case ViewNodeDetail:
		content = m.renderNodeDetail()
	case ViewPodDetail:
		content = m.renderPodDetail()
	case ViewEventDetail:
		content = m.renderEventDetail()
	case ViewJobDetail:
		content = m.renderJobDetail()
	case ViewServiceDetail:
		content = m.renderServiceDetail()
	case ViewDeploymentDetail:
		content = m.renderDeploymentDetail()
	case ViewStatefulSetDetail:
		content = m.renderStatefulSetDetail()
	case ViewDaemonSetDetail:
		content = m.renderDaemonSetDetail()
	case ViewCronJobDetail:
		content = m.renderCronJobDetail()
	case ViewVolcanoJobDetail:
		content = m.renderVolcanoJobDetail()
	case ViewStorage:
		content = m.renderStorage()
	case ViewPVDetail:
		content = m.renderPVDetail()
	case ViewPVCDetail:
		content = m.renderPVCDetail()
	case ViewQueues:
		content = m.renderQueues()
	case ViewQueueDetail:
		content = m.renderQueueDetail()
	case ViewTopology:
		content = m.renderTopology()
	case ViewTopologyDetail:
		content = m.renderSuperPodDetail()
	}

	// Render footer
	footer := m.renderFooter()

	// Render view tabs (only in main views, not in logs or detail modes)
	tabs := m.renderViewTabs()

	// Build base view
	var result string
	if tabs != "" {
		result = fmt.Sprintf("%s\n\n%s\n\n%s\n%s", header, content, tabs, footer)
	} else {
		result = fmt.Sprintf("%s\n\n%s\n\n%s", header, content, footer)
	}

	// Add export message if present
	if m.exportMessage != "" {
		result += "\n\n" + StyleKey.Render(m.exportMessage)
	}

	// Overlay action menu if active (should be on top)
	if m.actionMenuMode {
		menu := m.renderActionMenu()
		// Simple overlay - just append the menu
		// In a more sophisticated implementation, you could calculate position
		// and overlay it properly, but for now we'll just show it at the bottom
		result += "\n\n" + menu
	}

	return result
}

// renderHeader renders the header
func (m *Model) renderHeader() string {
	// Title with author and version
	titleText := m.T("app.title")
	author := m.T("app.author")
	version := m.version
	title := StyleTitle.Render(fmt.Sprintf("%s  by %s  %s", titleText, author, version))

	// spinner indicator toggles on each refresh
	spinChars := []string{"◐", "◓", "◑", "◒"}
	spin := spinChars[0]
	if m.refreshCounter > 0 {
		spin = spinChars[m.refreshCounter%len(spinChars)]
	}

	var statusText string
	if m.err != nil {
		statusText = StyleError.Render(fmt.Sprintf("%s: %v", m.T("common.error"), m.err))
	} else if m.clusterData != nil {
		status := fmt.Sprintf("%s %s: %s", spin, m.T("common.last_updated"), m.lastUpdate.Format("15:04:05"))
		if m.refreshInterval > 0 {
			status += fmt.Sprintf(" • %s: %s", m.T("common.auto_refresh"), m.refreshInterval)
		}
		statusText = StyleSubtitle.Render(status)
	} else {
		loading := m.T("common.loading")
		if m.refreshInterval > 0 {
			loading = fmt.Sprintf("%s %s: %s", loading, m.T("common.auto_refresh"), m.refreshInterval)
		}
		statusText = StyleSubtitle.Render(loading)
	}

	return fmt.Sprintf("%s\n%s", title, statusText)
}

// getMaxIndex returns the maximum index for the current view
func (m *Model) getMaxIndex() int {
	if m.clusterData == nil {
		return 0
	}
	switch m.currentView {
	case ViewNodes:
		if m.cachedSortedNodes != nil {
			return len(m.cachedSortedNodes)
		}
		return len(m.getFilteredNodes())
	case ViewPods:
		if m.cachedSortedPods != nil {
			return len(m.cachedSortedPods)
		}
		return len(m.getFilteredPods())
	case ViewEvents:
		if m.cachedSortedEvents != nil {
			return len(m.cachedSortedEvents)
		}
		return len(m.getFilteredEvents())
	case ViewAlerts:
		if m.clusterData.Summary != nil {
			return len(m.clusterData.Summary.Alerts)
		}
		return 0
	case ViewWorkloads:
		// Sum all selectable workloads including Volcano jobs
		total := len(m.clusterData.Services) + len(m.clusterData.Jobs) +
			len(m.clusterData.Deployments) + len(m.clusterData.StatefulSets) +
			len(m.clusterData.DaemonSets) + len(m.clusterData.CronJobs) +
			len(m.clusterData.VolcanoJobs)
		return total
	case ViewNetwork:
		// For network view, use services count as the scrollable items
		return len(m.clusterData.Services)
	case ViewStorage:
		// Storage view shows PVs and PVCs
		return len(m.clusterData.PVs) + len(m.clusterData.PVCs)
	case ViewQueues:
		// Queue view shows Volcano queues
		return len(m.clusterData.Queues)
	case ViewTopology:
		// Topology view shows SuperPods
		return len(m.getSuperPodTopology())
	default:
		return 0
	}
}

// renderFooter renders the footer with key bindings
func (m *Model) renderFooter() string {
	bindings := []string{
		RenderKeyBinding("q", m.T("keys.quit")),
		RenderKeyBinding("r", m.T("keys.refresh")),
	}

	// Different key bindings for different modes
	if m.commandOutputMode {
		// Command output mode - show scroll and exit bindings
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.back")))
	} else if m.logsSearchMode {
		// Logs search mode - show search-specific bindings
		bindings = append(bindings, RenderKeyBinding("text", m.T("keys.type_to_search")))
		bindings = append(bindings, RenderKeyBinding("backspace", m.T("keys.delete")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.cancel")))
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
	} else if m.logsMode {
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
		bindings = append(bindings, RenderKeyBinding("/", m.T("keys.search")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.back")))
	} else if m.searchMode {
		bindings = append(bindings, RenderKeyBinding("text", m.T("keys.type_to_search")))
		bindings = append(bindings, RenderKeyBinding("backspace", m.T("keys.delete")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.cancel")))
	} else if m.filterMode {
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.select")))
		bindings = append(bindings, RenderKeyBinding("enter", m.T("keys.apply")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.cancel")))
	} else if m.detailMode {
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.back")))
		// Add logs key binding for pod detail view
		if m.currentView == ViewPodDetail {
			bindings = append(bindings, RenderKeyBinding("l", m.T("keys.logs")))
		}
		// Add actions key binding for pod and node detail views
		if m.currentView == ViewPodDetail || m.currentView == ViewNodeDetail {
			bindings = append(bindings, RenderKeyBinding("a", m.T("keys.actions")))
		}
	} else {
		bindings = append(bindings, RenderKeyBinding("1-8", m.T("keys.views")))
		bindings = append(bindings, RenderKeyBinding("tab", m.T("keys.next")))
		// Add navigation help for list views
		if m.currentView != ViewOverview {
			bindings = append(bindings, RenderKeyBinding("↑/k", m.T("keys.up")), RenderKeyBinding("↓/j", m.T("keys.down")))
			bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
			bindings = append(bindings, RenderKeyBinding("enter", m.T("keys.detail")))
			bindings = append(bindings, RenderKeyBinding("s", m.T("keys.sort")))
			bindings = append(bindings, RenderKeyBinding("/", m.T("keys.search")))
		}
		// Add filter help for Pods view
		if m.currentView == ViewPods {
			bindings = append(bindings, RenderKeyBinding("f", m.T("keys.filter")))
		}
		// Show clear if any filter is active
		if m.filterNamespace != "" || m.filterStatus != "" || m.filterRole != "" || m.searchText != "" {
			bindings = append(bindings, RenderKeyBinding("c", m.T("keys.clear")))
		}
	}

	footer := ""
	for i, binding := range bindings {
		if i > 0 {
			footer += " • "
		}
		footer += binding
	}

	return StyleKeyDesc.Render(footer)
}

// renderViewTabs renders the view navigation tabs
func (m *Model) renderViewTabs() string {
	// Don't show tabs in detail mode
	if m.detailMode {
		return ""
	}

	type viewTab struct {
		number int
		nameKey string
		view   ViewType
	}

	tabs := []viewTab{
		{1, "views.overview.name", ViewOverview},
		{2, "views.nodes.name", ViewNodes},
		{3, "views.pods.name", ViewPods},
		{4, "views.workloads.name", ViewWorkloads},
		{5, "views.network.name", ViewNetwork},
		{6, "views.storage.name", ViewStorage},
		{7, "views.events.name", ViewEvents},
		{8, "views.alerts.name", ViewAlerts},
	}

	// Add Queues tab if Volcano is available
	if m.hasVolcanoQueues() {
		tabs = append(tabs, viewTab{9, "views.queues.name", ViewQueues})
	}

	// Add Topology tab if SuperPod info is available
	if m.hasSuperPodTopology() {
		tabs = append(tabs, viewTab{0, "views.topology.name", ViewTopology})
	}

	var tabParts []string
	for _, tab := range tabs {
		tabText := fmt.Sprintf("%d:%s", tab.number, m.T(tab.nameKey))

		if m.currentView == tab.view {
			// Highlight current view
			tabParts = append(tabParts, StyleSelected.Render(" "+tabText+" "))
		} else {
			// Normal view
			tabParts = append(tabParts, StyleTextMuted.Render(" "+tabText+" "))
		}
	}

	return strings.Join(tabParts, " ")
}

// fetchData fetches cluster data
func (m *Model) fetchData() tea.Cmd {
	return func() tea.Msg {
		data, err := m.dataProvider.GetClusterData()
		return clusterDataMsg{data: data, err: err}
	}
}

func (m *Model) forceRefresh() tea.Cmd {
	return func() tea.Msg {
		if err := m.dataProvider.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

func (m *Model) scheduleRefresh() tea.Cmd {
	if m.refreshInterval <= 0 {
		return nil
	}
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// Messages
type clusterDataMsg struct {
	data *model.ClusterData
	err  error
}

type errMsg struct {
	err error
}

type refreshTickMsg struct{}

type logsRefreshTickMsg time.Time

type logsMsg struct {
	logs string
	err  error
}

// fetchLogs fetches logs for the selected pod and container
func (m *Model) fetchLogs() tea.Cmd {
	if m.selectedPod == nil || m.selectedContainer == "" {
		return nil
	}

	pod := m.selectedPod
	container := m.selectedContainer

	return func() tea.Msg {
		// Need to get the APIServerClient to call GetPodLogs
		// We'll need to add a GetPodLogs method to DataProvider interface
		// For now, we'll type assert to get access to the underlying client

		// Try to get logs through the data provider
		apiClient, ok := m.dataProvider.(interface {
			GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error)
		})

		if !ok {
			return logsMsg{err: fmt.Errorf("data provider does not support log fetching")}
		}

		ctx := context.Background()
		logs, err := apiClient.GetPodLogs(ctx, pod.Namespace, pod.Name, container, int64(m.logTailLines))
		return logsMsg{logs: logs, err: err}
	}
}

// startLogsRefresh returns a command that sends a logsRefreshTickMsg after 2 seconds
func (m *Model) startLogsRefresh() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return logsRefreshTickMsg(t)
	})
}

// initLogsScrollPosition initializes the scroll position when first entering logs mode
func (m *Model) initLogsScrollPosition() {
	if m.containerLogs == "" {
		return
	}

	// Calculate bottom position to show latest logs - use cached lines
	logLines := m.cachedLogLines
	if len(logLines) == 0 {
		logLines = strings.Split(m.containerLogs, "\n")
	}
	maxVisible := m.height - 8
	if maxVisible < 1 {
		maxVisible = 1
	}
	totalLines := len(logLines)
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.logsScrollOffset = maxScroll // Start at bottom
	m.logsAutoScroll = true         // Enable auto-scroll by default
}

// handleFilterNavigation handles navigation in filter mode
func (m *Model) handleFilterNavigation(direction int) tea.Cmd {
	namespaces := m.getNamespaces()
	totalOptions := len(namespaces) + 1 // +1 for "All" option

	// Find current selection index
	currentIdx := 0
	if m.filterNamespace != "" {
		for i, ns := range namespaces {
			if ns == m.filterNamespace {
				currentIdx = i + 1 // +1 because "All" is at index 0
				break
			}
		}
	}

	// Update selection
	newIdx := currentIdx + direction
	if newIdx < 0 {
		newIdx = totalOptions - 1
	} else if newIdx >= totalOptions {
		newIdx = 0
	}

	// Apply selection
	if newIdx == 0 {
		m.filterNamespace = ""
	} else {
		m.filterNamespace = namespaces[newIdx-1]
	}

	// Reset scroll and selection when filter changes
	m.scrollOffset = 0
	m.selectedIndex = 0

	return nil
}

// getNamespaces returns a sorted list of unique namespaces from pods
func (m *Model) getNamespaces() []string {
	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		return []string{}
	}

	nsMap := make(map[string]bool)
	for _, pod := range m.clusterData.Pods {
		nsMap[pod.Namespace] = true
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}

	// Use sort.Strings for O(n log n) sorting
	sort.Strings(namespaces)

	return namespaces
}

// getFilteredPods returns pods filtered by namespace, status, and search text
func (m *Model) getFilteredPods() []*model.PodData {
	if m.clusterData == nil {
		return []*model.PodData{}
	}

	// Apply all filters in a single pass for better performance
	filtered := make([]*model.PodData, 0, len(m.clusterData.Pods))
	searchLower := strings.ToLower(m.searchText)

	for _, pod := range m.clusterData.Pods {
		// Check namespace filter
		if m.filterNamespace != "" && pod.Namespace != m.filterNamespace {
			continue
		}

		// Check status filter
		if m.filterStatus != "" && pod.Phase != m.filterStatus {
			continue
		}

		// Check search text filter (case-insensitive partial match on name)
		if m.searchText != "" && !strings.Contains(strings.ToLower(pod.Name), searchLower) {
			continue
		}

		// All filters passed, include this pod
		filtered = append(filtered, pod)
	}

	return filtered
}

// getFilteredEvents returns events filtered by type and search text
func (m *Model) getFilteredEvents() []*model.EventData {
	if m.clusterData == nil {
		return []*model.EventData{}
	}

	// Start with all events
	filtered := m.clusterData.Events

	// Apply event type filter
	if m.filterEventType != "" {
		temp := []*model.EventData{}
		for _, event := range filtered {
			if event.Type == m.filterEventType {
				temp = append(temp, event)
			}
		}
		filtered = temp
	}

	// Apply search text filter (case-insensitive partial match on reason or message)
	if m.searchText != "" {
		temp := []*model.EventData{}
		searchLower := strings.ToLower(m.searchText)
		for _, event := range filtered {
			if strings.Contains(strings.ToLower(event.Reason), searchLower) ||
				strings.Contains(strings.ToLower(event.Message), searchLower) ||
				strings.Contains(strings.ToLower(event.InvolvedObject), searchLower) {
				temp = append(temp, event)
			}
		}
		filtered = temp
	}

	return filtered
}

// getFilteredNodes returns nodes filtered by status, role, and search text
func (m *Model) getFilteredNodes() []*model.NodeData {
	if m.clusterData == nil {
		return []*model.NodeData{}
	}

	// Start with all nodes
	filtered := m.clusterData.Nodes

	// Apply status filter
	if m.filterStatus != "" {
		temp := []*model.NodeData{}
		for _, node := range filtered {
			if node.Status == m.filterStatus {
				temp = append(temp, node)
			}
		}
		filtered = temp
	}

	// Apply role filter
	if m.filterRole != "" {
		temp := []*model.NodeData{}
		for _, node := range filtered {
			for _, role := range node.Roles {
				if role == m.filterRole {
					temp = append(temp, node)
					break
				}
			}
		}
		filtered = temp
	}

	// Apply search text filter (case-insensitive partial match on name)
	if m.searchText != "" {
		temp := []*model.NodeData{}
		searchLower := strings.ToLower(m.searchText)
		for _, node := range filtered {
			if strings.Contains(strings.ToLower(node.Name), searchLower) {
				temp = append(temp, node)
			}
		}
		filtered = temp
	}

	return filtered
}

// clusterHasNPU returns true if the cluster has any nodes with NPU
func (m *Model) clusterHasNPU() bool {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return false
	}
	return m.clusterData.Summary.NPUCapacity > 0
}

// hasVolcanoQueues returns true if Volcano queues are available
func (m *Model) hasVolcanoQueues() bool {
	if m.clusterData == nil {
		return false
	}
	return len(m.clusterData.Queues) > 0
}

// renderSearchPanel renders the search panel
func (m *Model) renderSearchPanel() string {
	var lines []string

	lines = append(lines, StyleHeader.Render(m.T("search.title")))
	lines = append(lines, "")

	// Show search input with cursor
	searchDisplay := m.searchText + "█" // Use block cursor
	if m.searchText == "" {
		searchDisplay = "█" // Just show cursor when empty
	}
	lines = append(lines, fmt.Sprintf("  %s", StyleHighlight.Render(searchDisplay)))
	lines = append(lines, "")
	lines = append(lines, StyleTextMuted.Render("  "+m.T("search.placeholder")))
	lines = append(lines, StyleTextMuted.Render("  "+m.T("search.help")))

	return strings.Join(lines, "\n")
}

// recordMetricSnapshot records a snapshot of current metrics for trend calculation
func (m *Model) recordMetricSnapshot(data *model.ClusterData) {
	snapshot := MetricSnapshot{
		NodeMetrics: make(map[string]*NodeMetric),
		PodMetrics:  make(map[string]*PodMetric),
		Timestamp:   time.Now(),
	}

	// Record node metrics and accumulate cluster-wide NPU stats
	var clusterNPUCapacity, clusterNPUAllocated, clusterNPUAllocatable int64
	for _, node := range data.Nodes {
		// Use kubelet-provided timestamp if available, otherwise fallback to snapshot time
		ts := node.NetworkTimestamp
		if ts.IsZero() {
			ts = snapshot.Timestamp
		}
		snapshot.NodeMetrics[node.Name] = &NodeMetric{
			CPUUsage:       node.CPUUsage,
			MemoryUsage:    node.MemoryUsage,
			NetworkRxBytes: node.NetworkRxBytes,
			NetworkTxBytes: node.NetworkTxBytes,
			Timestamp:      ts,
			// NPU metrics
			NPUCapacity:    node.NPUCapacity,
			NPUAllocated:   node.NPUAllocated,
			NPUAllocatable: node.NPUAllocatable,
		}

		// Accumulate cluster-wide NPU stats
		clusterNPUCapacity += node.NPUCapacity
		clusterNPUAllocated += node.NPUAllocated
		clusterNPUAllocatable += node.NPUAllocatable
	}

	// Set cluster-wide NPU summary
	snapshot.ClusterNPUCapacity = clusterNPUCapacity
	snapshot.ClusterNPUAllocated = clusterNPUAllocated
	snapshot.ClusterNPUAllocatable = clusterNPUAllocatable

	// Record pod metrics
	for _, pod := range data.Pods {
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		// Use kubelet-provided timestamp if available, otherwise fallback to snapshot time
		ts := pod.NetworkTimestamp
		if ts.IsZero() {
			ts = snapshot.Timestamp
		}
		snapshot.PodMetrics[key] = &PodMetric{
			CPUUsage:       pod.CPUUsage,
			MemoryUsage:    pod.MemoryUsage,
			NetworkRxBytes: pod.NetworkRxBytes,
			NetworkTxBytes: pod.NetworkTxBytes,
			Timestamp:      ts,
		}
	}

	// Add to history
	m.metricHistory = append(m.metricHistory, snapshot)

	// Keep only last N snapshots
	if len(m.metricHistory) > m.maxHistory {
		m.metricHistory = m.metricHistory[1:]
	}
}

// calculateNodeCPUTrend calculates CPU trend for a node
func (m *Model) calculateNodeCPUTrend(nodeName string, currentCPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values (excluding the most recent snapshot which is current)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].NodeMetrics[nodeName]; ok {
			historicalValues = append(historicalValues, metric.CPUUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average of historical values
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (5% threshold)
	threshold := avg / 20 // 5%
	if threshold < 10 {
		threshold = 10 // Minimum threshold to avoid noise
	}

	if currentCPU > avg+threshold {
		return TrendUp
	} else if currentCPU < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// calculateNodeMemoryTrend calculates memory trend for a node
func (m *Model) calculateNodeMemoryTrend(nodeName string, currentMemory int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].NodeMetrics[nodeName]; ok {
			historicalValues = append(historicalValues, metric.MemoryUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (5% threshold)
	threshold := avg / 20
	if threshold < 1024*1024*10 { // 10MB minimum threshold
		threshold = 1024 * 1024 * 10
	}

	if currentMemory > avg+threshold {
		return TrendUp
	} else if currentMemory < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// calculatePodCPUTrend calculates CPU trend for a pod
func (m *Model) calculatePodCPUTrend(namespace, name string, currentCPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].PodMetrics[key]; ok {
			historicalValues = append(historicalValues, metric.CPUUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	threshold := avg / 20
	if threshold < 10 {
		threshold = 10
	}

	if currentCPU > avg+threshold {
		return TrendUp
	} else if currentCPU < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// calculatePodMemoryTrend calculates memory trend for a pod
func (m *Model) calculatePodMemoryTrend(namespace, name string, currentMemory int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].PodMetrics[key]; ok {
			historicalValues = append(historicalValues, metric.MemoryUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	threshold := avg / 20
	if threshold < 1024*1024*10 {
		threshold = 1024 * 1024 * 10
	}

	if currentMemory > avg+threshold {
		return TrendUp
	} else if currentMemory < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// renderTrendIndicator returns a visual indicator for a trend
func renderTrendIndicator(trend Trend) string {
	switch trend {
	case TrendUp:
		return StyleStatusNotReady.Render("↑")
	case TrendDown:
		return StyleStatusReady.Render("↓")
	default:
		return StyleTextMuted.Render("→")
	}
}

// getNodeCPUHistory returns CPU usage history for a node
func (m *Model) getNodeCPUHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			// Convert millicores to percentage if we have capacity info
			// For now just use the absolute value
			history = append(history, float64(metric.CPUUsage)/1000.0) // Convert to cores
		}
	}
	return history
}

// getNodeMemoryHistory returns memory usage history for a node
func (m *Model) getNodeMemoryHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			// Convert bytes to GiB
			history = append(history, float64(metric.MemoryUsage)/1024/1024/1024)
		}
	}
	return history
}

// getPodCPUHistory returns CPU usage history for a pod
func (m *Model) getPodCPUHistory(namespace, name string) []float64 {
	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.PodMetrics[key]; ok {
			// Convert millicores to cores
			history = append(history, float64(metric.CPUUsage)/1000.0)
		}
	}
	return history
}

// getPodMemoryHistory returns memory usage history for a pod
func (m *Model) getPodMemoryHistory(namespace, name string) []float64 {
	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.PodMetrics[key]; ok {
			// Convert bytes to MiB
			history = append(history, float64(metric.MemoryUsage)/1024/1024)
		}
	}
	return history
}

// getNodeNetworkRxHistory returns network RX rate history for a node (MB/s)
func (m *Model) getNodeNetworkRxHistory(nodeName string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].NodeMetrics[nodeName]
		currMetric, currOk := m.metricHistory[i].NodeMetrics[nodeName]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
				// Avoid negative rates (can happen if pod/node restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// getNodeNetworkTxHistory returns network TX rate history for a node (MB/s)
func (m *Model) getNodeNetworkTxHistory(nodeName string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].NodeMetrics[nodeName]
		currMetric, currOk := m.metricHistory[i].NodeMetrics[nodeName]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
				// Avoid negative rates (can happen if pod/node restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// getPodNetworkRxHistory returns network RX rate history for a pod (MB/s)
func (m *Model) getPodNetworkRxHistory(namespace, name string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].PodMetrics[key]
		currMetric, currOk := m.metricHistory[i].PodMetrics[key]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
				// Avoid negative rates (can happen if pod restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// getPodNetworkTxHistory returns network TX rate history for a pod (MB/s)
func (m *Model) getPodNetworkTxHistory(namespace, name string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].PodMetrics[key]
		currMetric, currOk := m.metricHistory[i].PodMetrics[key]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
				// Avoid negative rates (can happen if pod restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// calculatePodNetworkRxRate calculates the current RX rate for a pod (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculatePodNetworkRxRate(namespace, name string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	key := fmt.Sprintf("%s/%s", namespace, name)

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when pod has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].PodMetrics[key]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.PodMetrics[key]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/pod restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculatePodNetworkTxRate calculates the current TX rate for a pod (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculatePodNetworkTxRate(namespace, name string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	key := fmt.Sprintf("%s/%s", namespace, name)

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when pod has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].PodMetrics[key]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.PodMetrics[key]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/pod restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculateNodeNetworkRxRate calculates the current RX rate for a node (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculateNodeNetworkRxRate(nodeName string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when node has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].NodeMetrics[nodeName]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.NodeMetrics[nodeName]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/node restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculateNodeNetworkTxRate calculates the current TX rate for a node (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculateNodeNetworkTxRate(nodeName string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when node has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].NodeMetrics[nodeName]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.NodeMetrics[nodeName]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/node restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculateClusterNetworkRate calculates cluster-wide network rate (RX, TX in MB/s)
// Sums up individual node rates calculated with sliding window average
func (m *Model) calculateClusterNetworkRate() (float64, float64) {
	if len(m.metricHistory) < 2 {
		return 0, 0
	}

	if m.clusterData == nil {
		return 0, 0
	}

	// Calculate cluster rate by summing individual node rates
	// This ensures consistency with what users see in Node view
	var totalRxRate, totalTxRate float64

	for _, node := range m.clusterData.Nodes {
		// Use the same calculation method as individual node display
		rxRate := m.calculateNodeNetworkRxRate(node.Name)
		txRate := m.calculateNodeNetworkTxRate(node.Name)

		totalRxRate += rxRate
		totalTxRate += txRate
	}

	return totalRxRate, totalTxRate
}

// ============================================================================
// NPU Trend/History Functions
// ============================================================================

// getNodeNPUAllocatedHistory returns NPU allocation history for a node
func (m *Model) getNodeNPUAllocatedHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			history = append(history, float64(metric.NPUAllocated))
		}
	}
	return history
}

// getNodeNPUUtilizationHistory returns NPU utilization percentage history for a node
func (m *Model) getNodeNPUUtilizationHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			if metric.NPUAllocatable > 0 {
				util := float64(metric.NPUAllocated) / float64(metric.NPUAllocatable) * 100
				history = append(history, util)
			} else {
				history = append(history, 0)
			}
		}
	}
	return history
}

// getClusterNPUAllocatedHistory returns cluster-wide NPU allocation history
func (m *Model) getClusterNPUAllocatedHistory() []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		history = append(history, float64(snapshot.ClusterNPUAllocated))
	}
	return history
}

// getClusterNPUUtilizationHistory returns cluster-wide NPU utilization percentage history
func (m *Model) getClusterNPUUtilizationHistory() []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if snapshot.ClusterNPUAllocatable > 0 {
			util := float64(snapshot.ClusterNPUAllocated) / float64(snapshot.ClusterNPUAllocatable) * 100
			history = append(history, util)
		} else {
			history = append(history, 0)
		}
	}
	return history
}

// calculateNodeNPUTrend calculates NPU allocation trend for a node
func (m *Model) calculateNodeNPUTrend(nodeName string, currentNPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values (excluding the most recent snapshot which is current)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].NodeMetrics[nodeName]; ok {
			historicalValues = append(historicalValues, metric.NPUAllocated)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average of historical values
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (any change in NPU allocation is significant)
	if currentNPU > avg {
		return TrendUp
	} else if currentNPU < avg {
		return TrendDown
	}
	return TrendStable
}

// calculateClusterNPUTrend calculates cluster-wide NPU allocation trend
func (m *Model) calculateClusterNPUTrend(currentNPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		historicalValues = append(historicalValues, m.metricHistory[i].ClusterNPUAllocated)
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (any change in NPU allocation is significant)
	if currentNPU > avg {
		return TrendUp
	} else if currentNPU < avg {
		return TrendDown
	}
	return TrendStable
}
