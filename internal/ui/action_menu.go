package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ActionMenuItem represents a single action in the menu
type ActionMenuItem struct {
	Label       string
	Key         string
	Description string
	Action      ActionType
}

// ActionType defines the type of action
type ActionType int

const (
	ActionViewLogs ActionType = iota
	ActionDescribe
	ActionGetYAML
	ActionCopyName
	ActionCopyNamespaceName
	ActionShowEvents
)

// getActionMenuItems returns available actions based on current context
func (m *Model) getActionMenuItems() []ActionMenuItem {
	var items []ActionMenuItem

	// Common actions for Pod detail view
	if m.currentView == ViewPodDetail && m.selectedPod != nil {
		items = append(items, ActionMenuItem{
			Label:       "📜 View Logs",
			Key:         "1",
			Description: "Show container logs",
			Action:      ActionViewLogs,
		})
		items = append(items, ActionMenuItem{
			Label:       "📋 Describe",
			Key:         "2",
			Description: "kubectl describe pod",
			Action:      ActionDescribe,
		})
		items = append(items, ActionMenuItem{
			Label:       "📄 Get YAML",
			Key:         "3",
			Description: "kubectl get -o yaml",
			Action:      ActionGetYAML,
		})
		items = append(items, ActionMenuItem{
			Label:       "📎 Copy Name",
			Key:         "4",
			Description: "Copy pod name to clipboard",
			Action:      ActionCopyName,
		})
		items = append(items, ActionMenuItem{
			Label:       "📎 Copy Namespace/Name",
			Key:         "5",
			Description: "Copy namespace/name",
			Action:      ActionCopyNamespaceName,
		})
		items = append(items, ActionMenuItem{
			Label:       "⚡ Show Events",
			Key:         "6",
			Description: "Related events",
			Action:      ActionShowEvents,
		})
	}

	// Actions for Node detail view
	if m.currentView == ViewNodeDetail && m.selectedNode != nil {
		items = append(items, ActionMenuItem{
			Label:       "📋 Describe",
			Key:         "1",
			Description: "kubectl describe node",
			Action:      ActionDescribe,
		})
		items = append(items, ActionMenuItem{
			Label:       "📄 Get YAML",
			Key:         "2",
			Description: "kubectl get -o yaml",
			Action:      ActionGetYAML,
		})
		items = append(items, ActionMenuItem{
			Label:       "📎 Copy Name",
			Key:         "3",
			Description: "Copy node name",
			Action:      ActionCopyName,
		})
		items = append(items, ActionMenuItem{
			Label:       "⚡ Show Events",
			Key:         "4",
			Description: "Related events",
			Action:      ActionShowEvents,
		})
	}

	return items
}

// renderActionMenu renders the action menu overlay
func (m *Model) renderActionMenu() string {
	items := m.getActionMenuItems()

	if len(items) == 0 {
		return ""
	}

	// Build menu
	var menuLines []string

	// Title
	title := StyleHeader.Render("🎯 Quick Actions")
	menuLines = append(menuLines, title)
	menuLines = append(menuLines, "")

	// Menu items
	for i, item := range items {
		var itemStr string
		if i == m.actionMenuSelectedIndex {
			// Highlighted item
			itemStr = StyleSelected.Render(fmt.Sprintf("  %s  %s", item.Key, item.Label))
		} else {
			// Normal item
			itemStr = fmt.Sprintf("  %s  %s", StyleKey.Render(item.Key), item.Label)
		}
		menuLines = append(menuLines, itemStr)

		// Description (only for selected item)
		if i == m.actionMenuSelectedIndex {
			desc := StyleTextMuted.Render("     " + item.Description)
			menuLines = append(menuLines, desc)
		}
	}

	menuLines = append(menuLines, "")
	menuLines = append(menuLines, StyleTextMuted.Render("  ↑/↓ Navigate • Enter Select • ESC Cancel"))

	// Join and apply border
	content := strings.Join(menuLines, "\n")

	// Calculate menu dimensions
	maxWidth := 0
	for _, line := range menuLines {
		lineWidth := visualLength(line)
		if lineWidth > maxWidth {
			maxWidth = lineWidth
		}
	}

	// Add border
	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Width(maxWidth + 4)

	return menuStyle.Render(content)
}

// executeAction executes the selected action
func (m *Model) executeAction(action ActionType) tea.Cmd {
	switch action {
	case ActionViewLogs:
		if m.selectedPod != nil {
			m.logsMode = true
			m.logsScrollOffset = 0
			m.logsAutoScroll = true
			return m.fetchLogs()
		}

	case ActionDescribe:
		// This would execute kubectl describe and show output
		// For now, we'll show a message
		m.exportMessage = "Describe: kubectl describe not yet implemented"
		return tea.Tick(time.Second*3, func(time.Time) tea.Msg {
			return clearExportMessageMsg{}
		})

	case ActionGetYAML:
		// This would execute kubectl get -o yaml and show output
		m.exportMessage = "Get YAML: kubectl get not yet implemented"
		return tea.Tick(time.Second*3, func(time.Time) tea.Msg {
			return clearExportMessageMsg{}
		})

	case ActionCopyName:
		var name string
		if m.selectedPod != nil {
			name = m.selectedPod.Name
		} else if m.selectedNode != nil {
			name = m.selectedNode.Name
		}

		if name != "" {
			// Copy to clipboard (this requires a clipboard library)
			// For now, just show the name
			m.exportMessage = fmt.Sprintf("📋 Copied: %s", name)
			return tea.Tick(time.Second*2, func(time.Time) tea.Msg {
				return clearExportMessageMsg{}
			})
		}

	case ActionCopyNamespaceName:
		if m.selectedPod != nil {
			fullName := fmt.Sprintf("%s/%s", m.selectedPod.Namespace, m.selectedPod.Name)
			m.exportMessage = fmt.Sprintf("📋 Copied: %s", fullName)
			return tea.Tick(time.Second*2, func(time.Time) tea.Msg {
				return clearExportMessageMsg{}
			})
		}

	case ActionShowEvents:
		// Switch to events view filtered for this resource
		m.currentView = ViewEvents
		m.detailMode = false
		m.actionMenuMode = false
		return nil
	}

	return nil
}

// clearExportMessageMsg is sent to clear the export message
type clearExportMessageMsg struct{}
