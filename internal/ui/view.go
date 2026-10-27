package ui

import (
	"fmt"
	"strings"
)

// View renders the UI
func (m *Model) View() string {
	if m.quitting {
		return m.T("view.goodbye") + "\n"
	}

	if m.width == 0 {
		return m.T("view.loading")
	}

	// Render header
	header := m.renderHeader()

	// Render command output view if in command output mode
	if m.commandOutputMode {
		content := m.renderCommandOutput()
		footer := m.renderFooter()
		return m.fitFrame(header, content, "", footer)
	}

	// Render logs view if in logs mode
	if m.logsMode {
		content := m.renderLogs()
		footer := m.renderFooter()
		return m.fitFrame(header, content, "", footer)
	}

	// The list renderers report how many rows they printed; clear it so a view
	// that does not report rows never reads the previous view's number.
	m.listRows = 0

	// Render current view
	var content string
	switch m.currentView {
	case ViewOverview:
		content = m.renderOverview()
	case ViewNodes:
		content = m.renderNodes()
	case ViewPods:
		content = m.renderPods()
	case ViewEvents:
		content = m.renderEvents()
	case ViewAlerts:
		content = m.renderAlerts()
	case ViewWorkloads:
		content = m.renderWorkloads()
	case ViewNetwork:
		content = m.renderNetwork()
	case ViewNodeDetail:
		content = m.renderNodeDetail()
	case ViewPodDetail:
		content = m.renderPodDetail()
	case ViewEventDetail:
		content = m.renderEventDetail()
	case ViewJobDetail:
		content = m.renderJobDetail()
	case ViewServiceDetail:
		content = m.renderServiceDetail()
	case ViewDeploymentDetail:
		content = m.renderDeploymentDetail()
	case ViewStatefulSetDetail:
		content = m.renderStatefulSetDetail()
	case ViewDaemonSetDetail:
		content = m.renderDaemonSetDetail()
	case ViewCronJobDetail:
		content = m.renderCronJobDetail()
	case ViewVolcanoJobDetail:
		content = m.renderVolcanoJobDetail()
	case ViewStorage:
		content = m.renderStorage()
	case ViewPVDetail:
		content = m.renderPVDetail()
	case ViewPVCDetail:
		content = m.renderPVCDetail()
	case ViewQueues:
		content = m.renderQueues()
	case ViewQueueDetail:
		content = m.renderQueueDetail()
	case ViewTopology:
		content = m.renderTopology()
	case ViewNPU:
		content = m.renderNPUView()
	case ViewTopologyDetail:
		content = m.renderSuperPodDetail()
	}

	// Render footer
	footer := m.renderFooter()

	// Render view tabs (only in main views, not in logs or detail modes)
	tabs := m.renderViewTabs()

	// Build base view
	result := m.fitFrame(header, content, tabs, footer)

	// Add export message if present
	if m.exportMessage != "" {
		result += "\n\n" + StyleKey.Render(m.exportMessage)
	}

	// Overlay action menu if active (should be on top)
	if m.actionMenuMode {
		menu := m.renderActionMenu()
		// Simple overlay - just append the menu
		// In a more sophisticated implementation, you could calculate position
		// and overlay it properly, but for now we'll just show it at the bottom
		result += "\n\n" + menu
	}

	return result
}

// renderHeader renders the header
func (m *Model) renderHeader() string {
	// Title with author and version
	titleText := m.T("app.title")
	author := m.T("app.author")
	version := m.version
	title := StyleTitle.Render(m.TF("app.title_byline", map[string]interface{}{
		"Title":   titleText,
		"Author":  author,
		"Version": version,
	}))

	// spinner indicator toggles on each refresh
	spinChars := []string{"◐", "◓", "◑", "◒"}
	spin := spinChars[0]
	if m.refreshCounter > 0 {
		spin = spinChars[m.refreshCounter%len(spinChars)]
	}

	var statusText string
	if m.err != nil {
		statusText = StyleError.Render(fmt.Sprintf("%s: %v", m.T("common.error"), m.err))
	} else if m.clusterData != nil {
		status := fmt.Sprintf("%s %s: %s", spin, m.T("common.last_updated"), m.lastUpdate.Format("15:04:05"))
		if m.refreshInterval > 0 {
			status += fmt.Sprintf(" • %s: %s", m.T("common.auto_refresh"), m.refreshInterval)
		}
		statusText = StyleSubtitle.Render(status)
	} else {
		loading := m.T("common.loading")
		if m.refreshInterval > 0 {
			loading = fmt.Sprintf("%s %s: %s", loading, m.T("common.auto_refresh"), m.refreshInterval)
		}
		statusText = StyleSubtitle.Render(loading)
	}

	return fmt.Sprintf("%s\n%s", title, statusText)
}

// getMaxIndex returns the maximum index for the current view
func (m *Model) getMaxIndex() int {
	if m.clusterData == nil {
		return 0
	}
	switch m.currentView {
	case ViewNodes:
		if m.cachedSortedNodes != nil {
			return len(m.cachedSortedNodes)
		}
		return len(m.getFilteredNodes())
	case ViewPods:
		if m.cachedSortedPods != nil {
			return len(m.cachedSortedPods)
		}
		return len(m.getFilteredPods())
	case ViewEvents:
		if m.cachedSortedEvents != nil {
			return len(m.cachedSortedEvents)
		}
		return len(m.getFilteredEvents())
	case ViewAlerts:
		if m.clusterData.Summary != nil {
			return len(m.clusterData.Summary.Alerts)
		}
		return 0
	case ViewWorkloads:
		// Sum all selectable workloads including Volcano jobs
		total := len(m.clusterData.Services) + len(m.clusterData.Jobs) +
			len(m.clusterData.Deployments) + len(m.clusterData.StatefulSets) +
			len(m.clusterData.DaemonSets) + len(m.clusterData.CronJobs) +
			len(m.clusterData.VolcanoJobs)
		return total
	case ViewNetwork:
		// For network view, use services count as the scrollable items
		return len(m.clusterData.Services)
	case ViewStorage:
		// Storage view shows PVs and PVCs
		return len(m.clusterData.PVs) + len(m.clusterData.PVCs)
	case ViewQueues:
		// Queue view shows Volcano queues
		return len(m.clusterData.Queues)
	case ViewTopology:
		// Topology view shows SuperPods
		return len(m.getSuperPodTopology())
	case ViewNPU:
		// NPU view shows one row per NPU node
		return len(m.npuNodes())
	default:
		return 0
	}
}

// renderFooter renders the footer with key bindings
func (m *Model) renderFooter() string {
	bindings := []string{
		RenderKeyBinding("q", m.T("keys.quit")),
		RenderKeyBinding("r", m.T("keys.refresh")),
	}

	// Different key bindings for different modes
	if m.commandOutputMode {
		// Command output mode - show scroll and exit bindings
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.back")))
	} else if m.logsSearchMode {
		// Logs search mode - show search-specific bindings
		bindings = append(bindings, RenderKeyBinding("text", m.T("keys.type_to_search")))
		bindings = append(bindings, RenderKeyBinding("backspace", m.T("keys.delete")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.cancel")))
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
	} else if m.logsMode {
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
		bindings = append(bindings, RenderKeyBinding("/", m.T("keys.search")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.back")))
	} else if m.searchMode {
		bindings = append(bindings, RenderKeyBinding("text", m.T("keys.type_to_search")))
		bindings = append(bindings, RenderKeyBinding("backspace", m.T("keys.delete")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.cancel")))
	} else if m.filterMode {
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.select")))
		bindings = append(bindings, RenderKeyBinding("enter", m.T("keys.apply")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.cancel")))
	} else if m.detailMode {
		bindings = append(bindings, RenderKeyBinding("↑/↓", m.T("keys.scroll")))
		bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
		bindings = append(bindings, RenderKeyBinding("esc", m.T("keys.back")))
		// Add logs key binding for pod detail view
		if m.currentView == ViewPodDetail {
			bindings = append(bindings, RenderKeyBinding("l", m.T("keys.logs")))
		}
		// Add actions key binding for pod and node detail views
		if m.currentView == ViewPodDetail || m.currentView == ViewNodeDetail {
			bindings = append(bindings, RenderKeyBinding("a", m.T("keys.actions")))
		}
	} else {
		bindings = append(bindings, RenderKeyBinding("1-8", m.T("keys.views")))
		bindings = append(bindings, RenderKeyBinding("tab", m.T("keys.next")))
		// Add navigation help for list views
		if m.currentView != ViewOverview {
			bindings = append(bindings, RenderKeyBinding("↑/k", m.T("keys.up")), RenderKeyBinding("↓/j", m.T("keys.down")))
			bindings = append(bindings, RenderKeyBinding("PgUp/PgDn", m.T("keys.page")))
			bindings = append(bindings, RenderKeyBinding("enter", m.T("keys.detail")))
			bindings = append(bindings, RenderKeyBinding("s", m.T("keys.sort")))
			bindings = append(bindings, RenderKeyBinding("/", m.T("keys.search")))
		}
		// Add filter help for Pods view
		if m.currentView == ViewPods {
			bindings = append(bindings, RenderKeyBinding("f", m.T("keys.filter")))
		}
		// Add matrix-mode help for NPU view: the mode legend names the active
		// mode, this segment points at the key that changes it.
		if m.currentView == ViewNPU {
			bindings = append(bindings, RenderKeyBinding("h", m.T("keys.npu_matrix")))
		}
		// Show clear if any filter is active
		if m.filterNamespace != "" || m.filterStatus != "" || m.filterRole != "" || m.searchText != "" {
			bindings = append(bindings, RenderKeyBinding("c", m.T("keys.clear")))
		}
	}

	footer := ""
	for i, binding := range bindings {
		if i > 0 {
			footer += " • "
		}
		footer += binding
	}

	// On a terminal narrower than the joined bindings the footer would wrap
	// and push the layout around, so clip it to the terminal width. truncate
	// strips ANSI and adds "..." when it actually cuts, which is acceptable
	// for a help footer; a footer that fits is returned untouched. m.width 0
	// (unknown, e.g. tests) must not clip everything — View() only renders
	// with a positive width.
	if m.width > 0 {
		footer = truncate(footer, m.width)
	}

	return StyleKeyDesc.Render(footer)
}

// renderViewTabs renders the view navigation tabs
func (m *Model) renderViewTabs() string {
	// Don't show tabs in detail mode
	if m.detailMode {
		return ""
	}

	type viewTab struct {
		label   string // the key that switches to the view, as shown in the tab
		nameKey string
		view    ViewType
	}

	tabs := []viewTab{
		{"1", "views.overview.name", ViewOverview},
		{"2", "views.nodes.name", ViewNodes},
		{"3", "views.pods.name", ViewPods},
		{"4", "views.workloads.name", ViewWorkloads},
		{"5", "views.network.name", ViewNetwork},
		{"6", "views.storage.name", ViewStorage},
		{"7", "views.events.name", ViewEvents},
		{"8", "views.alerts.name", ViewAlerts},
	}

	// Add NPU tab if the cluster has NPU nodes. It sits before the Queues and
	// Topology tabs: chip health is the primary asset view, the scheduler and
	// topology views are the AI-specific extras behind it. The Tab cycle in
	// Update builds the same order.
	if m.hasNPUNodes() {
		tabs = append(tabs, viewTab{"n", "views.npu.name", ViewNPU})
	}

	// Add Queues tab if Volcano is available
	if m.hasVolcanoQueues() {
		tabs = append(tabs, viewTab{"9", "views.queues.name", ViewQueues})
	}

	// Add Topology tab if SuperPod info is available
	if m.hasSuperPodTopology() {
		tabs = append(tabs, viewTab{"0", "views.topology.name", ViewTopology})
	}

	var tabParts []string
	for _, tab := range tabs {
		tabText := fmt.Sprintf("%s:%s", tab.label, m.T(tab.nameKey))

		if m.currentView == tab.view {
			// Highlight current view
			tabParts = append(tabParts, StyleSelected.Render(" "+tabText+" "))
		} else {
			// Normal view
			tabParts = append(tabParts, StyleTextMuted.Render(" "+tabText+" "))
		}
	}

	return strings.Join(tabParts, " ")
}

// renderSearchPanel renders the search panel
func (m *Model) renderSearchPanel() string {
	var lines []string

	lines = append(lines, StyleHeader.Render(m.T("search.title")))
	lines = append(lines, "")

	// Show search input with cursor
	searchDisplay := m.searchText + "█" // Use block cursor
	if m.searchText == "" {
		searchDisplay = "█" // Just show cursor when empty
	}
	lines = append(lines, fmt.Sprintf("  %s", StyleHighlight.Render(searchDisplay)))
	lines = append(lines, "")
	lines = append(lines, StyleTextMuted.Render("  "+m.T("search.placeholder")))
	lines = append(lines, StyleTextMuted.Render("  "+m.T("search.help")))

	return strings.Join(lines, "\n")
}
