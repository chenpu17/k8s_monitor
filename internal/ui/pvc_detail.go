package ui

import (
	"fmt"
	"strings"
	"time"
)

// renderPVCDetail renders detailed information about a PersistentVolumeClaim
func (m *Model) renderPVCDetail() string {
	if m.selectedPVC == nil {
		return m.T("detail.pvc.no_selected")
	}

	pvc := m.selectedPVC
	var lines []string

	// Header
	header := StyleHeader.Render(fmt.Sprintf("📋 %s: %s", m.T("detail.pvc.title"), pvc.Name))
	lines = append(lines, header, "")

	// Basic Information Section
	lines = append(lines, StyleSubHeader.Render(m.T("detail.pvc.basic_info")))
	lines = append(lines, renderSeparator(m.width))

	// Namespace
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.namespace"), StyleHighlight.Render(pvc.Namespace)))

	// Status
	var statusStr string
	switch pvc.Status {
	case "Bound":
		statusStr = StyleStatusReady.Render(pvc.Status)
	case "Pending":
		statusStr = StyleWarning.Render(pvc.Status)
	case "Lost":
		statusStr = StyleStatusNotReady.Render(pvc.Status)
	default:
		statusStr = StyleTextMuted.Render(pvc.Status)
	}
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.status"), statusStr))

	// Bound Volume
	if pvc.Volume != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.volume"), StyleHighlight.Render(pvc.Volume)))
	} else {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.volume"), StyleTextMuted.Render(m.T("detail.pvc.not_bound"))))
	}

	// Capacity
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render(m.T("detail.pvc.spec")))
	lines = append(lines, renderSeparator(m.width))

	// Requested Storage
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.requested"), StyleHighlight.Render(formatMemory(pvc.RequestedStorage))))

	// Actual Capacity (if bound)
	if pvc.Capacity > 0 {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.capacity"), StyleHighlight.Render(formatMemory(pvc.Capacity))))
	} else {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.capacity"), StyleTextMuted.Render(m.T("detail.pvc.unknown"))))
	}

	// Used (if available)
	if pvc.UsedBytes > 0 {
		usagePercent := float64(pvc.UsedBytes) / float64(pvc.Capacity) * 100
		lines = append(lines, fmt.Sprintf("  Used: %s (%.1f%%)",
			StyleWarning.Render(formatMemory(pvc.UsedBytes)),
			usagePercent))
	}

	// Storage Configuration
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render("⚙️  Storage Configuration"))
	lines = append(lines, renderSeparator(m.width))

	// StorageClass
	if pvc.StorageClass != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.storageclass"), StyleHighlight.Render(pvc.StorageClass)))
	} else {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.storageclass"), StyleTextMuted.Render(m.T("detail.pvc.none"))))
	}

	// Volume Mode
	if pvc.VolumeMode != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.volume_mode"), pvc.VolumeMode))
	}

	// Access Modes
	if len(pvc.AccessModes) > 0 {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pvc.access_modes"), strings.Join(pvc.AccessModes, ", ")))
	}

	// Labels Section
	if len(pvc.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, StyleSubHeader.Render("🏷️  Labels"))
		lines = append(lines, renderSeparator(m.width))
		for key, value := range pvc.Labels {
			if len(key)+len(value) > 70 {
				lines = append(lines, fmt.Sprintf("  %s: %s...", key, value[:60]))
			} else {
				lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
			}
		}
	}

	// Annotations Section (show only if present and not too many)
	if len(pvc.Annotations) > 0 && len(pvc.Annotations) < 10 {
		lines = append(lines, "")
		lines = append(lines, StyleSubHeader.Render("📝 Annotations"))
		lines = append(lines, renderSeparator(m.width))
		count := 0
		for key, value := range pvc.Annotations {
			if count >= 5 {
				lines = append(lines, StyleTextMuted.Render(fmt.Sprintf("  ... and %d more", len(pvc.Annotations)-5)))
				break
			}
			if len(key)+len(value) > 70 {
				lines = append(lines, fmt.Sprintf("  %s: %s...", key, value[:60]))
			} else {
				lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
			}
			count++
		}
	}

	// Timestamps
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render(m.T("detail.pvc.lifecycle")))
	lines = append(lines, renderSeparator(m.width))
	lines = append(lines, fmt.Sprintf("  %s: %s (%s ago)",
		m.T("detail.created"),
		pvc.CreationTimestamp.Format("2006-01-02 15:04:05"),
		formatDuration(time.Since(pvc.CreationTimestamp))))

	// Handle scrolling for detail view
	maxVisible := m.height - 10
	if maxVisible < 5 {
		maxVisible = 5
	}

	// Clamp scroll offset to valid range (prevent scrolling beyond content)
	maxScroll := len(lines) - maxVisible
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
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	visibleLines := lines[startIdx:endIdx]

	// Add scroll indicator
	if len(lines) > maxVisible {
		scrollInfo := fmt.Sprintf("(viewing %d-%d of %d lines, use ↑↓ or PgUp/PgDn to scroll)",
			startIdx+1, endIdx, len(lines))
		visibleLines = append(visibleLines, "")
		visibleLines = append(visibleLines, StyleTextMuted.Render(scrollInfo))
	}

	return strings.Join(visibleLines, "\n")
}
