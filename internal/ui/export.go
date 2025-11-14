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
	return func() tea.Msg {
		// Get export directory
		exportDir, err := getExportDir()
		if err != nil {
			return exportErrorMsg{err: err}
		}

		timestamp := time.Now().Format("20060102-150405")
		var filename string
		var exportErr error

		switch m.currentView {
		case ViewNodes:
			filename = fmt.Sprintf("k8s-nodes-%s", timestamp)
			exportErr = m.exportNodes(exportDir, filename, format)
		case ViewPods:
			filename = fmt.Sprintf("k8s-pods-%s", timestamp)
			exportErr = m.exportPods(exportDir, filename, format)
		case ViewEvents:
			filename = fmt.Sprintf("k8s-events-%s", timestamp)
			exportErr = m.exportEvents(exportDir, filename, format)
		case ViewNetwork:
			filename = fmt.Sprintf("k8s-services-%s", timestamp)
			exportErr = m.exportServices(exportDir, filename, format)
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
		count := m.getExportCount()

		return exportSuccessMsg{filePath: fullPath, count: count}
	}
}

// getExportCount returns the number of items exported
func (m *Model) getExportCount() int {
	if m.clusterData == nil {
		return 0
	}

	switch m.currentView {
	case ViewNodes:
		return len(m.clusterData.Nodes)
	case ViewPods:
		return len(m.clusterData.Pods)
	case ViewEvents:
		return len(m.clusterData.Events)
	case ViewNetwork:
		return len(m.clusterData.Services)
	default:
		return 0
	}
}

// exportNodes exports nodes data
func (m *Model) exportNodes(exportDir, filename string, format ExportFormat) error {
	if m.clusterData == nil || len(m.clusterData.Nodes) == 0 {
		return fmt.Errorf("no nodes data to export")
	}

	if format == ExportCSV {
		return m.exportNodesCSV(exportDir, filename+".csv")
	}
	return m.exportNodesJSON(exportDir, filename+".json")
}

// exportNodesCSV exports nodes to CSV format
func (m *Model) exportNodesCSV(exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Name", "Status", "Roles", "CPU", "Memory", "Pods", "Age"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, node := range m.clusterData.Nodes {
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
func (m *Model) exportNodesJSON(exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m.clusterData.Nodes)
}

// exportPods exports pods data
func (m *Model) exportPods(exportDir, filename string, format ExportFormat) error {
	if m.clusterData == nil || len(m.clusterData.Pods) == 0 {
		return fmt.Errorf("no pods data to export")
	}

	if format == ExportCSV {
		return m.exportPodsCSV(exportDir, filename+".csv")
	}
	return m.exportPodsJSON(exportDir, filename+".json")
}

// exportPodsCSV exports pods to CSV format
func (m *Model) exportPodsCSV(exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{"Namespace", "Name", "Phase", "Node", "CPU", "Memory", "Restarts", "Age"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Write data
	for _, pod := range m.clusterData.Pods {
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
func (m *Model) exportPodsJSON(exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m.clusterData.Pods)
}

// exportEvents exports events data
func (m *Model) exportEvents(exportDir, filename string, format ExportFormat) error {
	if m.clusterData == nil || len(m.clusterData.Events) == 0 {
		return fmt.Errorf("no events data to export")
	}

	if format == ExportCSV {
		return m.exportEventsCSV(exportDir, filename+".csv")
	}
	return m.exportEventsJSON(exportDir, filename+".json")
}

// exportEventsCSV exports events to CSV format
func (m *Model) exportEventsCSV(exportDir, filename string) error {
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
	for _, event := range m.clusterData.Events {
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
func (m *Model) exportEventsJSON(exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m.clusterData.Events)
}

// exportServices exports services data
func (m *Model) exportServices(exportDir, filename string, format ExportFormat) error {
	if m.clusterData == nil || len(m.clusterData.Services) == 0 {
		return fmt.Errorf("no services data to export")
	}

	if format == ExportCSV {
		return m.exportServicesCSV(exportDir, filename+".csv")
	}
	return m.exportServicesJSON(exportDir, filename+".json")
}

// exportServicesCSV exports services to CSV format
func (m *Model) exportServicesCSV(exportDir, filename string) error {
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
	for _, svc := range m.clusterData.Services {
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
func (m *Model) exportServicesJSON(exportDir, filename string) error {
	fullPath := filepath.Join(exportDir, filename)
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m.clusterData.Services)
}
