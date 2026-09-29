package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus instruments.
type Metrics struct {
	DecisionsTotal       *prometheus.CounterVec
	DecisionDuration     *prometheus.HistogramVec
	DependencyErrors     *prometheus.CounterVec
	PolicyCacheHits      prometheus.Counter
	PolicyCacheMisses    prometheus.Counter
}

// NewMetrics registers and returns application metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		DecisionsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "quotaforge_decisions_total",
			Help: "Total number of rate-limit decisions made.",
		}, []string{"algorithm", "result"}), // result: allowed | denied

		DecisionDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "quotaforge_decision_duration_seconds",
			Help:    "Latency of rate-limit decision requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"algorithm"}),

		DependencyErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "quotaforge_dependency_errors_total",
			Help: "Total dependency errors by type.",
		}, []string{"dep"}), // dep: redis | postgres

		PolicyCacheHits: factory.NewCounter(prometheus.CounterOpts{
			Name: "quotaforge_policy_cache_hits_total",
			Help: "Policy cache hits.",
		}),
		PolicyCacheMisses: factory.NewCounter(prometheus.CounterOpts{
			Name: "quotaforge_policy_cache_misses_total",
			Help: "Policy cache misses.",
		}),
	}
}
