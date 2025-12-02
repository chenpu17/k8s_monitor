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
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NPUExporterClient provides access to NPU metrics from NPU-Exporter
type NPUExporterClient struct {
	restClient      rest.Interface
	httpClient      *http.Client
	serviceEndpoint string // Custom endpoint override (e.g., "http://localhost:8082")
	useProxy        bool   // Whether to use K8s API proxy (default true)
	serviceName     string // NPU-Exporter service name
	serviceNS       string // NPU-Exporter namespace
	servicePort     string // NPU-Exporter port
	logger          *zap.Logger
	available       bool
}

// NPUChipMetrics represents detailed NPU chip metrics from NPU-Exporter
type NPUChipMetrics struct {
	ID                int
	ModelName         string
	PCIeBusInfo       string
	VdieID            string
	PodName           string
	Namespace         string
	ContainerName     string
	Utilization       float64 // AI Core utilization %
	VectorUtilization float64 // Vector utilization %
	HBMTotalMemory    int64   // bytes
	HBMUsedMemory     int64   // bytes
	Temperature       int     // Celsius
	Power             float64 // Watts
	HealthStatus      int     // 1 = healthy, 0 = unhealthy
	AICoreCurrentFreq int     // MHz
	BandwidthRx       float64 // MB/s
	BandwidthTx       float64 // MB/s
	Voltage           float64 // V
	LinkStatus        int     // 1 = up, 0 = down
	LinkSpeed         int     // link speed
	LinkUpNum         int     // number of links up
	NetworkStatus     int
	ErrorCode         int
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
	c.serviceEndpoint = endpoint
	c.useProxy = false // Use direct HTTP when custom endpoint is provided
	c.logger.Info("NPU-Exporter using custom endpoint", zap.String("endpoint", endpoint))
}

// CheckAvailability checks if NPU-Exporter is available
func (c *NPUExporterClient) CheckAvailability(ctx context.Context) error {
	_, _, err := c.GetMetrics(ctx)
	if err != nil {
		c.available = false
		return err
	}
	c.available = true
	return nil
}

// IsAvailable returns whether NPU-Exporter is available
func (c *NPUExporterClient) IsAvailable() bool {
	return c.available
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
func (c *NPUExporterClient) fetchMetricsDirect(ctx context.Context) (io.ReadCloser, error) {
	url := c.serviceEndpoint + "/metrics"

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
	var body io.ReadCloser
	var err error

	if c.useProxy {
		body, err = c.fetchMetricsViaProxy(ctx)
	} else {
		body, err = c.fetchMetricsDirect(ctx)
	}

	if err != nil {
		c.available = false
		return nil, 0, err
	}
	defer body.Close()

	c.available = true
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
	if !c.available {
		if err := c.CheckAvailability(ctx); err != nil {
			c.logger.Debug("NPU-Exporter not available, skipping enrichment", zap.Error(err))
			return nil // Not an error, just skip enrichment
		}
	}

	chipMetrics, npuCount, err := c.GetMetrics(ctx)
	if err != nil {
		c.logger.Warn("Failed to get NPU metrics", zap.Error(err))
		return nil // Don't fail, just skip enrichment
	}

	c.logger.Debug("Got NPU metrics from NPU-Exporter",
		zap.Int("npu_count", npuCount),
		zap.Int("chip_count", len(chipMetrics)),
	)

	// For now, we enrich all NPU nodes with the aggregated metrics
	// In a multi-node setup, we would need to match chips to nodes
	// based on pod_name/namespace or other labels
	for _, node := range nodes {
		if node.NPUCapacity <= 0 {
			continue // Skip non-NPU nodes
		}

		// Aggregate chip metrics for this node
		var totalUtil, totalVecUtil, totalTemp, totalPower float64
		var totalHBMUsed, totalHBMTotal int64
		var healthyCount, unhealthyCount int
		var chipCount int

		// Collect chips for this node
		nodeChips := make([]model.NPUChipData, 0)
		for _, chip := range chipMetrics {
			// In a single-node scenario or if we can't determine node ownership,
			// assign all chips to NPU nodes
			// TODO: In multi-node, match based on node IP or pod allocation

			chipData := model.NPUChipData{
				NPUID:    chip.ID / 2, // Typically 2 chips per NPU
				Chip:     chip.ID % 2,
				PhyID:    chip.ID,
				BusID:    chip.PCIeBusInfo,
				AICore:   int(chip.Utilization),
				Temp:     chip.Temperature,
				Power:    chip.Power,
				HBMUsed:  chip.HBMUsedMemory,  // Already in MB from NPU-Exporter
				HBMTotal: chip.HBMTotalMemory, // Already in MB from NPU-Exporter

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
				healthyCount++
			} else {
				chipData.Health = "Error"
				unhealthyCount++
			}

			nodeChips = append(nodeChips, chipData)

			totalUtil += chip.Utilization
			totalVecUtil += chip.VectorUtilization
			totalTemp += float64(chip.Temperature)
			totalPower += chip.Power
			totalHBMUsed += chip.HBMUsedMemory
			totalHBMTotal += chip.HBMTotalMemory
			chipCount++
		}

		// Distribute chips across NPU nodes (simple round-robin for now)
		// This is a simplification - in production you'd match based on topology
		if chipCount > 0 && len(nodeChips) > 0 {
			// Sort chips by PhyID for consistent display
			sort.Slice(nodeChips, func(i, j int) bool {
				return nodeChips[i].PhyID < nodeChips[j].PhyID
			})

			// Only assign chips to this node if it has NPU capacity
			node.NPUChips = nodeChips
			node.NPUUtilization = totalUtil / float64(chipCount)
			// Convert MB to bytes for node-level HBM totals (NPU-Exporter provides MB)
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

			c.logger.Debug("Enriched node with NPU metrics",
				zap.String("node", node.Name),
				zap.Float64("utilization", node.NPUUtilization),
				zap.Int("chips", len(node.NPUChips)),
			)
		}
	}

	return nil
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
