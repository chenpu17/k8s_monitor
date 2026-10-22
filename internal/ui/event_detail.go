package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderEventDetail renders the event detail view
func (m *Model) renderEventDetail() string {
	if m.selectedEvent == nil {
		return m.T("detail.event.no_selected")
	}

	event := m.selectedEvent

	// Collect all content lines
	var allLines []string

	// Event header
	allLines = append(allLines, m.renderEventDetailHeader(event))
	allLines = append(allLines, "")

	// Event basic info
	basicInfo := m.renderEventBasicInfo(event)
	allLines = append(allLines, strings.Split(basicInfo, "\n")...)
	allLines = append(allLines, "")

	// Event timing info
	timingInfo := m.renderEventTimingInfo(event)
	allLines = append(allLines, strings.Split(timingInfo, "\n")...)

	// Apply scroll offset
	maxVisible := m.height - 8 // Reserve space for header/footer
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Use local variable to avoid state mutation in View
	detailScrollOffset := m.detailScrollOffset

	// Clamp scroll offset to valid range
	totalLines := len(allLines)
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

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator if needed
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render("\n" + m.TF("detail.scroll_indicator", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		}))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderEventDetailHeader renders the event detail view header
func (m *Model) renderEventDetailHeader(event *model.EventData) string {
	title := StyleHeader.Render(m.TF("detail.event.title", map[string]interface{}{
		"Reason": event.Reason,
	}))

	// Event type with color
	eventType := event.Type
	if event.Type == "Warning" {
		eventType = StyleStatusNotReady.Render(event.Type)
	} else if event.Type == "Normal" {
		eventType = StyleStatusReady.Render(event.Type)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		eventType,
	)
}

// renderEventBasicInfo renders basic event information
func (m *Model) renderEventBasicInfo(event *model.EventData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.event.basic_info")))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.event.type")),
		event.Type))

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.reason")),
		event.Reason))

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.event.involved_object")),
		event.InvolvedObject))

	if event.InvolvedNamespace != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.namespace")),
			event.InvolvedNamespace))
	}

	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render(m.T("detail.event.count")),
		event.Count))

	info = append(info, "")
	info = append(info, fmt.Sprintf("  %s:",
		StyleTextSecondary.Render(m.T("detail.field.message"))))

	// Word wrap the message
	messageLines := wrapText(event.Message, 70)
	for _, line := range messageLines {
		info = append(info, fmt.Sprintf("    %s", line))
	}

	return strings.Join(info, "\n")
}

// renderEventTimingInfo renders event timing information
func (m *Model) renderEventTimingInfo(event *model.EventData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.event.timing_info")))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.event.first_seen")),
		event.FirstTimestamp.Format("2006-01-02 15:04:05")))

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.event.last_seen")),
		event.LastTimestamp.Format("2006-01-02 15:04:05")))

	// Calculate time since last occurrence
	timeSince := event.LastTimestamp.Local().Format("2006-01-02 15:04:05")
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.event.last_occurrence")),
		timeSince))

	return strings.Join(info, "\n")
}

// wrapText wraps text to a specified width
func wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if len(currentLine)+len(word)+1 <= width {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
