package datasource

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
)

const (
	kubeletPort        = 10250
	summaryAPIPath     = "/stats/summary"
	defaultHTTPTimeout = 3 * time.Second // Reduced from 10s to 3s for faster failover
)

// KubeletClient implements MetricsSource using kubelet Summary API
type KubeletClient struct {
	httpClient *http.Client
	config     *rest.Config
	logger     *zap.Logger
	useProxy   bool // true: use API Server proxy, false: direct access
	insecure   bool // true: skip TLS verification
}

// NewKubeletClient creates a new kubelet client
// If useProxy is true, it will access kubelet through API Server proxy
// If useProxy is false, it will try direct access to kubelet (requires proper certificates)
// If insecure is true, it will skip TLS certificate verification
func NewKubeletClient(config *rest.Config, useProxy, insecure bool, logger *zap.Logger) (*KubeletClient, error) {
	var httpClient *http.Client

	if useProxy {
		// When using API Server proxy, use the config's transport for authentication
		// This handles all auth types: client certs, bearer tokens, exec auth, etc.
		transport, err := rest.TransportFor(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create transport from config: %w", err)
		}

		httpClient = &http.Client{
			Timeout:   defaultHTTPTimeout,
			Transport: transport,
		}

		logger.Info("Kubelet client using API Server proxy with kubeconfig auth")
	} else {
		// Direct access to kubelet (requires TLS configuration)
		httpClient = &http.Client{
			Timeout: defaultHTTPTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: insecure,
				},
			},
		}

		logger.Info("Kubelet client using direct access",
			zap.Bool("insecure", insecure))
	}

	client := &KubeletClient{
		httpClient: httpClient,
		config:     config,
		logger:     logger,
		useProxy:   useProxy,
		insecure:   insecure,
	}

	logger.Info("Kubelet client initialized",
		zap.Bool("use_proxy", useProxy),
		zap.Bool("insecure", insecure),
	)

	return client, nil
}

// GetNodeMetrics retrieves CPU/Memory/Network metrics for a node
func (c *KubeletClient) GetNodeMetrics(ctx context.Context, nodeName string) (cpuMillicores int64, memoryBytes int64, networkRxBytes int64, networkTxBytes int64, networkTimestamp time.Time, err error) {
	c.logger.Debug("Fetching node metrics from kubelet",
		zap.String("node", nodeName),
		zap.Bool("use_proxy", c.useProxy),
	)

	summary, err := c.fetchSummary(ctx, nodeName)
	if err != nil {
		return 0, 0, 0, 0, time.Time{}, fmt.Errorf("failed to fetch summary: %w", err)
	}

	// Extract node-level metrics
	if summary.Node.CPU != nil && summary.Node.CPU.UsageNanoCores != nil {
		cpuMillicores = int64(*summary.Node.CPU.UsageNanoCores / 1000000) // nanocores to millicores
	}

	if summary.Node.Memory != nil && summary.Node.Memory.WorkingSetBytes != nil {
		memoryBytes = int64(*summary.Node.Memory.WorkingSetBytes)
	}

	// Extract network metrics and timestamp
	if summary.Node.Network != nil {
		// Use kubelet-provided timestamp, fallback to current time if unavailable
		networkTimestamp = summary.Node.Network.Time
		if networkTimestamp.IsZero() {
			networkTimestamp = time.Now()
		}

		if summary.Node.Network.Interfaces != nil {
			for _, iface := range summary.Node.Network.Interfaces {
				if iface.RxBytes != nil {
					networkRxBytes += int64(*iface.RxBytes)
				}
				if iface.TxBytes != nil {
					networkTxBytes += int64(*iface.TxBytes)
				}
			}
		}

		// If interface aggregation yielded zero (some environments only expose top-level counters),
		// fall back to the kubelet-provided totals.
		if networkRxBytes == 0 && summary.Node.Network.RxBytes != nil {
			networkRxBytes = int64(*summary.Node.Network.RxBytes)
		}
		if networkTxBytes == 0 && summary.Node.Network.TxBytes != nil {
			networkTxBytes = int64(*summary.Node.Network.TxBytes)
		}
	}

	c.logger.Debug("Node metrics fetched successfully",
		zap.String("node", nodeName),
		zap.Int64("cpu_millicores", cpuMillicores),
		zap.Int64("memory_bytes", memoryBytes),
		zap.Int64("network_rx_bytes", networkRxBytes),
		zap.Int64("network_tx_bytes", networkTxBytes),
		zap.Time("network_timestamp", networkTimestamp),
	)

	return cpuMillicores, memoryBytes, networkRxBytes, networkTxBytes, networkTimestamp, nil
}

// GetPodMetrics retrieves CPU/Memory metrics for a pod
func (c *KubeletClient) GetPodMetrics(ctx context.Context, namespace, podName string) (cpuMillicores int64, memoryBytes int64, err error) {
	c.logger.Debug("Fetching pod metrics from kubelet",
		zap.String("namespace", namespace),
		zap.String("pod", podName),
	)

	// We need to know which node the pod is on
	// This should be provided by the caller or we need to query API Server first
	// For now, we'll return an error indicating this limitation
	return 0, 0, fmt.Errorf("GetPodMetrics requires node name - use GetAllPodMetrics instead")
}

// GetAllPodMetricsOnNode retrieves metrics for all pods on a specific node
func (c *KubeletClient) GetAllPodMetricsOnNode(ctx context.Context, nodeName string) (map[string]*model.PodData, error) {
	c.logger.Debug("Fetching all pod metrics on node",
		zap.String("node", nodeName),
	)

	summary, err := c.fetchSummary(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch summary: %w", err)
	}

	// Build map of pod metrics
	podMetrics := make(map[string]*model.PodData)
	for _, pod := range summary.Pods {
		key := fmt.Sprintf("%s/%s", pod.PodRef.Namespace, pod.PodRef.Name)

		var cpuMillicores int64
		var memoryBytes int64
		var rxBytes int64
		var txBytes int64
		var networkTimestamp time.Time

		if pod.CPU != nil && pod.CPU.UsageNanoCores != nil {
			cpuMillicores = int64(*pod.CPU.UsageNanoCores / 1000000)
		}

		if pod.Memory != nil && pod.Memory.WorkingSetBytes != nil {
			memoryBytes = int64(*pod.Memory.WorkingSetBytes)
		}

		if pod.Network != nil {
			// Use kubelet-provided timestamp, fallback to current time if unavailable
			networkTimestamp = pod.Network.Time
			if networkTimestamp.IsZero() {
				networkTimestamp = time.Now()
			}

			if pod.Network.Interfaces != nil {
				for _, iface := range pod.Network.Interfaces {
					if iface.RxBytes != nil {
						rxBytes += int64(*iface.RxBytes)
					}
					if iface.TxBytes != nil {
						txBytes += int64(*iface.TxBytes)
					}
				}
			}

			// Fallback to top-level counters only if interface aggregation yielded zero.
			if rxBytes == 0 && pod.Network.RxBytes != nil {
				rxBytes = int64(*pod.Network.RxBytes)
			}
			if txBytes == 0 && pod.Network.TxBytes != nil {
				txBytes = int64(*pod.Network.TxBytes)
			}
		}

		podData := &model.PodData{
			Name:             pod.PodRef.Name,
			Namespace:        pod.PodRef.Namespace,
			CPUUsage:         cpuMillicores,
			MemoryUsage:      memoryBytes,
			NetworkRxBytes:   rxBytes,
			NetworkTxBytes:   txBytes,
			NetworkTimestamp: networkTimestamp,
		}

		// Extract container-level metrics
		podData.ContainerStates = make([]model.ContainerState, 0, len(pod.Containers))
		for _, container := range pod.Containers {
			var containerCPU int64
			var containerMem int64

			if container.CPU != nil && container.CPU.UsageNanoCores != nil {
				containerCPU = int64(*container.CPU.UsageNanoCores / 1000000)
			}

			if container.Memory != nil && container.Memory.WorkingSetBytes != nil {
				containerMem = int64(*container.Memory.WorkingSetBytes)
			}

			containerState := model.ContainerState{
				Name:        container.Name,
				CPUUsage:    containerCPU,
				MemoryUsage: containerMem,
			}

			podData.ContainerStates = append(podData.ContainerStates, containerState)
		}

		podMetrics[key] = podData
	}

	c.logger.Debug("Pod metrics fetched successfully",
		zap.String("node", nodeName),
		zap.Int("pod_count", len(podMetrics)),
	)

	return podMetrics, nil
}

// fetchSummary fetches the summary from kubelet
func (c *KubeletClient) fetchSummary(ctx context.Context, nodeName string) (*KubeletSummary, error) {
	var url string
	var req *http.Request
	var err error

	if c.useProxy {
		// Access through API Server proxy
		url = fmt.Sprintf("%s/api/v1/nodes/%s/proxy%s", c.config.Host, nodeName, summaryAPIPath)
		req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Authentication is handled by the Transport (created from rest.Config)
		// which supports all auth types: client certs, bearer tokens, exec auth, etc.
	} else {
		// Direct access to kubelet
		// Note: This requires the node's IP address, which we should get from NodeData
		// For now, we'll return an error
		return nil, fmt.Errorf("direct kubelet access not yet implemented - use proxy mode")
	}

	c.logger.Debug("Fetching kubelet summary",
		zap.String("url", url),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kubelet returned status %d: %s", resp.StatusCode, string(body))
	}

	var summary KubeletSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("failed to decode summary: %w", err)
	}

	return &summary, nil
}

// Name returns the metrics source name
func (c *KubeletClient) Name() string {
	if c.useProxy {
		return "KubeletProxy"
	}
	return "KubeletDirect"
}

// Close cleans up resources
func (c *KubeletClient) Close() error {
	c.logger.Info("Closing kubelet client")
	c.httpClient.CloseIdleConnections()
	return nil
}
