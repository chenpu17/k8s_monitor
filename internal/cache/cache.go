package cache

import (
	"context"
	"sync"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

// Cache defines the interface for data caching
type Cache interface {
	// Get retrieves cached cluster data
	Get(ctx context.Context) (*model.ClusterData, bool)

	// Set stores cluster data in cache
	Set(ctx context.Context, data *model.ClusterData) error

	// Invalidate clears the cache
	Invalidate() error

	// IsExpired checks if cache is expired
	IsExpired() bool
}

// TTLCache implements a time-based cache
type TTLCache struct {
	data      *model.ClusterData
	ttl       time.Duration
	expiresAt time.Time
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewTTLCache creates a new TTL-based cache
func NewTTLCache(ttl time.Duration, logger *zap.Logger) *TTLCache {
	return &TTLCache{
		ttl:    ttl,
		logger: logger,
	}
}

// Get retrieves cached cluster data
func (c *TTLCache) Get(ctx context.Context) (*model.ClusterData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.data == nil {
		c.logger.Debug("Cache miss: no data")
		return nil, false
	}

	if c.IsExpired() {
		c.logger.Debug("Cache miss: expired",
			zap.Time("expires_at", c.expiresAt),
			zap.Time("now", time.Now()),
		)
		return nil, false
	}

	c.logger.Debug("Cache hit",
		zap.Time("expires_at", c.expiresAt),
		zap.Duration("time_left", time.Until(c.expiresAt)),
	)

	return c.data, true
}

// Set stores cluster data in cache
func (c *TTLCache) Set(ctx context.Context, data *model.ClusterData) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = data
	c.expiresAt = time.Now().Add(c.ttl)

	c.logger.Debug("Cache updated",
		zap.Time("expires_at", c.expiresAt),
		zap.Duration("ttl", c.ttl),
		zap.Int("nodes", len(data.Nodes)),
		zap.Int("pods", len(data.Pods)),
	)

	// Update last refresh time in summary
	if data.Summary != nil {
		data.Summary.LastRefreshTime = time.Now()
	}

	return nil
}

// Invalidate clears the cache
func (c *TTLCache) Invalidate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = nil
	c.expiresAt = time.Time{}

	c.logger.Debug("Cache invalidated")
	return nil
}

// IsExpired checks if cache is expired (must be called with lock held or use IsExpiredSafe)
func (c *TTLCache) IsExpired() bool {
	return c.data == nil || time.Now().After(c.expiresAt)
}

// IsExpiredSafe is a thread-safe version of IsExpired
func (c *TTLCache) IsExpiredSafe() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.IsExpired()
}

// GetTTL returns the configured TTL
func (c *TTLCache) GetTTL() time.Duration {
	return c.ttl
}

// SetTTL updates the TTL duration
func (c *TTLCache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
	c.logger.Info("Cache TTL updated", zap.Duration("new_ttl", ttl))
}
