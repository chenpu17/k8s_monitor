package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderAlerts renders the alerts view
func (m *Model) renderAlerts() string {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return m.T("msg.no_data")
	}

	alerts := m.clusterData.Summary.Alerts
	if len(alerts) == 0 {
		return m.renderNoAlerts()
	}

	// Header
	header := m.renderAlertsHeader(alerts)

	// Alert list
	alertList := m.renderAlertsList(alerts)

	// Footer with stats
	footer := m.renderAlertsFooter(alerts)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		alertList,
		"",
		footer,
	)

	return content
}

// renderNoAlerts renders a message when there are no alerts
func (m *Model) renderNoAlerts() string {
	var lines []string

	lines = append(lines, StyleHeader.Render("🚨 "+m.T("views.alerts.title")))
	lines = append(lines, "")
	lines = append(lines, "")
	lines = append(lines, StyleStatusReady.Render("✅ "+m.T("msg.all_healthy")))
	lines = append(lines, "")
	lines = append(lines, StyleTextSecondary.Render(m.T("msg.no_alerts_detected")))
	lines = append(lines, "")

	// Show cluster health summary
	if m.clusterData != nil && m.clusterData.Summary != nil {
		summary := m.clusterData.Summary
		lines = append(lines, StyleTextMuted.Render(m.T("stats.cluster_summary")))
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s %s / %d %s",
			m.T("common.nodes")+":",
			StyleStatusReady.Render(fmt.Sprintf("%d", summary.ReadyNodes)),
			summary.TotalNodes,
			m.T("status.ready"),
		))
		lines = append(lines, fmt.Sprintf("  %s  %s / %d %s",
			m.T("common.pods")+":",
			StyleStatusRunning.Render(fmt.Sprintf("%d", summary.RunningPods)),
			summary.TotalPods,
			m.T("status.running"),
		))

		if summary.PendingPods > 0 {
			lines = append(lines, fmt.Sprintf("         %s %s",
				StyleStatusPending.Render(fmt.Sprintf("%d", summary.PendingPods)),
				m.T("status.pending"),
			))
		}
		if summary.FailedPods > 0 {
			lines = append(lines, fmt.Sprintf("         %s %s",
				StyleStatusNotReady.Render(fmt.Sprintf("%d", summary.FailedPods)),
				m.T("status.failed"),
			))
		}
	}

	return strings.Join(lines, "\n")
}

// renderAlertsHeader renders the alerts view header
func (m *Model) renderAlertsHeader(alerts []model.Alert) string {
	title := StyleHeader.Render("🚨 " + m.T("views.alerts.title"))

	// Count by severity
	critical, warning, info := 0, 0, 0
	for _, alert := range alerts {
		switch alert.Severity {
		case model.AlertSeverityCritical:
			critical++
		case model.AlertSeverityWarning:
			warning++
		case model.AlertSeverityInfo:
			info++
		}
	}

	summary := fmt.Sprintf("%s %d", m.T("common.total")+":", len(alerts))
	if critical > 0 {
		summary += fmt.Sprintf(" • %s: %d", StyleDanger.Render(m.T("stats.critical")), critical)
	}
	if warning > 0 {
		summary += fmt.Sprintf(" • %s: %d", StyleWarning.Render(m.T("stats.warning")), warning)
	}
	if info > 0 {
		summary += fmt.Sprintf(" • %s: %d", StyleTextSecondary.Render(m.T("stats.info")), info)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		StyleTextSecondary.Render(summary),
	)
}

// renderAlertsList renders the list of alerts
func (m *Model) renderAlertsList(alerts []model.Alert) string {
	var rows []string

	// Group alerts by severity FIRST (before slicing)
	var critical, warning, info []model.Alert
	for _, alert := range alerts {
		switch alert.Severity {
		case model.AlertSeverityCritical:
			critical = append(critical, alert)
		case model.AlertSeverityWarning:
			warning = append(warning, alert)
		case model.AlertSeverityInfo:
			info = append(info, alert)
		}
	}

	// Flatten into display order: Critical → Warning → Info
	flatAlerts := make([]model.Alert, 0, len(alerts))
	flatAlerts = append(flatAlerts, critical...)
	flatAlerts = append(flatAlerts, warning...)
	flatAlerts = append(flatAlerts, info...)

	// Calculate visible range based on scroll
	maxVisible := m.height - 12
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalAlerts := len(flatAlerts)

	// Clamp scroll offset to valid range to prevent panic when alert count shrinks
	maxScroll := totalAlerts - maxVisible
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
	if endIdx > totalAlerts {
		endIdx = totalAlerts
	}

	visibleAlerts := flatAlerts[startIdx:endIdx]

	// Render grouped sections (Critical → Warning → Info)
	criticalCount := 0
	warningCount := 0
	infoCount := 0

	for i, alert := range visibleAlerts {
		absoluteIdx := startIdx + i

		// Section header when entering a new severity group
		if absoluteIdx < len(critical) && criticalCount == 0 {
			rows = append(rows, StyleDanger.Bold(true).Render(m.T("stats.critical")))
			rows = append(rows, "")
		} else if absoluteIdx >= len(critical) && absoluteIdx < len(critical)+len(warning) && warningCount == 0 {
			if criticalCount > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, StyleWarning.Bold(true).Render(m.T("stats.warning")))
			rows = append(rows, "")
		} else if absoluteIdx >= len(critical)+len(warning) && infoCount == 0 {
			if criticalCount > 0 || warningCount > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, StyleTextSecondary.Bold(true).Render(m.T("stats.info")))
			rows = append(rows, "")
		}

		// Render alert row
		row := m.renderAlertRow(alert)
		if absoluteIdx == m.selectedIndex {
			row = StyleSelected.Render(row)
		}
		rows = append(rows, row)

		// Count alerts in each section
		if absoluteIdx < len(critical) {
			criticalCount++
		} else if absoluteIdx < len(critical)+len(warning) {
			warningCount++
		} else {
			infoCount++
		}
	}

	return strings.Join(rows, "\n")
}

// renderAlertRow renders a single alert row
func (m *Model) renderAlertRow(alert model.Alert) string {
	// Format resource identifier
	var resource string
	if alert.Namespace != "" {
		resource = fmt.Sprintf("%s: %s/%s", alert.ResourceType, alert.Namespace, alert.ResourceName)
	} else {
		resource = fmt.Sprintf("%s: %s", alert.ResourceType, alert.ResourceName)
	}

	// Format message
	message := alert.Message

	// Format value/threshold
	var valueStr string
	if alert.Threshold != "" {
		valueStr = fmt.Sprintf("%s (threshold: %s)", alert.Value, alert.Threshold)
	} else if alert.Value != "" {
		valueStr = alert.Value
	}

	// Build the row
	var parts []string
	parts = append(parts, fmt.Sprintf("  • %s", resource))
	parts = append(parts, fmt.Sprintf("    %s", message))
	if valueStr != "" {
		parts = append(parts, fmt.Sprintf("    %s", StyleTextMuted.Render(valueStr)))
	}

	return strings.Join(parts, "\n")
}

// renderAlertsFooter renders the alerts view footer
func (m *Model) renderAlertsFooter(alerts []model.Alert) string {
	totalAlerts := len(alerts)

	// Count by category
	categories := make(map[string]int)
	for _, alert := range alerts {
		categories[alert.Category]++
	}

	var categoryStats []string
	for category, count := range categories {
		categoryStats = append(categoryStats, fmt.Sprintf("%s: %d", category, count))
	}

	stats := m.TF("stats.total_alerts", map[string]interface{}{
		"Count": totalAlerts,
	})
	if len(categoryStats) > 0 {
		stats += " • " + strings.Join(categoryStats, " • ")
	}

	// Add scroll position indicator if there are more items than visible
	maxVisible := m.height - 12
	if maxVisible < 1 {
		maxVisible = 1
	}
	if totalAlerts > maxVisible {
		scrollInfo := fmt.Sprintf("  [%d-%d %s %d]",
			m.scrollOffset+1,
			min(m.scrollOffset+maxVisible, totalAlerts),
			m.T("common.of"),
			totalAlerts,
		)
		stats += StyleTextMuted.Render(scrollInfo)
	}

	return StyleTextSecondary.Render(stats)
}
