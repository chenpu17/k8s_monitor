package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderLogs renders the logs viewer
func (m *Model) renderLogs() string {
	if m.selectedPod == nil {
		return m.T("logs.no_pod")
	}

	var sections []string

	// Header
	title := StyleHeader.Render(m.TF("logs.title", map[string]interface{}{
		"Namespace": m.selectedPod.Namespace,
		"Pod":       m.selectedPod.Name,
	}))
	container := StyleTextSecondary.Render(m.TF("logs.container", map[string]interface{}{
		"Name": m.selectedContainer,
	}))
	header := lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", container)
	sections = append(sections, header)

	// Show search bar if in search mode
	if m.logsSearchMode {
		searchBar := StyleKey.Render("Search: ") + m.logsSearchText + StyleTextMuted.Render("_")
		sections = append(sections, searchBar)
	}
	sections = append(sections, "")

	// Show error if any
	if m.logsError != "" {
		errorMsg := StyleError.Render(m.TF("logs.error", map[string]interface{}{
			"Error": m.logsError,
		}))
		sections = append(sections, errorMsg)
		return strings.Join(sections, "\n")
	}

	// Show loading if no logs yet
	if m.containerLogs == "" {
		sections = append(sections, StyleTextMuted.Render(m.T("logs.loading")))
		return strings.Join(sections, "\n")
	}

	// Use cached log lines (cache is updated in Update when logs change)
	logLines := m.cachedLogLines
	if len(logLines) == 0 {
		// Fallback: split if cache is empty (shouldn't happen normally)
		logLines = strings.Split(m.containerLogs, "\n")
	}

	// Filter lines if search is active (use simple string matching, not regex)
	var displayLines []string
	var originalIndices []int

	if m.logsSearchText != "" {
		lowerSearch := strings.ToLower(m.logsSearchText)
		for idx, line := range logLines {
			if strings.Contains(strings.ToLower(line), lowerSearch) {
				displayLines = append(displayLines, line)
				originalIndices = append(originalIndices, idx)
			}
		}
	} else {
		displayLines = logLines
		originalIndices = make([]int, len(logLines))
		for i := range originalIndices {
			originalIndices[i] = i
		}
	}

	// Apply scroll offset with proper bounds checking
	maxVisible := m.height - 8
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalLines := len(logLines)
	filteredCount := len(displayLines)

	// Calculate scroll bounds
	maxScroll := filteredCount - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Use clamped scroll offset for display only
	scrollOffset := m.logsScrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	startIdx := scrollOffset
	endIdx := startIdx + maxVisible
	if endIdx > filteredCount {
		endIdx = filteredCount
	}

	visibleLines := displayLines[startIdx:endIdx]
	visibleIndices := originalIndices[startIdx:endIdx]

	// Render log lines with line numbers (simplified highlighting for performance)
	var renderedLines []string

	for i, line := range visibleLines {
		lineNum := visibleIndices[i] + 1
		lineNumStr := StyleTextMuted.Render(fmt.Sprintf("%4d│ ", lineNum))

		// Simple highlighting: only highlight search term, skip expensive log level patterns during scroll
		displayLine := line
		if m.logsSearchText != "" {
			displayLine = highlightSearchTermSimple(line, m.logsSearchText)
		}

		// Truncate long lines instead of wrapping (much faster)
		maxLineWidth := m.width - 10
		if maxLineWidth < 20 {
			maxLineWidth = 20
		}
		if len(displayLine) > maxLineWidth {
			displayLine = displayLine[:maxLineWidth-3] + "..."
		}

		renderedLines = append(renderedLines, lineNumStr+displayLine)
	}

	sections = append(sections, renderedLines...)

	// Status bar
	var statusParts []string

	if m.logsSearchText != "" {
		matchInfo := fmt.Sprintf("🔍 %d/%d matches", filteredCount, totalLines)
		statusParts = append(statusParts, StyleKey.Render(matchInfo))
	} else if filteredCount > maxVisible {
		scrollPosStr := m.TF("logs.lines_range", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": filteredCount,
		})
		statusParts = append(statusParts, scrollPosStr)
	} else {
		statusParts = append(statusParts, m.TF("logs.lines_total", map[string]interface{}{
			"Total": totalLines,
		}))
	}

	if m.logsAutoScroll {
		statusParts = append(statusParts, "🔄 "+m.T("logs.auto_follow"))
	} else {
		statusParts = append(statusParts, "⏸ "+m.T("logs.paused"))
	}

	if !m.logsLastUpdate.IsZero() {
		elapsed := time.Since(m.logsLastUpdate)
		timeStr := m.TF("logs.updated_ago", map[string]interface{}{
			"Seconds": int(elapsed.Seconds()),
		})
		statusParts = append(statusParts, timeStr)
	} else {
		statusParts = append(statusParts, m.T("logs.loading"))
	}

	var helpText string
	if m.logsSearchMode {
		helpText = "Type to search • ESC to exit search"
	} else if filteredCount > maxVisible {
		helpText = m.T("logs.help.scroll") + " • / to search"
	} else {
		helpText = m.T("logs.help.back") + " • / to search"
	}

	scrollInfo := StyleTextMuted.Render(fmt.Sprintf("\n%s %s",
		strings.Join(statusParts, " • "), helpText))
	sections = append(sections, scrollInfo)

	return strings.Join(sections, "\n")
}
