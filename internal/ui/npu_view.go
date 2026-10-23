package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chenpu/k8s-monitor/internal/model"
	"github.com/mattn/go-runewidth"
)

// The NPU overview collects the NPU information that otherwise lives scattered
// across the node list, the node detail and the alerts view into one cross-node
// panel: a summary bar (chip health, allocation, monitoring coverage), a chip
// matrix with one row per NPU node, and the NPU alerts.
//
// The matrix cells carry their grade twice: in the colour of the style and in
// the glyph itself. --no-color drops the colour, so the glyph is what keeps the
// view readable there; TestNPUViewNoColorGolden pins that.

const (
	// npuViewNameWidthMax caps the node-name column so a long node name cannot
	// push the chip matrix off the screen on a wide terminal.
	npuViewNameWidthMax = 30
	// npuViewNameWidthMin is the narrowest the name column shrinks to before the
	// matrix loses chips instead.
	npuViewNameWidthMin = 8
	// npuViewGap separates the name column from the matrix and the matrix from
	// the per-row grade summary.
	npuViewGap = 2
	// npuViewSelectionPrefix marks the selected row. Unselected rows are
	// indented by the same width, so the matrix columns stay aligned when the
	// selection moves.
	npuViewSelectionPrefix = "> "
	npuViewGutter          = "  "
	// npuViewGroupSep separates the chips of two NPUs. The chips of one NPU are
	// separated by a single space.
	npuViewGroupSep = "  "
	// npuViewTruncatedCell marks a matrix cut short by the terminal width.
	npuViewTruncatedCell = " …"
	// npuViewNoWidthLimit stands in for the terminal width when it is unknown
	// (the renderers are exercised directly by tests).
	npuViewNoWidthLimit = 1 << 20
	// npuViewRecentAlerts is how many of the newest NPU alerts the summary
	// shows below the counts.
	npuViewRecentAlerts = 5
)

// npuGradeCell renders one matrix cell. The glyph alone distinguishes the three
// grades, which is what keeps the matrix readable in --no-color mode where the
// style collapses to plain text.
func npuGradeCell(grade string) string {
	switch grade {
	case model.NPUChipGradeFault:
		return StyleStatusNotReady.Render("▁")
	case model.NPUChipGradeDegraded:
		return StyleWarning.Render("▒")
	default:
		return StyleStatusReady.Render("▉")
	}
}

// npuChipCellGroups groups a node's chips by NPU, in the node's canonical chip
// order (NPUChipHealth is already in that order). A run of chips with the same
// NPUID is one group, so the matrix can render NPU boundaries as a wider gap.
func npuChipCellGroups(grades []model.NPUChipGrade) [][]string {
	groups := make([][]string, 0, len(grades))
	lastNPU := 0
	for i, grade := range grades {
		if i == 0 || grade.NPUID != lastNPU {
			groups = append(groups, nil)
			lastNPU = grade.NPUID
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], npuGradeCell(grade.Grade))
	}
	return groups
}

// npuMatrixText renders the cell groups, showing at most limit chips (-1 shows
// every chip). A truncated matrix ends with an ellipsis marker, so a chip the
// terminal cannot show is visibly missing instead of silently absent.
func npuMatrixText(groups [][]string, limit int) string {
	var (
		parts     []string
		used      int
		truncated bool
	)
	for _, group := range groups {
		cells := make([]string, 0, len(group))
		for _, cell := range group {
			if limit >= 0 && used >= limit {
				truncated = true
				break
			}
			cells = append(cells, cell)
			used++
		}
		if len(cells) > 0 {
			parts = append(parts, strings.Join(cells, " "))
		}
		if truncated {
			break
		}
	}
	out := strings.Join(parts, npuViewGroupSep)
	if truncated {
		out += npuViewTruncatedCell
	}
	return out
}

// npuRowSuffix renders the per-row grade summary, e.g. "1 degraded • 2 fault".
// A fully healthy row has no suffix: there is nothing to report about it.
func (m *Model) npuRowSuffix(node *model.NodeData) string {
	degraded, fault := 0, 0
	for _, grade := range node.NPUChipHealth {
		switch grade.Grade {
		case model.NPUChipGradeFault:
			fault++
		case model.NPUChipGradeDegraded:
			degraded++
		}
	}

	var parts []string
	if degraded > 0 {
		parts = append(parts, StyleWarning.Render(m.TF("npuview.row_degraded", map[string]interface{}{"Count": degraded})))
	}
	if fault > 0 {
		parts = append(parts, StyleStatusNotReady.Render(m.TF("npuview.row_fault", map[string]interface{}{"Count": fault})))
	}
	return strings.Join(parts, " • ")
}

// npuMatrixRow renders one node row: the name, the chip matrix and the grade
// summary, shrunk to the terminal width. The name column shrinks first, then the
// grade summary is dropped, then trailing chips are cut off the matrix - in that
// order, so the matrix itself survives as long as there is room for at least one
// chip.
func (m *Model) npuMatrixRow(node *model.NodeData, selected bool) string {
	prefix := npuViewGutter
	if selected {
		prefix = npuViewSelectionPrefix
	}

	budget := npuViewNoWidthLimit
	nameFloor := npuViewNameWidthMin
	if m.width > 0 {
		budget = max(1, m.width-visualLength(prefix))
		// On a terminal too narrow for the minimum name column plus one cell,
		// the name column gives way further: a chip glyph is the least this row
		// may show.
		nameFloor = min(npuViewNameWidthMin, max(1, budget-npuViewGap-1))
	}

	nameW := max(min(visualLength(node.Name), npuViewNameWidthMax), nameFloor)

	if len(node.NPUChipHealth) == 0 {
		// The node is an NPU node but no chip data arrived: say so instead of
		// drawing an empty matrix, which would read as "this node has no chips".
		// The name column gives way to the note, so the note stays readable
		// instead of being squeezed into an ellipsis.
		for nameW > nameFloor && nameW+npuViewGap+visualLength(m.T("npuview.monitoring_unavailable")) > budget {
			nameW--
		}
		noteW := max(1, budget-nameW-npuViewGap)
		note := StyleTextMuted.Render(truncate(m.T("npuview.monitoring_unavailable"), noteW))
		row := npuFitWidth(padRight(truncate(node.Name, nameW), nameW)+strings.Repeat(" ", npuViewGap)+note, budget)
		if selected {
			return StyleSelected.Render(prefix + row)
		}
		return prefix + row
	}

	groups := npuChipCellGroups(node.NPUChipHealth)
	suffix := m.npuRowSuffix(node)

	// The row width for a given name column, chip count and suffix, measured on
	// exactly the strings that are going to be printed.
	fits := func(shown int, withSuffix bool) bool {
		w := nameW + npuViewGap + visualLength(npuMatrixText(groups, shown))
		if withSuffix {
			w += npuViewGap + visualLength(suffix)
		}
		return w <= budget
	}

	shown := len(node.NPUChipHealth)
	withSuffix := suffix != ""
	if !fits(shown, withSuffix) {
		// The matrix is the content of this view, so the summary is the first
		// thing to go when the terminal is narrow.
		withSuffix = false
	}
	if !fits(shown, withSuffix) && nameW > nameFloor {
		nameW = nameFloor
	}
	for shown > 1 && !fits(shown, withSuffix) {
		shown--
	}

	row := padRight(truncate(node.Name, nameW), nameW) + strings.Repeat(" ", npuViewGap) + npuMatrixText(groups, shown)
	if withSuffix {
		row += strings.Repeat(" ", npuViewGap) + suffix
	}
	// The floor of one chip cell still overflows on a terminal narrower than the
	// prefix, the name column and that cell; the clamp is what keeps such a row
	// from wrapping.
	row = npuFitWidth(row, budget)
	if selected {
		return StyleSelected.Render(prefix + row)
	}
	return prefix + row
}

// npuNodes returns the cluster's NPU nodes, sorted by name so the matrix order
// does not follow the API Server's listing order from refresh to refresh.
func (m *Model) npuNodes() []*model.NodeData {
	if m.clusterData == nil {
		return nil
	}

	var nodes []*model.NodeData
	for _, node := range m.clusterData.Nodes {
		if node.NPUCapacity > 0 {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	return nodes
}

// hasNPUNodes reports whether the cluster has any NPU node. It follows the
// overview NPU panel's presence rule (the cluster summary's NPU capacity) and
// falls back to the node-level capacity for data that carries it only there.
func (m *Model) hasNPUNodes() bool {
	if m.clusterData == nil {
		return false
	}
	if m.clusterData.Summary != nil && m.clusterData.Summary.NPUCapacity > 0 {
		return true
	}
	for _, node := range m.clusterData.Nodes {
		if node.NPUCapacity > 0 {
			return true
		}
	}
	return false
}

// npuChipTally counts the cluster's chips by grade.
func npuChipTally(nodes []*model.NodeData) (healthy, degraded, fault int) {
	for _, node := range nodes {
		for _, grade := range node.NPUChipHealth {
			switch grade.Grade {
			case model.NPUChipGradeFault:
				fault++
			case model.NPUChipGradeDegraded:
				degraded++
			default:
				healthy++
			}
		}
	}
	return healthy, degraded, fault
}

// npuNodesWithData counts the NPU nodes whose chip data is present. A node
// without chip data is an NPU node whose metrics are missing (no exporter
// answer), which the summary bar reports as uncovered monitoring.
func npuNodesWithData(nodes []*model.NodeData) int {
	count := 0
	for _, node := range nodes {
		if len(node.NPUChipHealth) > 0 {
			count++
		}
	}
	return count
}

// npuAlertTypeKeys is the explicit list of NPU alert types the view counts and
// lists, in display order, with their short labels. It is an explicit list
// rather than a "npu_" prefix match so a future alert type cannot silently join
// the panel: a new type shows up here only after being given a label.
var npuAlertTypeKeys = []struct {
	alertType model.AlertType
	key       string
}{
	{model.AlertTypeNPUChipFault, "npuview.type_chip_fault"},
	{model.AlertTypeNPUFaultLevel, "npuview.type_fault_level"},
	{model.AlertTypeNPUSwitchFault, "npuview.type_switch_fault"},
	{model.AlertTypeNPUCheckFailed, "npuview.type_check_failed"},
	{model.AlertTypeNPUTempHigh, "npuview.type_temp_high"},
	{model.AlertTypeNPUEccDouble, "npuview.type_ecc_double"},
	{model.AlertTypeNPUEccSingle, "npuview.type_ecc_single"},
	{model.AlertTypeNPULinkDown, "npuview.type_link_down"},
	{model.AlertTypeNPUErrorCode, "npuview.type_error_code"},
	{model.AlertTypeNPUMonitorDegraded, "npuview.type_monitor_degraded"},
}

// npuAlertTypeSet is the lookup set behind npuAlerts, built from the same list
// so the filter and the counts can never disagree.
var npuAlertTypeSet = func() map[model.AlertType]bool {
	set := make(map[model.AlertType]bool, len(npuAlertTypeKeys))
	for _, entry := range npuAlertTypeKeys {
		set[entry.alertType] = true
	}
	return set
}()

// npuAlerts returns the cluster's NPU alerts in the order the summary stores
// them.
func (m *Model) npuAlerts() []model.Alert {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return nil
	}
	var alerts []model.Alert
	for _, alert := range m.clusterData.Summary.Alerts {
		if npuAlertTypeSet[alert.AlertType] {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// sortedNPUAlerts returns the alerts in display order: the critical ones first,
// then by recency. The sort is stable and only orders on those two keys, so
// alerts the data layer produced at the same severity and instant keep their
// original order.
func sortedNPUAlerts(alerts []model.Alert) []model.Alert {
	sorted := append([]model.Alert(nil), alerts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Severity != sorted[j].Severity {
			return sorted[i].Severity > sorted[j].Severity
		}
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})
	return sorted
}

// recentNPUAlerts returns the alerts in display order, limited to the newest
// limit of them per severity group.
func recentNPUAlerts(alerts []model.Alert, limit int) []model.Alert {
	sorted := sortedNPUAlerts(alerts)
	if limit >= 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

// npuSummarySegments builds the summary bar segments, already localized and
// styled.
func (m *Model) npuSummarySegments(nodes []*model.NodeData) []string {
	var segments []string

	// Chip health across every node that reports chips.
	healthy, degraded, fault := npuChipTally(nodes)
	total := healthy + degraded + fault
	if total == 0 {
		segments = append(segments, StyleTextMuted.Render(m.T("npuview.health_no_data")))
	} else {
		segments = append(segments, m.TF("npuview.health_summary", map[string]interface{}{
			"Healthy":  StyleStatusReady.Render(fmt.Sprintf("%d", healthy)),
			"Total":    total,
			"Degraded": StyleWarning.Render(fmt.Sprintf("%d", degraded)),
			"Fault":    StyleStatusNotReady.Render(fmt.Sprintf("%d", fault)),
		}))
	}

	// Allocation from the cluster summary. A cluster whose allocatable NPU count
	// is zero has no percentage to show: dividing by it would print NaN.
	if m.clusterData.Summary != nil {
		summary := m.clusterData.Summary
		if summary.NPUAllocatable > 0 {
			percent := float64(summary.NPUAllocated) / float64(summary.NPUAllocatable) * 100
			segments = append(segments, m.TF("npuview.alloc_summary", map[string]interface{}{
				"Allocated":   StyleHighlight.Render(fmt.Sprintf("%d", summary.NPUAllocated)),
				"Allocatable": summary.NPUAllocatable,
				"Percent":     fmt.Sprintf("%.1f", percent),
			}))
		} else {
			segments = append(segments, m.TF("npuview.alloc_summary_no_pct", map[string]interface{}{
				"Allocated":   StyleHighlight.Render(fmt.Sprintf("%d", summary.NPUAllocated)),
				"Allocatable": summary.NPUAllocatable,
			}))
		}
	}

	// Monitoring coverage: how many NPU nodes actually answered with chip data.
	segments = append(segments, m.TF("npuview.coverage", map[string]interface{}{
		"With":  npuNodesWithData(nodes),
		"Total": len(nodes),
	}))

	return segments
}

// wrapSummarySegments joins the summary segments with sep, wrapping onto a new
// line when the terminal is too narrow for the whole bar in one line. It
// guarantees every returned line fits width (when width is known), because the
// view's lines are asserted to fit pixel for pixel.
func wrapSummarySegments(segments []string, width int, sep string) []string {
	if len(segments) == 0 {
		return nil
	}
	if width <= 0 {
		return []string{strings.Join(segments, sep)}
	}

	var lines []string
	current := ""
	for _, segment := range segments {
		switch {
		case current == "":
			current = segment
		case visualLength(current)+visualLength(sep)+visualLength(segment) <= width:
			current += sep + segment
		default:
			lines = append(lines, truncate(current, width))
			current = segment
		}
	}
	if current != "" {
		lines = append(lines, truncate(current, width))
	}
	return lines
}

// npuAlertCountsLine renders the per-type alert counts, e.g.
// "temp high: 2 • link down: 1". Types without alerts are left out so the line
// stays short; the order is the fixed order of npuAlertTypeKeys.
func (m *Model) npuAlertCountsLine(alerts []model.Alert) string {
	counts := make(map[model.AlertType]int, len(npuAlertTypeKeys))
	for _, alert := range alerts {
		counts[alert.AlertType]++
	}

	var parts []string
	for _, entry := range npuAlertTypeKeys {
		if count := counts[entry.alertType]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", m.T(entry.key), count))
		}
	}
	return strings.Join(parts, " • ")
}

// npuAlertLine renders one NPU alert as a single line: severity, the node it was
// raised on and the localized message. The message falls back to the stored
// English text for alerts that quote cluster-provided text (no MessageKey).
func (m *Model) npuAlertLine(alert model.Alert) string {
	message := alert.Message
	if alert.MessageKey != "" {
		message = m.TF(alert.MessageKey, alert.MessageArgs)
	}

	line := fmt.Sprintf("%s  %s  %s", m.alertSectionHeader(alert.Severity), alert.ResourceName, message)
	if m.width > 0 {
		line = truncate(line, m.width)
	}
	return line
}

// npuAlertLines renders the alert section: the counts, then the newest alerts.
func (m *Model) npuAlertLines(alerts []model.Alert) []string {
	lines := []string{m.npuClipLine(StyleSubHeader.Render(m.T("npuview.alerts_header")))}

	if len(alerts) == 0 {
		return append(lines, m.npuClipLine(StyleTextMuted.Render(m.T("msg.no_alerts_detected"))))
	}

	lines = append(lines, m.npuClipLine(StyleTextMuted.Render(m.npuAlertCountsLine(alerts))))
	for _, alert := range recentNPUAlerts(alerts, npuViewRecentAlerts) {
		lines = append(lines, m.npuAlertLine(alert))
	}
	return lines
}

// npuFitWidth is the hard width clamp: a string wider than width is cut to
// exactly width cells. Unlike truncate it adds no ellipsis, so it can be used as
// the last step of a row that has already decided what to keep; the cell styles
// of a cut row are dropped, which only happens on terminals narrower than a
// single styled row.
func npuFitWidth(s string, width int) string {
	if width <= 0 || visualLength(s) <= width {
		return s
	}
	return runewidth.Truncate(stripANSI(s), width, "")
}

// npuClipLine keeps a fixed line inside the terminal width. The summary bar and
// the matrix rows size themselves; the title and the legend are fixed strings, so
// on a terminal narrower than they are they are clipped instead of wrapping.
func (m *Model) npuClipLine(line string) string {
	if m.width <= 0 {
		return line
	}
	return truncate(line, m.width)
}

// npuContentLines returns every line of the NPU view, in order, before it is
// sliced down to what fits on screen. Scrolling and the "showing X-Y of Z"
// indicator both work off this list, so they cannot disagree with the content.
// It also reports the document line of every node row, so the selection-follow
// logic in Update can bring the selected row into view.
func (m *Model) npuContentLines() []string {
	if m.clusterData == nil {
		return nil
	}

	nodes := m.npuNodes()

	var lines []string
	lines = append(lines, m.npuClipLine(StyleHeader.Render(m.T("npuview.title"))))
	lines = append(lines, "")

	lines = append(lines, wrapSummarySegments(m.npuSummarySegments(nodes), m.width, "  •  ")...)
	lines = append(lines, m.npuClipLine(StyleTextMuted.Render(m.T("npuview.legend"))))
	lines = append(lines, "")

	m.npuNodeLines = make([]int, len(nodes))
	for i, node := range nodes {
		m.npuNodeLines[i] = len(lines)
		lines = append(lines, m.npuMatrixRow(node, i == m.selectedIndex))
	}

	lines = append(lines, "")
	lines = append(lines, m.npuAlertLines(m.npuAlerts())...)

	return lines
}

// npuMaxScroll returns the largest scroll offset the NPU content can be
// scrolled to.
func (m *Model) npuMaxScroll() int {
	return maxScrollOffset(len(m.npuContentLines()), m.contentBudget(), scrollIndicatorLines)
}

// npuScrollBy moves the NPU view by delta lines without stepping past the end of
// the content.
func (m *Model) npuScrollBy(delta int) {
	limit := m.npuMaxScroll()
	offset := min(m.scrollOffset, limit) + delta
	m.scrollOffset = max(0, min(offset, limit))
}

// npuScrollToSelection brings the selected node row into the visible window. It
// shares the visible-line arithmetic with scrollWindow, so the row it brings in
// is the row the renderer shows. Without reported geometry (nothing rendered
// yet) it is a no-op.
func (m *Model) npuScrollToSelection() {
	total := len(m.npuContentLines())
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.npuNodeLines) {
		return
	}
	line := m.npuNodeLines[m.selectedIndex]
	visible := visibleLines(total, m.contentBudget(), scrollIndicatorLines)
	if line < m.scrollOffset {
		m.scrollOffset = line
	} else if line >= m.scrollOffset+visible {
		m.scrollOffset = line - visible + 1
	}
	m.scrollOffset = max(0, min(m.scrollOffset, maxScrollOffset(total, m.contentBudget(), scrollIndicatorLines)))
}

// renderNPUView renders the NPU overview.
func (m *Model) renderNPUView() string {
	if m.clusterData == nil {
		return m.T("msg.no_data")
	}
	if !m.hasNPUNodes() {
		return StyleTextMuted.Render(m.T("npuview.no_nodes"))
	}

	allLines := m.npuContentLines()
	budget := m.contentBudget()
	startIdx, endIdx := scrollWindow(len(allLines), m.scrollOffset, budget, scrollIndicatorLines)
	visibleLines := allLines[startIdx:endIdx]

	// Report how many node rows the window actually shows, so Update's paging
	// moves the selection by what is on screen rather than by a guess derived
	// from the terminal height (the view carries a header, a summary bar and an
	// alert section around the rows).
	visibleRows := 0
	for _, line := range m.npuNodeLines {
		if line >= startIdx && line < endIdx {
			visibleRows++
		}
	}
	m.listRows = max(1, visibleRows)

	// Add scroll indicator if needed. Its lines are clipped like the rest, so it
	// cannot wrap on a narrow terminal.
	if len(allLines) > budget {
		indicator := StyleTextMuted.Render(fmt.Sprintf("\n[%s] (↑/↓ %s, PgUp/PgDn %s)",
			m.TF("scroll.showing", map[string]interface{}{
				"Start": startIdx + 1,
				"End":   endIdx,
				"Total": len(allLines),
			}),
			m.T("keys.scroll"),
			m.T("keys.page"),
		))
		for _, line := range strings.Split(indicator, "\n") {
			visibleLines = append(visibleLines, m.npuClipLine(line))
		}
	}

	return strings.Join(visibleLines, "\n")
}
