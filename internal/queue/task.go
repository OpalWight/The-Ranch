package queue

import "time"

const (
	StreamName = "filesync:tasks"
	GroupName  = "filesync-workers"
)

// TaskType identifies the kind of work to perform.
type TaskType string

const (
	TaskProcessUpload    TaskType = "process_upload"
	TaskGenerateThumbnail TaskType = "generate_thumbnail"
	TaskCleanupOrphans   TaskType = "cleanup_orphans"
)

// Task represents a unit of work enqueued via Redis Streams.
type Task struct {
	ID        string            `json:"id"`
	Type      TaskType          `json:"type"`
	Payload   map[string]string `json:"payload"`
	CreatedAt time.Time         `json:"created_at"`
}
