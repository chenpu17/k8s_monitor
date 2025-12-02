package model

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ClusterData represents the overall cluster state
type ClusterData struct {
	Nodes        []*NodeData
	Pods         []*PodData
	Events       []*EventData
	Services     []*ServiceData
	PVs          []*PVData
	PVCs         []*PVCData
	Deployments  []*DeploymentData
	StatefulSets []*StatefulSetData
	DaemonSets   []*DaemonSetData
	Jobs         []*JobData
	CronJobs     []*CronJobData
	Summary      *ClusterSummary

	// Volcano scheduler data
	VolcanoJobs    []*VolcanoJobData
	HyperNodes     []*HyperNodeData
	Queues         []*QueueData
	VolcanoSummary *VolcanoSummary
}

// ClusterSummary provides high-level cluster metrics
type ClusterSummary struct {
	// Node metrics
	TotalNodes    int
	ReadyNodes    int
	NotReadyNodes int

	// Pod metrics
	TotalPods   int
	RunningPods int
	PendingPods int
	FailedPods  int
	UnknownPods int

	// Event metrics
	TotalEvents   int
	WarningEvents int
	ErrorEvents   int

	// Cluster-wide CPU metrics (millicores)
	CPUCapacity    int64 // Total CPU capacity across all nodes
	CPUAllocatable int64 // Total allocatable CPU
	CPURequested   int64 // Total CPU requested by pods
	CPULimited     int64 // Total CPU limits set by pods
	CPUUsed        int64 // Total CPU actually used (from metrics)

	// Cluster-wide Memory metrics (bytes)
	MemoryCapacity    int64 // Total memory capacity across all nodes
	MemoryAllocatable int64 // Total allocatable memory
	MemoryRequested   int64 // Total memory requested by pods
	MemoryLimited     int64 // Total memory limits set by pods
	MemoryUsed        int64 // Total memory actually used (from metrics)

	// Cluster-wide Pod capacity
	PodCapacity    int64 // Total pod capacity across all nodes
	PodAllocatable int64 // Total allocatable pods

	// Utilization percentages
	CPURequestUtilization float64 // CPURequested / CPUAllocatable * 100
	CPUUsageUtilization   float64 // CPUUsed / CPUCapacity * 100
	MemRequestUtilization float64 // MemoryRequested / MemoryAllocatable * 100
	MemUsageUtilization   float64 // MemoryUsed / MemoryCapacity * 100
	PodUtilization        float64 // TotalPods / PodAllocatable * 100

	// Workload statistics
	TotalDeployments  int
	TotalStatefulSets int
	TotalDaemonSets   int
	TotalJobs         int
	TotalCronJobs     int

	// Service statistics
	TotalServices     int
	ClusterIPServices int
	NodePortServices  int
	LoadBalancerSvcs  int

	// Storage statistics
	TotalPVs         int
	BoundPVs         int
	AvailablePVs     int
	ReleasedPVs      int
	TotalPVCs        int
	BoundPVCs        int
	PendingPVCs      int
	TotalStorageSize int64 // Total PV capacity in bytes
	UsedStorageSize  int64 // Total bound PV size in bytes

	// Network statistics (from kubelet metrics)
	NetworkRxBytes          int64    // Total received bytes across all nodes
	NetworkTxBytes          int64    // Total transmitted bytes across all nodes
	NetworkRxRate           int64    // Receive rate in bytes/sec
	NetworkTxRate           int64    // Transmit rate in bytes/sec
	NodesWithMetrics        int      // Number of nodes with kubelet metrics
	NodesWithoutMetrics     int      // Number of nodes missing kubelet metrics
	KubeletMetricsAvailable bool     // True if at least one kubelet metrics call succeeded
	KubeletError            string   // First kubelet error encountered
	KubeletErrors           []string // Unique kubelet error messages

	// Node health statistics (pressure indicators)
	MemoryPressureNodes int // Number of nodes with memory pressure
	DiskPressureNodes   int // Number of nodes with disk pressure
	PIDPressureNodes    int // Number of nodes with PID pressure

	// Pod anomaly statistics
	CrashLoopBackOffPods  int              // Pods in CrashLoopBackOff state
	ImagePullBackOffPods  int              // Pods failing to pull images
	OOMKilledPods         int              // Pods killed due to OOM
	HighRestartPods       []PodRestartInfo // Pods with high restart counts (Top 5)
	ContainerCreatingPods int              // Pods stuck in ContainerCreating

	// Service health statistics
	NoEndpointServices int     // Services with 0 ready endpoints
	TotalEndpoints     int     // Total number of ready endpoints
	AvgEndpointsPerSvc float64 // Average endpoints per service

	// Storage utilization
	StorageUsagePercent float64 // UsedStorageSize / TotalStorageSize * 100

	// Resource limits utilization
	CPULimitUtilization float64 // CPULimited / CPUAllocatable * 100
	MemLimitUtilization float64 // MemoryLimited / MemoryAllocatable * 100

	// NPU statistics (Ascend AI accelerators)
	NPUCapacity    int64   // Total NPU capacity across all nodes
	NPUAllocatable int64   // Total allocatable NPUs
	NPUAllocated   int64   // Total NPUs allocated to pods
	NPUUtilization float64 // NPUAllocated / NPUAllocatable * 100
	NPUNodesCount  int     // Number of nodes with NPU

	// NPU type information
	NPUResourceName string // e.g., "huawei.com/ascend-1980"
	NPUChipType     string // e.g., "Ascend910", "Ascend310"

	// Topology information (Volcano HyperNode)
	HyperClusterID   string // volcano.sh/hypercluster
	HyperNodeCount   int    // Number of HyperNodes (Tier 1)
	SuperPodCount    int    // Number of SuperPods

	LastRefreshTime time.Time

	// Alerts collected from cluster state
	Alerts []Alert
}

// AlertType identifies the specific type of alert for i18n and actions
type AlertType string

const (
	// Node alert types
	AlertTypeNodeNotReady       AlertType = "node_not_ready"
	AlertTypeNodeMemoryPressure AlertType = "node_memory_pressure"
	AlertTypeNodeDiskPressure   AlertType = "node_disk_pressure"
	AlertTypeNodePIDPressure    AlertType = "node_pid_pressure"
	AlertTypeNodeCPUCritical    AlertType = "node_cpu_critical"
	AlertTypeNodeCPUHigh        AlertType = "node_cpu_high"
	AlertTypeNodeMemoryCritical AlertType = "node_memory_critical"
	AlertTypeNodeMemoryHigh     AlertType = "node_memory_high"

	// Pod alert types
	AlertTypePodOOMKilled         AlertType = "pod_oom_killed"
	AlertTypePodCrashLoopBackOff  AlertType = "pod_crash_loop"
	AlertTypePodImagePullBackOff  AlertType = "pod_image_pull"
	AlertTypePodHighRestarts      AlertType = "pod_high_restarts"
	AlertTypePodPendingTooLong    AlertType = "pod_pending_long"
	AlertTypePodFailed            AlertType = "pod_failed"
	AlertTypePodEvicted           AlertType = "pod_evicted"
	AlertTypePodUnschedulable     AlertType = "pod_unschedulable"

	// Service alert types
	AlertTypeServiceNoEndpoints AlertType = "service_no_endpoints"

	// Storage alert types
	AlertTypePVCPendingTooLong AlertType = "pvc_pending_long"
	AlertTypePVCNearCapacity   AlertType = "pvc_near_capacity"

	// Resource alert types
	AlertTypeClusterCPUCritical    AlertType = "cluster_cpu_critical"
	AlertTypeClusterMemoryCritical AlertType = "cluster_memory_critical"
	AlertTypeClusterPodCapacity    AlertType = "cluster_pod_capacity"
)

// Alert represents a resource alert
type Alert struct {
	Severity          AlertSeverity // Critical, Warning, Info
	Category          string        // Resource, Pod, Node, Storage, Network, Service
	AlertType         AlertType     // Specific alert type for i18n and actions
	ResourceType      string        // Node, Pod, PVC, Service, etc.
	ResourceName      string
	Namespace         string // Empty for cluster-scoped resources
	Message           string
	Value             string // e.g., "95.2%", "3 restarts"
	Threshold         string // e.g., "80%"
	RecommendedAction string // Suggested action to resolve the alert
	Timestamp         time.Time
}

// AlertSeverity represents the severity level of an alert
type AlertSeverity int

const (
	AlertSeverityInfo AlertSeverity = iota
	AlertSeverityWarning
	AlertSeverityCritical
)

func (s AlertSeverity) String() string {
	switch s {
	case AlertSeverityCritical:
		return "Critical"
	case AlertSeverityWarning:
		return "Warning"
	case AlertSeverityInfo:
		return "Info"
	default:
		return "Unknown"
	}
}

// PodRestartInfo represents a pod with high restart count
type PodRestartInfo struct {
	Name         string
	Namespace    string
	RestartCount int32
	Reason       string // Last container termination reason
}

// NodeData represents a Kubernetes node with metrics
type NodeData struct {
	Name              string
	InternalIP        string
	ExternalIP        string
	Roles             []string
	Status            string // Ready, NotReady, Unknown
	Conditions        []corev1.NodeCondition
	Taints            []corev1.Taint
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time

	// Capacity and Allocatable
	CPUCapacity    int64 // millicores
	MemoryCapacity int64 // bytes
	PodCapacity    int64
	CPUAllocatable int64
	MemAllocatable int64
	PodAllocatable int64

	// Usage metrics (from kubelet or metrics-server)
	CPUUsage    int64 // millicores
	MemoryUsage int64 // bytes
	PodCount    int

	// Network metrics (from kubelet)
	NetworkRxBytes     int64     // Total received bytes
	NetworkTxBytes     int64     // Total transmitted bytes
	NetworkTimestamp   time.Time // Kubelet-provided timestamp for network metrics

	// Derived metrics
	CPUUsagePercent    float64
	MemoryUsagePercent float64
	PodUsagePercent    float64

	// Pressure indicators
	MemoryPressure bool
	DiskPressure   bool
	PIDPressure    bool

	// Metrics availability
	HasKubeletMetrics bool
	KubeletError      string

	// NPU (Ascend AI accelerator) information
	NPUCapacity     int64  // Total NPU capacity on this node
	NPUAllocatable  int64  // Allocatable NPUs on this node
	NPUAllocated    int64  // NPUs currently allocated to pods on this node
	NPUResourceName string // Resource name, e.g., "huawei.com/ascend-1980"

	// NPU device information (from node labels/annotations)
	NPUChipType      string // e.g., "Ascend910" from node.kubernetes.io/npu.chip.name
	NPUDeviceType    string // e.g., "ascend-snt9c" from accelerator/huawei-npu
	NPUDriverVersion string // e.g., "7.7.0.9.220-25.2.1" from os.modelarts.node/npu.firmware.driver.version

	// NPU runtime metrics (from annotations or external metrics)
	NPUUtilization    float64 // AI Core utilization percentage (0-100)
	NPUMemoryTotal    int64   // HBM total in bytes
	NPUMemoryUsed     int64   // HBM used in bytes
	NPUMemoryUtil     float64 // HBM utilization percentage (0-100)
	NPUTemperature    int     // NPU temperature in Celsius
	NPUPower          int     // NPU power consumption in Watts
	NPUHealthStatus   string  // Health status: "Healthy", "Warning", "Unhealthy"
	NPUErrorCount     int     // Number of NPU errors detected
	NPUAICoreCount    int     // Number of AI cores per NPU
	NPUMetricsTime    time.Time // Timestamp of last metrics update

	// Topology information (from node labels)
	HyperNodeID    string // volcano.sh/hypernode
	HyperClusterID string // volcano.sh/hypercluster
	SuperPodID     string // os.modelarts.node/superpod.id
	CabinetInfo    string // cce.kubectl.kubernetes.io/cabinet
}

// PodData represents a Kubernetes pod with status
type PodData struct {
	Name              string
	Namespace         string
	Node              string
	Phase             string // Pending, Running, Succeeded, Failed, Unknown
	Reason            string
	Message           string
	HostIP            string
	PodIP             string
	QOSClass          string
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time
	StartTime         time.Time

	// Container status
	Containers      int
	ReadyContainers int
	RestartCount    int32
	ContainerStates []ContainerState

	// Resource requests/limits
	CPURequest    int64 // millicores
	CPULimit      int64
	MemoryRequest int64 // bytes
	MemoryLimit   int64

	// NPU requests (Ascend AI accelerators)
	NPURequest      int64  // Number of NPUs requested
	NPUResourceName string // Resource name, e.g., "huawei.com/ascend-1980"

	// Usage metrics (from kubelet)
	CPUUsage         int64
	MemoryUsage      int64
	NetworkRxBytes   int64
	NetworkTxBytes   int64
	NetworkTimestamp time.Time // Kubelet-provided timestamp for network metrics

	// Conditions
	Conditions []corev1.PodCondition
}

// ContainerState represents container status
type ContainerState struct {
	Name         string
	Image        string
	Ready        bool
	RestartCount int32
	State        string // Running, Waiting, Terminated
	Reason       string
	Message      string
	ExitCode     int32

	// Resource usage (from kubelet metrics)
	CPUUsage    int64 // millicores
	MemoryUsage int64 // bytes

	// Resource requests and limits
	CPURequest    int64 // millicores
	CPULimit      int64 // millicores
	MemoryRequest int64 // bytes
	MemoryLimit   int64 // bytes
}

// EventData represents a Kubernetes event
type EventData struct {
	Type              string // Normal, Warning, Error
	Reason            string
	Message           string
	Count             int32
	FirstTimestamp    time.Time
	LastTimestamp     time.Time
	InvolvedObject    string // e.g., "Pod/mypod", "Node/node1"
	InvolvedNamespace string
	Source            string
}

// ServiceData represents a Kubernetes service
type ServiceData struct {
	Name              string
	Namespace         string
	Type              string // ClusterIP, NodePort, LoadBalancer, ExternalName
	ClusterIP         string
	ExternalIPs       []string
	Ports             []ServicePort
	Selector          map[string]string
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time

	// LoadBalancer info
	LoadBalancerIP string
	Ingress        []string

	// Endpoint info
	EndpointCount int // Number of ready endpoints
}

// ServicePort represents a service port
type ServicePort struct {
	Name       string
	Protocol   string // TCP, UDP, SCTP
	Port       int32
	TargetPort string
	NodePort   int32
}

// PVData represents a PersistentVolume
type PVData struct {
	Name              string
	Capacity          int64 // bytes
	StorageClass      string
	AccessModes       []string
	ReclaimPolicy     string
	Status            string // Available, Bound, Released, Failed
	Claim             string // namespace/pvc-name
	VolumeMode        string
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time

	// Volume source type
	VolumeType string // NFS, iSCSI, HostPath, etc.
}

// PVCData represents a PersistentVolumeClaim
type PVCData struct {
	Name              string
	Namespace         string
	Status            string // Pending, Bound, Lost
	Volume            string // PV name
	Capacity          int64  // bytes
	RequestedStorage  int64  // bytes
	StorageClass      string
	AccessModes       []string
	VolumeMode        string
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time

	// Usage info (if available from metrics)
	UsedBytes int64
}

// DeploymentData represents a Kubernetes Deployment
type DeploymentData struct {
	Name              string
	Namespace         string
	Replicas          int32 // Desired replicas
	ReadyReplicas     int32
	AvailableReplicas int32
	UpdatedReplicas   int32
	Strategy          string // RollingUpdate, Recreate
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time

	// Selector
	Selector map[string]string

	// Conditions
	Conditions []string
}

// StatefulSetData represents a Kubernetes StatefulSet
type StatefulSetData struct {
	Name              string
	Namespace         string
	Replicas          int32 // Desired replicas
	ReadyReplicas     int32
	CurrentReplicas   int32
	UpdatedReplicas   int32
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time

	// Selector
	Selector map[string]string
}

// DaemonSetData represents a Kubernetes DaemonSet
type DaemonSetData struct {
	Name                   string
	Namespace              string
	DesiredNumberScheduled int32
	CurrentNumberScheduled int32
	NumberReady            int32
	NumberAvailable        int32
	Labels                 map[string]string
	Annotations            map[string]string
	CreationTimestamp      time.Time

	// Selector
	Selector map[string]string
}

// JobData represents a Kubernetes Job
type JobData struct {
	Name              string
	Namespace         string
	Completions       int32 // Desired completions
	Succeeded         int32
	Failed            int32
	Active            int32
	StartTime         time.Time
	CompletionTime    time.Time
	Duration          time.Duration
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time
}

// CronJobData represents a Kubernetes CronJob
type CronJobData struct {
	Name              string
	Namespace         string
	Schedule          string
	Suspend           bool
	Active            int32 // Number of active jobs
	LastScheduleTime  time.Time
	Labels            map[string]string
	Annotations       map[string]string
	CreationTimestamp time.Time
}

// ============================================================================
// Volcano Scheduler Data Models
// ============================================================================

// VolcanoJobData represents a Volcano Job (vcjob)
type VolcanoJobData struct {
	Name              string
	Namespace         string
	Status            string // Running, Completed, Pending, Failed, Aborted, Terminating
	Queue             string
	MinAvailable      int32
	Replicas          int32 // Total task replicas
	Running           int32
	Succeeded         int32
	Failed            int32
	Pending           int32
	NPURequested      int64  // Total NPU requested by this job
	NPUResourceName   string // e.g., "huawei.com/ascend-1980"
	CreationTimestamp time.Time
	StartTime         time.Time
	CompletionTime    time.Time
	Duration          time.Duration
	Labels            map[string]string
	Annotations       map[string]string

	// HyperJob info (if created by HyperJob)
	HyperJobName  string // volcano.sh/hyperjob-name
	HyperJobIndex string // volcano.sh/hyperjob-replicatedjob-index

	// Tasks info
	Tasks []VolcanoTaskData
}

// VolcanoTaskData represents a task in a Volcano Job
type VolcanoTaskData struct {
	Name         string
	Replicas     int32
	MinAvailable int32
	NPURequest   int64 // NPU per replica
}

// HyperNodeData represents a Volcano HyperNode (network topology)
type HyperNodeData struct {
	Name      string
	Tier      int    // 1 = SuperPod level, 2 = HyperCluster level
	NodeCount int    // Number of physical nodes
	Members   []HyperNodeMember
	Labels    map[string]string

	// Aggregated NPU info
	TotalNPU     int64
	AllocatedNPU int64
}

// HyperNodeMember represents a member of a HyperNode
type HyperNodeMember struct {
	Type string // "Node" or "HyperNode"
	Name string
}

// QueueData represents a Volcano Queue
type QueueData struct {
	Name              string
	Parent            string
	State             string // Open, Closed, Unknown
	Weight            int32
	Reclaimable       bool
	CreationTimestamp time.Time

	// Deserved resources (quotas)
	CPUDeserved    int64  // millicores
	MemoryDeserved int64  // bytes
	NPUDeserved    int64  // NPU count
	PodDeserved    int64  // Pod count limit

	// Allocated resources (current usage from status)
	CPUAllocated    int64 // millicores
	MemoryAllocated int64 // bytes
	NPUAllocated    int64 // NPU count
	PodAllocated    int64 // Pod count

	// Guaranteed resources (minimum guarantee)
	CPUGuarantee    int64
	MemoryGuarantee int64
	NPUGuarantee    int64

	// NPU resource name (e.g., "huawei.com/ascend-1980")
	NPUResourceName string

	// Job statistics (calculated from Volcano jobs)
	RunningJobs   int32
	PendingJobs   int32
	CompletedJobs int32
	FailedJobs    int32
	TotalJobs     int32
}

// VolcanoSummary provides Volcano-specific metrics
type VolcanoSummary struct {
	// Job statistics
	TotalJobs     int
	RunningJobs   int
	CompletedJobs int
	PendingJobs   int
	FailedJobs    int

	// Queue statistics
	TotalQueues int

	// HyperNode topology
	TotalHyperNodes int
	Tier1Nodes      int // SuperPod level
	Tier2Nodes      int // HyperCluster level

	// NPU usage by Volcano jobs
	NPURequestedByJobs int64
	NPURunningByJobs   int64
}
