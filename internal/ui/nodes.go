package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderNodes renders the nodes view
func (m *Model) renderNodes() string {
	nodes := m.getFilteredNodes()
	if len(nodes) == 0 {
		return m.T("views.nodes.no_nodes")
	}

	// Sort nodes before rendering and cache
	m.cachedSortedNodes = m.getSortedNodes(nodes)

	// Header
	header := m.renderNodesHeader(m.cachedSortedNodes)

	// Node list
	nodeList := m.renderNodesList(m.cachedSortedNodes)

	// Footer with stats
	footer := m.renderNodesFooter()

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		nodeList,
		"",
		footer,
	)

	// Show search indicator if in search mode
	if m.searchMode {
		searchPanel := m.renderSearchPanel()
		return lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			searchPanel,
		)
	}

	return content
}

// renderNodesHeader renders the nodes view header
func (m *Model) renderNodesHeader(nodes []*model.NodeData) string {
	title := StyleHeader.Render("💻  " + m.T("views.nodes.title"))
	summary := fmt.Sprintf("%s: %d", m.T("common.total"), len(nodes))

	// Add sort indicator
	var sortInfo string
	switch m.sortField {
	case SortByName:
		sortInfo = m.T("columns.name")
	case SortByCPU:
		sortInfo = m.T("columns.cpu")
	case SortByMemory:
		sortInfo = m.T("columns.memory")
	case SortByPods:
		sortInfo = m.T("columns.pods")
	}
	if sortInfo != "" {
		arrow := "↑"
		if m.sortOrder == SortDesc {
			arrow = "↓"
		}
		summary += fmt.Sprintf(" • %s: %s %s", m.T("common.sort"), sortInfo, arrow)
	}

	headerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		StyleTextSecondary.Render(summary),
	)

	// Add NPU cluster-level summary if cluster has NPU nodes
	if m.clusterData != nil && m.clusterData.Summary != nil {
		clusterSummary := m.clusterData.Summary
		if clusterSummary.NPUCapacity > 0 {
			npuSummary := m.renderNPUClusterSummary(clusterSummary)
			if npuSummary != "" {
				return headerLine + "\n" + npuSummary
			}
		}
	}

	return headerLine
}

// renderNPUClusterSummary renders NPU cluster-level summary for nodes view
func (m *Model) renderNPUClusterSummary(summary *model.ClusterSummary) string {
	var parts []string

	// NPU allocation: allocated / total
	allocStyle := StyleStatusReady
	if summary.NPUUtilization >= 90 {
		allocStyle = StyleWarning
	} else if summary.NPUUtilization >= 100 {
		allocStyle = StyleStatusNotReady
	}
	parts = append(parts, fmt.Sprintf("%s: %s",
		m.T("npu.allocated"),
		allocStyle.Render(fmt.Sprintf("%d/%d", summary.NPUAllocated, summary.NPUAllocatable))))

	// NPU utilization percentage
	utilStyle := StyleStatusReady
	if summary.NPUUtilization < 50 {
		utilStyle = StyleTextMuted // Underutilizing
	} else if summary.NPUUtilization >= 90 {
		utilStyle = StyleWarning
	}
	parts = append(parts, fmt.Sprintf("%s: %s",
		m.T("npu.usage"),
		utilStyle.Render(formatPercentage(summary.NPUUtilization))))

	// NPU nodes count
	parts = append(parts, fmt.Sprintf("%s: %d",
		m.T("common.nodes"),
		summary.NPUNodesCount))

	// NPU chip type if available
	if summary.NPUChipType != "" {
		parts = append(parts, fmt.Sprintf("%s: %s",
			m.T("npu.chip_type"),
			StyleHighlight.Render(summary.NPUChipType)))
	}

	return StyleTextMuted.Render("  🧮 NPU: ") + strings.Join(parts, " • ")
}

// renderNodesList renders the list of nodes
func (m *Model) renderNodesList(nodes []*model.NodeData) string {
	var rows []string

	// Check if cluster has NPU nodes
	hasNPU := m.clusterHasNPU()

	// Table header - define fixed column widths
	const (
		colName   = 30
		colStatus = 12
		colRoles  = 15
		colCPU    = 18 // Increased to fit trend indicator
		colMemory = 23 // Increased to fit trend indicator
		colRx     = 11 // Network RX bandwidth
		colTx     = 11 // Network TX bandwidth
		colPods   = 10
		colNPU    = 12 // NPU usage column
	)

	var headerRow string
	var separatorWidth int
	if hasNPU {
		headerRow = fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colName),
			padRight(m.T("columns.status"), colStatus),
			padRight(m.T("columns.roles"), colRoles),
			padRight(m.T("columns.cpu"), colCPU),
			padRight(m.T("columns.memory"), colMemory),
			padRight("NPU", colNPU),
			padRight(m.T("columns.rx"), colRx),
			padRight(m.T("columns.tx"), colTx),
			padRight(m.T("columns.pods"), colPods),
		)
		separatorWidth = colName + colStatus + colRoles + colCPU + colMemory + colNPU + colRx + colTx + colPods + 16
	} else {
		headerRow = fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colName),
			padRight(m.T("columns.status"), colStatus),
			padRight(m.T("columns.roles"), colRoles),
			padRight(m.T("columns.cpu"), colCPU),
			padRight(m.T("columns.memory"), colMemory),
			padRight(m.T("columns.rx"), colRx),
			padRight(m.T("columns.tx"), colTx),
			padRight(m.T("columns.pods"), colPods),
		)
		separatorWidth = colName + colStatus + colRoles + colCPU + colMemory + colRx + colTx + colPods + 14
	}
	rows = append(rows, StyleHeader.Render(headerRow))
	rows = append(rows, strings.Repeat("─", separatorWidth))

	// Calculate visible range based on scroll
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalNodes := len(nodes)

	// Clamp scroll offset to valid range to prevent panic when node count shrinks
	maxScroll := totalNodes - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	startIdx := m.scrollOffset
	endIdx := startIdx + maxVisible
	if endIdx > totalNodes {
		endIdx = totalNodes
	}

	visibleNodes := nodes[startIdx:endIdx]

	// Node rows with selection highlighting
	for i, node := range visibleNodes {
		absoluteIndex := startIdx + i
		row := m.renderNodeRow(node, colName, colStatus, colRoles, colCPU, colMemory, colNPU, colRx, colTx, colPods, hasNPU)

		// Highlight selected row
		if absoluteIndex == m.selectedIndex {
			row = StyleSelected.Render(row)
		}

		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderNodeRow renders a single node row
func (m *Model) renderNodeRow(node *model.NodeData, colName, colStatus, colRoles, colCPU, colMemory, colNPU, colRx, colTx, colPods int, hasNPU bool) string {
	// Node name
	name := truncate(node.Name, colName)

	// Status with color
	status := RenderStatus(node.Status)

	// Roles
	roles := truncate(strings.Join(node.Roles, ","), colRoles)

	// CPU usage with trend
	var cpuUsage string
	if node.CPUUsage > 0 && node.CPUAllocatable > 0 {
		cpuTrend := m.calculateNodeCPUTrend(node.Name, node.CPUUsage)
		trendIndicator := renderTrendIndicator(cpuTrend)
		cpuUsage = fmt.Sprintf("%s/%s %s",
			FormatMillicores(node.CPUUsage),
			FormatMillicores(node.CPUAllocatable),
			trendIndicator,
		)
	} else {
		cpuUsage = "-"
	}
	cpuUsage = truncate(cpuUsage, colCPU)

	// Memory usage with trend
	var memUsage string
	if node.MemoryUsage > 0 && node.MemAllocatable > 0 {
		memTrend := m.calculateNodeMemoryTrend(node.Name, node.MemoryUsage)
		trendIndicator := renderTrendIndicator(memTrend)
		memUsage = fmt.Sprintf("%s/%s %s",
			FormatBytes(node.MemoryUsage),
			FormatBytes(node.MemAllocatable),
			trendIndicator,
		)
	} else {
		memUsage = "-"
	}
	memUsage = truncate(memUsage, colMemory)

	// NPU usage (only for nodes with NPU)
	var npuUsage string
	if hasNPU {
		if node.NPUCapacity > 0 {
			npuUsage = fmt.Sprintf("%d/%d", node.NPUAllocated, node.NPUAllocatable)
		} else {
			npuUsage = "-"
		}
	}

	// Network RX (download/receive)
	rxRate := m.calculateNodeNetworkRxRate(node.Name)
	rxStr := formatNetworkRate(rxRate)
	if rxRate == 0 {
		if node.HasKubeletMetrics && len(m.metricHistory) >= 2 {
			rxStr = StyleTextMuted.Render("0 B/s")
		} else {
			rxStr = StyleTextMuted.Render("-")
		}
	}

	// Network TX (upload/send)
	txRate := m.calculateNodeNetworkTxRate(node.Name)
	txStr := formatNetworkRate(txRate)
	if txRate == 0 {
		if node.HasKubeletMetrics && len(m.metricHistory) >= 2 {
			txStr = StyleTextMuted.Render("0 B/s")
		} else {
			txStr = StyleTextMuted.Render("-")
		}
	}

	// Pod count
	podCount := fmt.Sprintf("%d/%d", node.PodCount, node.PodAllocatable)

	if hasNPU {
		return fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s  %s",
			padRight(name, colName),
			padRight(status, colStatus),
			padRight(roles, colRoles),
			padRight(cpuUsage, colCPU),
			padRight(memUsage, colMemory),
			padRight(npuUsage, colNPU),
			padRight(rxStr, colRx),
			padRight(txStr, colTx),
			padRight(podCount, colPods),
		)
	}

	return fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
		padRight(name, colName),
		padRight(status, colStatus),
		padRight(roles, colRoles),
		padRight(cpuUsage, colCPU),
		padRight(memUsage, colMemory),
		padRight(rxStr, colRx),
		padRight(txStr, colTx),
		padRight(podCount, colPods),
	)
}

// renderNodesFooter renders the nodes view footer
func (m *Model) renderNodesFooter() string {
	summary := m.clusterData.Summary
	if summary == nil {
		return ""
	}

	totalNodes := len(m.clusterData.Nodes)
	stats := fmt.Sprintf("%s: %s  %s: %s  %s: %d",
		m.T("status.ready"),
		StyleStatusReady.Render(fmt.Sprintf("%d", summary.ReadyNodes)),
		m.T("status.not_ready"),
		StyleStatusNotReady.Render(fmt.Sprintf("%d", summary.NotReadyNodes)),
		m.T("common.total"),
		totalNodes,
	)

	// Add scroll position indicator if there are more items than visible
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}
	if totalNodes > maxVisible {
		scrollInfo := fmt.Sprintf("  [%d-%d %s %d]",
			m.scrollOffset+1,
			min(m.scrollOffset+maxVisible, totalNodes),
			m.T("common.of"),
			totalNodes,
		)
		stats += StyleTextMuted.Render(scrollInfo)
	}

	return StyleTextSecondary.Render(stats)
}

// getSortedNodes returns a sorted copy of nodes based on current sort settings
func (m *Model) getSortedNodes(inputNodes []*model.NodeData) []*model.NodeData {
	if len(inputNodes) == 0 {
		return []*model.NodeData{}
	}

	// Create a copy to avoid modifying original
	nodes := make([]*model.NodeData, len(inputNodes))
	copy(nodes, inputNodes)

	// Sort based on current field and order using sort.Slice (O(n log n))
	switch m.sortField {
	case SortByName:
		sort.Slice(nodes, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].Name > nodes[j].Name
		})

	case SortByCPU:
		sort.Slice(nodes, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return nodes[i].CPUUsage < nodes[j].CPUUsage
			}
			return nodes[i].CPUUsage > nodes[j].CPUUsage
		})

	case SortByMemory:
		sort.Slice(nodes, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return nodes[i].MemoryUsage < nodes[j].MemoryUsage
			}
			return nodes[i].MemoryUsage > nodes[j].MemoryUsage
		})

	case SortByPods:
		sort.Slice(nodes, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return nodes[i].PodCount < nodes[j].PodCount
			}
			return nodes[i].PodCount > nodes[j].PodCount
		})
	}

	return nodes
}
