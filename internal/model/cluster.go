package model

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ClusterData represents the overall cluster state
type ClusterData struct {
	Nodes       []*NodeData
	Pods        []*PodData
	Events      []*EventData
	Services    []*ServiceData
	PVs         []*PVData
	PVCs        []*PVCData
	Deployments []*DeploymentData
	StatefulSets []*StatefulSetData
	DaemonSets  []*DaemonSetData
	Jobs        []*JobData
	CronJobs    []*CronJobData
	Summary     *ClusterSummary
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

	LastRefreshTime time.Time

	// Alerts collected from cluster state
	Alerts []Alert
}

// Alert represents a resource alert
type Alert struct {
	Severity     AlertSeverity // Critical, Warning, Info
	Category     string        // Resource, Pod, Node, Storage, Network, Service
	ResourceType string        // Node, Pod, PVC, Service, etc.
	ResourceName string
	Namespace    string // Empty for cluster-scoped resources
	Message      string
	Value        string // e.g., "95.2%", "3 restarts"
	Threshold    string // e.g., "80%"
	Timestamp    time.Time
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
