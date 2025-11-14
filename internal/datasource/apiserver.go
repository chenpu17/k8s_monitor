package datasource

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/diagnostic"
	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

// APIServerClient implements DataSource using Kubernetes API Server
type APIServerClient struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
	logger    *zap.Logger

	// Cache for incremental updates
	nodesResourceVersion  string
	podsResourceVersion   string
	eventsResourceVersion string
	lastNodesFetch        time.Time
	lastPodsFetch         time.Time
	cacheValidityDuration time.Duration
}

// NewAPIServerClient creates a new API Server client
func NewAPIServerClient(kubeconfig, context string, logger *zap.Logger) (*APIServerClient, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		// Try in-cluster config first
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fall back to default kubeconfig location
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			if context != "" {
				configOverrides.CurrentContext = context
			}
			config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				loadingRules,
				configOverrides,
			).ClientConfig()
			if err != nil {
				return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
			}
		}
	} else {
		// Use specified kubeconfig
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
		configOverrides := &clientcmd.ConfigOverrides{}
		if context != "" {
			configOverrides.CurrentContext = context
		}
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			configOverrides,
		).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfig, err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	client := &APIServerClient{
		clientset:             clientset,
		config:                config,
		logger:                logger,
		cacheValidityDuration: 5 * time.Second, // Cache valid for 5 seconds
	}

	logger.Info("API Server client initialized",
		zap.String("host", config.Host),
		zap.Duration("cache_validity", client.cacheValidityDuration),
	)

	return client, nil
}

// GetNodes retrieves all nodes in the cluster
func (c *APIServerClient) GetNodes(ctx context.Context) ([]*model.NodeData, error) {
	c.logger.Debug("Fetching nodes from API Server")

	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]*model.NodeData, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodeData := ConvertNode(&nodeList.Items[i])
		nodes = append(nodes, nodeData)
	}

	c.logger.Debug("Nodes fetched successfully",
		zap.Int("count", len(nodes)),
	)

	return nodes, nil
}

// GetPods retrieves all pods, optionally filtered by namespace
func (c *APIServerClient) GetPods(ctx context.Context, namespace string) ([]*model.PodData, error) {
	c.logger.Debug("Fetching pods from API Server",
		zap.String("namespace", namespace),
	)

	var podList *corev1.PodList
	var err error

	if namespace == "" {
		// Get all pods across all namespaces
		podList, err = c.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		// Get pods in specific namespace
		podList, err = c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pods := make([]*model.PodData, 0, len(podList.Items))
	for i := range podList.Items {
		podData := ConvertPod(&podList.Items[i])
		pods = append(pods, podData)
	}

	c.logger.Debug("Pods fetched successfully",
		zap.Int("count", len(pods)),
		zap.String("namespace", namespace),
	)

	return pods, nil
}

// GetEvents retrieves recent events, optionally filtered by type
func (c *APIServerClient) GetEvents(ctx context.Context, namespace string, eventTypes []string, limit int) ([]*model.EventData, error) {
	c.logger.Debug("Fetching events from API Server",
		zap.String("namespace", namespace),
		zap.Strings("types", eventTypes),
		zap.Int("limit", limit),
	)

	var eventList *corev1.EventList
	var err error

	if namespace == "" {
		// Get all events across all namespaces
		eventList, err = c.clientset.CoreV1().Events(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		// Get events in specific namespace
		eventList, err = c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Convert and filter events
	events := make([]*model.EventData, 0)
	for i := range eventList.Items {
		event := &eventList.Items[i]

		// Filter by event type if specified
		if len(eventTypes) > 0 {
			typeMatch := false
			for _, t := range eventTypes {
				if event.Type == t {
					typeMatch = true
					break
				}
			}
			if !typeMatch {
				continue
			}
		}

		eventData := ConvertEvent(event)
		events = append(events, eventData)
	}

	// Sort by last timestamp (most recent first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTimestamp.After(events[j].LastTimestamp)
	})

	// Apply limit
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	c.logger.Debug("Events fetched successfully",
		zap.Int("total", len(eventList.Items)),
		zap.Int("filtered", len(events)),
	)

	return events, nil
}

// GetServices retrieves all services across namespaces
func (c *APIServerClient) GetServices(ctx context.Context, namespace string) ([]*model.ServiceData, error) {
	c.logger.Debug("Fetching services from API Server")

	var services *corev1.ServiceList
	var err error

	if namespace == "" {
		services, err = c.clientset.CoreV1().Services(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		services, err = c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Fetch all endpoints in one List call to avoid O(N) API calls
	var endpointsList *corev1.EndpointsList
	if namespace == "" {
		endpointsList, err = c.clientset.CoreV1().Endpoints(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		endpointsList, err = c.clientset.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{})
	}

	// Build endpoint count map: namespace/name -> count
	endpointCounts := make(map[string]int)
	if err == nil && endpointsList != nil {
		for _, ep := range endpointsList.Items {
			count := 0
			for _, subset := range ep.Subsets {
				count += len(subset.Addresses)
			}
			key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
			endpointCounts[key] = count
		}
	} else {
		c.logger.Warn("Failed to fetch endpoints, endpoint counts will be unavailable",
			zap.Error(err),
		)
	}

	result := make([]*model.ServiceData, 0, len(services.Items))
	for _, svc := range services.Items {
		serviceData := &model.ServiceData{
			Name:              svc.Name,
			Namespace:         svc.Namespace,
			Type:              string(svc.Spec.Type),
			ClusterIP:         svc.Spec.ClusterIP,
			ExternalIPs:       svc.Spec.ExternalIPs,
			Selector:          svc.Spec.Selector,
			Labels:            svc.Labels,
			Annotations:       svc.Annotations,
			CreationTimestamp: svc.CreationTimestamp.Time,
		}

		// Parse ports
		serviceData.Ports = make([]model.ServicePort, len(svc.Spec.Ports))
		for i, port := range svc.Spec.Ports {
			serviceData.Ports[i] = model.ServicePort{
				Name:       port.Name,
				Protocol:   string(port.Protocol),
				Port:       port.Port,
				TargetPort: port.TargetPort.String(),
				NodePort:   port.NodePort,
			}
		}

		// LoadBalancer info
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if len(svc.Status.LoadBalancer.Ingress) > 0 {
				serviceData.LoadBalancerIP = svc.Status.LoadBalancer.Ingress[0].IP
				for _, ing := range svc.Status.LoadBalancer.Ingress {
					if ing.IP != "" {
						serviceData.Ingress = append(serviceData.Ingress, ing.IP)
					} else if ing.Hostname != "" {
						serviceData.Ingress = append(serviceData.Ingress, ing.Hostname)
					}
				}
			}
		}

		// Get endpoint count from pre-built map (O(1) lookup instead of O(N) API calls)
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		if count, ok := endpointCounts[key]; ok {
			serviceData.EndpointCount = count
		}

		result = append(result, serviceData)
	}

	c.logger.Debug("Services fetched successfully",
		zap.Int("count", len(result)),
	)

	return result, nil
}

// GetPersistentVolumes retrieves all persistent volumes
func (c *APIServerClient) GetPersistentVolumes(ctx context.Context) ([]*model.PVData, error) {
	c.logger.Debug("Fetching persistent volumes from API Server")

	pvList, err := c.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list persistent volumes: %w", err)
	}

	result := make([]*model.PVData, 0, len(pvList.Items))
	for _, pv := range pvList.Items {
		capacity := int64(0)
		if storage, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			capacity = storage.Value()
		}

		// Handle VolumeMode (defaults to Filesystem if nil)
		volumeMode := "Filesystem"
		if pv.Spec.VolumeMode != nil {
			volumeMode = string(*pv.Spec.VolumeMode)
		}

		pvData := &model.PVData{
			Name:              pv.Name,
			Capacity:          capacity,
			StorageClass:      pv.Spec.StorageClassName,
			AccessModes:       convertAccessModes(pv.Spec.AccessModes),
			ReclaimPolicy:     string(pv.Spec.PersistentVolumeReclaimPolicy),
			Status:            string(pv.Status.Phase),
			VolumeMode:        volumeMode,
			Labels:            pv.Labels,
			Annotations:       pv.Annotations,
			CreationTimestamp: pv.CreationTimestamp.Time,
			VolumeType:        getVolumeType(&pv),
		}

		if pv.Spec.ClaimRef != nil {
			pvData.Claim = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}

		result = append(result, pvData)
	}

	c.logger.Debug("Persistent volumes fetched successfully",
		zap.Int("count", len(result)),
	)

	return result, nil
}

// GetPersistentVolumeClaims retrieves all PVCs across namespaces
func (c *APIServerClient) GetPersistentVolumeClaims(ctx context.Context, namespace string) ([]*model.PVCData, error) {
	c.logger.Debug("Fetching persistent volume claims from API Server")

	var pvcList *corev1.PersistentVolumeClaimList
	var err error

	if namespace == "" {
		pvcList, err = c.clientset.CoreV1().PersistentVolumeClaims(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		pvcList, err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list persistent volume claims: %w", err)
	}

	result := make([]*model.PVCData, 0, len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		requestedStorage := int64(0)
		if storage, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			requestedStorage = storage.Value()
		}

		capacity := int64(0)
		if pvc.Status.Capacity != nil {
			if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
				capacity = storage.Value()
			}
		}

		// Handle VolumeMode (defaults to Filesystem if nil)
		volumeMode := "Filesystem"
		if pvc.Spec.VolumeMode != nil {
			volumeMode = string(*pvc.Spec.VolumeMode)
		}

		pvcData := &model.PVCData{
			Name:              pvc.Name,
			Namespace:         pvc.Namespace,
			Status:            string(pvc.Status.Phase),
			Volume:            pvc.Spec.VolumeName,
			Capacity:          capacity,
			RequestedStorage:  requestedStorage,
			StorageClass:      stringPtrToString(pvc.Spec.StorageClassName),
			AccessModes:       convertAccessModes(pvc.Spec.AccessModes),
			VolumeMode:        volumeMode,
			Labels:            pvc.Labels,
			Annotations:       pvc.Annotations,
			CreationTimestamp: pvc.CreationTimestamp.Time,
		}

		result = append(result, pvcData)
	}

	c.logger.Debug("Persistent volume claims fetched successfully",
		zap.Int("count", len(result)),
	)

	return result, nil
}

// Helper functions
func convertAccessModes(modes []corev1.PersistentVolumeAccessMode) []string {
	result := make([]string, len(modes))
	for i, mode := range modes {
		result[i] = string(mode)
	}
	return result
}

func stringPtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func getVolumeType(pv *corev1.PersistentVolume) string {
	spec := pv.Spec.PersistentVolumeSource
	switch {
	case spec.HostPath != nil:
		return "HostPath"
	case spec.NFS != nil:
		return "NFS"
	case spec.ISCSI != nil:
		return "iSCSI"
	case spec.Glusterfs != nil:
		return "Glusterfs"
	case spec.RBD != nil:
		return "RBD"
	case spec.CephFS != nil:
		return "CephFS"
	case spec.Cinder != nil:
		return "Cinder"
	case spec.FC != nil:
		return "FC"
	case spec.AWSElasticBlockStore != nil:
		return "AWS-EBS"
	case spec.GCEPersistentDisk != nil:
		return "GCE-PD"
	case spec.AzureDisk != nil:
		return "Azure-Disk"
	case spec.AzureFile != nil:
		return "Azure-File"
	case spec.CSI != nil:
		return "CSI-" + spec.CSI.Driver
	case spec.Local != nil:
		return "Local"
	default:
		return "Unknown"
	}
}

// Name returns the data source name
func (c *APIServerClient) Name() string {
	return "APIServer"
}

// CheckKubeletAccess verifies whether the current identity can reach kubelet via the API Server proxy.
func (c *APIServerClient) CheckKubeletAccess(ctx context.Context) (*diagnostic.KubeletAccessStatus, error) {
	if c == nil || c.clientset == nil {
		return nil, fmt.Errorf("api server client not initialised")
	}
	return diagnostic.CheckKubeletAccess(ctx, c.clientset.AuthorizationV1())
}

// GetConfig returns the Kubernetes client config
func (c *APIServerClient) GetConfig() *rest.Config {
	return c.config
}

// GetDeployments retrieves all deployments
func (c *APIServerClient) GetDeployments(ctx context.Context, namespace string) ([]*model.DeploymentData, error) {
	c.logger.Debug("Fetching deployments from API Server")

	var deployments *appsv1.DeploymentList
	var err error

	if namespace == "" {
		deployments, err = c.clientset.AppsV1().Deployments(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		deployments, err = c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	result := make([]*model.DeploymentData, 0, len(deployments.Items))
	for _, deploy := range deployments.Items {
		strategy := "RollingUpdate"
		if deploy.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			strategy = "Recreate"
		}

		conditions := make([]string, 0)
		for _, cond := range deploy.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				conditions = append(conditions, string(cond.Type))
			}
		}

		deployData := &model.DeploymentData{
			Name:              deploy.Name,
			Namespace:         deploy.Namespace,
			Replicas:          *deploy.Spec.Replicas,
			ReadyReplicas:     deploy.Status.ReadyReplicas,
			AvailableReplicas: deploy.Status.AvailableReplicas,
			UpdatedReplicas:   deploy.Status.UpdatedReplicas,
			Strategy:          strategy,
			Labels:            deploy.Labels,
			Annotations:       deploy.Annotations,
			CreationTimestamp: deploy.CreationTimestamp.Time,
			Selector:          deploy.Spec.Selector.MatchLabels,
			Conditions:        conditions,
		}

		result = append(result, deployData)
	}

	c.logger.Debug("Deployments fetched successfully", zap.Int("count", len(result)))
	return result, nil
}

// GetStatefulSets retrieves all statefulsets
func (c *APIServerClient) GetStatefulSets(ctx context.Context, namespace string) ([]*model.StatefulSetData, error) {
	c.logger.Debug("Fetching statefulsets from API Server")

	var statefulsets *appsv1.StatefulSetList
	var err error

	if namespace == "" {
		statefulsets, err = c.clientset.AppsV1().StatefulSets(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		statefulsets, err = c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list statefulsets: %w", err)
	}

	result := make([]*model.StatefulSetData, 0, len(statefulsets.Items))
	for _, sts := range statefulsets.Items {
		stsData := &model.StatefulSetData{
			Name:              sts.Name,
			Namespace:         sts.Namespace,
			Replicas:          *sts.Spec.Replicas,
			ReadyReplicas:     sts.Status.ReadyReplicas,
			CurrentReplicas:   sts.Status.CurrentReplicas,
			UpdatedReplicas:   sts.Status.UpdatedReplicas,
			Labels:            sts.Labels,
			Annotations:       sts.Annotations,
			CreationTimestamp: sts.CreationTimestamp.Time,
			Selector:          sts.Spec.Selector.MatchLabels,
		}

		result = append(result, stsData)
	}

	c.logger.Debug("StatefulSets fetched successfully", zap.Int("count", len(result)))
	return result, nil
}

// GetDaemonSets retrieves all daemonsets
func (c *APIServerClient) GetDaemonSets(ctx context.Context, namespace string) ([]*model.DaemonSetData, error) {
	c.logger.Debug("Fetching daemonsets from API Server")

	var daemonsets *appsv1.DaemonSetList
	var err error

	if namespace == "" {
		daemonsets, err = c.clientset.AppsV1().DaemonSets(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		daemonsets, err = c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list daemonsets: %w", err)
	}

	result := make([]*model.DaemonSetData, 0, len(daemonsets.Items))
	for _, ds := range daemonsets.Items {
		dsData := &model.DaemonSetData{
			Name:                   ds.Name,
			Namespace:              ds.Namespace,
			DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
			CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
			NumberReady:            ds.Status.NumberReady,
			NumberAvailable:        ds.Status.NumberAvailable,
			Labels:                 ds.Labels,
			Annotations:            ds.Annotations,
			CreationTimestamp:      ds.CreationTimestamp.Time,
			Selector:               ds.Spec.Selector.MatchLabels,
		}

		result = append(result, dsData)
	}

	c.logger.Debug("DaemonSets fetched successfully", zap.Int("count", len(result)))
	return result, nil
}

// GetJobs retrieves all jobs
func (c *APIServerClient) GetJobs(ctx context.Context, namespace string) ([]*model.JobData, error) {
	c.logger.Debug("Fetching jobs from API Server")

	var jobs *batchv1.JobList
	var err error

	if namespace == "" {
		jobs, err = c.clientset.BatchV1().Jobs(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		jobs, err = c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	result := make([]*model.JobData, 0, len(jobs.Items))
	for _, job := range jobs.Items {
		completions := int32(1)
		if job.Spec.Completions != nil {
			completions = *job.Spec.Completions
		}

		var duration time.Duration
		if job.Status.StartTime != nil && job.Status.CompletionTime != nil {
			duration = job.Status.CompletionTime.Sub(job.Status.StartTime.Time)
		}

		jobData := &model.JobData{
			Name:              job.Name,
			Namespace:         job.Namespace,
			Completions:       completions,
			Succeeded:         job.Status.Succeeded,
			Failed:            job.Status.Failed,
			Active:            job.Status.Active,
			Labels:            job.Labels,
			Annotations:       job.Annotations,
			CreationTimestamp: job.CreationTimestamp.Time,
			Duration:          duration,
		}

		if job.Status.StartTime != nil {
			jobData.StartTime = job.Status.StartTime.Time
		}
		if job.Status.CompletionTime != nil {
			jobData.CompletionTime = job.Status.CompletionTime.Time
		}

		result = append(result, jobData)
	}

	c.logger.Debug("Jobs fetched successfully", zap.Int("count", len(result)))
	return result, nil
}

// GetCronJobs retrieves all cronjobs
func (c *APIServerClient) GetCronJobs(ctx context.Context, namespace string) ([]*model.CronJobData, error) {
	c.logger.Debug("Fetching cronjobs from API Server")

	var cronjobs *batchv1.CronJobList
	var err error

	if namespace == "" {
		cronjobs, err = c.clientset.BatchV1().CronJobs(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		cronjobs, err = c.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list cronjobs: %w", err)
	}

	result := make([]*model.CronJobData, 0, len(cronjobs.Items))
	for _, cj := range cronjobs.Items {
		suspend := false
		if cj.Spec.Suspend != nil {
			suspend = *cj.Spec.Suspend
		}

		cronJobData := &model.CronJobData{
			Name:              cj.Name,
			Namespace:         cj.Namespace,
			Schedule:          cj.Spec.Schedule,
			Suspend:           suspend,
			Active:            int32(len(cj.Status.Active)),
			Labels:            cj.Labels,
			Annotations:       cj.Annotations,
			CreationTimestamp: cj.CreationTimestamp.Time,
		}

		if cj.Status.LastScheduleTime != nil {
			cronJobData.LastScheduleTime = cj.Status.LastScheduleTime.Time
		}

		result = append(result, cronJobData)
	}

	c.logger.Debug("CronJobs fetched successfully", zap.Int("count", len(result)))
	return result, nil
}

// GetPodLogs retrieves logs for a specific pod and container
func (c *APIServerClient) GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error) {
	c.logger.Debug("Fetching pod logs",
		zap.String("namespace", namespace),
		zap.String("pod", podName),
		zap.String("container", containerName),
		zap.Int64("tailLines", tailLines),
	)

	opts := &corev1.PodLogOptions{
		Container: containerName,
		TailLines: &tailLines,
	}

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	logStream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get log stream: %w", err)
	}
	defer logStream.Close()

	// Read logs from stream
	var buf strings.Builder
	_, err = io.Copy(&buf, logStream)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}

	return buf.String(), nil
}

// DescribePod returns kubectl describe output for a pod
func (c *APIServerClient) DescribePod(ctx context.Context, namespace, podName string) (string, error) {
	c.logger.Debug("Describing pod",
		zap.String("namespace", namespace),
		zap.String("pod", podName),
	)

	// Get pod details
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// Get events for this pod
	events, err := c.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", podName, namespace),
	})
	if err != nil {
		c.logger.Warn("Failed to get events", zap.Error(err))
		events = &corev1.EventList{}
	}

	// Format describe output
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("Name:         %s\n", pod.Name))
	buf.WriteString(fmt.Sprintf("Namespace:    %s\n", pod.Namespace))
	buf.WriteString(fmt.Sprintf("Node:         %s\n", pod.Spec.NodeName))
	buf.WriteString(fmt.Sprintf("Status:       %s\n", pod.Status.Phase))
	buf.WriteString(fmt.Sprintf("IP:           %s\n", pod.Status.PodIP))
	buf.WriteString(fmt.Sprintf("Created:      %s\n", pod.CreationTimestamp.Format(time.RFC3339)))

	// Labels
	if len(pod.Labels) > 0 {
		buf.WriteString("\nLabels:\n")
		for k, v := range pod.Labels {
			buf.WriteString(fmt.Sprintf("  %s=%s\n", k, v))
		}
	}

	// Containers
	buf.WriteString("\nContainers:\n")
	for _, container := range pod.Spec.Containers {
		buf.WriteString(fmt.Sprintf("  %s:\n", container.Name))
		buf.WriteString(fmt.Sprintf("    Image:         %s\n", container.Image))
		if len(container.Ports) > 0 {
			buf.WriteString("    Ports:         ")
			for i, port := range container.Ports {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol))
			}
			buf.WriteString("\n")
		}
	}

	// Container statuses
	buf.WriteString("\nContainer Statuses:\n")
	for _, cs := range pod.Status.ContainerStatuses {
		buf.WriteString(fmt.Sprintf("  %s:\n", cs.Name))
		buf.WriteString(fmt.Sprintf("    State:         %s\n", getContainerState(cs)))
		buf.WriteString(fmt.Sprintf("    Ready:         %t\n", cs.Ready))
		buf.WriteString(fmt.Sprintf("    Restart Count: %d\n", cs.RestartCount))
	}

	// Conditions
	if len(pod.Status.Conditions) > 0 {
		buf.WriteString("\nConditions:\n")
		for _, cond := range pod.Status.Conditions {
			buf.WriteString(fmt.Sprintf("  Type:    %s\n", cond.Type))
			buf.WriteString(fmt.Sprintf("  Status:  %s\n", cond.Status))
			if cond.Reason != "" {
				buf.WriteString(fmt.Sprintf("  Reason:  %s\n", cond.Reason))
			}
			if cond.Message != "" {
				buf.WriteString(fmt.Sprintf("  Message: %s\n", cond.Message))
			}
			buf.WriteString("\n")
		}
	}

	// Events
	if len(events.Items) > 0 {
		buf.WriteString("Events:\n")
		for _, event := range events.Items {
			buf.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
				event.LastTimestamp.Format("15:04:05"),
				event.Type,
				event.Reason,
				event.Message,
			))
		}
	}

	return buf.String(), nil
}

// DescribeNode returns kubectl describe output for a node
func (c *APIServerClient) DescribeNode(ctx context.Context, nodeName string) (string, error) {
	c.logger.Debug("Describing node", zap.String("node", nodeName))

	// Get node details
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node: %w", err)
	}

	// Get events for this node
	events, err := c.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", nodeName),
	})
	if err != nil {
		c.logger.Warn("Failed to get events", zap.Error(err))
		events = &corev1.EventList{}
	}

	// Format describe output
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("Name:         %s\n", node.Name))

	// Roles
	roles := []string{}
	for label := range node.Labels {
		if strings.HasPrefix(label, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
			roles = append(roles, role)
		}
	}
	if len(roles) > 0 {
		buf.WriteString(fmt.Sprintf("Roles:        %s\n", strings.Join(roles, ",")))
	}

	// Labels
	buf.WriteString("\nLabels:\n")
	for k, v := range node.Labels {
		buf.WriteString(fmt.Sprintf("  %s=%s\n", k, v))
	}

	// Addresses
	buf.WriteString("\nAddresses:\n")
	for _, addr := range node.Status.Addresses {
		buf.WriteString(fmt.Sprintf("  %s: %s\n", addr.Type, addr.Address))
	}

	// Capacity
	buf.WriteString("\nCapacity:\n")
	buf.WriteString(fmt.Sprintf("  cpu:     %s\n", node.Status.Capacity.Cpu().String()))
	buf.WriteString(fmt.Sprintf("  memory:  %s\n", node.Status.Capacity.Memory().String()))
	buf.WriteString(fmt.Sprintf("  pods:    %s\n", node.Status.Capacity.Pods().String()))

	// Allocatable
	buf.WriteString("\nAllocatable:\n")
	buf.WriteString(fmt.Sprintf("  cpu:     %s\n", node.Status.Allocatable.Cpu().String()))
	buf.WriteString(fmt.Sprintf("  memory:  %s\n", node.Status.Allocatable.Memory().String()))
	buf.WriteString(fmt.Sprintf("  pods:    %s\n", node.Status.Allocatable.Pods().String()))

	// System Info
	buf.WriteString("\nSystem Info:\n")
	buf.WriteString(fmt.Sprintf("  OS Image:           %s\n", node.Status.NodeInfo.OSImage))
	buf.WriteString(fmt.Sprintf("  Kernel Version:     %s\n", node.Status.NodeInfo.KernelVersion))
	buf.WriteString(fmt.Sprintf("  Container Runtime:  %s\n", node.Status.NodeInfo.ContainerRuntimeVersion))
	buf.WriteString(fmt.Sprintf("  Kubelet Version:    %s\n", node.Status.NodeInfo.KubeletVersion))

	// Conditions
	buf.WriteString("\nConditions:\n")
	for _, cond := range node.Status.Conditions {
		buf.WriteString(fmt.Sprintf("  Type:    %s\n", cond.Type))
		buf.WriteString(fmt.Sprintf("  Status:  %s\n", cond.Status))
		if cond.Reason != "" {
			buf.WriteString(fmt.Sprintf("  Reason:  %s\n", cond.Reason))
		}
		if cond.Message != "" {
			buf.WriteString(fmt.Sprintf("  Message: %s\n", cond.Message))
		}
		buf.WriteString("\n")
	}

	// Events
	if len(events.Items) > 0 {
		buf.WriteString("Events:\n")
		for _, event := range events.Items {
			buf.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
				event.LastTimestamp.Format("15:04:05"),
				event.Type,
				event.Reason,
				event.Message,
			))
		}
	}

	return buf.String(), nil
}

// GetPodYAML returns YAML representation of a pod
func (c *APIServerClient) GetPodYAML(ctx context.Context, namespace, podName string) (string, error) {
	c.logger.Debug("Getting pod YAML",
		zap.String("namespace", namespace),
		zap.String("pod", podName),
	)

	// Get pod
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(pod)
	if err != nil {
		return "", fmt.Errorf("failed to convert to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// GetNodeYAML returns YAML representation of a node
func (c *APIServerClient) GetNodeYAML(ctx context.Context, nodeName string) (string, error) {
	c.logger.Debug("Getting node YAML", zap.String("node", nodeName))

	// Get node
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node: %w", err)
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(node)
	if err != nil {
		return "", fmt.Errorf("failed to convert to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// getContainerState returns a human-readable container state
func getContainerState(cs corev1.ContainerStatus) string {
	if cs.State.Running != nil {
		return fmt.Sprintf("Running (started at %s)", cs.State.Running.StartedAt.Format(time.RFC3339))
	}
	if cs.State.Waiting != nil {
		return fmt.Sprintf("Waiting (%s: %s)", cs.State.Waiting.Reason, cs.State.Waiting.Message)
	}
	if cs.State.Terminated != nil {
		return fmt.Sprintf("Terminated (exit code %d, reason: %s)",
			cs.State.Terminated.ExitCode, cs.State.Terminated.Reason)
	}
	return "Unknown"
}

// Close cleans up resources
func (c *APIServerClient) Close() error {
	c.logger.Info("Closing API Server client")
	// No resources to clean up for API Server client
	return nil
}
