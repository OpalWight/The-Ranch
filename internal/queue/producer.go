package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Producer handles enqueuing work items into the system's task stream.
type Producer struct {
	client *redis.Client
}

// NewProducer initializes a new task producer with a Redis client.
func NewProducer(client *redis.Client) *Producer {
	return &Producer{client: client}
}

// Enqueue serializes and pushes a new task to the Redis stream for processing.
func (p *Producer) Enqueue(ctx context.Context, taskType TaskType, payload map[string]string) (string, error) {
	values := map[string]interface{}{
		"type":       string(taskType),
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range payload {
		values[k] = v
	}

	id, err := p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamName,
		Values: values,
	}).Result()
	if err != nil {
		return "", fmt.Errorf("enqueue %s: %w", taskType, err)
	}
	return id, nil
}
