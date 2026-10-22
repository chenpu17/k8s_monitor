package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderServiceDetail renders the service detail view
func (m *Model) renderServiceDetail() string {
	if m.selectedService == nil {
		return m.T("service.no_selected")
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
		scrollInfo := StyleTextMuted.Render("\n" + m.TF("detail.scroll_indicator_short", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		}))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderServiceBasicInfo renders service basic information
func (m *Model) renderServiceBasicInfo(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.TF("service.title", map[string]interface{}{"Name": svc.Name})))
	info = append(info, "")

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("service.label_namespace")),
		svc.Namespace))

	// Service type
	svcType := svc.Type
	if svcType == "" {
		svcType = "ClusterIP"
	}
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("service.label_type")),
		StyleHighlight.Render(svcType)))

	// Cluster IP
	if svc.ClusterIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("service.label_cluster_ip")),
			svc.ClusterIP))
	}

	// External IPs
	if len(svc.ExternalIPs) > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("service.label_external_ips")),
			strings.Join(svc.ExternalIPs, ", ")))
	}

	// LoadBalancer IP
	if svc.LoadBalancerIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("service.label_loadbalancer_ip")),
			svc.LoadBalancerIP))
	}

	// Ingress
	if len(svc.Ingress) > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("service.label_ingress")),
			strings.Join(svc.Ingress, ", ")))
	}

	// Age
	age := time.Since(svc.CreationTimestamp)
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("service.label_age")),
		formatAge(age)))

	return strings.Join(info, "\n")
}

// renderServiceEndpoints renders endpoint information
func (m *Model) renderServiceEndpoints(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleSubHeader.Render(m.T("service.endpoints_header")))
	info = append(info, "")

	endpointCount := svc.EndpointCount
	if endpointCount == 0 {
		info = append(info, StyleWarning.Render(m.TF("service.no_ready_endpoints", map[string]interface{}{"Count": endpointCount})))
		info = append(info, StyleTextMuted.Render(m.T("service.no_endpoints_hint")))
	} else {
		info = append(info, StyleStatusReady.Render(m.TF("service.ready_endpoints", map[string]interface{}{"Count": endpointCount})))
	}

	return strings.Join(info, "\n")
}

// renderServicePorts renders service ports information
func (m *Model) renderServicePorts(svc *model.ServiceData) string {
	var info []string

	info = append(info, StyleSubHeader.Render(m.T("service.ports_header")))
	info = append(info, "")

	if len(svc.Ports) == 0 {
		info = append(info, StyleTextMuted.Render(m.T("service.no_ports")))
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
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.protocol"), colProtocol),
		padRight(m.T("columns.port"), colPort),
		padRight(m.T("columns.target_port"), colTargetPort),
		padRight(m.T("columns.node_port"), colNodePort),
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

	info = append(info, StyleSubHeader.Render(m.T("service.selector_header")))
	info = append(info, "")

	if len(svc.Selector) == 0 {
		info = append(info, StyleTextMuted.Render(m.T("service.no_selector")))
	} else {
		for _, key := range sortedKeys(svc.Selector) {
			value := svc.Selector[key]
			info = append(info, fmt.Sprintf("  %s: %s",
				StyleTextSecondary.Render(key),
				value))
		}
	}

	info = append(info, "")
	info = append(info, StyleSubHeader.Render(m.T("service.labels_header")))
	info = append(info, "")

	if len(svc.Labels) == 0 {
		info = append(info, StyleTextMuted.Render(m.T("service.no_labels")))
	} else {
		for _, key := range sortedKeys(svc.Labels) {
			value := svc.Labels[key]
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
		info = append(info, StyleSubHeader.Render(m.T("service.pods_header")))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render(m.T("service.no_pod_info")))
		return strings.Join(info, "\n")
	}

	// Check if service has a selector
	if len(svc.Selector) == 0 {
		info = append(info, StyleSubHeader.Render(m.T("service.pods_header")))
		info = append(info, "")
		info = append(info, StyleTextMuted.Render(m.T("service.no_selector_external")))
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

	info = append(info, StyleSubHeader.Render(m.TF("service.pods_header_count", map[string]interface{}{"Count": len(matchingPods)})))
	info = append(info, "")

	if len(matchingPods) == 0 {
		info = append(info, StyleTextMuted.Render(m.T("service.no_pods_match")))
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
		info = append(info, StyleTextMuted.Render(m.TF("service.more_pods", map[string]interface{}{"Count": len(matchingPods) - displayCount})))
	}

	return strings.Join(info, "\n")
}
