package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderPods renders the pods view
func (m *Model) renderPods() string {
	pods := m.getFilteredPods()
	if len(pods) == 0 {
		return m.T("views.pods.no_pods")
	}

	// Sort pods and cache
	m.cachedSortedPods = m.getSortedPods(pods)

	// Header
	header := m.renderPodsHeader(m.cachedSortedPods)

	// Pod list
	podList := m.renderPodsList(m.cachedSortedPods)

	// Footer with stats
	footer := m.renderPodsFooter(m.cachedSortedPods)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		podList,
		"",
		footer,
	)

	// Show filter panel if in filter mode
	if m.filterMode {
		filterPanel := m.renderFilterPanel()
		return lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			filterPanel,
		)
	}

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

// renderPodsHeader renders the pods view header
func (m *Model) renderPodsHeader(pods []*model.PodData) string {
	title := StyleHeader.Render("📦 " + m.T("views.pods.title"))
	summary := fmt.Sprintf("%s: %d", m.T("common.total"), len(pods))

	// Show filter info if active
	if m.filterNamespace != "" {
		summary += fmt.Sprintf(" (%s: %s)", m.T("common.filtered_by"), m.filterNamespace)
	}

	// Add sort indicator
	var sortInfo string
	switch m.sortField {
	case SortByName:
		sortInfo = m.T("columns.name")
	case SortByNamespace:
		sortInfo = m.T("columns.namespace")
	case SortByRestarts:
		sortInfo = m.T("columns.restarts")
	}
	if sortInfo != "" {
		arrow := "↑"
		if m.sortOrder == SortDesc {
			arrow = "↓"
		}
		summary += fmt.Sprintf(" • %s: %s %s", m.T("common.sort"), sortInfo, arrow)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		StyleTextSecondary.Render(summary),
	)
}

// renderPodsList renders the list of pods
func (m *Model) renderPodsList(pods []*model.PodData) string {
	var rows []string

	// Table header - define fixed column widths
	const (
		colName      = 26
		colNamespace = 14
		colStatus    = 11
		colCPU       = 12  // Trend indicator
		colMemory    = 15  // Trend indicator
		colRx        = 11  // Network RX
		colTx        = 11  // Network TX
		colRestarts  = 8
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.status"), colStatus),
		padRight(m.T("columns.cpu"), colCPU),
		padRight(m.T("columns.memory"), colMemory),
		padRight(m.T("columns.rx"), colRx),
		padRight(m.T("columns.tx"), colTx),
		padRight(m.T("columns.restarts"), colRestarts),
	)
	rows = append(rows, StyleHeader.Render(headerRow))
	rows = append(rows, strings.Repeat("─", colName+colNamespace+colStatus+colCPU+colMemory+colRx+colTx+colRestarts+14))

	// Calculate visible range based on scroll
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalPods := len(pods)

	// Clamp scroll offset to valid range to prevent panic when pod count shrinks
	maxScroll := totalPods - maxVisible
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
	if endIdx > totalPods {
		endIdx = totalPods
	}

	visiblePods := pods[startIdx:endIdx]

	// Pod rows with selection highlighting
	for i, pod := range visiblePods {
		absoluteIndex := startIdx + i
		row := m.renderPodRow(pod, colName, colNamespace, colStatus, colCPU, colMemory, colRx, colTx, colRestarts)

		// Highlight selected row
		if absoluteIndex == m.selectedIndex {
			row = StyleSelected.Render(row)
		}

		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderPodRow renders a single pod row
func (m *Model) renderPodRow(pod *model.PodData, colName, colNamespace, colStatus, colCPU, colMemory, colRx, colTx, colRestarts int) string {
	// Pod name
	name := truncate(pod.Name, colName)

	// Namespace
	namespace := truncate(pod.Namespace, colNamespace)

	// Status with color
	status := RenderStatus(pod.Phase)

	// CPU usage with trend
	var cpuUsage string
	if pod.CPUUsage > 0 {
		cpuTrend := m.calculatePodCPUTrend(pod.Namespace, pod.Name, pod.CPUUsage)
		trendIndicator := renderTrendIndicator(cpuTrend)
		cpuUsage = fmt.Sprintf("%s %s", FormatMillicores(pod.CPUUsage), trendIndicator)
	} else {
		cpuUsage = "-"
	}
	cpuUsage = truncate(cpuUsage, colCPU)

	// Memory usage with trend
	var memUsage string
	if pod.MemoryUsage > 0 {
		memTrend := m.calculatePodMemoryTrend(pod.Namespace, pod.Name, pod.MemoryUsage)
		trendIndicator := renderTrendIndicator(memTrend)
		memUsage = fmt.Sprintf("%s %s", FormatBytes(pod.MemoryUsage), trendIndicator)
	} else {
		memUsage = "-"
	}
	memUsage = truncate(memUsage, colMemory)

	// Network RX (download/receive)
	rxRate := m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
	rxStr := formatNetworkRate(rxRate)
	if rxRate == 0 {
		rxStr = StyleTextMuted.Render("-")
	}

	// Network TX (upload/send)
	txRate := m.calculatePodNetworkTxRate(pod.Namespace, pod.Name)
	txStr := formatNetworkRate(txRate)
	if txRate == 0 {
		txStr = StyleTextMuted.Render("-")
	}

	// Restarts
	restarts := fmt.Sprintf("%d", pod.RestartCount)

	return fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
		padRight(name, colName),
		padRight(namespace, colNamespace),
		padRight(status, colStatus),
		padRight(cpuUsage, colCPU),
		padRight(memUsage, colMemory),
		padRight(rxStr, colRx),
		padRight(txStr, colTx),
		padRight(restarts, colRestarts),
	)
}

// renderPodsFooter renders the pods view footer
func (m *Model) renderPodsFooter(pods []*model.PodData) string {
	// Count pod statuses from filtered list
	running, pending, failed := 0, 0, 0
	for _, pod := range pods {
		switch pod.Phase {
		case "Running":
			running++
		case "Pending":
			pending++
		case "Failed":
			failed++
		}
	}

	totalPods := len(pods)
	stats := fmt.Sprintf("%s %s  %s %s  %s %s  %s %d",
		m.T("status.running")+":",
		StyleStatusRunning.Render(fmt.Sprintf("%d", running)),
		m.T("status.pending")+":",
		StyleStatusPending.Render(fmt.Sprintf("%d", pending)),
		m.T("status.failed")+":",
		StyleStatusNotReady.Render(fmt.Sprintf("%d", failed)),
		m.T("common.total")+":",
		totalPods,
	)

	// Add scroll position indicator if there are more items than visible
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}
	if totalPods > maxVisible {
		scrollInfo := fmt.Sprintf("  [%d-%d %s %d]",
			m.scrollOffset+1,
			min(m.scrollOffset+maxVisible, totalPods),
			m.T("common.of"),
			totalPods,
		)
		stats += StyleTextMuted.Render(scrollInfo)
	}

	return StyleTextSecondary.Render(stats)
}

// renderFilterPanel renders the namespace filter panel
func (m *Model) renderFilterPanel() string {
	var lines []string

	lines = append(lines, StyleHeader.Render(m.T("filter.title")))
	lines = append(lines, "")

	namespaces := m.getNamespaces()
	if len(namespaces) == 0 {
		lines = append(lines, StyleTextMuted.Render("  "+m.T("filter.no_namespaces")))
		return strings.Join(lines, "\n")
	}

	// Show "All" option
	allOption := "  " + m.T("filter.all_namespaces")
	if m.filterNamespace == "" {
		allOption = StyleSelected.Render(allOption)
	}
	lines = append(lines, allOption)

	// Show namespace options
	for _, ns := range namespaces {
		option := fmt.Sprintf("  %s", ns)
		if m.filterNamespace == ns {
			option = StyleSelected.Render(option)
		}
		lines = append(lines, option)
	}

	return strings.Join(lines, "\n")
}

// getSortedPods returns a sorted copy of pods based on current sort settings
func (m *Model) getSortedPods(pods []*model.PodData) []*model.PodData {
	if len(pods) == 0 {
		return []*model.PodData{}
	}

	// Create a copy to avoid modifying original
	sortedPods := make([]*model.PodData, len(pods))
	copy(sortedPods, pods)

	// Sort based on current field and order using sort.Slice (O(n log n))
	switch m.sortField {
	case SortByName:
		sort.Slice(sortedPods, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return sortedPods[i].Name < sortedPods[j].Name
			}
			return sortedPods[i].Name > sortedPods[j].Name
		})

	case SortByNamespace:
		sort.Slice(sortedPods, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return sortedPods[i].Namespace < sortedPods[j].Namespace
			}
			return sortedPods[i].Namespace > sortedPods[j].Namespace
		})

	case SortByRestarts:
		sort.Slice(sortedPods, func(i, j int) bool {
			if m.sortOrder == SortAsc {
				return sortedPods[i].RestartCount < sortedPods[j].RestartCount
			}
			return sortedPods[i].RestartCount > sortedPods[j].RestartCount
		})
	}

	return sortedPods
}
