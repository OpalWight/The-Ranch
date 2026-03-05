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

// EventHandler handles SSE connections for real-time file change events.
type EventHandler struct {
	subscriber pubsub.Subscriber
	logger     *slog.Logger
}

// NewEventHandler creates a new EventHandler with the given subscriber and logger.
func NewEventHandler(sub pubsub.Subscriber, logger *slog.Logger) *EventHandler {
	return &EventHandler{subscriber: sub, logger: logger}
}

// Stream handles GET /api/v1/events/stream — an SSE endpoint that pushes
// file change events to connected clients in real time.
func (h *EventHandler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe to file change events
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

	// Send an initial comment to confirm the connection is live
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			h.logger.Info("SSE client disconnected", "remote", r.RemoteAddr)
			return
		case msg, ok := <-msgCh:
			if !ok {
				// Channel closed — subscription ended
				h.logger.Info("SSE subscription channel closed", "remote", r.RemoteAddr)
				return
			}
			fmt.Fprintf(w, "event: file_changed\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
