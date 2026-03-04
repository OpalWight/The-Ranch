package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockPubSub is an in-memory implementation of PubSub for testing.
type mockPubSub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan string
}

func newMockPubSub() *mockPubSub {
	return &mockPubSub{
		subscribers: make(map[string][]chan string),
	}
}

func (m *mockPubSub) Publish(_ context.Context, channel string, payload string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ch := range m.subscribers[channel] {
		select {
		case ch <- payload:
		default:
			// Drop message if subscriber is slow
		}
	}
	return nil
}

func (m *mockPubSub) Subscribe(_ context.Context, channel string) (<-chan string, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan string, 64)
	m.subscribers[channel] = append(m.subscribers[channel], ch)

	cancel := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		subs := m.subscribers[channel]
		for i, sub := range subs {
			if sub == ch {
				m.subscribers[channel] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}

	return ch, cancel, nil
}

func TestMockPubSub_PublishAndSubscribe(t *testing.T) {
	ps := newMockPubSub()
	ctx := context.Background()

	msgCh, cancel, err := ps.Subscribe(ctx, "test-channel")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	payload := `{"event":"file_created","file_id":"abc-123"}`
	if err := ps.Publish(ctx, "test-channel", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-msgCh:
		if msg != payload {
			t.Errorf("got %q, want %q", msg, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestMockPubSub_MultipleSubscribers(t *testing.T) {
	ps := newMockPubSub()
	ctx := context.Background()

	ch1, cancel1, err := ps.Subscribe(ctx, "test-channel")
	if err != nil {
		t.Fatalf("Subscribe 1: %v", err)
	}
	defer cancel1()

	ch2, cancel2, err := ps.Subscribe(ctx, "test-channel")
	if err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	defer cancel2()

	payload := `{"event":"file_deleted"}`
	if err := ps.Publish(ctx, "test-channel", payload); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	for i, ch := range []<-chan string{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg != payload {
				t.Errorf("subscriber %d: got %q, want %q", i+1, msg, payload)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for message", i+1)
		}
	}
}

func TestMockPubSub_NoMessageOnDifferentChannel(t *testing.T) {
	ps := newMockPubSub()
	ctx := context.Background()

	msgCh, cancel, err := ps.Subscribe(ctx, "channel-a")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	// Publish to a different channel
	if err := ps.Publish(ctx, "channel-b", "hello"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-msgCh:
		t.Errorf("unexpected message on channel-a: %q", msg)
	case <-time.After(100 * time.Millisecond):
		// Expected — no message should arrive
	}
}

func TestMockPubSub_CancelStopsDelivery(t *testing.T) {
	ps := newMockPubSub()
	ctx := context.Background()

	msgCh, cancel, err := ps.Subscribe(ctx, "test-channel")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	cancel()

	// After cancel, the channel should be closed
	select {
	case _, ok := <-msgCh:
		if ok {
			t.Error("expected channel to be closed after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected channel to be closed immediately after cancel")
	}
}

// TestPubSubInterfaceCompliance ensures both mock and Redis implementations satisfy the interface.
func TestPubSubInterfaceCompliance(t *testing.T) {
	var _ PubSub = newMockPubSub()
	var _ PubSub = &RedisPubSub{}
}
