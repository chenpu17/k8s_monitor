package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderNetwork renders the network view
func (m *Model) renderNetwork() string {
	if m.clusterData == nil {
		return m.T("msg.no_data")
	}

	allLines := m.networkContentLines()
	budget := m.contentBudget()
	startIdx, endIdx := scrollWindow(len(allLines), m.scrollOffset, budget, scrollIndicatorLines)

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator if needed
	if len(allLines) > budget {
		scrollInfo := StyleTextMuted.Render(fmt.Sprintf("\n[%s] (↑/↓ %s, PgUp/PgDn %s)",
			m.TF("scroll.showing", map[string]interface{}{
				"Start": startIdx + 1,
				"End":   endIdx,
				"Total": len(allLines),
			}),
			m.T("keys.scroll"),
			m.T("keys.page"),
		))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// networkContentLines returns every line of the network view, in order, before
// it is sliced down to what fits on screen. Scrolling and the frame height both
// work off this list, so the window and the "showing X-Y of Z" indicator can
// never disagree with the content.
func (m *Model) networkContentLines() []string {
	if m.clusterData == nil {
		return nil
	}

	var allLines []string

	// Header
	header := m.renderNetworkHeader()
	allLines = append(allLines, header, "")

	// Services and Endpoints
	if len(m.clusterData.Services) > 0 {
		servicesLines := strings.Split(m.renderServices(), "\n")
		allLines = append(allLines, servicesLines...)
		allLines = append(allLines, "")
	}

	// Pod Network Information
	podNetworkLines := strings.Split(m.renderPodNetwork(), "\n")
	allLines = append(allLines, podNetworkLines...)

	return allLines
}

// networkMaxScroll returns the largest scroll offset the network content can be
// scrolled to.
func (m *Model) networkMaxScroll() int {
	return maxScrollOffset(len(m.networkContentLines()), m.contentBudget(), scrollIndicatorLines)
}

// networkScrollBy moves the network view by delta lines without ever stepping
// past the end of the content. Scrolling up starts from the offset that is
// actually on screen, so a single ↑ always moves the view one line - also after
// a refresh shortened the content under a dangling offset.
func (m *Model) networkScrollBy(delta int) {
	limit := m.networkMaxScroll()
	// Start from the offset that is on screen: a dangling offset (the content
	// shrank under it on a refresh) must not swallow the next ↑ presses.
	offset := min(m.scrollOffset, limit) + delta
	m.scrollOffset = max(0, min(offset, limit))
}

// scrollLineViewBy scrolls the views that move by content lines rather than by
// selected item (overview and network), and reports whether it handled the key.
// Both views render far more lines than fit on screen, so their offset has to be
// clamped against the content: an unclamped offset runs away past the end and
// then needs several ↑ presses before the view moves again.
func (m *Model) scrollLineViewBy(delta int) bool {
	switch m.currentView {
	case ViewOverview:
		m.overviewScrollBy(delta)
		return true
	case ViewNetwork:
		m.networkScrollBy(delta)
		return true
	}
	return false
}

// renderNetworkHeader renders the network view header
func (m *Model) renderNetworkHeader() string {
	title := StyleHeader.Render("🌐 " + m.T("views.network.title"))

	totalServices := len(m.clusterData.Services)
	totalEndpoints := 0
	for _, svc := range m.clusterData.Services {
		totalEndpoints += svc.EndpointCount
	}

	summary := m.TF("network.stats", map[string]interface{}{
		"Services":  totalServices,
		"Endpoints": totalEndpoints,
	})

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		StyleTextSecondary.Render(summary),
	)
}

// renderServices renders services and their endpoints
func (m *Model) renderServices() string {
	var rows []string

	// Count service stats
	totalServices := len(m.clusterData.Services)
	servicesWithEndpoints := 0
	servicesWithoutEndpoints := 0
	totalEndpoints := 0

	for _, svc := range m.clusterData.Services {
		totalEndpoints += svc.EndpointCount
		if svc.EndpointCount > 0 {
			servicesWithEndpoints++
		} else {
			servicesWithoutEndpoints++
		}
	}

	// Header with stats. network.stats_detailed is a whole sentence carrying the
	// WithEP/Without/Total template variables, so it must be rendered through TF
	// with real values (calling T() alone leaves the placeholders as <no value>).
	detailedStats := m.TF("network.stats_detailed", map[string]interface{}{
		"WithEP":  StyleStatusReady.Render(fmt.Sprintf("%d", servicesWithEndpoints)),
		"Without": StyleStatusNotReady.Render(fmt.Sprintf("%d", servicesWithoutEndpoints)),
		"Total":   totalEndpoints,
	})

	header := fmt.Sprintf("%s  (%s: %d • %s)",
		StyleSubHeader.Render(m.T("network.services_endpoints")),
		m.T("common.total"),
		totalServices,
		detailedStats,
	)
	rows = append(rows, header)
	rows = append(rows, "")

	const (
		colName      = 28
		colNamespace = 12
		colType      = 14
		colClusterIP = 16
		colPorts     = 25
		colEndpoints = 12
		colStatus    = 8
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.name"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.type"), colType),
		padRight(m.T("columns.cluster_ip"), colClusterIP),
		padRight(m.T("columns.ports"), colPorts),
		padRight(m.T("columns.endpoints"), colEndpoints),
		padRight(m.T("columns.status"), colStatus),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	// Sort services: without endpoints first (they need attention) - O(n log n)
	sortedServices := make([]*model.ServiceData, len(m.clusterData.Services))
	copy(sortedServices, m.clusterData.Services)
	sort.Slice(sortedServices, func(i, j int) bool {
		return sortedServices[i].EndpointCount < sortedServices[j].EndpointCount
	})

	for _, svc := range sortedServices {
		// Format ports with better truncation
		var portStrs []string
		for i, port := range svc.Ports {
			if i >= 2 { // Show max 2 ports
				portStrs = append(portStrs, m.TF("network.ports_more", map[string]interface{}{
					"Count": len(svc.Ports) - 2,
				}))
				break
			}
			if port.NodePort > 0 {
				portStrs = append(portStrs, fmt.Sprintf("%d:%d/%s", port.Port, port.NodePort, port.Protocol))
			} else {
				portStrs = append(portStrs, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
			}
		}
		portsStr := strings.Join(portStrs, ", ")

		// Color code endpoints
		endpointsStr := fmt.Sprintf("%d", svc.EndpointCount)
		statusStr := ""
		if svc.EndpointCount == 0 {
			endpointsStr = StyleStatusNotReady.Render(endpointsStr)
			statusStr = StyleStatusNotReady.Render(m.T("network.status_no_endpoints"))
		} else {
			endpointsStr = StyleStatusReady.Render(endpointsStr)
			statusStr = StyleStatusReady.Render(m.T("network.status_ready"))
		}

		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s",
			padRight(truncate(svc.Name, colName), colName),
			padRight(truncate(svc.Namespace, colNamespace), colNamespace),
			padRight(truncate(svc.Type, colType), colType),
			padRight(truncate(svc.ClusterIP, colClusterIP), colClusterIP),
			padRight(truncate(portsStr, colPorts), colPorts),
			padRight(endpointsStr, colEndpoints),
			padRight(statusStr, colStatus),
		)
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// renderPodNetwork renders pod network information
func (m *Model) renderPodNetwork() string {
	var rows []string
	rows = append(rows, StyleSubHeader.Render(m.T("network.pod_network_title")))
	rows = append(rows, "")

	const (
		colName      = 30
		colNamespace = 12
		colPodIP     = 16
		colNode      = 20
		colRx        = 15
		colTx        = 15
	)

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(m.T("columns.pod"), colName),
		padRight(m.T("columns.namespace"), colNamespace),
		padRight(m.T("columns.pod_ip"), colPodIP),
		padRight(m.T("columns.node"), colNode),
		padRight(m.T("network.pod_network_rx"), colRx),
		padRight(m.T("network.pod_network_tx"), colTx),
	)
	rows = append(rows, StyleTextMuted.Render(headerRow))

	// Show pods with network info, sorted by current network rate (MB/s)
	type podTraffic struct {
		pod       *model.PodData
		rxRate    float64
		txRate    float64
		totalRate float64
	}

	var podList []podTraffic
	for _, pod := range m.clusterData.Pods {
		// Only skip if both PodIP and HostIP are empty
		// This ensures we show hostNetwork pods and Pending pods with HostIP
		if pod.PodIP == "" && pod.HostIP == "" {
			continue
		}

		// Calculate current network rate (MB/s)
		rxRate := m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
		txRate := m.calculatePodNetworkTxRate(pod.Namespace, pod.Name)
		totalRate := rxRate + txRate

		podList = append(podList, podTraffic{
			pod:       pod,
			rxRate:    rxRate,
			txRate:    txRate,
			totalRate: totalRate,
		})
	}

	// Sort by total rate (descending) - O(n log n)
	// This ensures pods with highest CURRENT throughput appear at top
	sort.Slice(podList, func(i, j int) bool {
		return podList[i].totalRate > podList[j].totalRate
	})

	// Show top pods
	count := 0
	maxDisplay := 20
	for _, pt := range podList {
		pod := pt.pod

		podIP := pod.PodIP
		if podIP == "" {
			podIP = StyleTextMuted.Render("-")
		}

		// Use pre-calculated rates
		rxStr := formatNetworkRate(pt.rxRate)
		txStr := formatNetworkRate(pt.txRate)

		row := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(truncate(pod.Name, colName), colName),
			padRight(truncate(pod.Namespace, colNamespace), colNamespace),
			padRight(podIP, colPodIP),
			padRight(truncate(pod.Node, colNode), colNode),
			padRight(rxStr, colRx),
			padRight(txStr, colTx),
		)
		rows = append(rows, row)

		count++
		if count >= maxDisplay {
			rows = append(rows, "")
			rows = append(rows, StyleTextMuted.Render("  "+m.TF("network.pod_network_showing_top", map[string]interface{}{
				"Top":   maxDisplay,
				"Total": len(podList),
			})))
			break
		}
	}

	if count == 0 {
		rows = append(rows, StyleTextMuted.Render("  "+m.T("network.pod_network_no_pods")))
	}

	return strings.Join(rows, "\n")
}
