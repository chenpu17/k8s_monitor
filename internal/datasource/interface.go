package datasource

import (
	"context"

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
