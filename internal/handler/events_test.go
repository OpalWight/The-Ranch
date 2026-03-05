package handler

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// testSubscriber is a mock subscriber for testing the SSE handler.
type testSubscriber struct {
	mu       sync.RWMutex
	channels map[string][]chan string
}

func newTestSubscriber() *testSubscriber {
	return &testSubscriber{
		channels: make(map[string][]chan string),
	}
}

func (s *testSubscriber) Subscribe(_ context.Context, channel string) (<-chan string, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string, 64)
	s.channels[channel] = append(s.channels[channel], ch)

	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		subs := s.channels[channel]
		for i, sub := range subs {
			if sub == ch {
				s.channels[channel] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}

	return ch, cancel, nil
}

// send pushes a message to all subscribers on a channel.
func (s *testSubscriber) send(channel, msg string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.channels[channel] {
		select {
		case ch <- msg:
		default:
		}
	}
}

func TestEventHandler_Stream_SendsSSEEvents(t *testing.T) {
	sub := newTestSubscriber()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	h := NewEventHandler(sub, logger)

	// Create a request with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	req = req.WithContext(ctx)

	// Use a pipe to read the streamed response
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Stream(rec, req)
	}()

	// Give the handler a moment to start and subscribe
	time.Sleep(50 * time.Millisecond)

	// Send a test event
	payload := `{"event":"file_created","file_id":"abc-123","name":"test.txt","timestamp":"2026-03-06T12:00:00Z"}`
	sub.send(EventsChannel, payload)

	// Give the handler time to process and write
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to stop the handler
	cancel()

	// Wait for the handler to return
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancellation")
	}

	// Parse the SSE output
	body := rec.Body.String()

	if !strings.Contains(body, ": connected") {
		t.Error("expected initial connection comment")
	}

	if !strings.Contains(body, "event: file_changed") {
		t.Error("expected 'event: file_changed' line in SSE output")
	}

	if !strings.Contains(body, "data: "+payload) {
		t.Errorf("expected data line with payload, got:\n%s", body)
	}
}

func TestEventHandler_Stream_SetsCorrectHeaders(t *testing.T) {
	sub := newTestSubscriber()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	h := NewEventHandler(sub, logger)

	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Stream(rec, req)
	}()

	// Let the handler start
	time.Sleep(50 * time.Millisecond)
	cancel()

	<-done

	headers := rec.Header()

	if ct := headers.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
	if cc := headers.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-cache")
	}
	if conn := headers.Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q, want %q", conn, "keep-alive")
	}
}

func TestEventHandler_Stream_MultipleEvents(t *testing.T) {
	sub := newTestSubscriber()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	h := NewEventHandler(sub, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Stream(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)

	events := []string{
		`{"event":"file_created","file_id":"1"}`,
		`{"event":"file_uploaded","file_id":"2"}`,
		`{"event":"file_deleted","file_id":"3"}`,
	}

	for _, evt := range events {
		sub.send(EventsChannel, evt)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))

	dataCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataCount++
		}
	}

	if dataCount != len(events) {
		t.Errorf("expected %d data lines, got %d\nfull body:\n%s", len(events), dataCount, body)
	}
}

func TestEventHandler_Stream_ClientDisconnect(t *testing.T) {
	sub := newTestSubscriber()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	h := NewEventHandler(sub, logger)

	// Create a server that serves the SSE endpoint
	server := httptest.NewServer(http.HandlerFunc(h.Stream))
	defer server.Close()

	// Connect as a client
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	// Cancel the client context — this should cause the handler to exit
	cancel()

	// Give the server a moment to clean up
	time.Sleep(100 * time.Millisecond)

	// Verify no subscribers remain on the channel
	sub.mu.RLock()
	remaining := len(sub.channels[EventsChannel])
	sub.mu.RUnlock()

	if remaining != 0 {
		t.Errorf("expected 0 remaining subscribers after disconnect, got %d", remaining)
	}
}
