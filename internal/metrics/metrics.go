// Package metrics provides a decoupled, extensible Prometheus metrics
// registry for the WOMS application. New metric types can be added by
// registering additional collectors via the exported Registry.
package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps a Prometheus registry so callers can register custom
// collectors without coupling to prometheus directly.
var Registry = prometheus.NewRegistry()

func init() {
	// Expose default Go runtime and process metrics alongside custom ones.
	Registry.MustRegister(prometheus.NewGoCollector())
	Registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
}

// ────────────────────────────────────────────────────────────────────
// Application Counters
// ────────────────────────────────────────────────────────────────────

var (
	registerOnce sync.Once

	// ClientAccessCount counts authenticated API accesses.
	ClientAccessCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "woms",
			Name:      "client_access_count",
			Help:      "Total number of authenticated API accesses.",
		},
		[]string{"method", "path"},
	)

	// OrdersSubmittedCount counts successfully created orders.
	OrdersSubmittedCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "woms",
			Name:      "orders_submitted_count",
			Help:      "Total number of orders submitted via the API.",
		},
	)

	// ScheduleJobsCreatedCount counts schedule jobs created.
	ScheduleJobsCreatedCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "woms",
			Name:      "schedule_jobs_created_count",
			Help:      "Total number of schedule jobs created.",
		},
	)

	// HTTPRequestsTotal counts all HTTP requests by method, path, and status.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "woms",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)
)

// Register registers the application-level metrics once. It is safe to
// call from multiple goroutines.
func Register() {
	registerOnce.Do(func() {
		Registry.MustRegister(ClientAccessCount)
		Registry.MustRegister(OrdersSubmittedCount)
		Registry.MustRegister(ScheduleJobsCreatedCount)
		Registry.MustRegister(HTTPRequestsTotal)
	})
}

// Handler returns an http.Handler that serves the /metrics endpoint for
// the custom Registry.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}
