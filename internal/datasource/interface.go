package datasource

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// DataSource defines the interface for Kubernetes data providers
type DataSource interface {
	// GetNodes retrieves all nodes in the cluster
	GetNodes(ctx context.Context) ([]*model.NodeData, error)

	// GetPods retrieves all pods, optionally filtered by namespace
	GetPods(ctx context.Context, namespace string) ([]*model.PodData, error)

	// GetEvents retrieves recent events, optionally filtered by type
	GetEvents(ctx context.Context, namespace string, eventTypes []string, limit int) ([]*model.EventData, error)

	// Name returns the data source name (for logging/debugging)
	Name() string

	// Close cleans up resources
	Close() error
}

// MetricsSource defines the interface for pod/node metrics providers
type MetricsSource interface {
	// GetNodeMetrics retrieves CPU/Memory/Network metrics for a node
	GetNodeMetrics(ctx context.Context, nodeName string) (cpuMillicores int64, memoryBytes int64, networkRxBytes int64, networkTxBytes int64, err error)

	// GetPodMetrics retrieves CPU/Memory metrics for a pod
	GetPodMetrics(ctx context.Context, namespace, podName string) (cpuMillicores int64, memoryBytes int64, err error)

	// Name returns the metrics source name
	Name() string

	// Close cleans up resources
	Close() error
}

// Helper functions to convert Kubernetes API objects to internal models

// ConvertNode converts a Kubernetes Node to NodeData
func ConvertNode(node *corev1.Node) *model.NodeData {
	nodeData := &model.NodeData{
		Name:              node.Name,
		Roles:             extractNodeRoles(node),
		Status:            extractNodeStatus(node),
		Conditions:        node.Status.Conditions,
		Taints:            node.Spec.Taints,
		Labels:            node.Labels,
		Annotations:       node.Annotations,
		CreationTimestamp: node.CreationTimestamp.Time,
	}

	// Extract IPs
	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			nodeData.InternalIP = addr.Address
		case corev1.NodeExternalIP:
			nodeData.ExternalIP = addr.Address
		}
	}

	// Extract capacity
	nodeData.CPUCapacity = node.Status.Capacity.Cpu().MilliValue()
	nodeData.MemoryCapacity = node.Status.Capacity.Memory().Value()
	nodeData.PodCapacity = node.Status.Capacity.Pods().Value()

	// Extract allocatable
	nodeData.CPUAllocatable = node.Status.Allocatable.Cpu().MilliValue()
	nodeData.MemAllocatable = node.Status.Allocatable.Memory().Value()
	nodeData.PodAllocatable = node.Status.Allocatable.Pods().Value()

	// Extract pressure indicators
	for _, cond := range node.Status.Conditions {
		switch cond.Type {
		case corev1.NodeMemoryPressure:
			nodeData.MemoryPressure = cond.Status == corev1.ConditionTrue
		case corev1.NodeDiskPressure:
			nodeData.DiskPressure = cond.Status == corev1.ConditionTrue
		case corev1.NodePIDPressure:
			nodeData.PIDPressure = cond.Status == corev1.ConditionTrue
		}
	}

	// Extract NPU (Ascend AI accelerator) information from capacity/allocatable
	// Look for resources like "huawei.com/ascend-1980", "huawei.com/Ascend910", etc.
	for resourceName, quantity := range node.Status.Capacity {
		resName := string(resourceName)
		if strings.Contains(resName, "huawei.com/") && strings.Contains(strings.ToLower(resName), "ascend") {
			nodeData.NPUCapacity = quantity.Value()
			nodeData.NPUResourceName = resName
			break
		}
	}
	for resourceName, quantity := range node.Status.Allocatable {
		resName := string(resourceName)
		if strings.Contains(resName, "huawei.com/") && strings.Contains(strings.ToLower(resName), "ascend") {
			nodeData.NPUAllocatable = quantity.Value()
			break
		}
	}

	// Extract NPU device info from node labels
	if node.Labels != nil {
		// NPU chip type: node.kubernetes.io/npu.chip.name
		if chipType, ok := node.Labels["node.kubernetes.io/npu.chip.name"]; ok {
			nodeData.NPUChipType = chipType
		}
		// NPU device type: accelerator/huawei-npu
		if deviceType, ok := node.Labels["accelerator/huawei-npu"]; ok {
			nodeData.NPUDeviceType = deviceType
		}
		// NPU driver version: os.modelarts.node/npu.firmware.driver.version
		if driverVersion, ok := node.Labels["os.modelarts.node/npu.firmware.driver.version"]; ok {
			nodeData.NPUDriverVersion = driverVersion
		}
		// AI Core count from label
		if aiCoreCount, ok := node.Labels["npu.huawei.com/aicore-count"]; ok {
			if count, err := strconv.Atoi(aiCoreCount); err == nil {
				nodeData.NPUAICoreCount = count
			}
		}

		// Topology information (Volcano HyperNode)
		if hyperNodeID, ok := node.Labels["volcano.sh/hypernode"]; ok {
			nodeData.HyperNodeID = hyperNodeID
		}
		if hyperClusterID, ok := node.Labels["volcano.sh/hypercluster"]; ok {
			nodeData.HyperClusterID = hyperClusterID
		}
		// SuperPod ID: os.modelarts.node/superpod.id
		if superPodID, ok := node.Labels["os.modelarts.node/superpod.id"]; ok {
			nodeData.SuperPodID = superPodID
		}
		// Cabinet info: cce.kubectl.kubernetes.io/cabinet
		if cabinetInfo, ok := node.Labels["cce.kubectl.kubernetes.io/cabinet"]; ok {
			nodeData.CabinetInfo = cabinetInfo
		}
	}

	// Extract NPU runtime metrics from annotations (set by device-plugin or npu-exporter)
	if node.Annotations != nil {
		// NPU utilization (AI Core utilization percentage)
		if util, ok := node.Annotations["npu.huawei.com/utilization"]; ok {
			if val, err := strconv.ParseFloat(util, 64); err == nil {
				nodeData.NPUUtilization = val
			}
		}
		// HBM (High Bandwidth Memory) total
		if hbmTotal, ok := node.Annotations["npu.huawei.com/hbm-total"]; ok {
			nodeData.NPUMemoryTotal = parseMemoryValue(hbmTotal)
		}
		// HBM used
		if hbmUsed, ok := node.Annotations["npu.huawei.com/hbm-used"]; ok {
			nodeData.NPUMemoryUsed = parseMemoryValue(hbmUsed)
		}
		// HBM utilization percentage
		if hbmUtil, ok := node.Annotations["npu.huawei.com/hbm-utilization"]; ok {
			if val, err := strconv.ParseFloat(hbmUtil, 64); err == nil {
				nodeData.NPUMemoryUtil = val
			}
		} else if nodeData.NPUMemoryTotal > 0 {
			// Calculate from total and used if utilization not directly provided
			nodeData.NPUMemoryUtil = float64(nodeData.NPUMemoryUsed) / float64(nodeData.NPUMemoryTotal) * 100
		}
		// NPU temperature
		if temp, ok := node.Annotations["npu.huawei.com/temperature"]; ok {
			if val, err := strconv.Atoi(temp); err == nil {
				nodeData.NPUTemperature = val
			}
		}
		// NPU power consumption
		if power, ok := node.Annotations["npu.huawei.com/power"]; ok {
			if val, err := strconv.Atoi(power); err == nil {
				nodeData.NPUPower = val
			}
		}
		// NPU health status
		if health, ok := node.Annotations["npu.huawei.com/health"]; ok {
			nodeData.NPUHealthStatus = health
		}
		// NPU error count
		if errCount, ok := node.Annotations["npu.huawei.com/error-count"]; ok {
			if val, err := strconv.Atoi(errCount); err == nil {
				nodeData.NPUErrorCount = val
			}
		}
		// AI Core count from annotation (fallback)
		if nodeData.NPUAICoreCount == 0 {
			if aiCoreCount, ok := node.Annotations["npu.huawei.com/aicore-count"]; ok {
				if count, err := strconv.Atoi(aiCoreCount); err == nil {
					nodeData.NPUAICoreCount = count
				}
			}
		}

		// Parse k8s-monitor NPU collector metrics (from DaemonSet)
		if npuMetricsJSON, ok := node.Annotations["k8s-monitor.io/npu-metrics"]; ok {
			var npuMetrics struct {
				Timestamp string `json:"timestamp"`
				Chips     []struct {
					NPU     int     `json:"npu"`
					Chip    int     `json:"chip"`
					PhyID   int     `json:"phy_id"`
					BusID   string  `json:"bus_id"`
					Health  string  `json:"health"`
					Power   float64 `json:"power"`
					Temp    int     `json:"temp"`
					AICore  int     `json:"aicore"`
					HBMUsed int64   `json:"hbm_used"`
					HBMTotal int64  `json:"hbm_total"`
				} `json:"chips"`
			}
			if err := json.Unmarshal([]byte(npuMetricsJSON), &npuMetrics); err == nil {
				// Parse timestamp
				if ts, err := time.Parse(time.RFC3339, npuMetrics.Timestamp); err == nil {
					nodeData.NPUMetricsTime = ts
				}

				// Convert to NPUChipData and calculate aggregate metrics
				nodeData.NPUChips = make([]model.NPUChipData, len(npuMetrics.Chips))
				var totalAICore, totalTemp, totalPower float64
				var totalHBMUsed, totalHBMTotal int64
				healthyCount := 0

				for i, chip := range npuMetrics.Chips {
					nodeData.NPUChips[i] = model.NPUChipData{
						NPUID:    chip.NPU,
						Chip:     chip.Chip,
						PhyID:    chip.PhyID,
						BusID:    chip.BusID,
						Health:   chip.Health,
						Power:    chip.Power,
						Temp:     chip.Temp,
						AICore:   chip.AICore,
						HBMUsed:  chip.HBMUsed,
						HBMTotal: chip.HBMTotal,
					}
					totalAICore += float64(chip.AICore)
					totalTemp += float64(chip.Temp)
					totalPower += chip.Power
					totalHBMUsed += chip.HBMUsed
					totalHBMTotal += chip.HBMTotal
					if chip.Health == "OK" {
						healthyCount++
					}
				}

				// Calculate averages and totals for node-level metrics
				if len(npuMetrics.Chips) > 0 {
					nodeData.NPUUtilization = totalAICore / float64(len(npuMetrics.Chips))
					nodeData.NPUTemperature = int(totalTemp / float64(len(npuMetrics.Chips)))
					nodeData.NPUPower = int(totalPower)
					nodeData.NPUMemoryUsed = totalHBMUsed * 1024 * 1024 // Convert MB to bytes
					nodeData.NPUMemoryTotal = totalHBMTotal * 1024 * 1024 // Convert MB to bytes
					if totalHBMTotal > 0 {
						nodeData.NPUMemoryUtil = float64(totalHBMUsed) / float64(totalHBMTotal) * 100
					}
					// Set health status based on chip health
					if healthyCount == len(npuMetrics.Chips) {
						nodeData.NPUHealthStatus = "Healthy"
					} else if healthyCount > 0 {
						nodeData.NPUHealthStatus = "Warning"
					} else {
						nodeData.NPUHealthStatus = "Unhealthy"
					}
				}
			}
		}
	}

	// Set default health status if NPU exists but no health annotation
	if nodeData.NPUCapacity > 0 && nodeData.NPUHealthStatus == "" {
		nodeData.NPUHealthStatus = "Healthy"
	}

	return nodeData
}

// extractNodeRoles extracts node roles from labels
func extractNodeRoles(node *corev1.Node) []string {
	roles := []string{}
	for key := range node.Labels {
		// Check for role labels (node-role.kubernetes.io/<role>)
		if key == "node-role.kubernetes.io/master" || key == "node-role.kubernetes.io/control-plane" {
			roles = append(roles, "master")
		} else if key == "node-role.kubernetes.io/worker" {
			roles = append(roles, "worker")
		}
	}
	if len(roles) == 0 {
		roles = append(roles, "worker") // default role
	}
	return roles
}

// extractNodeStatus extracts node status from conditions
func extractNodeStatus(node *corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			if cond.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

// ConvertPod converts a Kubernetes Pod to PodData
func ConvertPod(pod *corev1.Pod) *model.PodData {
	podData := &model.PodData{
		Name:              pod.Name,
		Namespace:         pod.Namespace,
		Node:              pod.Spec.NodeName,
		Phase:             string(pod.Status.Phase),
		Reason:            pod.Status.Reason,
		Message:           pod.Status.Message,
		HostIP:            pod.Status.HostIP,
		PodIP:             pod.Status.PodIP,
		QOSClass:          string(pod.Status.QOSClass),
		Labels:            pod.Labels,
		Annotations:       pod.Annotations,
		CreationTimestamp: pod.CreationTimestamp.Time,
		Conditions:        pod.Status.Conditions,
	}

	if pod.Status.StartTime != nil {
		podData.StartTime = pod.Status.StartTime.Time
	}

	// Count containers
	podData.Containers = len(pod.Spec.Containers)

	// Extract container statuses and match with resource specs
	podData.ContainerStates = make([]model.ContainerState, 0, len(pod.Status.ContainerStatuses))

	// Create a map of container specs by name for quick lookup
	containerSpecs := make(map[string]corev1.Container)
	for _, container := range pod.Spec.Containers {
		containerSpecs[container.Name] = container
	}

	for _, cs := range pod.Status.ContainerStatuses {
		podData.RestartCount += cs.RestartCount
		if cs.Ready {
			podData.ReadyContainers++
		}

		state := extractContainerState(&cs)

		// Fill in resource requests/limits for this container
		if spec, found := containerSpecs[cs.Name]; found {
			if spec.Resources.Requests != nil {
				if cpu := spec.Resources.Requests.Cpu(); cpu != nil {
					state.CPURequest = cpu.MilliValue()
				}
				if mem := spec.Resources.Requests.Memory(); mem != nil {
					state.MemoryRequest = mem.Value()
				}
			}
			if spec.Resources.Limits != nil {
				if cpu := spec.Resources.Limits.Cpu(); cpu != nil {
					state.CPULimit = cpu.MilliValue()
				}
				if mem := spec.Resources.Limits.Memory(); mem != nil {
					state.MemoryLimit = mem.Value()
				}
			}
		}

		podData.ContainerStates = append(podData.ContainerStates, state)
	}

	// Extract resource requests/limits
	for _, container := range pod.Spec.Containers {
		if container.Resources.Requests != nil {
			if cpu := container.Resources.Requests.Cpu(); cpu != nil {
				podData.CPURequest += cpu.MilliValue()
			}
			if mem := container.Resources.Requests.Memory(); mem != nil {
				podData.MemoryRequest += mem.Value()
			}
			// Extract NPU requests (Ascend AI accelerators)
			for resourceName, quantity := range container.Resources.Requests {
				resName := string(resourceName)
				if strings.Contains(resName, "huawei.com/") && strings.Contains(strings.ToLower(resName), "ascend") {
					podData.NPURequest += quantity.Value()
					if podData.NPUResourceName == "" {
						podData.NPUResourceName = resName
					}
				}
			}
		}
		if container.Resources.Limits != nil {
			if cpu := container.Resources.Limits.Cpu(); cpu != nil {
				podData.CPULimit += cpu.MilliValue()
			}
			if mem := container.Resources.Limits.Memory(); mem != nil {
				podData.MemoryLimit += mem.Value()
			}
		}
	}

	return podData
}

// extractContainerState extracts container state from ContainerStatus
func extractContainerState(cs *corev1.ContainerStatus) model.ContainerState {
	state := model.ContainerState{
		Name:         cs.Name,
		Image:        cs.Image,
		Ready:        cs.Ready,
		RestartCount: cs.RestartCount,
	}

	if cs.State.Running != nil {
		state.State = "Running"
	} else if cs.State.Waiting != nil {
		state.State = "Waiting"
		state.Reason = cs.State.Waiting.Reason
		state.Message = cs.State.Waiting.Message
	} else if cs.State.Terminated != nil {
		state.State = "Terminated"
		state.Reason = cs.State.Terminated.Reason
		state.Message = cs.State.Terminated.Message
		state.ExitCode = cs.State.Terminated.ExitCode
	}

	return state
}

// ConvertEvent converts a Kubernetes Event to EventData
func ConvertEvent(event *corev1.Event) *model.EventData {
	return &model.EventData{
		Type:              event.Type,
		Reason:            event.Reason,
		Message:           event.Message,
		Count:             event.Count,
		FirstTimestamp:    event.FirstTimestamp.Time,
		LastTimestamp:     event.LastTimestamp.Time,
		InvolvedObject:    event.InvolvedObject.Kind + "/" + event.InvolvedObject.Name,
		InvolvedNamespace: event.InvolvedObject.Namespace,
		Source:            event.Source.Component,
	}
}

// parseMemoryValue parses memory value strings like "64Gi", "32G", "65536Mi" to bytes
func parseMemoryValue(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	// Try to parse as plain number first
	if v, err := strconv.ParseInt(value, 10, 64); err == nil {
		return v
	}

	// Handle suffixes
	multiplier := int64(1)
	numStr := value

	if strings.HasSuffix(value, "Gi") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(value, "Gi")
	} else if strings.HasSuffix(value, "G") {
		multiplier = 1000 * 1000 * 1000
		numStr = strings.TrimSuffix(value, "G")
	} else if strings.HasSuffix(value, "Mi") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(value, "Mi")
	} else if strings.HasSuffix(value, "M") {
		multiplier = 1000 * 1000
		numStr = strings.TrimSuffix(value, "M")
	} else if strings.HasSuffix(value, "Ki") {
		multiplier = 1024
		numStr = strings.TrimSuffix(value, "Ki")
	} else if strings.HasSuffix(value, "K") {
		multiplier = 1000
		numStr = strings.TrimSuffix(value, "K")
	}

	if v, err := strconv.ParseFloat(numStr, 64); err == nil {
		return int64(v * float64(multiplier))
	}

	return 0
}
