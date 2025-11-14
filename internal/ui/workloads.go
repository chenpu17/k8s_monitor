package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderWorkloads renders the workloads view
func (m *Model) renderWorkloads() string {
	if m.clusterData == nil {
		return m.T("msg.no_data")
	}

	// Build all content lines
	var allLines []string

	// Header
	header := m.renderWorkloadsHeader()
	allLines = append(allLines, header, "")

	// Clear workload sections map
	m.workloadSections = make(map[string]workloadSection)

	// Track global item index across all sections for selection
	currentItemIndex := 0

	// Jobs (selectable - FIRST PRIORITY)
	if len(m.clusterData.Jobs) > 0 {
		sectionStart := len(allLines)
		jobLines, jobCount := m.renderJobsList(currentItemIndex)
		allLines = append(allLines, jobLines...)
		allLines = append(allLines, "")

		m.workloadSections["job"] = workloadSection{
			startLine: sectionStart,
			count:     jobCount,
			itemType:  "job",
		}
		currentItemIndex += jobCount
	}

	// Services (selectable - SECOND PRIORITY)
	if len(m.clusterData.Services) > 0 {
		sectionStart := len(allLines)
		serviceLines, serviceCount := m.renderServicesList(currentItemIndex)
		allLines = append(allLines, serviceLines...)
		allLines = append(allLines, "")

		m.workloadSections["service"] = workloadSection{
			startLine: sectionStart,
			count:     serviceCount,
			itemType:  "service",
		}
		currentItemIndex += serviceCount
	}

	// Deployments (selectable)
	if len(m.clusterData.Deployments) > 0 {
		sectionStart := len(allLines)
		deploymentLines, deploymentCount := m.renderDeploymentsList(currentItemIndex)
		allLines = append(allLines, deploymentLines...)
		allLines = append(allLines, "")

		m.workloadSections["deployment"] = workloadSection{
			startLine: sectionStart,
			count:     deploymentCount,
			itemType:  "deployment",
		}
		currentItemIndex += deploymentCount
	}

	// StatefulSets (selectable)
	if len(m.clusterData.StatefulSets) > 0 {
		sectionStart := len(allLines)
		stsLines, stsCount := m.renderStatefulSetsList(currentItemIndex)
		allLines = append(allLines, stsLines...)
		allLines = append(allLines, "")

		m.workloadSections["statefulset"] = workloadSection{
			startLine: sectionStart,
			count:     stsCount,
			itemType:  "statefulset",
		}
		currentItemIndex += stsCount
	}

	// DaemonSets (selectable)
	if len(m.clusterData.DaemonSets) > 0 {
		sectionStart := len(allLines)
		dsLines, dsCount := m.renderDaemonSetsList(currentItemIndex)
		allLines = append(allLines, dsLines...)
		allLines = append(allLines, "")

		m.workloadSections["daemonset"] = workloadSection{
			startLine: sectionStart,
			count:     dsCount,
			itemType:  "daemonset",
		}
		currentItemIndex += dsCount
	}

	// CronJobs (selectable)
	if len(m.clusterData.CronJobs) > 0 {
		sectionStart := len(allLines)
		cronLines, cronCount := m.renderCronJobsList(currentItemIndex)
		allLines = append(allLines, cronLines...)
		allLines = append(allLines, "")

		m.workloadSections["cronjob"] = workloadSection{
			startLine: sectionStart,
			count:     cronCount,
			itemType:  "cronjob",
		}
		currentItemIndex += cronCount
	}

	if len(allLines) <= 2 {
		return header + "\n\n" + m.T("msg.no_workloads")
	}

	// Apply scroll with bounds checking
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalLines := len(allLines)

	// Auto-scroll to keep selected item visible
	selectedLine := m.findSelectedItemLine()
	if selectedLine >= 0 {
		visibleStart := m.scrollOffset
		visibleEnd := m.scrollOffset + maxVisible

		// If selected line is above visible area, scroll up
		if selectedLine < visibleStart {
			m.scrollOffset = selectedLine
		}
		// If selected line is below visible area, scroll down
		if selectedLine >= visibleEnd {
			m.scrollOffset = selectedLine - maxVisible + 1
		}
	}

	// Clamp scroll offset to valid range
	maxScroll := totalLines - maxVisible
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
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator if needed
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render(fmt.Sprintf("\n[%s %d-%d %s %d] (↑/↓ %s, %s %s)",
			m.T("scroll.showing"),
			startIdx+1,
			endIdx,
			m.T("common.of"),
			totalLines,
			m.T("keys.scroll"),
			m.T("keys.select"),
			m.T("keys.detail"),
		))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// findSelectedItemLine finds the line number of the currently selected item
func (m *Model) findSelectedItemLine() int {
	// Iterate through sections in order to find which item is selected
	currentItemIndex := 0

	// Order: job, service, deployment, statefulset, daemonset, cronjob
	sectionOrder := []string{"job", "service", "deployment", "statefulset", "daemonset", "cronjob"}

	for _, sectionType := range sectionOrder {
		section, exists := m.workloadSections[sectionType]
		if !exists || section.count == 0 {
			continue
		}

		// Check if selected index falls in this section
		if m.selectedIndex >= currentItemIndex && m.selectedIndex < currentItemIndex+section.count {
			// Found the section containing selected item
			itemIndexInSection := m.selectedIndex - currentItemIndex
			// Calculate line number: section start + header(1) + blank(1) + table header(1) + item index
			return section.startLine + 3 + itemIndexInSection
		}

		currentItemIndex += section.count
	}

	return -1 // Not found
}

// renderWorkloadsHeader renders the workloads view header
func (m *Model) renderWorkloadsHeader() string {
	title := StyleHeader.Render("⚙️  " + m.T("views.workloads.title"))

	counts := fmt.Sprintf("%s: %d • %s: %d • %s: %d • %s: %d • %s: %d",
		m.T("workloads.deployments"),
		len(m.clusterData.Deployments),
		m.T("workloads.statefulsets"),
		len(m.clusterData.StatefulSets),
		m.T("workloads.daemonsets"),
		len(m.clusterData.DaemonSets),
		m.T("workloads.jobs"),
		len(m.clusterData.Jobs),
		m.T("workloads.cronjobs"),
		len(m.clusterData.CronJobs),
	)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		StyleTextSecondary.Render(counts),
	)
}

// renderDeployments renders deployments section
func (m *Model) renderDeployments() string {
	var rows []string
	rows = append(rows, StyleSubHeader.Render(m.T("workloads.deployments.title")))
	rows = append(rows, "")

	const (
		colName      = 35
		colNamespace = 15
		colReady     = 15
		colUpToDate  = 12
		colAvailable = 12
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.ready"), colReady),
		padRight(m.T("columns.up_to_date"), colUpToDate),
		padRight(m.T("columns.available"), colAvailable),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	for _, deploy := range m.clusterData.Deployments {
		ready := fmt.Sprintf("%d/%d", deploy.ReadyReplicas, deploy.Replicas)
		if deploy.ReadyReplicas == deploy.Replicas {
			ready = StyleStatusReady.Render(ready)
		} else if deploy.ReadyReplicas == 0 {
			ready = StyleStatusNotReady.Render(ready)
		} else {
			ready = StyleStatusPending.Render(ready)
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(truncate(deploy.Name, colName), colName),
			padRight(truncate(deploy.Namespace, colNamespace), colNamespace),
			padRight(ready, colReady),
			padRight(fmt.Sprintf("%d", deploy.UpdatedReplicas), colUpToDate),
			padRight(fmt.Sprintf("%d", deploy.AvailableReplicas), colAvailable),
		)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderStatefulSets renders statefulsets section
func (m *Model) renderStatefulSets() string {
	var rows []string
	rows = append(rows, StyleSubHeader.Render(m.T("workloads.statefulsets.title")))
	rows = append(rows, "")

	const (
		colName      = 35
		colNamespace = 15
		colReady     = 15
		colCurrent   = 12
		colUpdated   = 12
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.ready"), colReady),
		padRight(m.T("columns.current"), colCurrent),
		padRight(m.T("columns.updated"), colUpdated),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	for _, sts := range m.clusterData.StatefulSets {
		ready := fmt.Sprintf("%d/%d", sts.ReadyReplicas, sts.Replicas)
		if sts.ReadyReplicas == sts.Replicas {
			ready = StyleStatusReady.Render(ready)
		} else if sts.ReadyReplicas == 0 {
			ready = StyleStatusNotReady.Render(ready)
		} else {
			ready = StyleStatusPending.Render(ready)
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(truncate(sts.Name, colName), colName),
			padRight(truncate(sts.Namespace, colNamespace), colNamespace),
			padRight(ready, colReady),
			padRight(fmt.Sprintf("%d", sts.CurrentReplicas), colCurrent),
			padRight(fmt.Sprintf("%d", sts.UpdatedReplicas), colUpdated),
		)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderDaemonSets renders daemonsets section
func (m *Model) renderDaemonSets() string {
	var rows []string
	rows = append(rows, StyleSubHeader.Render(m.T("workloads.daemonsets.title")))
	rows = append(rows, "")

	const (
		colName      = 35
		colNamespace = 15
		colDesired   = 12
		colCurrent   = 12
		colReady     = 12
		colAvailable = 12
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.desired"), colDesired),
		padRight(m.T("columns.current"), colCurrent),
		padRight(m.T("columns.ready"), colReady),
		padRight(m.T("columns.available"), colAvailable),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	for _, ds := range m.clusterData.DaemonSets {
		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(truncate(ds.Name, colName), colName),
			padRight(truncate(ds.Namespace, colNamespace), colNamespace),
			padRight(fmt.Sprintf("%d", ds.DesiredNumberScheduled), colDesired),
			padRight(fmt.Sprintf("%d", ds.CurrentNumberScheduled), colCurrent),
			padRight(fmt.Sprintf("%d", ds.NumberReady), colReady),
			padRight(fmt.Sprintf("%d", ds.NumberAvailable), colAvailable),
		)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderJobs renders jobs section with status grouping
func (m *Model) renderJobs() string {
	var rows []string

	jobs := m.clusterData.Jobs

	// Calculate stats
	totalJobs := len(jobs)
	succeeded := 0
	failed := 0
	active := 0

	for _, job := range jobs {
		if job.Succeeded == job.Completions {
			succeeded++
		} else if job.Failed > 0 {
			failed++
		} else if job.Active > 0 {
			active++
		}
	}

	// Header with stats
	header := fmt.Sprintf("%s  (%s)",
		StyleSubHeader.Render(m.T("workloads.jobs.title")),
		m.TF("workloads.jobs.stats", map[string]interface{}{
			"Total": totalJobs,
			"Succeeded": StyleStatusReady.Render(m.TF("workloads.jobs.stats_succeeded", map[string]interface{}{
				"Count": succeeded,
			})),
			"Failed": StyleStatusNotReady.Render(m.TF("workloads.jobs.stats_failed", map[string]interface{}{
				"Count": failed,
			})),
			"Active": StyleStatusRunning.Render(m.TF("workloads.jobs.stats_active", map[string]interface{}{
				"Count": active,
			})),
		}),
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName        = 30
		colNamespace   = 12
		colCompletions = 18
		colDuration    = 12
		colAge         = 10
		colStatus      = 10
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.completions"), colCompletions),
		padRight(m.T("columns.duration"), colDuration),
		padRight(m.T("columns.age"), colAge),
		padRight(m.T("columns.status"), colStatus),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	// Group jobs by status
	var activeJobs, failedJobs, succeededJobs []*model.JobData
	for _, job := range jobs {
		if job.Active > 0 {
			activeJobs = append(activeJobs, job)
		} else if job.Failed > 0 {
			failedJobs = append(failedJobs, job)
		} else if job.Succeeded == job.Completions {
			succeededJobs = append(succeededJobs, job)
		}
	}

	// Render active jobs first (most important)
	if len(activeJobs) > 0 {
		rows = append(rows, "")
		rows = append(rows, StyleStatusRunning.Render(m.T("workloads.jobs.active_section")))
		for _, job := range activeJobs {
			rows = append(rows, m.renderJobRow(job, colName, colNamespace, colCompletions, colDuration, colAge, colStatus))
		}
	}

	// Then failed jobs (need attention)
	if len(failedJobs) > 0 {
		rows = append(rows, "")
		rows = append(rows, StyleStatusNotReady.Render(m.T("workloads.jobs.failed_section")))
		for _, job := range failedJobs {
			rows = append(rows, m.renderJobRow(job, colName, colNamespace, colCompletions, colDuration, colAge, colStatus))
		}
	}

	// Finally succeeded jobs (show last 5)
	if len(succeededJobs) > 0 {
		rows = append(rows, "")
		rows = append(rows, StyleStatusReady.Render(m.T("workloads.jobs.completed_section")))
		displayCount := 5
		if len(succeededJobs) < displayCount {
			displayCount = len(succeededJobs)
		}
		for i := 0; i < displayCount; i++ {
			rows = append(rows, m.renderJobRow(succeededJobs[i], colName, colNamespace, colCompletions, colDuration, colAge, colStatus))
		}
		if len(succeededJobs) > displayCount {
			rows = append(rows, StyleTextMuted.Render("  "+m.TF("workloads.jobs.and_more", map[string]interface{}{
				"Count": len(succeededJobs) - displayCount,
			})))
		}
	}

	if len(rows) == 2 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("workloads.no_jobs")))
	}

	return strings.Join(rows, "\n")
}

// renderJobsList renders jobs section with selectable items (without scroll logic)
func (m *Model) renderJobsList(sectionOffset int) ([]string, int) {
	var rows []string

	jobs := m.clusterData.Jobs

	// Calculate stats
	totalJobs := len(jobs)
	succeeded := 0
	failed := 0
	active := 0

	for _, job := range jobs {
		if job.Succeeded == job.Completions {
			succeeded++
		} else if job.Failed > 0 {
			failed++
		} else if job.Active > 0 {
			active++
		}
	}

	// Header with stats
	header := fmt.Sprintf("%s  (%s)",
		StyleSubHeader.Render(m.T("workloads.jobs.title")),
		m.TF("workloads.jobs.stats", map[string]interface{}{
			"Total": totalJobs,
			"Succeeded": StyleStatusReady.Render(m.TF("workloads.jobs.stats_succeeded", map[string]interface{}{
				"Count": succeeded,
			})),
			"Failed": StyleStatusNotReady.Render(m.TF("workloads.jobs.stats_failed", map[string]interface{}{
				"Count": failed,
			})),
			"Active": StyleStatusRunning.Render(m.TF("workloads.jobs.stats_active", map[string]interface{}{
				"Count": active,
			})),
		}),
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName        = 30
		colNamespace   = 12
		colCompletions = 18
		colDuration    = 12
		colAge         = 10
		colStatus      = 10
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.completions"), colCompletions),
		padRight(m.T("columns.duration"), colDuration),
		padRight(m.T("columns.age"), colAge),
		padRight(m.T("columns.status"), colStatus),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	if totalJobs == 0 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("workloads.no_jobs")))
		return rows, 0
	}

	// Render all jobs with selection highlighting
	for i, job := range jobs {
		row := m.renderJobRow(job, colName, colNamespace, colCompletions, colDuration, colAge, colStatus)

		// Highlight selected job (using global index)
		globalIndex := sectionOffset + i
		if globalIndex == m.selectedIndex {
			row = StyleSelected.Render("> " + row)
		} else {
			row = "  " + row
		}

		rows = append(rows, row)
	}

	return rows, totalJobs
}

// renderJobRow renders a single job row
func (m *Model) renderJobRow(job *model.JobData, colName, colNamespace, colCompletions, colDuration, colAge, colStatus int) string {
	completions := fmt.Sprintf("%d/%d", job.Succeeded, job.Completions)
	statusStr := ""

	if job.Succeeded == job.Completions {
		completions = StyleStatusReady.Render(completions)
		statusStr = StyleStatusReady.Render(m.T("workloads.jobs.status_complete"))
	} else if job.Failed > 0 {
		completions = StyleStatusNotReady.Render(fmt.Sprintf("%d/%d", job.Succeeded, job.Completions))
		statusStr = StyleStatusNotReady.Render(m.TF("workloads.jobs.status_failed", map[string]interface{}{
			"Count": job.Failed,
		}))
	} else if job.Active > 0 {
		completions = StyleStatusRunning.Render(fmt.Sprintf("%d/%d", job.Succeeded, job.Completions))
		statusStr = StyleStatusRunning.Render(m.TF("workloads.jobs.status_active", map[string]interface{}{
			"Count": job.Active,
		}))
	} else {
		statusStr = StyleTextMuted.Render(m.T("workloads.jobs.status_pending"))
	}

	duration := "-"
	if job.Duration > 0 {
		duration = formatDuration(job.Duration)
	} else if job.Active > 0 {
		duration = StyleTextMuted.Render(m.T("workloads.jobs.status_running"))
	}

	age := formatAge(time.Since(job.CreationTimestamp))

	return fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(truncate(job.Name, colName), colName),
		padRight(truncate(job.Namespace, colNamespace), colNamespace),
		padRight(completions, colCompletions),
		padRight(duration, colDuration),
		padRight(age, colAge),
		padRight(statusStr, colStatus),
	)
}

// renderCronJobs renders cronjobs section
func (m *Model) renderCronJobs() string {
	var rows []string
	rows = append(rows, StyleSubHeader.Render(m.T("workloads.cronjobs.title")))
	rows = append(rows, "")

	const (
		colName      = 30
		colNamespace = 15
		colSchedule  = 20
		colSuspend   = 10
		colActive    = 10
		colLastSchedule = 20
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.schedule"), colSchedule),
		padRight(m.T("columns.suspend"), colSuspend),
		padRight(m.T("columns.active"), colActive),
		padRight(m.T("columns.last_schedule"), colLastSchedule),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	for _, cj := range m.clusterData.CronJobs {
		suspend := m.T("workloads.cronjobs.suspend_false")
		if cj.Suspend {
			suspend = StyleWarning.Render(m.T("workloads.cronjobs.suspend_true"))
		}

		lastSchedule := "-"
		if !cj.LastScheduleTime.IsZero() {
			lastSchedule = formatAge(time.Since(cj.LastScheduleTime))
		}

		active := fmt.Sprintf("%d", cj.Active)
		if cj.Active > 0 {
			active = StyleStatusRunning.Render(active)
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(truncate(cj.Name, colName), colName),
			padRight(truncate(cj.Namespace, colNamespace), colNamespace),
			padRight(truncate(cj.Schedule, colSchedule), colSchedule),
			padRight(suspend, colSuspend),
			padRight(active, colActive),
			padRight(lastSchedule, colLastSchedule),
		)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// formatDuration formats a duration to human readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// formatAge formats age to human readable format
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// renderServicesList renders services section with selectable items
func (m *Model) renderServicesList(sectionOffset int) ([]string, int) {
	var rows []string

	services := m.clusterData.Services
	totalServices := len(services)

	// Header with stats
	header := fmt.Sprintf("%s  (Total: %d)",
		StyleSubHeader.Render("Services"),
		totalServices,
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName      = 28
		colNamespace = 12
		colType      = 14
		colClusterIP = 16
		colPorts     = 24
		colEndpoints = 10
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight("NAME", colName),
		padRight("NAMESPACE", colNamespace),
		padRight("TYPE", colType),
		padRight("CLUSTER-IP", colClusterIP),
		padRight("PORTS", colPorts),
		padRight("ENDPOINTS", colEndpoints),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	if totalServices == 0 {
		rows = append(rows, StyleTextMuted.Render("  No services found"))
		return rows, 0
	}

	// Render all services with selection highlighting
	for i, svc := range services {
		// Format service type
		svcType := svc.Type
		if svcType == "" {
			svcType = "ClusterIP"
		}

		// Format cluster IP
		clusterIP := svc.ClusterIP
		if clusterIP == "" {
			clusterIP = "-"
		}

		// Format ports (show first 2-3 ports compactly)
		portsStr := formatServicePorts(svc.Ports)

		// Format endpoints count
		endpointsStr := fmt.Sprintf("%d", svc.EndpointCount)
		if svc.EndpointCount == 0 {
			endpointsStr = StyleWarning.Render("0")
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(truncate(svc.Name, colName), colName),
			padRight(truncate(svc.Namespace, colNamespace), colNamespace),
			padRight(truncate(svcType, colType), colType),
			padRight(truncate(clusterIP, colClusterIP), colClusterIP),
			padRight(truncate(portsStr, colPorts), colPorts),
			padRight(endpointsStr, colEndpoints),
		)

		// Highlight selected service (using global index)
		globalIndex := sectionOffset + i
		if globalIndex == m.selectedIndex {
			row = StyleSelected.Render("> " + row)
		} else {
			row = "  " + row
		}

		rows = append(rows, row)
	}

	return rows, totalServices
}

// formatServicePorts formats service ports for display
func formatServicePorts(ports []model.ServicePort) string {
	if len(ports) == 0 {
		return "-"
	}

	var portStrs []string
	for i, p := range ports {
		if i >= 2 { // Show only first 2 ports
			portStrs = append(portStrs, "...")
			break
		}

		var portStr string
		if p.NodePort > 0 {
			// NodePort service: show port:nodePort
			portStr = fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, p.Protocol)
		} else {
			// ClusterIP/LoadBalancer: show port/protocol
			portStr = fmt.Sprintf("%d/%s", p.Port, p.Protocol)
		}
		portStrs = append(portStrs, portStr)
	}

	return strings.Join(portStrs, ",")
}

// renderDeploymentsList renders deployments section with selectable items
func (m *Model) renderDeploymentsList(sectionOffset int) ([]string, int) {
	var rows []string

	deployments := m.clusterData.Deployments
	totalDeployments := len(deployments)

	header := fmt.Sprintf("%s  (Total: %d)",
		StyleSubHeader.Render(m.T("workloads.deployments.title")),
		totalDeployments,
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName      = 35
		colNamespace = 15
		colReady     = 15
		colUpToDate  = 12
		colAvailable = 12
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.ready"), colReady),
		padRight(m.T("columns.up_to_date"), colUpToDate),
		padRight(m.T("columns.available"), colAvailable),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	if totalDeployments == 0 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("workloads.no_deployments")))
		return rows, 0
	}

	for i, deploy := range deployments {
		ready := fmt.Sprintf("%d/%d", deploy.ReadyReplicas, deploy.Replicas)
		if deploy.ReadyReplicas == deploy.Replicas {
			ready = StyleStatusReady.Render(ready)
		} else if deploy.ReadyReplicas == 0 {
			ready = StyleStatusNotReady.Render(ready)
		} else {
			ready = StyleStatusPending.Render(ready)
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(truncate(deploy.Name, colName), colName),
			padRight(truncate(deploy.Namespace, colNamespace), colNamespace),
			padRight(ready, colReady),
			padRight(fmt.Sprintf("%d", deploy.UpdatedReplicas), colUpToDate),
			padRight(fmt.Sprintf("%d", deploy.AvailableReplicas), colAvailable),
		)

		globalIndex := sectionOffset + i
		if globalIndex == m.selectedIndex {
			row = StyleSelected.Render("> " + row)
		} else {
			row = "  " + row
		}

		rows = append(rows, row)
	}

	return rows, totalDeployments
}

// renderStatefulSetsList renders statefulsets section with selectable items
func (m *Model) renderStatefulSetsList(sectionOffset int) ([]string, int) {
	var rows []string

	statefulsets := m.clusterData.StatefulSets
	totalStatefulSets := len(statefulsets)

	header := fmt.Sprintf("%s  (Total: %d)",
		StyleSubHeader.Render(m.T("workloads.statefulsets.title")),
		totalStatefulSets,
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName      = 35
		colNamespace = 15
		colReady     = 15
		colCurrent   = 12
		colUpdated   = 12
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.ready"), colReady),
		padRight(m.T("columns.current"), colCurrent),
		padRight(m.T("columns.updated"), colUpdated),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	if totalStatefulSets == 0 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("workloads.no_statefulsets")))
		return rows, 0
	}

	for i, sts := range statefulsets {
		ready := fmt.Sprintf("%d/%d", sts.ReadyReplicas, sts.Replicas)
		if sts.ReadyReplicas == sts.Replicas {
			ready = StyleStatusReady.Render(ready)
		} else if sts.ReadyReplicas == 0 {
			ready = StyleStatusNotReady.Render(ready)
		} else {
			ready = StyleStatusPending.Render(ready)
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(truncate(sts.Name, colName), colName),
			padRight(truncate(sts.Namespace, colNamespace), colNamespace),
			padRight(ready, colReady),
			padRight(fmt.Sprintf("%d", sts.CurrentReplicas), colCurrent),
			padRight(fmt.Sprintf("%d", sts.UpdatedReplicas), colUpdated),
		)

		globalIndex := sectionOffset + i
		if globalIndex == m.selectedIndex {
			row = StyleSelected.Render("> " + row)
		} else {
			row = "  " + row
		}

		rows = append(rows, row)
	}

	return rows, totalStatefulSets
}

// renderDaemonSetsList renders daemonsets section with selectable items
func (m *Model) renderDaemonSetsList(sectionOffset int) ([]string, int) {
	var rows []string

	daemonsets := m.clusterData.DaemonSets
	totalDaemonSets := len(daemonsets)

	header := fmt.Sprintf("%s  (Total: %d)",
		StyleSubHeader.Render(m.T("workloads.daemonsets.title")),
		totalDaemonSets,
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName      = 35
		colNamespace = 15
		colDesired   = 12
		colCurrent   = 12
		colReady     = 12
		colAvailable = 12
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.desired"), colDesired),
		padRight(m.T("columns.current"), colCurrent),
		padRight(m.T("columns.ready"), colReady),
		padRight(m.T("columns.available"), colAvailable),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	if totalDaemonSets == 0 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("workloads.no_daemonsets")))
		return rows, 0
	}

	for i, ds := range daemonsets {
		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(truncate(ds.Name, colName), colName),
			padRight(truncate(ds.Namespace, colNamespace), colNamespace),
			padRight(fmt.Sprintf("%d", ds.DesiredNumberScheduled), colDesired),
			padRight(fmt.Sprintf("%d", ds.CurrentNumberScheduled), colCurrent),
			padRight(fmt.Sprintf("%d", ds.NumberReady), colReady),
			padRight(fmt.Sprintf("%d", ds.NumberAvailable), colAvailable),
		)

		globalIndex := sectionOffset + i
		if globalIndex == m.selectedIndex {
			row = StyleSelected.Render("> " + row)
		} else {
			row = "  " + row
		}

		rows = append(rows, row)
	}

	return rows, totalDaemonSets
}

// renderCronJobsList renders cronjobs section with selectable items
func (m *Model) renderCronJobsList(sectionOffset int) ([]string, int) {
	var rows []string

	cronjobs := m.clusterData.CronJobs
	totalCronJobs := len(cronjobs)

	header := fmt.Sprintf("%s  (Total: %d)",
		StyleSubHeader.Render(m.T("workloads.cronjobs.title")),
		totalCronJobs,
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName         = 30
		colNamespace    = 15
		colSchedule     = 20
		colSuspend      = 10
		colActive       = 10
		colLastSchedule = 20
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.schedule"), colSchedule),
		padRight(m.T("columns.suspend"), colSuspend),
		padRight(m.T("columns.active"), colActive),
		padRight(m.T("columns.last_schedule"), colLastSchedule),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	if totalCronJobs == 0 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("workloads.no_cronjobs")))
		return rows, 0
	}

	for i, cj := range cronjobs {
		suspend := m.T("workloads.cronjobs.suspend_false")
		if cj.Suspend {
			suspend = StyleWarning.Render(m.T("workloads.cronjobs.suspend_true"))
		}

		lastSchedule := "-"
		if !cj.LastScheduleTime.IsZero() {
			lastSchedule = formatAge(time.Since(cj.LastScheduleTime))
		}

		active := fmt.Sprintf("%d", cj.Active)
		if cj.Active > 0 {
			active = StyleStatusRunning.Render(active)
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(truncate(cj.Name, colName), colName),
			padRight(truncate(cj.Namespace, colNamespace), colNamespace),
			padRight(truncate(cj.Schedule, colSchedule), colSchedule),
			padRight(suspend, colSuspend),
			padRight(active, colActive),
			padRight(lastSchedule, colLastSchedule),
		)

		globalIndex := sectionOffset + i
		if globalIndex == m.selectedIndex {
			row = StyleSelected.Render("> " + row)
		} else {
			row = "  " + row
		}

		rows = append(rows, row)
	}

	return rows, totalCronJobs
}
