package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type SSEEvent struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type SSEBus struct {
	mu   sync.RWMutex
	subs map[chan SSEEvent]struct{}
}

func NewSSEBus() *SSEBus {
	return &SSEBus{
		subs: make(map[chan SSEEvent]struct{}),
	}
}

func (b *SSEBus) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 100)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *SSEBus) Unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	// drain channel
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func (b *SSEBus) Broadcast(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- event:
		default: // slow client, drop
		}
	}
}

// ServeSSE serves an SSE endpoint. It blocks until client disconnects.
func (b *SSEBus) ServeSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Send initial ping
	fmt.Fprintf(w, "data: {\"type\":\"ping\"}\n\n")
	flusher.Flush()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
