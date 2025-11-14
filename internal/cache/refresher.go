package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/k8s-monitor/internal/datasource"
	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// Refresher handles automatic data refresh
type Refresher struct {
	dataSource      *datasource.AggregatedDataSource
	cache           *TTLCache
	refreshInterval time.Duration
	namespace       string
	logger          *zap.Logger

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
	isRunning  bool
	lastError  error
	lastUpdate time.Time

	// For rate calculation
	lastSummary *model.ClusterSummary
	lastSample time.Time
}

// NewRefresher creates a new data refresher
func NewRefresher(
	dataSource *datasource.AggregatedDataSource,
	cache *TTLCache,
	refreshInterval time.Duration,
	namespace string,
	logger *zap.Logger,
) *Refresher {
	ctx, cancel := context.WithCancel(context.Background())

	return &Refresher{
		dataSource:      dataSource,
		cache:           cache,
		refreshInterval: refreshInterval,
		namespace:       namespace,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start begins the automatic refresh process
func (r *Refresher) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRunning {
		return fmt.Errorf("refresher already running")
	}

	r.logger.Info("Starting data refresher",
		zap.Duration("interval", r.refreshInterval),
		zap.String("namespace", r.namespace),
	)

	r.isRunning = true
	r.wg.Add(1)

	go r.run()

	return nil
}

// Stop stops the automatic refresh process
func (r *Refresher) Stop() error {
	r.mu.Lock()
	if !r.isRunning {
		r.mu.Unlock()
		return fmt.Errorf("refresher not running")
	}
	r.mu.Unlock()

	r.logger.Info("Stopping data refresher")

	r.cancel()
	r.wg.Wait()

	r.mu.Lock()
	r.isRunning = false
	r.mu.Unlock()

	r.logger.Info("Data refresher stopped")
	return nil
}

// run is the main refresh loop
func (r *Refresher) run() {
	defer r.wg.Done()

	// Do initial refresh immediately
	r.refresh()

	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			r.logger.Debug("Refresh loop exiting")
			return

		case <-ticker.C:
			r.refresh()
		}
	}
}

// refresh performs a single refresh operation
func (r *Refresher) refresh() {
	r.logger.Debug("Refreshing cluster data")

	startTime := time.Now()

	data, err := r.dataSource.GetClusterData(r.ctx, r.namespace)
	if err != nil {
		r.mu.Lock()
		r.lastError = err
		r.mu.Unlock()

		r.logger.Error("Failed to refresh cluster data",
			zap.Error(err),
			zap.Duration("elapsed", time.Since(startTime)),
		)
		return
	}

	r.computeNetworkRates(data.Summary)

	// Update cache
	if err := r.cache.Set(r.ctx, data); err != nil {
		r.logger.Error("Failed to update cache",
			zap.Error(err),
		)
		return
	}

	now := time.Now()
	r.mu.Lock()
	r.lastError = nil
	r.lastUpdate = now
	if data.Summary != nil {
		snapshot := *data.Summary
		r.lastSummary = &snapshot
		r.lastSample = now
	}
	r.mu.Unlock()

	r.logger.Info("Cluster data refreshed successfully",
		zap.Duration("elapsed", time.Since(startTime)),
		zap.Int("nodes", len(data.Nodes)),
		zap.Int("pods", len(data.Pods)),
		zap.Int("events", len(data.Events)),
	)
}

// computeNetworkRates updates the summary with instantaneous network rates
func (r *Refresher) computeNetworkRates(summary *model.ClusterSummary) {
	if summary == nil {
		return
	}

	// Skip if no network data available
	if summary.NetworkRxBytes == 0 && summary.NetworkTxBytes == 0 {
		return
	}

	r.mu.RLock()
	prevSummary := r.lastSummary
	prevSample := r.lastSample
	r.mu.RUnlock()

	// First refresh, no historical data yet
	if prevSummary == nil || prevSample.IsZero() {
		r.logger.Debug("Network rate calculation skipped: no historical data (first refresh)")
		return
	}

	elapsed := time.Since(prevSample).Seconds()
	if elapsed <= 0 {
		r.logger.Warn("Network rate calculation skipped: invalid time delta",
			zap.Float64("elapsed", elapsed))
		return
	}

	// Calculate RX rate
	rxDelta := summary.NetworkRxBytes - prevSummary.NetworkRxBytes
	if rxDelta >= 0 {
		summary.NetworkRxRate = int64(float64(rxDelta) / elapsed)
	} else {
		// Negative delta indicates counter reset (node restart, etc.)
		r.logger.Debug("Network RX counter reset detected",
			zap.Int64("current", summary.NetworkRxBytes),
			zap.Int64("previous", prevSummary.NetworkRxBytes))
		summary.NetworkRxRate = 0
	}

	// Calculate TX rate
	txDelta := summary.NetworkTxBytes - prevSummary.NetworkTxBytes
	if txDelta >= 0 {
		summary.NetworkTxRate = int64(float64(txDelta) / elapsed)
	} else {
		// Negative delta indicates counter reset
		r.logger.Debug("Network TX counter reset detected",
			zap.Int64("current", summary.NetworkTxBytes),
			zap.Int64("previous", prevSummary.NetworkTxBytes))
		summary.NetworkTxRate = 0
	}

	r.logger.Debug("Network rates calculated",
		zap.Int64("rx_rate", summary.NetworkRxRate),
		zap.Int64("tx_rate", summary.NetworkTxRate),
		zap.Float64("interval_seconds", elapsed),
		zap.Int64("rx_delta", rxDelta),
		zap.Int64("tx_delta", txDelta),
	)
}

// RefreshNow forces an immediate refresh
func (r *Refresher) RefreshNow() error {
	r.logger.Info("Forcing immediate refresh")
	r.refresh()

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastError
}

// GetStatus returns the current refresher status
func (r *Refresher) GetStatus() RefresherStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return RefresherStatus{
		IsRunning:  r.isRunning,
		LastUpdate: r.lastUpdate,
		LastError:  r.lastError,
		Interval:   r.refreshInterval,
	}
}

// RefresherStatus represents the current state of the refresher
type RefresherStatus struct {
	IsRunning  bool
	LastUpdate time.Time
	LastError  error
	Interval   time.Duration
}

// SetInterval updates the refresh interval
func (r *Refresher) SetInterval(interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Updating refresh interval",
		zap.Duration("old_interval", r.refreshInterval),
		zap.Duration("new_interval", interval),
	)

	r.refreshInterval = interval
}

// SetNamespace updates the namespace filter
func (r *Refresher) SetNamespace(namespace string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Updating namespace filter",
		zap.String("old_namespace", r.namespace),
		zap.String("new_namespace", namespace),
	)

	r.namespace = namespace

	// Trigger immediate refresh with new namespace
	go r.refresh()
}
