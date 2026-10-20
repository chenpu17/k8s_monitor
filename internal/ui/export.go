package ui

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// ExportFormat represents the export file format
type ExportFormat int

const (
	ExportCSV ExportFormat = iota
	ExportJSON
)

// exportSuccessMsg is sent when export completes successfully
type exportSuccessMsg struct {
	filePath string
	count    int
}

// exportErrorMsg is sent when export fails
type exportErrorMsg struct {
	err error
}

// getExportDir returns the export directory path and ensures it exists
func getExportDir() (string, error) {
	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir not available
		return ".", nil
	}

	// Use ~/.config/k8s-monitor/exports/
	exportDir := filepath.Join(homeDir, ".config", "k8s-monitor", "exports")

	// Check if directory exists
	if _, err := os.Stat(exportDir); os.IsNotExist(err) {
		// Create directory with proper permissions
		if err := os.MkdirAll(exportDir, 0755); err != nil {
			return "", fmt.Errorf("cannot create export directory %s: %w", exportDir, err)
		}
	}

	// Test write permission by creating a temporary file
	testFile := filepath.Join(exportDir, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return "", fmt.Errorf("export directory %s is not writable: %w", exportDir, err)
	}
	f.Close()
	os.Remove(testFile)

	return exportDir, nil
}

// exportData exports the current view data to a file
func (m *Model) exportData(format ExportFormat) tea.Cmd {
	// Snapshot the state the export reads: the returned command runs on its own
	// goroutine while Update keeps mutating the model, so reading m.currentView
	// or m.clusterData from inside the closure would race with the UI.
	view := m.currentView
	data := m.clusterData

	return func() tea.Msg {
		// Get export directory
		exportDir, err := getExportDir()
		if err != nil {
			return exportErrorMsg{err: err}
		}

		timestamp := time.Now().Format("20060102-150405")
		var filename string
		var exportErr error

		switch view {
		case ViewNodes:
			filename = fmt.Sprintf("k8s-nodes-%s", timestamp)
			exportErr = m.exportNodes(data, exportDir, filename, format)
		case ViewPods:
			filename = fmt.Sprintf("k8s-pods-%s", timestamp)
			exportErr = m.exportPods(data, exportDir, filename, format)
		case ViewEvents:
			filename = fmt.Sprintf("k8s-events-%s", timestamp)
			exportErr = m.exportEvents(data, exportDir, filename, format)
		case ViewNetwork:
			filename = fmt.Sprintf("k8s-services-%s", timestamp)
			exportErr = m.exportServices(data, exportDir, filename, format)
		default:
			return exportErrorMsg{err: fmt.Errorf("export not supported for this view")}
		}

		if exportErr != nil {
			return exportErrorMsg{err: exportErr}
		}

		// Get full path
		ext := ".csv"
		if format == ExportJSON {
			ext = ".json"
		}
		fullPath := filepath.Join(exportDir, filename+ext)
		count := m.getExportCount(view, data)

		return exportSuccessMsg{filePath: fullPath, count: count}
	}
}

// getExportCount returns the number of items exported
func (m *Model) getExportCount(view ViewType, data *model.ClusterData) int {
	if data == nil {
		return 0
	}

	switch view {
	case ViewNodes:
		return len(data.Nodes)
	case ViewPods:
		return len(data.Pods)
	case ViewEvents:
		return len(data.Events)
	case ViewNetwork:
		return len(data.Services)
	default:
		return 0
	}
}

// exportNodes exports nodes data
func (m *Model) exportNodes(data *model.ClusterData, exportDir, filename string, format ExportFormat) error {
	if data == nil || len(data.Nodes) == 0 {
		return fmt.Errorf("no nodes data to export")
	}

	if format == ExportCSV {
		return m.exportNodesCSV(data, exportDir, filename+".csv")
	}
	return m.exportNodesJSON(data, exportDir, filename+".json")
}

// exportNodesCSV exports nodes to CSV format
func (m *Model) exportNodesCSV(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		m.T("columns.name"),
		m.T("columns.status"),
		m.T("columns.roles"),
		m.T("columns.cpu"),
		m.T("columns.memory"),
		m.T("columns.pods"),
		m.T("columns.age"),
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, node := range data.Nodes {
		age := formatAge(time.Since(node.CreationTimestamp))
		roles := strings.Join(node.Roles, ",")
		if roles == "" {
			roles = "<none>"
		}

		record := []string{
			node.Name,
			node.Status,
			roles,
			fmt.Sprintf("%d", node.CPUUsage),
			fmt.Sprintf("%d", node.MemoryUsage),
			fmt.Sprintf("%d", node.PodCount),
			age,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// exportNodesJSON exports nodes to JSON format
func (m *Model) exportNodesJSON(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data.Nodes)
}

// exportPods exports pods data
func (m *Model) exportPods(data *model.ClusterData, exportDir, filename string, format ExportFormat) error {
	if data == nil || len(data.Pods) == 0 {
		return fmt.Errorf("no pods data to export")
	}

	if format == ExportCSV {
		return m.exportPodsCSV(data, exportDir, filename+".csv")
	}
	return m.exportPodsJSON(data, exportDir, filename+".json")
}

// exportPodsCSV exports pods to CSV format
func (m *Model) exportPodsCSV(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		m.T("columns.namespace"),
		m.T("columns.name"),
		m.T("columns.phase"),
		m.T("columns.node"),
		m.T("columns.cpu"),
		m.T("columns.memory"),
		m.T("columns.restarts"),
		m.T("columns.age"),
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, pod := range data.Pods {
		age := formatAge(time.Since(pod.CreationTimestamp))

		record := []string{
			pod.Namespace,
			pod.Name,
			pod.Phase,
			pod.Node,
			fmt.Sprintf("%d", pod.CPUUsage),
			fmt.Sprintf("%d", pod.MemoryUsage),
			fmt.Sprintf("%d", pod.RestartCount),
			age,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// exportPodsJSON exports pods to JSON format
func (m *Model) exportPodsJSON(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data.Pods)
}

// exportEvents exports events data
func (m *Model) exportEvents(data *model.ClusterData, exportDir, filename string, format ExportFormat) error {
	if data == nil || len(data.Events) == 0 {
		return fmt.Errorf("no events data to export")
	}

	if format == ExportCSV {
		return m.exportEventsCSV(data, exportDir, filename+".csv")
	}
	return m.exportEventsJSON(data, exportDir, filename+".json")
}

// exportEventsCSV exports events to CSV format
func (m *Model) exportEventsCSV(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Type", "Reason", "Object", "Message", "Count", "Age"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, event := range data.Events {
		age := formatAge(time.Since(event.LastTimestamp))

		record := []string{
			event.Type,
			event.Reason,
			event.InvolvedObject,
			event.Message,
			fmt.Sprintf("%d", event.Count),
			age,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// exportEventsJSON exports events to JSON format
func (m *Model) exportEventsJSON(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data.Events)
}

// exportServices exports services data
func (m *Model) exportServices(data *model.ClusterData, exportDir, filename string, format ExportFormat) error {
	if data == nil || len(data.Services) == 0 {
		return fmt.Errorf("no services data to export")
	}

	if format == ExportCSV {
		return m.exportServicesCSV(data, exportDir, filename+".csv")
	}
	return m.exportServicesJSON(data, exportDir, filename+".json")
}

// exportServicesCSV exports services to CSV format
func (m *Model) exportServicesCSV(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Namespace", "Name", "Type", "ClusterIP", "ExternalIP", "Ports", "Age"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, svc := range data.Services {
		age := formatAge(time.Since(svc.CreationTimestamp))

		// Format external IPs
		externalIP := strings.Join(svc.ExternalIPs, ",")
		if externalIP == "" {
			externalIP = "<none>"
		}

		// Format ports
		var portStrings []string
		for _, port := range svc.Ports {
			portStrings = append(portStrings, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
		}
		ports := strings.Join(portStrings, ",")
		if ports == "" {
			ports = "<none>"
		}

		record := []string{
			svc.Namespace,
			svc.Name,
			svc.Type,
			svc.ClusterIP,
			externalIP,
			ports,
			age,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// exportServicesJSON exports services to JSON format
func (m *Model) exportServicesJSON(data *model.ClusterData, exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data.Services)
}
