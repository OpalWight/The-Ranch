package pubsub

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Publisher defines the interface for sending messages to a specific channel.
type Publisher interface {
	Publish(ctx context.Context, channel string, payload string) error
}

// Subscriber defines the interface for receiving messages from a channel.
// The returned function is used to gracefully unsubscribe.
type Subscriber interface {
	Subscribe(ctx context.Context, channel string) (<-chan string, func(), error)
}

// PubSub combines Publisher and Subscriber for full messaging capabilities.
type PubSub interface {
	Publisher
	Subscriber
}

// RedisPubSub implements the PubSub interface using Redis as the backbone.
type RedisPubSub struct {
	client *redis.Client
}

// NewRedisPubSub initializes a new Redis-based pub/sub provider.
func NewRedisPubSub(client *redis.Client) *RedisPubSub {
	return &RedisPubSub{client: client}
}

// ConnectRedis establishes a connection to a Redis server using a URL.
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

// Publish broadcasts a message to all active subscribers of the given channel.
func (r *RedisPubSub) Publish(ctx context.Context, channel string, payload string) error {
	if err := r.client.Publish(ctx, channel, payload).Err(); err != nil {
		return fmt.Errorf("publishing to %s: %w", channel, err)
	}
	return nil
}

// Subscribe returns a channel that receives payloads from the specified Redis channel.
func (r *RedisPubSub) Subscribe(ctx context.Context, channel string) (<-chan string, func(), error) {
	sub := r.client.Subscribe(ctx, channel)

	// Ensure the subscription is successfully established.
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
