package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/diagnostic"
	"github.com/chenpu/k8s-monitor/internal/model"
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

	// Alert list, windowed to the lines that fit between the header and the
	// stats line
	flatAlerts := alertsBySeverity(alerts)
	listLines, first, end := m.alertWindowLines(flatAlerts, m.scrollOffset, m.alertListBudget())

	// Footer with stats
	footer := m.renderAlertsFooter(alerts, first, end, len(flatAlerts))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		strings.Join(listLines, "\n"),
		"",
		footer,
	)

	return content
}

// alertListBudget returns how many lines the alert list itself may occupy: the
// content budget minus the view header, the stats line and the blank lines
// between them. The alert rows are several lines tall, so the number of alerts
// that fit is derived from the lines they actually render, not from a row count.
func (m *Model) alertListBudget() int {
	// header + blank + list + blank + stats line
	const fixed = 4
	return max(1, m.contentBudget()-fixed)
}

// alertsBySeverity returns the alerts in display order: critical, warning, info.
func alertsBySeverity(alerts []model.Alert) []model.Alert {
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

	flatAlerts := make([]model.Alert, 0, len(alerts))
	flatAlerts = append(flatAlerts, critical...)
	flatAlerts = append(flatAlerts, warning...)
	flatAlerts = append(flatAlerts, info...)
	return flatAlerts
}

// alertWindowLines renders the window of alerts that starts at index start and
// fits in budget lines, and returns the lines along with the exclusive index of
// the last alert shown. The first alert of a severity always carries its section
// header, so a window that opens in the middle of a group still says which group
// it is; a single alert taller than the budget is still shown (never an empty
// window).
func (m *Model) alertWindowLines(flatAlerts []model.Alert, start, budget int) (lines []string, first, end int) {
	if len(flatAlerts) == 0 {
		return nil, 0, 0
	}
	start = max(0, min(start, len(flatAlerts)-1))
	first = start
	end = start
	for i := start; i < len(flatAlerts); i++ {
		// Section header: two lines for the group the window opens with, three
		// (a separating blank line on top) for every later group.
		extra := 0
		if i == start {
			extra = 2
		} else if flatAlerts[i].Severity != flatAlerts[i-1].Severity {
			extra = 3
		}

		row := m.renderAlertRow(flatAlerts[i])
		if i == m.selectedIndex {
			row = StyleSelected.Render(row)
		}
		rowLines := strings.Split(row, "\n")

		if len(lines) > 0 && len(lines)+extra+len(rowLines) > budget {
			break
		}
		if extra == 3 {
			lines = append(lines, "")
		}
		if extra > 0 {
			lines = append(lines, m.alertSectionHeader(flatAlerts[i].Severity), "")
		}
		lines = append(lines, rowLines...)
		end = i + 1
	}
	return lines, first, end
}

// alertSectionHeader renders the severity heading for a group of alerts.
func (m *Model) alertSectionHeader(severity model.AlertSeverity) string {
	switch severity {
	case model.AlertSeverityCritical:
		return StyleDanger.Bold(true).Render(m.T("stats.critical"))
	case model.AlertSeverityWarning:
		return StyleWarning.Bold(true).Render(m.T("stats.warning"))
	default:
		return StyleTextSecondary.Bold(true).Render(m.T("stats.info"))
	}
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

// alertViewport returns the window of alerts shown for the given first-visible
// index: the exclusive index one past the last alert in the window, and the
// lines the window renders. Update uses it to keep the selected alert on screen
// (the rows are several lines tall, so the number of alerts that fit is not the
// terminal height minus a constant).
func (m *Model) alertViewport(start int) (first, end int) {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return 0, 0
	}
	_, first, end = m.alertWindowLines(alertsBySeverity(m.clusterData.Summary.Alerts), start, m.alertListBudget())
	return first, end
}

// alertMaxScroll returns the first-visible index of the bottom page: the
// smallest offset whose window reaches the last alert. Anything larger would
// show less of the last alerts without reaching anything new.
func (m *Model) alertMaxScroll() int {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return 0
	}
	flatAlerts := alertsBySeverity(m.clusterData.Summary.Alerts)
	budget := m.alertListBudget()
	for start := 0; start < len(flatAlerts); start++ {
		if _, _, end := m.alertWindowLines(flatAlerts, start, budget); end >= len(flatAlerts) {
			return start
		}
	}
	return 0
}

// alertsScrollToSelection moves the window so the selected alert is inside it.
// The offset is first clamped to the bottom page, so a stale offset from before
// a refresh cannot hide the selection.
func (m *Model) alertsScrollToSelection() {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		m.scrollOffset = 0
		return
	}
	total := len(m.clusterData.Summary.Alerts)
	if total == 0 {
		m.scrollOffset = 0
		return
	}
	if m.selectedIndex >= total {
		m.selectedIndex = total - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}

	m.scrollOffset = min(m.scrollOffset, m.alertMaxScroll())
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > m.selectedIndex {
		m.scrollOffset = m.selectedIndex
		return
	}
	for _, end := m.alertViewport(m.scrollOffset); end <= m.selectedIndex; {
		m.scrollOffset++
		if m.scrollOffset >= total {
			break
		}
		_, end = m.alertViewport(m.scrollOffset)
	}
}

// alertsPageBy moves the alert selection by one page - the alerts that fit on
// screen - and keeps the selection visible.
func (m *Model) alertsPageBy(direction int) {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return
	}
	total := len(m.clusterData.Summary.Alerts)
	if total == 0 {
		return
	}
	first, end := m.alertViewport(m.scrollOffset)
	page := max(1, end-first)

	m.selectedIndex = max(0, min(m.selectedIndex+direction*page, total-1))
	m.scrollOffset = max(0, min(m.scrollOffset+direction*page, total-1))
	m.alertsScrollToSelection()
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

	// Format message: keyed alerts render through the localizer so the message
	// follows the UI locale; alerts without a key carry raw cluster-provided
	// text and fall back to the stored (English) Message.
	message := alert.Message
	if alert.MessageKey != "" {
		message = m.TF(alert.MessageKey, alert.MessageArgs)
	}

	// Format value/threshold. Keyed values render through the localizer so the
	// value column follows the UI locale; alerts without a ValueKey keep the
	// stored (English) Value, which may be a data identifier.
	valueStr := alert.Value
	if alert.ValueKey != "" {
		valueStr = m.TF(alert.ValueKey, alert.ValueArgs)
	}
	if alert.Threshold != "" {
		valueStr = fmt.Sprintf("%s (%s: %s)", valueStr, m.T("alerts.threshold"), alert.Threshold)
	}

	// Build the row
	var parts []string
	parts = append(parts, fmt.Sprintf("  • %s", resource))
	parts = append(parts, fmt.Sprintf("    %s", message))
	if valueStr != "" {
		parts = append(parts, fmt.Sprintf("    %s", StyleTextMuted.Render(valueStr)))
	}

	// Generate locale-aware recommended action
	var recommendedAction string
	if alert.AlertType != "" {
		if m.isChinese() {
			recommendedAction = diagnostic.GetRecommendedActionChinese(alert.AlertType, alert.Namespace, alert.ResourceName)
		} else {
			recommendedAction = diagnostic.GetRecommendedAction(alert.AlertType, alert.Namespace, alert.ResourceName)
		}
	}

	// Add recommended action if available
	if recommendedAction != "" {
		parts = append(parts, fmt.Sprintf("    %s %s", StyleHighlight.Render("→"), StyleTextSecondary.Render(recommendedAction)))
	}

	return strings.Join(parts, "\n")
}

// alertCategoryKeys maps the category literals the data layer stores to the
// bundle keys the footer renders. The categories are product literals, not
// cluster-provided text, so a localized UI must translate them; a category
// without a mapping keeps its raw value rather than disappearing.
var alertCategoryKeys = map[string]string{
	"Node":     "alerts.category.node",
	"Pod":      "alerts.category.pod",
	"NPU":      "alerts.category.npu",
	"Service":  "alerts.category.service",
	"Storage":  "alerts.category.storage",
	"Resource": "alerts.category.resource",
}

// renderAlertsFooter renders the alerts view footer, including the position of
// the alert window that is on screen (alerts first..end-1 of totalAlerts).
func (m *Model) renderAlertsFooter(alerts []model.Alert, first, end, totalAlerts int) string {
	// Count by category, in a stable order: walking the map directly would
	// reshuffle the line on every refresh.
	categories := make(map[string]int)
	for _, alert := range alerts {
		categories[alert.Category]++
	}
	categoryNames := make([]string, 0, len(categories))
	for category := range categories {
		categoryNames = append(categoryNames, category)
	}
	sort.Strings(categoryNames)

	var categoryStats []string
	for _, category := range categoryNames {
		name := category
		if key, ok := alertCategoryKeys[category]; ok {
			name = m.T(key)
		}
		categoryStats = append(categoryStats, fmt.Sprintf("%s: %d", name, categories[category]))
	}

	stats := m.TF("stats.total_alerts", map[string]interface{}{
		"Count": totalAlerts,
	})
	if len(categoryStats) > 0 {
		stats += " • " + strings.Join(categoryStats, " • ")
	}

	// Scroll position indicator: the window the list actually shows, which the
	// row heights (an alert is several lines tall) make impossible to derive
	// from the terminal height alone.
	if first > 0 || end < totalAlerts {
		scrollInfo := m.TF("scroll.showing", map[string]interface{}{
			"Start": first + 1,
			"End":   end,
			"Total": totalAlerts,
		})
		stats += " " + StyleTextMuted.Render("["+scrollInfo+"]")
	}

	return StyleTextSecondary.Render(stats)
}
