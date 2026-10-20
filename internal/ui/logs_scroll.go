package ui

// initLogsScrollPosition initializes the scroll position when first entering logs mode
func (m *Model) initLogsScrollPosition() {
	if m.containerLogs == "" {
		return
	}

	// Calculate bottom position to show latest logs - use wrapped line count
	totalLines := m.getLogsDisplayLineCount()
	maxVisible := m.height - 8
	if maxVisible < 1 {
		maxVisible = 1
	}
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.logsScrollOffset = maxScroll // Start at bottom
	m.logsAutoScroll = true        // Enable auto-scroll by default
}

// getLogsDisplayLineCount calculates the total number of display lines after wrapping
// This is used for scroll calculations to account for wrapped long lines
func (m *Model) getLogsDisplayLineCount() int {
	logLines := m.cachedLogLines
	if len(logLines) == 0 {
		return 0
	}

	// Calculate max width for log content (same as in renderLogs)
	maxLineWidth := m.width - 8
	if maxLineWidth < 20 {
		maxLineWidth = 20
	}

	// Count total display lines after wrapping
	totalDisplayLines := 0
	for _, line := range logLines {
		wrapped := wrapLogLine(line, maxLineWidth)
		totalDisplayLines += len(wrapped)
	}

	return totalDisplayLines
}
