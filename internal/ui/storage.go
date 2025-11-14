package ui

import (
	"fmt"
	"strings"

	"github.com/yourusername/k8s-monitor/internal/model"
)

// renderStorage renders the storage view (PVs and PVCs)
func (m *Model) renderStorage() string {
	if m.clusterData == nil {
		return m.T("msg.no_data")
	}

	var lines []string

	// Header
	header := StyleHeader.Render(m.T("storage.title"))
	lines = append(lines, header, "")

	// Summary statistics
	summary := m.clusterData.Summary
	if summary != nil {
		statLines := []string{
			m.TF("storage.stats.pvs", map[string]interface{}{
				"Total":     summary.TotalPVs,
				"Bound":     summary.BoundPVs,
				"Available": summary.AvailablePVs,
				"Released":  summary.ReleasedPVs,
			}),
			m.TF("storage.stats.pvcs", map[string]interface{}{
				"Total":   summary.TotalPVCs,
				"Bound":   summary.BoundPVCs,
				"Pending": summary.PendingPVCs,
			}),
			m.TF("storage.stats.size", map[string]interface{}{
				"Used":    formatMemory(summary.UsedStorageSize),
				"Total":   formatMemory(summary.TotalStorageSize),
				"Percent": fmt.Sprintf("%.1f", summary.StorageUsagePercent),
			}),
		}
		lines = append(lines, statLines...)
		lines = append(lines, "")
	}

	totalPVs := len(m.clusterData.PVs)
	totalPVCs := len(m.clusterData.PVCs)
	totalItems := totalPVs + totalPVCs

	// Calculate max visible items based on screen height
	// Use same value as global scroll logic (m.height - 10) for consistency
	maxVisible := m.height - 10 // Reserve space for header, footer, tabs
	if maxVisible < 5 {
		maxVisible = 5
	}

	// Clamp scroll offset to valid range
	maxScroll := totalItems - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Column widths for tables
	const (
		colPVName         = 40
		colPVStatus       = 12
		colPVClaim        = 20
		colPVCapacity     = 12
		colPVStorageClass = 20

		colPVCName         = 30
		colPVCNamespace    = 15
		colPVCStatus       = 12
		colPVCVolume       = 20
		colPVCCapacity     = 12
		colPVCStorageClass = 20
	)

	// Track how many items we've rendered and the actual visible range
	rendered := 0
	startItem := -1 // First item shown (for scroll indicator)
	endItem := -1   // Last item shown (for scroll indicator)

	// Render PersistentVolumes section
	if len(m.clusterData.PVs) > 0 {
		pvHeader := StyleSubHeader.Render(m.T("storage.pvs.title"))
		lines = append(lines, pvHeader)
		lines = append(lines, renderSeparator(m.width))

		// Table header
		headerLine := fmt.Sprintf("%s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colPVName),
			padRight(m.T("columns.status"), colPVStatus),
			padRight(m.T("columns.claim"), colPVClaim),
			padRight(m.T("columns.capacity"), colPVCapacity),
			padRight(m.T("columns.storageclass"), colPVStorageClass))
		lines = append(lines, StyleTextMuted.Render(headerLine))
		lines = append(lines, renderSeparator(m.width))

		// Render PV rows with pagination
		for idx, pv := range m.clusterData.PVs {
			// Skip items before scroll offset
			if idx < m.scrollOffset {
				continue
			}
			// Stop if we've rendered enough items
			if rendered >= maxVisible {
				break
			}

			// Track first and last visible items
			if startItem == -1 {
				startItem = idx
			}
			endItem = idx

			pvLine := m.renderPVRow(pv, idx, colPVName, colPVStatus, colPVClaim, colPVCapacity, colPVStorageClass)
			lines = append(lines, pvLine)
			rendered++
		}
		lines = append(lines, "")
	}

	// Render PersistentVolumeClaims section
	if len(m.clusterData.PVCs) > 0 && rendered < maxVisible {
		pvcHeader := StyleSubHeader.Render(m.T("storage.pvcs.title"))
		lines = append(lines, pvcHeader)
		lines = append(lines, renderSeparator(m.width))

		// Table header
		headerLine := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
			padRight(m.T("columns.name"), colPVCName),
			padRight(m.T("columns.namespace"), colPVCNamespace),
			padRight(m.T("columns.status"), colPVCStatus),
			padRight(m.T("columns.volume"), colPVCVolume),
			padRight(m.T("columns.capacity"), colPVCCapacity),
			padRight(m.T("columns.storageclass"), colPVCStorageClass))
		lines = append(lines, StyleTextMuted.Render(headerLine))
		lines = append(lines, renderSeparator(m.width))

		// Render PVC rows with pagination
		for idx, pvc := range m.clusterData.PVCs {
			virtualIdx := totalPVs + idx // Virtual index in unified list
			// Skip items before scroll offset
			if virtualIdx < m.scrollOffset {
				continue
			}
			// Stop if we've rendered enough items
			if rendered >= maxVisible {
				break
			}

			// Track first and last visible items
			if startItem == -1 {
				startItem = virtualIdx
			}
			endItem = virtualIdx

			pvcLine := m.renderPVCRow(pvc, virtualIdx, colPVCName, colPVCNamespace, colPVCStatus, colPVCVolume, colPVCCapacity, colPVCStorageClass)
			lines = append(lines, pvcLine)
			rendered++
		}
	}

	// Scroll indicator
	if totalItems > maxVisible && startItem != -1 && endItem != -1 {
		scrollInfo := m.TF("scroll.showing", map[string]interface{}{
			"Start": startItem + 1,
			"End":   endItem + 1,
			"Total": totalItems,
		})
		lines = append(lines, "")
		lines = append(lines, StyleTextMuted.Render(scrollInfo))
	}

	return strings.Join(lines, "\n")
}

// renderPVRow renders a single PV row
func (m *Model) renderPVRow(pv *model.PVData, index int, colName, colStatus, colClaim, colCapacity, colStorageClass int) string {
	// Truncate name if too long
	name := truncate(pv.Name, colName)

	// Status with color
	var status string
	switch pv.Status {
	case "Bound":
		status = StyleStatusReady.Render(pv.Status)
	case "Available":
		status = StyleStatusRunning.Render(pv.Status)
	case "Released":
		status = StyleTextMuted.Render(pv.Status)
	default:
		status = StyleWarning.Render(pv.Status)
	}

	// Claim (truncate if too long)
	claim := pv.Claim
	if claim == "" {
		claim = "-"
	}
	claim = truncate(claim, colClaim)

	// Capacity
	capacity := formatMemory(pv.Capacity)

	// StorageClass
	storageClass := pv.StorageClass
	if storageClass == "" {
		storageClass = "-"
	}
	storageClass = truncate(storageClass, colStorageClass)

	// Build line with proper padding
	line := fmt.Sprintf("%s  %s  %s  %s  %s",
		padRight(name, colName),
		padRight(status, colStatus),
		padRight(claim, colClaim),
		padRight(capacity, colCapacity),
		padRight(storageClass, colStorageClass),
	)

	// Highlight selected row
	if index == m.selectedIndex {
		return StyleSelected.Render(line)
	}

	return line
}

// renderPVCRow renders a single PVC row
func (m *Model) renderPVCRow(pvc *model.PVCData, index int, colName, colNamespace, colStatus, colVolume, colCapacity, colStorageClass int) string {
	// Truncate name if too long
	name := truncate(pvc.Name, colName)

	// Namespace
	namespace := truncate(pvc.Namespace, colNamespace)

	// Status with color
	var status string
	switch pvc.Status {
	case "Bound":
		status = StyleStatusReady.Render(pvc.Status)
	case "Pending":
		status = StyleWarning.Render(pvc.Status)
	default:
		status = StyleTextMuted.Render(pvc.Status)
	}

	// Volume
	volume := pvc.Volume
	if volume == "" {
		volume = "-"
	}
	volume = truncate(volume, colVolume)

	// Capacity
	var capacity string
	if pvc.Capacity > 0 {
		capacity = formatMemory(pvc.Capacity)
	} else {
		capacity = "-"
	}

	// StorageClass
	storageClass := pvc.StorageClass
	if storageClass == "" {
		storageClass = "-"
	}
	storageClass = truncate(storageClass, colStorageClass)

	// Build line with proper padding
	line := fmt.Sprintf("%s  %s  %s  %s  %s  %s",
		padRight(name, colName),
		padRight(namespace, colNamespace),
		padRight(status, colStatus),
		padRight(volume, colVolume),
		padRight(capacity, colCapacity),
		padRight(storageClass, colStorageClass),
	)

	// Highlight selected row
	if index == m.selectedIndex {
		return StyleSelected.Render(line)
	}

	return line
}
