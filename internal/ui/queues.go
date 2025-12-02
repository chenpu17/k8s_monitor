package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderQueues renders the Volcano Queue view
func (m *Model) renderQueues() string {
	if m.clusterData == nil {
		return m.T("msg.no_data")
	}

	if len(m.clusterData.Queues) == 0 {
		return m.T("views.queues.no_queues")
	}

	var lines []string

	// Header
	header := StyleHeader.Render(m.T("views.queues.title"))
	lines = append(lines, header, "")

	// Summary statistics
	volcanoSummary := m.clusterData.VolcanoSummary
	if volcanoSummary != nil {
		statLine := m.TF("views.queues.stats", map[string]interface{}{
			"Total":   volcanoSummary.TotalQueues,
			"Running": volcanoSummary.RunningJobs,
			"Pending": volcanoSummary.PendingJobs,
		})
		lines = append(lines, statLine)
		lines = append(lines, "")
	}

	queues := m.clusterData.Queues
	totalItems := len(queues)

	// Calculate max visible items based on screen height
	maxVisible := m.height - 10
	if maxVisible < 5 {
		maxVisible = 5
	}

	// Clamp scroll offset to valid range
	maxScroll := totalItems - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Column widths
	const (
		colName     = 25
		colState    = 10
		colWeight   = 8
		colNPU      = 15
		colCPU      = 15
		colMemory   = 15
		colJobs     = 12
	)

	// Table header
	headerLine := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.status"), colState),
		padRight(m.T("views.queues.weight"), colWeight),
		padRight(m.T("columns.npu"), colNPU),
		padRight(m.T("columns.cpu"), colCPU),
		padRight(m.T("columns.memory"), colMemory),
		padRight(m.T("views.queues.jobs"), colJobs))
	lines = append(lines, StyleTextMuted.Render(headerLine))
	lines = append(lines, renderSeparator(m.width))

	// Track how many items we've rendered and the actual visible range
	rendered := 0
	startItem := -1
	endItem := -1

	// Render queue rows with pagination
	for idx, queue := range queues {
		// Skip items before scroll offset
		if idx < m.scrollOffset {
			continue
		}
		// Stop if we've rendered enough items
		if rendered >= maxVisible {
			break
		}

		// Track first and last visible items
		if startItem == -1 {
			startItem = idx
		}
		endItem = idx

		queueLine := m.renderQueueRow(queue, idx, colName, colState, colWeight, colNPU, colCPU, colMemory, colJobs)
		lines = append(lines, queueLine)
		rendered++
	}

	// Scroll indicator
	if totalItems > maxVisible && startItem != -1 && endItem != -1 {
		scrollInfo := m.TF("scroll.showing", map[string]interface{}{
			"Start": startItem + 1,
			"End":   endItem + 1,
			"Total": totalItems,
		})
		lines = append(lines, "")
		lines = append(lines, StyleTextMuted.Render(scrollInfo))
	}

	return strings.Join(lines, "\n")
}

// renderQueueRow renders a single Queue row
func (m *Model) renderQueueRow(queue *model.QueueData, index int, colName, colState, colWeight, colNPU, colCPU, colMemory, colJobs int) string {
	// Name (truncate if too long)
	name := truncate(queue.Name, colName)

	// State with color
	var state string
	switch queue.State {
	case "Open":
		state = StyleStatusReady.Render(queue.State)
	case "Closed":
		state = StyleStatusNotReady.Render(queue.State)
	default:
		state = StyleTextMuted.Render(queue.State)
	}

	// Weight
	weight := fmt.Sprintf("%d", queue.Weight)

	// NPU (allocated/deserved)
	var npu string
	if queue.NPUDeserved > 0 {
		npu = fmt.Sprintf("%d/%d", queue.NPUAllocated, queue.NPUDeserved)
	} else if queue.NPUAllocated > 0 {
		npu = fmt.Sprintf("%d", queue.NPUAllocated)
	} else {
		npu = "-"
	}

	// CPU (allocated/deserved in cores)
	var cpu string
	if queue.CPUDeserved > 0 {
		allocCores := float64(queue.CPUAllocated) / 1000.0
		deservedCores := float64(queue.CPUDeserved) / 1000.0
		cpu = fmt.Sprintf("%.1f/%.1f", allocCores, deservedCores)
	} else if queue.CPUAllocated > 0 {
		allocCores := float64(queue.CPUAllocated) / 1000.0
		cpu = fmt.Sprintf("%.1f", allocCores)
	} else {
		cpu = "-"
	}

	// Memory (allocated/deserved)
	var memory string
	if queue.MemoryDeserved > 0 {
		memory = fmt.Sprintf("%s/%s", formatMemoryShort(queue.MemoryAllocated), formatMemoryShort(queue.MemoryDeserved))
	} else if queue.MemoryAllocated > 0 {
		memory = formatMemoryShort(queue.MemoryAllocated)
	} else {
		memory = "-"
	}

	// Jobs (running/pending)
	jobs := fmt.Sprintf("%d/%d", queue.RunningJobs, queue.PendingJobs)

	// Build line with proper padding
	line := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s",
		padRight(name, colName),
		padRight(state, colState),
		padRight(weight, colWeight),
		padRight(npu, colNPU),
		padRight(cpu, colCPU),
		padRight(memory, colMemory),
		padRight(jobs, colJobs),
	)

	// Highlight selected row
	if index == m.selectedIndex {
		return StyleSelected.Render(line)
	}

	return line
}

// renderQueueDetail renders detailed information about a Volcano Queue
func (m *Model) renderQueueDetail() string {
	if m.selectedQueue == nil {
		return m.T("detail.queue.no_selected")
	}

	queue := m.selectedQueue
	var lines []string

	// Header
	header := StyleHeader.Render(fmt.Sprintf("📋 %s: %s", m.T("detail.queue.title"), queue.Name))
	lines = append(lines, header, "")

	// Basic Information Section
	lines = append(lines, StyleSubHeader.Render(m.T("detail.queue.basic_info")))
	lines = append(lines, renderSeparator(m.width))

	// State with color
	var stateStr string
	switch queue.State {
	case "Open":
		stateStr = StyleStatusReady.Render(queue.State)
	case "Closed":
		stateStr = StyleStatusNotReady.Render(queue.State)
	default:
		stateStr = StyleTextMuted.Render(queue.State)
	}
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.state"), stateStr))

	// Weight
	lines = append(lines, fmt.Sprintf("  %s: %d", m.T("detail.queue.weight"), queue.Weight))

	// Parent queue (if any)
	if queue.Parent != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.parent"), StyleHighlight.Render(queue.Parent)))
	}

	// Reclaimable
	reclaimable := m.T("common.no")
	if queue.Reclaimable {
		reclaimable = m.T("common.yes")
	}
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.reclaimable"), reclaimable))

	// Resource Quotas Section
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render(m.T("detail.queue.resources")))
	lines = append(lines, renderSeparator(m.width))

	// NPU Resources
	if queue.NPUDeserved > 0 || queue.NPUAllocated > 0 {
		npuLine := fmt.Sprintf("  %s: %s %d / %s %d",
			m.T("columns.npu"),
			m.T("detail.queue.allocated"), queue.NPUAllocated,
			m.T("detail.queue.quota"), queue.NPUDeserved)
		if queue.NPUGuarantee > 0 {
			npuLine += fmt.Sprintf(" / %s %d", m.T("detail.queue.guarantee"), queue.NPUGuarantee)
		}
		lines = append(lines, npuLine)

		// NPU utilization bar
		if queue.NPUDeserved > 0 {
			percent := float64(queue.NPUAllocated) / float64(queue.NPUDeserved) * 100
			bar := renderProgressBar(percent, 30)
			lines = append(lines, fmt.Sprintf("           %s %.1f%%", bar, percent))
		}
	}

	// CPU Resources
	if queue.CPUDeserved > 0 || queue.CPUAllocated > 0 {
		allocCores := float64(queue.CPUAllocated) / 1000.0
		deservedCores := float64(queue.CPUDeserved) / 1000.0
		guaranteeCores := float64(queue.CPUGuarantee) / 1000.0

		cpuLine := fmt.Sprintf("  %s: %s %.1f / %s %.1f",
			m.T("columns.cpu"),
			m.T("detail.queue.allocated"), allocCores,
			m.T("detail.queue.quota"), deservedCores)
		if queue.CPUGuarantee > 0 {
			cpuLine += fmt.Sprintf(" / %s %.1f", m.T("detail.queue.guarantee"), guaranteeCores)
		}
		cpuLine += " cores"
		lines = append(lines, cpuLine)

		// CPU utilization bar
		if queue.CPUDeserved > 0 {
			percent := float64(queue.CPUAllocated) / float64(queue.CPUDeserved) * 100
			bar := renderProgressBar(percent, 30)
			lines = append(lines, fmt.Sprintf("           %s %.1f%%", bar, percent))
		}
	}

	// Memory Resources
	if queue.MemoryDeserved > 0 || queue.MemoryAllocated > 0 {
		memLine := fmt.Sprintf("  %s: %s %s / %s %s",
			m.T("columns.memory"),
			m.T("detail.queue.allocated"), formatMemory(queue.MemoryAllocated),
			m.T("detail.queue.quota"), formatMemory(queue.MemoryDeserved))
		if queue.MemoryGuarantee > 0 {
			memLine += fmt.Sprintf(" / %s %s", m.T("detail.queue.guarantee"), formatMemory(queue.MemoryGuarantee))
		}
		lines = append(lines, memLine)

		// Memory utilization bar
		if queue.MemoryDeserved > 0 {
			percent := float64(queue.MemoryAllocated) / float64(queue.MemoryDeserved) * 100
			bar := renderProgressBar(percent, 30)
			lines = append(lines, fmt.Sprintf("           %s %.1f%%", bar, percent))
		}
	}

	// Pod Resources
	if queue.PodDeserved > 0 || queue.PodAllocated > 0 {
		podLine := fmt.Sprintf("  %s: %s %d / %s %d",
			m.T("columns.pods"),
			m.T("detail.queue.allocated"), queue.PodAllocated,
			m.T("detail.queue.quota"), queue.PodDeserved)
		lines = append(lines, podLine)

		// Pod utilization bar
		if queue.PodDeserved > 0 {
			percent := float64(queue.PodAllocated) / float64(queue.PodDeserved) * 100
			bar := renderProgressBar(percent, 30)
			lines = append(lines, fmt.Sprintf("           %s %.1f%%", bar, percent))
		}
	}

	// Job Statistics Section
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render(m.T("detail.queue.job_stats")))
	lines = append(lines, renderSeparator(m.width))

	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.running_jobs"), StyleStatusRunning.Render(fmt.Sprintf("%d", queue.RunningJobs))))
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.pending_jobs"), StyleWarning.Render(fmt.Sprintf("%d", queue.PendingJobs))))
	if queue.CompletedJobs > 0 {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.completed_jobs"), StyleTextMuted.Render(fmt.Sprintf("%d", queue.CompletedJobs))))
	}
	if queue.FailedJobs > 0 {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.queue.failed_jobs"), StyleError.Render(fmt.Sprintf("%d", queue.FailedJobs))))
	}

	// Queue Wait Time Statistics
	waitStats := m.calculateQueueWaitTimeStats(queue.Name)
	if waitStats.jobCount > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s:", m.T("detail.queue.wait_time_stats")))

		// Average wait time
		avgWaitStyle := StyleStatusReady
		if waitStats.avgWait > 30*time.Minute {
			avgWaitStyle = StyleStatusNotReady
		} else if waitStats.avgWait > 10*time.Minute {
			avgWaitStyle = StyleWarning
		}
		lines = append(lines, fmt.Sprintf("    %s: %s (%s: %d)",
			m.T("detail.queue.avg_wait_time"),
			avgWaitStyle.Render(formatDuration(waitStats.avgWait)),
			m.T("detail.queue.sample_jobs"),
			waitStats.jobCount))

		// Min/Max wait times if we have enough samples
		if waitStats.jobCount > 1 {
			lines = append(lines, fmt.Sprintf("    %s: %s  •  %s: %s",
				m.T("detail.queue.min_wait"),
				StyleStatusReady.Render(formatDuration(waitStats.minWait)),
				m.T("detail.queue.max_wait"),
				StyleWarning.Render(formatDuration(waitStats.maxWait))))
		}

		// Current pending jobs wait time
		if waitStats.pendingCount > 0 {
			pendingStyle := StyleStatusPending
			if waitStats.avgPendingWait > 30*time.Minute {
				pendingStyle = StyleStatusNotReady
			} else if waitStats.avgPendingWait > 10*time.Minute {
				pendingStyle = StyleWarning
			}
			lines = append(lines, fmt.Sprintf("    %s: %s (%d %s)",
				m.T("detail.queue.current_pending_wait"),
				pendingStyle.Render(formatDuration(waitStats.avgPendingWait)),
				waitStats.pendingCount,
				m.T("detail.queue.jobs_waiting")))
		}
	}

	// NPU Resource Name (if available)
	if queue.NPUResourceName != "" {
		lines = append(lines, "")
		lines = append(lines, StyleSubHeader.Render(m.T("npu.details")))
		lines = append(lines, renderSeparator(m.width))
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("npu.resource_name"), StyleHighlight.Render(queue.NPUResourceName)))
	}

	// Lifecycle
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render(m.T("detail.queue.lifecycle")))
	lines = append(lines, renderSeparator(m.width))
	lines = append(lines, fmt.Sprintf("  %s: %s (%s)",
		m.T("detail.created"),
		queue.CreationTimestamp.Format("2006-01-02 15:04:05"),
		formatDuration(time.Since(queue.CreationTimestamp))))

	// Handle scrolling for detail view
	maxVisible := m.height - 10
	if maxVisible < 5 {
		maxVisible = 5
	}

	// Clamp scroll offset to valid range
	maxScroll := len(lines) - maxVisible
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
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	visibleLines := lines[startIdx:endIdx]

	// Add scroll indicator
	if len(lines) > maxVisible {
		scrollInfo := fmt.Sprintf("(viewing %d-%d of %d lines, use ↑↓ or PgUp/PgDn to scroll)",
			startIdx+1, endIdx, len(lines))
		visibleLines = append(visibleLines, "")
		visibleLines = append(visibleLines, StyleTextMuted.Render(scrollInfo))
	}

	return strings.Join(visibleLines, "\n")
}

// formatMemoryShort formats memory in a shorter format for table display
func formatMemoryShort(bytes int64) string {
	if bytes == 0 {
		return "0"
	}
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fT", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.0fM", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0fK", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// queueWaitTimeStats holds wait time statistics for a queue
type queueWaitTimeStats struct {
	avgWait        time.Duration
	minWait        time.Duration
	maxWait        time.Duration
	jobCount       int
	avgPendingWait time.Duration
	pendingCount   int
}

// calculateQueueWaitTimeStats calculates wait time statistics for jobs in a queue
func (m *Model) calculateQueueWaitTimeStats(queueName string) queueWaitTimeStats {
	stats := queueWaitTimeStats{}

	if m.clusterData == nil || len(m.clusterData.VolcanoJobs) == 0 {
		return stats
	}

	var totalWait time.Duration
	var totalPendingWait time.Duration

	for _, job := range m.clusterData.VolcanoJobs {
		// Check if job belongs to this queue
		jobQueue := job.Queue
		if jobQueue == "" {
			jobQueue = "default"
		}
		if jobQueue != queueName {
			continue
		}

		// For running/completed jobs, calculate actual queue wait time
		if !job.StartTime.IsZero() {
			waitTime := job.StartTime.Sub(job.CreationTimestamp)
			if waitTime > 0 {
				if stats.jobCount == 0 {
					stats.minWait = waitTime
					stats.maxWait = waitTime
				} else {
					if waitTime < stats.minWait {
						stats.minWait = waitTime
					}
					if waitTime > stats.maxWait {
						stats.maxWait = waitTime
					}
				}
				totalWait += waitTime
				stats.jobCount++
			}
		} else if job.Status == "Pending" {
			// For pending jobs, calculate current wait time
			currentWait := time.Since(job.CreationTimestamp)
			if currentWait > 0 {
				totalPendingWait += currentWait
				stats.pendingCount++
			}
		}
	}

	// Calculate averages
	if stats.jobCount > 0 {
		stats.avgWait = totalWait / time.Duration(stats.jobCount)
	}
	if stats.pendingCount > 0 {
		stats.avgPendingWait = totalPendingWait / time.Duration(stats.pendingCount)
	}

	return stats
}
