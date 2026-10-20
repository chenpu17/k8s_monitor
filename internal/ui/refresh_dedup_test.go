package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// blockingProvider is a DataProvider whose GetClusterData stays in flight until
// the test releases it, so a second refresh can be attempted while the first one
// has not reported back yet.
type blockingProvider struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func newBlockingProvider() *blockingProvider {
	return &blockingProvider{
		started: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
}

func (p *blockingProvider) GetClusterData() (*model.ClusterData, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	p.started <- struct{}{}
	<-p.release
	return &model.ClusterData{}, nil
}

func (p *blockingProvider) ForceRefresh() error { return nil }

func (p *blockingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// TestRefreshInFlightDeduplicated checks that a second refresh - manual 'r' or
// the auto-refresh tick - does not start another GetClusterData while the
// previous one is still in flight, and that refreshing works again once the
// result has arrived.
func TestRefreshInFlightDeduplicated(t *testing.T) {
	provider := newBlockingProvider()
	m := NewModel(provider, zap.NewNop(), time.Second, "en", "test", 200)
	m.width = 120
	m.height = 24
	m.currentView = ViewOverview

	// First refresh: run the command the way Bubble Tea would, in its own
	// goroutine, and leave it blocked inside GetClusterData.
	_, cmd := m.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("first 'r' did not start a refresh")
	}
	go cmd()
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh never reached the data provider")
	}

	// While it is in flight, neither 'r' nor a refresh tick may reach the data
	// provider again. Any command they hand back is run, so a refresh that
	// should have been suppressed shows up in the call count.
	if _, extra := m.Update(keyMsg("r")); extra != nil {
		go extra()
	}
	if _, extra := m.Update(refreshTickMsg{}); extra != nil {
		go extra()
	}

	// Give a command that should not have been issued time to reach the provider.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && provider.callCount() == 1 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := provider.callCount(); got != 1 {
		t.Errorf("GetClusterData called %d times while the first refresh was in flight, want 1", got)
	}

	// The result lands: the next 'r' must refresh again.
	m.Update(clusterDataMsg{data: &model.ClusterData{}})
	_, cmd = m.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("'r' did not refresh after the in-flight refresh had returned")
	}
	go cmd()
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh after the in-flight one never reached the data provider")
	}

	close(provider.release)
}
