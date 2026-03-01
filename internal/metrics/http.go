package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

type HTTPMetrics struct {
	RequestDuration *prometheus.HistogramVec
	RequestsTotal   *prometheus.CounterVec
	InFlightGauge   prometheus.Gauge
}

func NewHTTPMetrics(reg *prometheus.Registry) *HTTPMetrics {
	m := &HTTPMetrics{
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "route", "status_code"},
		),
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests.",
			},
			[]string{"method", "route", "status_code"},
		),
		InFlightGauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "http_in_flight_requests",
				Help:      "Number of HTTP requests currently being processed.",
			},
		),
	}
	reg.MustRegister(m.RequestDuration, m.RequestsTotal, m.InFlightGauge)
	return m
}

func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't record metrics for the metrics endpoint itself.
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		m.InFlightGauge.Inc()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		m.InFlightGauge.Dec()

		route := "/*"
		if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
			route = rctx.RoutePattern()
		}

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}

		labels := prometheus.Labels{
			"method":      r.Method,
			"route":       route,
			"status_code": strconv.Itoa(status),
		}
		m.RequestDuration.With(labels).Observe(time.Since(start).Seconds())
		m.RequestsTotal.With(labels).Inc()
	})
}
