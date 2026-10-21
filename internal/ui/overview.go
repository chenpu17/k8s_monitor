package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
)

const (
	summaryPanelWidth          = 20
	summaryPanelMinContentLine = 5
	// Border draws 1 line on top and bottom, padding adds 1 line each by default.
	summaryPanelExtraHeight = 4
)

var summaryPanelStyle = StyleBorder.Copy().Padding(1, 1)

// formatCPU formats CPU millicores to a human-readable string
func formatCPU(millicores int64) string {
	if millicores == 0 {
		return "0"
	}
	cores := float64(millicores) / 1000.0
	if cores >= 1.0 {
		return fmt.Sprintf("%.1f", cores)
	}
	return fmt.Sprintf("%dm", millicores)
}

// formatMemory formats bytes to a human-readable string
func formatMemory(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	if bytes == 0 {
		return "0"
	}

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTi", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1fGi", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMi", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKi", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func truncateText(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

// formatPercentage formats a float percentage
func formatPercentage(percent float64) string {
	if percent == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", percent)
}

type kubeletHintKind int

const (
	kubeletHintUnknown kubeletHintKind = iota
	kubeletHintRBAC
	kubeletHintTLS
)

func detectKubeletHintKind(errMsg string) kubeletHintKind {
	if errMsg == "" {
		return kubeletHintUnknown
	}

	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "permission"),
		strings.Contains(lower, "nodes/proxy"),
		strings.Contains(errMsg, "权限"),
		strings.Contains(lower, "rbac"):
		return kubeletHintRBAC
	case strings.Contains(lower, "x509"),
		strings.Contains(lower, "certificate"),
		strings.Contains(lower, "unknown authority"),
		strings.Contains(lower, "tls"):
		return kubeletHintTLS
	default:
		return kubeletHintUnknown
	}
}

func kubeletHintEnglish(kind kubeletHintKind) string {
	switch kind {
	case kubeletHintRBAC:
		return "Check RBAC: kubectl auth can-i get nodes/proxy"
	case kubeletHintTLS:
		return "Use --insecure-kubelet for TLS issues (test clusters only)"
	default:
		return "Try --insecure-kubelet"
	}
}

func kubeletHintChinese(kind kubeletHintKind) string {
	switch kind {
	case kubeletHintRBAC:
		return "检查 RBAC：kubectl auth can-i get nodes/proxy"
	case kubeletHintTLS:
		return "使用 --insecure-kubelet（仅限测试环境）"
	default:
		return "尝试 --insecure-kubelet"
	}
}

// kubeletHint returns the appropriate hint based on current locale
func (m *Model) kubeletHint(kind kubeletHintKind) string {
	if m.isChinese() {
		return kubeletHintChinese(kind)
	}
	return kubeletHintEnglish(kind)
}

// renderProgressBar renders a simple progress bar
func renderProgressBar(percent float64, width int) string {
	if width < 2 {
		width = 20
	}
	filled := int(percent / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)

	// Color based on utilization
	var style lipgloss.Style
	switch {
	case percent >= 90:
		style = StyleDanger
	case percent >= 75:
		style = StyleWarning
	case percent >= 50:
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // Yellow
	default:
		style = StyleStatusReady
	}

	return style.Render(bar)
}

func padContentLines(lines []string, target int) []string {
	for len(lines) < target {
		lines = append(lines, "")
	}
	return lines
}

func maxContentLines(target int, panels ...[]string) int {
	maxLines := target
	for _, p := range panels {
		if len(p) > maxLines {
			maxLines = len(p)
		}
	}
	return maxLines
}

func renderSummaryPanel(lines []string, targetContentLines int) string {
	if targetContentLines < summaryPanelMinContentLine {
		targetContentLines = summaryPanelMinContentLine
	}
	lines = padContentLines(lines, targetContentLines)
	return summaryPanelStyle.
		Width(summaryPanelWidth).
		Height(targetContentLines + summaryPanelExtraHeight).
		Render(strings.Join(lines, "\n"))
}

// renderOverview renders the overview view with adaptive layout
func (m *Model) renderOverview() string {
	if m.clusterData == nil {
		return "Loading cluster data..."
	}

	summary := m.clusterData.Summary
	if summary == nil {
		return "No cluster summary available"
	}

	allLines := m.overviewContentLines(summary)
	budget := m.contentBudget()
	startIdx, endIdx := scrollWindow(len(allLines), m.scrollOffset, budget, scrollIndicatorLines)

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator if content exceeds screen
	if len(allLines) > budget {
		scrollInfo := StyleTextMuted.Render(fmt.Sprintf("\n[%s] (↑/↓ %s, PgUp/PgDn %s)",
			m.TF("scroll.showing", map[string]interface{}{
				"Start": startIdx + 1,
				"End":   endIdx,
				"Total": len(allLines),
			}),
			m.T("keys.scroll"),
			m.T("keys.page"),
		))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// overviewMaxScroll returns the largest scroll offset the overview content can
// be scrolled to.
func (m *Model) overviewMaxScroll() int {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return 0
	}
	total := len(m.overviewContentLines(m.clusterData.Summary))
	return maxScrollOffset(total, m.contentBudget(), scrollIndicatorLines)
}

// overviewScrollBy moves the overview by delta lines without ever stepping past
// the end of the content. Scrolling up starts from the offset that is actually
// on screen, so a single ↑ always moves the view one line - also after a refresh
// shortened the content under a dangling offset.
func (m *Model) overviewScrollBy(delta int) {
	limit := m.overviewMaxScroll()
	// Start from the offset that is on screen: a dangling offset (the content
	// shrank under it on a refresh) must not swallow the next ↑ presses.
	offset := min(m.scrollOffset, limit) + delta
	m.scrollOffset = max(0, min(offset, limit))
}

// overviewContentLines builds every line of the overview, in order, before it is
// sliced down to what fits on screen. The layout is adaptive: the panels below
// the cluster load are only added when there is room for them, and the budget
// they are measured against is the same one the window uses.
func (m *Model) overviewContentLines(summary *model.ClusterSummary) []string {
	// Collect all content lines with priority levels
	var allLines []string

	// Priority 1: Cluster Load (CPU/Memory/Network - most important, users care most)
	clusterLoad := m.renderClusterLoadCompact(summary)
	allLines = append(allLines, strings.Split(clusterLoad, "\n")...)
	allLines = append(allLines, "")

	// Calculate available space for remaining content
	// The scroll indicator needs two lines of the budget when the content
	// overflows; the priority checks below only decide what is worth building.
	availableHeight := max(10, m.contentBudget()-scrollIndicatorLines)

	// Check if we have enough space for all content
	estimatedTotalLines := len(allLines) + 15 // Estimate remaining content height

	// Priority 2: Resource Details (compact horizontal layout)
	// Only show if we have space or enable scrolling
	if availableHeight > 20 || estimatedTotalLines <= availableHeight {
		cpuLines := m.cpuDetailsLines(summary)
		memoryLines := m.memoryDetailsLines(summary)
		podLines := m.podDetailsLines(summary)
		nodesLines := m.nodesAndPodsLines(summary)

		// Compact Alert Panel (if there are any alerts) - show as small panel instead of top banner
		var alertLines []string
		if m.hasAlerts(summary) {
			alertLines = m.compactAlertLines(summary)
		} else {
			// If no alerts, show events instead
			alertLines = m.eventSummaryLines(summary)
		}

		targetLines := maxContentLines(summaryPanelMinContentLine, cpuLines, memoryLines, podLines, nodesLines, alertLines)

		cpuPanel := renderSummaryPanel(cpuLines, targetLines)
		memoryPanel := renderSummaryPanel(memoryLines, targetLines)
		podPanel := renderSummaryPanel(podLines, targetLines)
		nodesPods := renderSummaryPanel(nodesLines, targetLines)
		alertPanel := renderSummaryPanel(alertLines, targetLines)

		row2 := lipgloss.JoinHorizontal(
			lipgloss.Top,
			cpuPanel,
			memoryPanel,
			podPanel,
			nodesPods,
			alertPanel,
		)
		allLines = append(allLines, strings.Split(row2, "\n")...)
		allLines = append(allLines, "")
	}

	// Priority 3: Services & Storage info
	if availableHeight > 30 || estimatedTotalLines <= availableHeight {
		servicesInfo := m.renderServicesAndStorage(summary)
		allLines = append(allLines, strings.Split(servicesInfo, "\n")...)
	}

	// Priority 4: SuperPod Topology (only if cluster has NPU nodes with SuperPod info)
	if summary.NPUCapacity > 0 && summary.SuperPodCount > 0 {
		topologyInfo := m.renderSuperPodTopology()
		if topologyInfo != "" {
			allLines = append(allLines, "")
			allLines = append(allLines, strings.Split(topologyInfo, "\n")...)
		}
	}

	return allLines
}

// hasAlerts checks if there are any alerts to display
func (m *Model) hasAlerts(summary *model.ClusterSummary) bool {
	return summary.MemoryPressureNodes > 0 ||
		summary.DiskPressureNodes > 0 ||
		summary.PIDPressureNodes > 0 ||
		summary.NotReadyNodes > 0 ||
		summary.CrashLoopBackOffPods > 0 ||
		summary.ImagePullBackOffPods > 0 ||
		summary.OOMKilledPods > 0 ||
		summary.NoEndpointServices > 0
}

// renderCompactAlertPanel renders a compact alert panel for the overview
func (m *Model) compactAlertLines(summary *model.ClusterSummary) []string {
	var alerts []string
	alerts = append(alerts, StyleHeader.Render("[🔔 Alerts]"))
	alerts = append(alerts, "")

	alertCount := 0

	// Node alerts
	if summary.NotReadyNodes > 0 {
		alerts = append(alerts, StyleDanger.Render(fmt.Sprintf("[X] %d NotReady", summary.NotReadyNodes)))
		alertCount++
	}
	if summary.MemoryPressureNodes > 0 {
		alerts = append(alerts, StyleWarning.Render(fmt.Sprintf("[M] %d MemPress", summary.MemoryPressureNodes)))
		alertCount++
	}
	if summary.DiskPressureNodes > 0 {
		alerts = append(alerts, StyleWarning.Render(fmt.Sprintf("[D] %d DiskPress", summary.DiskPressureNodes)))
		alertCount++
	}

	// Pod anomaly alerts (most critical)
	if summary.CrashLoopBackOffPods > 0 {
		alerts = append(alerts, StyleDanger.Render(fmt.Sprintf("[!] %d CrashLoop", summary.CrashLoopBackOffPods)))
		alertCount++
	}
	if summary.ImagePullBackOffPods > 0 {
		alerts = append(alerts, StyleDanger.Render(fmt.Sprintf("[I] %d ImgPull", summary.ImagePullBackOffPods)))
		alertCount++
	}
	if summary.OOMKilledPods > 0 {
		alerts = append(alerts, StyleDanger.Render(fmt.Sprintf("[!] %d OOMKill", summary.OOMKilledPods)))
		alertCount++
	}

	// If no alerts, show status
	if alertCount == 0 {
		alerts = append(alerts, StyleStatusReady.Render("[OK] No alerts"))
	}

	return alerts
}

// renderClusterLoadCompact renders a compact cluster load summary (most important metrics)
func (m *Model) renderClusterLoadCompact(summary *model.ClusterSummary) string {
	content := []string{
		StyleHeader.Render(m.T("overview.cluster_load_header")),
		"",
	}

	// CPU Load - show both capacity and allocatable for transparency
	if summary.CPUUsed > 0 {
		usagePercent := summary.CPUUsageUtilization
		content = append(content,
			fmt.Sprintf("%s %s / %s %s (%s) %s",
				m.T("metrics.cpu_used"),
				StyleWarning.Render(formatCPU(summary.CPUUsed)),
				formatCPU(summary.CPUCapacity),
				m.T("metrics.capacity"),
				StyleHighlight.Render(formatPercentage(usagePercent)),
				renderProgressBar(usagePercent, 40),
			),
		)
		// Show allocatable for reference
		if summary.CPUAllocatable != summary.CPUCapacity {
			allocPercent := float64(summary.CPUUsed) / float64(summary.CPUAllocatable) * 100
			content = append(content,
				fmt.Sprintf("          %s / %s %s (%s)",
					StyleWarning.Render(formatCPU(summary.CPUUsed)),
					formatCPU(summary.CPUAllocatable),
					m.T("metrics.allocatable"),
					StyleTextMuted.Render(formatPercentage(allocPercent)),
				),
			)
		}
	} else {
		requestPercent := summary.CPURequestUtilization
		content = append(content,
			fmt.Sprintf("%s  %s / %s %s (%s) %s %s",
				m.T("metrics.cpu_req"),
				StyleStatusRunning.Render(formatCPU(summary.CPURequested)),
				formatCPU(summary.CPUAllocatable),
				m.T("metrics.allocatable"),
				StyleHighlight.Render(formatPercentage(requestPercent)),
				renderProgressBar(requestPercent, 40),
				StyleTextMuted.Render(m.T("metrics.no_usage_metrics")),
			),
		)
	}

	// Memory Load - show both capacity and allocatable for transparency
	if summary.MemoryUsed > 0 {
		usagePercent := summary.MemUsageUtilization
		content = append(content,
			fmt.Sprintf("%s %s / %s %s (%s) %s",
				m.T("metrics.mem_used"),
				StyleWarning.Render(formatMemory(summary.MemoryUsed)),
				formatMemory(summary.MemoryCapacity),
				m.T("metrics.capacity"),
				StyleHighlight.Render(formatPercentage(usagePercent)),
				renderProgressBar(usagePercent, 40),
			),
		)
		// Show allocatable for reference
		if summary.MemoryAllocatable != summary.MemoryCapacity {
			allocPercent := float64(summary.MemoryUsed) / float64(summary.MemoryAllocatable) * 100
			content = append(content,
				fmt.Sprintf("          %s / %s %s (%s)",
					StyleWarning.Render(formatMemory(summary.MemoryUsed)),
					formatMemory(summary.MemoryAllocatable),
					m.T("metrics.allocatable"),
					StyleTextMuted.Render(formatPercentage(allocPercent)),
				),
			)
		}
	} else {
		requestPercent := summary.MemRequestUtilization
		content = append(content,
			fmt.Sprintf("%s  %s / %s %s (%s) %s %s",
				m.T("metrics.mem_req"),
				StyleStatusRunning.Render(formatMemory(summary.MemoryRequested)),
				formatMemory(summary.MemoryAllocatable),
				m.T("metrics.allocatable"),
				StyleHighlight.Render(formatPercentage(requestPercent)),
				renderProgressBar(requestPercent, 40),
				StyleTextMuted.Render(m.T("metrics.no_usage_metrics")),
			),
		)
	}

	// Network Load - calculate real-time bandwidth rate from metric history
	rxRate, txRate := m.calculateClusterNetworkRate()
	hasRate := rxRate > 0 || txRate > 0
	hasData := summary.NetworkRxBytes > 0 || summary.NetworkTxBytes > 0
	hasEnoughHistory := len(m.metricHistory) >= 2

	if hasRate || (hasEnoughHistory && hasData) {
		// Show instantaneous rates (MB/s format is more intuitive)
		// If rate is 0 but we have enough history, show "0 MB/s" instead of "collecting..."
		line := fmt.Sprintf("%s %s %s  %s %s",
			m.T("metrics.net"),
			m.T("metrics.rx"),
			formatNetworkRate(rxRate),
			m.T("metrics.tx"),
			formatNetworkRate(txRate),
		)
		content = append(content, line)
	} else if hasData {
		// No rate yet and not enough history - show brief waiting message
		message := m.T("overview.collecting_bandwidth")
		if summary.TotalNodes > 0 && summary.NodesWithMetrics < summary.TotalNodes {
			message += StyleWarning.Render(fmt.Sprintf(" (%d/%d %s)", summary.NodesWithMetrics, summary.TotalNodes, m.T("common.nodes")))
		}
		content = append(content, StyleTextMuted.Render(message))
	} else {
		// No data available at all
		message := m.T("overview.metrics_unavailable")
		if summary.TotalNodes > 0 {
			message += fmt.Sprintf(" (%d/%d %s)", summary.NodesWithMetrics, summary.TotalNodes, m.T("common.nodes"))
		}
		if summary.KubeletError != "" {
			message += fmt.Sprintf(" • %s", truncateText(summary.KubeletError, 60))
		}
		hint := m.kubeletHint(detectKubeletHintKind(summary.KubeletError))
		if hint != "" {
			message += fmt.Sprintf(" • %s", hint)
		}
		content = append(content, StyleTextMuted.Render(message))
	}

	// Hint for getting metrics - more compact
	if !summary.KubeletMetricsAvailable {
		hint := m.kubeletHint(detectKubeletHintKind(summary.KubeletError))
		if hint != "" {
			content = append(content, "", StyleTextMuted.Render("💡 "+hint))
		}
	} else if summary.TotalNodes > 0 && summary.NodesWithMetrics < summary.TotalNodes {
		content = append(content, "",
			StyleWarning.Render(m.TF("overview.partial_metrics", map[string]interface{}{
				"WithMetrics": summary.NodesWithMetrics,
				"Total":       summary.TotalNodes,
			})),
		)
	}

	return StyleBorder.Width(95).Render(strings.Join(content, "\n"))
}

func (m *Model) cpuDetailsLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("[⚡ CPU]"),
		"",
		fmt.Sprintf("%s   %s", m.T("overview.capacity"), StyleHighlight.Render(formatCPU(summary.CPUCapacity))),
		fmt.Sprintf("%s %s", m.T("overview.allocatable"), formatCPU(summary.CPUAllocatable)),
		fmt.Sprintf("%s %s", m.T("overview.requested"), formatCPU(summary.CPURequested)),
	}
	if summary.CPUUsed > 0 {
		lines = append(lines, fmt.Sprintf("%s   %s", m.T("overview.actual"), StyleWarning.Render(formatCPU(summary.CPUUsed))))
	}
	return lines
}

func (m *Model) memoryDetailsLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("[🧠 Memory]"),
		"",
		fmt.Sprintf("%s   %s", m.T("overview.capacity"), StyleHighlight.Render(formatMemory(summary.MemoryCapacity))),
		fmt.Sprintf("%s %s", m.T("overview.allocatable"), formatMemory(summary.MemoryAllocatable)),
		fmt.Sprintf("%s %s", m.T("overview.requested"), formatMemory(summary.MemoryRequested)),
	}
	if summary.MemoryUsed > 0 {
		lines = append(lines, fmt.Sprintf("%s   %s", m.T("overview.actual"), StyleWarning.Render(formatMemory(summary.MemoryUsed))))
	}
	return lines
}

func (m *Model) podDetailsLines(summary *model.ClusterSummary) []string {
	return []string{
		StyleHeader.Render("[📦 Pods]"),
		"",
		fmt.Sprintf("%s %s", m.T("overview.capacity"), StyleHighlight.Render(fmt.Sprintf("%d", summary.PodAllocatable))),
		fmt.Sprintf("%s %s", m.T("overview.running"), StyleStatusRunning.Render(fmt.Sprintf("%d", summary.RunningPods))),
		fmt.Sprintf("%s %s", m.T("overview.pending"), StyleStatusPending.Render(fmt.Sprintf("%d", summary.PendingPods))),
		fmt.Sprintf("%s %s", m.T("overview.failed"), StyleStatusNotReady.Render(fmt.Sprintf("%d", summary.FailedPods))),
	}
}

// renderNodesAndPods renders nodes and pods summary
func (m *Model) nodesAndPodsLines(summary *model.ClusterSummary) []string {
	return []string{
		StyleHeader.Render("[💻 Nodes]"),
		"",
		fmt.Sprintf("Total:    %s", StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalNodes))),
		fmt.Sprintf("Ready:    %s", StyleStatusReady.Render(fmt.Sprintf("%d", summary.ReadyNodes))),
		fmt.Sprintf("NotReady: %s", StyleStatusNotReady.Render(fmt.Sprintf("%d", summary.NotReadyNodes))),
	}
}

// renderEventSummary renders event summary section
func (m *Model) eventSummaryLines(summary *model.ClusterSummary) []string {
	return []string{
		StyleHeader.Render("[📋 Events]"),
		"",
		fmt.Sprintf("Total:   %s", StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalEvents))),
		fmt.Sprintf("Warning: %s", StyleWarning.Render(fmt.Sprintf("%d", summary.WarningEvents))),
		fmt.Sprintf("Error:   %s", StyleDanger.Render(fmt.Sprintf("%d", summary.ErrorEvents))),
	}
}

// renderServicesAndStorage renders services and storage statistics
func (m *Model) renderServicesAndStorage(summary *model.ClusterSummary) string {
	servicesContent := m.servicesPanelLines(summary)
	storageContent := m.storagePanelLines(summary)
	workloadsContent := m.workloadsPanelLines(summary)

	// Include NPU panel if cluster has NPU nodes
	var npuContent []string
	hasNPU := summary.NPUCapacity > 0
	if hasNPU {
		npuContent = m.npuPanelLines(summary)
	}

	// Include Volcano panel if Volcano is available
	var volcanoContent []string
	hasVolcano := m.clusterData != nil && m.clusterData.VolcanoSummary != nil
	if hasVolcano {
		volcanoContent = m.volcanoPanelLines(summary)
	}

	panelsToCompare := [][]string{servicesContent, storageContent, workloadsContent}
	if hasNPU {
		panelsToCompare = append(panelsToCompare, npuContent)
	}
	if hasVolcano {
		panelsToCompare = append(panelsToCompare, volcanoContent)
	}

	targetLines := maxContentLines(summaryPanelMinContentLine, panelsToCompare...)

	servicesPanel := renderSummaryPanel(servicesContent, targetLines)
	storagePanel := renderSummaryPanel(storageContent, targetLines)
	workloadsPanel := renderSummaryPanel(workloadsContent, targetLines)

	panels := []string{servicesPanel, storagePanel, workloadsPanel}

	if hasNPU {
		npuPanel := renderSummaryPanel(npuContent, targetLines)
		panels = append(panels, npuPanel)
	}

	if hasVolcano {
		volcanoPanel := renderSummaryPanel(volcanoContent, targetLines)
		panels = append(panels, volcanoPanel)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
}

func (m *Model) servicesPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("🔌 Services"),
		"",
		fmt.Sprintf("Total:   %s", StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalServices))),
		fmt.Sprintf("ClustIP: %s", fmt.Sprintf("%d", summary.ClusterIPServices)),
		fmt.Sprintf("NodePt:  %s", fmt.Sprintf("%d", summary.NodePortServices)),
		fmt.Sprintf("LoadBal: %s", fmt.Sprintf("%d", summary.LoadBalancerSvcs)),
	}

	if summary.NoEndpointServices > 0 {
		lines = append(lines,
			"",
			StyleWarning.Render(fmt.Sprintf("NoEndpt: %d", summary.NoEndpointServices)),
		)
	}

	return lines
}

func (m *Model) storagePanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("💾 Storage"),
		"",
	}

	if summary.TotalPVs > 0 {
		lines = append(lines,
			fmt.Sprintf("PVs:  %s (%s)",
				StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalPVs)),
				formatMemory(summary.TotalStorageSize),
			),
			fmt.Sprintf("Bound: %s", StyleStatusReady.Render(fmt.Sprintf("%d", summary.BoundPVs))),
		)
		if summary.StorageUsagePercent > 0 {
			lines = append(lines,
				fmt.Sprintf("Used: %s", StyleHighlight.Render(formatPercentage(summary.StorageUsagePercent))),
			)
		}
	} else {
		lines = append(lines, StyleTextMuted.Render("No PVs"))
	}

	if summary.TotalPVCs > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("PVCs: %s", StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalPVCs))),
			fmt.Sprintf("Bound: %s", StyleStatusReady.Render(fmt.Sprintf("%d", summary.BoundPVCs))),
		)
		if summary.PendingPVCs > 0 {
			lines = append(lines,
				StyleWarning.Render(fmt.Sprintf("Pend: %d", summary.PendingPVCs)),
			)
		}
	} else {
		lines = append(lines, StyleTextMuted.Render("No PVCs"))
	}

	return lines
}

func (m *Model) workloadsPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("📋 Workloads"),
		"",
	}

	totalWorkloads := summary.TotalDeployments +
		summary.TotalStatefulSets +
		summary.TotalDaemonSets +
		summary.TotalJobs +
		summary.TotalCronJobs

	if totalWorkloads > 0 {
		lines = append(lines,
			fmt.Sprintf("Deploy:  %d", summary.TotalDeployments),
			fmt.Sprintf("StatSet: %d", summary.TotalStatefulSets),
			fmt.Sprintf("DaemSet: %d", summary.TotalDaemonSets),
			fmt.Sprintf("Jobs:    %d", summary.TotalJobs),
			fmt.Sprintf("Cron:    %d", summary.TotalCronJobs),
		)
	} else {
		lines = append(lines,
			StyleTextMuted.Render("Limited detection"),
			StyleTextMuted.Render("(based on labels)"),
		)
	}

	return lines
}

// npuPanelLines generates NPU (Ascend AI accelerator) panel content
func (m *Model) npuPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("🧮 NPU"),
		"",
	}

	if summary.NPUCapacity > 0 {
		// Basic NPU stats
		lines = append(lines,
			fmt.Sprintf("Total:   %s", StyleHighlight.Render(fmt.Sprintf("%d", summary.NPUAllocatable))),
			fmt.Sprintf("Alloc:   %s", StyleStatusRunning.Render(fmt.Sprintf("%d", summary.NPUAllocated))),
			fmt.Sprintf("Usage:   %s", StyleHighlight.Render(formatPercentage(summary.NPUUtilization))),
		)

		// Progress bar
		lines = append(lines, fmt.Sprintf("  %s", renderProgressBar(summary.NPUUtilization, 14)))

		// NPU Efficiency: Running jobs NPU / Allocated NPU
		volcanoSummary := m.clusterData.VolcanoSummary
		if volcanoSummary != nil && summary.NPUAllocated > 0 {
			// Calculate NPU efficiency (how much of allocated NPU is actually being used by running jobs)
			npuEfficiency := float64(volcanoSummary.NPURunningByJobs) / float64(summary.NPUAllocated) * 100
			effStyle := StyleStatusReady
			if npuEfficiency < 50 {
				effStyle = StyleWarning // Underutilizing NPU
			} else if npuEfficiency < 30 {
				effStyle = StyleTextMuted
			}
			lines = append(lines,
				fmt.Sprintf("Eff:     %s", effStyle.Render(formatPercentage(npuEfficiency))),
			)

			// Job scheduling efficiency: Running / (Running + Pending)
			totalActive := volcanoSummary.RunningJobs + volcanoSummary.PendingJobs
			if totalActive > 0 {
				schedEfficiency := float64(volcanoSummary.RunningJobs) / float64(totalActive) * 100
				schedStyle := StyleStatusReady
				if schedEfficiency < 80 {
					schedStyle = StyleWarning
				} else if schedEfficiency < 50 {
					schedStyle = StyleStatusNotReady
				}
				lines = append(lines,
					fmt.Sprintf("Sched:   %s", schedStyle.Render(fmt.Sprintf("%.0f%% (%d/%d)", schedEfficiency, volcanoSummary.RunningJobs, totalActive))),
				)
			}
		}

		// NPU utilization trend sparkline (if history available)
		npuHistory := m.getClusterNPUUtilizationHistory()
		if len(npuHistory) >= 2 {
			sparkline := RenderSparkline(npuHistory, 14)
			trend := m.calculateClusterNPUTrend(summary.NPUAllocated)
			trendIndicator := renderTrendIndicator(trend)
			lines = append(lines, fmt.Sprintf("Trend: %s %s", sparkline, trendIndicator))
		}

		// NPU type info
		if summary.NPUChipType != "" {
			lines = append(lines,
				"",
				fmt.Sprintf("Chip: %s", StyleTextMuted.Render(summary.NPUChipType)),
			)
		}

		// Topology info
		if summary.SuperPodCount > 0 {
			lines = append(lines,
				fmt.Sprintf("Pods: %d nodes", summary.NPUNodesCount),
			)
		}

		// Check if NPU detailed metrics are available from collector
		hasDetailedMetrics := false
		if m.clusterData != nil && len(m.clusterData.Nodes) > 0 {
			for _, node := range m.clusterData.Nodes {
				if len(node.NPUChips) > 0 {
					hasDetailedMetrics = true
					break
				}
			}
		}
		if !hasDetailedMetrics && summary.NPUNodesCount > 0 {
			lines = append(lines, "")
			lines = append(lines, StyleTextMuted.Render("Deploy collector"))
			lines = append(lines, StyleTextMuted.Render("for chip metrics"))
		}
	} else {
		lines = append(lines, StyleTextMuted.Render("No NPU nodes"))
	}

	return lines
}

// volcanoPanelLines generates Volcano scheduler panel content
func (m *Model) volcanoPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render("🌋 Volcano"),
		"",
	}

	volcanoSummary := m.clusterData.VolcanoSummary
	if volcanoSummary == nil {
		lines = append(lines, StyleTextMuted.Render("Not available"))
		return lines
	}

	// Job statistics
	lines = append(lines,
		fmt.Sprintf("Jobs:    %s", StyleHighlight.Render(fmt.Sprintf("%d", volcanoSummary.TotalJobs))),
		fmt.Sprintf("Running: %s", StyleStatusRunning.Render(fmt.Sprintf("%d", volcanoSummary.RunningJobs))),
		fmt.Sprintf("Pending: %s", StyleStatusPending.Render(fmt.Sprintf("%d", volcanoSummary.PendingJobs))),
	)

	if volcanoSummary.FailedJobs > 0 {
		lines = append(lines,
			fmt.Sprintf("Failed:  %s", StyleDanger.Render(fmt.Sprintf("%d", volcanoSummary.FailedJobs))),
		)
	}

	// NPU usage by Volcano jobs (if cluster has NPU)
	if summary.NPUCapacity > 0 && volcanoSummary.NPURequestedByJobs > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("NPU Req: %s", StyleHighlight.Render(fmt.Sprintf("%d", volcanoSummary.NPURequestedByJobs))),
			fmt.Sprintf("NPU Run: %s", StyleStatusRunning.Render(fmt.Sprintf("%d", volcanoSummary.NPURunningByJobs))),
		)
	}

	// Queue info
	if volcanoSummary.TotalQueues > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("Queues:  %s", StyleTextMuted.Render(fmt.Sprintf("%d", volcanoSummary.TotalQueues))),
		)
	}

	return lines
}

// SuperPodInfo holds aggregated information for a SuperPod
type SuperPodInfo struct {
	ID         string
	NodeCount  int
	NodeIPs    []string
	TotalNPU   int64
	NPUPerNode int64
}

// getSuperPodTopology groups nodes by SuperPod and aggregates NPU information
func (m *Model) getSuperPodTopology() []SuperPodInfo {
	if m.clusterData == nil || len(m.clusterData.Nodes) == 0 {
		return nil
	}

	// Group nodes by SuperPod ID
	superPods := make(map[string]*SuperPodInfo)

	for _, node := range m.clusterData.Nodes {
		if node.SuperPodID == "" {
			continue
		}

		sp, exists := superPods[node.SuperPodID]
		if !exists {
			sp = &SuperPodInfo{
				ID:      node.SuperPodID,
				NodeIPs: []string{},
			}
			superPods[node.SuperPodID] = sp
		}

		sp.NodeCount++
		sp.TotalNPU += node.NPUAllocatable
		if node.InternalIP != "" {
			sp.NodeIPs = append(sp.NodeIPs, node.InternalIP)
		}
	}

	// Convert map to slice and calculate NPU per node
	result := make([]SuperPodInfo, 0, len(superPods))
	for _, sp := range superPods {
		if sp.NodeCount > 0 {
			sp.NPUPerNode = sp.TotalNPU / int64(sp.NodeCount)
		}
		result = append(result, *sp)
	}

	// Sort by SuperPod ID for consistent display
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].ID > result[j].ID {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// renderSuperPodTopology renders the SuperPod topology section
func (m *Model) renderSuperPodTopology() string {
	superPods := m.getSuperPodTopology()
	if len(superPods) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, StyleHeader.Render(m.T("topology.superpod.title")))
	lines = append(lines, "")

	for _, sp := range superPods {
		// Build IP list (limit to first 4 IPs for readability)
		ipList := sp.NodeIPs
		suffix := ""
		if len(ipList) > 4 {
			ipList = ipList[:4]
			suffix = fmt.Sprintf(" +%d", len(sp.NodeIPs)-4)
		}
		ipStr := strings.Join(ipList, ", ") + suffix

		// Format: Superpod X: IP1, IP2 (N节点 × M NPU = Total NPU)
		line := fmt.Sprintf("  %s %s: %s  (%d%s × %d NPU = %s)",
			StyleTextMuted.Render("Superpod"),
			StyleHighlight.Render(sp.ID),
			StyleTextSecondary.Render(ipStr),
			sp.NodeCount,
			m.T("topology.superpod.nodes"),
			sp.NPUPerNode,
			StyleHighlight.Render(fmt.Sprintf("%d NPU", sp.TotalNPU)),
		)
		lines = append(lines, line)
	}

	return StyleBorder.Width(115).Render(strings.Join(lines, "\n"))
}
