package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Producer enqueues tasks into a Redis Stream.
type Producer struct {
	client *redis.Client
}

// NewProducer creates a Producer from an existing redis.Client.
func NewProducer(client *redis.Client) *Producer {
	return &Producer{client: client}
}

// Enqueue adds a task to the stream. Returns the stream entry ID.
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
