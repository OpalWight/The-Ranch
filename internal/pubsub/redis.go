package pubsub

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Publisher publishes messages to a channel.
type Publisher interface {
	Publish(ctx context.Context, channel string, payload string) error
}

// Subscriber subscribes to a channel and returns a channel of messages.
// The returned function should be called to unsubscribe.
type Subscriber interface {
	Subscribe(ctx context.Context, channel string) (<-chan string, func(), error)
}

// PubSub combines both Publisher and Subscriber interfaces.
type PubSub interface {
	Publisher
	Subscriber
}

// RedisPubSub implements PubSub using Redis.
type RedisPubSub struct {
	client *redis.Client
}

// NewRedisPubSub creates a new RedisPubSub from a redis.Client.
func NewRedisPubSub(client *redis.Client) *RedisPubSub {
	return &RedisPubSub{client: client}
}

// ConnectRedis parses a Redis URL and returns a connected client.
func ConnectRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	return client, nil
}

// Publish sends a message to the specified channel.
func (r *RedisPubSub) Publish(ctx context.Context, channel string, payload string) error {
	if err := r.client.Publish(ctx, channel, payload).Err(); err != nil {
		return fmt.Errorf("publishing to %s: %w", channel, err)
	}
	return nil
}

// Subscribe listens on the specified channel and returns a channel that receives messages.
// The returned function should be called to close the subscription.
func (r *RedisPubSub) Subscribe(ctx context.Context, channel string) (<-chan string, func(), error) {
	sub := r.client.Subscribe(ctx, channel)

	// Verify the subscription is active
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, nil, fmt.Errorf("subscribing to %s: %w", channel, err)
	}

	msgCh := make(chan string, 64)

	go func() {
		defer close(msgCh)
		ch := sub.Channel()
		for msg := range ch {
			select {
			case msgCh <- msg.Payload:
			case <-ctx.Done():
				return
			}
		}
	}()

	cancel := func() {
		_ = sub.Close()
	}

	return msgCh, cancel, nil
}
