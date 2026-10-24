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
			StyleTextSecondary.Render(m.T("detail.field.roles")),
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
			StyleTextSecondary.Render(m.T("detail.field.npu")),
			node.NPUAllocated,
			node.NPUAllocatable,
			npuPercent))

		// NPU details
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.details")))
		if node.NPUChipType != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npu.chip_type")),
				node.NPUChipType))
		}
		if node.NPUDeviceType != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npudetail.device_type")),
				node.NPUDeviceType))
		}
		if node.NPUDriverVersion != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npudetail.driver_version")),
				node.NPUDriverVersion))
		}
		if node.NPUResourceName != "" {
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npu.resource_name")),
				node.NPUResourceName))
		}

		// Topology information
		if node.HyperNodeID != "" || node.SuperPodID != "" {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.topology")))
			if node.SuperPodID != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render(m.T("topology.superpod_id")),
					node.SuperPodID))
			}
			if node.HyperNodeID != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render(m.T("npudetail.hypernode_id")),
					truncate(node.HyperNodeID, m.detailValueWidth(m.T("npudetail.hypernode_id"), 36))))
			}
			if node.CabinetInfo != "" {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render(m.T("npudetail.cabinet")),
					node.CabinetInfo))
			}
		}

		// NPU Runtime Metrics (from k8s-monitor collector)
		if len(node.NPUChips) > 0 {
			info = append(info, "")
			info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.runtime")))

			// Overall status
			healthStyle := StyleStatusReady
			switch node.NPUHealthStatus {
			case "Warning":
				healthStyle = StyleWarning
			case "Unhealthy":
				// The worst aggregate the collector produces (no chip reports
				// OK): it stays red, distinct from the yellow of a merely
				// degraded node.
				healthStyle = StyleStatusNotReady
			case "Unknown":
				// Unknown is the aggregate of chips whose health_status was
				// missing or -1 (DCMI failure): not a fault, so it must not be
				// painted as one.
				healthStyle = StyleTextMuted
			}
			// The runtime metrics (including this health status) come from
			// npu-exporter, unlike the per-chip bitmap block below which comes
			// from the device plugin: label the source so the two are not read
			// as one signal.
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(fmt.Sprintf("%s (%s)", m.T("npudetail.health_status"), m.T("npudetail.health_exporter"))),
				healthStyle.Render(node.NPUHealthStatus)))

			// Chips the exporter said nothing about. They keep a healthy grade
			// in the matrix (an absent signal is not a fault), so this line is
			// where their existence stays visible.
			if unknown := countNPUChipsByHealth(node.NPUChips, "Unknown"); unknown > 0 {
				info = append(info, fmt.Sprintf("    %s: %d",
					StyleTextMuted.Render(m.T("npudetail.health_unknown")),
					unknown))
			}

			// NPU-Exporter liveness: whether the fetch path behind the metrics
			// above is currently answering. The state comes from the cluster
			// summary (one exporter feeds the whole cluster), so it is rendered
			// only when the data carries it.
			if line := m.npuExporterLivenessLine(); line != "" {
				info = append(info, line)
			}

			// Average utilization
			info = append(info, fmt.Sprintf("    %s: %s",
				StyleTextMuted.Render(m.T("npudetail.ai_core_util")),
				StyleHighlight.Render(fmt.Sprintf("%.1f%%", node.NPUUtilization))))

			// HBM Memory
			if node.NPUMemoryTotal > 0 {
				memUsedGB := float64(node.NPUMemoryUsed) / (1024 * 1024 * 1024)
				memTotalGB := float64(node.NPUMemoryTotal) / (1024 * 1024 * 1024)
				info = append(info, fmt.Sprintf("    %s: %.1f / %.1f GiB (%.1f%%)",
					StyleTextMuted.Render(m.T("npudetail.hbm_memory")),
					memUsedGB, memTotalGB, node.NPUMemoryUtil))
			}

			// Temperature and Power
			info = append(info, fmt.Sprintf("    %s: %d W   %s: %d °C",
				StyleTextMuted.Render(m.T("npudetail.total_power")),
				node.NPUPower,
				StyleTextMuted.Render(m.T("npudetail.avg_temp")),
				node.NPUTemperature))

			// Metrics timestamp
			if !node.NPUMetricsTime.IsZero() {
				info = append(info, fmt.Sprintf("    %s: %s",
					StyleTextMuted.Render(m.T("npudetail.last_updated")),
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
			info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.chip_details")))

			if hasExtendedMetrics {
				// Column widths for the extended metrics table. HBM adapts to
				// the data: 14 covers the widest cell the format emits at the
				// real 64 GiB per-chip geometry ("58.9/64G(92%)"); larger HBM
				// capacities (a 128 GB seed reaches 16) grow the column so the
				// widest cell is never clipped.
				const (
					colID     = 3
					colAICore = 7
					colVector = 7
					colTemp   = 6
					colPower  = 6
					colVolt   = 6
					colFreq   = 7
					colLink   = 4
				)

				// Pre-scan the HBM cells once: the column width and the row
				// cells share these strings, so the width can never drift
				// from the data.
				hbmCells := make([]string, len(node.NPUChips))
				colHBM := 14
				for i, chip := range node.NPUChips {
					hbmCells[i] = extendedHBMCell(chip)
					if w := visualLength(hbmCells[i]); w > colHBM {
						colHBM = w
					}
				}

				// Terminal-width tiers, derived from the column widths so a
				// wider HBM column widens the tiers with the table. The row
				// width is 4 (indent) + surviving columns + one separator per
				// gap between them:
				//   w >= full tier : 9 columns, row 4 + Σ(9) + 8
				//   w >= mid tier  : 7 columns (Freq, Link dropped),
				//                    row 4 + Σ(7) + 6
				//   w < mid tier   : 6 columns (Volt dropped as well),
				//                    row 4 + Σ(6) + 5
				// The mid-tier threshold counts the full tier's nine gaps
				// rather than the surviving seven's six: the historical
				// boundary sat three cells above the seven-column row (59 at
				// colHBM 14) and the width-61 golden pins the tier switch at
				// 62, so the derivation reproduces that boundary exactly
				// (4 + 49 + 9 = 62 at colHBM 14).
				extFullWidth := 4 + colID + colAICore + colVector + colHBM + colTemp + colPower + colVolt + colFreq + colLink + 8
				extMidWidth := 4 + colID + colAICore + colVector + colHBM + colTemp + colPower + colVolt + 9

				extCols := []struct {
					title string
					width int
				}{
					{m.T("npudetail.col_id"), colID},
					{m.T("npudetail.col_ai_core"), colAICore},
					{m.T("npudetail.col_vector"), colVector},
					{m.T("npudetail.col_hbm"), colHBM},
					{m.T("npudetail.col_temp"), colTemp},
					{m.T("npudetail.col_power"), colPower},
					{m.T("npudetail.col_volt"), colVolt},
					{m.T("npudetail.col_freq"), colFreq},
					{m.T("npudetail.col_link"), colLink},
				}
				switch {
				case m.width >= extFullWidth:
					// all nine columns
				case m.width >= extMidWidth:
					extCols = extCols[:7]
				default:
					extCols = extCols[:6]
				}
				// Freq and Link are the columns dropped first; compute their
				// cells only while their column survives.
				showFreq := len(extCols) >= 8
				showLink := len(extCols) >= 9

				extWidth := 0
				for _, c := range extCols {
					extWidth += c.width
				}

				// extRow renders one table row. Header and data rows go through
				// the same cell/width list, so the column set can never drift
				// between them.
				extRow := func(cells []string) string {
					parts := make([]string, len(extCols))
					for i, c := range extCols {
						parts[i] = padRight(cells[i], c.width)
					}
					return "    " + strings.Join(parts, " ")
				}

				titles := make([]string, len(extCols))
				for i, c := range extCols {
					titles[i] = c.title
				}
				info = append(info, extRow(titles))
				info = append(info, "    "+strings.Repeat("─", extWidth+len(extCols)-1))

				for i, chip := range node.NPUChips {
					// Temperature with color coding
					tempStr := fmt.Sprintf("%d°C", chip.Temp)
					if chip.Temp >= 80 {
						tempStr = StyleStatusNotReady.Render(tempStr)
					} else if chip.Temp >= 70 {
						tempStr = StyleWarning.Render(tempStr)
					}

					// HBM usage with capacity, formatted by the same helper
					// the column width was measured from.
					hbmStr := hbmCells[i]

					// Health indicator in ID column. Only a chip the exporter
					// reported unhealthy is called out; "Unknown" (health_status
					// missing or -1) is not a fault and keeps the plain cell.
					idStr := fmt.Sprintf("%d", chip.PhyID)
					switch chip.Health {
					case "OK", "":
					case "Unknown":
						idStr = StyleTextMuted.Render(idStr)
					default:
						idStr = StyleWarning.Render(idStr)
					}

					cells := []string{
						idStr,
						fmt.Sprintf("%d%%", chip.AICore),
						fmt.Sprintf("%.1f%%", chip.VectorUtil),
						hbmStr,
						tempStr,
						fmt.Sprintf("%dW", int(chip.Power)),
						fmt.Sprintf("%.2fV", chip.Voltage),
					}
					if showFreq {
						cells = append(cells, fmt.Sprintf("%dMHz", chip.AICoreFreq))
					}
					if showLink {
						// Link status
						linkStr := m.T("npudetail.link_down")
						if chip.LinkStatus == 1 {
							linkStr = StyleStatusReady.Render(m.T("npudetail.link_up"))
						} else {
							linkStr = StyleStatusNotReady.Render(m.T("npudetail.link_down"))
						}
						cells = append(cells, linkStr)
					}

					info = append(info, extRow(cells))
				}
			} else {
				// Basic header without extended metrics
				const (
					bColID     = 4
					bColAICore = 8
					bColTemp   = 8
					bColPower  = 8
					bColHealth = 8
					// 20 cells is the widest HBM value at the real Ascend 910B1
					// geometry ("65536/65536MB (100%)", 64 GiB per chip), so the
					// column fits its data instead of overflowing it.
					bColHBM = 20
				)
				info = append(info, fmt.Sprintf("    %s %s %s %s %s %s",
					padRight(m.T("npudetail.col_id"), bColID),
					padRight(m.T("npudetail.col_ai_core"), bColAICore),
					padRight(m.T("npudetail.col_temp"), bColTemp),
					padRight(m.T("npudetail.col_power"), bColPower),
					padRight(m.T("npudetail.col_health"), bColHealth),
					padRight(m.T("npudetail.col_hbm"), bColHBM)))
				info = append(info, "    "+strings.Repeat("─", bColID+bColAICore+bColTemp+bColPower+bColHealth+bColHBM+5))

				for _, chip := range node.NPUChips {
					// Format HBM usage
					hbmStr := fmt.Sprintf("%d/%d MB", chip.HBMUsed, chip.HBMTotal)
					if chip.HBMTotal > 0 {
						hbmPercent := float64(chip.HBMUsed) / float64(chip.HBMTotal) * 100
						hbmStr = fmt.Sprintf("%d/%dMB (%.0f%%)", chip.HBMUsed, chip.HBMTotal, hbmPercent)
					}

					// Color coding for health. Two producers feed the field:
					// NPU-Exporter yields OK / Error / Unknown, the deploy
					// collector copies the npu-smi string (adds Warning). Only
					// Error is a fault; Warning renders yellow, and Unknown
					// ("health_status" missing or -1) is muted rather than
					// painted as a failure.
					healthStr := chip.Health
					switch chip.Health {
					case "OK":
						healthStr = StyleStatusReady.Render(chip.Health)
					case "Warning":
						healthStr = StyleWarning.Render(chip.Health)
					case "Error":
						healthStr = StyleStatusNotReady.Render(chip.Health)
					case "Unknown":
						healthStr = StyleTextMuted.Render(chip.Health)
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
						padRight(hbmStr, bColHBM)))
				}
			}

			// Sparse per-chip rows: only the chips that carry something the
			// table columns cannot show. They are appended after the table
			// rather than as extra columns, because the table's column widths
			// (including the 61-column tier boundary) are byte-anchored, and a
			// blank line keeps the table block itself unchanged for the readers
			// that count its lines.
			if sparse := m.npuChipSparseLines(node); len(sparse) > 0 {
				info = append(info, "")
				info = append(info, sparse...)
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
					info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.roce_section")))
					info = append(info, fmt.Sprintf("    %s: %s  %s: %s",
						StyleTextMuted.Render(m.T("npudetail.tx_packets")),
						formatPacketCount(totalRoCETx),
						StyleTextMuted.Render(m.T("npudetail.rx_packets")),
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
						StyleTextMuted.Render(m.T("npudetail.tx_errors")),
						txErrStyle.Render(formatPacketCount(totalRoCETxErr)),
						StyleTextMuted.Render(m.T("npudetail.rx_errors")),
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
					info = append(info, StyleTextSecondary.Render("  "+m.T("npudetail.ecc_section")))

					singleStyle := StyleTextMuted
					doubleStyle := StyleTextMuted
					if totalEccSingle > 0 {
						singleStyle = StyleWarning
					}
					if totalEccDouble > 0 {
						doubleStyle = StyleStatusNotReady // Critical - double-bit errors are uncorrectable
					}

					// The full summary line is 62 cells wide; the two parenthesised
					// qualifiers push it past narrow terminals, so they are
					// dropped below 64 columns while the counts stay visible.
					if m.width < 64 {
						info = append(info, fmt.Sprintf("    %s: %s  %s: %s",
							StyleTextMuted.Render(m.T("npudetail.ecc_single")),
							singleStyle.Render(fmt.Sprintf("%d", totalEccSingle)),
							StyleTextMuted.Render(m.T("npudetail.ecc_double")),
							doubleStyle.Render(fmt.Sprintf("%d", totalEccDouble))))
					} else {
						info = append(info, fmt.Sprintf("    %s: %s %s  %s: %s %s",
							StyleTextMuted.Render(m.T("npudetail.ecc_single")),
							singleStyle.Render(fmt.Sprintf("%d", totalEccSingle)),
							m.T("npudetail.ecc_correctable"),
							StyleTextMuted.Render(m.T("npudetail.ecc_double")),
							doubleStyle.Render(fmt.Sprintf("%d", totalEccDouble)),
							m.T("npudetail.ecc_uncorrectable")))
					}
				}
			}
		} else if node.NPUCapacity > 0 {
			// NPU exists but no metrics from NPU-Exporter - show hint
			info = append(info, "")
			info = append(info, StyleTextMuted.Render("  "+m.T("npudetail.runtime_na")))
			info = append(info, StyleTextMuted.Render("    "+m.T("npudetail.runtime_na_hint_unreachable")))
			info = append(info, StyleTextMuted.Render("    "+m.T("npudetail.runtime_na_hint_not_deployed")))
			info = append(info, StyleTextMuted.Render("    "+m.T("npudetail.runtime_na_flag")))
			// The hint above names the two usual causes; the liveness row says
			// which state the fetch path is actually in and why it last failed,
			// which is what turns "unreachable" into something actionable.
			if line := m.npuExporterLivenessLine(); line != "" {
				info = append(info, line)
			}
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
				padRight(m.T("npudetail.col_id"), colNetID),
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

		info = append(info, fmt.Sprintf("  %s: %s  %s: %s  %s: %s",
			StyleTextMuted.Render(m.T("detail.node.rx")),
			rxStr,
			StyleTextMuted.Render(m.T("detail.node.tx")),
			txStr,
			m.T("detail.node.total"),
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

	info = append(info, StyleHeader.Render(m.TF("detail.node.pods_title", map[string]interface{}{
		"Count": node.PodCount,
	})))
	info = append(info, "")

	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.node.no_pod_info")))
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
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.node.no_pods")))
		return strings.Join(info, "\n")
	}

	// Table header and rows share one cell list. Every cell goes through
	// padRight rather than fmt's %-Ns: RenderStatus emits ANSI codes, which
	// %-Ns counts as bytes and therefore pads short, shifting every column
	// after STATUS.
	//
	// Terminal-width tiers, row width = indent (2) + surviving columns + one
	// separator per gap between them:
	//   w >= 85 : NAME 35 + NAMESPACE 20 + STATUS 15 + RESTARTS 10 + 3 = 85
	//   w >= 64 : NAMESPACE dropped                = 2 + 35+15+10 + 2 = 64
	//   w >= 49 : NAME shrunk to 20 (truncate)     = 2 + 20+15+10 + 2 = 49
	//   w <  49 : RESTARTS dropped as well         = 2 + 20+15 + 1   = 38
	const (
		colName      = 35
		colNamespace = 20
		colStatus    = 15
		colRestarts  = 10
		shortName    = 20
	)

	nameWidth := colName
	showNamespace, showRestarts := true, true
	switch {
	case m.width >= 85:
		// full four-column table
	case m.width >= 64:
		showNamespace = false
	case m.width >= 49:
		showNamespace = false
		nameWidth = shortName
	default:
		showNamespace = false
		nameWidth = shortName
		showRestarts = false
	}

	type podsCol struct {
		title string
		width int
	}
	podsCols := []podsCol{{m.T("columns.name"), nameWidth}}
	if showNamespace {
		podsCols = append(podsCols, podsCol{m.T("columns.namespace"), colNamespace})
	}
	podsCols = append(podsCols, podsCol{m.T("columns.status"), colStatus})
	if showRestarts {
		podsCols = append(podsCols, podsCol{m.T("columns.restarts"), colRestarts})
	}

	// podsRow renders one table row. Header and data rows go through the same
	// column list, so the column set can never drift between them.
	podsRow := func(cells []string) string {
		parts := make([]string, len(podsCols))
		for i, c := range podsCols {
			parts[i] = padRight(cells[i], c.width)
		}
		return "  " + strings.Join(parts, " ")
	}

	titles := make([]string, len(podsCols))
	for i, c := range podsCols {
		titles[i] = c.title
	}
	info = append(info, StyleTextSecondary.Render(podsRow(titles)))
	podsSep := 0
	for _, c := range podsCols {
		podsSep += c.width
	}
	podsSep += len(podsCols) - 1
	info = append(info, "  "+strings.Repeat("─", podsSep))

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
		// On the narrowed-NAME tiers the byte-based 33-cell cut above is not
		// enough: truncate to the column width (width-aware, "..." suffix).
		if nameWidth < colName {
			name = truncate(pod.Name, nameWidth)
		}

		namespace := pod.Namespace
		if len(namespace) > 18 {
			namespace = namespace[:15] + "..."
		}

		status := RenderStatus(pod.Phase)
		restarts := fmt.Sprintf("%d", pod.RestartCount)

		cells := []string{name}
		if showNamespace {
			cells = append(cells, namespace)
		}
		cells = append(cells, status)
		if showRestarts {
			cells = append(cells, restarts)
		}
		info = append(info, podsRow(cells))
	}

	if len(nodePods) > maxVisible {
		info = append(info, "")
		info = append(info, StyleTextMuted.Render("  "+
			m.TF("detail.node.more_pods", map[string]interface{}{
				"Count": len(nodePods) - maxVisible,
			})))
	}

	return strings.Join(info, "\n")
}

// countNPUChipsByHealth counts the chips whose exported health string equals
// state. It is how the node detail counts the chips the exporter said nothing
// about ("Unknown").
func countNPUChipsByHealth(chips []model.NPUChipData, state string) int {
	count := 0
	for _, chip := range chips {
		if chip.Health == state {
			count++
		}
	}
	return count
}

// npuExporterLivenessLine renders the NPU-Exporter liveness row of the node
// detail's runtime section, or "" when the data carries no liveness state (no
// exporter client, or no NPU node so the exporter is never polled). The state
// is localized through its key, with the raw error text appended for the
// degraded states, truncated to the field width like every other value.
func (m *Model) npuExporterLivenessLine() string {
	if m.clusterData == nil || m.clusterData.Summary == nil || m.clusterData.Summary.NPUExporterLiveness == nil {
		return ""
	}
	liveness := m.clusterData.Summary.NPUExporterLiveness

	label := m.T("npudetail.exporter_liveness")
	state := liveness.Status
	if liveness.MessageKey != "" {
		state = m.TF(liveness.MessageKey, liveness.MessageArgs)
	} else if state == "" {
		// A status nobody set must not read as healthy: fall back to the
		// "not polled yet" wording instead of rendering an empty green value.
		state = m.T("npu.liveness.never")
	}

	style := StyleStatusReady
	switch liveness.Status {
	case "backoff":
		style = StyleWarning
	case "never":
		style = StyleTextMuted
	default:
		if liveness.Status != "ok" {
			style = StyleTextMuted
		}
	}

	value := style.Render(state)
	if liveness.LastError != "" && liveness.Status != "ok" {
		value += " " + StyleTextMuted.Render(truncate(collapseWhitespace(liveness.LastError), m.detailValueWidth(label, 60)))
	}
	return fmt.Sprintf("    %s: %s", StyleTextMuted.Render(label), value)
}

// npuChipSparseLines renders the per-chip lines that follow the chip table: one
// for chips reporting an error code (with the fault-code knowledge base class
// when the code is known), one for chips with HBM ECC history and one for chips
// the exporter attributes to a workload. All are sparse — a healthy idle node
// renders nothing here — and all stay outside the table so its column geometry
// cannot move.
func (m *Model) npuChipSparseLines(node *model.NodeData) []string {
	var lines []string

	for _, chip := range node.NPUChips {
		if chip.ErrorCode == 0 {
			continue
		}
		family := chip.ErrorCodeFamily
		if family == "" {
			family = "unknown"
		}
		lines = append(lines, m.TF("npudetail.error_code_chip", map[string]interface{}{
			"NPU":      chip.NPUID,
			"Chip":     chip.Chip,
			"Code":     fmt.Sprintf("%08X", chip.ErrorCode),
			"Category": StyleWarning.Render(m.T("npu.errorcat." + family)),
		}))
	}

	for _, chip := range node.NPUChips {
		single, double := chip.HBMEccSingleErrTotal, chip.HBMEccDoubleErrTotal
		if single == 0 && double == 0 && chip.HBMEccSingleErrDelta == 0 && chip.HBMEccDoubleErrDelta == 0 {
			continue
		}
		// The delta is only shown once the window holds two samples: with one
		// sample there is no increment to report, and printing Δ+0 would read
		// as "nothing happened since monitoring started".
		lines = append(lines, m.TF("npudetail.ecc_delta", map[string]interface{}{
			"NPU":         chip.NPUID,
			"Chip":        chip.Chip,
			"Single":      single,
			"SingleDelta": npuDeltaSuffix(chip.HBMEccDeltaSamples, chip.HBMEccSingleErrDelta),
			"Double":      double,
			"DoubleDelta": npuDeltaSuffix(chip.HBMEccDeltaSamples, chip.HBMEccDoubleErrDelta),
		}))
	}

	// One row per owned chip, not per pod: the row's identity is the chip, and
	// collapsing a pod's two chips into one row would drop which chips it holds.
	for _, chip := range node.NPUChips {
		if chip.OwnerPod == "" {
			continue
		}
		owner := chip.OwnerPod
		if chip.OwnerNamespace != "" {
			owner = chip.OwnerNamespace + "/" + chip.OwnerPod
		}
		// The value budget needs the label as it is actually rendered, with the
		// real chip numbers. The bundle template would measure its own
		// placeholders and cost the row the difference — a budget shrink no
		// terminal took away.
		rendered := m.TF("npudetail.chip_owner", map[string]interface{}{
			"NPU": chip.NPUID, "Chip": chip.Chip, "Owner": "",
		})
		label := strings.TrimPrefix(strings.TrimSuffix(rendered, " "), "    ")
		lines = append(lines, m.TF("npudetail.chip_owner", map[string]interface{}{
			"NPU":   chip.NPUID,
			"Chip":  chip.Chip,
			"Owner": truncate(owner, m.detailValueWidth(label, 60)),
		}))
	}

	return lines
}

// npuDeltaSuffix renders the " (Δ+n)" qualifier of the ECC delta line, or ""
// while the window is too short to have a delta.
func npuDeltaSuffix(samples int, delta int64) string {
	if samples < 2 {
		return ""
	}
	return fmt.Sprintf(" (Δ+%d)", delta)
}

// detailValueWidth returns how wide the value of a "    <label>: <value>" line
// may be so the line fits the terminal. The label is measured as rendered
// (localized) and stripped of ANSI, the indent is the four cells those lines
// use and one column is left free for the separator. Fallback caps the width on
// a terminal that reports no width yet (tests, before the first resize).
func (m *Model) detailValueWidth(label string, fallback int) int {
	if m.width <= 0 {
		return fallback
	}
	width := m.width - 4 - visualLength(label) - 2
	return max(1, min(fallback, width))
}

// extendedHBMCell renders the HBM cell of the extended chip table for one
// chip: used/total in GB with a percent, "-" when the chip reports no HBM.
// The HBM column width is measured from these same strings, so the cell and
// the width can never drift apart.
func extendedHBMCell(chip model.NPUChipData) string {
	if chip.HBMTotal <= 0 {
		return "-"
	}
	usedGB := float64(chip.HBMUsed) / 1024
	totalGB := float64(chip.HBMTotal) / 1024
	hbmPercent := float64(chip.HBMUsed) / float64(chip.HBMTotal) * 100
	return fmt.Sprintf("%.1f/%.0fG(%d%%)", usedGB, totalGB, int(hbmPercent))
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
