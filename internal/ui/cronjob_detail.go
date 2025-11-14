package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderCronJobDetail renders the cronjob detail view
func (m *Model) renderCronJobDetail() string {
	if m.selectedCronJob == nil {
		return "No cronjob selected"
	}

	cj := m.selectedCronJob

	// Build sections
	var sections []string

	// Basic info
	sections = append(sections, m.renderCronJobBasicInfo(cj))
	sections = append(sections, "")

	// Schedule info
	sections = append(sections, m.renderCronJobSchedule(cj))
	sections = append(sections, "")

	// Status
	sections = append(sections, m.renderCronJobStatus(cj))
	sections = append(sections, "")

	// Selector
	sections = append(sections, m.renderCronJobSelector(cj))
	sections = append(sections, "")

	// Active jobs
	sections = append(sections, m.renderCronJobActiveJobs(cj))

	// Split into lines for scroll handling
	lines := strings.Split(strings.Join(sections, "\n"), "\n")
	totalLines := len(lines)

	// Apply scroll offset
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Clamp scroll offset
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
		scrollInfo := StyleTextMuted.Render(fmt.Sprintf("\n[Lines %d-%d of %d] (↑/↓ to scroll)", startIdx+1, endIdx, totalLines))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderCronJobBasicInfo renders cronjob basic information
func (m *Model) renderCronJobBasicInfo(cj *model.CronJobData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("⏰ CronJob: %s", cj.Name)))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Namespace"),
		cj.Namespace))

	// Age
	age := time.Since(cj.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Age"),
		formatAge(age)))

	return strings.Join(info, "\n")
}

// renderCronJobSchedule renders cronjob schedule information
func (m *Model) renderCronJobSchedule(cj *model.CronJobData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Schedule"))
	info = append(info, "")

	// Schedule
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Schedule"),
		StyleHighlight.Render(cj.Schedule)))

	// Suspend status
	suspendStr := "False"
	if cj.Suspend {
		suspendStr = StyleWarning.Render("True") + " " + StyleTextMuted.Render("(Job execution suspended)")
	} else {
		suspendStr = StyleStatusReady.Render("False")
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Suspend"),
		suspendStr))

	return strings.Join(info, "\n")
}

// renderCronJobStatus renders cronjob status information
func (m *Model) renderCronJobStatus(cj *model.CronJobData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Status"))
	info = append(info, "")

	// Active jobs
	activeStr := fmt.Sprintf("%d", cj.Active)
	if cj.Active > 0 {
		activeStr = StyleStatusRunning.Render(activeStr)
	} else {
		activeStr = StyleTextMuted.Render(activeStr)
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Active Jobs"),
		activeStr))

	// Last schedule time
	if !cj.LastScheduleTime.IsZero() {
		lastSchedule := formatAge(time.Since(cj.LastScheduleTime))
		info = append(info, fmt.Sprintf("  %s: %s ago",
			StyleTextSecondary.Render("Last Schedule Time"),
			lastSchedule))
	} else {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("Last Schedule Time"),
			StyleTextMuted.Render("Never")))
	}

	return strings.Join(info, "\n")
}

// renderCronJobSelector renders selector and labels
func (m *Model) renderCronJobSelector(cj *model.CronJobData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Labels"))
	info = append(info, "")

	if len(cj.Labels) == 0 {
		info = append(info, StyleTextMuted.Render("  No labels"))
	} else {
		for key, value := range cj.Labels {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	return strings.Join(info, "\n")
}

// renderCronJobActiveJobs renders active jobs created by this cronjob
func (m *Model) renderCronJobActiveJobs(cj *model.CronJobData) string {
	var info []string

	if m.clusterData == nil || len(m.clusterData.Jobs) == 0 {
		info = append(info, StyleSubHeader.Render("Active Jobs"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No job information available"))
		return strings.Join(info, "\n")
	}

	// Find jobs that belong to this cronjob
	// Jobs created by CronJob typically have the cronjob name as a prefix
	var relatedJobs []*model.JobData
	for _, job := range m.clusterData.Jobs {
		// Check if job is in same namespace
		if job.Namespace != cj.Namespace {
			continue
		}

		// Check if job name starts with cronjob name
		if strings.HasPrefix(job.Name, cj.Name+"-") {
			relatedJobs = append(relatedJobs, job)
		}
	}

	info = append(info, StyleSubHeader.Render(fmt.Sprintf("Related Jobs (%d)", len(relatedJobs))))
	info = append(info, "")

	if len(relatedJobs) == 0 {
		info = append(info, StyleTextMuted.Render("  No jobs found for this cronjob"))
		return strings.Join(info, "\n")
	}

	// Table header
	const (
		colName        = 35
		colCompletions = 15
		colDuration    = 12
		colAge         = 10
		colStatus      = 12
	)

	headerRow := fmt.Sprintf("  %s  %s  %s  %s  %s",
		padRight("JOB NAME", colName),
		padRight("COMPLETIONS", colCompletions),
		padRight("DURATION", colDuration),
		padRight("AGE", colAge),
		padRight("STATUS", colStatus),
	)
	info = append(info, StyleTextMuted.Render(headerRow))

	// Show up to 10 most recent jobs
	displayCount := len(relatedJobs)
	if displayCount > 10 {
		displayCount = 10
	}

	for i := 0; i < displayCount; i++ {
		job := relatedJobs[i]

		completions := fmt.Sprintf("%d/%d", job.Succeeded, job.Completions)
		statusStr := ""

		if job.Succeeded == job.Completions {
			completions = StyleStatusReady.Render(completions)
			statusStr = StyleStatusReady.Render("Complete")
		} else if job.Failed > 0 {
			completions = StyleStatusNotReady.Render(fmt.Sprintf("%d/%d", job.Succeeded, job.Completions))
			statusStr = StyleStatusNotReady.Render(fmt.Sprintf("Failed(%d)", job.Failed))
		} else if job.Active > 0 {
			completions = StyleStatusRunning.Render(fmt.Sprintf("%d/%d", job.Succeeded, job.Completions))
			statusStr = StyleStatusRunning.Render(fmt.Sprintf("Active(%d)", job.Active))
		} else {
			statusStr = StyleTextMuted.Render("Pending")
		}

		duration := "-"
		if job.Duration > 0 {
			duration = formatDuration(job.Duration)
		} else if job.Active > 0 {
			duration = StyleTextMuted.Render("running")
		}

		age := formatAge(time.Since(job.CreationTimestamp))

		row := fmt.Sprintf("  %s  %s  %s  %s  %s",
			padRight(truncate(job.Name, colName), colName),
			padRight(completions, colCompletions),
			padRight(duration, colDuration),
			padRight(age, colAge),
			padRight(statusStr, colStatus),
		)
		info = append(info, row)
	}

	if len(relatedJobs) > displayCount {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render(fmt.Sprintf("  ... and %d more jobs", len(relatedJobs)-displayCount)))
	}

	return strings.Join(info, "\n")
}
