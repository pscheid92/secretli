package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
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

func (m *HTTPMetrics) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Don't record metrics for the metrics endpoint itself.
			if c.Request().URL.Path == "/metrics" {
				return next(c)
			}

			start := time.Now()
			m.InFlightGauge.Inc()

			err := next(c)

			m.InFlightGauge.Dec()

			route := c.Path()
			if route == "" {
				route = "/*"
			}

			status := c.Response().Status
			if status == 0 {
				status = http.StatusOK
			}

			labels := prometheus.Labels{
				"method":      c.Request().Method,
				"route":       route,
				"status_code": strconv.Itoa(status),
			}
			m.RequestDuration.With(labels).Observe(time.Since(start).Seconds())
			m.RequestsTotal.With(labels).Inc()

			return err
		}
	}
}
