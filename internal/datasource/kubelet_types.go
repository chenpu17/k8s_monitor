package datasource

import "time"

// KubeletSummary represents the response from /stats/summary API
type KubeletSummary struct {
	Node Node  `json:"node"`
	Pods []Pod `json:"pods"`
}

// Node represents node-level metrics from kubelet
type Node struct {
	NodeName  string        `json:"nodeName"`
	StartTime time.Time     `json:"startTime"`
	CPU       *CPUStats     `json:"cpu,omitempty"`
	Memory    *MemoryStats  `json:"memory,omitempty"`
	Network   *NetworkStats `json:"network,omitempty"`
	Fs        *FsStats      `json:"fs,omitempty"`
	Runtime   *RuntimeStats `json:"runtime,omitempty"`
	Rlimit    *RlimitStats  `json:"rlimit,omitempty"`
}

// Pod represents pod-level metrics from kubelet
type Pod struct {
	PodRef           PodReference  `json:"podRef"`
	StartTime        time.Time     `json:"startTime"`
	CPU              *CPUStats     `json:"cpu,omitempty"`
	Memory           *MemoryStats  `json:"memory,omitempty"`
	Network          *NetworkStats `json:"network,omitempty"`
	Volume           []VolumeStats `json:"volume,omitempty"`
	EphemeralStorage *FsStats      `json:"ephemeral-storage,omitempty"`
	Containers       []Container   `json:"containers"`
}

// PodReference identifies a pod
type PodReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
}

// Container represents container-level metrics
type Container struct {
	Name      string       `json:"name"`
	StartTime time.Time    `json:"startTime"`
	CPU       *CPUStats    `json:"cpu,omitempty"`
	Memory    *MemoryStats `json:"memory,omitempty"`
	Rootfs    *FsStats     `json:"rootfs,omitempty"`
	Logs      *FsStats     `json:"logs,omitempty"`
}

// CPUStats represents CPU usage statistics
type CPUStats struct {
	Time                 time.Time `json:"time"`
	UsageNanoCores       *uint64   `json:"usageNanoCores,omitempty"`       // Current rate of CPU usage in nanocores
	UsageCoreNanoSeconds *uint64   `json:"usageCoreNanoSeconds,omitempty"` // Cumulative CPU usage in nanoseconds
}

// MemoryStats represents memory usage statistics
type MemoryStats struct {
	Time            time.Time `json:"time"`
	AvailableBytes  *uint64   `json:"availableBytes,omitempty"`
	UsageBytes      *uint64   `json:"usageBytes,omitempty"`
	WorkingSetBytes *uint64   `json:"workingSetBytes,omitempty"`
	RSSBytes        *uint64   `json:"rssBytes,omitempty"`
	PageFaults      *uint64   `json:"pageFaults,omitempty"`
	MajorPageFaults *uint64   `json:"majorPageFaults,omitempty"`
}

// NetworkStats represents network I/O statistics
type NetworkStats struct {
	Time       time.Time        `json:"time"`
	RxBytes    *uint64          `json:"rxBytes,omitempty"`
	TxBytes    *uint64          `json:"txBytes,omitempty"`
	Interfaces []InterfaceStats `json:"interfaces,omitempty"`
}

// InterfaceStats represents network interface statistics
type InterfaceStats struct {
	Name     string  `json:"name"`
	RxBytes  *uint64 `json:"rxBytes,omitempty"`
	RxErrors *uint64 `json:"rxErrors,omitempty"`
	TxBytes  *uint64 `json:"txBytes,omitempty"`
	TxErrors *uint64 `json:"txErrors,omitempty"`
}

// FsStats represents filesystem usage statistics
type FsStats struct {
	Time           time.Time `json:"time"`
	AvailableBytes *uint64   `json:"availableBytes,omitempty"`
	CapacityBytes  *uint64   `json:"capacityBytes,omitempty"`
	UsedBytes      *uint64   `json:"usedBytes,omitempty"`
	InodesFree     *uint64   `json:"inodesFree,omitempty"`
	Inodes         *uint64   `json:"inodes,omitempty"`
	InodesUsed     *uint64   `json:"inodesUsed,omitempty"`
}

// RuntimeStats represents container runtime statistics
type RuntimeStats struct {
	ImageFs *FsStats `json:"imageFs,omitempty"`
}

// RlimitStats represents resource limit statistics
type RlimitStats struct {
	Time                  time.Time `json:"time"`
	MaxPID                *uint64   `json:"maxpid,omitempty"`
	NumOfRunningProcesses *uint64   `json:"curproc,omitempty"`
}

// VolumeStats represents volume usage statistics
type VolumeStats struct {
	Name    string  `json:"name"`
	FsStats FsStats `json:"fsStats"`
	PVCRef  *PVCRef `json:"pvcRef,omitempty"`
}

// PVCRef represents a PersistentVolumeClaim reference
type PVCRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
