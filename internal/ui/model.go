package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/chenpu/k8s-monitor/internal/model"
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
	SortByCPU
	SortByMemory
	SortByPods      // For Nodes view
	SortByRestarts  // For Pods view
	SortByNamespace // For Pods view
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
	ClusterNPUCapacity    int64 // Total NPU capacity across all nodes
	ClusterNPUAllocated   int64 // Total NPUs allocated to pods
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
	NPUCapacity    int64 // Total NPU capacity on this node
	NPUAllocated   int64 // NPUs allocated to pods on this node
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
	refreshInFlight     bool // True while a cluster data fetch is awaiting its clusterDataMsg
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
	jobPodSelectedIndex        int  // Selected pod index in job detail view
	volcanoJobPodSelectedIndex int  // Selected pod index in volcano job detail view
	fromJobDetail              bool // True when navigating from job detail to pod detail
	fromVolcanoJobDetail       bool // True when navigating from volcano job detail to pod detail

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
	logsFetched       bool      // True once a fetch completed successfully (logs may legitimately be empty)
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

// viewSwitchBlocked reports whether a mode that owns the keyboard is active.
// The digit and Tab shortcuts only switch the view in the plain list views: in
// any other mode they would change the view underneath the panel the user is
// looking at (and, for filter mode, leave the panel active over the new view).
func (m *Model) viewSwitchBlocked() bool {
	return m.detailMode || m.filterMode || m.logsMode || m.commandOutputMode || m.actionMenuMode
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.startFetchData(),
		tea.EnterAltScreen,
		m.scheduleRefresh(),
	)
}
