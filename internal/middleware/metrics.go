package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/albertvo/the-ranch/internal/metrics"
)

// Metrics returns middleware that exports HTTP request metrics to Prometheus.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		path := metrics.NormalizePath(r.URL.Path)
		status := strconv.Itoa(wrapped.statusCode)
		duration := time.Since(start).Seconds()

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}
