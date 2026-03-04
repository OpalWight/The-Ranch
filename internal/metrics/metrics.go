package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "filesync_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "filesync_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	UploadBytesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "filesync_upload_bytes_total",
			Help: "Total bytes uploaded.",
		},
	)

	ActiveSSEConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "filesync_active_sse_connections",
			Help: "Number of active SSE connections.",
		},
	)

	WorkerTasksProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "filesync_worker_tasks_processed_total",
			Help: "Total worker tasks processed.",
		},
		[]string{"type", "result"},
	)

	WorkerTaskDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "filesync_worker_task_duration_seconds",
			Help:    "Worker task processing duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		UploadBytesTotal,
		ActiveSSEConnections,
		WorkerTasksProcessed,
		WorkerTaskDuration,
	)
}

// uuidPattern matches UUID-like path segments to prevent label cardinality explosion.
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// NormalizePath replaces UUIDs in paths with {id}.
func NormalizePath(path string) string {
	return uuidPattern.ReplaceAllString(path, "{id}")
}
