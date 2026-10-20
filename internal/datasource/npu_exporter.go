package datasource

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
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
	HealthStatus       int     // 1 = healthy, 0 = unhealthy
	AICoreCurrentFreq  int     // MHz
	BandwidthRx        float64 // MB/s
	BandwidthTx        float64 // MB/s
	Voltage            float64 // V
	LinkStatus         int     // 1 = up, 0 = down
	LinkSpeed          int     // link speed
	LinkUpNum          int     // number of links up
	NetworkStatus      int
	ErrorCode          int
	HBMEccSingleBitErr int64
	HBMEccDoubleBitErr int64
	RoCETxAllPktNum    int64
	RoCERxAllPktNum    int64
	RoCETxErrPktNum    int64
	RoCERxErrPktNum    int64
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

// inBackoff reports whether a recent failure should suppress new requests
func (c *NPUExporterClient) inBackoff() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.lastFailure.IsZero() && time.Since(c.lastFailure) < npuExporterRetryDelay
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
}

// recordFailure marks NPU-Exporter as unreachable and arms the retry backoff
func (c *NPUExporterClient) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.available = false
	c.lastFailure = time.Now()
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
		c.recordFailure()
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

// EnrichNodeData enriches node data with NPU metrics from NPU-Exporter
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

	// Convert chip metrics map to sorted slice for consistent assignment
	chips := make([]*NPUChipMetrics, 0, len(chipMetrics))
	for _, chip := range chipMetrics {
		chips = append(chips, chip)
	}
	sort.Slice(chips, func(i, j int) bool {
		return chips[i].ID < chips[j].ID
	})

	c.enrichSingleNode(npuNodes[0], chips)

	return nil
}

// enrichSingleNode assigns all chip metrics to a single node
func (c *NPUExporterClient) enrichSingleNode(node *model.NodeData, chips []*NPUChipMetrics) {
	if len(chips) == 0 {
		return
	}

	var totalUtil, totalTemp, totalPower float64
	var totalHBMUsed, totalHBMTotal int64
	var healthyCount, unhealthyCount int

	nodeChips := make([]model.NPUChipData, 0, len(chips))
	for _, chip := range chips {
		chipData := c.convertChipMetrics(chip)
		nodeChips = append(nodeChips, chipData)

		if chip.HealthStatus == 1 {
			healthyCount++
		} else {
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
		VectorUtil:      chip.VectorUtilization,
		Voltage:         chip.Voltage,
		AICoreFreq:      chip.AICoreCurrentFreq,
		LinkStatus:      chip.LinkStatus,
		LinkSpeed:       chip.LinkSpeed,
		LinkUpNum:       chip.LinkUpNum,
		NetworkStatus:   chip.NetworkStatus,
		ErrorCode:       chip.ErrorCode,
		HBMEccSingleErr: chip.HBMEccSingleBitErr,
		HBMEccDoubleErr: chip.HBMEccDoubleBitErr,
		RoCETxPkts:      chip.RoCETxAllPktNum,
		RoCERxPkts:      chip.RoCERxAllPktNum,
		RoCETxErrPkts:   chip.RoCETxErrPktNum,
		RoCERxErrPkts:   chip.RoCERxErrPktNum,
		BandwidthRx:     chip.BandwidthRx,
		BandwidthTx:     chip.BandwidthTx,
	}

	if chip.HealthStatus == 1 {
		chipData.Health = "OK"
	} else {
		chipData.Health = "Error"
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
