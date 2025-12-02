package ui

import (
	"fmt"
	"strings"
	"time"
)

// renderPVDetail renders detailed information about a PersistentVolume
func (m *Model) renderPVDetail() string {
	if m.selectedPV == nil {
		return m.T("detail.pv.no_selected")
	}

	pv := m.selectedPV
	var lines []string

	// Header
	header := StyleHeader.Render(fmt.Sprintf("📦 %s: %s", m.T("detail.pv.title"), pv.Name))
	lines = append(lines, header, "")

	// Basic Information Section
	lines = append(lines, StyleSubHeader.Render(m.T("detail.pv.basic_info")))
	lines = append(lines, renderSeparator(m.width))

	// Status
	var statusStr string
	switch pv.Status {
	case "Bound":
		statusStr = StyleStatusReady.Render(pv.Status)
	case "Available":
		statusStr = StyleStatusRunning.Render(pv.Status)
	case "Released":
		statusStr = StyleTextMuted.Render(pv.Status)
	default:
		statusStr = StyleWarning.Render(pv.Status)
	}
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.status"), statusStr))

	// Capacity
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.capacity"), StyleHighlight.Render(formatMemory(pv.Capacity))))

	// StorageClass
	if pv.StorageClass != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.storageclass"), StyleHighlight.Render(pv.StorageClass)))
	} else {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.storageclass"), StyleTextMuted.Render("<none>")))
	}

	// Reclaim Policy
	lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.reclaim_policy"), pv.ReclaimPolicy))

	// Volume Mode
	if pv.VolumeMode != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.volume_mode"), pv.VolumeMode))
	}

	// Volume Type
	if pv.VolumeType != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.type"), StyleHighlight.Render(pv.VolumeType)))
	}

	// Access Modes
	if len(pv.AccessModes) > 0 {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.access_modes"), strings.Join(pv.AccessModes, ", ")))
	}

	// Claim Binding
	lines = append(lines, "")
	lines = append(lines, StyleSubHeader.Render(m.T("detail.pv.claim_info")))
	lines = append(lines, renderSeparator(m.width))
	if pv.Claim != "" {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.bound_to"), StyleHighlight.Render(pv.Claim)))
	} else {
		lines = append(lines, fmt.Sprintf("  %s: %s", m.T("detail.pv.bound_to"), StyleTextMuted.Render(m.T("detail.pv.not_bound"))))
	}

	// Labels Section
	if len(pv.Labels) > 0 {
		lines = append(lines, "")
		lines = append(lines, StyleSubHeader.Render("🏷️  Labels"))
		lines = append(lines, renderSeparator(m.width))
		for key, value := range pv.Labels {
			if len(key)+len(value) > 70 {
				lines = append(lines, fmt.Sprintf("  %s: %s...", key, value[:60]))
			} else {
				lines = append(lines, fmt.Sprintf("  %s: %s", key, value))
			}
		}
	}

	// Annotations Section (show only if present and not too many)
	if len(pv.Annotations) > 0 && len(pv.Annotations) < 10 {
		lines = append(lines, "")
		lines = append(lines, StyleSubHeader.Render("📝 Annotations"))
		lines = append(lines, renderSeparator(m.width))
		count := 0
		for key, value := range pv.Annotations {
			if count >= 5 {
				lines = append(lines, StyleTextMuted.Render(fmt.Sprintf("  ... and %d more", len(pv.Annotations)-5)))
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
	lines = append(lines, StyleSubHeader.Render(m.T("detail.pv.lifecycle")))
	lines = append(lines, renderSeparator(m.width))
	lines = append(lines, fmt.Sprintf("  %s: %s (%s ago)",
		m.T("detail.created"),
		pv.CreationTimestamp.Format("2006-01-02 15:04:05"),
		formatDuration(time.Since(pv.CreationTimestamp))))

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
