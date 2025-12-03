package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Log level colors
var (
	StyleLogError   = lipgloss.NewStyle().Foreground(ColorDanger).Bold(true)
	StyleLogWarn    = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	StyleLogInfo    = lipgloss.NewStyle().Foreground(ColorInfo)
	StyleLogDebug   = lipgloss.NewStyle().Foreground(ColorTextMuted)
	StyleLogSuccess = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleSearchMatch = lipgloss.NewStyle().Background(ColorWarning).Foreground(lipgloss.Color("#000000")).Bold(true)
)

// Log level patterns (case-insensitive)
var logLevelPatterns = []struct {
	pattern *regexp.Regexp
	style   lipgloss.Style
}{
	{regexp.MustCompile(`(?i)\b(ERROR|ERR|FATAL|CRIT|CRITICAL)\b`), StyleLogError},
	{regexp.MustCompile(`(?i)\b(WARN|WARNING)\b`), StyleLogWarn},
	{regexp.MustCompile(`(?i)\b(INFO)\b`), StyleLogInfo},
	{regexp.MustCompile(`(?i)\b(DEBUG|TRACE)\b`), StyleLogDebug},
	{regexp.MustCompile(`(?i)\b(SUCCESS|OK)\b`), StyleLogSuccess},
}

// highlightLogLine applies syntax highlighting to a log line
func highlightLogLine(line string, searchText string) string {
	if line == "" {
		return line
	}

	// First, apply log level highlighting
	highlightedLine := line

	for _, lp := range logLevelPatterns {
		matches := lp.pattern.FindAllStringIndex(highlightedLine, -1)
		if len(matches) == 0 {
			continue
		}

		// Apply highlighting from right to left to preserve indices
		for i := len(matches) - 1; i >= 0; i-- {
			match := matches[i]
			start, end := match[0], match[1]
			word := highlightedLine[start:end]
			styled := lp.style.Render(word)
			highlightedLine = highlightedLine[:start] + styled + highlightedLine[end:]
		}
	}

	// Then, apply search term highlighting if search is active
	if searchText != "" {
		highlightedLine = highlightSearchTerm(highlightedLine, searchText)
	}

	return highlightedLine
}

// highlightSearchTerm highlights all occurrences of the search term
func highlightSearchTerm(line string, searchText string) string {
	if searchText == "" {
		return line
	}

	// Case-insensitive search
	lowerLine := strings.ToLower(stripANSI(line))
	lowerSearch := strings.ToLower(searchText)

	// Find all matches
	var matches []int
	startIdx := 0
	for {
		idx := strings.Index(lowerLine[startIdx:], lowerSearch)
		if idx == -1 {
			break
		}
		matches = append(matches, startIdx+idx)
		startIdx += idx + len(lowerSearch)
	}

	if len(matches) == 0 {
		return line
	}

	// Build result with highlighted matches
	// We need to work with the original line (with ANSI codes) carefully
	plainLine := stripANSI(line)
	result := ""
	lastEnd := 0

	for _, matchStart := range matches {
		// Add text before match
		if matchStart > lastEnd {
			result += plainLine[lastEnd:matchStart]
		}
		// Add highlighted match
		matchEnd := matchStart + len(searchText)
		matched := plainLine[matchStart:matchEnd]
		result += StyleSearchMatch.Render(matched)
		lastEnd = matchEnd
	}

	// Add remaining text
	if lastEnd < len(plainLine) {
		result += plainLine[lastEnd:]
	}

	return result
}

// filterLogLines filters log lines based on search text
func filterLogLines(lines []string, searchText string) []string {
	if searchText == "" {
		return lines
	}

	lowerSearch := strings.ToLower(searchText)
	var filtered []string

	for _, line := range lines {
		if strings.Contains(strings.ToLower(stripANSI(line)), lowerSearch) {
			filtered = append(filtered, line)
		}
	}

	return filtered
}

// countMatches counts how many lines match the search text
func countMatches(lines []string, searchText string) int {
	if searchText == "" {
		return len(lines)
	}

	lowerSearch := strings.ToLower(searchText)
	count := 0

	for _, line := range lines {
		if strings.Contains(strings.ToLower(stripANSI(line)), lowerSearch) {
			count++
		}
	}

	return count
}

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
