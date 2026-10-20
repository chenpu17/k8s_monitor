package ui

import (
	"fmt"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// recordMetricSnapshot records a snapshot of current metrics for trend calculation
func (m *Model) recordMetricSnapshot(data *model.ClusterData) {
	snapshot := MetricSnapshot{
		NodeMetrics: make(map[string]*NodeMetric),
		PodMetrics:  make(map[string]*PodMetric),
		Timestamp:   time.Now(),
	}

	// Record node metrics and accumulate cluster-wide NPU stats
	var clusterNPUCapacity, clusterNPUAllocated, clusterNPUAllocatable int64
	for _, node := range data.Nodes {
		// Use kubelet-provided timestamp if available, otherwise fallback to snapshot time
		ts := node.NetworkTimestamp
		if ts.IsZero() {
			ts = snapshot.Timestamp
		}
		snapshot.NodeMetrics[node.Name] = &NodeMetric{
			CPUUsage:       node.CPUUsage,
			MemoryUsage:    node.MemoryUsage,
			NetworkRxBytes: node.NetworkRxBytes,
			NetworkTxBytes: node.NetworkTxBytes,
			Timestamp:      ts,
			// NPU metrics
			NPUCapacity:    node.NPUCapacity,
			NPUAllocated:   node.NPUAllocated,
			NPUAllocatable: node.NPUAllocatable,
		}

		// Accumulate cluster-wide NPU stats
		clusterNPUCapacity += node.NPUCapacity
		clusterNPUAllocated += node.NPUAllocated
		clusterNPUAllocatable += node.NPUAllocatable
	}

	// Set cluster-wide NPU summary
	snapshot.ClusterNPUCapacity = clusterNPUCapacity
	snapshot.ClusterNPUAllocated = clusterNPUAllocated
	snapshot.ClusterNPUAllocatable = clusterNPUAllocatable

	// Record pod metrics
	for _, pod := range data.Pods {
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		// Use kubelet-provided timestamp if available, otherwise fallback to snapshot time
		ts := pod.NetworkTimestamp
		if ts.IsZero() {
			ts = snapshot.Timestamp
		}
		snapshot.PodMetrics[key] = &PodMetric{
			CPUUsage:       pod.CPUUsage,
			MemoryUsage:    pod.MemoryUsage,
			NetworkRxBytes: pod.NetworkRxBytes,
			NetworkTxBytes: pod.NetworkTxBytes,
			Timestamp:      ts,
		}
	}

	// Add to history
	m.metricHistory = append(m.metricHistory, snapshot)

	// Keep only last N snapshots
	if len(m.metricHistory) > m.maxHistory {
		m.metricHistory = m.metricHistory[1:]
	}
}

// calculateNodeCPUTrend calculates CPU trend for a node
func (m *Model) calculateNodeCPUTrend(nodeName string, currentCPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values (excluding the most recent snapshot which is current)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].NodeMetrics[nodeName]; ok {
			historicalValues = append(historicalValues, metric.CPUUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average of historical values
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (5% threshold)
	threshold := avg / 20 // 5%
	if threshold < 10 {
		threshold = 10 // Minimum threshold to avoid noise
	}

	if currentCPU > avg+threshold {
		return TrendUp
	} else if currentCPU < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// calculateNodeMemoryTrend calculates memory trend for a node
func (m *Model) calculateNodeMemoryTrend(nodeName string, currentMemory int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].NodeMetrics[nodeName]; ok {
			historicalValues = append(historicalValues, metric.MemoryUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (5% threshold)
	threshold := avg / 20
	if threshold < 1024*1024*10 { // 10MB minimum threshold
		threshold = 1024 * 1024 * 10
	}

	if currentMemory > avg+threshold {
		return TrendUp
	} else if currentMemory < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// calculatePodCPUTrend calculates CPU trend for a pod
func (m *Model) calculatePodCPUTrend(namespace, name string, currentCPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].PodMetrics[key]; ok {
			historicalValues = append(historicalValues, metric.CPUUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	threshold := avg / 20
	if threshold < 10 {
		threshold = 10
	}

	if currentCPU > avg+threshold {
		return TrendUp
	} else if currentCPU < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// calculatePodMemoryTrend calculates memory trend for a pod
func (m *Model) calculatePodMemoryTrend(namespace, name string, currentMemory int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		if metric, ok := m.metricHistory[i].PodMetrics[key]; ok {
			historicalValues = append(historicalValues, metric.MemoryUsage)
		}
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	threshold := avg / 20
	if threshold < 1024*1024*10 {
		threshold = 1024 * 1024 * 10
	}

	if currentMemory > avg+threshold {
		return TrendUp
	} else if currentMemory < avg-threshold {
		return TrendDown
	}
	return TrendStable
}

// renderTrendIndicator returns a visual indicator for a trend
func renderTrendIndicator(trend Trend) string {
	switch trend {
	case TrendUp:
		return StyleStatusNotReady.Render("↑")
	case TrendDown:
		return StyleStatusReady.Render("↓")
	default:
		return StyleTextMuted.Render("→")
	}
}

// getNodeCPUHistory returns CPU usage history for a node
func (m *Model) getNodeCPUHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			// Convert millicores to percentage if we have capacity info
			// For now just use the absolute value
			history = append(history, float64(metric.CPUUsage)/1000.0) // Convert to cores
		}
	}
	return history
}

// getNodeMemoryHistory returns memory usage history for a node
func (m *Model) getNodeMemoryHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			// Convert bytes to GiB
			history = append(history, float64(metric.MemoryUsage)/1024/1024/1024)
		}
	}
	return history
}

// getPodCPUHistory returns CPU usage history for a pod
func (m *Model) getPodCPUHistory(namespace, name string) []float64 {
	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.PodMetrics[key]; ok {
			// Convert millicores to cores
			history = append(history, float64(metric.CPUUsage)/1000.0)
		}
	}
	return history
}

// getPodMemoryHistory returns memory usage history for a pod
func (m *Model) getPodMemoryHistory(namespace, name string) []float64 {
	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.PodMetrics[key]; ok {
			// Convert bytes to MiB
			history = append(history, float64(metric.MemoryUsage)/1024/1024)
		}
	}
	return history
}

// getNodeNetworkRxHistory returns network RX rate history for a node (MB/s)
func (m *Model) getNodeNetworkRxHistory(nodeName string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].NodeMetrics[nodeName]
		currMetric, currOk := m.metricHistory[i].NodeMetrics[nodeName]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
				// Avoid negative rates (can happen if pod/node restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// getNodeNetworkTxHistory returns network TX rate history for a node (MB/s)
func (m *Model) getNodeNetworkTxHistory(nodeName string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].NodeMetrics[nodeName]
		currMetric, currOk := m.metricHistory[i].NodeMetrics[nodeName]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
				// Avoid negative rates (can happen if pod/node restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// getPodNetworkRxHistory returns network RX rate history for a pod (MB/s)
func (m *Model) getPodNetworkRxHistory(namespace, name string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].PodMetrics[key]
		currMetric, currOk := m.metricHistory[i].PodMetrics[key]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
				// Avoid negative rates (can happen if pod restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// getPodNetworkTxHistory returns network TX rate history for a pod (MB/s)
func (m *Model) getPodNetworkTxHistory(namespace, name string) []float64 {
	if len(m.metricHistory) < 2 {
		return []float64{}
	}

	key := fmt.Sprintf("%s/%s", namespace, name)
	var history []float64
	// Calculate rate from consecutive snapshots (bytes/sec converted to MB/s)
	for i := 1; i < len(m.metricHistory); i++ {
		prevMetric, prevOk := m.metricHistory[i-1].PodMetrics[key]
		currMetric, currOk := m.metricHistory[i].PodMetrics[key]

		if prevOk && currOk {
			// Use metric-specific timestamps for accurate rate calculation
			timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()
			// Fallback to snapshot timestamps if metric timestamps are invalid
			if timeDelta <= 0 {
				timeDelta = m.metricHistory[i].Timestamp.Sub(m.metricHistory[i-1].Timestamp).Seconds()
			}
			if timeDelta > 0 {
				bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
				// Avoid negative rates (can happen if pod restarts)
				if bytesDelta >= 0 {
					rateMBps := float64(bytesDelta) / timeDelta / 1024 / 1024
					history = append(history, rateMBps)
				} else {
					history = append(history, 0)
				}
			}
		}
	}
	return history
}

// calculatePodNetworkRxRate calculates the current RX rate for a pod (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculatePodNetworkRxRate(namespace, name string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	key := fmt.Sprintf("%s/%s", namespace, name)

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when pod has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].PodMetrics[key]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.PodMetrics[key]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/pod restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculatePodNetworkTxRate calculates the current TX rate for a pod (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculatePodNetworkTxRate(namespace, name string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	key := fmt.Sprintf("%s/%s", namespace, name)

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when pod has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].PodMetrics[key]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.PodMetrics[key]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/pod restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculateNodeNetworkRxRate calculates the current RX rate for a node (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculateNodeNetworkRxRate(nodeName string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when node has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].NodeMetrics[nodeName]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.NodeMetrics[nodeName]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkRxBytes - prevMetric.NetworkRxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/node restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculateNodeNetworkTxRate calculates the current TX rate for a node (MB/s)
// Uses 20-second time-based sliding window for stable measurements
func (m *Model) calculateNodeNetworkTxRate(nodeName string) float64 {
	if len(m.metricHistory) < 2 {
		return 0
	}

	// Find the most recent snapshot with different network bytes
	// This avoids reporting 0 rate when node has no activity between last 2 snapshots
	currIdx := len(m.metricHistory) - 1
	currMetric, currOk := m.metricHistory[currIdx].NodeMetrics[nodeName]
	if !currOk {
		return 0
	}

	// Use 20-second time window for stable bandwidth metrics
	const timeWindow = 20 * time.Second
	cutoffTime := m.metricHistory[currIdx].Timestamp.Add(-timeWindow)

	// Collect multiple valid rate measurements for averaging (sliding window)
	var validRates []float64

	// Search backwards within 20-second time window
	for prevIdx := currIdx - 1; prevIdx >= 0; prevIdx-- {
		prevSnapshot := m.metricHistory[prevIdx]

		// Stop if outside time window
		if prevSnapshot.Timestamp.Before(cutoffTime) {
			break
		}

		prevMetric, prevOk := prevSnapshot.NodeMetrics[nodeName]
		if !prevOk {
			continue
		}

		// Only calculate rate if bytes actually changed
		bytesDelta := currMetric.NetworkTxBytes - prevMetric.NetworkTxBytes
		if bytesDelta == 0 {
			continue // No change, try older snapshot
		}

		// Skip negative deltas (counter reset/node restart), try older snapshot
		if bytesDelta < 0 {
			continue
		}

		// Use metric-specific timestamps for accurate rate calculation
		timeDelta := currMetric.Timestamp.Sub(prevMetric.Timestamp).Seconds()

		if timeDelta <= 0 {
			continue
		}

		// Convert to MB/s and collect
		rate := float64(bytesDelta) / timeDelta / 1024 / 1024
		validRates = append(validRates, rate)
	}

	// Calculate average rate from valid samples
	if len(validRates) == 0 {
		return 0
	}

	var sum float64
	for _, rate := range validRates {
		sum += rate
	}
	return sum / float64(len(validRates))
}

// calculateClusterNetworkRate calculates cluster-wide network rate (RX, TX in MB/s)
// Sums up individual node rates calculated with sliding window average
func (m *Model) calculateClusterNetworkRate() (float64, float64) {
	if len(m.metricHistory) < 2 {
		return 0, 0
	}

	if m.clusterData == nil {
		return 0, 0
	}

	// Calculate cluster rate by summing individual node rates
	// This ensures consistency with what users see in Node view
	var totalRxRate, totalTxRate float64

	for _, node := range m.clusterData.Nodes {
		// Use the same calculation method as individual node display
		rxRate := m.calculateNodeNetworkRxRate(node.Name)
		txRate := m.calculateNodeNetworkTxRate(node.Name)

		totalRxRate += rxRate
		totalTxRate += txRate
	}

	return totalRxRate, totalTxRate
}

// getNodeNPUUtilizationHistory returns NPU utilization percentage history for a node
func (m *Model) getNodeNPUUtilizationHistory(nodeName string) []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if metric, ok := snapshot.NodeMetrics[nodeName]; ok {
			if metric.NPUAllocatable > 0 {
				util := float64(metric.NPUAllocated) / float64(metric.NPUAllocatable) * 100
				history = append(history, util)
			} else {
				history = append(history, 0)
			}
		}
	}
	return history
}

// getClusterNPUUtilizationHistory returns cluster-wide NPU utilization percentage history
func (m *Model) getClusterNPUUtilizationHistory() []float64 {
	var history []float64
	for _, snapshot := range m.metricHistory {
		if snapshot.ClusterNPUAllocatable > 0 {
			util := float64(snapshot.ClusterNPUAllocated) / float64(snapshot.ClusterNPUAllocatable) * 100
			history = append(history, util)
		} else {
			history = append(history, 0)
		}
	}
	return history
}

// calculateClusterNPUTrend calculates cluster-wide NPU allocation trend
func (m *Model) calculateClusterNPUTrend(currentNPU int64) Trend {
	if len(m.metricHistory) < 3 {
		return TrendStable // Not enough data
	}

	// Get historical values
	var historicalValues []int64
	for i := 0; i < len(m.metricHistory)-1; i++ {
		historicalValues = append(historicalValues, m.metricHistory[i].ClusterNPUAllocated)
	}

	if len(historicalValues) == 0 {
		return TrendStable
	}

	// Calculate average
	var sum int64
	for _, val := range historicalValues {
		sum += val
	}
	avg := sum / int64(len(historicalValues))

	// Determine trend (any change in NPU allocation is significant)
	if currentNPU > avg {
		return TrendUp
	} else if currentNPU < avg {
		return TrendDown
	}
	return TrendStable
}
