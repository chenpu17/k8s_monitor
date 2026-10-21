package ui

import (
	"fmt"
	"strings"
)

// scrollIndicatorLines is the number of lines a view spends on the scroll
// indicator it appends below its content: one blank line plus the indicator
// itself. Views that print the indicator on a line they already have (the list
// footers do) do not reserve anything.
const scrollIndicatorLines = 2

// lineCount returns the number of screen lines s occupies.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// chromeLines returns the number of lines View() prints around the content area
// of the current view: the app header, the blank separators, the view tabs and
// the help footer. It is derived from the rendered strings rather than from a
// constant so that it follows the header when it grows (the title carries a
// bottom margin, the node header grows an NPU summary line).
func (m *Model) chromeLines() int {
	if m.logsMode || m.commandOutputMode {
		// Those modes render without the view tabs.
		return lineCount(m.renderHeader()) + 2 + lineCount(m.renderFooter())
	}

	chrome := lineCount(m.renderHeader()) + 2 + lineCount(m.renderFooter())
	if tabs := m.renderViewTabs(); tabs != "" {
		chrome += lineCount(tabs)
	}
	return chrome
}

// contentBudget returns how many lines the content area of the current view may
// occupy for the whole frame to fit the terminal. A view that renders more makes
// the terminal scroll, which pushes the app header (and the view title) off the
// top of the screen.
func (m *Model) contentBudget() int {
	if m.height <= 0 {
		return 0
	}
	return max(1, m.height-m.chromeLines())
}

// panelLines returns the lines the active filter or search panel adds below the
// list content of the current view: one blank separator plus the panel itself.
// List budgets must hold these lines back, or the renderer appends the panel
// below the budget and fitFrame silently trims it away.
func (m *Model) panelLines() int {
	switch {
	case m.filterMode:
		return 1 + lineCount(m.renderFilterPanel())
	case m.searchMode:
		return 1 + lineCount(m.renderSearchPanel())
	}
	return 0
}

// visibleLines returns how many of total content lines fit into a budget-line
// content area. reserve lines are held back for the scroll indicator when the
// content does not fit; content that fits is shown whole.
func visibleLines(total, budget, reserve int) int {
	if total <= 0 {
		return 0
	}
	visible := budget
	if total > budget {
		visible = budget - reserve
	}
	return max(1, min(visible, total))
}

// maxScrollOffset returns the largest offset the content can be scrolled to. It
// is the offset that shows the last page, so scrolling there never leaves the
// tail of the content unreachable.
func maxScrollOffset(total, budget, reserve int) int {
	return max(0, total-visibleLines(total, budget, reserve))
}

// scrollWindow returns the [start, end) slice of total content lines the given
// offset shows in a budget-line content area, with the offset clamped into
// range. A dangling offset (the content shrank under it, e.g. on a refresh)
// displays the last page, exactly like an offset at maxScrollOffset.
func scrollWindow(total, offset, budget, reserve int) (start, end int) {
	visible := visibleLines(total, budget, reserve)
	start = max(0, min(offset, max(0, total-visible)))
	return start, start + visible
}

// listRowsFor returns the number of rows the current list view shows on screen.
// The renderers that window their rows report the number they actually printed
// (m.listRows), so the selection-follow logic in Update cannot drift from the
// renderer; views that do not report it (they scroll by lines, not by items)
// keep the historical estimate.
func (m *Model) listRowsFor() int {
	if m.listRows > 0 {
		return m.listRows
	}
	return max(1, m.height-10)
}

// fitFrame joins the parts of the screen and, as a last line of defence, trims
// the content area so the frame never grows taller than the terminal. A taller
// frame scrolls the terminal, which pushes the header off the top of the screen;
// trimming the content keeps the header, the view tabs and the help footer in
// place. Views window their own content, so this only fires for a view that
// renders more lines than its contentBudget allows.
func (m *Model) fitFrame(header, content, tabs, footer string) string {
	if m.height > 0 {
		budget := m.height - (lineCount(header) + 2 + lineCount(footer))
		if tabs != "" {
			budget -= lineCount(tabs)
		}
		budget = max(1, budget)
		if lines := strings.Split(content, "\n"); len(lines) > budget {
			content = strings.Join(lines[:budget], "\n")
		}
	}

	if tabs != "" {
		return fmt.Sprintf("%s\n\n%s\n\n%s\n%s", header, content, tabs, footer)
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s", header, content, footer)
}
