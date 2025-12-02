package ui

import (
	"fmt"
	"sort"
	"strings"
)

// renderTopology renders the SuperPod topology list view
func (m *Model) renderTopology() string {
	if m.clusterData == nil {
		return m.T("topology.loading")
	}

	superPods := m.getSuperPodTopology()
	if len(superPods) == 0 {
		return m.T("topology.no_superpods")
	}

	var lines []string

	// Header
	lines = append(lines, StyleHeader.Render(m.T("topology.title")))
	lines = append(lines, "")

	// Summary statistics
	totalSuperPods := len(superPods)
	var totalNodes int
	var totalNPU int64
	for _, sp := range superPods {
		totalNodes += sp.NodeCount
		totalNPU += sp.TotalNPU
	}

	summaryLine := fmt.Sprintf("%s: %d  •  %s: %d  •  %s: %d",
		m.T("topology.total_superpods"), totalSuperPods,
		m.T("topology.total_nodes"), totalNodes,
		m.T("topology.total_npu"), totalNPU,
	)
	lines = append(lines, StyleSubHeader.Render(summaryLine))
	lines = append(lines, "")

	// Table header
	colID := 12
	colNodes := 8
	colNPUPerNode := 12
	colTotalNPU := 12
	colIPs := 50

	headerRow := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(m.T("topology.col_superpod_id"), colID),
		padRight(m.T("topology.col_nodes"), colNodes),
		padRight(m.T("topology.col_npu_per_node"), colNPUPerNode),
		padRight(m.T("topology.col_total_npu"), colTotalNPU),
		padRight(m.T("topology.col_node_ips"), colIPs),
	)
	lines = append(lines, StyleTextMuted.Render(headerRow))
	lines = append(lines, StyleTextMuted.Render(strings.Repeat("─", colID+colNodes+colNPUPerNode+colTotalNPU+colIPs+8)))

	// Calculate visible range
	maxVisible := m.height - 12
	if maxVisible < 1 {
		maxVisible = 1
	}

	// Calculate scroll bounds (read-only, don't modify state in View)
	maxScroll := len(superPods) - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Use clamped scroll offset for display only
	scrollOffset := m.scrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Render SuperPod rows
	startIdx := scrollOffset
	endIdx := startIdx + maxVisible
	if endIdx > len(superPods) {
		endIdx = len(superPods)
	}

	for i := startIdx; i < endIdx; i++ {
		sp := superPods[i]

		// Build IP list (limit for display)
		ipList := sp.NodeIPs
		suffix := ""
		if len(ipList) > 5 {
			ipList = ipList[:5]
			suffix = fmt.Sprintf(" +%d", len(sp.NodeIPs)-5)
		}
		ipStr := strings.Join(ipList, ", ") + suffix

		// NPU per node
		npuPerNodeStr := fmt.Sprintf("%d NPU", sp.NPUPerNode)

		// Total NPU
		totalNPUStr := fmt.Sprintf("%d NPU", sp.TotalNPU)

		row := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(sp.ID, colID),
			padRight(fmt.Sprintf("%d", sp.NodeCount), colNodes),
			padRight(npuPerNodeStr, colNPUPerNode),
			padRight(totalNPUStr, colTotalNPU),
			truncate(ipStr, colIPs),
		)

		// Highlight selected row
		if i == m.selectedIndex {
			row = StyleSelected.Render(row)
		}

		lines = append(lines, row)
	}

	// Scroll indicator
	if len(superPods) > maxVisible {
		lines = append(lines, "")
		scrollInfo := StyleTextMuted.Render(m.TF("topology.scroll_indicator", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": len(superPods),
		}))
		lines = append(lines, scrollInfo)
	}

	// Help text
	lines = append(lines, "")
	lines = append(lines, StyleTextMuted.Render(m.T("topology.help_text")))

	return strings.Join(lines, "\n")
}

// renderSuperPodDetail renders the detail view for a selected SuperPod
func (m *Model) renderSuperPodDetail() string {
	if m.selectedSuperPod == nil {
		return m.T("topology.no_selected")
	}

	sp := m.selectedSuperPod
	var lines []string

	// Header
	lines = append(lines, StyleHeader.Render(fmt.Sprintf("🔷 %s: %s", m.T("topology.superpod_detail"), sp.ID)))
	lines = append(lines, "")

	// Basic Info Section
	lines = append(lines, StyleSubHeader.Render(m.T("topology.section_basic_info")))
	lines = append(lines, "")

	lines = append(lines, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("topology.superpod_id")),
		StyleHighlight.Render(sp.ID)))

	lines = append(lines, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("topology.node_count")),
		StyleHighlight.Render(fmt.Sprintf("%d", sp.NodeCount))))

	lines = append(lines, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("topology.npu_per_node")),
		StyleHighlight.Render(fmt.Sprintf("%d NPU", sp.NPUPerNode))))

	lines = append(lines, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("topology.total_npu")),
		StyleHighlight.Render(fmt.Sprintf("%d NPU", sp.TotalNPU))))

	// NPU Utilization progress bar
	if sp.TotalNPU > 0 {
		npuUtil := m.calculateSuperPodNPUUtilization(sp)
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %s: %s  %s",
			StyleTextSecondary.Render(m.T("topology.npu_utilization")),
			StyleHighlight.Render(FormatPercentage(npuUtil)),
			renderProgressBar(npuUtil, 30)))
	}

	lines = append(lines, "")

	// Node List Section
	lines = append(lines, StyleSubHeader.Render(m.T("topology.section_nodes")))
	lines = append(lines, "")

	// Get nodes in this SuperPod
	spNodes := m.getSuperPodNodes(sp.ID)

	if len(spNodes) == 0 {
		lines = append(lines, StyleTextMuted.Render("  "+m.T("topology.no_nodes")))
	} else {
		// Table header for nodes (compact view)
		colName := 30
		colIP := 16
		colNPUAlloc := 10
		colNPUUtil := 10
		colHBMUtil := 10
		colTemp := 8
		colHealth := 10

		nodeHeaderRow := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colName),
			padRight(m.T("columns.ip"), colIP),
			padRight(m.T("topology.col_npu_allocated"), colNPUAlloc),
			padRight(m.T("topology.npu_util_short"), colNPUUtil),
			padRight(m.T("topology.hbm_util_short"), colHBMUtil),
			padRight(m.T("topology.temp_short"), colTemp),
			padRight(m.T("topology.health_short"), colHealth),
		)
		lines = append(lines, StyleTextMuted.Render(nodeHeaderRow))

		for _, node := range spNodes {
			// NPU allocation
			npuAllocStr := fmt.Sprintf("%d/%d", node.NPUAllocated, node.NPUAllocatable)
			if node.NPUAllocated == node.NPUAllocatable && node.NPUAllocatable > 0 {
				npuAllocStr = StyleWarning.Render(npuAllocStr) // Fully allocated
			} else if node.NPUAllocated > 0 {
				npuAllocStr = StyleStatusRunning.Render(npuAllocStr)
			}

			// NPU Utilization
			npuUtilStr := "-"
			if node.NPUUtilization > 0 {
				npuUtilStr = FormatPercentage(node.NPUUtilization)
				if node.NPUUtilization > 90 {
					npuUtilStr = StyleDanger.Render(npuUtilStr)
				} else if node.NPUUtilization > 70 {
					npuUtilStr = StyleWarning.Render(npuUtilStr)
				} else {
					npuUtilStr = StyleStatusRunning.Render(npuUtilStr)
				}
			}

			// HBM Utilization
			hbmUtilStr := "-"
			if node.NPUMemoryUtil > 0 {
				hbmUtilStr = FormatPercentage(node.NPUMemoryUtil)
				if node.NPUMemoryUtil > 90 {
					hbmUtilStr = StyleDanger.Render(hbmUtilStr)
				} else if node.NPUMemoryUtil > 70 {
					hbmUtilStr = StyleWarning.Render(hbmUtilStr)
				} else {
					hbmUtilStr = StyleStatusRunning.Render(hbmUtilStr)
				}
			}

			// Temperature
			tempStr := "-"
			if node.NPUTemperature > 0 {
				tempStr = fmt.Sprintf("%d°C", node.NPUTemperature)
				if node.NPUTemperature > 80 {
					tempStr = StyleDanger.Render(tempStr)
				} else if node.NPUTemperature > 65 {
					tempStr = StyleWarning.Render(tempStr)
				}
			}

			// Health status
			healthStr := node.NPUHealthStatus
			if healthStr == "" {
				healthStr = "-"
			} else if healthStr == "Healthy" {
				healthStr = StyleStatusReady.Render(healthStr)
			} else if healthStr == "Warning" {
				healthStr = StyleWarning.Render(healthStr)
			} else {
				healthStr = StyleDanger.Render(healthStr)
			}

			nodeRow := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
				padRight(truncate(node.Name, colName), colName),
				padRight(node.InternalIP, colIP),
				padRight(npuAllocStr, colNPUAlloc),
				padRight(npuUtilStr, colNPUUtil),
				padRight(hbmUtilStr, colHBMUtil),
				padRight(tempStr, colTemp),
				healthStr,
			)
			lines = append(lines, nodeRow)
		}

		// Show detailed NPU info for first few nodes
		lines = append(lines, "")
		lines = append(lines, StyleSubHeader.Render(m.T("topology.section_npu_details")))
		lines = append(lines, "")

		displayNodes := spNodes
		if len(displayNodes) > 3 {
			displayNodes = displayNodes[:3]
		}

		for _, node := range displayNodes {
			// Node name header
			lines = append(lines, fmt.Sprintf("  %s %s",
				StyleHighlight.Render("▸"),
				StyleHighlight.Render(truncate(node.Name, 40))))

			// NPU chip info
			if node.NPUChipType != "" {
				lines = append(lines, fmt.Sprintf("    %s: %s",
					StyleTextSecondary.Render(m.T("npu.chip_type")),
					node.NPUChipType))
			}
			if node.NPUAICoreCount > 0 {
				lines = append(lines, fmt.Sprintf("    %s: %d",
					StyleTextSecondary.Render(m.T("topology.ai_cores")),
					node.NPUAICoreCount))
			}

			// HBM memory details
			if node.NPUMemoryTotal > 0 {
				hbmUsedStr := FormatBytes(node.NPUMemoryUsed)
				hbmTotalStr := FormatBytes(node.NPUMemoryTotal)
				lines = append(lines, fmt.Sprintf("    %s: %s / %s (%s)",
					StyleTextSecondary.Render(m.T("topology.hbm_memory")),
					hbmUsedStr, hbmTotalStr,
					FormatPercentage(node.NPUMemoryUtil)))
			}

			// Power consumption
			if node.NPUPower > 0 {
				lines = append(lines, fmt.Sprintf("    %s: %dW",
					StyleTextSecondary.Render(m.T("topology.power")),
					node.NPUPower))
			}

			// Error count
			if node.NPUErrorCount > 0 {
				lines = append(lines, fmt.Sprintf("    %s: %s",
					StyleTextSecondary.Render(m.T("topology.errors")),
					StyleDanger.Render(fmt.Sprintf("%d", node.NPUErrorCount))))
			}

			lines = append(lines, "")
		}

		if len(spNodes) > 3 {
			lines = append(lines, StyleTextMuted.Render(fmt.Sprintf("  ... +%d %s",
				len(spNodes)-3, m.T("topology.more_nodes"))))
		}
	}

	lines = append(lines, "")

	// Running Volcano Jobs Section (jobs using NPUs in this SuperPod)
	volcanoJobs := m.getSuperPodVolcanoJobs(sp.ID)
	if len(volcanoJobs) > 0 {
		lines = append(lines, StyleSubHeader.Render(m.T("topology.section_volcano_jobs")))
		lines = append(lines, "")

		// Table header for jobs
		colJobName := 40
		colJobNS := 20
		colJobStatus := 12
		colJobNPU := 10

		jobHeaderRow := fmt.Sprintf("  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colJobName),
			padRight(m.T("columns.namespace"), colJobNS),
			padRight(m.T("columns.status"), colJobStatus),
			padRight("NPU", colJobNPU),
		)
		lines = append(lines, StyleTextMuted.Render(jobHeaderRow))

		// Limit to first 10 jobs
		displayCount := len(volcanoJobs)
		if displayCount > 10 {
			displayCount = 10
		}

		for i := 0; i < displayCount; i++ {
			job := volcanoJobs[i]

			statusStr := job.Status
			switch job.Status {
			case "Running":
				statusStr = StyleStatusRunning.Render(job.Status)
			case "Pending":
				statusStr = StyleStatusPending.Render(job.Status)
			case "Completed":
				statusStr = StyleStatusReady.Render(job.Status)
			case "Failed":
				statusStr = StyleStatusNotReady.Render(job.Status)
			}

			npuStr := fmt.Sprintf("%d", job.NPURequested)
			if job.NPURequested > 0 {
				npuStr = StyleHighlight.Render(npuStr)
			}

			jobRow := fmt.Sprintf("  %s  %s  %s  %s",
				padRight(truncate(job.Name, colJobName), colJobName),
				padRight(truncate(job.Namespace, colJobNS), colJobNS),
				padRight(statusStr, colJobStatus),
				npuStr,
			)
			lines = append(lines, jobRow)
		}

		if len(volcanoJobs) > displayCount {
			lines = append(lines, StyleTextMuted.Render(fmt.Sprintf("  ... +%d %s", len(volcanoJobs)-displayCount, m.T("topology.more_jobs"))))
		}

		lines = append(lines, "")
	}

	// Network Statistics Section
	rxRate, txRate := m.calculateSuperPodNetworkRate(sp.ID)
	if rxRate > 0 || txRate > 0 {
		lines = append(lines, StyleSubHeader.Render(m.T("topology.section_network")))
		lines = append(lines, "")

		lines = append(lines, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("topology.network_rx")),
			StyleHighlight.Render(formatNetworkRate(rxRate))))

		lines = append(lines, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("topology.network_tx")),
			StyleHighlight.Render(formatNetworkRate(txRate))))

		lines = append(lines, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("topology.network_total")),
			StyleHighlight.Render(formatNetworkRate(rxRate+txRate))))

		lines = append(lines, "")
	}

	// Apply scroll offset for long content
	allLines := lines
	maxVisible := m.height - 8
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalLines := len(allLines)
	maxScroll := totalLines - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}

	// Use clamped scroll offset for display only (don't modify state in View)
	detailScrollOffset := m.detailScrollOffset
	if detailScrollOffset > maxScroll {
		detailScrollOffset = maxScroll
	}
	if detailScrollOffset < 0 {
		detailScrollOffset = 0
	}

	startIdx := detailScrollOffset
	endIdx := startIdx + maxVisible
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render(m.TF("detail.scroll_indicator", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		}))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// getSuperPodNodes returns nodes belonging to a specific SuperPod
func (m *Model) getSuperPodNodes(superPodID string) []*nodeInfo {
	if m.clusterData == nil || len(m.clusterData.Nodes) == 0 {
		return nil
	}

	var nodes []*nodeInfo
	for _, node := range m.clusterData.Nodes {
		if node.SuperPodID == superPodID {
			nodes = append(nodes, &nodeInfo{
				Name:            node.Name,
				InternalIP:      node.InternalIP,
				Status:          node.Status,
				NPUCapacity:     node.NPUCapacity,
				NPUAllocatable:  node.NPUAllocatable,
				NPUAllocated:    node.NPUAllocated,
				NPUUtilization:  node.NPUUtilization,
				NPUMemoryTotal:  node.NPUMemoryTotal,
				NPUMemoryUsed:   node.NPUMemoryUsed,
				NPUMemoryUtil:   node.NPUMemoryUtil,
				NPUTemperature:  node.NPUTemperature,
				NPUPower:        node.NPUPower,
				NPUHealthStatus: node.NPUHealthStatus,
				NPUErrorCount:   node.NPUErrorCount,
				NPUAICoreCount:  node.NPUAICoreCount,
				NPUChipType:     node.NPUChipType,
			})
		}
	}

	// Sort by name
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})

	return nodes
}

// nodeInfo is a simplified node structure for topology display
type nodeInfo struct {
	Name           string
	InternalIP     string
	Status         string
	NPUCapacity    int64
	NPUAllocatable int64
	NPUAllocated   int64
	// Rich NPU metrics
	NPUUtilization  float64 // AI Core utilization %
	NPUMemoryTotal  int64   // HBM total bytes
	NPUMemoryUsed   int64   // HBM used bytes
	NPUMemoryUtil   float64 // HBM utilization %
	NPUTemperature  int     // Temperature in Celsius
	NPUPower        int     // Power in Watts
	NPUHealthStatus string  // Health status
	NPUErrorCount   int     // Error count
	NPUAICoreCount  int     // AI core count
	NPUChipType     string  // Chip type (e.g., Ascend910)
}

// getSuperPodVolcanoJobs returns Volcano jobs running on nodes in this SuperPod
func (m *Model) getSuperPodVolcanoJobs(superPodID string) []*volcanoJobInfo {
	if m.clusterData == nil || len(m.clusterData.VolcanoJobs) == 0 {
		return nil
	}

	// Get node names in this SuperPod
	nodeNames := make(map[string]bool)
	for _, node := range m.clusterData.Nodes {
		if node.SuperPodID == superPodID {
			nodeNames[node.Name] = true
		}
	}

	// Find jobs with pods running on these nodes
	var jobs []*volcanoJobInfo
	jobSet := make(map[string]bool) // Dedup by job name

	for _, job := range m.clusterData.VolcanoJobs {
		if jobSet[job.Namespace+"/"+job.Name] {
			continue
		}

		// Check if any of the job's pods are on SuperPod nodes
		jobPods := m.getVolcanoJobPods(job)
		for _, pod := range jobPods {
			if nodeNames[pod.Node] {
				jobs = append(jobs, &volcanoJobInfo{
					Name:         job.Name,
					Namespace:    job.Namespace,
					Status:       job.Status,
					NPURequested: job.NPURequested,
				})
				jobSet[job.Namespace+"/"+job.Name] = true
				break
			}
		}
	}

	// Sort by name
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Name < jobs[j].Name
	})

	return jobs
}

// volcanoJobInfo is a simplified Volcano job structure for topology display
type volcanoJobInfo struct {
	Name         string
	Namespace    string
	Status       string
	NPURequested int64
}

// calculateSuperPodNPUUtilization calculates NPU utilization for a SuperPod
func (m *Model) calculateSuperPodNPUUtilization(sp *SuperPodInfo) float64 {
	if sp.TotalNPU == 0 || m.clusterData == nil {
		return 0
	}

	var totalAllocated int64
	for _, node := range m.clusterData.Nodes {
		if node.SuperPodID == sp.ID {
			totalAllocated += node.NPUAllocated
		}
	}

	return float64(totalAllocated) / float64(sp.TotalNPU) * 100
}

// calculateSuperPodNetworkRate calculates network rate for all nodes in a SuperPod
func (m *Model) calculateSuperPodNetworkRate(superPodID string) (rxRate, txRate float64) {
	if m.clusterData == nil {
		return 0, 0
	}

	for _, node := range m.clusterData.Nodes {
		if node.SuperPodID == superPodID {
			rxRate += m.calculateNodeNetworkRxRate(node.Name)
			txRate += m.calculateNodeNetworkTxRate(node.Name)
		}
	}

	return rxRate, txRate
}

// hasSuperPodTopology returns true if cluster has SuperPod topology info
func (m *Model) hasSuperPodTopology() bool {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return false
	}
	return m.clusterData.Summary.NPUCapacity > 0 && m.clusterData.Summary.SuperPodCount > 0
}
