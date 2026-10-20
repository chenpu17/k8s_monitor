package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chenpu/k8s-monitor/internal/model"
)

// fetchData fetches cluster data
func (m *Model) fetchData() tea.Cmd {
	return func() tea.Msg {
		data, err := m.dataProvider.GetClusterData()
		return clusterDataMsg{data: data, err: err}
	}
}

// startFetchData issues a cluster data fetch unless the previous one is still in
// flight. Update runs on a single goroutine and the command reports back with a
// clusterDataMsg, so a plain flag is enough: without it, holding 'r' down (or a
// refresh interval shorter than a slow GetClusterData) piles up concurrent
// fetches, each of which replaces the whole cluster snapshot when it lands.
func (m *Model) startFetchData() tea.Cmd {
	if m.refreshInFlight {
		m.logger.Debug("refresh already in flight, skipping")
		return nil
	}
	m.refreshInFlight = true
	return m.fetchData()
}

func (m *Model) forceRefresh() tea.Cmd {
	return func() tea.Msg {
		if err := m.dataProvider.ForceRefresh(); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

func (m *Model) scheduleRefresh() tea.Cmd {
	if m.refreshInterval <= 0 {
		return nil
	}
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// Messages
type clusterDataMsg struct {
	data *model.ClusterData
	err  error
}

type errMsg struct {
	err error
}

type refreshTickMsg struct{}

type logsRefreshTickMsg time.Time

type logsMsg struct {
	logs string
	err  error
}

// fetchLogs fetches logs for the selected pod and container
func (m *Model) fetchLogs() tea.Cmd {
	if m.selectedPod == nil || m.selectedContainer == "" {
		return nil
	}

	pod := m.selectedPod
	container := m.selectedContainer

	return func() tea.Msg {
		// Need to get the APIServerClient to call GetPodLogs
		// We'll need to add a GetPodLogs method to DataProvider interface
		// For now, we'll type assert to get access to the underlying client

		// Try to get logs through the data provider
		apiClient, ok := m.dataProvider.(interface {
			GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error)
		})

		if !ok {
			return logsMsg{err: fmt.Errorf("data provider does not support log fetching")}
		}

		ctx := context.Background()
		logs, err := apiClient.GetPodLogs(ctx, pod.Namespace, pod.Name, container, int64(m.logTailLines))
		return logsMsg{logs: logs, err: err}
	}
}

// startLogsRefresh returns a command that sends a logsRefreshTickMsg after 2 seconds
func (m *Model) startLogsRefresh() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return logsRefreshTickMsg(t)
	})
}
