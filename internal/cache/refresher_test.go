package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chenpu/k8s-monitor/internal/datasource"
	"github.com/chenpu/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// blockingDataSource stands in for a stalled API Server: every call blocks
// until the caller's context is done, then reports the context error.
type blockingDataSource struct {
	observedErr error
}

func (b *blockingDataSource) GetNodes(ctx context.Context) ([]*model.NodeData, error) {
	<-ctx.Done()
	b.observedErr = ctx.Err()
	return nil, ctx.Err()
}

func (b *blockingDataSource) GetPods(ctx context.Context, namespace string) ([]*model.PodData, error) {
	<-ctx.Done()
	b.observedErr = ctx.Err()
	return nil, ctx.Err()
}

func (b *blockingDataSource) GetEvents(ctx context.Context, namespace string, eventTypes []string, limit int) ([]*model.EventData, error) {
	<-ctx.Done()
	b.observedErr = ctx.Err()
	return nil, ctx.Err()
}

func (b *blockingDataSource) Name() string { return "blocking" }

func (b *blockingDataSource) Close() error { return nil }

// TestRefreshRoundIsBoundedByTimeout pins the per-round deadline: a data source
// that never answers must not block the refresh loop forever.
func TestRefreshRoundIsBoundedByTimeout(t *testing.T) {
	source := &blockingDataSource{}
	dataSource := datasource.NewAggregatedDataSource(source, nil, zap.NewNop(), 1)
	refresher := NewRefresher(dataSource, NewTTLCache(time.Minute, zap.NewNop()), time.Hour, 50*time.Millisecond, "", zap.NewNop())

	start := time.Now()
	refresher.refresh()
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("refresh round took %s with a 50ms timeout, want it bounded by the round deadline", elapsed)
	}
	if !errors.Is(source.observedErr, context.DeadlineExceeded) {
		t.Errorf("data source context error = %v, want context.DeadlineExceeded (round must carry a deadline)", source.observedErr)
	}

	// The failure must be recorded so RefreshNow can report it to the UI.
	if err := refresher.RefreshNow(); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("RefreshNow error = %v, want context.DeadlineExceeded", err)
	}
}

// TestNewRefresherDefaultsTimeout covers the fallback for an unset timeout so a
// zero value can never turn into an immediately-expired round.
func TestNewRefresherDefaultsTimeout(t *testing.T) {
	dataSource := datasource.NewAggregatedDataSource(&blockingDataSource{}, nil, zap.NewNop(), 1)

	for _, configured := range []time.Duration{0, -time.Second} {
		refresher := NewRefresher(dataSource, NewTTLCache(time.Minute, zap.NewNop()), time.Second, configured, "", zap.NewNop())
		if refresher.refreshTimeout != defaultRefreshTimeout {
			t.Errorf("refreshTimeout for configured %s = %s, want %s", configured, refresher.refreshTimeout, defaultRefreshTimeout)
		}
	}

	refresher := NewRefresher(dataSource, NewTTLCache(time.Minute, zap.NewNop()), time.Second, 7*time.Second, "", zap.NewNop())
	if refresher.refreshTimeout != 7*time.Second {
		t.Errorf("refreshTimeout = %s, want the configured 7s", refresher.refreshTimeout)
	}
}

// TestRefreshStopsWhenCancelled verifies that Stop() (which cancels the
// refresher context) unblocks an in-flight round immediately instead of making
// the caller wait for the round timeout.
func TestRefreshStopsWhenCancelled(t *testing.T) {
	source := &blockingDataSource{}
	dataSource := datasource.NewAggregatedDataSource(source, nil, zap.NewNop(), 1)
	refresher := NewRefresher(dataSource, NewTTLCache(time.Minute, zap.NewNop()), time.Hour, time.Hour, "", zap.NewNop())

	done := make(chan struct{})
	go func() {
		refresher.refresh()
		close(done)
	}()

	// Give the round time to reach the blocking data source, then cancel.
	time.Sleep(20 * time.Millisecond)
	refresher.cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh round did not return after the refresher context was cancelled")
	}

	if !errors.Is(source.observedErr, context.Canceled) {
		t.Errorf("data source context error = %v, want context.Canceled", source.observedErr)
	}
}
