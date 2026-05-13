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

	// CurrentOnlineUserCount tracks the number of currently online users.
	CurrentOnlineUserCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "woms",
			Name:      "current_online_user_count",
			Help:      "Current number of online users.",
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
		Registry.MustRegister(CurrentOnlineUserCount)
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
