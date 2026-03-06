package queue

import "time"

const (
	// StreamName is the Redis Stream key used for the task queue.
	StreamName = "filesync:tasks"
	// GroupName is the Redis Consumer Group used for parallel processing.
	GroupName  = "filesync-workers"
)

// TaskType categorizes the work to be performed by background workers.
type TaskType string

const (
	// TaskProcessUpload triggers metadata extraction and validation.
	TaskProcessUpload    TaskType = "process_upload"
	// TaskGenerateThumbnail triggers the creation of preview images for files.
	TaskGenerateThumbnail TaskType = "generate_thumbnail"
	// TaskCleanupOrphans identifies and removes unused storage objects.
	TaskCleanupOrphans   TaskType = "cleanup_orphans"
)

// Task represents a serialized unit of work stored in Redis.
type Task struct {
	ID        string            `json:"id"`
	Type      TaskType          `json:"type"`
	Payload   map[string]string `json:"payload"`
	CreatedAt time.Time         `json:"created_at"`
}
