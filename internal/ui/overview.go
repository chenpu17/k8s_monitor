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
	// clusterLoadBoxWidth is the width the cluster-load box is drawn at. lipgloss
	// counts the horizontal padding in Width but not the border, so the widest
	// content line that renders in full is this minus the 4 cells of padding
	// (measured: 91 cells fit, 92 wrap onto a second line and shift every panel
	// below the box).
	clusterLoadBoxWidth = 95
	clusterLoadContent  = clusterLoadBoxWidth - 4
	// summaryPanelContent is the widest line renderSummaryPanel prints in full.
	// Unlike the load box, a longer line is clipped by lipgloss, so a value
	// pushed past it is lost silently instead of wrapping.
	summaryPanelContent = summaryPanelWidth - 2
	overviewSparkWidth  = 10
	npuPanelSparkWidth  = 10
	npuPanelBarWidth    = 6
	// npuPanelAllocBarWidth keeps the pre-batch 14-cell allocation bar: that
	// line is unchanged by this batch.
	npuPanelAllocBarWidth = 14
)

var summaryPanelStyle = StyleBorder.Copy().Padding(1, 1)

// appendSpark appends a sparkline of series to line when the whole sparkline
// fits the given content budget. It yields entirely rather than shortening: a
// half sparkline would misreport the recent trend, and the line must not be
// pushed past its panel's width (see summaryPanelContent).
func appendSpark(line string, series []float64, width, budget int) (string, bool) {
	if len(series) == 0 || width < 1 {
		return line, false
	}
	if visualLength(line)+1+width > budget {
		return line, false
	}
	spark := sparkline(series, width)
	if spark == "" {
		return line, false
	}
	return line + " " + spark, true
}

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

// kubeletHint returns the appropriate hint based on current locale
func (m *Model) kubeletHint(kind kubeletHintKind) string {
	switch kind {
	case kubeletHintRBAC:
		return m.T("overview.kubelet_hint_rbac")
	case kubeletHintTLS:
		return m.T("overview.kubelet_hint_tls")
	default:
		return m.T("overview.kubelet_hint_default")
	}
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

// joinPanelRows lays summary panels out in as few rows as the terminal width
// allows; a wide terminal keeps the single-row JoinHorizontal layout byte for
// byte. An unset width (0) means unknown, so it keeps the single-row layout
// too — real rendering is gated on a positive width by View().
func (m *Model) joinPanelRows(panels []string) string {
	if len(panels) == 0 {
		return ""
	}

	// All summary panels share renderSummaryPanel's width; measuring the
	// first one is enough.
	perRow := len(panels)
	if w1 := lipgloss.Width(panels[0]); m.width > 0 && w1 > 0 {
		perRow = max(1, m.width/w1)
	}

	if perRow >= len(panels) {
		return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
	}

	// Group the panels per row and join the rows directly adjacent; every
	// group fits the terminal by construction of perRow.
	var rows []string
	for start := 0; start < len(panels); start += perRow {
		end := min(start+perRow, len(panels))
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, panels[start:end]...))
	}
	return strings.Join(rows, "\n")
}

// renderOverview renders the overview view with adaptive layout
func (m *Model) renderOverview() string {
	if m.clusterData == nil {
		return m.T("overview.loading_data")
	}

	summary := m.clusterData.Summary
	if summary == nil {
		return m.T("overview.no_summary")
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

		row2 := m.joinPanelRows([]string{
			cpuPanel,
			memoryPanel,
			podPanel,
			nodesPods,
			alertPanel,
		})
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
	alerts = append(alerts, StyleHeader.Render(m.T("overview.compact_alerts_header")))
	alerts = append(alerts, "")

	alertCount := 0

	// Node alerts
	if summary.NotReadyNodes > 0 {
		alerts = append(alerts, StyleDanger.Render(m.TF("overview.compact_not_ready", map[string]interface{}{"Count": summary.NotReadyNodes})))
		alertCount++
	}
	if summary.MemoryPressureNodes > 0 {
		alerts = append(alerts, StyleWarning.Render(m.TF("overview.compact_mem_press", map[string]interface{}{"Count": summary.MemoryPressureNodes})))
		alertCount++
	}
	if summary.DiskPressureNodes > 0 {
		alerts = append(alerts, StyleWarning.Render(m.TF("overview.compact_disk_press", map[string]interface{}{"Count": summary.DiskPressureNodes})))
		alertCount++
	}
	if summary.PIDPressureNodes > 0 {
		alerts = append(alerts, StyleWarning.Render(m.TF("overview.compact_pid_press", map[string]interface{}{"Count": summary.PIDPressureNodes})))
		alertCount++
	}

	// Pod anomaly alerts (most critical)
	if summary.CrashLoopBackOffPods > 0 {
		alerts = append(alerts, StyleDanger.Render(m.TF("overview.compact_crash_loop", map[string]interface{}{"Count": summary.CrashLoopBackOffPods})))
		alertCount++
	}
	if summary.ImagePullBackOffPods > 0 {
		alerts = append(alerts, StyleDanger.Render(m.TF("overview.compact_img_pull", map[string]interface{}{"Count": summary.ImagePullBackOffPods})))
		alertCount++
	}
	if summary.OOMKilledPods > 0 {
		alerts = append(alerts, StyleDanger.Render(m.TF("overview.compact_oom_kill", map[string]interface{}{"Count": summary.OOMKilledPods})))
		alertCount++
	}

	// Service alerts
	if summary.NoEndpointServices > 0 {
		alerts = append(alerts, StyleWarning.Render(m.TF("overview.compact_no_endpoints", map[string]interface{}{"Count": summary.NoEndpointServices})))
		alertCount++
	}

	// If no alerts, show status
	if alertCount == 0 {
		alerts = append(alerts, StyleStatusReady.Render(m.T("overview.compact_no_alerts")))
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
		line := fmt.Sprintf("%s %s / %s %s (%s) %s",
			m.T("metrics.cpu_used"),
			StyleWarning.Render(formatCPU(summary.CPUUsed)),
			formatCPU(summary.CPUCapacity),
			m.T("metrics.capacity"),
			StyleHighlight.Render(formatPercentage(usagePercent)),
			renderProgressBar(usagePercent, 40),
		)
		// Session trend beside the instantaneous figure. It is measured against
		// the box's content width and is left out whole when it does not fit, so
		// the bar and the numbers keep their layout.
		line, _ = appendSpark(line, m.vizSeries(func(s VizSample) (float64, bool) { return s.CPUPct, true }),
			overviewSparkWidth, clusterLoadContent)
		content = append(content, line)
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
		line := fmt.Sprintf("%s %s / %s %s (%s) %s",
			m.T("metrics.mem_used"),
			StyleWarning.Render(formatMemory(summary.MemoryUsed)),
			formatMemory(summary.MemoryCapacity),
			m.T("metrics.capacity"),
			StyleHighlight.Render(formatPercentage(usagePercent)),
			renderProgressBar(usagePercent, 40),
		)
		line, _ = appendSpark(line, m.vizSeries(func(s VizSample) (float64, bool) { return s.MemPct, true }),
			overviewSparkWidth, clusterLoadContent)
		content = append(content, line)
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
			errText := summary.KubeletError
			// A keyed denial renders through the localizer (zh copy is
			// byte-identical to the raw text; en fixes its Chinese leak).
			// Raw node errors have no key and keep the stored text.
			if summary.KubeletErrorKey != "" {
				errText = m.TF(summary.KubeletErrorKey, summary.KubeletErrorArgs)
			}
			message += fmt.Sprintf(" • %s", truncateText(errText, 60))
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

	return StyleBorder.Width(clusterLoadBoxWidth).Render(strings.Join(content, "\n"))
}

func (m *Model) cpuDetailsLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render(m.T("overview.panel_cpu")),
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
		StyleHeader.Render(m.T("overview.panel_memory")),
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
		StyleHeader.Render(m.T("overview.panel_pods")),
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
		StyleHeader.Render(m.T("overview.panel_nodes")),
		"",
		fmt.Sprintf("%s%s", padRight(m.T("overview.panel_total"), 10), StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalNodes))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.nodes_ready"), 10), StyleStatusReady.Render(fmt.Sprintf("%d", summary.ReadyNodes))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.nodes_notready"), 10), StyleStatusNotReady.Render(fmt.Sprintf("%d", summary.NotReadyNodes))),
	}
}

// renderEventSummary renders event summary section
func (m *Model) eventSummaryLines(summary *model.ClusterSummary) []string {
	return []string{
		StyleHeader.Render(m.T("overview.panel_events")),
		"",
		fmt.Sprintf("%s%s", padRight(m.T("overview.panel_total"), 9), StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalEvents))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.events_warning"), 9), StyleWarning.Render(fmt.Sprintf("%d", summary.WarningEvents))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.events_error"), 9), StyleDanger.Render(fmt.Sprintf("%d", summary.ErrorEvents))),
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

	return m.joinPanelRows(panels)
}

func (m *Model) servicesPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render(m.T("overview.panel_services")),
		"",
		fmt.Sprintf("%s%s", padRight(m.T("overview.panel_total"), 9), StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalServices))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.panel_clustip"), 9), fmt.Sprintf("%d", summary.ClusterIPServices)),
		fmt.Sprintf("%s%s", padRight(m.T("overview.panel_nodept"), 9), fmt.Sprintf("%d", summary.NodePortServices)),
		fmt.Sprintf("%s%s", padRight(m.T("overview.panel_loadbal"), 9), fmt.Sprintf("%d", summary.LoadBalancerSvcs)),
	}

	if summary.NoEndpointServices > 0 {
		lines = append(lines,
			"",
			StyleWarning.Render(fmt.Sprintf("%s%d", padRight(m.T("overview.panel_noendpt"), 9), summary.NoEndpointServices)),
		)
	}

	return lines
}

func (m *Model) storagePanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render(m.T("overview.panel_storage")),
		"",
	}

	if summary.TotalPVs > 0 {
		lines = append(lines,
			fmt.Sprintf("%s%s (%s)",
				padRight(m.T("overview.panel_pvs"), 6),
				StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalPVs)),
				formatMemory(summary.TotalStorageSize),
			),
			fmt.Sprintf("%s%s", padRight(m.T("overview.panel_bound"), 7), StyleStatusReady.Render(fmt.Sprintf("%d", summary.BoundPVs))),
		)
		if summary.StorageUsagePercent > 0 {
			lines = append(lines,
				fmt.Sprintf("%s%s", padRight(m.T("overview.panel_used"), 6), StyleHighlight.Render(formatPercentage(summary.StorageUsagePercent))),
			)
		}
	} else {
		lines = append(lines, StyleTextMuted.Render(m.T("overview.panel_no_pvs")))
	}

	if summary.TotalPVCs > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("%s%s", padRight(m.T("overview.panel_pvcs"), 6), StyleHighlight.Render(fmt.Sprintf("%d", summary.TotalPVCs))),
			fmt.Sprintf("%s%s", padRight(m.T("overview.panel_bound"), 7), StyleStatusReady.Render(fmt.Sprintf("%d", summary.BoundPVCs))),
		)
		if summary.PendingPVCs > 0 {
			lines = append(lines,
				StyleWarning.Render(fmt.Sprintf("%s%d", padRight(m.T("overview.panel_pend"), 6), summary.PendingPVCs)),
			)
		}
	} else {
		lines = append(lines, StyleTextMuted.Render(m.T("overview.panel_no_pvcs")))
	}

	return lines
}

func (m *Model) workloadsPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render(m.T("overview.panel_workloads")),
		"",
	}

	totalWorkloads := summary.TotalDeployments +
		summary.TotalStatefulSets +
		summary.TotalDaemonSets +
		summary.TotalJobs +
		summary.TotalCronJobs

	if totalWorkloads > 0 {
		lines = append(lines,
			fmt.Sprintf("%s%d", padRight(m.T("overview.panel_deploy"), 9), summary.TotalDeployments),
			fmt.Sprintf("%s%d", padRight(m.T("overview.panel_statset"), 9), summary.TotalStatefulSets),
			fmt.Sprintf("%s%d", padRight(m.T("overview.panel_daemset"), 9), summary.TotalDaemonSets),
			fmt.Sprintf("%s%d", padRight(m.T("overview.panel_jobs"), 9), summary.TotalJobs),
			fmt.Sprintf("%s%d", padRight(m.T("overview.panel_cron"), 9), summary.TotalCronJobs),
		)
	} else {
		lines = append(lines,
			StyleTextMuted.Render(m.T("overview.panel_limited_detection")),
			StyleTextMuted.Render(m.T("overview.panel_based_on_labels")),
		)
	}

	return lines
}

// npuPanelLines generates NPU (Ascend AI accelerator) panel content
func (m *Model) npuPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render(m.T("overview.panel_npu")),
		"",
	}

	if summary.NPUCapacity > 0 {
		// Basic NPU stats
		lines = append(lines,
			fmt.Sprintf("%s%s", padRight(m.T("overview.panel_total"), 9), StyleHighlight.Render(fmt.Sprintf("%d", summary.NPUAllocatable))),
			fmt.Sprintf("%s%s", padRight(m.T("overview.npu_alloc"), 9), StyleStatusRunning.Render(fmt.Sprintf("%d", summary.NPUAllocated))),
			fmt.Sprintf("%s%s", padRight(m.T("overview.npu_usage"), 9), StyleHighlight.Render(formatPercentage(summary.NPUUtilization))),
		)

		// Progress bar
		lines = append(lines, fmt.Sprintf("  %s", renderProgressBar(summary.NPUUtilization, npuPanelAllocBarWidth)))

		// Chip-level lines. They exist only when chip data arrived: the card's
		// allocation figures above are about the resource, these are about the
		// silicon, and a card that printed "0°C" for a cluster whose exporter
		// never answered would claim a measurement it does not have.
		//
		// Each line is measured against the panel's content width before it is
		// printed: a longer line is clipped by lipgloss, so a value pushed past
		// the edge would be lost silently rather than wrapping.
		if sample := m.currentVizSample(); sample != nil && npuChipDataCount(m.npuNodes()) > 0 {
			healthy, degraded, fault := npuChipTally(m.npuNodes())
			healthLine := fmt.Sprintf("%s%s/%d",
				padRight(m.T("overview.npu_chip_health"), 6),
				StyleStatusReady.Render(fmt.Sprintf("%d", healthy)),
				healthy+degraded+fault)
			// Unknown-health chips make the H/T tally "no reported fault"
			// rather than a health signal (the summary bar's wording rule), so
			// the card states the unknown count outright instead of letting
			// "8/8" read as an all-clear the exporters never gave.
			if unknown := npuUnknownHealthTally(m.npuNodes()); unknown > 0 {
				healthLine += " " + StyleTextMuted.Render(m.TF("overview.npu_chip_unknown",
					map[string]interface{}{"Count": unknown}))
			}
			lines = append(lines, healthLine)

			// The non-healthy counts keep their own rows, one per severity, so a
			// mixed cluster never prints them truncated and each keeps the style
			// its severity owns.
			if degraded > 0 {
				lines = append(lines, StyleWarning.Render(m.TF("npuview.row_degraded",
					map[string]interface{}{"Count": degraded})))
			}
			if fault > 0 {
				lines = append(lines, StyleStatusNotReady.Render(m.TF("npuview.row_fault",
					map[string]interface{}{"Count": fault})))
			}

			// The chip utilisation row is only drawn when a chip reported a
			// utilisation: an idle cluster's 0% is a real value, a silent
			// exporter's is not. The ramp sparkline fills whatever the 20-cell
			// panel leaves over and is dropped whole below three cells; it is a
			// ramp, never the block bar of the allocation row above, so the two
			// cannot be read as the same gauge.
			if sample.NPUUtilKnown {
				// The percentage is the payload, the bar its decoration: the
				// bar yields width to the value first (zh at 100% does not fit
				// both at full width) and disappears before the value could be
				// clipped by the panel edge.
				label := padRight(m.T("overview.npu_chip_util"), 6)
				pct := fmt.Sprintf("%.0f%%", sample.NPUUtilPct)
				barWidth := npuPanelBarWidth
				if avail := summaryPanelContent - visualLength(label) - 1 - visualLength(pct) - 1; barWidth > avail {
					barWidth = avail
				}
				var line string
				if barWidth >= 1 {
					line = fmt.Sprintf("%s%s %s", label, renderProgressBar(sample.NPUUtilPct, barWidth), pct)
				} else {
					line = label + pct
				}
				if left := summaryPanelContent - visualLength(line) - 1; left >= 3 {
					if spark := sparkline(m.vizSeries(func(s VizSample) (float64, bool) {
						return s.NPUUtilPct, s.NPUUtilKnown
					}), min(left, npuPanelSparkWidth)); spark != "" {
						line += " " + spark
					}
				}
				lines = append(lines, line)
			}

			// Temperature and power share one line so the card gains as few rows
			// as the data allows; a pair that does not fit keeps the temperature,
			// which is the figure nothing else on the card carries.
			tempPower := m.TF("overview.npu_temp_power", map[string]interface{}{
				"Temp":  fmt.Sprintf("%d", int(sample.NPUTempAvg)),
				"Power": fmt.Sprintf("%.1f", sample.NPUPowerW/1000),
			})
			if visualLength(tempPower) > summaryPanelContent {
				tempPower = m.TF("overview.npu_temp_only", map[string]interface{}{
					"Temp": fmt.Sprintf("%d", int(sample.NPUTempAvg)),
				})
			}
			lines = append(lines, tempPower)
		}

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
				fmt.Sprintf("%s%s", padRight(m.T("overview.npu_eff"), 9), effStyle.Render(formatPercentage(npuEfficiency))),
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
					fmt.Sprintf("%s%s", padRight(m.T("overview.npu_sched"), 9), schedStyle.Render(fmt.Sprintf("%.0f%% (%d/%d)", schedEfficiency, volcanoSummary.RunningJobs, totalActive))),
				)
			}
		}

		// NPU utilization trend sparkline (if history available)
		npuHistory := m.getClusterNPUUtilizationHistory()
		if len(npuHistory) >= 2 {
			sparkline := RenderSparkline(npuHistory, 14)
			trend := m.calculateClusterNPUTrend(summary.NPUAllocated)
			trendIndicator := renderTrendIndicator(trend)
			lines = append(lines, fmt.Sprintf("%s %s %s", padRight(m.T("overview.npu_trend"), 6), sparkline, trendIndicator))
		}

		// NPU type info
		if summary.NPUChipType != "" {
			lines = append(lines,
				"",
				fmt.Sprintf("%s%s", padRight(m.T("overview.npu_chip"), 6), StyleTextMuted.Render(summary.NPUChipType)),
			)
		}

		// Topology info
		if summary.SuperPodCount > 0 {
			lines = append(lines,
				m.TF("overview.npu_superpod_nodes", map[string]interface{}{"Count": summary.NPUNodesCount}),
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
			lines = append(lines, StyleTextMuted.Render(m.T("overview.npu_deploy_collector")))
			lines = append(lines, StyleTextMuted.Render(m.T("overview.npu_chip_metrics_hint")))
		}
	} else {
		lines = append(lines, StyleTextMuted.Render(m.T("overview.npu_no_nodes")))
	}

	return lines
}

// volcanoPanelLines generates Volcano scheduler panel content
func (m *Model) volcanoPanelLines(summary *model.ClusterSummary) []string {
	lines := []string{
		StyleHeader.Render(m.T("overview.panel_volcano")),
		"",
	}

	volcanoSummary := m.clusterData.VolcanoSummary
	if volcanoSummary == nil {
		lines = append(lines, StyleTextMuted.Render(m.T("overview.volcano_not_available")))
		return lines
	}

	// Job statistics
	lines = append(lines,
		fmt.Sprintf("%s%s", padRight(m.T("overview.volcano_jobs"), 9), StyleHighlight.Render(fmt.Sprintf("%d", volcanoSummary.TotalJobs))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.running"), 9), StyleStatusRunning.Render(fmt.Sprintf("%d", volcanoSummary.RunningJobs))),
		fmt.Sprintf("%s%s", padRight(m.T("overview.pending"), 9), StyleStatusPending.Render(fmt.Sprintf("%d", volcanoSummary.PendingJobs))),
	)

	if volcanoSummary.FailedJobs > 0 {
		lines = append(lines,
			fmt.Sprintf("%s%s", padRight(m.T("overview.failed"), 9), StyleDanger.Render(fmt.Sprintf("%d", volcanoSummary.FailedJobs))),
		)
	}

	// NPU usage by Volcano jobs (if cluster has NPU)
	if summary.NPUCapacity > 0 && volcanoSummary.NPURequestedByJobs > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("%s%s", padRight(m.T("overview.volcano_npu_req"), 9), StyleHighlight.Render(fmt.Sprintf("%d", volcanoSummary.NPURequestedByJobs))),
			fmt.Sprintf("%s%s", padRight(m.T("overview.volcano_npu_run"), 9), StyleStatusRunning.Render(fmt.Sprintf("%d", volcanoSummary.NPURunningByJobs))),
		)
	}

	// Queue info
	if volcanoSummary.TotalQueues > 0 {
		lines = append(lines,
			"",
			fmt.Sprintf("%s%s", padRight(m.T("overview.volcano_queues"), 9), StyleTextMuted.Render(fmt.Sprintf("%d", volcanoSummary.TotalQueues))),
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
			StyleTextMuted.Render(m.T("topology.superpod.label")),
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
