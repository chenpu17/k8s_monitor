package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
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
		if m.selectedPod != nil && len(m.selectedPod.ContainerStates) > 0 {
			// Select first container by default
			m.selectedContainer = m.selectedPod.ContainerStates[0].Name
			m.logsMode = true
			m.logsScrollOffset = 0
			m.logsAutoScroll = true
			return m.fetchLogs()
		}

	case ActionDescribe:
		// Execute describe command asynchronously
		return func() tea.Msg {
			var content string
			var err error

			// Try to get the API client through type assertion
			apiClient, ok := m.dataProvider.(interface {
				DescribePod(ctx context.Context, namespace, podName string) (string, error)
				DescribeNode(ctx context.Context, nodeName string) (string, error)
			})

			if !ok {
				return commandOutputMsg{
					title: "Error",
					content: "API client does not support describe functionality",
					err: fmt.Errorf("unsupported operation"),
				}
			}

			ctx := context.Background()
			if m.selectedPod != nil {
				content, err = apiClient.DescribePod(ctx, m.selectedPod.Namespace, m.selectedPod.Name)
			} else if m.selectedNode != nil {
				content, err = apiClient.DescribeNode(ctx, m.selectedNode.Name)
			}

			if err != nil {
				return commandOutputMsg{
					title: "Describe Error",
					content: err.Error(),
					err: err,
				}
			}

			var title string
			if m.selectedPod != nil {
				title = fmt.Sprintf("Describe Pod: %s/%s", m.selectedPod.Namespace, m.selectedPod.Name)
			} else if m.selectedNode != nil {
				title = fmt.Sprintf("Describe Node: %s", m.selectedNode.Name)
			}

			return commandOutputMsg{
				title: title,
				content: content,
			}
		}

	case ActionGetYAML:
		// Execute get yaml command asynchronously
		return func() tea.Msg {
			var content string
			var err error

			// Try to get the API client through type assertion
			apiClient, ok := m.dataProvider.(interface {
				GetPodYAML(ctx context.Context, namespace, podName string) (string, error)
				GetNodeYAML(ctx context.Context, nodeName string) (string, error)
			})

			if !ok {
				return commandOutputMsg{
					title: "Error",
					content: "API client does not support YAML export",
					err: fmt.Errorf("unsupported operation"),
				}
			}

			ctx := context.Background()
			if m.selectedPod != nil {
				content, err = apiClient.GetPodYAML(ctx, m.selectedPod.Namespace, m.selectedPod.Name)
			} else if m.selectedNode != nil {
				content, err = apiClient.GetNodeYAML(ctx, m.selectedNode.Name)
			}

			if err != nil {
				return commandOutputMsg{
					title: "Get YAML Error",
					content: err.Error(),
					err: err,
				}
			}

			var title string
			if m.selectedPod != nil {
				title = fmt.Sprintf("YAML: %s/%s", m.selectedPod.Namespace, m.selectedPod.Name)
			} else if m.selectedNode != nil {
				title = fmt.Sprintf("YAML: %s", m.selectedNode.Name)
			}

			return commandOutputMsg{
				title: title,
				content: content,
			}
		}

	case ActionCopyName:
		var name string
		if m.selectedPod != nil {
			name = m.selectedPod.Name
		} else if m.selectedNode != nil {
			name = m.selectedNode.Name
		}

		if name != "" {
			// Actually copy to clipboard
			err := clipboard.WriteAll(name)
			if err != nil {
				m.exportMessage = fmt.Sprintf("❌ Copy failed: %v", err)
			} else {
				m.exportMessage = fmt.Sprintf("✅ Copied: %s", name)
			}
			return tea.Tick(time.Second*2, func(time.Time) tea.Msg {
				return clearExportMessageMsg{}
			})
		}

	case ActionCopyNamespaceName:
		if m.selectedPod != nil {
			fullName := fmt.Sprintf("%s/%s", m.selectedPod.Namespace, m.selectedPod.Name)
			// Actually copy to clipboard
			err := clipboard.WriteAll(fullName)
			if err != nil {
				m.exportMessage = fmt.Sprintf("❌ Copy failed: %v", err)
			} else {
				m.exportMessage = fmt.Sprintf("✅ Copied: %s", fullName)
			}
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

// commandOutputMsg is sent when a command output is ready to display
type commandOutputMsg struct {
	title   string
	content string
	err     error
}

// clearExportMessageMsg is sent to clear the export message
type clearExportMessageMsg struct{}
