package ui

import (
	"fmt"
	"strings"
)

// The command output viewer spends content lines on three things besides the
// output rows: the title, the blank separator below it, and the position line
// with its blank separator. Reserving only "some room for header/footer" made
// the rendered content overflow the content area by four lines on every
// terminal, and fitFrame silently trimmed the tail — the position line and the
// last output rows were unreachable. The budget is now derived from the same
// content area the other views use, so the viewer cannot drift from the frame.
const (
	commandOutputHeaderLines = 2 // title + blank separator
	commandOutputStatusLines = 2 // blank separator + position line
)

// commandOutputBudget returns the lines this view may occupy. It is the same
// content area View() gives the command output mode (which renders without the
// view tabs), computed from the mode itself rather than from m.commandOutputMode
// so the renderer measures its own window even when it is exercised directly.
func (m *Model) commandOutputBudget() int {
	if m.height <= 0 {
		return 0
	}
	chrome := lineCount(m.renderHeader()) + 2 + lineCount(m.renderFooter())
	return max(1, m.height-chrome)
}

// commandOutputChrome returns the lines the viewer uses above and below the
// output rows. On a terminal too short for the full tail the separators go
// first — the blank line above the position line, then the one below the
// title — because both the title and the position line carry information the
// user cannot get anywhere else.
func (m *Model) commandOutputChrome() (header, status int) {
	header = commandOutputHeaderLines
	status = commandOutputStatusLines
	if m.commandOutputBudget()-header-status < 1 {
		status = 1
	}
	if m.commandOutputBudget()-header-status < 1 {
		header = 1
	}
	return header, status
}

// commandOutputRows returns how many output rows fit the content area.
func (m *Model) commandOutputRows() int {
	header, status := m.commandOutputChrome()
	return max(1, m.commandOutputBudget()-header-status)
}

// commandOutputMaxScroll returns the largest offset the output can be scrolled
// to: the one that shows the last row.
func (m *Model) commandOutputMaxScroll() int {
	total := len(strings.Split(m.commandOutputContent, "\n"))
	// An empty content is the loading shell, which has no output rows.
	if m.commandOutputContent == "" {
		total = 0
	}
	return max(0, total-m.commandOutputRows())
}

// renderCommandOutput renders the command output viewer
func (m *Model) renderCommandOutput() string {
	if m.commandOutputContent == "" {
		return StyleTextMuted.Render(m.T("view.loading"))
	}

	var sections []string

	// Header
	headerLines, statusLines := m.commandOutputChrome()
	title := StyleHeader.Render(m.commandOutputTitle)
	sections = append(sections, title)
	if headerLines > 1 {
		sections = append(sections, "")
	}

	// Split content into lines
	lines := strings.Split(m.commandOutputContent, "\n")

	// Calculate visible lines
	maxVisible := m.commandOutputRows()

	totalLines := len(lines)
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Clamp scroll offset
	if m.commandOutputScroll > maxScroll {
		m.commandOutputScroll = maxScroll
	}
	if m.commandOutputScroll < 0 {
		m.commandOutputScroll = 0
	}

	// Get visible lines
	startIdx := m.commandOutputScroll
	endIdx := startIdx + maxVisible
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := lines[startIdx:endIdx]

	// Render lines with line numbers
	for i, line := range visibleLines {
		lineNum := startIdx + i + 1
		lineNumStr := StyleTextMuted.Render(fmt.Sprintf("%4d│ ", lineNum))
		sections = append(sections, lineNumStr+line)
	}

	// Status bar
	if totalLines > maxVisible {
		scrollInfo := m.TF("command_output.lines_range", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		})
		if statusLines > 1 {
			sections = append(sections, "")
		}
		sections = append(sections, StyleTextMuted.Render(scrollInfo))
	}

	return strings.Join(sections, "\n")
}
