package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Consumer manages reading and acknowledging tasks from a Redis Consumer Group.
type Consumer struct {
	client       *redis.Client
	consumerName string
}

// NewConsumer initializes a consumer with a unique name for reliable tracking.
func NewConsumer(client *redis.Client, consumerName string) *Consumer {
	return &Consumer{client: client, consumerName: consumerName}
}

// EnsureGroup creates the consumer group in Redis if it hasn't been initialized.
func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, StreamName, GroupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("creating consumer group: %w", err)
	}
	return nil
}

// Read waits for and retrieves new tasks assigned to this consumer.
func (c *Consumer) Read(ctx context.Context, count int64, blockTime time.Duration) ([]Task, error) {
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    GroupName,
		Consumer: c.consumerName,
		Streams:  []string{StreamName, ">"},
		Count:    count,
		Block:    blockTime,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("reading stream: %w", err)
	}

	var tasks []Task
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			t := Task{
				ID:      msg.ID,
				Payload: make(map[string]string),
			}
			for k, v := range msg.Values {
				s, _ := v.(string)
				switch k {
				case "type":
					t.Type = TaskType(s)
				case "created_at":
					t.CreatedAt, _ = time.Parse(time.RFC3339, s)
				default:
					t.Payload[k] = s
				}
			}
			tasks = append(tasks, t)
		}
	}
	return tasks, nil
}

// Ack confirms successful task completion to prevent re-delivery.
func (c *Consumer) Ack(ctx context.Context, id string) error {
	return c.client.XAck(ctx, StreamName, GroupName, id).Err()
}
