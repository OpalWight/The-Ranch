package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// wrappedWriter captures the HTTP status code for logging purposes.
type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader overrides the default implementation to store the status code.
func (w *wrappedWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Logging returns middleware that records incoming request details and execution time.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
