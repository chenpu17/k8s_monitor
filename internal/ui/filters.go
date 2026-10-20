package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// handleFilterNavigation handles navigation in filter mode
func (m *Model) handleFilterNavigation(direction int) tea.Cmd {
	namespaces := m.getNamespaces()
	totalOptions := len(namespaces) + 1 // +1 for "All" option

	// Find current selection index
	currentIdx := 0
	if m.filterNamespace != "" {
		for i, ns := range namespaces {
			if ns == m.filterNamespace {
				currentIdx = i + 1 // +1 because "All" is at index 0
				break
			}
		}
	}

	// Update selection
	newIdx := currentIdx + direction
	if newIdx < 0 {
		newIdx = totalOptions - 1
	} else if newIdx >= totalOptions {
		newIdx = 0
	}

	// Apply selection
	if newIdx == 0 {
		m.filterNamespace = ""
	} else {
		m.filterNamespace = namespaces[newIdx-1]
	}

	// Reset scroll and selection when filter changes
	m.scrollOffset = 0
	m.selectedIndex = 0

	return nil
}

// getNamespaces returns a sorted list of unique namespaces from pods
func (m *Model) getNamespaces() []string {
	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		return []string{}
	}

	nsMap := make(map[string]bool)
	for _, pod := range m.clusterData.Pods {
		nsMap[pod.Namespace] = true
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}

	// Use sort.Strings for O(n log n) sorting
	sort.Strings(namespaces)

	return namespaces
}

// getFilteredPods returns pods filtered by namespace, status, and search text
func (m *Model) getFilteredPods() []*model.PodData {
	if m.clusterData == nil {
		return []*model.PodData{}
	}

	// Apply all filters in a single pass for better performance
	filtered := make([]*model.PodData, 0, len(m.clusterData.Pods))
	searchLower := strings.ToLower(m.searchText)

	for _, pod := range m.clusterData.Pods {
		// Check namespace filter
		if m.filterNamespace != "" && pod.Namespace != m.filterNamespace {
			continue
		}

		// Check status filter
		if m.filterStatus != "" && pod.Phase != m.filterStatus {
			continue
		}

		// Check search text filter (case-insensitive partial match on name)
		if m.searchText != "" && !strings.Contains(strings.ToLower(pod.Name), searchLower) {
			continue
		}

		// All filters passed, include this pod
		filtered = append(filtered, pod)
	}

	return filtered
}

// getFilteredEvents returns events filtered by type and search text
func (m *Model) getFilteredEvents() []*model.EventData {
	if m.clusterData == nil {
		return []*model.EventData{}
	}

	// Start with all events
	filtered := m.clusterData.Events

	// Apply event type filter
	if m.filterEventType != "" {
		temp := []*model.EventData{}
		for _, event := range filtered {
			if event.Type == m.filterEventType {
				temp = append(temp, event)
			}
		}
		filtered = temp
	}

	// Apply search text filter (case-insensitive partial match on reason or message)
	if m.searchText != "" {
		temp := []*model.EventData{}
		searchLower := strings.ToLower(m.searchText)
		for _, event := range filtered {
			if strings.Contains(strings.ToLower(event.Reason), searchLower) ||
				strings.Contains(strings.ToLower(event.Message), searchLower) ||
				strings.Contains(strings.ToLower(event.InvolvedObject), searchLower) {
				temp = append(temp, event)
			}
		}
		filtered = temp
	}

	return filtered
}

// getFilteredNodes returns nodes filtered by status, role, and search text
func (m *Model) getFilteredNodes() []*model.NodeData {
	if m.clusterData == nil {
		return []*model.NodeData{}
	}

	// Start with all nodes
	filtered := m.clusterData.Nodes

	// Apply status filter
	if m.filterStatus != "" {
		temp := []*model.NodeData{}
		for _, node := range filtered {
			if node.Status == m.filterStatus {
				temp = append(temp, node)
			}
		}
		filtered = temp
	}

	// Apply role filter
	if m.filterRole != "" {
		temp := []*model.NodeData{}
		for _, node := range filtered {
			for _, role := range node.Roles {
				if role == m.filterRole {
					temp = append(temp, node)
					break
				}
			}
		}
		filtered = temp
	}

	// Apply search text filter (case-insensitive partial match on name)
	if m.searchText != "" {
		temp := []*model.NodeData{}
		searchLower := strings.ToLower(m.searchText)
		for _, node := range filtered {
			if strings.Contains(strings.ToLower(node.Name), searchLower) {
				temp = append(temp, node)
			}
		}
		filtered = temp
	}

	return filtered
}

// clusterHasNPU returns true if the cluster has any nodes with NPU
func (m *Model) clusterHasNPU() bool {
	if m.clusterData == nil || m.clusterData.Summary == nil {
		return false
	}
	return m.clusterData.Summary.NPUCapacity > 0
}

// hasVolcanoQueues returns true if Volcano queues are available
func (m *Model) hasVolcanoQueues() bool {
	if m.clusterData == nil {
		return false
	}
	return len(m.clusterData.Queues) > 0
}
