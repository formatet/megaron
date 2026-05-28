// Package notify provides a lightweight WebSocket broadcast hub.
// One Hub per process; worlds are isolated by ID so a message to
// world A never reaches a client connected to world B.
package notify

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Msg is a JSON-encodable push message sent to all clients in a world.
type Msg struct {
	Kind    string `json:"kind"`    // e.g. "ArmyArrival", "BuildComplete"
	WorldID string `json:"world_id"`
	Payload any    `json:"payload"`
}

type client struct {
	conn    *websocket.Conn
	worldID uuid.UUID
	send    chan []byte
	hub     *Hub
}

// Hub routes broadcast messages to all WebSocket clients subscribed to a world.
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

// New creates a Hub ready for use.
func New() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// Broadcast sends msg to every client connected to worldID.
// Safe to call from any goroutine. Non-blocking; slow clients are dropped.
func (h *Hub) Broadcast(worldID uuid.UUID, msg Msg) {
	msg.WorldID = worldID.String()
	raw, err := json.Marshal(msg)
	if err != nil {
		slog.Error("notify marshal", "err", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.worldID != worldID {
			continue
		}
		select {
		case c.send <- raw:
		default:
			// Slow client — drop the message, not the connection.
		}
	}
}

// Register adds a new WebSocket client to the hub and starts its write pump.
// Blocks until the connection is closed.
func (h *Hub) Register(conn *websocket.Conn, worldID uuid.UUID) {
	c := &client{conn: conn, worldID: worldID, send: make(chan []byte, 32), hub: h}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		conn.Close()
	}()

	// Write pump — drain send channel to the WebSocket.
	go func() {
		for raw := range c.send {
			if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
				return
			}
		}
	}()

	// Read pump — keep connection alive, detect client disconnect.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			close(c.send)
			return
		}
	}
}
