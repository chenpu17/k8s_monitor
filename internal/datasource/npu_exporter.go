package datasource

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// npuExporterTimeout is the timeout for NPU-Exporter requests
	// This prevents blocking the entire refresh cycle if the exporter is unavailable
	npuExporterTimeout = 5 * time.Second

	// npuExporterRetryDelay is the delay before retrying after a failure
	npuExporterRetryDelay = 30 * time.Second

	// chipsPerNPU is the typical number of chips per NPU device
	// Ascend 910 series typically has 2 chips per NPU card
	chipsPerNPU = 2
)

// npuExporterPodSelectors lists the pod labels NPU-Exporter DaemonSets
// carry in the wild. Huawei's official manifests (CCE) label the pods
// app.kubernetes.io/name=npu-exporter, while this repository's
// deploy/npu-exporter.yaml historically uses app=npu-exporter. Every pod
// scrapes only its own node's chips, which is what makes an answer
// attributable at all, so discovery tries both shapes before falling back
// to the label-less service path.
var npuExporterPodSelectors = []string{
	"app.kubernetes.io/name=npu-exporter",
	"app=npu-exporter",
}

// NPUExporterClient provides access to NPU metrics from NPU-Exporter
// It is safe for concurrent use: the refresh loop, the ForceRefresh path and
// the UI-triggered data lookups may all call it at the same time.
type NPUExporterClient struct {
	restClient  rest.Interface
	httpClient  *http.Client
	serviceName string // NPU-Exporter service name
	serviceNS   string // NPU-Exporter namespace
	servicePort string // NPU-Exporter port
	logger      *zap.Logger

	// mu guards the mutable fields below. It is never held while performing
	// network I/O, so a slow exporter cannot block other goroutines.
	mu              sync.Mutex
	serviceEndpoint string    // Custom endpoint override (e.g., "http://localhost:8082")
	useProxy        bool      // Whether to use K8s API proxy (default true)
	available       bool      // Whether the last request to NPU-Exporter succeeded
	lastFailure     time.Time // Time of last failure, used for retry backoff
	lastSuccess     time.Time // Time of last success, reported by Liveness
	lastError       string    // Raw text of the last failure, reported by Liveness
	everSucceeded   bool      // Whether any request has ever succeeded
	// perNodeAttribution reports whether the last enrichment cycle scraped the
	// exporter per node (one pod each) instead of through the single-answer
	// service path. Where it is true the metrics are attributable per node and
	// the liveness state describes a path that really was walked for every NPU
	// node, not just for a one-node cluster.
	perNodeAttribution bool
	// eccSamples is the per-chip sliding window of cumulative ECC counters the
	// enrichment path keeps (last eccSampleWindow refreshes), used to derive the
	// session-scoped delta. Guarded by mu like the fields above.
	//
	// The key carries the node name: every node numbers its chips from 0, so a
	// chip-ID-only key would mix one node's counters into another's window and
	// produce deltas across two unrelated chips.
	eccSamples map[eccChipKey][]eccSample

	// podLister/podFetcher are the seams to the per-exporter-pod scrape path.
	// The defaults talk to the API server (list kube-system pods, then proxy
	// :8082/metrics per pod); tests inject fixtures. Both are set once at
	// construction and only read afterwards.
	podLister  func(ctx context.Context) ([]npuExporterPod, error)
	podFetcher func(ctx context.Context, pod npuExporterPod) (io.ReadCloser, error)
}

// eccChipKey identifies one chip's ECC window: chip IDs repeat across nodes.
type eccChipKey struct {
	node string
	id   int
}

// npuExporterPod is one NPU-Exporter pod discovered through the API server, the
// unit of a per-node scrape: NodeName is the node whose chips it reports.
type npuExporterPod struct {
	Name     string
	NodeName string
}

// npuExporterTarget is one discovered pod paired with the node it reports on.
type npuExporterTarget struct {
	pod  npuExporterPod
	node *model.NodeData
}

// eccSampleWindow is how many cumulative ECC counter samples the client keeps
// per chip: the same depth the UI keeps for its metric trends. The oldest
// sample of the window is the baseline the delta is measured against.
const eccSampleWindow = 10

// eccSample is one refresh's cumulative ECC counters for a chip.
type eccSample struct {
	at     time.Time
	single int64
	double int64
}

// NPUChipMetrics represents detailed NPU chip metrics from NPU-Exporter
type NPUChipMetrics struct {
	ID                 int
	ModelName          string
	PCIeBusInfo        string
	VdieID             string
	PodName            string
	Namespace          string
	ContainerName      string
	Utilization        float64 // AI Core utilization %
	VectorUtilization  float64 // Vector utilization %
	HBMTotalMemory     int64   // bytes
	HBMUsedMemory      int64   // bytes
	Temperature        int     // Celsius
	Power              float64 // Watts
	HealthStatus       int     // 1 = healthy, 0 = unhealthy, -1 = unknown (DCMI failure); Known=false when the metric is absent
	HealthStatusKnown  bool    // False when the exporter did not report health status
	AICoreCurrentFreq  int     // MHz
	BandwidthRx        float64 // MB/s
	BandwidthTx        float64 // MB/s
	Voltage            float64 // V
	LinkStatus         int     // 1 = up, 0 = down
	LinkStatusKnown    bool    // False when the exporter did not report link status
	LinkSpeed          int     // link speed
	LinkUpNum          int     // number of links up
	NetworkStatus      int
	ErrorCode          int
	HBMEccSingleBitErr int64
	HBMEccDoubleBitErr int64
	// Cumulative totals (npu_chip_info_hbm_ecc_total_*_bit_error_cnt). Carried
	// for display only; threshold alerts deliberately read the per-window _cnt.
	HBMEccSingleBitErrTotal int64
	HBMEccDoubleBitErrTotal int64
	RoCETxAllPktNum         int64
	RoCERxAllPktNum         int64
	RoCETxErrPktNum         int64
	RoCERxErrPktNum         int64
}

// NodeNPUMetrics represents aggregated NPU metrics for a node
type NodeNPUMetrics struct {
	NodeName    string
	NPUCount    int
	ChipMetrics []NPUChipMetrics
	Timestamp   time.Time
}

// NewNPUExporterClient creates a new NPU Exporter client
func NewNPUExporterClient(config *rest.Config, logger *zap.Logger) (*NPUExporterClient, error) {
	// Create kubernetes clientset for API proxy access
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Get the REST client for core API (services are in core API group)
	restClient := clientset.CoreV1().RESTClient()

	client := &NPUExporterClient{
		restClient:  restClient,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		useProxy:    true, // Default to using K8s API proxy
		serviceName: "npu-exporter",
		serviceNS:   "kube-system",
		servicePort: "8082",
		logger:      logger,
		available:   false,
	}
	client.podLister = client.listExporterPods
	client.podFetcher = client.fetchPodMetrics

	return client, nil
}

// SetEndpoint allows overriding the default NPU-Exporter endpoint
// When a custom endpoint is set, direct HTTP access is used instead of K8s API proxy
func (c *NPUExporterClient) SetEndpoint(endpoint string) {
	c.mu.Lock()
	c.serviceEndpoint = endpoint
	c.useProxy = false // Use direct HTTP when custom endpoint is provided
	c.mu.Unlock()

	c.logger.Info("NPU-Exporter using custom endpoint", zap.String("endpoint", endpoint))
}

// inBackoffLocked reports whether a recent failure should suppress new
// requests. The caller must hold mu.
func (c *NPUExporterClient) inBackoffLocked() bool {
	return !c.lastFailure.IsZero() && time.Since(c.lastFailure) < npuExporterRetryDelay
}

// inBackoff reports whether a recent failure should suppress new requests
func (c *NPUExporterClient) inBackoff() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inBackoffLocked()
}

// isAvailable reports whether the last request to NPU-Exporter succeeded
func (c *NPUExporterClient) isAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.available
}

// recordSuccess marks NPU-Exporter as reachable and clears the retry backoff
func (c *NPUExporterClient) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.available = true
	c.lastFailure = time.Time{}
	c.lastSuccess = time.Now()
	c.lastError = ""
	c.everSucceeded = true
}

// recordFailure marks NPU-Exporter as unreachable and arms the retry backoff.
// The optional cause is kept as the raw failure text Liveness reports; callers
// that only flip the state (tests, probes) may omit it.
func (c *NPUExporterClient) recordFailure(cause ...error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.available = false
	c.lastFailure = time.Now()
	if len(cause) > 0 && cause[0] != nil {
		c.lastError = cause[0].Error()
	}
}

// setPerNodeAttribution records which enrichment path the running cycle took.
func (c *NPUExporterClient) setPerNodeAttribution(perNode bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perNodeAttribution = perNode
}

// LivenessAppliesTo reports whether the exporter's liveness state describes a
// path that was actually walked for a cluster with npuNodeCount NPU nodes. The
// per-pod path covers every NPU node, so its state is meaningful there; the
// single-answer service path is only walked for a one-node cluster, and
// claiming a liveness state for several nodes would render as "monitoring
// unavailable" while the exporter is fine.
func (c *NPUExporterClient) LivenessAppliesTo(npuNodeCount int) bool {
	if npuNodeCount == 0 {
		return false
	}

	c.mu.Lock()
	perNode := c.perNodeAttribution
	c.mu.Unlock()

	return perNode || npuNodeCount == 1
}

// endpointConfig returns the endpoint settings to use for the next request
func (c *NPUExporterClient) endpointConfig() (useProxy bool, endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.useProxy, c.serviceEndpoint
}

// CheckAvailability checks if NPU-Exporter is available
// Uses retry backoff to avoid repeated failures blocking the refresh cycle
func (c *NPUExporterClient) CheckAvailability(ctx context.Context) error {
	_, _, err := c.checkAvailability(ctx)
	return err
}

// checkAvailability probes NPU-Exporter and returns the metrics it fetched,
// so callers can reuse that response instead of issuing a second request
func (c *NPUExporterClient) checkAvailability(ctx context.Context) (map[int]*NPUChipMetrics, int, error) {
	// Check if we should skip due to recent failure (retry backoff)
	if c.inBackoff() {
		return nil, 0, fmt.Errorf("NPU-Exporter recently failed, waiting for retry backoff")
	}

	// Create a timeout context for the availability check
	timeoutCtx, cancel := context.WithTimeout(ctx, npuExporterTimeout)
	defer cancel()

	return c.GetMetrics(timeoutCtx)
}

// IsAvailable returns whether NPU-Exporter is available
func (c *NPUExporterClient) IsAvailable() bool {
	return c.isAvailable()
}

// Degraded reports whether NPU monitoring is currently suspended: a request
// to the exporter failed recently enough that enrichment is being skipped for
// the retry backoff. It is false before the first attempt, so a cold start
// reads as "unknown" rather than "degraded", and it clears on the first
// successful request.
func (c *NPUExporterClient) Degraded() bool {
	return c.inBackoff()
}

// Liveness reports the current state of the exporter fetch path. Unlike
// IsAvailable it distinguishes "never attempted" from "failed", so the UI can
// say "not polled yet" instead of claiming a failure that never happened. The
// returned value is a copy taken under mu; the raw failure text is included for
// the export and log paths, and MessageKey is what the UI localizes.
func (c *NPUExporterClient) Liveness() model.NPUExporterLiveness {
	c.mu.Lock()
	defer c.mu.Unlock()

	liveness := model.NPUExporterLiveness{LastSuccess: c.lastSuccess}
	switch {
	case c.inBackoffLocked():
		liveness.Status = "backoff"
		liveness.LastError = c.lastError
		liveness.MessageKey = "npu.liveness.backoff"
		liveness.MessageArgs = map[string]interface{}{"Error": c.lastError}
	case !c.everSucceeded:
		liveness.Status = "never"
		liveness.MessageKey = "npu.liveness.never"
	default:
		liveness.Status = "ok"
		liveness.MessageKey = "npu.liveness.ok"
	}
	return liveness
}

// listExporterPods discovers the NPU-Exporter pods and the node each of them
// runs on. This is the only way to attribute an exporter answer to a node: the
// metrics carry no node identity, but the pod that serves them does.
//
// Every known label variant is listed and the results merged, so a cluster
// becomes undiscoverable only when no variant answers. Pods seen twice (both
// variants matching) are kept once; attribution orders by pod name anyway.
func (c *NPUExporterClient) listExporterPods(ctx context.Context) ([]npuExporterPod, error) {
	seen := make(map[string]struct{})
	pods := make([]npuExporterPod, 0, len(seen))
	var lastErr error
	for _, selector := range npuExporterPodSelectors {
		result := c.restClient.Get().
			Namespace(c.serviceNS).
			Resource("pods").
			VersionedParams(&metav1.ListOptions{LabelSelector: selector}, metav1.ParameterCodec).
			Do(ctx)

		rawResp, err := result.Raw()
		if err != nil {
			// One label variant failing to list must not hide the other: only
			// when no variant answered is the discovery declared failed and
			// the cycle falls back to the service endpoint.
			lastErr = fmt.Errorf("failed to list NPU-Exporter pods (%s): %w", selector, err)
			c.logger.Debug("NPU-Exporter pod list failed for one label variant",
				zap.String("selector", selector), zap.Error(err),
			)
			continue
		}

		var podList corev1.PodList
		if err := json.Unmarshal(rawResp, &podList); err != nil {
			lastErr = fmt.Errorf("failed to decode NPU-Exporter pod list (%s): %w", selector, err)
			continue
		}

		for _, pod := range podList.Items {
			if _, dup := seen[pod.Name]; dup {
				continue
			}
			seen[pod.Name] = struct{}{}
			pods = append(pods, npuExporterPod{Name: pod.Name, NodeName: pod.Spec.NodeName})
		}
	}
	if len(pods) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return pods, nil
}

// fetchPodMetrics fetches one exporter pod's own /metrics through the API
// server pod proxy. Unlike the service path this hits a specific pod, so the
// answer describes exactly one node.
func (c *NPUExporterClient) fetchPodMetrics(ctx context.Context, pod npuExporterPod) (io.ReadCloser, error) {
	// Path: /api/v1/namespaces/{ns}/pods/{name}:{port}/proxy/metrics
	result := c.restClient.Get().
		Namespace(c.serviceNS).
		Resource("pods").
		Name(fmt.Sprintf("%s:%s", pod.Name, c.servicePort)).
		SubResource("proxy").
		Suffix("metrics").
		Do(ctx)

	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("failed to proxy to NPU-Exporter pod %s: %w", pod.Name, err)
	}

	rawResp, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw response from pod %s: %w", pod.Name, err)
	}

	return io.NopCloser(bytes.NewReader(rawResp)), nil
}

// fetchMetricsViaProxy fetches metrics using Kubernetes API server proxy
func (c *NPUExporterClient) fetchMetricsViaProxy(ctx context.Context) (io.ReadCloser, error) {
	// Use Kubernetes API server proxy to access the service
	// Path: /api/v1/namespaces/{namespace}/services/{service}:{port}/proxy/metrics
	result := c.restClient.Get().
		Namespace(c.serviceNS).
		Resource("services").
		Name(fmt.Sprintf("%s:%s", c.serviceName, c.servicePort)).
		SubResource("proxy").
		Suffix("metrics").
		Do(ctx)

	if err := result.Error(); err != nil {
		return nil, fmt.Errorf("failed to proxy to NPU-Exporter: %w", err)
	}

	// Get raw response
	rawResp, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw response: %w", err)
	}

	return io.NopCloser(strings.NewReader(string(rawResp))), nil
}

// fetchMetricsDirect fetches metrics using direct HTTP
func (c *NPUExporterClient) fetchMetricsDirect(ctx context.Context, endpoint string) (io.ReadCloser, error) {
	url := endpoint + "/metrics"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("NPU-Exporter returned status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// GetMetrics fetches all NPU metrics from NPU-Exporter
func (c *NPUExporterClient) GetMetrics(ctx context.Context) (map[int]*NPUChipMetrics, int, error) {
	useProxy, endpoint := c.endpointConfig()

	var body io.ReadCloser
	var err error

	if useProxy {
		body, err = c.fetchMetricsViaProxy(ctx)
	} else {
		body, err = c.fetchMetricsDirect(ctx, endpoint)
	}

	if err != nil {
		c.recordFailure(err)
		return nil, 0, err
	}
	defer body.Close()

	c.recordSuccess()
	return c.parseMetrics(body)
}

// parseMetrics parses Prometheus format metrics from NPU-Exporter
func (c *NPUExporterClient) parseMetrics(body io.Reader) (map[int]*NPUChipMetrics, int, error) {
	chipMetrics := make(map[int]*NPUChipMetrics)
	npuCount := 0

	// Regular expression to parse metric lines
	// Format: metric_name{label1="value1",label2="value2"} value timestamp
	metricLineRegex := regexp.MustCompile(`^([a-z_]+)\{([^}]*)\}\s+([\d.e+-]+)(?:\s+\d+)?$`)
	labelRegex := regexp.MustCompile(`([a-z_]+)="([^"]*)"`)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Parse machine_npu_nums for total NPU count
		if strings.HasPrefix(line, "machine_npu_nums ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if count, err := strconv.Atoi(parts[1]); err == nil {
					npuCount = count
				}
			}
			continue
		}

		// Parse labeled metrics
		matches := metricLineRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		metricName := matches[1]
		labelStr := matches[2]
		valueStr := matches[3]

		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		// Parse labels
		labels := make(map[string]string)
		labelMatches := labelRegex.FindAllStringSubmatch(labelStr, -1)
		for _, lm := range labelMatches {
			labels[lm[1]] = lm[2]
		}

		// Get chip ID from labels
		idStr := labels["id"]
		if idStr == "" {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		// Get or create chip metrics
		chip, exists := chipMetrics[id]
		if !exists {
			chip = &NPUChipMetrics{
				ID:            id,
				ModelName:     labels["model_name"],
				PCIeBusInfo:   labels["pcie_bus_info"],
				VdieID:        labels["vdie_id"],
				PodName:       labels["pod_name"],
				Namespace:     labels["namespace"],
				ContainerName: labels["container_name"],
			}
			chipMetrics[id] = chip
		}

		// Update chip metrics based on metric name
		switch metricName {
		case "npu_chip_info_utilization":
			chip.Utilization = value
		case "npu_chip_info_vector_utilization":
			chip.VectorUtilization = value
		case "npu_chip_info_hbm_total_memory":
			chip.HBMTotalMemory = int64(value)
		case "npu_chip_info_hbm_used_memory":
			chip.HBMUsedMemory = int64(value)
		case "npu_chip_info_temperature":
			chip.Temperature = int(value)
		case "npu_chip_info_power":
			chip.Power = value
		case "npu_chip_info_health_status":
			chip.HealthStatus = int(value)
			chip.HealthStatusKnown = true
		case "npu_chip_info_aicore_current_freq":
			chip.AICoreCurrentFreq = int(value)
		case "npu_chip_info_bandwidth_rx":
			chip.BandwidthRx = value
		case "npu_chip_info_bandwidth_tx":
			chip.BandwidthTx = value
		case "npu_chip_info_voltage":
			chip.Voltage = value
		case "npu_chip_info_link_status":
			chip.LinkStatus = int(value)
			chip.LinkStatusKnown = true
		case "npu_chip_link_speed":
			chip.LinkSpeed = int(value)
		case "npu_chip_link_up_num":
			chip.LinkUpNum = int(value)
		case "npu_chip_info_network_status":
			chip.NetworkStatus = int(value)
		case "npu_chip_info_error_code":
			chip.ErrorCode = int(value)
		case "npu_chip_info_hbm_ecc_single_bit_error_cnt":
			chip.HBMEccSingleBitErr = int64(value)
		case "npu_chip_info_hbm_ecc_double_bit_error_cnt":
			chip.HBMEccDoubleBitErr = int64(value)
		case "npu_chip_info_hbm_ecc_total_single_bit_error_cnt":
			chip.HBMEccSingleBitErrTotal = int64(value)
		case "npu_chip_info_hbm_ecc_total_double_bit_error_cnt":
			chip.HBMEccDoubleBitErrTotal = int64(value)
		case "npu_chip_roce_tx_all_pkt_num":
			chip.RoCETxAllPktNum = int64(value)
		case "npu_chip_roce_rx_all_pkt_num":
			chip.RoCERxAllPktNum = int64(value)
		case "npu_chip_roce_tx_err_pkt_num":
			chip.RoCETxErrPktNum = int64(value)
		case "npu_chip_roce_rx_err_pkt_num":
			chip.RoCERxErrPktNum = int64(value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("error reading response: %w", err)
	}

	return chipMetrics, npuCount, nil
}

// EnrichNodeData enriches node data with NPU metrics from NPU-Exporter.
//
// Two paths reach the exporter and they differ in what an answer proves:
//   - Per exporter pod (API server path): every NPU-Exporter DaemonSet pod is
//     scraped through its own pod proxy, so each answer belongs to the node
//     that pod runs on and a multi-NPU-node cluster is enriched per node.
//   - Service / custom endpoint: one answer whose metrics carry no node
//     identity, so only a single-NPU-node cluster can be enriched.
//
// The per-pod path is tried first and falls back to the service path when no
// exporter pod can be discovered.
func (c *NPUExporterClient) EnrichNodeData(ctx context.Context, nodes []*model.NodeData) error {
	// Skip if recently failed (retry backoff)
	if c.inBackoff() {
		c.logger.Debug("NPU-Exporter in backoff period, skipping enrichment")
		return nil
	}

	// Count NPU nodes before any network call: with no NPU nodes there is
	// nothing to enrich and the request would be wasted.
	var npuNodes []*model.NodeData
	for _, node := range nodes {
		if node.NPUCapacity > 0 {
			npuNodes = append(npuNodes, node)
		}
	}

	if len(npuNodes) == 0 {
		c.logger.Debug("No NPU nodes found in cluster")
		return nil
	}

	if c.enrichPerExporterPod(ctx, npuNodes) {
		return nil
	}

	return c.enrichViaService(ctx, npuNodes)
}

// enrichPerExporterPod scrapes one exporter pod per NPU node and returns
// whether it handled the cycle. It declines (returns false) when the answer
// cannot be attributed: a custom endpoint has no node identity, pod discovery
// failed, or no discovered pod runs on an NPU node.
func (c *NPUExporterClient) enrichPerExporterPod(ctx context.Context, npuNodes []*model.NodeData) bool {
	useProxy, _ := c.endpointConfig()
	if !useProxy || c.podLister == nil || c.podFetcher == nil {
		// Custom endpoint mode: a single endpoint is scraped directly and
		// carries no node identity, so there is nothing to attribute here.
		return false
	}

	pods, err := c.podLister(ctx)
	if err != nil {
		c.logger.Warn("Failed to list NPU-Exporter pods, falling back to the service endpoint", zap.Error(err))
		return false
	}
	if len(pods) == 0 {
		c.logger.Debug("No NPU-Exporter pods discovered, falling back to the service endpoint")
		return false
	}

	targets := c.attributePods(pods, npuNodes)
	if len(targets) == 0 {
		c.logger.Debug("No discovered NPU-Exporter pod runs on an NPU node, falling back to the service endpoint",
			zap.Int("pods", len(pods)),
		)
		return false
	}

	// Claim per-node attribution before scraping: a cycle where every scrape
	// fails must still report a liveness state, otherwise a multi-node cluster
	// with a broken proxy path would read as "not polled yet". The claim is
	// about the shape of the path (each node has its own answer), not about
	// this cycle's luck.
	c.setPerNodeAttribution(true)

	attempted, succeeded := 0, 0
	var lastErr error
	for _, target := range targets {
		attempted++
		chips, err := c.fetchChipsFromPod(ctx, target.pod)
		if err != nil {
			lastErr = err
			c.logger.Warn("Failed to scrape NPU-Exporter pod, keeping the rest of the cluster",
				zap.String("pod", target.pod.Name),
				zap.String("node", target.node.Name),
				zap.Error(err),
			)
			continue
		}
		succeeded++
		c.enrichSingleNode(target.node, chips)
	}

	// The backoff is a cluster-wide state, so only a cycle in which every
	// attempted scrape failed may arm it: one flapping node must not silence
	// the chip metrics of the nodes that answered.
	switch {
	case succeeded > 0:
		c.recordSuccess()
	case attempted > 0 && len(npuNodes) == 1:
		// A credential that may list pods but may not proxy them (the legacy
		// grant is the service proxy, a separate verb) fails every scrape
		// here. On a single-NPU-node cluster the service answer is
		// attributable to that one node just as well, and it is exactly the
		// path such credentials have always been able to use — so hand the
		// cycle over before arming the backoff, otherwise the failure just
		// recorded would silence the service request that follows through
		// the backoff check it starts with. Returning false lets
		// EnrichNodeData run enrichViaService, whose own bookkeeping then
		// owns the cycle's success or failure.
		c.logger.Debug("Every per-pod scrape failed on a single-NPU-node cluster, falling back to the service endpoint",
			zap.Int("attempted", attempted),
		)
		return false
	case attempted > 0:
		c.recordFailure(lastErr)
	}

	c.logger.Debug("Enriched NPU nodes from per-pod scrapes",
		zap.Int("npu_nodes", len(npuNodes)),
		zap.Int("attempted", attempted),
		zap.Int("succeeded", succeeded),
	)

	return true
}

// enrichViaService is the legacy path: one answer from the ClusterIP Service
// (or a custom endpoint), usable only when the cluster has one NPU node.
func (c *NPUExporterClient) enrichViaService(ctx context.Context, npuNodes []*model.NodeData) error {
	c.setPerNodeAttribution(false)

	// NPU-Exporter is reached through a ClusterIP Service, so the response comes
	// from a single exporter pod on a single node and carries no node identity.
	// Spreading those chips over several nodes would fabricate per-node data,
	// so only a single-NPU-node cluster is enriched.
	if len(npuNodes) > 1 {
		c.logger.Warn("metrics from single exporter cannot be attributed to specific nodes, skipping per-node enrichment",
			zap.Int("npu_nodes", len(npuNodes)),
		)
		return nil
	}

	// Create timeout context to prevent blocking the refresh cycle
	timeoutCtx, cancel := context.WithTimeout(ctx, npuExporterTimeout)
	defer cancel()

	var chipMetrics map[int]*NPUChipMetrics
	var npuCount int
	var err error

	if c.isAvailable() {
		chipMetrics, npuCount, err = c.GetMetrics(timeoutCtx)
		if err != nil {
			c.logger.Warn("Failed to get NPU metrics", zap.Error(err))
			return nil // Don't fail, just skip enrichment
		}
	} else {
		// The availability probe returns the metrics of the same request, so
		// the first refresh does not fetch them twice.
		chipMetrics, npuCount, err = c.checkAvailability(timeoutCtx)
		if err != nil {
			c.logger.Debug("NPU-Exporter not available, skipping enrichment", zap.Error(err))
			return nil // Not an error, just skip enrichment
		}
	}

	c.logger.Debug("Got NPU metrics from NPU-Exporter",
		zap.Int("npu_count", npuCount),
		zap.Int("chip_count", len(chipMetrics)),
	)

	c.enrichSingleNode(npuNodes[0], sortedChips(chipMetrics))

	return nil
}

// attributePods pairs discovered exporter pods with the NPU nodes they report
// on. A pod without a node assignment, or on a node without NPU capacity, is
// skipped; when several pods run on the same node (docker and containerd
// variants side by side) the first by pod name wins, so the choice is stable
// across refreshes.
func (c *NPUExporterClient) attributePods(pods []npuExporterPod, npuNodes []*model.NodeData) []npuExporterTarget {
	byName := make(map[string]*model.NodeData, len(npuNodes))
	for _, node := range npuNodes {
		byName[node.Name] = node
	}

	sorted := make([]npuExporterPod, len(pods))
	copy(sorted, pods)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	targets := make([]npuExporterTarget, 0, len(sorted))
	chosen := make(map[string]string, len(sorted)) // node name -> pod scraped for it
	for _, pod := range sorted {
		if pod.NodeName == "" {
			c.logger.Debug("NPU-Exporter pod has no node assignment, skipping", zap.String("pod", pod.Name))
			continue
		}
		node, ok := byName[pod.NodeName]
		if !ok {
			c.logger.Debug("NPU-Exporter pod runs on a node without NPU capacity, skipping",
				zap.String("pod", pod.Name), zap.String("node", pod.NodeName),
			)
			continue
		}
		if first, dup := chosen[pod.NodeName]; dup {
			c.logger.Debug("Node already covered by another NPU-Exporter pod, skipping duplicate",
				zap.String("pod", pod.Name), zap.String("node", pod.NodeName), zap.String("kept_pod", first),
			)
			continue
		}
		chosen[pod.NodeName] = pod.Name
		targets = append(targets, npuExporterTarget{pod: pod, node: node})
	}

	return targets
}

// fetchChipsFromPod scrapes one exporter pod and parses its chips.
func (c *NPUExporterClient) fetchChipsFromPod(ctx context.Context, pod npuExporterPod) ([]*NPUChipMetrics, error) {
	// Each pod gets its own request timeout, so one hung pod cannot stall the
	// rest of the nodes beyond the same bound the service path uses.
	reqCtx, cancel := context.WithTimeout(ctx, npuExporterTimeout)
	defer cancel()

	body, err := c.podFetcher(reqCtx, pod)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	chipMetrics, npuCount, err := c.parseMetrics(body)
	if err != nil {
		return nil, err
	}

	c.logger.Debug("Got NPU metrics from NPU-Exporter pod",
		zap.String("pod", pod.Name),
		zap.String("node", pod.NodeName),
		zap.Int("npu_count", npuCount),
		zap.Int("chip_count", len(chipMetrics)),
	)

	return sortedChips(chipMetrics), nil
}

// sortedChips converts the chip map to a slice ordered by chip ID, the order
// the node enrichment consumes.
func sortedChips(chipMetrics map[int]*NPUChipMetrics) []*NPUChipMetrics {
	chips := make([]*NPUChipMetrics, 0, len(chipMetrics))
	for _, chip := range chipMetrics {
		chips = append(chips, chip)
	}
	sort.Slice(chips, func(i, j int) bool {
		return chips[i].ID < chips[j].ID
	})
	return chips
}

// recordECCSamples appends this refresh's cumulative ECC counters to each
// chip's sliding window and returns the session delta per chip: newest sample
// minus oldest sample of the window, zero while fewer than two samples exist.
// A counter that went backwards (chip reset, driver reload, exporter restart)
// clears the window first and keeps only the new sample, the same reset
// handling the UI's rate trends use, so the delta never goes negative.
//
// nodeName scopes the windows: every node numbers its chips from 0, so the
// windows are keyed by (node, chip) and one node's counters can never land in
// another node's window. The returned map is keyed by chip ID alone because a
// single scrape's answer holds at most one entry per chip ID.
func (c *NPUExporterClient) recordECCSamples(nodeName string, chips []*NPUChipMetrics, now time.Time) map[int]eccSampleDelta {
	deltas := make(map[int]eccSampleDelta, len(chips))

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.eccSamples == nil {
		c.eccSamples = make(map[eccChipKey][]eccSample, len(chips))
	}

	for _, chip := range chips {
		key := eccChipKey{node: nodeName, id: chip.ID}
		sample := eccSample{at: now, single: chip.HBMEccSingleBitErrTotal, double: chip.HBMEccDoubleBitErrTotal}
		window := c.eccSamples[key]
		if n := len(window); n > 0 {
			last := window[n-1]
			if sample.single < last.single || sample.double < last.double {
				window = nil
			}
		}
		window = append(window, sample)
		if len(window) > eccSampleWindow {
			window = window[len(window)-eccSampleWindow:]
		}
		c.eccSamples[key] = window

		delta := eccSampleDelta{samples: len(window)}
		if len(window) >= 2 {
			delta.single = window[len(window)-1].single - window[0].single
			delta.double = window[len(window)-1].double - window[0].double
		}
		deltas[chip.ID] = delta
	}

	return deltas
}

// eccSampleDelta is the session increment derived from one chip's window.
type eccSampleDelta struct {
	single  int64
	double  int64
	samples int
}

// enrichSingleNode assigns all chip metrics to a single node
func (c *NPUExporterClient) enrichSingleNode(node *model.NodeData, chips []*NPUChipMetrics) {
	if len(chips) == 0 {
		return
	}

	var totalUtil, totalTemp, totalPower float64
	var totalHBMUsed, totalHBMTotal int64
	var healthyCount, unhealthyCount int

	deltas := c.recordECCSamples(node.Name, chips, time.Now())

	nodeChips := make([]model.NPUChipData, 0, len(chips))
	for _, chip := range chips {
		chipData := c.convertChipMetrics(chip)
		if delta, ok := deltas[chip.ID]; ok {
			chipData.HBMEccSingleErrDelta = delta.single
			chipData.HBMEccDoubleErrDelta = delta.double
			chipData.HBMEccDeltaSamples = delta.samples
		}
		nodeChips = append(nodeChips, chipData)

		// Only a chip the exporter explicitly reported as unhealthy (0) counts
		// against the node's health here. A missing metric or -1 (DCMI failure)
		// is unknown, and unknown is not evidence of a fault: those chips are
		// surfaced by the UI's "unknown health" row instead of being painted
		// red.
		if chip.HealthStatusKnown && chip.HealthStatus == 1 {
			healthyCount++
		} else if chip.HealthStatusKnown && chip.HealthStatus == 0 {
			unhealthyCount++
		}

		totalUtil += chip.Utilization
		totalTemp += float64(chip.Temperature)
		totalPower += chip.Power
		totalHBMUsed += chip.HBMUsedMemory
		totalHBMTotal += chip.HBMTotalMemory
	}

	chipCount := len(chips)
	node.NPUChips = nodeChips
	node.NPUUtilization = totalUtil / float64(chipCount)
	node.NPUMemoryTotal = totalHBMTotal * 1024 * 1024
	node.NPUMemoryUsed = totalHBMUsed * 1024 * 1024
	if totalHBMTotal > 0 {
		node.NPUMemoryUtil = float64(totalHBMUsed) / float64(totalHBMTotal) * 100
	}
	node.NPUTemperature = int(totalTemp / float64(chipCount))
	node.NPUPower = int(totalPower)
	node.NPUMetricsTime = time.Now()

	if unhealthyCount > 0 {
		node.NPUHealthStatus = "Warning"
		node.NPUErrorCount = unhealthyCount
	} else if healthyCount == 0 {
		// Every chip's health_status was missing or -1: the node aggregate is
		// "Unknown" rather than a healthy claim nobody made.
		node.NPUHealthStatus = "Unknown"
	} else {
		node.NPUHealthStatus = "Healthy"
	}

	c.logger.Debug("Enriched single node with NPU metrics",
		zap.String("node", node.Name),
		zap.Float64("utilization", node.NPUUtilization),
		zap.Int("chips", len(node.NPUChips)),
	)
}

// convertChipMetrics converts NPUChipMetrics to model.NPUChipData
func (c *NPUExporterClient) convertChipMetrics(chip *NPUChipMetrics) model.NPUChipData {
	chipData := model.NPUChipData{
		NPUID:    chip.ID / chipsPerNPU, // Each NPU has chipsPerNPU chips
		Chip:     chip.ID % chipsPerNPU,
		PhyID:    chip.ID,
		BusID:    chip.PCIeBusInfo,
		AICore:   int(chip.Utilization),
		Temp:     chip.Temperature,
		Power:    chip.Power,
		HBMUsed:  chip.HBMUsedMemory,
		HBMTotal: chip.HBMTotalMemory,

		// Extended metrics from NPU-Exporter
		VectorUtil:           chip.VectorUtilization,
		Voltage:              chip.Voltage,
		AICoreFreq:           chip.AICoreCurrentFreq,
		LinkStatus:           chip.LinkStatus,
		LinkStatusKnown:      chip.LinkStatusKnown,
		LinkSpeed:            chip.LinkSpeed,
		LinkUpNum:            chip.LinkUpNum,
		NetworkStatus:        chip.NetworkStatus,
		ErrorCode:            chip.ErrorCode,
		HBMEccSingleErr:      chip.HBMEccSingleBitErr,
		HBMEccDoubleErr:      chip.HBMEccDoubleBitErr,
		HBMEccSingleErrTotal: chip.HBMEccSingleBitErrTotal,
		HBMEccDoubleErrTotal: chip.HBMEccDoubleBitErrTotal,
		RoCETxPkts:           chip.RoCETxAllPktNum,
		RoCERxPkts:           chip.RoCERxAllPktNum,
		RoCETxErrPkts:        chip.RoCETxErrPktNum,
		RoCERxErrPkts:        chip.RoCERxErrPktNum,
		BandwidthRx:          chip.BandwidthRx,
		BandwidthTx:          chip.BandwidthTx,

		// Ownership as attributed by the exporter's container mapping. It
		// emits empty strings for an unallocated chip, so the zero value
		// carries over and omitempty drops the keys in the export.
		OwnerPod:       chip.PodName,
		OwnerNamespace: chip.Namespace,
	}

	// health_status is three-valued upstream (1 healthy, 0 unhealthy, -1 DCMI
	// call failed) and absent from some exporter versions; only a reported 1 or
	// 0 says anything about the chip. A missing metric and -1 both mean "not
	// known", which must not paint the chip as faulty.
	switch {
	case chip.HealthStatusKnown && chip.HealthStatus == 1:
		chipData.Health = "OK"
	case chip.HealthStatusKnown && chip.HealthStatus == 0:
		chipData.Health = "Error"
	default:
		chipData.Health = "Unknown"
	}

	if category, ok := npuErrorCodeInfo(int64(chip.ErrorCode)); ok {
		chipData.ErrorCodeFamily = category.Family
	}

	return chipData
}

// Close cleans up resources
func (c *NPUExporterClient) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// Name returns the data source name
func (c *NPUExporterClient) Name() string {
	return "NPUExporter"
}
