package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderNodeDetail renders the node detail view
func (m *Model) renderNodeDetail() string {
	if m.selectedNode == nil {
		return m.T("detail.node.no_selected")
	}

	node := m.selectedNode

	// Collect all content lines
	var allLines []string

	// Node header
	allLines = append(allLines, m.renderNodeDetailHeader(node))
	allLines = append(allLines, "")

	// Node basic info
	basicInfo := m.renderNodeBasicInfo(node)
	allLines = append(allLines, strings.Split(basicInfo, "\n")...)
	allLines = append(allLines, "")

	// Node resource info
	resourceInfo := m.renderNodeResourceInfo(node)
	allLines = append(allLines, strings.Split(resourceInfo, "\n")...)
	allLines = append(allLines, "")

	// Pods running on this node
	podsInfo := m.renderNodePodsInfo(node)
	allLines = append(allLines, strings.Split(podsInfo, "\n")...)

	// Apply scroll offset
	maxVisible := m.height - 8 // Reserve space for header/footer
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalLines := len(allLines)

	// Clamp scroll offset to valid range (prevent scrolling beyond content)
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.detailScrollOffset > maxScroll {
		m.detailScrollOffset = maxScroll
	}
	if m.detailScrollOffset < 0 {
		m.detailScrollOffset = 0
	}

	startIdx := m.detailScrollOffset
	endIdx := startIdx + maxVisible
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator if needed
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render("\n" + m.TF("detail.scroll_indicator", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		}))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderNodeDetailHeader renders the node detail view header
func (m *Model) renderNodeDetailHeader(node *model.NodeData) string {
	title := StyleHeader.Render(fmt.Sprintf("🖥️  %s: %s", m.T("detail.node.title"), node.Name))
	status := RenderStatus(node.Status)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		status,
	)
}

// renderNodeBasicInfo renders basic node information
func (m *Model) renderNodeBasicInfo(node *model.NodeData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.node.basic_info")))
	info = append(info, "")
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.name")),
		node.Name))

	if len(node.Roles) > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("Roles"),
			strings.Join(node.Roles, ", ")))
	}

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.status")),
		RenderStatus(node.Status)))

	if node.InternalIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.internal_ip")),
			node.InternalIP))
	}

	if node.ExternalIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.external_ip")),
			node.ExternalIP))
	}

	return strings.Join(info, "\n")
}

// renderNodeResourceInfo renders node resource information
func (m *Model) renderNodeResourceInfo(node *model.NodeData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.node.resource_info")))
	info = append(info, "")

	// CPU
	if node.CPUAllocatable > 0 {
		cpuUsage := "N/A"
		cpuPercent := "N/A"
		if node.CPUUsage > 0 {
			cpuUsage = FormatMillicores(node.CPUUsage)
			cpuPercent = fmt.Sprintf("%.1f%%", float64(node.CPUUsage)*100.0/float64(node.CPUAllocatable))
		}
		info = append(info, fmt.Sprintf("  %s: %s / %s (%s)",
			StyleTextSecondary.Render(m.T("detail.field.cpu")),
			cpuUsage,
			FormatMillicores(node.CPUAllocatable),
			cpuPercent))
	}

	// Memory
	if node.MemAllocatable > 0 {
		memUsage := "N/A"
		memPercent := "N/A"
		if node.MemoryUsage > 0 {
			memUsage = FormatBytes(node.MemoryUsage)
			memPercent = fmt.Sprintf("%.1f%%", float64(node.MemoryUsage)*100.0/float64(node.MemAllocatable))
		}
		info = append(info, fmt.Sprintf("  %s: %s / %s (%s)",
			StyleTextSecondary.Render(m.T("detail.field.memory")),
			memUsage,
			FormatBytes(node.MemAllocatable),
			memPercent))
	}

	// Pods
	if node.PodAllocatable > 0 {
		podPercent := fmt.Sprintf("%.1f%%", float64(node.PodCount)*100.0/float64(node.PodAllocatable))
		info = append(info, fmt.Sprintf("  %s: %d / %d (%s)",
			StyleTextSecondary.Render(m.T("detail.field.pods")),
			node.PodCount,
			node.PodAllocatable,
			podPercent))
	}

	// Network Traffic (if available)
	if node.NetworkRxBytes > 0 || node.NetworkTxBytes > 0 {
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("detail.node.network_traffic")))

		rxStr := formatNetworkTraffic(node.NetworkRxBytes)
		txStr := formatNetworkTraffic(node.NetworkTxBytes)
		totalStr := formatNetworkTraffic(node.NetworkRxBytes + node.NetworkTxBytes)

		info = append(info, fmt.Sprintf("  %s: %s  %s: %s  Total: %s",
			StyleTextMuted.Render(m.T("detail.node.rx")),
			rxStr,
			StyleTextMuted.Render(m.T("detail.node.tx")),
			txStr,
			totalStr,
		))
	}

	// Historical trends (if available)
	if len(m.metricHistory) >= 2 {
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("detail.node.historical_trends")))

		cpuHistory := m.getNodeCPUHistory(node.Name)
		if len(cpuHistory) >= 2 {
			sparkline := RenderSparkline(cpuHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.cpu_label")),
				sparkline))
		}

		memHistory := m.getNodeMemoryHistory(node.Name)
		if len(memHistory) >= 2 {
			sparkline := RenderSparkline(memHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.memory_label")),
				sparkline))
		}

		// Network traffic trends
		rxHistory := m.getNodeNetworkRxHistory(node.Name)
		if len(rxHistory) >= 2 {
			sparkline := RenderSparkline(rxHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.network_rx_label")),
				sparkline))
		}

		txHistory := m.getNodeNetworkTxHistory(node.Name)
		if len(txHistory) >= 2 {
			sparkline := RenderSparkline(txHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.network_tx_label")),
				sparkline))
		}

		info = append(info, StyleTextMuted.Render("  "+m.TF("detail.node.snapshots", map[string]interface{}{
			"Count": len(m.metricHistory),
		})))
	}

	return strings.Join(info, "\n")
}

// renderNodePodsInfo renders pods running on this node
func (m *Model) renderNodePodsInfo(node *model.NodeData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("📦 Pods on this Node (%d)", node.PodCount)))
	info = append(info, "")

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleTextMuted.Render("  No pod information available"))
		return strings.Join(info, "\n")
	}

	// Filter pods running on this node
	nodePods := []*model.PodData{}
	for _, pod := range m.clusterData.Pods {
		if pod.Node == node.Name {
			nodePods = append(nodePods, pod)
		}
	}

	if len(nodePods) == 0 {
		info = append(info, StyleTextMuted.Render("  No pods running on this node"))
		return strings.Join(info, "\n")
	}

	// Table header
	headerRow := fmt.Sprintf("  %-35s %-20s %-15s %-10s",
		"NAME", "NAMESPACE", "STATUS", "RESTARTS")
	info = append(info, StyleTextSecondary.Render(headerRow))
	info = append(info, "  "+strings.Repeat("─", 85))

	// Pod rows (limited to visible area)
	maxVisible := m.height - 20
	if maxVisible < 1 {
		maxVisible = 1
	}

	visiblePods := nodePods
	if len(nodePods) > maxVisible {
		visiblePods = nodePods[:maxVisible]
	}

	for _, pod := range visiblePods {
		name := pod.Name
		if len(name) > 33 {
			name = name[:30] + "..."
		}

		namespace := pod.Namespace
		if len(namespace) > 18 {
			namespace = namespace[:15] + "..."
		}

		status := RenderStatus(pod.Phase)
		restarts := fmt.Sprintf("%d", pod.RestartCount)

		row := fmt.Sprintf("  %-35s %-20s %-23s %-10s",
			name,
			namespace,
			status,
			restarts)
		info = append(info, row)
	}

	if len(nodePods) > maxVisible {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render(fmt.Sprintf("  ... and %d more pods", len(nodePods)-maxVisible)))
	}

	return strings.Join(info, "\n")
}
