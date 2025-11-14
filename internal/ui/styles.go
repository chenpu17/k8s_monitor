package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Color scheme
var (
	// Primary colors
	ColorPrimary   = lipgloss.Color("#00D9FF")
	ColorSecondary = lipgloss.Color("#7C3AED")
	ColorSuccess   = lipgloss.Color("#10B981")
	ColorWarning   = lipgloss.Color("#F59E0B")
	ColorDanger    = lipgloss.Color("#EF4444")
	ColorInfo      = lipgloss.Color("#3B82F6")

	// Text colors
	ColorTextPrimary   = lipgloss.Color("#FFFFFF")
	ColorTextSecondary = lipgloss.Color("#9CA3AF")
	ColorTextMuted     = lipgloss.Color("#6B7280")

	// Background colors
	ColorBgPrimary   = lipgloss.Color("#1F2937")
	ColorBgSecondary = lipgloss.Color("#374151")
	ColorBgHover     = lipgloss.Color("#4B5563")
)

// Common styles
var (
	// Title style
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	// Subtitle style
	StyleSubtitle = lipgloss.NewStyle().
			Foreground(ColorTextSecondary).
			Italic(true)

	// Header style
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorTextPrimary).
			Background(ColorBgSecondary).
			Padding(0, 1)

	// Sub-header style
	StyleSubHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary)

	// Status styles
	StyleStatusReady = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	StyleStatusNotReady = lipgloss.NewStyle().
				Foreground(ColorDanger).
				Bold(true)

	StyleStatusPending = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true)

	StyleStatusRunning = lipgloss.NewStyle().
				Foreground(ColorInfo).
				Bold(true)

	// Key binding styles
	StyleKey = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	StyleKeyDesc = lipgloss.NewStyle().
			Foreground(ColorTextSecondary)

	// Border styles
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBgSecondary).
			Padding(1, 2)

	// Error style
	StyleError = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	// Highlight style
	StyleHighlight = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// Warning/Danger styles
	StyleWarning = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)

	StyleDanger = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	// Text secondary and muted
	StyleTextSecondary = lipgloss.NewStyle().
				Foreground(ColorTextSecondary)

	StyleTextMuted = lipgloss.NewStyle().
			Foreground(ColorTextMuted)

	// Selection style (for highlighting selected row in lists)
	StyleSelected = lipgloss.NewStyle().
			Background(ColorBgHover).
			Foreground(ColorPrimary).
			Bold(true)
)

// FormatBytes formats bytes to human readable format
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatPercentage formats a percentage value
func FormatPercentage(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

// FormatMillicores formats millicores to human readable format
func FormatMillicores(millicores int64) string {
	if millicores < 1000 {
		return fmt.Sprintf("%dm", millicores)
	}
	return fmt.Sprintf("%.2f", float64(millicores)/1000.0)
}

// RenderKeyBinding renders a key binding help text
func RenderKeyBinding(key, desc string) string {
	return fmt.Sprintf("%s %s", StyleKey.Render(key), StyleKeyDesc.Render(desc))
}

// ansiRegex matches ANSI color codes
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI color codes from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// visualLength returns the visual length of a string (excluding ANSI codes)
// Correctly handles wide characters (e.g., Chinese, Japanese, Korean)
func visualLength(s string) int {
	// Strip ANSI codes first, then calculate display width
	stripped := stripANSI(s)
	return runewidth.StringWidth(stripped)
}

// padRight pads a string to the specified width (handling ANSI codes correctly)
// If the string contains ANSI codes, we calculate the visual length and add padding accordingly
func padRight(s string, width int) string {
	vlen := visualLength(s)
	if vlen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vlen)
}

// truncate truncates a string to maxLen display width, adding "..." if truncated
// Correctly handles wide characters (e.g., Chinese, Japanese, Korean)
func truncate(s string, maxLen int) string {
	// Strip ANSI codes for accurate length calculation
	stripped := stripANSI(s)
	width := runewidth.StringWidth(stripped)

	if width <= maxLen {
		return s
	}

	if maxLen <= 3 {
		// Just truncate without ellipsis for very short limits
		return runewidth.Truncate(s, maxLen, "")
	}

	// Truncate to maxLen-3 width and add ellipsis
	return runewidth.Truncate(stripped, maxLen-3, "") + "..."
}

// RenderStatus renders a status with appropriate color
func RenderStatus(status string) string {
	switch status {
	case "Ready", "Running", "Succeeded":
		return StyleStatusReady.Render(status)
	case "NotReady", "Failed", "Error":
		return StyleStatusNotReady.Render(status)
	case "Pending", "Unknown":
		return StyleStatusPending.Render(status)
	default:
		return status
	}
}

// RenderSparkline renders a sparkline chart from data points
// data: slice of values (e.g., CPU usage percentages)
// width: target width in characters
func RenderSparkline(data []float64, width int) string {
	if len(data) == 0 {
		return strings.Repeat(" ", width)
	}

	// Sparkline characters from low to high
	chars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Find min and max
	minVal, maxVal := data[0], data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Handle case where all values are the same
	if maxVal == minVal {
		return strings.Repeat(string(chars[3]), min(len(data), width))
	}

	// Map data to sparkline characters
	var result []rune
	for i, v := range data {
		if i >= width {
			break
		}
		// Normalize to 0-1 range
		normalized := (v - minVal) / (maxVal - minVal)
		// Map to character index
		idx := int(normalized * float64(len(chars)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(chars) {
			idx = len(chars) - 1
		}
		result = append(result, chars[idx])
	}

	// Pad if needed
	for len(result) < width {
		result = append(result, ' ')
	}

	return string(result[:width])
}

// formatNetworkTraffic formats network bytes with color coding
func formatNetworkTraffic(bytes int64) string {
	if bytes == 0 {
		return StyleTextMuted.Render("0B")
	}

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	var formatted string

	switch {
	case bytes >= TB:
		formatted = fmt.Sprintf("%.2fTB", float64(bytes)/float64(TB))
		return StyleDanger.Render(formatted) // TB level is concerning
	case bytes >= GB:
		formatted = fmt.Sprintf("%.2fGB", float64(bytes)/float64(GB))
		return StyleWarning.Render(formatted) // GB level needs attention
	case bytes >= MB:
		formatted = fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
		return StyleHighlight.Render(formatted) // MB level is normal
	case bytes >= KB:
		formatted = fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
		return StyleTextSecondary.Render(formatted) // KB level is low
	default:
		formatted = fmt.Sprintf("%dB", bytes)
		return StyleTextMuted.Render(formatted) // Bytes level is minimal
	}
}

// formatNetworkRate formats network rate (MB/s) with color coding
func formatNetworkRate(mbps float64) string {
	if mbps < 0.001 {
		return StyleTextMuted.Render("0 B/s")
	}

	var formatted string

	switch {
	case mbps >= 1000: // >= 1 GB/s
		formatted = fmt.Sprintf("%.2f GB/s", mbps/1024)
		return StyleDanger.Render(formatted) // Very high bandwidth
	case mbps >= 100: // >= 100 MB/s
		formatted = fmt.Sprintf("%.1f MB/s", mbps)
		return StyleWarning.Render(formatted) // High bandwidth
	case mbps >= 1: // >= 1 MB/s
		formatted = fmt.Sprintf("%.2f MB/s", mbps)
		return StyleHighlight.Render(formatted) // Normal bandwidth
	case mbps >= 0.1: // >= 100 KB/s
		formatted = fmt.Sprintf("%.0f KB/s", mbps*1024)
		return StyleTextSecondary.Render(formatted) // Low bandwidth
	default: // < 100 KB/s
		formatted = fmt.Sprintf("%.1f KB/s", mbps*1024)
		return StyleTextMuted.Render(formatted) // Minimal bandwidth
	}
}

// renderSeparator returns a horizontal separator line, safely handling window width
// If width is too small, returns a minimum-width separator
func renderSeparator(width int) string {
	const minWidth = 10
	lineWidth := width - 2

	// Ensure positive width
	if lineWidth < minWidth {
		lineWidth = minWidth
	}

	return strings.Repeat("─", lineWidth)
}

// wrapLine wraps a long line into multiple lines with proper indentation
// Returns a slice of wrapped lines (continuation lines are NOT indented, caller should add indent)
func wrapLine(line string, maxWidth int, indentWidth int) []string {
	// Strip ANSI codes for accurate length calculation
	stripped := stripANSI(line)

	// If line fits within maxWidth, return as-is
	if runewidth.StringWidth(stripped) <= maxWidth {
		return []string{line}
	}

	var result []string
	currentLine := ""

	// First line uses full width, continuation lines reserve space for indent
	lineWidth := maxWidth

	// Process runes to build wrapped lines
	for _, r := range stripped {
		charWidth := runewidth.RuneWidth(r)

		if runewidth.StringWidth(currentLine)+charWidth > lineWidth {
			// Current line is full, save it and start new line
			if currentLine != "" {
				result = append(result, currentLine)
			}
			currentLine = string(r)
			// After first line, reduce width for indent
			lineWidth = maxWidth - indentWidth
		} else {
			currentLine += string(r)
		}
	}

	// Add remaining content
	if currentLine != "" {
		result = append(result, currentLine)
	}

	return result
}
