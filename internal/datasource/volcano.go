package datasource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Volcano API Group/Version/Resources
var (
	volcanoJobGVR = schema.GroupVersionResource{
		Group:    "batch.volcano.sh",
		Version:  "v1alpha1",
		Resource: "jobs",
	}

	hyperNodeGVR = schema.GroupVersionResource{
		Group:    "topology.volcano.sh",
		Version:  "v1alpha1",
		Resource: "hypernodes",
	}

	queueGVR = schema.GroupVersionResource{
		Group:    "scheduling.volcano.sh",
		Version:  "v1beta1",
		Resource: "queues",
	}
)

// VolcanoClient provides access to Volcano CRD resources
type VolcanoClient struct {
	dynamicClient dynamic.Interface
	logger        *zap.Logger
	available     bool
}

// NewVolcanoClient creates a new Volcano client
func NewVolcanoClient(config *rest.Config, logger *zap.Logger) (*VolcanoClient, error) {
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	client := &VolcanoClient{
		dynamicClient: dynamicClient,
		logger:        logger,
		available:     true,
	}

	// Check if Volcano CRDs are available
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = dynamicClient.Resource(volcanoJobGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		logger.Warn("Volcano CRDs not available, Volcano features disabled",
			zap.Error(err),
		)
		client.available = false
	} else {
		logger.Info("Volcano CRDs detected, Volcano features enabled")
	}

	return client, nil
}

// IsAvailable returns true if Volcano CRDs are available
func (c *VolcanoClient) IsAvailable() bool {
	return c.available
}

// GetVolcanoJobs retrieves all Volcano jobs
func (c *VolcanoClient) GetVolcanoJobs(ctx context.Context, namespace string) ([]*model.VolcanoJobData, error) {
	if !c.available {
		return nil, nil
	}

	var list *unstructured.UnstructuredList
	var err error

	if namespace == "" {
		list, err = c.dynamicClient.Resource(volcanoJobGVR).Namespace("").List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.dynamicClient.Resource(volcanoJobGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list Volcano jobs: %w", err)
	}

	jobs := make([]*model.VolcanoJobData, 0, len(list.Items))
	for _, item := range list.Items {
		job := c.convertVolcanoJob(&item)
		if job != nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// convertVolcanoJob converts an unstructured Volcano job to VolcanoJobData
func (c *VolcanoClient) convertVolcanoJob(obj *unstructured.Unstructured) *model.VolcanoJobData {
	job := &model.VolcanoJobData{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Labels:    obj.GetLabels(),
	}

	// Get annotations
	annotations := obj.GetAnnotations()
	job.Annotations = annotations
	if annotations != nil {
		job.HyperJobName = annotations["volcano.sh/hyperjob-name"]
		job.HyperJobIndex = annotations["volcano.sh/hyperjob-replicatedjob-index"]
	}

	// Get creation timestamp
	job.CreationTimestamp = obj.GetCreationTimestamp().Time

	// Get spec fields
	spec, found, _ := unstructured.NestedMap(obj.Object, "spec")
	if found {
		if queue, ok, _ := unstructured.NestedString(spec, "queue"); ok {
			job.Queue = queue
		}
		if minAvailable, ok, _ := unstructured.NestedInt64(spec, "minAvailable"); ok {
			job.MinAvailable = int32(minAvailable)
		}

		// Get tasks and calculate total replicas and NPU
		tasks, found, _ := unstructured.NestedSlice(spec, "tasks")
		if found {
			for _, t := range tasks {
				taskMap, ok := t.(map[string]interface{})
				if !ok {
					continue
				}

				taskData := model.VolcanoTaskData{}
				if name, ok, _ := unstructured.NestedString(taskMap, "name"); ok {
					taskData.Name = name
				}
				if replicas, ok, _ := unstructured.NestedInt64(taskMap, "replicas"); ok {
					taskData.Replicas = int32(replicas)
					job.Replicas += int32(replicas)
				}

				// Get NPU request from task template
				containers, found, _ := unstructured.NestedSlice(taskMap, "template", "spec", "containers")
				if found {
					for _, container := range containers {
						containerMap, ok := container.(map[string]interface{})
						if !ok {
							continue
						}
						requests, found, _ := unstructured.NestedMap(containerMap, "resources", "requests")
						if found {
							for resName, qty := range requests {
								if strings.Contains(resName, "huawei.com/") && strings.Contains(strings.ToLower(resName), "ascend") {
									if qtyStr, ok := qty.(string); ok {
										// Parse quantity (e.g., "16")
										var npuVal int64
										fmt.Sscanf(qtyStr, "%d", &npuVal)
										taskData.NPURequest = npuVal
										job.NPURequested += npuVal * int64(taskData.Replicas)
										if job.NPUResourceName == "" {
											job.NPUResourceName = resName
										}
									} else if qtyInt, ok := qty.(int64); ok {
										taskData.NPURequest = qtyInt
										job.NPURequested += qtyInt * int64(taskData.Replicas)
										if job.NPUResourceName == "" {
											job.NPUResourceName = resName
										}
									}
								}
							}
						}
					}
				}

				job.Tasks = append(job.Tasks, taskData)
			}
		}
	}

	// Get status fields
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if found {
		if state, ok, _ := unstructured.NestedString(status, "state", "phase"); ok {
			job.Status = state
		}
		if running, ok, _ := unstructured.NestedInt64(status, "running"); ok {
			job.Running = int32(running)
		}
		if succeeded, ok, _ := unstructured.NestedInt64(status, "succeeded"); ok {
			job.Succeeded = int32(succeeded)
		}
		if failed, ok, _ := unstructured.NestedInt64(status, "failed"); ok {
			job.Failed = int32(failed)
		}
		if pending, ok, _ := unstructured.NestedInt64(status, "pending"); ok {
			job.Pending = int32(pending)
		}

		// Parse timestamps
		if startTimeStr, ok, _ := unstructured.NestedString(status, "startTime"); ok {
			if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
				job.StartTime = t
			}
		}
		if completionTimeStr, ok, _ := unstructured.NestedString(status, "completionTime"); ok {
			if t, err := time.Parse(time.RFC3339, completionTimeStr); err == nil {
				job.CompletionTime = t
			}
		}

		// Calculate duration
		if !job.StartTime.IsZero() {
			if !job.CompletionTime.IsZero() {
				job.Duration = job.CompletionTime.Sub(job.StartTime)
			} else {
				job.Duration = time.Since(job.StartTime)
			}
		}
	}

	return job
}

// GetHyperNodes retrieves all HyperNodes
func (c *VolcanoClient) GetHyperNodes(ctx context.Context) ([]*model.HyperNodeData, error) {
	if !c.available {
		return nil, nil
	}

	list, err := c.dynamicClient.Resource(hyperNodeGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		// HyperNode might not be available in all Volcano installations
		c.logger.Debug("Failed to list HyperNodes", zap.Error(err))
		return nil, nil
	}

	hyperNodes := make([]*model.HyperNodeData, 0, len(list.Items))
	for _, item := range list.Items {
		hn := c.convertHyperNode(&item)
		if hn != nil {
			hyperNodes = append(hyperNodes, hn)
		}
	}

	return hyperNodes, nil
}

// convertHyperNode converts an unstructured HyperNode to HyperNodeData
func (c *VolcanoClient) convertHyperNode(obj *unstructured.Unstructured) *model.HyperNodeData {
	hn := &model.HyperNodeData{
		Name:   obj.GetName(),
		Labels: obj.GetLabels(),
	}

	// Get spec fields
	spec, found, _ := unstructured.NestedMap(obj.Object, "spec")
	if found {
		if tier, ok, _ := unstructured.NestedInt64(spec, "tier"); ok {
			hn.Tier = int(tier)
		}

		// Get members
		members, found, _ := unstructured.NestedSlice(spec, "members")
		if found {
			for _, m := range members {
				memberMap, ok := m.(map[string]interface{})
				if !ok {
					continue
				}

				member := model.HyperNodeMember{}
				if memberType, ok, _ := unstructured.NestedString(memberMap, "type"); ok {
					member.Type = memberType
				}
				if selector, found, _ := unstructured.NestedMap(memberMap, "selector", "exactMatch"); found {
					if name, ok := selector["name"].(string); ok {
						member.Name = name
					}
				}
				hn.Members = append(hn.Members, member)
			}
		}
	}

	// Get status fields
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if found {
		if nodeCount, ok, _ := unstructured.NestedInt64(status, "nodeCount"); ok {
			hn.NodeCount = int(nodeCount)
		}
	}

	return hn
}

// GetQueues retrieves all Volcano queues
func (c *VolcanoClient) GetQueues(ctx context.Context) ([]*model.QueueData, error) {
	if !c.available {
		return nil, nil
	}

	list, err := c.dynamicClient.Resource(queueGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.logger.Debug("Failed to list Volcano queues", zap.Error(err))
		return nil, nil
	}

	queues := make([]*model.QueueData, 0, len(list.Items))
	for _, item := range list.Items {
		q := c.convertQueue(&item)
		if q != nil {
			queues = append(queues, q)
		}
	}

	return queues, nil
}

// convertQueue converts an unstructured Queue to QueueData
func (c *VolcanoClient) convertQueue(obj *unstructured.Unstructured) *model.QueueData {
	q := &model.QueueData{
		Name:              obj.GetName(),
		CreationTimestamp: obj.GetCreationTimestamp().Time,
	}

	// Get spec fields
	spec, found, _ := unstructured.NestedMap(obj.Object, "spec")
	if found {
		if parent, ok, _ := unstructured.NestedString(spec, "parent"); ok {
			q.Parent = parent
		}
		if weight, ok, _ := unstructured.NestedInt64(spec, "weight"); ok {
			q.Weight = int32(weight)
		}
		if reclaimable, ok, _ := unstructured.NestedBool(spec, "reclaimable"); ok {
			q.Reclaimable = reclaimable
		}

		// Parse deserved resources (quotas)
		if deserved, ok, _ := unstructured.NestedMap(spec, "deserved"); ok {
			q.CPUDeserved, q.MemoryDeserved, q.NPUDeserved, q.PodDeserved, q.NPUResourceName = c.parseQueueResources(deserved)
		}

		// Parse guarantee resources
		if guarantee, ok, _ := unstructured.NestedMap(spec, "guarantee"); ok {
			q.CPUGuarantee, q.MemoryGuarantee, q.NPUGuarantee, _, _ = c.parseQueueResources(guarantee)
		}
	}

	// Get status fields
	status, found, _ := unstructured.NestedMap(obj.Object, "status")
	if found {
		if state, ok, _ := unstructured.NestedString(status, "state"); ok {
			q.State = state
		}
		if running, ok, _ := unstructured.NestedInt64(status, "running"); ok {
			q.RunningJobs = int32(running)
		}
		if pending, ok, _ := unstructured.NestedInt64(status, "pending"); ok {
			q.PendingJobs = int32(pending)
		}

		// Parse allocated resources
		if allocated, ok, _ := unstructured.NestedMap(status, "allocated"); ok {
			q.CPUAllocated, q.MemoryAllocated, q.NPUAllocated, q.PodAllocated, _ = c.parseQueueResources(allocated)
		}
	}

	return q
}

// parseQueueResources parses resource quantities from a map
// Returns: cpu (millicores), memory (bytes), npu (count), pods (count), npuResourceName
func (c *VolcanoClient) parseQueueResources(resources map[string]interface{}) (int64, int64, int64, int64, string) {
	var cpu, memory, npu, pods int64
	var npuResourceName string

	for key, val := range resources {
		valStr, ok := val.(string)
		if !ok {
			continue
		}

		switch key {
		case "cpu":
			cpu = parseResourceQuantity(valStr)
		case "memory":
			memory = parseMemoryQuantity(valStr)
		case "pods":
			// Pods are count-type resources, not CPU-like
			pods = parseCountQuantity(valStr)
		default:
			// Check for NPU resources (huawei.com/ascend-*)
			if strings.Contains(key, "ascend") || strings.Contains(key, "npu") {
				// NPU is a count-type resource, not CPU-like
				npu = parseCountQuantity(valStr)
				npuResourceName = key
			}
		}
	}

	return cpu, memory, npu, pods, npuResourceName
}

// parseResourceQuantity parses a Kubernetes CPU resource quantity string
// Returns value in millicores
func parseResourceQuantity(s string) int64 {
	if s == "" {
		return 0
	}
	// Handle millicores (e.g., "1000m")
	if strings.HasSuffix(s, "m") {
		var val int64
		fmt.Sscanf(s, "%dm", &val)
		return val
	}
	// Handle plain number - assume it's in cores, convert to millicores
	var val int64
	fmt.Sscanf(s, "%d", &val)
	return val * 1000
}

// parseCountQuantity parses a count-type resource (e.g., NPU, pods)
// Returns the integer value without any conversion
func parseCountQuantity(s string) int64 {
	if s == "" {
		return 0
	}
	// Use resource.ParseQuantity for proper handling of various formats
	q, err := resource.ParseQuantity(s)
	if err != nil {
		// Fallback to simple integer parsing
		var val int64
		fmt.Sscanf(s, "%d", &val)
		return val
	}
	// For count-type resources, get the integer value
	return q.Value()
}

// parseMemoryQuantity parses a memory quantity string to bytes
func parseMemoryQuantity(s string) int64 {
	if s == "" {
		return 0
	}
	var val int64
	if strings.HasSuffix(s, "Ki") {
		fmt.Sscanf(s, "%dKi", &val)
		return val * 1024
	}
	if strings.HasSuffix(s, "Mi") {
		fmt.Sscanf(s, "%dMi", &val)
		return val * 1024 * 1024
	}
	if strings.HasSuffix(s, "Gi") {
		fmt.Sscanf(s, "%dGi", &val)
		return val * 1024 * 1024 * 1024
	}
	if strings.HasSuffix(s, "Ti") {
		fmt.Sscanf(s, "%dTi", &val)
		return val * 1024 * 1024 * 1024 * 1024
	}
	// Plain bytes
	fmt.Sscanf(s, "%d", &val)
	return val
}

// BuildVolcanoSummary builds summary statistics for Volcano resources
func (c *VolcanoClient) BuildVolcanoSummary(jobs []*model.VolcanoJobData, hyperNodes []*model.HyperNodeData, queues []*model.QueueData) *model.VolcanoSummary {
	summary := &model.VolcanoSummary{
		TotalQueues: len(queues),
	}

	// Job statistics
	for _, job := range jobs {
		summary.TotalJobs++
		summary.NPURequestedByJobs += job.NPURequested

		switch job.Status {
		case "Running":
			summary.RunningJobs++
			// Calculate running NPU (NPU per replica * running replicas)
			if job.Replicas > 0 && job.Running > 0 {
				npuPerReplica := job.NPURequested / int64(job.Replicas)
				summary.NPURunningByJobs += npuPerReplica * int64(job.Running)
			}
		case "Completed":
			summary.CompletedJobs++
		case "Pending":
			summary.PendingJobs++
		case "Failed", "Aborted":
			summary.FailedJobs++
		}
	}

	// HyperNode statistics
	for _, hn := range hyperNodes {
		summary.TotalHyperNodes++
		switch hn.Tier {
		case 1:
			summary.Tier1Nodes++
		case 2:
			summary.Tier2Nodes++
		}
	}

	return summary
}

// Close cleans up resources
func (c *VolcanoClient) Close() error {
	return nil
}
