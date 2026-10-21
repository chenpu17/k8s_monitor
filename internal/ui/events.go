package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderEvents renders the events view
func (m *Model) renderEvents() string {
	events := m.getFilteredEvents()
	if len(events) == 0 {
		return m.T("msg.no_events")
	}

	// Sort events before rendering and cache
	m.cachedSortedEvents = m.getSortedEvents(events)

	// Header
	header := m.renderEventsHeader(m.cachedSortedEvents)

	// Rows that fit under the header/table header/stats line.
	rows := m.eventsRowBudget(header)

	// Event list. It clamps the scroll offset, so it runs before the stats line,
	// which reports the window it ended up showing.
	eventList := m.renderEventsList(m.cachedSortedEvents, rows)

	// Footer with stats
	footer := m.renderEventsFooter(m.cachedSortedEvents, rows)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		eventList,
		"",
		footer,
	)

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

// renderEventsHeader renders the events view header
func (m *Model) renderEventsHeader(events []*model.EventData) string {
	title := StyleHeader.Render("⚡ " + m.T("views.events.title"))
	summary := fmt.Sprintf("%s: %d", m.T("common.total"), len(events))

	// Show filter info if active
	if m.filterEventType != "" {
		summary += " " + m.TF("events.type_filter", map[string]interface{}{
			"Type": m.filterEventType,
		})
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		StyleTextSecondary.Render(summary),
	)
}

// eventsRowBudget returns how many event rows fit on screen under the view
// header, the table header and the stats line. The active search panel is held
// back the same way, or its lines would be appended below the budget and
// silently trimmed.
func (m *Model) eventsRowBudget(header string) int {
	// header + blank + table header + separator + rows + blank + stats line
	const fixed = 5
	return max(1, m.contentBudget()-lineCount(header)-fixed-m.panelLines())
}

// renderEventsList renders the list of events, showing at most maxVisible rows.
func (m *Model) renderEventsList(events []*model.EventData, maxVisible int) string {
	var rows []string

	// Table header - define fixed column widths
	const (
		colType    = 10
		colReason  = 25
		colObject  = 30
		colMessage = 50
		colCount   = 8
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(m.T("columns.type"), colType),
		padRight(m.T("columns.reason"), colReason),
		padRight(m.T("columns.object"), colObject),
		padRight(m.T("columns.message"), colMessage),
		padRight(m.T("columns.count"), colCount),
	)
	rows = append(rows, StyleHeader.Render(headerRow))
	rows = append(rows, strings.Repeat("─", colType+colReason+colObject+colMessage+colCount+8))

	// The renderer reports the rows it printed so the selection-follow logic in
	// Update scrolls by the same amount the user sees.
	m.listRows = maxVisible

	totalEvents := len(events)

	// Calculate scroll bounds (read-only, don't modify state in View)
	maxScroll := totalEvents - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Use clamped scroll offset for display only
	scrollOffset := m.scrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	startIdx := scrollOffset
	endIdx := startIdx + maxVisible
	if endIdx > totalEvents {
		endIdx = totalEvents
	}

	visibleEvents := events[startIdx:endIdx]

	// Event rows with selection highlighting
	for i, event := range visibleEvents {
		absoluteIndex := startIdx + i
		row := m.renderEventRow(event, colType, colReason, colObject, colMessage, colCount)

		// Highlight selected row
		if absoluteIndex == m.selectedIndex {
			row = StyleSelected.Render(row)
		}

		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderEventRow renders a single event row
func (m *Model) renderEventRow(event *model.EventData, colType, colReason, colObject, colMessage, colCount int) string {
	// Event type with color
	eventType := event.Type
	if event.Type == "Warning" {
		eventType = StyleStatusNotReady.Render(event.Type)
	} else if event.Type == "Normal" {
		eventType = StyleStatusReady.Render(event.Type)
	}

	// Reason
	reason := truncate(event.Reason, colReason)

	// Involved object
	object := truncate(event.InvolvedObject, colObject)

	// Message
	message := truncate(event.Message, colMessage)

	// Count
	count := fmt.Sprintf("%d", event.Count)

	return fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(eventType, colType),
		padRight(reason, colReason),
		padRight(object, colObject),
		padRight(message, colMessage),
		padRight(count, colCount),
	)
}

// renderEventsFooter renders the events view footer
// renderEventsFooter renders the events view footer, including the position of
// the maxVisible-row window the list is showing.
func (m *Model) renderEventsFooter(events []*model.EventData, maxVisible int) string {
	// Count event types
	warning, normal := 0, 0
	for _, event := range events {
		switch event.Type {
		case "Warning":
			warning++
		case "Normal":
			normal++
		}
	}

	totalEvents := len(events)
	stats := fmt.Sprintf("%s %s  %s %s  %s %d",
		m.T("events.type.warning")+":",
		StyleStatusNotReady.Render(fmt.Sprintf("%d", warning)),
		m.T("events.type.normal")+":",
		StyleStatusReady.Render(fmt.Sprintf("%d", normal)),
		m.T("common.total")+":",
		totalEvents,
	)

	// Add scroll position indicator if there are more items than visible
	if totalEvents > maxVisible {
		scrollInfo := fmt.Sprintf("  [%d-%d %s %d]",
			m.scrollOffset+1,
			min(m.scrollOffset+maxVisible, totalEvents),
			m.T("common.of"),
			totalEvents,
		)
		stats += StyleTextMuted.Render(scrollInfo)
	}

	return StyleTextSecondary.Render(stats)
}

// getSortedEvents returns a sorted copy of events (by time, most recent first)
func (m *Model) getSortedEvents(inputEvents []*model.EventData) []*model.EventData {
	if len(inputEvents) == 0 {
		return []*model.EventData{}
	}

	// Create a copy to avoid modifying original
	events := make([]*model.EventData, len(inputEvents))
	copy(events, inputEvents)

	// Sort by LastTimestamp (most recent first) using sort.Slice (O(n log n))
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTimestamp.After(events[j].LastTimestamp)
	})

	return events
}
