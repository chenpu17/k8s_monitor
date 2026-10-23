package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// wrapLogLine wraps a single log line into multiple display lines
// Returns the wrapped lines and the number of display lines
func wrapLogLine(line string, maxWidth int) []string {
	if maxWidth < 20 {
		maxWidth = 20
	}

	// If line fits, return as-is
	lineLen := utf8.RuneCountInString(line)
	if lineLen <= maxWidth {
		return []string{line}
	}

	// Wrap line into multiple parts
	var wrapped []string
	runes := []rune(line)

	for len(runes) > 0 {
		// Calculate how many runes to take
		takeLen := maxWidth
		if takeLen > len(runes) {
			takeLen = len(runes)
		}

		wrapped = append(wrapped, string(runes[:takeLen]))
		runes = runes[takeLen:]
	}

	return wrapped
}

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
		searchBar := StyleKey.Render(m.T("logs.search_label")) + m.logsSearchText + StyleTextMuted.Render("_")
		sections = append(sections, searchBar)
	}
	if headerLines, _ := m.logsChrome(); headerLines > 1 {
		sections = append(sections, "")
	}

	// Show error if any
	if m.logsError != "" {
		errorMsg := StyleError.Render(m.TF("logs.error", map[string]interface{}{
			"Error": m.logsError,
		}))
		sections = append(sections, errorMsg)
		return strings.Join(sections, "\n")
	}

	// Show loading if no logs yet, or an empty state once the fetch
	// completed successfully but the container has no output
	if m.containerLogs == "" {
		if m.logsFetched {
			sections = append(sections, StyleTextMuted.Render(m.T("logs.empty")))
		} else {
			sections = append(sections, StyleTextMuted.Render(m.T("logs.loading")))
		}
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

	// Calculate max width for log content (reserve space for line number prefix "9999│ ")
	maxLineWidth := m.width - 8
	if maxLineWidth < 20 {
		maxLineWidth = 20
	}

	// Build wrapped display lines with their original line indices
	type wrappedLine struct {
		text          string
		originalIndex int
		isFirstLine   bool // true for first line of a logical log line
	}

	var allWrappedLines []wrappedLine
	for i, line := range displayLines {
		// Apply search highlighting before wrapping
		displayLine := line
		if m.logsSearchText != "" {
			displayLine = highlightSearchTermSimple(line, m.logsSearchText)
		}

		wrapped := wrapLogLine(displayLine, maxLineWidth)
		for j, w := range wrapped {
			allWrappedLines = append(allWrappedLines, wrappedLine{
				text:          w,
				originalIndex: originalIndices[i],
				isFirstLine:   j == 0,
			})
		}
	}

	// Apply scroll offset with proper bounds checking. The window is the
	// content area minus the lines this view spends on its title, its
	// separators and the two-line status tail; a fixed height-8 made the
	// renderer two lines taller than the content area, so fitFrame trimmed the
	// status tail (position / auto-follow) off the bottom.
	maxVisible := m.logsRows()

	totalLines := len(logLines)
	totalDisplayLines := len(allWrappedLines)

	// Calculate scroll bounds based on display lines
	maxScroll := totalDisplayLines - maxVisible
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
	if endIdx > totalDisplayLines {
		endIdx = totalDisplayLines
	}

	// Render visible wrapped lines
	var renderedLines []string

	for i := startIdx; i < endIdx; i++ {
		wl := allWrappedLines[i]
		lineNum := wl.originalIndex + 1

		var lineNumStr string
		if wl.isFirstLine {
			// First line of a logical log line: show line number
			lineNumStr = StyleTextMuted.Render(fmt.Sprintf("%4d│ ", lineNum))
		} else {
			// Continuation line: show indent marker
			lineNumStr = StyleTextMuted.Render("    │ ")
		}

		renderedLines = append(renderedLines, lineNumStr+wl.text)
	}

	sections = append(sections, renderedLines...)

	// Status bar
	var statusParts []string

	if m.logsSearchText != "" {
		matchInfo := m.TF("logs.search_matches", map[string]interface{}{
			"Matches": len(displayLines),
			"Total":   totalLines,
		})
		statusParts = append(statusParts, StyleKey.Render(matchInfo))
	} else if totalDisplayLines > maxVisible {
		scrollPosStr := m.TF("logs.lines_range", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalDisplayLines,
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
		helpText = m.T("logs.search_hint")
	} else if totalDisplayLines > maxVisible {
		helpText = m.T("logs.help.scroll") + m.T("logs.search_hint_slash")
	} else {
		helpText = m.T("logs.help.back") + m.T("logs.search_hint_slash")
	}

	statusText := StyleTextMuted.Render(fmt.Sprintf("%s %s",
		strings.Join(statusParts, " • "), helpText))
	if _, statusLines := m.logsChrome(); statusLines > 1 {
		sections = append(sections, "")
	}
	sections = append(sections, statusText)

	return strings.Join(sections, "\n")
}
