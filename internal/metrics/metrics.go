// Package metrics defines the Prometheus metrics exported by the gateway.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	TTFT = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "llm_ttft_seconds",
		Help:    "Time to first token in seconds",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
	}, []string{"worker", "cache_status"})

	ITL = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "llm_itl_seconds",
		Help:    "Inter-token latency in seconds",
		Buckets: []float64{0.01, 0.02, 0.05, 0.1, 0.2},
	}, []string{"worker"})

	CacheEvictions = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "llm_gateway_cache_evictions_total",
		Help: "Total number of expired prefix hashes evicted from the cache",
	})

	ActiveCachedPrefixes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "llm_gateway_active_cached_prefixes",
		Help: "Current count of warm prefix entries in gateway memory",
	})
)

func init() {
	prometheus.MustRegister(TTFT, ITL, CacheEvictions, ActiveCachedPrefixes)
}
