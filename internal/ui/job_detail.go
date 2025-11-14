package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderJobDetail renders the job detail view
func (m *Model) renderJobDetail() string {
	if m.selectedJob == nil {
		return m.T("detail.job.no_selected")
	}

	job := m.selectedJob

	// Clamp jobPodSelectedIndex to visible range (max 50 pods displayed)
	// This prevents selecting invisible pods when job has >50 pods
	jobPods := m.getJobPods(job)
	displayCount := len(jobPods)
	const maxDisplay = 50
	if displayCount > maxDisplay {
		displayCount = maxDisplay
	}
	if m.jobPodSelectedIndex >= displayCount {
		m.jobPodSelectedIndex = displayCount - 1
	}
	if m.jobPodSelectedIndex < 0 && displayCount > 0 {
		m.jobPodSelectedIndex = 0
	}

	// Build sections
	var sections []string

	// Basic info
	sections = append(sections, m.renderJobBasicInfo(job))
	sections = append(sections, "")

	// Job pods
	podSection := m.renderJobPods(job)
	sections = append(sections, podSection)

	// Split into lines to calculate pod row positions
	lines := strings.Split(strings.Join(sections, "\n"), "\n")
	totalLines := len(lines)

	// Calculate the line number where the selected pod is rendered
	// Find "Pods Detail" header to locate the pod table
	podsDetailHeader := m.T("detail.job.pods_detail")
	podTableStartLine := -1
	for i, line := range lines {
		if strings.Contains(line, podsDetailHeader) {
			// Pod table starts 3 lines after "Pods Detail" header (header line + blank line + table header)
			podTableStartLine = i + 3
			break
		}
	}

	// Auto-scroll to keep selected pod visible
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	// If we found the pod table and have a selection, adjust scroll offset
	if podTableStartLine >= 0 {
		jobPods := m.getJobPods(job)
		if len(jobPods) > 0 && m.jobPodSelectedIndex < len(jobPods) {
			// Calculate the line number of the selected pod
			selectedPodLine := podTableStartLine + m.jobPodSelectedIndex

			// Ensure the selected pod line is within visible range
			// Keep selected line in the middle-third of visible area if possible
			visibleStart := m.detailScrollOffset
			visibleEnd := m.detailScrollOffset + maxVisible

			// If selected line is above visible area, scroll up
			if selectedPodLine < visibleStart {
				m.detailScrollOffset = selectedPodLine
			}
			// If selected line is below visible area, scroll down
			if selectedPodLine >= visibleEnd {
				m.detailScrollOffset = selectedPodLine - maxVisible + 1
			}

			// Clamp scroll offset to valid range
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
		}
	}

	// Apply scroll offset
	startIdx := m.detailScrollOffset
	if startIdx >= totalLines {
		startIdx = totalLines - 1
		if startIdx < 0 {
			startIdx = 0
		}
	}

	endIdx := startIdx + maxVisible
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := lines[startIdx:endIdx]

	// Add scroll indicator
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render(m.TF("detail.scroll_indicator", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		}))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderJobBasicInfo renders job basic information
func (m *Model) renderJobBasicInfo(job *model.JobData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("💼 %s: %s", m.T("detail.job.title"), job.Name)))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.namespace")),
		job.Namespace))

	// Status
	statusStr := ""
	if job.Succeeded == job.Completions {
		statusStr = StyleStatusReady.Render(m.TF("detail.job.status_completed", map[string]interface{}{
			"Succeeded":   job.Succeeded,
			"Completions": job.Completions,
		}))
	} else if job.Failed > 0 {
		statusStr = StyleStatusNotReady.Render(m.TF("detail.job.status_failed", map[string]interface{}{
			"Failed":      job.Failed,
			"Succeeded":   job.Succeeded,
			"Completions": job.Completions,
		}))
	} else if job.Active > 0 {
		statusStr = StyleStatusRunning.Render(m.TF("detail.job.status_running", map[string]interface{}{
			"Active":      job.Active,
			"Succeeded":   job.Succeeded,
			"Completions": job.Completions,
		}))
	} else {
		statusStr = StyleTextMuted.Render(m.T("status.pending"))
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.status")),
		statusStr))

	// Completions
	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render(m.T("detail.job.target_completions")),
		job.Completions))

	// Duration
	if job.Duration > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.job.duration")),
			formatDuration(job.Duration)))
	} else if job.Active > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.job.duration")),
			StyleTextMuted.Render(m.T("detail.job.still_running"))))
	}

	// Age
	age := time.Since(job.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.age")),
		formatAge(age)))

	return strings.Join(info, "\n")
}

// renderJobPods renders pods associated with this job
func (m *Model) renderJobPods(job *model.JobData) string {
	var info []string

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleHeader.Render(m.T("detail.job.pods_header")))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.job.no_pod_info")))
		return strings.Join(info, "\n")
	}

	// Find pods owned by this job
	// Jobs typically name their pods with the job name as prefix
	jobPods := m.getJobPods(job)

	if len(jobPods) == 0 {
		info = append(info, StyleHeader.Render(m.T("detail.job.pods_header")))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.job.no_pods")))
		return strings.Join(info, "\n")
	}

	// Calculate resource totals
	var totalCPURequest, totalCPULimit, totalMemRequest, totalMemLimit int64
	var totalCPUUsage, totalMemUsage int64
	var totalNetworkRxRate, totalNetworkTxRate float64
	runningPods := 0
	succeededPods := 0
	failedPods := 0

	for _, pod := range jobPods {
		totalCPURequest += pod.CPURequest
		totalCPULimit += pod.CPULimit
		totalMemRequest += pod.MemoryRequest
		totalMemLimit += pod.MemoryLimit
		totalCPUUsage += pod.CPUUsage
		totalMemUsage += pod.MemoryUsage

		// Calculate network rate (MB/s) for each pod and sum
		totalNetworkRxRate += m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
		totalNetworkTxRate += m.calculatePodNetworkTxRate(pod.Namespace, pod.Name)

		switch pod.Phase {
		case "Running":
			runningPods++
		case "Succeeded":
			succeededPods++
		case "Failed":
			failedPods++
		}
	}

	// Header with stats
	header := m.TF("detail.job.pods_total", map[string]interface{}{
		"Total": len(jobPods),
	})
	if runningPods > 0 {
		header += StyleStatusRunning.Render(" • " + m.TF("detail.job.pods_running", map[string]interface{}{
			"Count": runningPods,
		}))
	}
	if succeededPods > 0 {
		header += StyleStatusReady.Render(" • " + m.TF("detail.job.pods_succeeded", map[string]interface{}{
			"Count": succeededPods,
		}))
	}
	if failedPods > 0 {
		header += StyleStatusNotReady.Render(" • " + m.TF("detail.job.pods_failed", map[string]interface{}{
			"Count": failedPods,
		}))
	}
	info = append(info, StyleHeader.Render(header))
	info = append(info, "")

	if len(jobPods) == 0 {
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.job.no_pods")))
		return strings.Join(info, "\n")
	}

	// Resource summary
	info = append(info, StyleSubHeader.Render(m.T("detail.job.resource_summary")))
	info = append(info, "")

	// CPU
	info = append(info, fmt.Sprintf("  %s:",
		StyleTextSecondary.Render(m.T("columns.cpu"))))
	info = append(info, fmt.Sprintf("    %s: %s  •  %s: %s  •  %s: %s",
		m.T("detail.job.request"),
		FormatMillicores(totalCPURequest),
		m.T("detail.job.limit"),
		FormatMillicores(totalCPULimit),
		m.T("detail.job.usage"),
		StyleHighlight.Render(FormatMillicores(totalCPUUsage))))

	// Memory
	info = append(info, fmt.Sprintf("  %s:",
		StyleTextSecondary.Render(m.T("columns.memory"))))
	info = append(info, fmt.Sprintf("    %s: %s  •  %s: %s  •  %s: %s",
		m.T("detail.job.request"),
		FormatBytes(totalMemRequest),
		m.T("detail.job.limit"),
		FormatBytes(totalMemLimit),
		m.T("detail.job.usage"),
		StyleHighlight.Render(FormatBytes(totalMemUsage))))

	// Network
	if totalNetworkRxRate > 0 || totalNetworkTxRate > 0 {
		info = append(info, fmt.Sprintf("  %s:",
			StyleTextSecondary.Render(m.T("detail.job.network_bandwidth"))))
		info = append(info, fmt.Sprintf("    %s: %s  •  %s: %s  •  %s: %s",
			m.T("columns.rx"),
			formatNetworkRate(totalNetworkRxRate),
			m.T("columns.tx"),
			formatNetworkRate(totalNetworkTxRate),
			m.T("detail.job.total"),
			formatNetworkRate(totalNetworkRxRate+totalNetworkTxRate)))
	}

	info = append(info, "")
	info = append(info, StyleSubHeader.Render(m.T("detail.job.pods_detail")))
	info = append(info, "")

	// Table header
	const (
		colName     = 40
		colPhase    = 11
		colCPU      = 12
		colMemory   = 13
		colRx       = 11
		colTx       = 11
		colRestarts = 8
		colAge      = 9
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("detail.job.phase"), colPhase),
		padRight(m.T("columns.cpu"), colCPU),
		padRight(m.T("columns.memory"), colMemory),
		padRight(m.T("columns.rx"), colRx),
		padRight(m.T("columns.tx"), colTx),
		padRight(m.T("columns.restarts"), colRestarts),
		padRight(m.T("columns.age"), colAge),
	)
	info = append(info, StyleTextMuted.Render(headerRow))

	// Limit display to 50 pods to prevent UI lag with large jobs
	// Show most important pods first (sorted by priority)
	displayCount := len(jobPods)
	const maxDisplay = 50
	if displayCount > maxDisplay {
		displayCount = maxDisplay
	}

	// Pods are already sorted by getJobPods()
	// Render pod rows with selection highlighting
	for i := 0; i < displayCount; i++ {
		pod := jobPods[i]
		phase := pod.Phase
		var phaseStyled string
		var phaseTranslated string
		switch phase {
		case "Running":
			phaseTranslated = m.T("status.running")
			phaseStyled = StyleStatusRunning.Render(phaseTranslated)
		case "Succeeded":
			phaseTranslated = m.T("status.succeeded")
			phaseStyled = StyleStatusReady.Render(phaseTranslated)
		case "Failed":
			phaseTranslated = m.T("status.failed")
			phaseStyled = StyleStatusNotReady.Render(phaseTranslated)
		case "Pending":
			phaseTranslated = m.T("status.pending")
			phaseStyled = StyleStatusPending.Render(phaseTranslated)
		default:
			phaseStyled = StyleTextMuted.Render(phase)
		}

		cpuStr := FormatMillicores(pod.CPUUsage)
		if pod.CPUUsage == 0 {
			cpuStr = StyleTextMuted.Render("-")
		}

		memStr := FormatBytes(pod.MemoryUsage)
		if pod.MemoryUsage == 0 {
			memStr = StyleTextMuted.Render("-")
		}

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

		restarts := fmt.Sprintf("%d", pod.RestartCount)
		if pod.RestartCount > 0 {
			restarts = StyleWarning.Render(restarts)
		}

		age := formatAge(time.Since(pod.CreationTimestamp))

		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
			padRight(truncate(pod.Name, colName), colName),
			padRight(phaseStyled, colPhase),
			padRight(cpuStr, colCPU),
			padRight(memStr, colMemory),
			padRight(rxStr, colRx),
			padRight(txStr, colTx),
			padRight(restarts, colRestarts),
			padRight(age, colAge),
		)

		// Highlight selected pod
		if i == m.jobPodSelectedIndex {
			row = StyleSelected.Render(row)
		}

		info = append(info, row)
	}

	// Show overflow message if there are more pods than displayed
	if len(jobPods) > displayCount {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+m.TF("detail.job.more_pods", map[string]interface{}{
			"Extra": len(jobPods) - displayCount,
			"Shown": maxDisplay,
		})))
	}

	// Add help text at the bottom
	info = append(info, "")
	helpText := StyleTextMuted.Render(m.T("detail.job.help_text"))
	info = append(info, helpText)

	return strings.Join(info, "\n")
}

// getPodPriority returns priority for sorting (lower = higher priority)
func getPodPriority(pod *model.PodData) int {
	switch pod.Phase {
	case "Failed":
		return 0 // Highest priority
	case "Running":
		return 1
	case "Pending":
		return 2
	case "Succeeded":
		return 3
	default:
		return 4 // Lowest priority
	}
}

// getJobPods returns sorted pods associated with this job
func (m *Model) getJobPods(job *model.JobData) []*model.PodData {
	var jobPods []*model.PodData
	for _, pod := range m.clusterData.Pods {
		// Check if pod belongs to this job (same namespace and name starts with job name)
		if pod.Namespace == job.Namespace && strings.HasPrefix(pod.Name, job.Name+"-") {
			jobPods = append(jobPods, pod)
		}
	}

	// Sort pods: Failed → Running → Pending → Succeeded → Others
	// Use sort.Slice for O(n log n) performance instead of O(n²) bubble sort
	// Add tie-breakers for stable ordering when priority is equal
	sort.Slice(jobPods, func(i, j int) bool {
		iPriority := getPodPriority(jobPods[i])
		jPriority := getPodPriority(jobPods[j])
		if iPriority != jPriority {
			return iPriority < jPriority
		}
		// Tie-breaker 1: sort by creation time (older first)
		if !jobPods[i].CreationTimestamp.Equal(jobPods[j].CreationTimestamp) {
			return jobPods[i].CreationTimestamp.Before(jobPods[j].CreationTimestamp)
		}
		// Tie-breaker 2: sort by name for complete determinism
		return jobPods[i].Name < jobPods[j].Name
	})

	return jobPods
}
