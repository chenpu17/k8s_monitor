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

	// Split logs into lines
	logLines := strings.Split(m.containerLogs, "\n")

	// Filter lines if search is active
	var displayLines []string
	var originalIndices []int // Track original line numbers for filtered results

	if m.logsSearchText != "" {
		// Filter matching lines
		for idx, line := range logLines {
			if strings.Contains(strings.ToLower(stripANSI(line)), strings.ToLower(m.logsSearchText)) {
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
	// Use consistent maxVisible calculation across the codebase
	maxVisible := m.height - 8 // Reserve space for header/footer (consistent with Down/Up key handling)
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalLines := len(logLines)
	filteredCount := len(displayLines)

	// Calculate scroll bounds (read-only, don't modify state in View)
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

	// Render log lines with line numbers, highlighting, and wrapping
	var renderedLines []string
	indent := "     │ " // 5 spaces + separator to align with line number (7 chars)

	for i, line := range visibleLines {
		lineNum := visibleIndices[i] + 1
		lineNumStr := StyleTextMuted.Render(fmt.Sprintf("%4d│ ", lineNum))

		// Apply log level highlighting and search highlighting
		highlightedLine := highlightLogLine(line, m.logsSearchText)

		// Calculate max width for log content (reserve space for line number)
		maxLineWidth := m.width - 10
		if maxLineWidth < 10 {
			maxLineWidth = 10 // Minimum width to prevent panic
		}

		// Wrap long lines instead of truncating them
		wrappedLines := wrapLine(highlightedLine, maxLineWidth, 7) // indent width is 7

		// Add line number to first line, indent to continuation lines
		for j, wrappedLine := range wrappedLines {
			if j == 0 {
				renderedLines = append(renderedLines, lineNumStr+wrappedLine)
			} else {
				renderedLines = append(renderedLines, StyleTextMuted.Render(indent)+wrappedLine)
			}
		}
	}

	sections = append(sections, renderedLines...)

	// Always show status bar for user feedback
	var statusParts []string

	// Show search results if filtering
	if m.logsSearchText != "" {
		matchInfo := fmt.Sprintf("🔍 %d/%d matches", filteredCount, totalLines)
		statusParts = append(statusParts, StyleKey.Render(matchInfo))
	} else {
		// Scroll position (only if scrollable)
		if filteredCount > maxVisible {
			scrollPosStr := m.TF("logs.lines_range", map[string]interface{}{
				"Start": startIdx + 1,
				"End":   endIdx,
				"Total": filteredCount,
			})
			statusParts = append(statusParts, scrollPosStr)
		} else {
			// Show total even if not scrollable
			statusParts = append(statusParts, m.TF("logs.lines_total", map[string]interface{}{
				"Total": totalLines,
			}))
		}
	}

	// Auto-scroll status (always show so user knows the mode)
	if m.logsAutoScroll {
		statusParts = append(statusParts, "🔄 "+m.T("logs.auto_follow"))
	} else {
		statusParts = append(statusParts, "⏸ "+m.T("logs.paused"))
	}

	// Last update time (always show so user knows logs are refreshing)
	if !m.logsLastUpdate.IsZero() {
		elapsed := time.Since(m.logsLastUpdate)
		timeStr := m.TF("logs.updated_ago", map[string]interface{}{
			"Seconds": int(elapsed.Seconds()),
		})
		statusParts = append(statusParts, timeStr)
	} else {
		statusParts = append(statusParts, m.T("logs.loading"))
	}

	// Build status bar
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
