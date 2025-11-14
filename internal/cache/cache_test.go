package cache

import (
	"context"
	"testing"
	"time"

	"github.com/yourusername/k8s-monitor/internal/model"
	"go.uber.org/zap"
)

func TestTTLCache(t *testing.T) {
	logger := zap.NewNop()
	cache := NewTTLCache(1*time.Second, logger)
	ctx := context.Background()

	// Test initial cache miss
	_, ok := cache.Get(ctx)
	if ok {
		t.Error("Expected cache miss on empty cache")
	}

	// Test cache set
	data := &model.ClusterData{
		Nodes: []*model.NodeData{
			{Name: "node1"},
		},
		Pods: []*model.PodData{
			{Name: "pod1"},
		},
	}

	err := cache.Set(ctx, data)
	if err != nil {
		t.Errorf("Failed to set cache: %v", err)
	}

	// Test cache hit
	cached, ok := cache.Get(ctx)
	if !ok {
		t.Error("Expected cache hit")
	}
	if len(cached.Nodes) != 1 || cached.Nodes[0].Name != "node1" {
		t.Error("Cached data mismatch")
	}

	// Test cache expiration
	time.Sleep(1100 * time.Millisecond)
	_, ok = cache.Get(ctx)
	if ok {
		t.Error("Expected cache miss after expiration")
	}
}

func TestTTLCacheInvalidate(t *testing.T) {
	logger := zap.NewNop()
	cache := NewTTLCache(10*time.Second, logger)
	ctx := context.Background()

	// Set cache
	data := &model.ClusterData{
		Nodes: []*model.NodeData{{Name: "node1"}},
	}
	cache.Set(ctx, data)

	// Verify cache hit
	_, ok := cache.Get(ctx)
	if !ok {
		t.Error("Expected cache hit")
	}

	// Invalidate cache
	err := cache.Invalidate()
	if err != nil {
		t.Errorf("Failed to invalidate cache: %v", err)
	}

	// Verify cache miss
	_, ok = cache.Get(ctx)
	if ok {
		t.Error("Expected cache miss after invalidation")
	}
}

func TestTTLCacheSetTTL(t *testing.T) {
	logger := zap.NewNop()
	cache := NewTTLCache(1*time.Second, logger)

	if cache.GetTTL() != 1*time.Second {
		t.Errorf("Expected TTL 1s, got %v", cache.GetTTL())
	}

	cache.SetTTL(5 * time.Second)

	if cache.GetTTL() != 5*time.Second {
		t.Errorf("Expected TTL 5s, got %v", cache.GetTTL())
	}
}

func TestTTLCacheIsExpired(t *testing.T) {
	logger := zap.NewNop()
	cache := NewTTLCache(100*time.Millisecond, logger)
	ctx := context.Background()

	// Initially expired (no data)
	if !cache.IsExpiredSafe() {
		t.Error("Expected cache to be expired initially")
	}

	// Set data
	data := &model.ClusterData{}
	cache.Set(ctx, data)

	// Should not be expired immediately
	if cache.IsExpiredSafe() {
		t.Error("Expected cache to not be expired immediately after set")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	if !cache.IsExpiredSafe() {
		t.Error("Expected cache to be expired after TTL")
	}
}
