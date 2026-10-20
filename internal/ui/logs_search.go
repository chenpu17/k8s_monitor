package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	StyleSearchMatch = lipgloss.NewStyle().Background(ColorWarning).Foreground(lipgloss.Color("#000000")).Bold(true)
)

// highlightSearchTermSimple highlights search term in plain text (no ANSI handling)
// This is optimized for performance during scrolling - avoids expensive regex operations
func highlightSearchTermSimple(line string, searchText string) string {
	if searchText == "" || line == "" {
		return line
	}

	// Case-insensitive search using simple string operations
	lowerLine := strings.ToLower(line)
	lowerSearch := strings.ToLower(searchText)

	// Find first match only for simplicity and performance
	idx := strings.Index(lowerLine, lowerSearch)
	if idx == -1 {
		return line
	}

	// Build result with single highlighted match
	// For performance, only highlight first occurrence
	matchEnd := idx + len(searchText)
	matched := line[idx:matchEnd]

	return line[:idx] + StyleSearchMatch.Render(matched) + line[matchEnd:]
}
