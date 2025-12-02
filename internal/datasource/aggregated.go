package datasource

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/yourusername/k8s-monitor/internal/diagnostic"
	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

const kubeletAccessCheckTTL = time.Minute

// AggregatedDataSource combines API Server and kubelet data sources
// It automatically handles fallback and data enrichment
type AggregatedDataSource struct {
	apiServer          DataSource
	apiServerClient    *APIServerClient
	kubeletClient      *KubeletClient
	volcanoClient      *VolcanoClient
	npuExporterClient  *NPUExporterClient
	logger             *zap.Logger
	mu                 sync.RWMutex
	maxConcurrent      int // Maximum concurrent kubelet queries
	kubeletAccessMu    sync.RWMutex
	kubeletAccess      *diagnostic.KubeletAccessStatus
	kubeletSkipReason  string
}

// NewAggregatedDataSource creates a new aggregated data source
func NewAggregatedDataSource(apiServer DataSource, kubeletClient *KubeletClient, logger *zap.Logger, maxConcurrent int) *AggregatedDataSource {
	if maxConcurrent <= 0 {
		maxConcurrent = 10 // Default to 10 if invalid
	}

	apiServerClient, _ := apiServer.(*APIServerClient)

	return &AggregatedDataSource{
		apiServer:       apiServer,
		apiServerClient: apiServerClient,
		kubeletClient:   kubeletClient,
		logger:          logger,
		maxConcurrent:   maxConcurrent,
	}
}

// SetVolcanoClient sets the Volcano client for the data source
func (a *AggregatedDataSource) SetVolcanoClient(volcanoClient *VolcanoClient) {
	a.volcanoClient = volcanoClient
}

// SetNPUExporterClient sets the NPU-Exporter client for the data source
func (a *AggregatedDataSource) SetNPUExporterClient(npuExporterClient *NPUExporterClient) {
	a.npuExporterClient = npuExporterClient
}

// GetNodes retrieves nodes from API Server
func (a *AggregatedDataSource) GetNodes(ctx context.Context) ([]*model.NodeData, error) {
	return a.apiServer.GetNodes(ctx)
}

// GetPods retrieves pods from API Server
func (a *AggregatedDataSource) GetPods(ctx context.Context, namespace string) ([]*model.PodData, error) {
	return a.apiServer.GetPods(ctx, namespace)
}

// GetEvents retrieves events from API Server
func (a *AggregatedDataSource) GetEvents(ctx context.Context, namespace string, eventTypes []string, limit int) ([]*model.EventData, error) {
	return a.apiServer.GetEvents(ctx, namespace, eventTypes, limit)
}

// GetClusterData retrieves complete cluster data with metrics enrichment
func (a *AggregatedDataSource) GetClusterData(ctx context.Context, namespace string) (*model.ClusterData, error) {
	a.logger.Info("Fetching cluster data")

	// Fetch basic data from API Server
	nodes, err := a.apiServer.GetNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	pods, err := a.apiServer.GetPods(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	events, err := a.apiServer.GetEvents(ctx, namespace, []string{"Normal", "Warning"}, 100)
	if err != nil {
		a.logger.Warn("Failed to get events, continuing without them",
			zap.Error(err),
		)
		events = []*model.EventData{}
	}

	// Fetch Services, PVs, PVCs (only if APIServerClient)
	var services []*model.ServiceData
	var pvs []*model.PVData
	var pvcs []*model.PVCData
	var deployments []*model.DeploymentData
	var statefulsets []*model.StatefulSetData
	var daemonsets []*model.DaemonSetData
	var jobs []*model.JobData
	var cronjobs []*model.CronJobData

	if apiServerClient, ok := a.apiServer.(*APIServerClient); ok {
		services, err = apiServerClient.GetServices(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get services, continuing without them", zap.Error(err))
			services = []*model.ServiceData{}
		}

		pvs, err = apiServerClient.GetPersistentVolumes(ctx)
		if err != nil {
			a.logger.Warn("Failed to get persistent volumes, continuing without them", zap.Error(err))
			pvs = []*model.PVData{}
		}

		pvcs, err = apiServerClient.GetPersistentVolumeClaims(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get PVCs, continuing without them", zap.Error(err))
			pvcs = []*model.PVCData{}
		}

		deployments, err = apiServerClient.GetDeployments(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get deployments, continuing without them", zap.Error(err))
			deployments = []*model.DeploymentData{}
		}

		statefulsets, err = apiServerClient.GetStatefulSets(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get statefulsets, continuing without them", zap.Error(err))
			statefulsets = []*model.StatefulSetData{}
		}

		daemonsets, err = apiServerClient.GetDaemonSets(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get daemonsets, continuing without them", zap.Error(err))
			daemonsets = []*model.DaemonSetData{}
		}

		jobs, err = apiServerClient.GetJobs(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get jobs, continuing without them", zap.Error(err))
			jobs = []*model.JobData{}
		}

		cronjobs, err = apiServerClient.GetCronJobs(ctx, namespace)
		if err != nil {
			a.logger.Warn("Failed to get cronjobs, continuing without them", zap.Error(err))
			cronjobs = []*model.CronJobData{}
		}
	}

	// Enrich with kubelet metrics if available
	if a.kubeletClient != nil {
		if skip, reason := a.shouldSkipKubeletEnrichment(ctx); skip {
			a.logger.Warn("Skipping kubelet metrics enrichment",
				zap.String("reason", reason),
			)
			a.applyKubeletSkipReason(reason, nodes, pods)
		} else {
			a.clearKubeletSkipReason()
			a.enrichWithKubeletMetrics(ctx, nodes, pods)
		}
	}

	// Enrich with NPU-Exporter metrics if available
	if a.npuExporterClient != nil {
		if err := a.npuExporterClient.EnrichNodeData(ctx, nodes); err != nil {
			a.logger.Warn("Failed to enrich NPU metrics from NPU-Exporter", zap.Error(err))
		}
	}

	// Build cluster summary
	summary := a.buildClusterSummary(nodes, pods, events, services, pvs, pvcs)

	// Fetch Volcano data if client is available
	var volcanoJobs []*model.VolcanoJobData
	var hyperNodes []*model.HyperNodeData
	var queues []*model.QueueData
	var volcanoSummary *model.VolcanoSummary

	if a.volcanoClient != nil && a.volcanoClient.IsAvailable() {
		var vcErr error

		volcanoJobs, vcErr = a.volcanoClient.GetVolcanoJobs(ctx, namespace)
		if vcErr != nil {
			a.logger.Warn("Failed to get Volcano jobs", zap.Error(vcErr))
		}

		hyperNodes, vcErr = a.volcanoClient.GetHyperNodes(ctx)
		if vcErr != nil {
			a.logger.Warn("Failed to get HyperNodes", zap.Error(vcErr))
		}

		queues, vcErr = a.volcanoClient.GetQueues(ctx)
		if vcErr != nil {
			a.logger.Warn("Failed to get Volcano queues", zap.Error(vcErr))
		}

		volcanoSummary = a.volcanoClient.BuildVolcanoSummary(volcanoJobs, hyperNodes, queues)
	}

	clusterData := &model.ClusterData{
		Nodes:          nodes,
		Pods:           pods,
		Events:         events,
		Services:       services,
		PVs:            pvs,
		PVCs:           pvcs,
		Deployments:    deployments,
		StatefulSets:   statefulsets,
		DaemonSets:     daemonsets,
		Jobs:           jobs,
		CronJobs:       cronjobs,
		Summary:        summary,
		VolcanoJobs:    volcanoJobs,
		HyperNodes:     hyperNodes,
		Queues:         queues,
		VolcanoSummary: volcanoSummary,
	}

	a.logger.Info("Cluster data fetched successfully",
		zap.Int("nodes", len(nodes)),
		zap.Int("pods", len(pods)),
		zap.Int("events", len(events)),
		zap.Int("services", len(services)),
		zap.Int("pvs", len(pvs)),
		zap.Int("pvcs", len(pvcs)),
		zap.Int("deployments", len(deployments)),
		zap.Int("statefulsets", len(statefulsets)),
		zap.Int("daemonsets", len(daemonsets)),
		zap.Int("jobs", len(jobs)),
		zap.Int("cronjobs", len(cronjobs)),
		zap.Int("volcanoJobs", len(volcanoJobs)),
		zap.Int("hyperNodes", len(hyperNodes)),
	)

	return clusterData, nil
}

// enrichWithKubeletMetrics enriches node and pod data with kubelet metrics
func (a *AggregatedDataSource) enrichWithKubeletMetrics(ctx context.Context, nodes []*model.NodeData, pods []*model.PodData) {
	startTime := time.Now()
	a.logger.Debug("Enriching data with kubelet metrics", zap.Int("node_count", len(nodes)))

	// Create pod lookup map by node
	podsByNode := make(map[string][]*model.PodData)
	for _, pod := range pods {
		if pod.Node != "" {
			podsByNode[pod.Node] = append(podsByNode[pod.Node], pod)
		}
	}

	// Limit concurrent kubelet queries to avoid throttling
	// Use a semaphore pattern with buffered channel
	sem := make(chan struct{}, a.maxConcurrent)

	// Fetch metrics for each node
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *model.NodeData) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }() // Release semaphore

			nodeStartTime := time.Now()

			// Get node metrics (including network)
			cpuMillicores, memoryBytes, networkRx, networkTx, networkTimestamp, err := a.kubeletClient.GetNodeMetrics(ctx, n.Name)
			if err != nil {
				a.mu.Lock()
				n.HasKubeletMetrics = false
				n.KubeletError = err.Error()
				n.CPUUsage = 0
				n.MemoryUsage = 0
				n.NetworkRxBytes = 0
				n.NetworkTxBytes = 0
				n.NetworkTimestamp = time.Time{}
				a.mu.Unlock()

				a.logger.Debug("Failed to get node metrics",
					zap.String("node", n.Name),
					zap.Duration("elapsed", time.Since(nodeStartTime)),
					zap.Error(err),
				)
				return
			}

			// Update node data
			a.mu.Lock()
			n.CPUUsage = cpuMillicores
			n.MemoryUsage = memoryBytes
			n.NetworkRxBytes = networkRx
			n.NetworkTxBytes = networkTx
			n.NetworkTimestamp = networkTimestamp
			n.HasKubeletMetrics = true
			n.KubeletError = ""

			// Calculate usage percentages
			if n.CPUAllocatable > 0 {
				n.CPUUsagePercent = float64(cpuMillicores) / float64(n.CPUAllocatable) * 100
			}
			if n.MemAllocatable > 0 {
				n.MemoryUsagePercent = float64(memoryBytes) / float64(n.MemAllocatable) * 100
			}

			// Count pods on this node
			n.PodCount = len(podsByNode[n.Name])
			if n.PodAllocatable > 0 {
				n.PodUsagePercent = float64(n.PodCount) / float64(n.PodAllocatable) * 100
			}
			a.mu.Unlock()

			// Get pod metrics on this node
			podMetricsMap, err := a.kubeletClient.GetAllPodMetricsOnNode(ctx, n.Name)
			if err != nil {
				a.mu.Lock()
				if n.HasKubeletMetrics {
					n.KubeletError = err.Error()
				}
				a.mu.Unlock()
				a.logger.Debug("Failed to get pod metrics",
					zap.String("node", n.Name),
					zap.Duration("elapsed", time.Since(nodeStartTime)),
					zap.Error(err),
				)
				return
			}

			// Update pod data
			a.mu.Lock()
			for _, pod := range podsByNode[n.Name] {
				key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
				if metrics, ok := podMetricsMap[key]; ok {
					pod.CPUUsage = metrics.CPUUsage
					pod.MemoryUsage = metrics.MemoryUsage
					pod.NetworkRxBytes = metrics.NetworkRxBytes
					pod.NetworkTxBytes = metrics.NetworkTxBytes
					pod.NetworkTimestamp = metrics.NetworkTimestamp

					// Update container-level metrics by matching container names
					for i := range pod.ContainerStates {
						containerName := pod.ContainerStates[i].Name
						// Find matching container in metrics
						for _, metricContainer := range metrics.ContainerStates {
							if metricContainer.Name == containerName {
								pod.ContainerStates[i].CPUUsage = metricContainer.CPUUsage
								pod.ContainerStates[i].MemoryUsage = metricContainer.MemoryUsage
								break
							}
						}
					}
				}
			}
			a.mu.Unlock()

			a.logger.Debug("Node metrics fetched",
				zap.String("node", n.Name),
				zap.Duration("elapsed", time.Since(nodeStartTime)),
			)
		}(node)
	}

	wg.Wait()
	a.logger.Info("Kubelet metrics enrichment completed",
		zap.Duration("total_elapsed", time.Since(startTime)),
		zap.Int("nodes", len(nodes)),
	)
}

// buildClusterSummary builds cluster summary statistics
func (a *AggregatedDataSource) buildClusterSummary(nodes []*model.NodeData, pods []*model.PodData, events []*model.EventData, services []*model.ServiceData, pvs []*model.PVData, pvcs []*model.PVCData) *model.ClusterSummary {
	summary := &model.ClusterSummary{
		TotalNodes: len(nodes),
		TotalPods:  len(pods),
	}

	errorSet := make(map[string]struct{})

	// Build a map for NPU allocation per node (from running/pending pods)
	npuAllocatedPerNode := make(map[string]int64)
	for _, pod := range pods {
		if pod.Node != "" && (pod.Phase == "Running" || pod.Phase == "Pending") && pod.NPURequest > 0 {
			npuAllocatedPerNode[pod.Node] += pod.NPURequest
		}
	}

	// Track unique SuperPod IDs for counting
	superPodIDs := make(map[string]struct{})
	hyperNodeIDs := make(map[string]struct{})

	// Aggregate node resources
	for _, node := range nodes {
		// Count node states
		if node.Status == "Ready" {
			summary.ReadyNodes++
		} else {
			summary.NotReadyNodes++
		}

		// Count node pressure indicators
		if node.MemoryPressure {
			summary.MemoryPressureNodes++
		}
		if node.DiskPressure {
			summary.DiskPressureNodes++
		}
		if node.PIDPressure {
			summary.PIDPressureNodes++
		}

		// Sum up cluster-wide capacity
		summary.CPUCapacity += node.CPUCapacity
		summary.MemoryCapacity += node.MemoryCapacity
		summary.PodCapacity += node.PodCapacity

		// Sum up cluster-wide allocatable
		summary.CPUAllocatable += node.CPUAllocatable
		summary.MemoryAllocatable += node.MemAllocatable
		summary.PodAllocatable += node.PodAllocatable

		// Sum up cluster-wide usage (from metrics)
		summary.CPUUsed += node.CPUUsage
		summary.MemoryUsed += node.MemoryUsage

		// Sum up network metrics
		summary.NetworkRxBytes += node.NetworkRxBytes
		summary.NetworkTxBytes += node.NetworkTxBytes
		if node.HasKubeletMetrics {
			summary.NodesWithMetrics++
		} else {
			summary.NodesWithoutMetrics++
			if node.KubeletError != "" {
				if _, exists := errorSet[node.KubeletError]; !exists {
					summary.KubeletErrors = append(summary.KubeletErrors, node.KubeletError)
					errorSet[node.KubeletError] = struct{}{}
				}
			}
		}

		// Sum up NPU (Ascend AI accelerator) resources
		if node.NPUCapacity > 0 {
			summary.NPUCapacity += node.NPUCapacity
			summary.NPUAllocatable += node.NPUAllocatable
			summary.NPUNodesCount++

			// Set NPU resource name and chip type (from first node with NPU)
			if summary.NPUResourceName == "" && node.NPUResourceName != "" {
				summary.NPUResourceName = node.NPUResourceName
			}
			if summary.NPUChipType == "" && node.NPUChipType != "" {
				summary.NPUChipType = node.NPUChipType
			}

			// Update node's NPU allocated count from pod requests
			if allocated, ok := npuAllocatedPerNode[node.Name]; ok {
				node.NPUAllocated = allocated
				summary.NPUAllocated += allocated
			}

			// Track topology information
			if node.HyperClusterID != "" && summary.HyperClusterID == "" {
				summary.HyperClusterID = node.HyperClusterID
			}
			if node.HyperNodeID != "" {
				hyperNodeIDs[node.HyperNodeID] = struct{}{}
			}
			if node.SuperPodID != "" {
				superPodIDs[node.SuperPodID] = struct{}{}
			}
		}
	}
	summary.KubeletMetricsAvailable = summary.NodesWithMetrics > 0
	if len(summary.KubeletErrors) > 0 {
		summary.KubeletError = summary.KubeletErrors[0]
	}
	if !summary.KubeletMetricsAvailable && a.kubeletClient == nil && len(nodes) > 0 {
		msg := "kubelet metrics disabled (client not initialized)"
		summary.KubeletError = msg
		summary.KubeletErrors = []string{msg}
	} else if !summary.KubeletMetricsAvailable && summary.KubeletError == "" {
		if reason := a.getKubeletSkipReason(); reason != "" {
			summary.KubeletError = reason
			if len(summary.KubeletErrors) == 0 {
				summary.KubeletErrors = []string{reason}
			} else {
				summary.KubeletErrors = append([]string{reason}, summary.KubeletErrors...)
			}
		}
	}

	// Aggregate pod resources and count workloads
	deployments := make(map[string]bool)
	statefulSets := make(map[string]bool)
	daemonSets := make(map[string]bool)
	jobs := make(map[string]bool)
	cronJobs := make(map[string]bool)

	// Track pods with high restart counts for Top 5
	type podWithRestarts struct {
		pod        *model.PodData
		lastReason string
	}
	highRestartCandidates := make([]podWithRestarts, 0)

	for _, pod := range pods {
		// Count pod phases
		switch pod.Phase {
		case "Running":
			summary.RunningPods++
		case "Pending":
			summary.PendingPods++
		case "Failed":
			summary.FailedPods++
		default:
			summary.UnknownPods++
		}

		// Analyze container states for anomalies
		hasOOMKilled := false
		hasCrashLoop := false
		hasImagePullError := false
		hasContainerCreating := false
		var lastTerminatedReason string

		for _, container := range pod.ContainerStates {
			// Check for specific error states
			switch container.Reason {
			case "OOMKilled":
				hasOOMKilled = true
				lastTerminatedReason = "OOMKilled"
			case "CrashLoopBackOff":
				hasCrashLoop = true
				lastTerminatedReason = "CrashLoopBackOff"
			case "ImagePullBackOff", "ErrImagePull":
				hasImagePullError = true
				lastTerminatedReason = container.Reason
			case "ContainerCreating":
				hasContainerCreating = true
			case "Error":
				if lastTerminatedReason == "" {
					lastTerminatedReason = "Error"
				}
			}
		}

		// Count pod anomalies (avoid double-counting)
		if hasOOMKilled {
			summary.OOMKilledPods++
		} else if hasCrashLoop {
			summary.CrashLoopBackOffPods++
		} else if hasImagePullError {
			summary.ImagePullBackOffPods++
		}

		if hasContainerCreating && pod.Phase == "Pending" {
			summary.ContainerCreatingPods++
		}

		// Track high restart pods (threshold: >= 5 restarts)
		if pod.RestartCount >= 5 {
			highRestartCandidates = append(highRestartCandidates, podWithRestarts{
				pod:        pod,
				lastReason: lastTerminatedReason,
			})
		}

		// Sum up cluster-wide requests and limits
		// Only count Running and Pending pods (not Failed/Succeeded)
		if pod.Phase == "Running" || pod.Phase == "Pending" {
			summary.CPURequested += pod.CPURequest
			summary.CPULimited += pod.CPULimit
			summary.MemoryRequested += pod.MemoryRequest
			summary.MemoryLimited += pod.MemoryLimit
		}

		// Count workloads by owner reference (from labels)
		if ownerKind, ok := pod.Labels["app.kubernetes.io/component"]; ok {
			ownerName := pod.Labels["app.kubernetes.io/name"]
			key := pod.Namespace + "/" + ownerName
			switch ownerKind {
			case "deployment":
				deployments[key] = true
			case "statefulset":
				statefulSets[key] = true
			case "daemonset":
				daemonSets[key] = true
			}
		}
		// Also check owner-kind annotation or name prefix
		if job, ok := pod.Labels["job-name"]; ok {
			jobs[pod.Namespace+"/"+job] = true
		}
		if cronjob, ok := pod.Labels["batch.kubernetes.io/cronjob"]; ok {
			cronJobs[pod.Namespace+"/"+cronjob] = true
		}
	}

	summary.TotalDeployments = len(deployments)
	summary.TotalStatefulSets = len(statefulSets)
	summary.TotalDaemonSets = len(daemonSets)
	summary.TotalJobs = len(jobs)
	summary.TotalCronJobs = len(cronJobs)

	// Sort and select Top 5 high restart pods using sort.Slice (O(n log n))
	if len(highRestartCandidates) > 0 {
		// Sort by restart count descending
		sort.Slice(highRestartCandidates, func(i, j int) bool {
			return highRestartCandidates[i].pod.RestartCount > highRestartCandidates[j].pod.RestartCount
		})

		// Take top 5
		limit := 5
		if len(highRestartCandidates) < limit {
			limit = len(highRestartCandidates)
		}
		summary.HighRestartPods = make([]model.PodRestartInfo, limit)
		for i := 0; i < limit; i++ {
			summary.HighRestartPods[i] = model.PodRestartInfo{
				Name:         highRestartCandidates[i].pod.Name,
				Namespace:    highRestartCandidates[i].pod.Namespace,
				RestartCount: highRestartCandidates[i].pod.RestartCount,
				Reason:       highRestartCandidates[i].lastReason,
			}
		}
	}

	// Count events
	summary.TotalEvents = len(events)
	for _, event := range events {
		if event.Type == "Warning" {
			summary.WarningEvents++
		} else if event.Type == "Error" {
			summary.ErrorEvents++
		}
	}

	// Aggregate service statistics and health
	summary.TotalServices = len(services)
	for _, svc := range services {
		switch svc.Type {
		case "ClusterIP":
			summary.ClusterIPServices++
		case "NodePort":
			summary.NodePortServices++
		case "LoadBalancer":
			summary.LoadBalancerSvcs++
		}

		// Count endpoints
		summary.TotalEndpoints += svc.EndpointCount
		if svc.EndpointCount == 0 {
			summary.NoEndpointServices++
		}
	}

	// Calculate average endpoints per service
	if summary.TotalServices > 0 {
		summary.AvgEndpointsPerSvc = float64(summary.TotalEndpoints) / float64(summary.TotalServices)
	}

	// Aggregate storage statistics
	summary.TotalPVs = len(pvs)
	for _, pv := range pvs {
		summary.TotalStorageSize += pv.Capacity
		switch pv.Status {
		case "Bound":
			summary.BoundPVs++
			summary.UsedStorageSize += pv.Capacity
		case "Available":
			summary.AvailablePVs++
		case "Released":
			summary.ReleasedPVs++
		}
	}

	summary.TotalPVCs = len(pvcs)
	for _, pvc := range pvcs {
		switch pvc.Status {
		case "Bound":
			summary.BoundPVCs++
		case "Pending":
			summary.PendingPVCs++
		}
	}

	// Calculate utilization percentages
	if summary.CPUAllocatable > 0 {
		summary.CPURequestUtilization = float64(summary.CPURequested) / float64(summary.CPUAllocatable) * 100
		summary.CPULimitUtilization = float64(summary.CPULimited) / float64(summary.CPUAllocatable) * 100
	}
	if summary.CPUCapacity > 0 {
		summary.CPUUsageUtilization = float64(summary.CPUUsed) / float64(summary.CPUCapacity) * 100
	}
	if summary.MemoryAllocatable > 0 {
		summary.MemRequestUtilization = float64(summary.MemoryRequested) / float64(summary.MemoryAllocatable) * 100
		summary.MemLimitUtilization = float64(summary.MemoryLimited) / float64(summary.MemoryAllocatable) * 100
	}
	if summary.MemoryCapacity > 0 {
		summary.MemUsageUtilization = float64(summary.MemoryUsed) / float64(summary.MemoryCapacity) * 100
	}
	if summary.PodAllocatable > 0 {
		summary.PodUtilization = float64(summary.TotalPods) / float64(summary.PodAllocatable) * 100
	}
	if summary.TotalStorageSize > 0 {
		summary.StorageUsagePercent = float64(summary.UsedStorageSize) / float64(summary.TotalStorageSize) * 100
	}

	// Calculate NPU utilization and topology statistics
	if summary.NPUAllocatable > 0 {
		summary.NPUUtilization = float64(summary.NPUAllocated) / float64(summary.NPUAllocatable) * 100
	}
	summary.HyperNodeCount = len(hyperNodeIDs)
	summary.SuperPodCount = len(superPodIDs)

	// Collect alerts based on thresholds
	summary.Alerts = a.collectAlerts(nodes, pods, services, pvcs, summary)

	return summary
}

// shouldSkipKubeletEnrichment performs a cached RBAC check to avoid spamming kubelet when access is denied.
func (a *AggregatedDataSource) shouldSkipKubeletEnrichment(ctx context.Context) (bool, string) {
	if a.kubeletClient == nil || a.apiServerClient == nil {
		return false, ""
	}

	a.kubeletAccessMu.RLock()
	status := a.kubeletAccess
	a.kubeletAccessMu.RUnlock()

	if status != nil {
		if status.ProxyAllowed {
			return false, ""
		}
		if time.Since(status.CheckedAt) < kubeletAccessCheckTTL {
			return true, status.Message()
		}
	}

	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	checkCtx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()

	newStatus, err := a.apiServerClient.CheckKubeletAccess(checkCtx)
	if err != nil {
		// If we can't even perform the access check (e.g., no permission to create
		// SelfSubjectAccessReviews), treat it as "no kubelet access" to avoid
		// repeatedly hitting kubelet and getting 401/403 errors.
		a.logger.Warn("Kubelet access review failed, treating as no access",
			zap.Error(err),
		)

		// Cache this as a denial to prevent repeated failed checks
		deniedStatus := &diagnostic.KubeletAccessStatus{
			ProxyAllowed: false,
			ProxyMessage: fmt.Sprintf("Access review failed: %v", err),
			CheckedAt:    time.Now(),
		}

		a.kubeletAccessMu.Lock()
		a.kubeletAccess = deniedStatus
		a.kubeletAccessMu.Unlock()

		return true, deniedStatus.Message()
	}

	a.kubeletAccessMu.Lock()
	a.kubeletAccess = newStatus
	a.kubeletAccessMu.Unlock()

	if newStatus.ProxyAllowed {
		return false, ""
	}

	return true, newStatus.Message()
}

func (a *AggregatedDataSource) applyKubeletSkipReason(reason string, nodes []*model.NodeData, pods []*model.PodData) {
	if reason == "" {
		return
	}

	a.kubeletAccessMu.Lock()
	a.kubeletSkipReason = reason
	a.kubeletAccessMu.Unlock()

	// Clear kubelet metrics from nodes
	for _, node := range nodes {
		if node == nil {
			continue
		}
		node.HasKubeletMetrics = false
		if node.KubeletError == "" {
			node.KubeletError = reason
		}

		// Clear all kubelet-sourced usage metrics to prevent showing stale data
		node.CPUUsage = 0
		node.MemoryUsage = 0
		node.NetworkRxBytes = 0
		node.NetworkTxBytes = 0
		node.CPUUsagePercent = 0
		node.MemoryUsagePercent = 0
	}

	// Clear kubelet metrics from pods and containers
	for _, pod := range pods {
		if pod == nil {
			continue
		}

		// Clear pod-level kubelet metrics
		pod.CPUUsage = 0
		pod.MemoryUsage = 0
		pod.NetworkRxBytes = 0
		pod.NetworkTxBytes = 0

		// Clear container-level kubelet metrics
		for i := range pod.ContainerStates {
			pod.ContainerStates[i].CPUUsage = 0
			pod.ContainerStates[i].MemoryUsage = 0
		}
	}
}

func (a *AggregatedDataSource) clearKubeletSkipReason() {
	a.kubeletAccessMu.Lock()
	a.kubeletSkipReason = ""
	a.kubeletAccessMu.Unlock()
}

func (a *AggregatedDataSource) getKubeletSkipReason() string {
	a.kubeletAccessMu.RLock()
	defer a.kubeletAccessMu.RUnlock()
	return a.kubeletSkipReason
}

// Name returns the data source name
func (a *AggregatedDataSource) Name() string {
	return "Aggregated"
}

// Close cleans up resources
func (a *AggregatedDataSource) Close() error {
	a.logger.Info("Closing aggregated data source")
	if err := a.apiServer.Close(); err != nil {
		a.logger.Error("Failed to close API Server client", zap.Error(err))
	}
	if a.kubeletClient != nil {
		if err := a.kubeletClient.Close(); err != nil {
			a.logger.Error("Failed to close kubelet client", zap.Error(err))
		}
	}
	if a.npuExporterClient != nil {
		if err := a.npuExporterClient.Close(); err != nil {
			a.logger.Error("Failed to close NPU-Exporter client", zap.Error(err))
		}
	}
	return nil
}

// collectAlerts generates alerts based on cluster state and thresholds
func (a *AggregatedDataSource) collectAlerts(nodes []*model.NodeData, pods []*model.PodData, services []*model.ServiceData, pvcs []*model.PVCData, summary *model.ClusterSummary) []model.Alert {
	alerts := make([]model.Alert, 0)
	now := time.Now()

	// Thresholds
	const (
		nodeCPUCriticalThreshold    = 90.0 // %
		nodeCPUWarningThreshold     = 80.0 // %
		nodeMemoryCriticalThreshold = 90.0 // %
		nodeMemoryWarningThreshold  = 80.0 // %
		podCPUWarningThreshold      = 80.0 // %
		podMemoryWarningThreshold   = 80.0 // %
		pendingPodWarningMinutes    = 5
		highRestartThreshold        = 5
	)

	// Node alerts
	for _, node := range nodes {
		// Node not ready
		if node.Status != "Ready" {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityCritical,
				Category:          "Node",
				AlertType:         model.AlertTypeNodeNotReady,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           fmt.Sprintf("Node is %s", node.Status),
				Value:             node.Status,
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeNotReady, "", node.Name),
				Timestamp:         now,
			})
		}

		// Node pressure indicators
		if node.MemoryPressure {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityCritical,
				Category:          "Node",
				AlertType:         model.AlertTypeNodeMemoryPressure,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node has memory pressure",
				Value:             "MemoryPressure",
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeMemoryPressure, "", node.Name),
				Timestamp:         now,
			})
		}
		if node.DiskPressure {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Node",
				AlertType:         model.AlertTypeNodeDiskPressure,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node has disk pressure",
				Value:             "DiskPressure",
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeDiskPressure, "", node.Name),
				Timestamp:         now,
			})
		}
		if node.PIDPressure {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Node",
				AlertType:         model.AlertTypeNodePIDPressure,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node has PID pressure",
				Value:             "PIDPressure",
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodePIDPressure, "", node.Name),
				Timestamp:         now,
			})
		}

		// High CPU usage
		if node.CPUUsagePercent >= nodeCPUCriticalThreshold {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityCritical,
				Category:          "Resource",
				AlertType:         model.AlertTypeNodeCPUCritical,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node CPU usage critical",
				Value:             fmt.Sprintf("%.1f%%", node.CPUUsagePercent),
				Threshold:         fmt.Sprintf("%.0f%%", nodeCPUCriticalThreshold),
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeCPUCritical, "", node.Name),
				Timestamp:         now,
			})
		} else if node.CPUUsagePercent >= nodeCPUWarningThreshold {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Resource",
				AlertType:         model.AlertTypeNodeCPUHigh,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node CPU usage high",
				Value:             fmt.Sprintf("%.1f%%", node.CPUUsagePercent),
				Threshold:         fmt.Sprintf("%.0f%%", nodeCPUWarningThreshold),
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeCPUHigh, "", node.Name),
				Timestamp:         now,
			})
		}

		// High memory usage
		if node.MemoryUsagePercent >= nodeMemoryCriticalThreshold {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityCritical,
				Category:          "Resource",
				AlertType:         model.AlertTypeNodeMemoryCritical,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node memory usage critical",
				Value:             fmt.Sprintf("%.1f%%", node.MemoryUsagePercent),
				Threshold:         fmt.Sprintf("%.0f%%", nodeMemoryCriticalThreshold),
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeMemoryCritical, "", node.Name),
				Timestamp:         now,
			})
		} else if node.MemoryUsagePercent >= nodeMemoryWarningThreshold {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Resource",
				AlertType:         model.AlertTypeNodeMemoryHigh,
				ResourceType:      "Node",
				ResourceName:      node.Name,
				Message:           "Node memory usage high",
				Value:             fmt.Sprintf("%.1f%%", node.MemoryUsagePercent),
				Threshold:         fmt.Sprintf("%.0f%%", nodeMemoryWarningThreshold),
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeNodeMemoryHigh, "", node.Name),
				Timestamp:         now,
			})
		}
	}

	// Pod alerts
	for _, pod := range pods {
		// OOMKilled
		for _, container := range pod.ContainerStates {
			if container.Reason == "OOMKilled" {
				alerts = append(alerts, model.Alert{
					Severity:          model.AlertSeverityCritical,
					Category:          "Pod",
					AlertType:         model.AlertTypePodOOMKilled,
					ResourceType:      "Pod",
					ResourceName:      pod.Name,
					Namespace:         pod.Namespace,
					Message:           fmt.Sprintf("Container %s was OOMKilled", container.Name),
					Value:             fmt.Sprintf("%d restarts", container.RestartCount),
					RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePodOOMKilled, pod.Namespace, pod.Name),
					Timestamp:         now,
				})
				break // One alert per pod
			}
		}

		// CrashLoopBackOff
		for _, container := range pod.ContainerStates {
			if container.Reason == "CrashLoopBackOff" {
				alerts = append(alerts, model.Alert{
					Severity:          model.AlertSeverityCritical,
					Category:          "Pod",
					AlertType:         model.AlertTypePodCrashLoopBackOff,
					ResourceType:      "Pod",
					ResourceName:      pod.Name,
					Namespace:         pod.Namespace,
					Message:           fmt.Sprintf("Container %s in CrashLoopBackOff", container.Name),
					Value:             container.Reason,
					RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePodCrashLoopBackOff, pod.Namespace, pod.Name),
					Timestamp:         now,
				})
				break
			}
		}

		// ImagePullBackOff
		for _, container := range pod.ContainerStates {
			if container.Reason == "ImagePullBackOff" || container.Reason == "ErrImagePull" {
				alerts = append(alerts, model.Alert{
					Severity:          model.AlertSeverityWarning,
					Category:          "Pod",
					AlertType:         model.AlertTypePodImagePullBackOff,
					ResourceType:      "Pod",
					ResourceName:      pod.Name,
					Namespace:         pod.Namespace,
					Message:           fmt.Sprintf("Container %s failed to pull image", container.Name),
					Value:             container.Reason,
					RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePodImagePullBackOff, pod.Namespace, pod.Name),
					Timestamp:         now,
				})
				break
			}
		}

		// High restart count
		if pod.RestartCount >= highRestartThreshold {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Pod",
				AlertType:         model.AlertTypePodHighRestarts,
				ResourceType:      "Pod",
				ResourceName:      pod.Name,
				Namespace:         pod.Namespace,
				Message:           "Pod has high restart count",
				Value:             fmt.Sprintf("%d restarts", pod.RestartCount),
				Threshold:         fmt.Sprintf("%d", highRestartThreshold),
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePodHighRestarts, pod.Namespace, pod.Name),
				Timestamp:         now,
			})
		}

		// Pending for too long
		if pod.Phase == "Pending" {
			pendingDuration := time.Since(pod.CreationTimestamp)
			if pendingDuration.Minutes() >= float64(pendingPodWarningMinutes) {
				alerts = append(alerts, model.Alert{
					Severity:          model.AlertSeverityWarning,
					Category:          "Pod",
					AlertType:         model.AlertTypePodPendingTooLong,
					ResourceType:      "Pod",
					ResourceName:      pod.Name,
					Namespace:         pod.Namespace,
					Message:           "Pod pending for too long",
					Value:             fmt.Sprintf("%.0fm", pendingDuration.Minutes()),
					Threshold:         fmt.Sprintf("%dm", pendingPodWarningMinutes),
					RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePodPendingTooLong, pod.Namespace, pod.Name),
					Timestamp:         now,
				})
			}
		}

		// Failed pods
		if pod.Phase == "Failed" {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Pod",
				AlertType:         model.AlertTypePodFailed,
				ResourceType:      "Pod",
				ResourceName:      pod.Name,
				Namespace:         pod.Namespace,
				Message:           "Pod is in Failed state",
				Value:             pod.Phase,
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePodFailed, pod.Namespace, pod.Name),
				Timestamp:         now,
			})
		}
	}

	// Service alerts
	for _, svc := range services {
		if svc.EndpointCount == 0 {
			alerts = append(alerts, model.Alert{
				Severity:          model.AlertSeverityWarning,
				Category:          "Service",
				AlertType:         model.AlertTypeServiceNoEndpoints,
				ResourceType:      "Service",
				ResourceName:      svc.Name,
				Namespace:         svc.Namespace,
				Message:           "Service has no ready endpoints",
				Value:             "0 endpoints",
				RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypeServiceNoEndpoints, svc.Namespace, svc.Name),
				Timestamp:         now,
			})
		}
	}

	// PVC alerts
	for _, pvc := range pvcs {
		if pvc.Status == "Pending" {
			pendingDuration := time.Since(pvc.CreationTimestamp)
			if pendingDuration.Minutes() >= float64(pendingPodWarningMinutes) {
				alerts = append(alerts, model.Alert{
					Severity:          model.AlertSeverityWarning,
					Category:          "Storage",
					AlertType:         model.AlertTypePVCPendingTooLong,
					ResourceType:      "PVC",
					ResourceName:      pvc.Name,
					Namespace:         pvc.Namespace,
					Message:           "PVC pending for too long",
					Value:             fmt.Sprintf("%.0fm", pendingDuration.Minutes()),
					Threshold:         fmt.Sprintf("%dm", pendingPodWarningMinutes),
					RecommendedAction: diagnostic.GetRecommendedAction(model.AlertTypePVCPendingTooLong, pvc.Namespace, pvc.Name),
					Timestamp:         now,
				})
			}
		}
	}

	// Sort alerts by priority (using diagnostic.GetAlertPriority)
	sort.Slice(alerts, func(i, j int) bool {
		priI := diagnostic.GetAlertPriority(alerts[i].AlertType, alerts[i].Severity)
		priJ := diagnostic.GetAlertPriority(alerts[j].AlertType, alerts[j].Severity)
		return priI > priJ
	})

	return alerts
}

// GetPodLogs retrieves logs for a specific pod and container
func (a *AggregatedDataSource) GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error) {
	if a.apiServerClient == nil {
		return "", fmt.Errorf("API server client not available")
	}
	return a.apiServerClient.GetPodLogs(ctx, namespace, podName, containerName, tailLines)
}
