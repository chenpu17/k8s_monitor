package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderDaemonSetDetail renders the daemonset detail view
func (m *Model) renderDaemonSetDetail() string {
	if m.selectedDaemonSet == nil {
		return "No daemonset selected"
	}

	ds := m.selectedDaemonSet

	// Build sections
	var sections []string

	// Basic info
	sections = append(sections, m.renderDaemonSetBasicInfo(ds))
	sections = append(sections, "")

	// Status
	sections = append(sections, m.renderDaemonSetStatus(ds))
	sections = append(sections, "")

	// Selector
	sections = append(sections, m.renderDaemonSetSelector(ds))
	sections = append(sections, "")

	// Pods
	sections = append(sections, m.renderDaemonSetPods(ds))

	// Split into lines for scroll handling
	lines := strings.Split(strings.Join(sections, "\n"), "\n")
	totalLines := len(lines)

	// Apply scroll offset
	maxVisible := m.height - 10
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Clamp scroll offset
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.detailScrollOffset > maxScroll {
		m.detailScrollOffset = maxScroll
	}
	if m.detailScrollOffset < 0 {
		m.detailScrollOffset = 0
	}

	startIdx := m.detailScrollOffset
	if startIdx >= totalLines {
		startIdx = totalLines - 1
		if startIdx < 0 {
			startIdx = 0
		}
	}

	endIdx := startIdx + maxVisible
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := lines[startIdx:endIdx]

	// Add scroll indicator
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render(fmt.Sprintf("\n[Lines %d-%d of %d] (↑/↓ to scroll)", startIdx+1, endIdx, totalLines))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderDaemonSetBasicInfo renders daemonset basic information
func (m *Model) renderDaemonSetBasicInfo(ds *model.DaemonSetData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("👾 DaemonSet: %s", ds.Name)))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Namespace"),
		ds.Namespace))

	// Age
	age := time.Since(ds.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Age"),
		formatAge(age)))

	return strings.Join(info, "\n")
}

// renderDaemonSetStatus renders daemonset status information
func (m *Model) renderDaemonSetStatus(ds *model.DaemonSetData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Status"))
	info = append(info, "")

	// Desired number scheduled
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Desired Number Scheduled"),
		StyleHighlight.Render(fmt.Sprintf("%d", ds.DesiredNumberScheduled))))

	// Current number scheduled
	currentStr := fmt.Sprintf("%d / %d", ds.CurrentNumberScheduled, ds.DesiredNumberScheduled)
	if ds.CurrentNumberScheduled == ds.DesiredNumberScheduled {
		currentStr = StyleStatusReady.Render(currentStr)
	} else {
		currentStr = StyleStatusPending.Render(currentStr)
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Current Number Scheduled"),
		currentStr))

	// Number ready
	readyStr := fmt.Sprintf("%d / %d", ds.NumberReady, ds.DesiredNumberScheduled)
	if ds.NumberReady == ds.DesiredNumberScheduled {
		readyStr = StyleStatusReady.Render(readyStr)
	} else if ds.NumberReady == 0 {
		readyStr = StyleStatusNotReady.Render(readyStr)
	} else {
		readyStr = StyleStatusPending.Render(readyStr)
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Number Ready"),
		readyStr))

	// Number available
	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render("Number Available"),
		ds.NumberAvailable))

	return strings.Join(info, "\n")
}

// renderDaemonSetSelector renders selector and labels
func (m *Model) renderDaemonSetSelector(ds *model.DaemonSetData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Selector"))
	info = append(info, "")

	if len(ds.Selector) == 0 {
		info = append(info, StyleTextMuted.Render("  No selector configured"))
	} else {
		for key, value := range ds.Selector {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	info = append(info, "")
	info = append(info, StyleSubHeader.Render("Labels"))
	info = append(info, "")

	if len(ds.Labels) == 0 {
		info = append(info, StyleTextMuted.Render("  No labels"))
	} else {
		for key, value := range ds.Labels {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	return strings.Join(info, "\n")
}

// renderDaemonSetPods renders pods managed by this daemonset
func (m *Model) renderDaemonSetPods(ds *model.DaemonSetData) string {
	var info []string

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleSubHeader.Render("Managed Pods"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No pod information available"))
		return strings.Join(info, "\n")
	}

	// Check if daemonset has a selector
	if len(ds.Selector) == 0 {
		info = append(info, StyleSubHeader.Render("Managed Pods"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No selector configured (matchLabels is empty)"))
		return strings.Join(info, "\n")
	}

	// Find pods that match the daemonset selector
	var matchingPods []*model.PodData
	for _, pod := range m.clusterData.Pods {
		// Check if pod is in same namespace
		if pod.Namespace != ds.Namespace {
			continue
		}

		// Check if pod labels match daemonset selector
		matches := true
		for selectorKey, selectorValue := range ds.Selector {
			if podLabelValue, exists := pod.Labels[selectorKey]; !exists || podLabelValue != selectorValue {
				matches = false
				break
			}
		}

		if matches {
			matchingPods = append(matchingPods, pod)
		}
	}

	info = append(info, StyleSubHeader.Render(fmt.Sprintf("Managed Pods (%d)", len(matchingPods))))
	info = append(info, "")

	if len(matchingPods) == 0 {
		info = append(info, StyleTextMuted.Render("  No pods match this daemonset selector"))
		return strings.Join(info, "\n")
	}

	// Table header
	const (
		colName   = 35
		colNode   = 22
		colPhase  = 11
		colCPU    = 10
		colMemory = 10
		colRx     = 9
		colTx     = 9
	)

	headerRow := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.pod_name"), colName),
		padRight(m.T("columns.node"), colNode),
		padRight(m.T("columns.phase"), colPhase),
		padRight(m.T("columns.cpu"), colCPU),
		padRight(m.T("columns.memory"), colMemory),
		padRight(m.T("columns.rx"), colRx),
		padRight(m.T("columns.tx"), colTx),
	)
	info = append(info, StyleTextMuted.Render(headerRow))

	// Show all pods (DaemonSets typically have 1 pod per node)
	for _, pod := range matchingPods {
		phase := pod.Phase
		var phaseStyled string
		switch phase {
		case "Running":
			phaseStyled = StyleStatusRunning.Render(phase)
		case "Succeeded":
			phaseStyled = StyleStatusReady.Render(phase)
		case "Failed":
			phaseStyled = StyleStatusNotReady.Render(phase)
		case "Pending":
			phaseStyled = StyleStatusPending.Render(phase)
		default:
			phaseStyled = StyleTextMuted.Render(phase)
		}

		cpuStr := FormatMillicores(pod.CPUUsage)
		if pod.CPUUsage == 0 {
			cpuStr = StyleTextMuted.Render("-")
		}

		memStr := FormatBytes(pod.MemoryUsage)
		if pod.MemoryUsage == 0 {
			memStr = StyleTextMuted.Render("-")
		}

		// Network RX (download/receive)
		rxRate := m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
		rxStr := formatNetworkRate(rxRate)
		if rxRate == 0 {
			rxStr = StyleTextMuted.Render("-")
		}

		// Network TX (upload/send)
		txRate := m.calculatePodNetworkTxRate(pod.Namespace, pod.Name)
		txStr := formatNetworkRate(txRate)
		if txRate == 0 {
			txStr = StyleTextMuted.Render("-")
		}

		nodeName := pod.Node
		if nodeName == "" {
			nodeName = "-"
		}

		row := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
			padRight(truncate(pod.Name, colName), colName),
			padRight(truncate(nodeName, colNode), colNode),
			padRight(phaseStyled, colPhase),
			padRight(cpuStr, colCPU),
			padRight(memStr, colMemory),
			padRight(rxStr, colRx),
			padRight(txStr, colTx),
		)
		info = append(info, row)
	}

	return strings.Join(info, "\n")
}
