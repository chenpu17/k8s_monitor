package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderPodDetail renders the pod detail view
func (m *Model) renderPodDetail() string {
	if m.selectedPod == nil {
		return m.T("detail.pod.no_selected")
	}

	pod := m.selectedPod

	// Collect all content lines
	var allLines []string

	// Pod header
	allLines = append(allLines, m.renderPodDetailHeader(pod))
	allLines = append(allLines, "")

	// Pod basic info
	basicInfo := m.renderPodBasicInfo(pod)
	allLines = append(allLines, strings.Split(basicInfo, "\n")...)
	allLines = append(allLines, "")

	// Pod container info
	containerInfo := m.renderPodContainerInfo(pod)
	allLines = append(allLines, strings.Split(containerInfo, "\n")...)

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
	if m.detailScrollOffset > maxScroll {
		m.detailScrollOffset = maxScroll
	}
	if m.detailScrollOffset < 0 {
		m.detailScrollOffset = 0
	}

	startIdx := m.detailScrollOffset
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

// renderPodDetailHeader renders the pod detail view header
func (m *Model) renderPodDetailHeader(pod *model.PodData) string {
	title := StyleHeader.Render(fmt.Sprintf("📦 %s: %s", m.T("detail.pod.title"), pod.Name))
	status := RenderStatus(pod.Phase)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		"  ",
		status,
	)
}

// renderPodBasicInfo renders basic pod information
func (m *Model) renderPodBasicInfo(pod *model.PodData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.T("detail.pod.basic_info")))
	info = append(info, "")
	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.name")),
		pod.Name))

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.namespace")),
		pod.Namespace))

	info = append(info, fmt.Sprintf("  %s: %s",
		StyleTextSecondary.Render(m.T("detail.field.status")),
		RenderStatus(pod.Phase)))

	if pod.Node != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.node")),
			pod.Node))
	}

	if pod.PodIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.pod_ip")),
			pod.PodIP))
	}

	if pod.HostIP != "" {
		info = append(info, fmt.Sprintf("  %s: %s",
			StyleTextSecondary.Render(m.T("detail.field.host_ip")),
			pod.HostIP))
	}

	info = append(info, fmt.Sprintf("  %s: %d",
		StyleTextSecondary.Render(m.T("detail.field.restarts")),
		pod.RestartCount))

	// Network bandwidth (instantaneous MB/s, if we have history)
	info = append(info, "")
	info = append(info, StyleTextSecondary.Render("  "+m.T("detail.pod.network_bandwidth")))

	if len(m.metricHistory) < 2 {
		info = append(info, StyleTextMuted.Render("    "+m.T("detail.pod.collecting_data")))
	} else {
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		if _, ok := m.metricHistory[len(m.metricHistory)-1].PodMetrics[key]; !ok {
			info = append(info, StyleTextMuted.Render("    "+m.T("detail.pod.metrics_unavailable")))
		} else {
			rxRate := m.calculatePodNetworkRxRate(pod.Namespace, pod.Name)
			txRate := m.calculatePodNetworkTxRate(pod.Namespace, pod.Name)
			totalRate := rxRate + txRate

			info = append(info, fmt.Sprintf("    RX ↓: %s  •  TX ↑: %s  •  Total: %s",
				formatNetworkRate(rxRate),
				formatNetworkRate(txRate),
				formatNetworkRate(totalRate)))
		}
	}

	// Historical trends (if available)
	if len(m.metricHistory) >= 2 {
		info = append(info, "")
		info = append(info, StyleTextSecondary.Render("  "+m.T("detail.pod.historical_trends")))

		cpuHistory := m.getPodCPUHistory(pod.Namespace, pod.Name)
		if len(cpuHistory) >= 2 {
			sparkline := RenderSparkline(cpuHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.pod.cpu_label")),
				sparkline))
		}

		memHistory := m.getPodMemoryHistory(pod.Namespace, pod.Name)
		if len(memHistory) >= 2 {
			sparkline := RenderSparkline(memHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.pod.memory_label")),
				sparkline))
		}

		// Network traffic trends
		rxHistory := m.getPodNetworkRxHistory(pod.Namespace, pod.Name)
		if len(rxHistory) >= 2 {
			sparkline := RenderSparkline(rxHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.pod.network_rx_label")),
				sparkline))
		}

		txHistory := m.getPodNetworkTxHistory(pod.Namespace, pod.Name)
		if len(txHistory) >= 2 {
			sparkline := RenderSparkline(txHistory, 40)
			info = append(info, fmt.Sprintf("  %s %s",
				StyleTextMuted.Render(m.T("detail.pod.network_tx_label")),
				sparkline))
		}

		info = append(info, StyleTextMuted.Render("  "+m.TF("detail.pod.snapshots", map[string]interface{}{
			"Count": len(m.metricHistory),
		})))
	}

	return strings.Join(info, "\n")
}

// renderPodContainerInfo renders pod container information
func (m *Model) renderPodContainerInfo(pod *model.PodData) string {
	var info []string

	info = append(info, StyleHeader.Render(m.TF("detail.pod.containers", map[string]interface{}{
		"Total": pod.Containers,
		"Ready": pod.ReadyContainers,
	})))
	info = append(info, "")

	if len(pod.ContainerStates) == 0 {
		info = append(info, StyleTextMuted.Render("  "+m.T("detail.pod.no_container_info")))
		return strings.Join(info, "\n")
	}

	for i, container := range pod.ContainerStates {
		if i > 0 {
			info = append(info, "")
		}

		// Container name with status
		containerHeader := fmt.Sprintf("  %s %s",
			StyleHighlight.Render(fmt.Sprintf("[%d]", i+1)),
			container.Name)

		if container.Ready {
			containerHeader += " " + StyleStatusReady.Render("●")
		} else {
			containerHeader += " " + StyleStatusNotReady.Render("●")
		}

		info = append(info, containerHeader)

		// Container image
		image := container.Image
		if len(image) > 70 {
			image = image[:67] + "..."
		}
		info = append(info, fmt.Sprintf("      %s: %s",
			StyleTextSecondary.Render(m.T("detail.pod.image")),
			image))

		// Container status
		info = append(info, fmt.Sprintf("      %s: %s (restarts: %d)",
			StyleTextSecondary.Render(m.T("detail.pod.state")),
			container.State,
			container.RestartCount))

		// Show reason and message if available
		if container.Reason != "" {
			info = append(info, fmt.Sprintf("      %s: %s",
				StyleTextSecondary.Render("Reason"),
				container.Reason))
		}

		if container.Message != "" {
			msg := container.Message
			if len(msg) > 70 {
				msg = msg[:67] + "..."
			}
			info = append(info, fmt.Sprintf("      %s: %s",
				StyleTextSecondary.Render("Message"),
				msg))
		}

		// Resource usage (from kubelet metrics)
		if container.CPUUsage > 0 || container.MemoryUsage > 0 {
			info = append(info, fmt.Sprintf("      %s:",
				StyleTextSecondary.Render("Usage")))

			if container.CPUUsage > 0 {
				cpuUsage := FormatMillicores(container.CPUUsage)
				cpuLine := fmt.Sprintf("        CPU: %s", StyleWarning.Render(cpuUsage))
				if container.CPULimit > 0 {
					cpuPercent := float64(container.CPUUsage) / float64(container.CPULimit) * 100
					cpuLine += fmt.Sprintf(" / %s (%s)",
						FormatMillicores(container.CPULimit),
						StyleHighlight.Render(fmt.Sprintf("%.1f%%", cpuPercent)))
				} else if container.CPURequest > 0 {
					cpuLine += fmt.Sprintf(" (req: %s)", FormatMillicores(container.CPURequest))
				}
				info = append(info, cpuLine)
			}

			if container.MemoryUsage > 0 {
				memUsage := FormatBytes(container.MemoryUsage)
				memLine := fmt.Sprintf("        Mem: %s", StyleWarning.Render(memUsage))
				if container.MemoryLimit > 0 {
					memPercent := float64(container.MemoryUsage) / float64(container.MemoryLimit) * 100
					memLine += fmt.Sprintf(" / %s (%s)",
						FormatBytes(container.MemoryLimit),
						StyleHighlight.Render(fmt.Sprintf("%.1f%%", memPercent)))
				} else if container.MemoryRequest > 0 {
					memLine += fmt.Sprintf(" (req: %s)", FormatBytes(container.MemoryRequest))
				}
				info = append(info, memLine)
			}
		}

		// Resource requests/limits (if no usage available)
		if container.CPUUsage == 0 && container.MemoryUsage == 0 &&
			(container.CPURequest > 0 || container.CPULimit > 0 ||
				container.MemoryRequest > 0 || container.MemoryLimit > 0) {

			info = append(info, fmt.Sprintf("      %s:",
				StyleTextSecondary.Render("Resources")))

			if container.CPURequest > 0 || container.CPULimit > 0 {
				cpuLine := "        CPU:"
				if container.CPURequest > 0 {
					cpuLine += fmt.Sprintf(" req=%s", FormatMillicores(container.CPURequest))
				}
				if container.CPULimit > 0 {
					if container.CPURequest > 0 {
						cpuLine += ","
					}
					cpuLine += fmt.Sprintf(" limit=%s", FormatMillicores(container.CPULimit))
				}
				info = append(info, cpuLine)
			}

			if container.MemoryRequest > 0 || container.MemoryLimit > 0 {
				memLine := "        Mem:"
				if container.MemoryRequest > 0 {
					memLine += fmt.Sprintf(" req=%s", FormatBytes(container.MemoryRequest))
				}
				if container.MemoryLimit > 0 {
					if container.MemoryRequest > 0 {
						memLine += ","
					}
					memLine += fmt.Sprintf(" limit=%s", FormatBytes(container.MemoryLimit))
				}
				info = append(info, memLine)
			}
		}
	}

	return strings.Join(info, "\n")
}
