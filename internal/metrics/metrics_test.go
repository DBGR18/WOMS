package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// ────────────────────────────────────────────────────────────────────
// Test 1: /metrics endpoint returns valid Prometheus-format text
// ────────────────────────────────────────────────────────────────────

func TestMetricsEndpointReturnsPrometheusText(t *testing.T) {
	Register()

	handler := Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", res.Code)
	}

	body, _ := io.ReadAll(res.Body)
	text := string(body)

	// Should contain Go runtime metric families registered in init().
	if !strings.Contains(text, "go_goroutines") {
		t.Fatal("expected go runtime metrics in /metrics output")
	}

	// Initialize the labeled counter so it appears in output.
	// (CounterVec metrics are invisible until at least one label set is observed.)
	ClientAccessCount.WithLabelValues("GET", "/healthz").Add(0)

	// Re-scrape after initialization.
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	body, _ = io.ReadAll(res.Body)
	text = string(body)

	// Should contain the custom woms metrics.
	if !strings.Contains(text, "woms_client_access_count") {
		t.Fatal("expected woms_client_access_count in /metrics output")
	}
	if !strings.Contains(text, "woms_orders_submitted_count") {
		t.Fatal("expected woms_orders_submitted_count in /metrics output")
	}
}

// ────────────────────────────────────────────────────────────────────
// Test 2: Custom counters increment correctly
// ────────────────────────────────────────────────────────────────────

func TestCustomCountersIncrement(t *testing.T) {
	Register()

	// Reset counters for this test by gathering baseline.
	before := gatherCounterValue(t, "woms_orders_submitted_count")
	OrdersSubmittedCount.Inc()
	OrdersSubmittedCount.Inc()
	after := gatherCounterValue(t, "woms_orders_submitted_count")

	delta := after - before
	if delta != 2 {
		t.Fatalf("expected orders_submitted_count to increase by 2, got delta %f", delta)
	}

	// Test labeled counter.
	ClientAccessCount.WithLabelValues("GET", "/api/orders").Inc()
	accessVal := gatherLabeledCounterValue(t, "woms_client_access_count", map[string]string{
		"method": "GET",
		"path":   "/api/orders",
	})
	if accessVal < 1 {
		t.Fatalf("expected client_access_count >= 1, got %f", accessVal)
	}
}

// ────────────────────────────────────────────────────────────────────
// Test 3: Adding a new metric type is easy via Registry
// ────────────────────────────────────────────────────────────────────

func TestRegistrySupportsNewMetricTypes(t *testing.T) {
	Register()

	// Simulate a new metric type that an external package might add.
	customHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "woms",
		Name:      "test_request_duration_seconds",
		Help:      "Test request duration histogram.",
		Buckets:   prometheus.DefBuckets,
	})

	// Register should succeed without panic.
	Registry.MustRegister(customHistogram)
	t.Cleanup(func() {
		Registry.Unregister(customHistogram)
	})

	customHistogram.Observe(0.42)

	handler := Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	body, _ := io.ReadAll(res.Body)
	text := string(body)

	if !strings.Contains(text, "woms_test_request_duration_seconds") {
		t.Fatal("expected newly registered histogram in /metrics output")
	}
}

// ────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────

func gatherCounterValue(t *testing.T, name string) float64 {
	t.Helper()
	families, err := Registry.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, family := range families {
		if family.GetName() == name {
			metrics := family.GetMetric()
			if len(metrics) > 0 && metrics[0].GetCounter() != nil {
				return metrics[0].GetCounter().GetValue()
			}
		}
	}
	return 0
}

func gatherLabeledCounterValue(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := Registry.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			match := true
			for k, v := range labels {
				found := false
				for _, lp := range metric.GetLabel() {
					if lp.GetName() == k && lp.GetValue() == v {
						found = true
						break
					}
				}
				if !found {
					match = false
					break
				}
			}
			if match && metric.GetCounter() != nil {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}
