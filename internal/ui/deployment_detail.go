package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderDeploymentDetail renders the deployment detail view
func (m *Model) renderDeploymentDetail() string {
	if m.selectedDeployment == nil {
		return "No deployment selected"
	}

	deploy := m.selectedDeployment

	// Build sections
	var sections []string

	// Basic info
	sections = append(sections, m.renderDeploymentBasicInfo(deploy))
	sections = append(sections, "")

	// Replica status
	sections = append(sections, m.renderDeploymentReplicaStatus(deploy))
	sections = append(sections, "")

	// Strategy
	sections = append(sections, m.renderDeploymentStrategy(deploy))
	sections = append(sections, "")

	// Selector
	sections = append(sections, m.renderDeploymentSelector(deploy))
	sections = append(sections, "")

	// Pods
	sections = append(sections, m.renderDeploymentPods(deploy))

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

// renderDeploymentBasicInfo renders deployment basic information
func (m *Model) renderDeploymentBasicInfo(deploy *model.DeploymentData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("🚀 Deployment: %s", deploy.Name)))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Namespace"),
		deploy.Namespace))

	// Age
	age := time.Since(deploy.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Age"),
		formatAge(age)))

	return strings.Join(info, "\n")
}

// renderDeploymentReplicaStatus renders replica status information
func (m *Model) renderDeploymentReplicaStatus(deploy *model.DeploymentData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Replica Status"))
	info = append(info, "")

	// Desired replicas
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Desired Replicas"),
		StyleHighlight.Render(fmt.Sprintf("%d", deploy.Replicas))))

	// Ready replicas
	readyStr := fmt.Sprintf("%d / %d", deploy.ReadyReplicas, deploy.Replicas)
	if deploy.ReadyReplicas == deploy.Replicas && deploy.Replicas > 0 {
		readyStr = StyleStatusReady.Render(readyStr)
	} else if deploy.ReadyReplicas == 0 {
		readyStr = StyleStatusNotReady.Render(readyStr)
	} else {
		readyStr = StyleStatusPending.Render(readyStr)
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Ready Replicas"),
		readyStr))

	// Updated replicas
	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render("Updated Replicas"),
		deploy.UpdatedReplicas))

	// Available replicas
	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render("Available Replicas"),
		deploy.AvailableReplicas))

	return strings.Join(info, "\n")
}

// renderDeploymentStrategy renders deployment strategy
func (m *Model) renderDeploymentStrategy(deploy *model.DeploymentData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Deployment Strategy"))
	info = append(info, "")

	strategy := deploy.Strategy
	if strategy == "" {
		strategy = "RollingUpdate"
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Type"),
		StyleHighlight.Render(strategy)))

	return strings.Join(info, "\n")
}

// renderDeploymentSelector renders selector and labels
func (m *Model) renderDeploymentSelector(deploy *model.DeploymentData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Selector"))
	info = append(info, "")

	if len(deploy.Selector) == 0 {
		info = append(info, StyleTextMuted.Render("  No selector configured"))
	} else {
		for key, value := range deploy.Selector {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	info = append(info, "")
	info = append(info, StyleSubHeader.Render("Labels"))
	info = append(info, "")

	if len(deploy.Labels) == 0 {
		info = append(info, StyleTextMuted.Render("  No labels"))
	} else {
		for key, value := range deploy.Labels {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	return strings.Join(info, "\n")
}

// renderDeploymentPods renders pods managed by this deployment
func (m *Model) renderDeploymentPods(deploy *model.DeploymentData) string {
	var info []string

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleSubHeader.Render("Managed Pods"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No pod information available"))
		return strings.Join(info, "\n")
	}

	// Check if deployment has a selector
	if len(deploy.Selector) == 0 {
		info = append(info, StyleSubHeader.Render("Managed Pods"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No selector configured (matchLabels is empty)"))
		return strings.Join(info, "\n")
	}

	// Find pods that match the deployment selector
	var matchingPods []*model.PodData
	for _, pod := range m.clusterData.Pods {
		// Check if pod is in same namespace
		if pod.Namespace != deploy.Namespace {
			continue
		}

		// Check if pod labels match deployment selector
		matches := true
		for selectorKey, selectorValue := range deploy.Selector {
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
		info = append(info, StyleTextMuted.Render("  No pods match this deployment selector"))
		return strings.Join(info, "\n")
	}

	// Table header
	const (
		colName     = 38
		colPhase    = 11
		colCPU      = 11
		colMemory   = 11
		colRx       = 10
		colTx       = 10
		colRestarts = 8
	)

	headerRow := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
		padRight("POD NAME", colName),
		padRight("PHASE", colPhase),
		padRight("CPU", colCPU),
		padRight("MEMORY", colMemory),
		padRight("RX ↓", colRx),
		padRight("TX ↑", colTx),
		padRight("RESTARTS", colRestarts),
	)
	info = append(info, StyleTextMuted.Render(headerRow))

	// Show up to 20 pods
	displayCount := len(matchingPods)
	if displayCount > 20 {
		displayCount = 20
	}

	for i := 0; i < displayCount; i++ {
		pod := matchingPods[i]

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

		restarts := fmt.Sprintf("%d", pod.RestartCount)
		if pod.RestartCount > 0 {
			restarts = StyleWarning.Render(restarts)
		}

		row := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
			padRight(truncate(pod.Name, colName), colName),
			padRight(phaseStyled, colPhase),
			padRight(cpuStr, colCPU),
			padRight(memStr, colMemory),
			padRight(rxStr, colRx),
			padRight(txStr, colTx),
			padRight(restarts, colRestarts),
		)
		info = append(info, row)
	}

	if len(matchingPods) > displayCount {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render(fmt.Sprintf("  ... and %d more pods", len(matchingPods)-displayCount)))
	}

	return strings.Join(info, "\n")
}
