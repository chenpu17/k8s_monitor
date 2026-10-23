package ui

// The logs viewer spends content lines on the title, a blank separator, the
// search bar when search mode is active, and a two-line status tail (the blank
// separator above the position / auto-follow line and the line itself). Its
// window was sized as height-8 regardless, so the rendered content was always
// two lines taller than the content area and fitFrame trimmed exactly the
// status tail away.
const (
	logsHeaderLines = 2 // title + blank separator below it
	logsStatusLines = 2 // blank separator above the status line + the status line
)

// logsChrome returns the non-log lines the logs viewer renders above and below
// its log rows. On a terminal too short for both, the blank separator above the
// status line is the first to go: the status line carries the position and the
// auto-follow state, which the user cannot get anywhere else.
func (m *Model) logsChrome() (header, status int) {
	header = logsHeaderLines
	status = logsStatusLines
	if m.logsSearchMode {
		header++ // the search bar sits between the title and the separator
	}
	if m.contentBudget()-header-status < 1 {
		status = 1
	}
	return header, status
}

// logsRows returns how many log rows fit the content area of the logs viewer.
func (m *Model) logsRows() int {
	header, status := m.logsChrome()
	return max(1, m.contentBudget()-header-status)
}

// initLogsScrollPosition initializes the scroll position when first entering logs mode
func (m *Model) initLogsScrollPosition() {
	if m.containerLogs == "" {
		return
	}

	// Calculate bottom position to show latest logs - use wrapped line count
	totalLines := m.getLogsDisplayLineCount()
	maxVisible := m.logsRows()
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
