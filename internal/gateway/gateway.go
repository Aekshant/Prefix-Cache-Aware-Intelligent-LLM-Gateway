// Package gateway implements prefix-cache-aware routing of chat requests
// to a pool of LLM workers.
package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"llm-gateway/internal/config"
	"llm-gateway/internal/metrics"
)

// Worker is a downstream LLM backend together with its live connection count.
type Worker struct {
	ID          string
	URL         string
	ActiveConns int64
}

// cacheEntry records which worker last served a given prompt prefix.
type cacheEntry struct {
	WorkerID     string
	LastAccessed time.Time
}

// Gateway routes requests with the same prompt prefix to the same worker so
// the worker's KV/prefix cache stays warm.
type Gateway struct {
	workers        []*Worker
	cache          map[uint64]cacheEntry
	ttl            time.Duration
	maxActiveConns int64
	prefixBytes    int

	mu         sync.Mutex
	rrIndex    uint64
	httpClient *http.Client
}

// New builds a Gateway from the given configuration.
func New(cfg *config.Config) *Gateway {
	workers := make([]*Worker, len(cfg.Workers))
	for i, w := range cfg.Workers {
		workers[i] = &Worker{ID: w.ID, URL: w.URL}
	}

	return &Gateway{
		workers:        workers,
		cache:          make(map[uint64]cacheEntry),
		ttl:            cfg.CacheTTL,
		maxActiveConns: cfg.MaxActiveConns,
		prefixBytes:    cfg.PrefixBytes,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// SelectWorker returns the worker that should serve a prefix and whether it
// was a cache hit. A hit refreshes the entry's timestamp; if the cached worker
// is saturated or unknown, it falls back to round robin and re-pins the prefix.
func (g *Gateway) SelectWorker(prefixHash uint64) (*Worker, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()

	if entry, hit := g.cache[prefixHash]; hit {
		entry.LastAccessed = now
		g.cache[prefixHash] = entry

		for _, w := range g.workers {
			if w.ID == entry.WorkerID && atomic.LoadInt64(&w.ActiveConns) < g.maxActiveConns {
				return w, true
			}
		}
	}

	idx := atomic.AddUint64(&g.rrIndex, 1) % uint64(len(g.workers))
	selected := g.workers[idx]

	g.cache[prefixHash] = cacheEntry{WorkerID: selected.ID, LastAccessed: now}
	metrics.ActiveCachedPrefixes.Set(float64(len(g.cache)))

	return selected, false
}

// RunJanitor periodically evicts expired cache entries until ctx is cancelled.
func (g *Gateway) RunJanitor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.evictExpired(time.Now())
		}
	}
}

func (g *Gateway) evictExpired(now time.Time) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	evicted := 0
	for hash, entry := range g.cache {
		if now.Sub(entry.LastAccessed) > g.ttl {
			delete(g.cache, hash)
			evicted++
			metrics.CacheEvictions.Inc()
			slog.Debug("evicted stale prefix", "prefix", hashHex(hash), "ttl", g.ttl)
		}
	}

	metrics.ActiveCachedPrefixes.Set(float64(len(g.cache)))

	if evicted > 0 {
		slog.Info("janitor cleaned expired entries", "evicted", evicted, "remaining", len(g.cache))
	}
	return evicted
}
