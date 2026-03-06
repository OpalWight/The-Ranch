package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// HTTPRequestsTotal tracks the cumulative number of HTTP requests by method, path, and status.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "filesync_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration tracks how long requests take to complete.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "filesync_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// UploadBytesTotal tracks the volume of file data successfully uploaded.
	UploadBytesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "filesync_upload_bytes_total",
			Help: "Total bytes uploaded.",
		},
	)

	// ActiveSSEConnections monitors currently open Server-Sent Events streams.
	ActiveSSEConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "filesync_active_sse_connections",
			Help: "Number of active SSE connections.",
		},
	)

	// WorkerTasksProcessed tracks background job execution counts.
	WorkerTasksProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "filesync_worker_tasks_processed_total",
			Help: "Total worker tasks processed.",
		},
		[]string{"type", "result"},
	)

	// WorkerTaskDuration tracks how long background jobs take to process.
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
	// Automatically register metrics on package initialization.
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		UploadBytesTotal,
		ActiveSSEConnections,
		WorkerTasksProcessed,
		WorkerTaskDuration,
	)
}

// uuidPattern matches standard UUID formats to group metrics by resource type rather than specific ID.
var uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// NormalizePath replaces dynamic path IDs with a generic placeholder to prevent metric explosion.
func NormalizePath(path string) string {
	return uuidPattern.ReplaceAllString(path, "{id}")
}
