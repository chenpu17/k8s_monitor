package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	// npuAggregateSparkWidth is the fixed sparkline width of the aggregate bar's
	// gauges. It matches the overview's CPU/Memory sparkline so the two views
	// read the same way; the bar drops the sparkline entirely rather than
	// overrun a terminal that cannot hold it.
	npuAggregateSparkWidth = 10
)

// The NPU matrix heat modes. Mode 0 is the health matrix every earlier batch
// rendered: the glyph is the grade and nothing else. Modes 1-3 shade the
// healthy chips by a metric, so a wall of ▉ chips becomes a readability map.
// The grades keep their own glyphs in every mode — heat is an addition to the
// health semantics, never a replacement.
const (
	npuMatrixModeHealth = iota // grade glyphs (default)
	npuMatrixModeAICore        // AI Core utilisation, %
	npuMatrixModeTemp          // temperature, °C
	npuMatrixModePower         // power draw, W
	npuMatrixModeCount
)

// npuHeatRamp is the heat glyph ramp, low to high. It is deliberately a
// different set from the health glyphs (▉▒▁): it starts at ▂, so no heat cell
// can collide with the fault glyph ▁ — in --no-color a cold healthy chip and a
// faulted chip must stay distinct bytes, and the two legend lines on screen
// must not give ▁ two contradictory readings. A flat series lands on ▅, the
// ramp's middle.
const npuHeatRamp = "▂▃▄▅▆▇█"

// npuHeatColors is the 256-colour ramp behind the heat glyphs, cold to hot,
// one entry per ramp glyph. It is only a second channel: --no-color drops it
// entirely and the glyph height is what carries the value then.
var npuHeatColors = []string{"21", "27", "39", "46", "226", "208", "196"}

// npuHeatValue extracts the metric the heat mode shades by.
func npuHeatValue(chip model.NPUChipData, mode int) float64 {
	switch mode {
	case npuMatrixModeAICore:
		return float64(chip.AICore)
	case npuMatrixModeTemp:
		return float64(chip.Temp)
	case npuMatrixModePower:
		return chip.Power
	default:
		return 0
	}
}

// npuChipValues returns the heat metric of every grade in the node's canonical
// chip order. The grades are built from the same slice as the chips, so the
// index alignment holds for produced data; a fixture that carries grades only
// yields zeros, which renders as the flat line it is.
func npuChipValues(node *model.NodeData, mode int) []float64 {
	values := make([]float64, len(node.NPUChipHealth))
	if mode == npuMatrixModeHealth {
		return values
	}
	if len(node.NPUChips) == len(node.NPUChipHealth) {
		for i, chip := range node.NPUChips {
			values[i] = npuHeatValue(chip, mode)
		}
		return values
	}
	// Fall back to the stable chip identity for data whose slice lengths differ.
	byChip := make(map[[2]int]model.NPUChipData, len(node.NPUChips))
	for _, chip := range node.NPUChips {
		byChip[[2]int{chip.NPUID, chip.Chip}] = chip
	}
	for i, grade := range node.NPUChipHealth {
		if chip, ok := byChip[[2]int{grade.NPUID, grade.Chip}]; ok {
			values[i] = npuHeatValue(chip, mode)
		}
	}
	return values
}

// npuHeatLevels normalizes values to the heat ramp, node-local: the ramp spans
// this node's own min and max, so a rack full of idle chips still shows which
// ones are busiest. Values from every chip of the node are considered, because
// the range is a property of the node, not of the healthy subset — a faulted
// chip's temperature is still evidence about the node's thermal band.
//
// A node whose values are all equal maps to the middle level, which renders as
// the flat ▄ row.
func npuHeatLevels(values []float64) []int {
	levels := make([]int, len(values))
	if len(values) == 0 {
		return levels
	}
	top := len([]rune(npuHeatRamp)) - 1
	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == minVal {
		for i := range levels {
			levels[i] = top / 2
		}
		return levels
	}
	for i, v := range values {
		idx := int((v - minVal) / (maxVal - minVal) * float64(top))
		if idx < 0 {
			idx = 0
		}
		if idx > top {
			idx = top
		}
		levels[i] = idx
	}
	return levels
}

// npuHeatCell renders one heat cell: the ramp glyph at its level, in the
// matching colour. The colour is decoration only; the glyph carries the value
// in --no-color mode.
func npuHeatCell(level int) string {
	ramp := []rune(npuHeatRamp)
	if level < 0 {
		level = 0
	}
	if level >= len(ramp) {
		level = len(ramp) - 1
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(npuHeatColors[level]))
	return style.Render(string(ramp[level]))
}

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

// npuHeatModeCell renders one cell in a heat mode: a healthy chip is shaded by
// its metric, while a degraded or faulted chip keeps the health glyph and style
// it has in the health matrix. The health semantics are not heat: they have to
// survive whatever the metric says about a broken chip.
func npuHeatModeCell(grade model.NPUChipGrade, level int) string {
	switch grade.Grade {
	case model.NPUChipGradeFault, model.NPUChipGradeDegraded:
		return npuGradeCell(grade.Grade)
	default:
		return npuHeatCell(level)
	}
}

// npuChipCellGroups groups a node's chips by NPU, in the node's canonical chip
// order (NPUChipHealth is already in that order). A run of chips with the same
// NPUID is one group, so the matrix can render NPU boundaries as a wider gap.
func npuChipCellGroups(grades []model.NPUChipGrade) [][]string {
	return npuChipCellRow(grades, nil)
}

// npuChipCellRow is npuChipCellGroups with an optional heat metric: levels is
// nil for the health matrix (the default, unchanged output) and carries one
// normalized heat level per grade otherwise.
func npuChipCellRow(grades []model.NPUChipGrade, levels []int) [][]string {
	groups := make([][]string, 0, len(grades))
	lastNPU := 0
	for i, grade := range grades {
		if i == 0 || grade.NPUID != lastNPU {
			groups = append(groups, nil)
			lastNPU = grade.NPUID
		}
		cell := npuGradeCell(grade.Grade)
		if levels != nil {
			level := 0
			if i < len(levels) {
				level = levels[i]
			}
			cell = npuHeatModeCell(grade, level)
		}
		groups[len(groups)-1] = append(groups[len(groups)-1], cell)
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

// npuRowSuffix renders the per-row summary, e.g. "1 degraded • 2 fault" and,
// when the exporter attributes some of the node's chips to workloads,
// "… • 3 occupied". A row with nothing to report has no suffix at all.
func (m *Model) npuRowSuffix(node *model.NodeData) string {
	parts := m.npuRowSuffixGradedParts(node)
	// Occupied chips are informational, not a health claim, so the segment is
	// left unstyled next to the graded/tinted counters.
	if occupied := npuOwnedChipCount(node); occupied > 0 {
		parts = append(parts, m.TF("npuview.row_occupied", map[string]interface{}{"Count": occupied}))
	}
	return strings.Join(parts, " • ")
}

// npuRowSuffixGradedParts returns the grade-counter segments of the row suffix —
// the summary that predates chip attribution. npuMatrixRow uses them as the
// fallback the occupancy count yields to when the row gets narrow.
func (m *Model) npuRowSuffixGradedParts(node *model.NodeData) []string {
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
	return parts
}

// npuOwnedChipCount counts the node's chips that NPU-Exporter attributes to a
// workload. The ownership labels are the only attribution source, so a node
// whose metrics come from the annotation collector path counts zero.
func npuOwnedChipCount(node *model.NodeData) int {
	count := 0
	for _, chip := range node.NPUChips {
		if chip.OwnerPod != "" {
			count++
		}
	}
	return count
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
	if m.npuMatrixMode != npuMatrixModeHealth {
		// Heat mode: the cells are shaded by the node-local metric range, so the
		// row shows this node's own hot and cold chips. The health glyphs stay
		// for the chips that are not healthy, which is why this is an override of
		// the healthy cells and not a different matrix.
		groups = npuChipCellRow(node.NPUChipHealth, npuHeatLevels(npuChipValues(node, m.npuMatrixMode)))
	}
	suffix := m.npuRowSuffix(node)
	// The suffix without its informational occupancy count — what the row
	// rendered before attribution existed.
	gradedSuffix := strings.Join(m.npuRowSuffixGradedParts(node), " • ")

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
		// thing to go when the terminal is narrow — but the occupancy count
		// yields before the grade counters: it is informational, it sits last,
		// and it must not evict the severity summary that predates it. A row
		// whose suffix is only grade counters keeps the all-or-nothing drop.
		// The swap happens before the refit: fits measures the suffix about to
		// be printed, not the one that just failed. An emptied suffix must also
		// clear the flag — a surviving flag would append the two-cell gap
		// before nothing and leave the row with trailing spaces.
		if suffix != gradedSuffix {
			suffix = gradedSuffix
		}
		withSuffix = suffix != "" && fits(shown, true)
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

// npuUnknownHealthTally counts the chips whose exporter health status is
// unknown, across the nodes that carry chip data. Unknown chips stay healthy in
// the matrix (an absent signal is not a fault), so this count is the only place
// the matrix diff reports them: the 1 fault + no alerts contradiction is fixed
// by alerting on reported faults, not by inventing a fourth grade.
func npuUnknownHealthTally(nodes []*model.NodeData) int {
	unknown := 0
	for _, node := range nodes {
		unknown += countNPUChipsByHealth(node.NPUChips, "Unknown")
	}
	return unknown
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

// npuChipDataCount counts every chip that carries data, across the given nodes.
// The aggregate gauge bar is keyed on this: it is the "is there anything to
// average at all" question.
func npuChipDataCount(nodes []*model.NodeData) int {
	count := 0
	for _, node := range nodes {
		count += len(node.NPUChips)
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
	unknown := npuUnknownHealthTally(nodes)
	if total == 0 {
		segments = append(segments, StyleTextMuted.Render(m.T("npuview.health_no_data")))
	} else {
		// The counts are the matrix's tally either way. The wording is not:
		// with unknown chips in the cluster the healthy cells are "no reported
		// fault", not a health signal anybody sent, so the segment must not
		// print "8/8 healthy" next to its own "unknown 8".
		key := "npuview.health_summary"
		if unknown > 0 {
			key = "npuview.summary_no_known"
		}
		segments = append(segments, m.TF(key, map[string]interface{}{
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

	// Chips the exporter said nothing about. The segment is conditional: a
	// cluster whose chips all report a health status keeps the summary bar
	// byte-identical to before this segment existed.
	if unknown > 0 {
		segments = append(segments, m.TF("npuview.summary_unknown", map[string]interface{}{
			"Count": StyleTextMuted.Render(fmt.Sprintf("%d", unknown)),
		}))
	}

	// NPU-Exporter liveness: is the fetch path behind the chip data above
	// answering right now. The coverage count cannot answer that — it keeps its
	// last successful value while the exporter is down. The wording is kept
	// short on purpose: the bar has to survive a 38-column terminal where every
	// segment must still render in full.
	if liveness := m.npuExporterLiveness(); liveness != nil {
		segments = append(segments, m.npuLivenessSegment(liveness))
	}

	return segments
}

// npuExporterLiveness returns the summary's exporter liveness state, or nil when
// the data carries none (no exporter client, or no NPU node so the exporter is
// never polled).
func (m *Model) npuExporterLiveness() *model.NPUExporterLiveness {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return nil
	}
	return m.clusterData.Summary.NPUExporterLiveness
}

// npuLivenessSegment renders the liveness segment of the summary bar:
// "Monitoring OK · last success 10:11:12", "Monitoring degraded · ..." or
// "Monitoring unavailable". The raw error text is deliberately left out — it is
// unbounded, the node detail carries it, and the bar has to survive a narrow
// terminal.
func (m *Model) npuLivenessSegment(liveness *model.NPUExporterLiveness) string {
	style := StyleStatusReady
	key := "npuview.liveness_ok"
	switch liveness.Status {
	case "backoff":
		style, key = StyleWarning, "npuview.liveness_backoff"
	case "never":
		style, key = StyleTextMuted, "npuview.liveness_never"
	default:
		// A status nobody set (empty struct from a cached payload, or a value
		// from a future producer) must not read as healthy: render the muted
		// unavailable wording, same as "never".
		if liveness.Status != "ok" {
			style, key = StyleTextMuted, "npuview.liveness_never"
		}
	}

	if key == "npuview.liveness_never" {
		return style.Render(m.T(key))
	}
	lastSuccess := "-"
	if !liveness.LastSuccess.IsZero() {
		lastSuccess = liveness.LastSuccess.Local().Format("15:04:05")
	}
	return style.Render(m.TF(key, map[string]interface{}{"LastSuccess": lastSuccess}))
}

// wrapSummarySegments joins the summary segments with sep, wrapping onto a new
// line when the terminal is too narrow for the whole bar in one line. It
// guarantees every returned line fits width (when width is known), because the
// view's lines are asserted to fit pixel for pixel.
//
// The health segment carries its own " · " separators between the
// healthy/degraded/fault counters, so its width grows with the cluster's chip
// total. When a segment alone outgrows the terminal, cutting its tail would
// silently drop the highest-severity counts, so a segment with that internal
// separator is split and the counters wrap onto their own lines instead; the
// bar separator ("  •  ") never appears inside a segment, which is what makes
// the two distinguishable. Segments without it keep the truncate behavior.
func wrapSummarySegments(segments []string, width int, sep string) []string {
	if len(segments) == 0 {
		return nil
	}
	if width <= 0 {
		return []string{strings.Join(segments, sep)}
	}

	var lines []string
	// Flush one accumulated line. An over-wide line is only cut when it has no
	// internal separator to wrap on. The split is guarded by the part count,
	// not by a separator search: a mismatch between the two literals would
	// otherwise split the line into a single unchanged part and recurse on it
	// forever.
	flush := func(s string) {
		parts := strings.Split(s, " · ")
		if visualLength(s) > width && len(parts) > 1 {
			lines = append(lines, wrapSummarySegments(parts, width, " · ")...)
			return
		}
		lines = append(lines, truncate(s, width))
	}

	current := ""
	for _, segment := range segments {
		switch {
		case current == "":
			current = segment
		case visualLength(current)+visualLength(sep)+visualLength(segment) <= width:
			current += sep + segment
		default:
			flush(current)
			current = segment
		}
	}
	if current != "" {
		flush(current)
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

// npuHeatModeName returns the localized name of a heat mode.
func (m *Model) npuHeatModeName(mode int) string {
	switch mode {
	case npuMatrixModeAICore:
		return m.T("npuview.mode_aicore")
	case npuMatrixModeTemp:
		return m.T("npuview.mode_temp")
	case npuMatrixModePower:
		return m.T("npuview.mode_power")
	default:
		return ""
	}
}

// npuHeatLegend renders the line that says which metric the matrix is shaded
// by, how to change it, and in which direction the ramp runs. It only exists in
// heat mode: the health matrix is self-describing through the health legend
// below it and keeps its bytes.
func (m *Model) npuHeatLegend() string {
	if m.npuMatrixMode == npuMatrixModeHealth {
		return ""
	}
	return m.npuClipLine(StyleTextMuted.Render(m.TF("npuview.heat_legend", map[string]interface{}{
		"Mode": m.npuHeatModeName(m.npuMatrixMode),
		"Ramp": npuHeatRamp,
	})))
}

// npuAggregateSpark appends a sparkline of the session series to a segment. The
// trend is an extra beside the value, so it takes only the room the value leaves
// on the terminal and disappears when there is none: a segment handed to the
// wrap path already wider than the terminal is how a narrow terminal loses the
// value itself.
func (m *Model) npuAggregateSpark(segment string, series []float64, width int) string {
	if len(series) < 2 || width < 1 {
		return segment
	}
	if m.width > 0 {
		width = min(width, m.width-visualLength(segment)-1)
	}
	if width < 1 {
		return segment
	}
	spark := sparkline(series, width)
	if spark == "" {
		return segment
	}
	return segment + " " + spark
}

// npuAggregateSegments builds the cluster-wide gauge bar: the numbers the chip
// matrix cannot show at a glance (power, average temperature, HBM occupancy,
// mean AI Core utilisation) with their recent trend beside them. It reuses the
// summary bar's segment list and wrap path, so it obeys the same width
// guarantees.
//
// Every segment is chip-derived, so the whole bar is omitted when no chip data
// arrived — a bar of empty gauges would read as a measured zero, which is the
// same lie the "monitoring unavailable" wording exists to avoid.
//
// The health segment is the one part of the bar that repeats the summary bar's
// tally, and it does so narrowly: it answers "how much of the silicon is
// healthy" with two numbers, while the severities stay on the bar above where
// they already have their own styled counters.
func (m *Model) npuAggregateSegments(nodes []*model.NodeData, sample *VizSample) []string {
	if sample == nil || npuChipDataCount(nodes) == 0 {
		return nil
	}

	var segments []string
	segments = append(segments, m.TF("npuview.aggregate_nodes", map[string]interface{}{
		"Count": len(nodes),
	}))

	// Chip health, in the "H/total healthy" short form. The degraded and fault
	// counts stay with the summary bar above: this segment answers "how much of
	// the silicon is healthy", and the severities are read there. The wording
	// follows the summary bar's rule: with unknown-health chips in the cluster
	// the healthy tally is "no reported fault", not a health signal anybody
	// sent, so the segment must not print "8/8 healthy" next to the summary's
	// own "unknown 8".
	healthy, degraded, fault := npuChipTally(nodes)
	healthKey := "npuview.aggregate_health"
	if npuUnknownHealthTally(nodes) > 0 {
		healthKey = "npuview.aggregate_health_no_known"
	}
	segments = append(segments, m.TF(healthKey, map[string]interface{}{
		"Healthy": StyleStatusReady.Render(fmt.Sprintf("%d", healthy)),
		"Total":   healthy + degraded + fault,
	}))

	segments = append(segments, m.npuAggregateSpark(
		m.TF("npuview.aggregate_power", map[string]interface{}{
			"Power": fmt.Sprintf("%d", int(sample.NPUPowerW)),
		}),
		m.vizSeries(func(s VizSample) (float64, bool) { return s.NPUPowerW, true }),
		npuAggregateSparkWidth,
	))

	segments = append(segments, m.npuAggregateSpark(
		m.TF("npuview.aggregate_temp", map[string]interface{}{
			"Temp": fmt.Sprintf("%d", int(sample.NPUTempAvg)),
		}),
		m.vizSeries(func(s VizSample) (float64, bool) { return s.NPUTempAvg, true }),
		npuAggregateSparkWidth,
	))

	// HBM is a ratio, so an unknown capacity has no segment: "0/0 (NaN%)" would
	// be worse than saying nothing.
	if sample.HBMTotalMB > 0 {
		percent := float64(sample.HBMUsedMB) / float64(sample.HBMTotalMB) * 100
		segments = append(segments, m.TF("npuview.aggregate_hbm", map[string]interface{}{
			"Used":    sample.HBMUsedMB / 1024,
			"Total":   sample.HBMTotalMB / 1024,
			"Percent": fmt.Sprintf("%.0f", percent),
		}))
	}

	// Utilisation is only drawn when a chip actually reported it: an idle
	// cluster's 0% is a real value, "no chip answered" is not.
	if sample.NPUUtilKnown {
		segments = append(segments, m.npuAggregateSpark(
			m.TF("npuview.aggregate_util", map[string]interface{}{
				"Percent": fmt.Sprintf("%.0f", sample.NPUUtilPct),
			}),
			m.vizSeries(func(s VizSample) (float64, bool) { return s.NPUUtilPct, s.NPUUtilKnown }),
			npuAggregateSparkWidth,
		))
	}

	return segments
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

	// The aggregate gauge bar and the heat legend only exist when they have
	// something to say: a cluster with no chip data gets no aggregate bar (the
	// summary's "monitoring unavailable" already covers it), and the health
	// matrix keeps the bytes it had before either existed.
	if agg := m.npuAggregateSegments(nodes, m.currentVizSample()); len(agg) > 0 {
		lines = append(lines, wrapSummarySegments(agg, m.width, "  •  ")...)
	}
	if heat := m.npuHeatLegend(); heat != "" {
		lines = append(lines, heat)
	}

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
