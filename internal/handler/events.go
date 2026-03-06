package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/albertvo/the-ranch/internal/metrics"
	"github.com/albertvo/the-ranch/internal/pubsub"
)

// EventsChannel is the Redis Pub/Sub channel for file change events.
const EventsChannel = "filesync:events"

// EventHandler manages real-time event streaming to clients via Server-Sent Events (SSE).
type EventHandler struct {
	subscriber pubsub.Subscriber
	logger     *slog.Logger
}

// NewEventHandler initializes an EventHandler with a subscriber and logger.
func NewEventHandler(sub pubsub.Subscriber, logger *slog.Logger) *EventHandler {
	return &EventHandler{subscriber: sub, logger: logger}
}

// Stream upgrades a connection to SSE and pushes file change events from Redis.
func (h *EventHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// Set required headers for long-lived SSE connections.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Start listening for messages on the events channel.
	msgCh, cancel, err := h.subscriber.Subscribe(r.Context(), EventsChannel)
	if err != nil {
		h.logger.Error("subscribing to events", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to subscribe"})
		return
	}
	defer cancel()

	metrics.ActiveSSEConnections.Inc()
	defer metrics.ActiveSSEConnections.Dec()

	h.logger.Info("SSE client connected", "remote", r.RemoteAddr)

	// Keep-alive: send an initial comment.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			h.logger.Info("SSE client disconnected", "remote", r.RemoteAddr)
			return
		case msg, ok := <-msgCh:
			if !ok {
				h.logger.Info("SSE subscription channel closed", "remote", r.RemoteAddr)
				return
			}
			// Push event data to the client.
			fmt.Fprintf(w, "event: file_changed\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
