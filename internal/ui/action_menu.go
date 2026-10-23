package ui

import (
	"context"
	"fmt"
	"strings"

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
			Label:       m.T("action_menu.view_logs"),
			Key:         "1",
			Description: m.T("action_menu.view_logs_desc"),
			Action:      ActionViewLogs,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.describe"),
			Key:         "2",
			Description: m.T("action_menu.describe_pod_desc"),
			Action:      ActionDescribe,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.get_yaml"),
			Key:         "3",
			Description: m.T("action_menu.get_yaml_desc"),
			Action:      ActionGetYAML,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.copy_name"),
			Key:         "4",
			Description: m.T("action_menu.copy_name_pod_desc"),
			Action:      ActionCopyName,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.copy_ns_name"),
			Key:         "5",
			Description: m.T("action_menu.copy_ns_name_desc"),
			Action:      ActionCopyNamespaceName,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.show_events"),
			Key:         "6",
			Description: m.T("action_menu.show_events_desc"),
			Action:      ActionShowEvents,
		})
	}

	// Actions for Node detail view
	if m.currentView == ViewNodeDetail && m.selectedNode != nil {
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.describe"),
			Key:         "1",
			Description: m.T("action_menu.describe_node_desc"),
			Action:      ActionDescribe,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.get_yaml"),
			Key:         "2",
			Description: m.T("action_menu.get_yaml_desc"),
			Action:      ActionGetYAML,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.copy_name"),
			Key:         "3",
			Description: m.T("action_menu.copy_name_node_desc"),
			Action:      ActionCopyName,
		})
		items = append(items, ActionMenuItem{
			Label:       m.T("action_menu.show_events"),
			Key:         "4",
			Description: m.T("action_menu.show_events_desc"),
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
	title := StyleHeader.Render(m.T("action_menu.title"))
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
	menuLines = append(menuLines, StyleTextMuted.Render(m.T("action_menu.help")))

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
					title:   m.T("common.error"),
					content: m.T("action_menu.unsupported_describe"),
					err:     fmt.Errorf("unsupported operation"),
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
					title:   m.T("action_menu.describe_error_title"),
					content: err.Error(),
					err:     err,
				}
			}

			var title string
			if m.selectedPod != nil {
				title = m.TF("action_menu.describe_pod_title", map[string]interface{}{
					"Namespace": m.selectedPod.Namespace,
					"Name":      m.selectedPod.Name,
				})
			} else if m.selectedNode != nil {
				title = m.TF("action_menu.describe_node_title", map[string]interface{}{
					"Name": m.selectedNode.Name,
				})
			}

			return commandOutputMsg{
				title:   title,
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
					title:   m.T("common.error"),
					content: m.T("action_menu.unsupported_yaml"),
					err:     fmt.Errorf("unsupported operation"),
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
					title:   m.T("action_menu.get_yaml_error_title"),
					content: err.Error(),
					err:     err,
				}
			}

			var title string
			if m.selectedPod != nil {
				title = m.TF("action_menu.yaml_pod_title", map[string]interface{}{
					"Namespace": m.selectedPod.Namespace,
					"Name":      m.selectedPod.Name,
				})
			} else if m.selectedNode != nil {
				title = m.TF("action_menu.yaml_node_title", map[string]interface{}{
					"Name": m.selectedNode.Name,
				})
			}

			return commandOutputMsg{
				title:   title,
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
			return copyToClipboard(name)
		}

	case ActionCopyNamespaceName:
		if m.selectedPod != nil {
			return copyToClipboard(fmt.Sprintf("%s/%s", m.selectedPod.Namespace, m.selectedPod.Name))
		}

	case ActionShowEvents:
		// Switch to the events view (the listing is unfiltered). The events
		// view is a plain list, so every origin flag the detail set on the way
		// in is dropped here: leaving them set made a later pod detail opened
		// from the pods list believe it came from a job, and Esc jumped to the
		// job detail instead of back to the list.
		m.currentView = ViewEvents
		m.detailMode = false
		m.actionMenuMode = false
		m.fromJobDetail = false
		m.fromVolcanoJobDetail = false
		m.fromNetworkView = false
		m.fromNPUView = false
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

// clipboardResultMsg is sent when an asynchronous clipboard write finishes
type clipboardResultMsg struct {
	text string
	err  error
}

// copyToClipboard writes text to the system clipboard. On Linux the clipboard
// libraries spawn a helper process (xclip/xsel), which can block for a long
// time, so the write must not run on the UI goroutine.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		return clipboardResultMsg{text: text, err: clipboard.WriteAll(text)}
	}
}

// clearExportMessageMsg is sent to clear the export message
type clearExportMessageMsg struct{}
