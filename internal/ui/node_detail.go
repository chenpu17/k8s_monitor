package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chenpu/k8s-monitor/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// renderNodeDetail renders the node detail view
func (m *Model) renderNodeDetail() string {
	if m.selectedNode == nil {
		return m.T("detail.node.no_selected")
	}

	node := m.selectedNode

	// Collect all content lines
	var allLines []string

	// Node header
	allLines = append(allLines, m.renderNodeDetailHeader(node))
	allLines = append(allLines, "")

	// Node basic info
	basicInfo := m.renderNodeBasicInfo(node)
	allLines = append(allLines, strings.Split(basicInfo, "\n")...)
	allLines = append(allLines, "")

	// Node resource info
	resourceInfo := m.renderNodeResourceInfo(node)
	allLines = append(allLines, strings.Split(resourceInfo, "\n")...)
	allLines = append(allLines, "")

	// Pods running on this node
	podsInfo := m.renderNodePodsInfo(node)
	allLines = append(allLines, strings.Split(podsInfo, "\n")...)

	// Apply scroll offset
	maxVisible := m.height - 8 // Reserve space for header/footer
	if maxVisible < 1 {
		maxVisible = 1
	}

	totalLines := len(allLines)

	// Clamp scroll offset to valid range (prevent scrolling beyond content)
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
	endIdx := startIdx + maxVisible
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := allLines[startIdx:endIdx]

	// Add scroll indicator if needed
	if totalLines > maxVisible {
		scrollInfo := StyleTextMuted.Render("\n" + m.TF("detail.scroll_indicator", map[string]interface{}{
			"Start": startIdx + 1,
			"End":   endIdx,
			"Total": totalLines,
		}))
		visibleLines = append(visibleLines, scrollInfo)
	}

	return strings.Join(visibleLines, "\n")
}

// renderNodeDetailHeader renders the node detail view header
func (m *Model) renderNodeDetailHeader(node *model.NodeData) string {
	title := StyleHeader.Render(fmt.Sprintf("💻  %s: %s", m.T("detail.node.title"), node.Name))
	status := RenderStatus(node.Status)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		status,
	)
}

// renderNodeBasicInfo renders basic node information
func (m *Model) renderNodeBasicInfo(node *model.NodeData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.node.basic_info")))
	info = append(info, "")
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.name")),
		node.Name))

	if len(node.Roles) > 0 {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render("Roles"),
			strings.Join(node.Roles, ", ")))
	}

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.status")),
		RenderStatus(node.Status)))

	if node.InternalIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.internal_ip")),
			node.InternalIP))
	}

	if node.ExternalIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.external_ip")),
			node.ExternalIP))
	}

	return strings.Join(info, "\n")
}

// renderNodeResourceInfo renders node resource information
func (m *Model) renderNodeResourceInfo(node *model.NodeData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.node.resource_info")))
	info = append(info, "")

	// CPU
	if node.CPUAllocatable > 0 {
		cpuUsage := "N/A"
		cpuPercent := "N/A"
		if node.CPUUsage > 0 {
			cpuUsage = FormatMillicores(node.CPUUsage)
			cpuPercent = fmt.Sprintf("%.1f%%", float64(node.CPUUsage)*100.0/float64(node.CPUAllocatable))
		}
		info = append(info, fmt.Sprintf("  %s: %s / %s (%s)",
			StyleTextSecondary.Render(m.T("detail.field.cpu")),
			cpuUsage,
			FormatMillicores(node.CPUAllocatable),
			cpuPercent))
	}

	// Memory
	if node.MemAllocatable > 0 {
		memUsage := "N/A"
		memPercent := "N/A"
		if node.MemoryUsage > 0 {
			memUsage = FormatBytes(node.MemoryUsage)
			memPercent = fmt.Sprintf("%.1f%%", float64(node.MemoryUsage)*100.0/float64(node.MemAllocatable))
		}
		info = append(info, fmt.Sprintf("  %s: %s / %s (%s)",
			StyleTextSecondary.Render(m.T("detail.field.memory")),
			memUsage,
			FormatBytes(node.MemAllocatable),
			memPercent))
	}

	// Pods
	if node.PodAllocatable > 0 {
		podPercent := fmt.Sprintf("%.1f%%", float64(node.PodCount)*100.0/float64(node.PodAllocatable))
		info = append(info, fmt.Sprintf("  %s: %d / %d (%s)",
			StyleTextSecondary.Render(m.T("detail.field.pods")),
			node.PodCount,
			node.PodAllocatable,
			podPercent))
	}

	// NPU (Ascend AI accelerator)
	if node.NPUCapacity > 0 {
		npuPercent := "0.0%"
		if node.NPUAllocatable > 0 {
			npuPercent = fmt.Sprintf("%.1f%%", float64(node.NPUAllocated)*100.0/float64(node.NPUAllocatable))
		}
		info = append(info, fmt.Sprintf("  %s: %d / %d (%s)",
			StyleTextSecondary.Render("NPU"),
			node.NPUAllocated,
			node.NPUAllocatable,
			npuPercent))

		// NPU details
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  NPU Details"))
		if node.NPUChipType != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render("Chip Type"),
				node.NPUChipType))
		}
		if node.NPUDeviceType != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render("Device Type"),
				node.NPUDeviceType))
		}
		if node.NPUDriverVersion != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render("Driver Version"),
				node.NPUDriverVersion))
		}
		if node.NPUResourceName != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render("Resource Name"),
				node.NPUResourceName))
		}

		// Topology information
		if node.HyperNodeID != "" || node.SuperPodID != "" {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  Topology"))
			if node.SuperPodID != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render("SuperPod ID"),
					node.SuperPodID))
			}
			if node.HyperNodeID != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render("HyperNode ID"),
					truncate(node.HyperNodeID, 36)))
			}
			if node.CabinetInfo != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render("Cabinet"),
					node.CabinetInfo))
			}
		}

		// NPU Runtime Metrics (from k8s-monitor collector)
		if len(node.NPUChips) > 0 {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  NPU Runtime Metrics"))

			// Overall status
			healthStyle := StyleStatusReady
			if node.NPUHealthStatus == "Warning" {
				healthStyle = StyleWarning
			} else if node.NPUHealthStatus == "Unhealthy" {
				healthStyle = StyleStatusNotReady
			}
			// The runtime metrics (including this health status) come from
			// npu-exporter, unlike the per-chip bitmap block below which comes
			// from the device plugin: label the source so the two are not read
			// as one signal.
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(fmt.Sprintf("Health Status (%s)", m.T("npudetail.health_exporter"))),
				healthStyle.Render(node.NPUHealthStatus)))

			// Average utilization
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render("AI Core Util"),
				StyleHighlight.Render(fmt.Sprintf("%.1f%%", node.NPUUtilization))))

			// HBM Memory
			if node.NPUMemoryTotal > 0 {
				memUsedGB := float64(node.NPUMemoryUsed) / (1024 * 1024 * 1024)
				memTotalGB := float64(node.NPUMemoryTotal) / (1024 * 1024 * 1024)
				info = append(info, fmt.Sprintf("    %s: %.1f / %.1f GiB (%.1f%%)",
					StyleTextMuted.Render("HBM Memory"),
					memUsedGB, memTotalGB, node.NPUMemoryUtil))
			}

			// Temperature and Power
			info = append(info, fmt.Sprintf("    %s: %d W   %s: %d °C",
				StyleTextMuted.Render("Total Power"),
				node.NPUPower,
				StyleTextMuted.Render("Avg Temp"),
				node.NPUTemperature))

			// Metrics timestamp
			if !node.NPUMetricsTime.IsZero() {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render("Last Updated"),
					node.NPUMetricsTime.Local().Format("15:04:05")))
			}

			// Check if we have extended metrics
			hasExtendedMetrics := false
			for _, chip := range node.NPUChips {
				if chip.VectorUtil > 0 || chip.Voltage > 0 || chip.AICoreFreq > 0 {
					hasExtendedMetrics = true
					break
				}
			}

			// Combined NPU Chip Details table
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  NPU Chip Details"))

			if hasExtendedMetrics {
				// Column widths for extended metrics table
				const (
					colID     = 3
					colAICore = 7
					colVector = 7
					colHBM    = 14
					colTemp   = 6
					colPower  = 6
					colVolt   = 6
					colFreq   = 7
					colLink   = 4
				)

				// Extended header with all metrics - HBM next to AICore
				info = append(info, fmt.Sprintf("    %s %s %s %s %s %s %s %s %s",
					padRight("ID", colID),
					padRight("AICore", colAICore),
					padRight("Vector", colVector),
					padRight("HBM", colHBM),
					padRight("Temp", colTemp),
					padRight("Power", colPower),
					padRight("Volt", colVolt),
					padRight("Freq", colFreq),
					padRight("Link", colLink)))
				info = append(info, "    "+strings.Repeat("─", colID+colAICore+colVector+colHBM+colTemp+colPower+colVolt+colFreq+colLink+8))

				for _, chip := range node.NPUChips {
					// Temperature with color coding
					tempStr := fmt.Sprintf("%d°C", chip.Temp)
					if chip.Temp >= 80 {
						tempStr = StyleStatusNotReady.Render(tempStr)
					} else if chip.Temp >= 70 {
						tempStr = StyleWarning.Render(tempStr)
					}

					// Link status
					linkStr := "Down"
					if chip.LinkStatus == 1 {
						linkStr = StyleStatusReady.Render("Up")
					} else {
						linkStr = StyleStatusNotReady.Render("Down")
					}

					// HBM usage with capacity (convert MB to GB for readability)
					hbmStr := "-"
					if chip.HBMTotal > 0 {
						usedGB := float64(chip.HBMUsed) / 1024
						totalGB := float64(chip.HBMTotal) / 1024
						hbmPercent := float64(chip.HBMUsed) / float64(chip.HBMTotal) * 100
						hbmStr = fmt.Sprintf("%.1f/%.0fG(%d%%)", usedGB, totalGB, int(hbmPercent))
					}

					// Health indicator in ID column
					idStr := fmt.Sprintf("%d", chip.PhyID)
					if chip.Health != "OK" && chip.Health != "" {
						idStr = StyleWarning.Render(idStr)
					}

					info = append(info, fmt.Sprintf("    %s %s %s %s %s %s %s %s %s",
						padRight(idStr, colID),
						padRight(fmt.Sprintf("%d%%", chip.AICore), colAICore),
						padRight(fmt.Sprintf("%.1f%%", chip.VectorUtil), colVector),
						padRight(hbmStr, colHBM),
						padRight(tempStr, colTemp),
						padRight(fmt.Sprintf("%dW", int(chip.Power)), colPower),
						padRight(fmt.Sprintf("%.2fV", chip.Voltage), colVolt),
						padRight(fmt.Sprintf("%dMHz", chip.AICoreFreq), colFreq),
						padRight(linkStr, colLink)))
				}
			} else {
				// Basic header without extended metrics
				const (
					bColID     = 4
					bColAICore = 8
					bColTemp   = 8
					bColPower  = 8
					bColHealth = 8
					bColHBM    = 15
				)
				info = append(info, fmt.Sprintf("    %s %s %s %s %s %s",
					padRight("ID", bColID),
					padRight("AICore", bColAICore),
					padRight("Temp", bColTemp),
					padRight("Power", bColPower),
					padRight("Health", bColHealth),
					padRight("HBM", bColHBM)))
				info = append(info, "    "+strings.Repeat("─", bColID+bColAICore+bColTemp+bColPower+bColHealth+bColHBM+5))

				for _, chip := range node.NPUChips {
					// Format HBM usage
					hbmStr := fmt.Sprintf("%d/%d MB", chip.HBMUsed, chip.HBMTotal)
					if chip.HBMTotal > 0 {
						hbmPercent := float64(chip.HBMUsed) / float64(chip.HBMTotal) * 100
						hbmStr = fmt.Sprintf("%d/%dMB (%.0f%%)", chip.HBMUsed, chip.HBMTotal, hbmPercent)
					}

					// Color coding for health
					healthStr := chip.Health
					switch chip.Health {
					case "OK":
						healthStr = StyleStatusReady.Render(chip.Health)
					case "Warning":
						healthStr = StyleWarning.Render(chip.Health)
					default:
						healthStr = StyleStatusNotReady.Render(chip.Health)
					}

					// Color coding for temperature
					tempStr := fmt.Sprintf("%d°C", chip.Temp)
					if chip.Temp >= 80 {
						tempStr = StyleStatusNotReady.Render(tempStr)
					} else if chip.Temp >= 70 {
						tempStr = StyleWarning.Render(tempStr)
					}

					info = append(info, fmt.Sprintf("    %s %s %s %s %s %s",
						padRight(fmt.Sprintf("%d", chip.PhyID), bColID),
						padRight(fmt.Sprintf("%d%%", chip.AICore), bColAICore),
						padRight(tempStr, bColTemp),
						padRight(fmt.Sprintf("%.1fW", chip.Power), bColPower),
						padRight(healthStr, bColHealth),
						hbmStr))
				}
			}

			// RoCE and ECC Statistics (only if extended metrics available)
			if hasExtendedMetrics {
				// RoCE Network Statistics
				var totalRoCETx, totalRoCERx, totalRoCETxErr, totalRoCERxErr int64
				for _, chip := range node.NPUChips {
					totalRoCETx += chip.RoCETxPkts
					totalRoCERx += chip.RoCERxPkts
					totalRoCETxErr += chip.RoCETxErrPkts
					totalRoCERxErr += chip.RoCERxErrPkts
				}

				if totalRoCETx > 0 || totalRoCERx > 0 {
					info = append(info, "")
					info = append(info, StyleTextSecondary.Render("  RoCE Network Statistics"))
					info = append(info, fmt.Sprintf("    %s: %s  %s: %s",
						StyleTextMuted.Render("TX Packets"),
						formatPacketCount(totalRoCETx),
						StyleTextMuted.Render("RX Packets"),
						formatPacketCount(totalRoCERx)))

					// Show error packets with warning color if non-zero
					txErrStyle := StyleTextMuted
					rxErrStyle := StyleTextMuted
					if totalRoCETxErr > 0 {
						txErrStyle = StyleWarning
					}
					if totalRoCERxErr > 0 {
						rxErrStyle = StyleWarning
					}
					info = append(info, fmt.Sprintf("    %s: %s  %s: %s",
						StyleTextMuted.Render("TX Errors"),
						txErrStyle.Render(formatPacketCount(totalRoCETxErr)),
						StyleTextMuted.Render("RX Errors"),
						rxErrStyle.Render(formatPacketCount(totalRoCERxErr))))
				}

				// ECC Error Statistics
				var totalEccSingle, totalEccDouble int64
				for _, chip := range node.NPUChips {
					totalEccSingle += chip.HBMEccSingleErr
					totalEccDouble += chip.HBMEccDoubleErr
				}

				if totalEccSingle > 0 || totalEccDouble > 0 {
					info = append(info, "")
					info = append(info, StyleTextSecondary.Render("  HBM ECC Error Statistics"))

					singleStyle := StyleTextMuted
					doubleStyle := StyleTextMuted
					if totalEccSingle > 0 {
						singleStyle = StyleWarning
					}
					if totalEccDouble > 0 {
						doubleStyle = StyleStatusNotReady // Critical - double-bit errors are uncorrectable
					}

					info = append(info, fmt.Sprintf("    %s: %s (correctable)  %s: %s (uncorrectable)",
						StyleTextMuted.Render("Single-bit"),
						singleStyle.Render(fmt.Sprintf("%d", totalEccSingle)),
						StyleTextMuted.Render("Double-bit"),
						doubleStyle.Render(fmt.Sprintf("%d", totalEccDouble))))
				}
			}
		} else if node.NPUCapacity > 0 {
			// NPU exists but no metrics from NPU-Exporter - show hint
			info = append(info, "")
			info = append(info, StyleTextMuted.Render("  NPU Runtime Metrics: Not available"))
			info = append(info, StyleTextMuted.Render("    NPU-Exporter not reachable or not deployed in kube-system namespace"))
			info = append(info, StyleTextMuted.Render("    Use --npu-exporter flag to specify a custom endpoint"))
		}

		// Chip health bitmask (per-chip, reported by the NPU device plugin)
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.chip_health"))+
			StyleTextMuted.Render("  ("+m.T("npudetail.chip_health_src")+")"))
		if !node.NPUStatusKnown {
			info = append(info, StyleTextMuted.Render("    "+m.T("npudetail.mask_unknown")))
		} else {
			info = append(info, "    "+m.renderNPUChipHealth(node))

			// The legend only counts the chips the bitmask covers; chips the
			// device plugin did not report are called out separately instead of
			// being counted as healthy.
			rendered, covered := npuChipHealthCells(node)
			unreported := rendered - covered
			healthy, faulty := countChipHealth(node)
			if faulty > 0 || unreported > 0 {
				legend := fmt.Sprintf("      %s: %d   %s: %d",
					StyleStatusReady.Render("✓ "+m.T("npudetail.chip_ok")),
					healthy,
					StyleStatusNotReady.Render("✗ "+m.T("npudetail.chip_fault")),
					faulty)
				if unreported > 0 {
					legend += "   " + StyleTextMuted.Render(m.TF("npudetail.chip_unreported", map[string]interface{}{
						"Count": unreported,
					}))
				}
				info = append(info, legend)
			}
		}

		// HCCS affinity groups (volcano.sh/device-topologys node annotation)
		if groups := formatSwitchGroups(node.NPUTopology); groups != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npudetail.hccs_affinity")),
				groups))
		}

		// Management-plane faults (device / switch / custom)
		if fault := node.NPUFault; fault != nil &&
			fault.DeviceFaultLevel != "" && fault.DeviceFaultLevel != "NormalNPU" {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.mgmt_fault")))
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npudetail.device_fault_level")),
				StyleError.Render(fault.DeviceFaultLevel)))

			for _, event := range fault.DeviceFaultEvents {
				info = append(info, "      "+StyleTextMuted.Render("•")+" "+
					truncate(collapseWhitespace(event), 90))
			}

			if fault.SwitchFaultName != "" {
				level := fault.SwitchFaultLevel
				if level == "" {
					level = "-"
				}
				info = append(info, fmt.Sprintf("    %s: %s (%s)",
					StyleTextMuted.Render(m.T("npudetail.switch_fault")),
					truncate(collapseWhitespace(fault.SwitchFaultName), 60),
					StyleWarning.Render(level)))
			}

			if fault.CustomFaultLevel != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render(m.T("npudetail.custom_fault_level")),
					StyleWarning.Render(fault.CustomFaultLevel)))
			}
		}

		// NPU inspection checks (NT_* node conditions written by the inspection job)
		if checks := npuInspectionConditions(node); len(checks) > 0 {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.inspection")))

			const (
				colCheckName = 30
				colCheckStat = 8
				colCheckBeat = 12
				// A message column narrower than this is unreadable, so on
				// terminals too narrow to fit it the MESSAGE column is dropped
				// entirely (leaving CHECK / STATUS / HEARTBEAT) instead of
				// overflowing the terminal edge.
				minMsgWidth = 20
				maxMsgWidth = 80
				// Width of the table without the message column: indent (4), the
				// three columns and one separator after each of them.
				checkTableWidth = 4 + colCheckName + 1 + colCheckStat + 1 + colCheckBeat
			)

			// 8 = indent plus column separators, kept as-is so the message
			// column is exactly as wide as before on terminals that fit it.
			msgWidth := m.width - colCheckName - colCheckStat - colCheckBeat - 8
			if msgWidth > maxMsgWidth {
				msgWidth = maxMsgWidth
			}

			showMsg := m.width >= checkTableWidth+1+minMsgWidth
			if !showMsg {
				msgWidth = 0
			} else if msgWidth < minMsgWidth {
				msgWidth = minMsgWidth
			}

			// inspectionRow renders one table row. On narrow terminals the
			// message cell is omitted, so the row ends after the heartbeat.
			inspectionRow := func(name, status, heartbeat, message string) string {
				cells := []string{padRight(name, colCheckName), padRight(status, colCheckStat)}
				if showMsg {
					cells = append(cells, padRight(heartbeat, colCheckBeat), message)
				} else {
					cells = append(cells, heartbeat)
				}
				return "    " + strings.Join(cells, " ")
			}

			info = append(info, StyleTextSecondary.Render(inspectionRow(
				m.T("npudetail.col_check"),
				m.T("npudetail.col_status"),
				m.T("npudetail.col_heartbeat"),
				m.T("npudetail.col_message"))))

			for _, check := range checks {
				info = append(info, inspectionRow(
					truncate(strings.TrimPrefix(string(check.Type), "NT_"), colCheckName-1),
					renderInspectionStatus(check.Status),
					inspectionHeartbeat(check),
					truncate(collapseWhitespace(check.Message), msgWidth)))
			}
		}

		// NPU network mapping (cce.kubectl.kubernetes.io/ascend-rank-table node
		// annotation): which TOR switch and port each chip is attached to.
		if len(node.NPUDeviceNet) > 0 {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.net_section")))

			const (
				colNetID    = 4
				colNetDevIP = 16
				colNetTOR   = 16
				colNetPort  = 9
			)

			info = append(info, StyleTextSecondary.Render(fmt.Sprintf("    %s %s %s %s",
				padRight("ID", colNetID),
				padRight(m.T("npudetail.col_dev_ip"), colNetDevIP),
				padRight(m.T("npudetail.col_tor"), colNetTOR),
				m.T("npudetail.col_tor_port"))))
			info = append(info, "    "+strings.Repeat("─", colNetID+colNetDevIP+colNetTOR+colNetPort+3))

			for _, entry := range node.NPUDeviceNet {
				info = append(info, fmt.Sprintf("    %s %s %s %s",
					padRight(fmt.Sprintf("%d", entry.DeviceID), colNetID),
					padRight(truncate(entry.DeviceIP, colNetDevIP-1), colNetDevIP),
					padRight(truncate(entry.TORIP, colNetTOR-1), colNetTOR),
					truncate(entry.TORPort, colNetPort)))
			}
		}
	}

	// Network Traffic (if available)
	if node.NetworkRxBytes > 0 || node.NetworkTxBytes > 0 {
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("detail.node.network_traffic")))

		rxStr := formatNetworkTraffic(node.NetworkRxBytes)
		txStr := formatNetworkTraffic(node.NetworkTxBytes)
		totalStr := formatNetworkTraffic(node.NetworkRxBytes + node.NetworkTxBytes)

		info = append(info, fmt.Sprintf("  %s: %s  %s: %s  Total: %s",
			StyleTextMuted.Render(m.T("detail.node.rx")),
			rxStr,
			StyleTextMuted.Render(m.T("detail.node.tx")),
			txStr,
			totalStr,
		))
	}

	// Historical trends (if available)
	if len(m.metricHistory) >= 2 {
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("detail.node.historical_trends")))

		cpuHistory := m.getNodeCPUHistory(node.Name)
		if len(cpuHistory) >= 2 {
			sparkline := RenderSparkline(cpuHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.cpu_label")),
				sparkline))
		}

		memHistory := m.getNodeMemoryHistory(node.Name)
		if len(memHistory) >= 2 {
			sparkline := RenderSparkline(memHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.memory_label")),
				sparkline))
		}

		// Network traffic trends
		rxHistory := m.getNodeNetworkRxHistory(node.Name)
		if len(rxHistory) >= 2 {
			sparkline := RenderSparkline(rxHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.network_rx_label")),
				sparkline))
		}

		txHistory := m.getNodeNetworkTxHistory(node.Name)
		if len(txHistory) >= 2 {
			sparkline := RenderSparkline(txHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.node.network_tx_label")),
				sparkline))
		}

		// NPU utilization trends (if node has NPU)
		if node.NPUCapacity > 0 {
			npuHistory := m.getNodeNPUUtilizationHistory(node.Name)
			if len(npuHistory) >= 2 {
				sparkline := RenderSparkline(npuHistory, 40)
				info = append(info, fmt.Sprintf("  %s %s",
					StyleTextMuted.Render(m.T("detail.node.npu_label")),
					sparkline))
			}
		}

		info = append(info, StyleTextMuted.Render("  "+m.TF("detail.node.snapshots", map[string]interface{}{
			"Count": len(m.metricHistory),
		})))
	}

	return strings.Join(info, "\n")
}

// renderNodePodsInfo renders pods running on this node
func (m *Model) renderNodePodsInfo(node *model.NodeData) string {
	var info []string

	info = append(info, StyleHeader.Render(fmt.Sprintf("📦 Pods on this Node (%d)", node.PodCount)))
	info = append(info, "")

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleTextMuted.Render("  No pod information available"))
		return strings.Join(info, "\n")
	}

	// Filter pods running on this node
	nodePods := []*model.PodData{}
	for _, pod := range m.clusterData.Pods {
		if pod.Node == node.Name {
			nodePods = append(nodePods, pod)
		}
	}

	if len(nodePods) == 0 {
		info = append(info, StyleTextMuted.Render("  No pods running on this node"))
		return strings.Join(info, "\n")
	}

	// Table header
	headerRow := fmt.Sprintf("  %-35s %-20s %-15s %-10s",
		"NAME", "NAMESPACE", "STATUS", "RESTARTS")
	info = append(info, StyleTextSecondary.Render(headerRow))
	info = append(info, "  "+strings.Repeat("─", 85))

	// Pod rows (limited to visible area)
	maxVisible := m.height - 20
	if maxVisible < 1 {
		maxVisible = 1
	}

	visiblePods := nodePods
	if len(nodePods) > maxVisible {
		visiblePods = nodePods[:maxVisible]
	}

	for _, pod := range visiblePods {
		name := pod.Name
		if len(name) > 33 {
			name = name[:30] + "..."
		}

		namespace := pod.Namespace
		if len(namespace) > 18 {
			namespace = namespace[:15] + "..."
		}

		status := RenderStatus(pod.Phase)
		restarts := fmt.Sprintf("%d", pod.RestartCount)

		row := fmt.Sprintf("  %-35s %-20s %-23s %-10s",
			name,
			namespace,
			status,
			restarts)
		info = append(info, row)
	}

	if len(nodePods) > maxVisible {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render(fmt.Sprintf("  ... and %d more pods", len(nodePods)-maxVisible)))
	}

	return strings.Join(info, "\n")
}

// formatPacketCount formats a large packet count with K/M/G suffix
func formatPacketCount(count int64) string {
	if count >= 1_000_000_000 {
		return fmt.Sprintf("%.2fG", float64(count)/1_000_000_000)
	} else if count >= 1_000_000 {
		return fmt.Sprintf("%.2fM", float64(count)/1_000_000)
	} else if count >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(count)/1_000)
	}
	return fmt.Sprintf("%d", count)
}

// maxChipHealthCells bounds the number of rendered chip cells, so a node that
// reports an implausible NPU capacity cannot produce an unbounded line.
const maxChipHealthCells = 64

// npuChipHealthCells returns how many chip cells the bitmap block draws and how
// many of them the device-plugin bitmask actually covers. The two differ when
// the node reports more chips than the bitmask has bits: the extra cells are
// drawn as "?" and say nothing about the chip, so the legend must not count
// them either way.
func npuChipHealthCells(node *model.NodeData) (rendered, covered int) {
	rendered = int(node.NPUCapacity)
	if rendered > maxChipHealthCells {
		rendered = maxChipHealthCells
	}
	covered = rendered
	if covered > model.NPUStatusChipBits {
		covered = model.NPUStatusChipBits
	}
	return rendered, covered
}

// renderNPUChipHealth renders the per-chip health cells of the NPU status
// bitmask, e.g. "[0]✓ [1]✗ [2]✓". One cell per chip in NPUCapacity; chips past
// the last bit the device plugin reports are marked "?" rather than being
// assumed healthy.
func (m *Model) renderNPUChipHealth(node *model.NodeData) string {
	chips, _ := npuChipHealthCells(node)

	cells := make([]string, 0, chips)
	for i := 0; i < chips; i++ {
		switch {
		case i >= model.NPUStatusChipBits:
			cells = append(cells, StyleTextMuted.Render(fmt.Sprintf("[%d]?", i)))
		case node.NPUChipFaulty(i):
			cells = append(cells, StyleStatusNotReady.Render(fmt.Sprintf("[%d]✗", i)))
		default:
			cells = append(cells, StyleStatusReady.Render(fmt.Sprintf("[%d]✓", i)))
		}
	}
	return strings.Join(cells, " ")
}

// countChipHealth counts healthy and faulty chips among the chips the NPU status
// bitmask actually covers.
func countChipHealth(node *model.NodeData) (healthy, faulty int) {
	_, covered := npuChipHealthCells(node)

	for i := 0; i < covered; i++ {
		if node.NPUChipFaulty(i) {
			faulty++
		} else {
			healthy++
		}
	}
	return healthy, faulty
}

// compressChipRanges renders a set of chip ids as sorted ranges, e.g.
// [0,1,2,3,4,5,6,7] -> "0-7" and [0,1,5] -> "0-1,5". Duplicates are dropped.
func compressChipRanges(chips []int) string {
	if len(chips) == 0 {
		return ""
	}

	sorted := append([]int(nil), chips...)
	sort.Ints(sorted)

	parts := make([]string, 0, len(sorted))
	start, prev := sorted[0], sorted[0]
	flush := func() {
		if start == prev {
			parts = append(parts, fmt.Sprintf("%d", start))
			return
		}
		parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
	}

	for _, chip := range sorted[1:] {
		switch {
		case chip == prev:
			continue // duplicate id
		case chip == prev+1:
			prev = chip
		default:
			flush()
			start, prev = chip, chip
		}
	}
	flush()

	return strings.Join(parts, ",")
}

// formatSwitchGroups renders the switch topology groups of a node, e.g.
// "HCCS [0-7]" or "[0-7] SIO [8-15]" for a mixed topology. Groups of type HCCS
// (the default affinity fabric) are rendered bare; any other type is prefixed
// with its own name. Groups without chips, and groups whose chips are all
// unrenderable, are skipped; "" means there is nothing to show.
func formatSwitchGroups(groups []model.NPUSwitchGroup) string {
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		rendered := make([]string, 0, len(group.Groups))
		for _, chips := range group.Groups {
			if ranges := compressChipRanges(chips); ranges != "" {
				rendered = append(rendered, "["+ranges+"]")
			}
		}
		if len(rendered) == 0 {
			continue
		}

		entry := strings.Join(rendered, " ")
		if group.Type != "" && !strings.EqualFold(group.Type, "HCCS") {
			entry = group.Type + " " + entry
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, "  ")
}

// npuInspectionConditions returns the NPU inspection conditions with faulted
// checks first, the rest ordered by condition type. It filters on
// model.NPUCheckConditionPrefix: a live healthy Ascend node carries many more
// NT_* conditions (yum, ntp, sfs, data-pipeline) that are not NPU fault
// signals, so "any NT_*" would report healthy nodes as faulted.
func npuInspectionConditions(node *model.NodeData) []corev1.NodeCondition {
	checks := make([]corev1.NodeCondition, 0, len(node.Conditions))
	for _, cond := range node.Conditions {
		if strings.HasPrefix(string(cond.Type), model.NPUCheckConditionPrefix) {
			checks = append(checks, cond)
		}
	}

	sort.SliceStable(checks, func(i, j int) bool {
		iFailed := checks[i].Status == corev1.ConditionTrue
		jFailed := checks[j].Status == corev1.ConditionTrue
		if iFailed != jFailed {
			return iFailed
		}
		return checks[i].Type < checks[j].Type
	})
	return checks
}

// renderInspectionStatus renders an NT_ condition status: False means the check
// passed, True means a fault was detected, Unknown means it did not apply.
func renderInspectionStatus(status corev1.ConditionStatus) string {
	switch status {
	case corev1.ConditionTrue:
		return StyleStatusNotReady.Render("✗")
	case corev1.ConditionUnknown:
		return StyleTextMuted.Render("–")
	default:
		return StyleStatusReady.Render("✓")
	}
}

// inspectionHeartbeat renders how long ago the check last reported, falling back
// to the transition time when no heartbeat was recorded. "-" when both are unset.
func inspectionHeartbeat(check corev1.NodeCondition) string {
	ts := check.LastHeartbeatTime.Time
	if ts.IsZero() {
		ts = check.LastTransitionTime.Time
	}
	if ts.IsZero() {
		return "-"
	}

	age := time.Since(ts)
	if age < 0 {
		age = 0
	}
	return formatAge(age)
}

// collapseWhitespace flattens a multi-line message onto a single line.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
