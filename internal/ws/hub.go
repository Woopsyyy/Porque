// Package ws provides a topic-based WebSocket fan-out hub with per-connection
// backpressure handling, plus helpers for streaming container logs.
package ws

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// sendBuffer bounds per-connection queued messages; overflow => disconnect.
	sendBuffer = 64
	// writeWait is the deadline for a single write before the client is dropped.
	writeWait = 10 * time.Second
)

// Hub broadcasts messages to subscribers grouped by topic (typically a
// server id). Slow clients are disconnected rather than allowed to block.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]map[*conn]struct{}
}

// NewHub creates an empty hub.
func NewHub() *Hub {
	return &Hub{topics: make(map[string]map[*conn]struct{})}
}

// conn never closes its send channel; termination is signalled via done so
// publishers can safely select without risking a send on a closed channel.
type conn struct {
	ws   *websocket.Conn
	send chan []byte
	done chan struct{}
	once sync.Once
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

// PublishStatus marshals payload to JSON and fans it out to topic subscribers.
// It satisfies mcserver.StatusPublisher.
func (h *Hub) PublishStatus(topic string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.publish(topic, b)
}

func (h *Hub) publish(topic string, msg []byte) {
	h.mu.RLock()
	subs := h.topics[topic]
	targets := make([]*conn, 0, len(subs))
	for c := range subs {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- msg:
		case <-c.done:
			// Connection already closing; nothing to do.
		default:
			// Buffer full: client is too slow, drop it.
			h.remove(topic, c)
			c.close()
		}
	}
}

// Subscribe registers a WebSocket connection to a topic and runs its write
// pump until the connection closes. Blocks for the lifetime of the connection.
func (h *Hub) Subscribe(topic string, wsc *websocket.Conn) {
	c := &conn{ws: wsc, send: make(chan []byte, sendBuffer), done: make(chan struct{})}

	h.mu.Lock()
	if h.topics[topic] == nil {
		h.topics[topic] = make(map[*conn]struct{})
	}
	h.topics[topic][c] = struct{}{}
	h.mu.Unlock()

	// Reader: discard client messages but detect close.
	go func() {
		defer func() { h.remove(topic, c); c.close() }()
		for {
			if _, _, err := wsc.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case msg := <-c.send:
			_ = wsc.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsc.WriteMessage(websocket.TextMessage, msg); err != nil {
				h.remove(topic, c)
				c.close()
				return
			}
		case <-c.done:
			h.remove(topic, c)
			return
		}
	}
}

func (h *Hub) remove(topic string, c *conn) {
	h.mu.Lock()
	if subs := h.topics[topic]; subs != nil {
		delete(subs, c)
		if len(subs) == 0 {
			delete(h.topics, topic)
		}
	}
	h.mu.Unlock()
}
