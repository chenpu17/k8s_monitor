package ui

import (
	"fmt"
	"strings"
)

// renderCommandOutput renders the command output viewer
func (m *Model) renderCommandOutput() string {
	if m.commandOutputContent == "" {
		return StyleTextMuted.Render("Loading...")
	}

	var sections []string

	// Header
	title := StyleHeader.Render(m.commandOutputTitle)
	sections = append(sections, title)
	sections = append(sections, "")

	// Split content into lines
	lines := strings.Split(m.commandOutputContent, "\n")

	// Calculate visible lines
	maxVisible := m.height - 6 // Reserve space for header/footer
	if maxVisible < 1 {
		maxVisible = 1
	}

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
		scrollInfo := fmt.Sprintf("Lines %d-%d of %d", startIdx+1, endIdx, totalLines)
		sections = append(sections, "")
		sections = append(sections, StyleTextMuted.Render(scrollInfo))
	}

	return strings.Join(sections, "\n")
}
