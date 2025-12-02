package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderVolcanoJobDetail renders the Volcano job detail view
func (m *Model) renderVolcanoJobDetail() string {
	if m.selectedVolcanoJob == nil {
		return m.T("detail.volcanojob.no_selected")
	}

	job := m.selectedVolcanoJob

	// Clamp volcanoJobPodSelectedIndex to visible range (max 50 pods displayed)
	volcanoJobPods := m.getVolcanoJobPods(job)
	displayCount := len(volcanoJobPods)
	const maxDisplay = 50
	if displayCount > maxDisplay {
		displayCount = maxDisplay
	}
	if m.volcanoJobPodSelectedIndex >= displayCount {
		m.volcanoJobPodSelectedIndex = displayCount - 1
	}
	if m.volcanoJobPodSelectedIndex < 0 && displayCount > 0 {
		m.volcanoJobPodSelectedIndex = 0
	}

	// Build sections
	var sections []string

	// Basic info
	sections = append(sections, m.renderVolcanoJobBasicInfo(job))
	sections = append(sections, "")

	// Resource summary
	sections = append(sections, m.renderVolcanoJobResourceSummary(job))
	sections = append(sections, "")

	// Task breakdown (if job has tasks)
	taskBreakdown := m.renderVolcanoJobTaskBreakdown(job)
	if taskBreakdown != "" {
		sections = append(sections, taskBreakdown)
		sections = append(sections, "")
	}

	// Job pods
	podSection := m.renderVolcanoJobPods(job)
	sections = append(sections, podSection)

	// Split into lines to calculate pod row positions
	lines := strings.Split(strings.Join(sections, "\n"), "\n")
	totalLines := len(lines)

	// Calculate the line number where the selected pod is rendered
	// Find "Pods Detail" header to locate the pod table
	podsDetailHeader := m.T("detail.volcanojob.pods_detail")
	podTableStartLine := -1
	for i, line := range lines {
		if strings.Contains(line, podsDetailHeader) {
			// Pod table starts 3 lines after "Pods Detail" header (header line + blank line + table header)
			podTableStartLine = i + 3
			break
		}
	}

	// Calculate scroll bounds (read-only for View function)
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Use a local variable for scroll offset to avoid modifying state in View
	detailScrollOffset := m.detailScrollOffset

	// If we found the pod table and have a selection, calculate appropriate scroll position
	if podTableStartLine >= 0 {
		volcanoJobPods := m.getVolcanoJobPods(job)
		if len(volcanoJobPods) > 0 && m.volcanoJobPodSelectedIndex < len(volcanoJobPods) {
			// Calculate the line number of the selected pod
			selectedPodLine := podTableStartLine + m.volcanoJobPodSelectedIndex

			// Ensure the selected pod line is within visible range
			visibleStart := detailScrollOffset
			visibleEnd := detailScrollOffset + maxVisible

			// If selected line is above visible area, adjust scroll position
			if selectedPodLine < visibleStart {
				detailScrollOffset = selectedPodLine
			}
			// If selected line is below visible area, adjust scroll position
			if selectedPodLine >= visibleEnd {
				detailScrollOffset = selectedPodLine - maxVisible + 1
			}
		}
	}

	// Clamp scroll offset to valid range
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if detailScrollOffset > maxScroll {
		detailScrollOffset = maxScroll
	}
	if detailScrollOffset < 0 {
		detailScrollOffset = 0
	}

	// Apply scroll offset
	startIdx := detailScrollOffset
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

// renderVolcanoJobBasicInfo renders Volcano job basic information
func (m *Model) renderVolcanoJobBasicInfo(job *model.VolcanoJobData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("🌋 %s: %s", m.T("detail.volcanojob.title"), job.Name)))
	info = append(info, "")

	// Basic Info Section
	info = append(info, StyleSubHeader.Render(m.T("detail.volcanojob.basic_info")))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.namespace")),
		job.Namespace))

	// Status with color
	statusStr := job.Status
	switch job.Status {
	case "Running":
		statusStr = StyleStatusRunning.Render(job.Status)
	case "Pending":
		statusStr = StyleStatusPending.Render(job.Status)
	case "Completed":
		statusStr = StyleStatusReady.Render(job.Status)
	case "Failed", "Aborted":
		statusStr = StyleStatusNotReady.Render(job.Status)
	default:
		statusStr = StyleTextMuted.Render(job.Status)
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.status")),
		statusStr))

	// Queue
	queue := job.Queue
	if queue == "" {
		queue = "default"
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.volcanojob.queue")),
		queue))

	// MinAvailable
	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render(m.T("detail.volcanojob.min_available")),
		job.MinAvailable))

	// Replicas status
	replicasStr := fmt.Sprintf("%d/%d", job.Running+job.Succeeded, job.Replicas)
	if job.Running == job.Replicas {
		replicasStr = StyleStatusReady.Render(replicasStr)
	} else if job.Running > 0 {
		replicasStr = StyleStatusRunning.Render(replicasStr)
	} else if job.Pending > 0 {
		replicasStr = StyleStatusPending.Render(replicasStr)
	}
	info = append(info, fmt.Sprintf("  %s: %s (%s: %d, %s: %d, %s: %d, %s: %d)",
		StyleTextSecondary.Render(m.T("detail.volcanojob.replicas")),
		replicasStr,
		m.T("status.running"), job.Running,
		m.T("status.succeeded"), job.Succeeded,
		m.T("status.pending"), job.Pending,
		m.T("status.failed"), job.Failed))

	// NPU info
	if job.NPURequested > 0 {
		info = append(info, fmt.Sprintf("  %s: %s (%s)",
			StyleTextSecondary.Render(m.T("detail.volcanojob.npu_requested")),
			StyleHighlight.Render(fmt.Sprintf("%d", job.NPURequested)),
			job.NPUResourceName))
	}

	// Duration
	if job.Duration > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.volcanojob.duration")),
			formatDuration(job.Duration)))
	} else if job.Status == "Running" && !job.StartTime.IsZero() {
		info = append(info, fmt.Sprintf("  %s: %s (%s)",
			StyleTextSecondary.Render(m.T("detail.volcanojob.duration")),
			formatDuration(time.Since(job.StartTime)),
			StyleTextMuted.Render(m.T("detail.volcanojob.still_running"))))
	}

	// Start time
	if !job.StartTime.IsZero() {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.volcanojob.start_time")),
			job.StartTime.Format("2006-01-02 15:04:05")))
	}

	// Completion time
	if !job.CompletionTime.IsZero() {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.volcanojob.completion_time")),
			job.CompletionTime.Format("2006-01-02 15:04:05")))
	}

	// Age
	age := time.Since(job.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.age")),
		formatAge(age)))

	// Queue wait time (time from creation to start)
	if !job.StartTime.IsZero() {
		queueWaitTime := job.StartTime.Sub(job.CreationTimestamp)
		if queueWaitTime > 0 {
			waitStyle := StyleStatusReady
			if queueWaitTime > 30*time.Minute {
				waitStyle = StyleStatusNotReady
			} else if queueWaitTime > 10*time.Minute {
				waitStyle = StyleWarning
			}
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(m.T("detail.volcanojob.queue_wait_time")),
				waitStyle.Render(formatDuration(queueWaitTime))))
		}
	} else if job.Status == "Pending" {
		// Show current wait time for pending jobs
		currentWait := time.Since(job.CreationTimestamp)
		waitStyle := StyleStatusPending
		if currentWait > 30*time.Minute {
			waitStyle = StyleStatusNotReady
		} else if currentWait > 10*time.Minute {
			waitStyle = StyleWarning
		}
		info = append(info, fmt.Sprintf("  %s: %s (%s)",
			StyleTextSecondary.Render(m.T("detail.volcanojob.queue_wait_time")),
			waitStyle.Render(formatDuration(currentWait)),
			m.T("detail.volcanojob.still_waiting")))
	}

	// Performance Analysis Section
	if job.Replicas > 0 {
		info = append(info, "")
		info = append(info, StyleSubHeader.Render(m.T("detail.job.performance")))
		info = append(info, "")

		// Success Rate (based on completed pods)
		totalAttempts := job.Succeeded + job.Failed
		if totalAttempts > 0 {
			successRate := float64(job.Succeeded) / float64(totalAttempts) * 100
			rateStyle := StyleStatusReady
			if successRate < 100 && successRate >= 80 {
				rateStyle = StyleWarning
			} else if successRate < 80 {
				rateStyle = StyleStatusNotReady
			}
			info = append(info, fmt.Sprintf("  %s: %s (%d/%d)",
				StyleTextSecondary.Render(m.T("detail.job.success_rate")),
				rateStyle.Render(fmt.Sprintf("%.1f%%", successRate)),
				job.Succeeded, totalAttempts))
		}

		// Completion Progress
		completedOrRunning := job.Succeeded + job.Running
		completionPercent := float64(completedOrRunning) / float64(job.Replicas) * 100
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.job.progress")),
			StyleHighlight.Render(fmt.Sprintf("%.0f%% (%d/%d)", completionPercent, completedOrRunning, job.Replicas))))

		// Throughput (completions per hour) if job has duration
		duration := job.Duration
		if duration == 0 && job.Status == "Running" && !job.StartTime.IsZero() {
			duration = time.Since(job.StartTime)
		}
		if duration > 0 && job.Succeeded > 0 {
			throughputPerHour := float64(job.Succeeded) / duration.Hours()
			if throughputPerHour >= 1 {
				info = append(info, fmt.Sprintf("  %s: %s",
					StyleTextSecondary.Render(m.T("detail.job.throughput")),
					StyleHighlight.Render(fmt.Sprintf("%.1f %s", throughputPerHour, m.T("detail.job.per_hour")))))
			} else {
				// For slow jobs, show per minute
				throughputPerMin := float64(job.Succeeded) / duration.Minutes()
				info = append(info, fmt.Sprintf("  %s: %s",
					StyleTextSecondary.Render(m.T("detail.job.throughput")),
					StyleHighlight.Render(fmt.Sprintf("%.2f %s", throughputPerMin, m.T("detail.job.per_minute")))))
			}

			// ETA for running jobs
			if job.Status == "Running" && job.Succeeded < job.Replicas {
				remaining := job.Replicas - job.Succeeded - job.Running
				if remaining > 0 {
					etaHours := float64(remaining) / throughputPerHour
					etaDuration := time.Duration(etaHours * float64(time.Hour))
					if etaDuration > 0 {
						info = append(info, fmt.Sprintf("  %s: %s",
							StyleTextSecondary.Render(m.T("detail.job.eta")),
							StyleHighlight.Render(formatDuration(etaDuration))))
					}
				}
			}
		}
	}

	return strings.Join(info, "\n")
}

// renderVolcanoJobResourceSummary renders resource summary for Volcano job pods
func (m *Model) renderVolcanoJobResourceSummary(job *model.VolcanoJobData) string {
	var info []string

	volcanoJobPods := m.getVolcanoJobPods(job)
	if len(volcanoJobPods) == 0 {
		return ""
	}

	// Calculate resource totals
	var totalCPURequest, totalCPULimit, totalMemRequest, totalMemLimit int64
	var totalCPUUsage, totalMemUsage int64
	var totalNetworkRxRate, totalNetworkTxRate float64

	for _, pod := range volcanoJobPods {
		totalCPURequest += pod.CPURequest
		totalCPULimit += pod.CPULimit
		totalMemRequest += pod.MemoryRequest
		totalMemLimit += pod.MemoryLimit
		totalCPUUsage += pod.CPUUsage
		totalMemUsage += pod.MemoryUsage

		// Calculate network rate for each pod and sum
		totalNetworkRxRate += m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
		totalNetworkTxRate += m.calculatePodNetworkTxRate(pod.Namespace, pod.Name)
	}

	info = append(info, StyleSubHeader.Render(m.T("detail.volcanojob.resource_summary")))
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

	// CPU Efficiency (actual usage / requested) with progress bar
	if totalCPURequest > 0 {
		cpuEfficiency := float64(totalCPUUsage) / float64(totalCPURequest) * 100
		effStyle := StyleStatusReady
		if cpuEfficiency > 100 {
			effStyle = StyleWarning // Overusing
		} else if cpuEfficiency < 30 {
			effStyle = StyleTextMuted // Underutilizing
		}
		info = append(info, fmt.Sprintf("    %s: %s  %s",
			m.T("detail.job.efficiency"),
			effStyle.Render(fmt.Sprintf("%5.1f%%", cpuEfficiency)),
			renderProgressBar(cpuEfficiency, 30)))
	}

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

	// Memory Efficiency (actual usage / requested) with progress bar
	if totalMemRequest > 0 {
		memEfficiency := float64(totalMemUsage) / float64(totalMemRequest) * 100
		effStyle := StyleStatusReady
		if memEfficiency > 100 {
			effStyle = StyleWarning // Overusing
		} else if memEfficiency < 30 {
			effStyle = StyleTextMuted // Underutilizing
		}
		info = append(info, fmt.Sprintf("    %s: %s  %s",
			m.T("detail.job.efficiency"),
			effStyle.Render(fmt.Sprintf("%5.1f%%", memEfficiency)),
			renderProgressBar(memEfficiency, 30)))
	}

	// Network
	if totalNetworkRxRate > 0 || totalNetworkTxRate > 0 {
		info = append(info, fmt.Sprintf("  %s:",
			StyleTextSecondary.Render(m.T("detail.volcanojob.network_bandwidth"))))
		info = append(info, fmt.Sprintf("    %s: %s  •  %s: %s  •  %s: %s",
			m.T("columns.rx"),
			formatNetworkRate(totalNetworkRxRate),
			m.T("columns.tx"),
			formatNetworkRate(totalNetworkTxRate),
			m.T("detail.job.total"),
			formatNetworkRate(totalNetworkRxRate+totalNetworkTxRate)))
	}

	// NPU summary if job uses NPU
	if job.NPURequested > 0 {
		info = append(info, fmt.Sprintf("  %s:",
			StyleTextSecondary.Render(m.T("detail.volcanojob.npu_total"))))

		// Calculate NPU allocation from running/pending pods
		var npuInUse int64
		var runningWithNPU, pendingWithNPU int
		for _, pod := range volcanoJobPods {
			if pod.NPURequest > 0 {
				if pod.Phase == "Running" {
					npuInUse += pod.NPURequest
					runningWithNPU++
				} else if pod.Phase == "Pending" {
					pendingWithNPU++
				}
			}
		}

		// NPU requested total
		info = append(info, fmt.Sprintf("    %s: %s",
			m.T("detail.volcanojob.npu_requested"),
			StyleHighlight.Render(fmt.Sprintf("%d", job.NPURequested))))

		// NPU in use by running pods
		if npuInUse > 0 || runningWithNPU > 0 {
			info = append(info, fmt.Sprintf("    %s: %s (%d %s)",
				m.T("detail.job.usage"),
				StyleHighlight.Render(fmt.Sprintf("%d", npuInUse)),
				runningWithNPU,
				m.T("status.running")))
		}

		// NPU allocation efficiency with progress bar
		if job.NPURequested > 0 {
			npuEfficiency := float64(npuInUse) / float64(job.NPURequested) * 100
			effStyle := StyleStatusReady
			if npuEfficiency < 30 {
				effStyle = StyleStatusNotReady
			} else if npuEfficiency < 50 {
				effStyle = StyleWarning
			}
			info = append(info, fmt.Sprintf("    %s: %s  %s",
				m.T("detail.volcanojob.npu_efficiency"),
				effStyle.Render(fmt.Sprintf("%5.1f%%", npuEfficiency)),
				renderProgressBar(npuEfficiency, 30)))
		}

		// Show pending NPU pods if any
		if pendingWithNPU > 0 {
			info = append(info, fmt.Sprintf("    %s",
				StyleStatusPending.Render(fmt.Sprintf("%d %s %s", pendingWithNPU, m.T("status.pending"), m.T("common.pods")))))
		}
	}

	return strings.Join(info, "\n")
}

// renderVolcanoJobTaskBreakdown renders per-task resource breakdown
func (m *Model) renderVolcanoJobTaskBreakdown(job *model.VolcanoJobData) string {
	if len(job.Tasks) == 0 {
		return ""
	}

	var info []string
	info = append(info, StyleSubHeader.Render(m.T("detail.volcanojob.task_breakdown")))
	info = append(info, "")

	// Get pods grouped by task
	volcanoJobPods := m.getVolcanoJobPods(job)
	podsByTask := make(map[string][]*model.PodData)
	for _, pod := range volcanoJobPods {
		taskName := pod.Labels["volcano.sh/task-spec"]
		if taskName == "" {
			taskName = "default"
		}
		podsByTask[taskName] = append(podsByTask[taskName], pod)
	}

	// Determine if job uses NPU
	hasNPU := job.NPURequested > 0

	// Table header
	var headerRow string
	if hasNPU {
		headerRow = fmt.Sprintf("  %s  %s  %s  %s  %s  %s",
			padRight(m.T("detail.volcanojob.task_name"), 20),
			padRight(m.T("detail.volcanojob.task_replicas"), 12),
			padRight(m.T("columns.cpu"), 12),
			padRight(m.T("columns.memory"), 12),
			padRight("NPU", 8),
			padRight(m.T("columns.status"), 20),
		)
	} else {
		headerRow = fmt.Sprintf("  %s  %s  %s  %s  %s",
			padRight(m.T("detail.volcanojob.task_name"), 20),
			padRight(m.T("detail.volcanojob.task_replicas"), 12),
			padRight(m.T("columns.cpu"), 12),
			padRight(m.T("columns.memory"), 12),
			padRight(m.T("columns.status"), 20),
		)
	}
	info = append(info, StyleTextMuted.Render(headerRow))

	// Render each task
	for _, task := range job.Tasks {
		pods := podsByTask[task.Name]

		// Count pod phases for this task
		running, pending, succeeded, failed := 0, 0, 0, 0
		var taskCPUUsage, taskMemUsage int64
		for _, pod := range pods {
			switch pod.Phase {
			case "Running":
				running++
			case "Pending":
				pending++
			case "Succeeded":
				succeeded++
			case "Failed":
				failed++
			}
			taskCPUUsage += pod.CPUUsage
			taskMemUsage += pod.MemoryUsage
		}

		// Replicas status
		replicasStr := fmt.Sprintf("%d/%d", running+succeeded, task.Replicas)
		if running == int(task.Replicas) {
			replicasStr = StyleStatusReady.Render(replicasStr)
		} else if running > 0 {
			replicasStr = StyleStatusRunning.Render(replicasStr)
		} else if pending > 0 {
			replicasStr = StyleStatusPending.Render(replicasStr)
		}

		// CPU/Memory usage
		cpuStr := FormatMillicores(taskCPUUsage)
		if taskCPUUsage == 0 {
			cpuStr = StyleTextMuted.Render("-")
		}
		memStr := FormatBytes(taskMemUsage)
		if taskMemUsage == 0 {
			memStr = StyleTextMuted.Render("-")
		}

		// NPU
		npuStr := "-"
		if task.NPURequest > 0 {
			totalNPU := task.NPURequest * int64(task.Replicas)
			npuStr = StyleHighlight.Render(fmt.Sprintf("%dx%d=%d", task.NPURequest, task.Replicas, totalNPU))
		}

		// Status summary
		var statusParts []string
		if running > 0 {
			statusParts = append(statusParts, StyleStatusRunning.Render(fmt.Sprintf("%d %s", running, m.T("status.running"))))
		}
		if pending > 0 {
			statusParts = append(statusParts, StyleStatusPending.Render(fmt.Sprintf("%d %s", pending, m.T("status.pending"))))
		}
		if succeeded > 0 {
			statusParts = append(statusParts, StyleStatusReady.Render(fmt.Sprintf("%d %s", succeeded, m.T("status.succeeded"))))
		}
		if failed > 0 {
			statusParts = append(statusParts, StyleStatusNotReady.Render(fmt.Sprintf("%d %s", failed, m.T("status.failed"))))
		}
		statusStr := strings.Join(statusParts, ", ")
		if statusStr == "" {
			statusStr = StyleTextMuted.Render("-")
		}

		// Build row
		var row string
		if hasNPU {
			row = fmt.Sprintf("  %s  %s  %s  %s  %s  %s",
				padRight(task.Name, 20),
				padRight(replicasStr, 12),
				padRight(cpuStr, 12),
				padRight(memStr, 12),
				padRight(npuStr, 8),
				statusStr,
			)
		} else {
			row = fmt.Sprintf("  %s  %s  %s  %s  %s",
				padRight(task.Name, 20),
				padRight(replicasStr, 12),
				padRight(cpuStr, 12),
				padRight(memStr, 12),
				statusStr,
			)
		}
		info = append(info, row)
	}

	return strings.Join(info, "\n")
}

// renderVolcanoJobPods renders pods associated with this Volcano job
func (m *Model) renderVolcanoJobPods(job *model.VolcanoJobData) string {
	var info []string

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleSubHeader.Render(m.T("detail.volcanojob.pods_header")))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.volcanojob.no_pod_info")))
		return strings.Join(info, "\n")
	}

	// Find pods owned by this Volcano job
	volcanoJobPods := m.getVolcanoJobPods(job)

	if len(volcanoJobPods) == 0 {
		info = append(info, StyleSubHeader.Render(m.T("detail.volcanojob.pods_header")))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.volcanojob.no_pods")))
		return strings.Join(info, "\n")
	}

	// Calculate stats
	runningPods := 0
	succeededPods := 0
	failedPods := 0
	pendingPods := 0

	for _, pod := range volcanoJobPods {
		switch pod.Phase {
		case "Running":
			runningPods++
		case "Succeeded":
			succeededPods++
		case "Failed":
			failedPods++
		case "Pending":
			pendingPods++
		}
	}

	// Header with stats
	header := m.TF("detail.volcanojob.pods_total", map[string]interface{}{
		"Total": len(volcanoJobPods),
	})
	if runningPods > 0 {
		header += StyleStatusRunning.Render(" • " + m.TF("detail.volcanojob.pods_running", map[string]interface{}{
			"Count": runningPods,
		}))
	}
	if pendingPods > 0 {
		header += StyleStatusPending.Render(" • " + m.TF("detail.volcanojob.pods_pending", map[string]interface{}{
			"Count": pendingPods,
		}))
	}
	if succeededPods > 0 {
		header += StyleStatusReady.Render(" • " + m.TF("detail.volcanojob.pods_succeeded", map[string]interface{}{
			"Count": succeededPods,
		}))
	}
	if failedPods > 0 {
		header += StyleStatusNotReady.Render(" • " + m.TF("detail.volcanojob.pods_failed", map[string]interface{}{
			"Count": failedPods,
		}))
	}
	info = append(info, StyleSubHeader.Render(header))
	info = append(info, "")

	info = append(info, StyleSubHeader.Render(m.T("detail.volcanojob.pods_detail")))
	info = append(info, "")

	// Determine if job uses NPU to show NPU column
	hasNPU := job.NPURequested > 0

	// Table header - adjust columns based on NPU availability
	var colName, colPhase, colCPU, colMemory, colNPU, colRx, colTx, colRestarts, colAge int
	if hasNPU {
		colName = 38
		colPhase = 10
		colCPU = 10
		colMemory = 10
		colNPU = 6
		colRx = 10
		colTx = 10
		colRestarts = 6
		colAge = 8
	} else {
		colName = 45
		colPhase = 11
		colCPU = 12
		colMemory = 13
		colRx = 11
		colTx = 11
		colRestarts = 8
		colAge = 9
	}

	var headerRow string
	if hasNPU {
		headerRow = fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colName),
			padRight(m.T("detail.job.phase"), colPhase),
			padRight(m.T("columns.cpu"), colCPU),
			padRight(m.T("columns.memory"), colMemory),
			padRight("NPU", colNPU),
			padRight(m.T("columns.rx"), colRx),
			padRight(m.T("columns.tx"), colTx),
			padRight(m.T("columns.restarts"), colRestarts),
			padRight(m.T("columns.age"), colAge),
		)
	} else {
		headerRow = fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colName),
			padRight(m.T("detail.job.phase"), colPhase),
			padRight(m.T("columns.cpu"), colCPU),
			padRight(m.T("columns.memory"), colMemory),
			padRight(m.T("columns.rx"), colRx),
			padRight(m.T("columns.tx"), colTx),
			padRight(m.T("columns.restarts"), colRestarts),
			padRight(m.T("columns.age"), colAge),
		)
	}
	info = append(info, StyleTextMuted.Render(headerRow))

	// Limit display to 50 pods
	displayCount := len(volcanoJobPods)
	const maxDisplay = 50
	if displayCount > maxDisplay {
		displayCount = maxDisplay
	}

	// Render pod rows with selection highlighting
	for i := 0; i < displayCount; i++ {
		pod := volcanoJobPods[i]
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

		// NPU display
		npuStr := "-"
		if pod.NPURequest > 0 {
			npuStr = StyleHighlight.Render(fmt.Sprintf("%d", pod.NPURequest))
		}

		// Network RX
		rxRate := m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
		rxStr := formatNetworkRate(rxRate)
		if rxRate == 0 {
			rxStr = StyleTextMuted.Render("-")
		}

		// Network TX
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

		var row string
		if hasNPU {
			row = fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s  %s",
				padRight(truncate(pod.Name, colName), colName),
				padRight(phaseStyled, colPhase),
				padRight(cpuStr, colCPU),
				padRight(memStr, colMemory),
				padRight(npuStr, colNPU),
				padRight(rxStr, colRx),
				padRight(txStr, colTx),
				padRight(restarts, colRestarts),
				padRight(age, colAge),
			)
		} else {
			row = fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s  %s",
				padRight(truncate(pod.Name, colName), colName),
				padRight(phaseStyled, colPhase),
				padRight(cpuStr, colCPU),
				padRight(memStr, colMemory),
				padRight(rxStr, colRx),
				padRight(txStr, colTx),
				padRight(restarts, colRestarts),
				padRight(age, colAge),
			)
		}

		// Highlight selected pod
		if i == m.volcanoJobPodSelectedIndex {
			row = StyleSelected.Render(row)
		}

		info = append(info, row)
	}

	// Show overflow message if there are more pods than displayed
	if len(volcanoJobPods) > displayCount {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+m.TF("detail.volcanojob.more_pods", map[string]interface{}{
			"Extra": len(volcanoJobPods) - displayCount,
			"Shown": maxDisplay,
		})))
	}

	// Add help text at the bottom
	info = append(info, "")
	helpText := StyleTextMuted.Render(m.T("detail.volcanojob.help_text"))
	info = append(info, helpText)

	return strings.Join(info, "\n")
}

// getVolcanoJobPods returns sorted pods associated with this Volcano job
func (m *Model) getVolcanoJobPods(job *model.VolcanoJobData) []*model.PodData {
	if m.clusterData == nil {
		return nil
	}

	var volcanoJobPods []*model.PodData
	for _, pod := range m.clusterData.Pods {
		// Check if pod belongs to this Volcano job
		// Volcano jobs typically set job-name label or use job name as prefix
		if pod.Namespace != job.Namespace {
			continue
		}

		// Check by label first (most reliable)
		if jobName, ok := pod.Labels["volcano.sh/job-name"]; ok && jobName == job.Name {
			volcanoJobPods = append(volcanoJobPods, pod)
			continue
		}

		// Fallback: check if pod name starts with job name
		if strings.HasPrefix(pod.Name, job.Name+"-") {
			volcanoJobPods = append(volcanoJobPods, pod)
		}
	}

	// Sort pods: Failed -> Running -> Pending -> Succeeded -> Others
	sort.Slice(volcanoJobPods, func(i, j int) bool {
		iPriority := getPodPriority(volcanoJobPods[i])
		jPriority := getPodPriority(volcanoJobPods[j])
		if iPriority != jPriority {
			return iPriority < jPriority
		}
		// Tie-breaker 1: sort by creation time (older first)
		if !volcanoJobPods[i].CreationTimestamp.Equal(volcanoJobPods[j].CreationTimestamp) {
			return volcanoJobPods[i].CreationTimestamp.Before(volcanoJobPods[j].CreationTimestamp)
		}
		// Tie-breaker 2: sort by name for complete determinism
		return volcanoJobPods[i].Name < volcanoJobPods[j].Name
	})

	return volcanoJobPods
}
