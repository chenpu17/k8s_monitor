package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderServiceDetail renders the service detail view
func (m *Model) renderServiceDetail() string {
	if m.selectedService == nil {
		return "No service selected"
	}

	svc := m.selectedService

	// Build sections
	var sections []string

	// Basic info
	sections = append(sections, m.renderServiceBasicInfo(svc))
	sections = append(sections, "")

	// Endpoints info
	sections = append(sections, m.renderServiceEndpoints(svc))
	sections = append(sections, "")

	// Ports info
	sections = append(sections, m.renderServicePorts(svc))
	sections = append(sections, "")

	// Selector and labels
	sections = append(sections, m.renderServiceSelector(svc))
	sections = append(sections, "")

	// Pods backing this service
	sections = append(sections, m.renderServicePods(svc))

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
	detailScrollOffset := m.detailScrollOffset
	if detailScrollOffset > maxScroll {
		detailScrollOffset = maxScroll
	}
	if detailScrollOffset < 0 {
		detailScrollOffset = 0
	}

	startIdx := detailScrollOffset
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

// renderServiceBasicInfo renders service basic information
func (m *Model) renderServiceBasicInfo(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("🔌 Service: %s", svc.Name)))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Namespace"),
		svc.Namespace))

	// Service type
	svcType := svc.Type
	if svcType == "" {
		svcType = "ClusterIP"
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Type"),
		StyleHighlight.Render(svcType)))

	// Cluster IP
	if svc.ClusterIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("Cluster IP"),
			svc.ClusterIP))
	}

	// External IPs
	if len(svc.ExternalIPs) > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("External IPs"),
			strings.Join(svc.ExternalIPs, ", ")))
	}

	// LoadBalancer IP
	if svc.LoadBalancerIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("LoadBalancer IP"),
			svc.LoadBalancerIP))
	}

	// Ingress
	if len(svc.Ingress) > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("Ingress"),
			strings.Join(svc.Ingress, ", ")))
	}

	// Age
	age := time.Since(svc.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render("Age"),
		formatAge(age)))

	return strings.Join(info, "\n")
}

// renderServiceEndpoints renders endpoint information
func (m *Model) renderServiceEndpoints(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Endpoints"))
	info = append(info, "")

	endpointCount := svc.EndpointCount
	if endpointCount == 0 {
		info = append(info, StyleWarning.Render(fmt.Sprintf("  ⚠️  No ready endpoints (%d)", endpointCount)))
		info = append(info, StyleTextMuted.Render("  This service has no pods backing it or pods are not ready"))
	} else {
		info = append(info, StyleStatusReady.Render(fmt.Sprintf("  ✓ %d ready endpoint(s)", endpointCount)))
	}

	return strings.Join(info, "\n")
}

// renderServicePorts renders service ports information
func (m *Model) renderServicePorts(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Ports"))
	info = append(info, "")

	if len(svc.Ports) == 0 {
		info = append(info, StyleTextMuted.Render("  No ports configured"))
		return strings.Join(info, "\n")
	}

	// Table header
	const (
		colName       = 20
		colProtocol   = 10
		colPort       = 10
		colTargetPort = 15
		colNodePort   = 12
	)

	headerRow := fmt.Sprintf("  %s  %s  %s  %s  %s",
		padRight("NAME", colName),
		padRight("PROTOCOL", colProtocol),
		padRight("PORT", colPort),
		padRight("TARGET PORT", colTargetPort),
		padRight("NODE PORT", colNodePort),
	)
	info = append(info, StyleTextMuted.Render(headerRow))

	// Port rows
	for _, port := range svc.Ports {
		name := port.Name
		if name == "" {
			name = "-"
		}

		nodePort := "-"
		if port.NodePort > 0 {
			nodePort = fmt.Sprintf("%d", port.NodePort)
		}

		row := fmt.Sprintf("  %s  %s  %s  %s  %s",
			padRight(truncate(name, colName), colName),
			padRight(port.Protocol, colProtocol),
			padRight(fmt.Sprintf("%d", port.Port), colPort),
			padRight(port.TargetPort, colTargetPort),
			padRight(nodePort, colNodePort),
		)
		info = append(info, row)
	}

	return strings.Join(info, "\n")
}

// renderServiceSelector renders selector and labels
func (m *Model) renderServiceSelector(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleSubHeader.Render("Selector"))
	info = append(info, "")

	if len(svc.Selector) == 0 {
		info = append(info, StyleTextMuted.Render("  No selector configured"))
	} else {
		for key, value := range svc.Selector {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	info = append(info, "")
	info = append(info, StyleSubHeader.Render("Labels"))
	info = append(info, "")

	if len(svc.Labels) == 0 {
		info = append(info, StyleTextMuted.Render("  No labels"))
	} else {
		for key, value := range svc.Labels {
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	return strings.Join(info, "\n")
}

// renderServicePods renders pods backing this service
func (m *Model) renderServicePods(svc *model.ServiceData) string {
	var info []string

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleSubHeader.Render("Backing Pods"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No pod information available"))
		return strings.Join(info, "\n")
	}

	// Check if service has a selector
	if len(svc.Selector) == 0 {
		info = append(info, StyleSubHeader.Render("Backing Pods"))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  No selector configured (ExternalName, manual Endpoints, or headless service)"))
		return strings.Join(info, "\n")
	}

	// Find pods that match the service selector
	var matchingPods []*model.PodData
	for _, pod := range m.clusterData.Pods {
		// Check if pod is in same namespace
		if pod.Namespace != svc.Namespace {
			continue
		}

		// Check if pod labels match service selector
		matches := true
		for selectorKey, selectorValue := range svc.Selector {
			if podLabelValue, exists := pod.Labels[selectorKey]; !exists || podLabelValue != selectorValue {
				matches = false
				break
			}
		}

		if matches {
			matchingPods = append(matchingPods, pod)
		}
	}

	info = append(info, StyleSubHeader.Render(fmt.Sprintf("Backing Pods (%d)", len(matchingPods))))
	info = append(info, "")

	if len(matchingPods) == 0 {
		info = append(info, StyleTextMuted.Render("  No pods match this service selector"))
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
		padRight(m.T("columns.pod_name"), colName),
		padRight(m.T("columns.phase"), colPhase),
		padRight(m.T("columns.cpu"), colCPU),
		padRight(m.T("columns.memory"), colMemory),
		padRight(m.T("columns.rx"), colRx),
		padRight(m.T("columns.tx"), colTx),
		padRight(m.T("columns.restarts"), colRestarts),
	)
	info = append(info, StyleTextMuted.Render(headerRow))

	// Show up to 10 pods
	displayCount := len(matchingPods)
	if displayCount > 10 {
		displayCount = 10
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
