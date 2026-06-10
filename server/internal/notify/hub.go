// Package notify provides a lightweight WebSocket broadcast hub.
// One Hub per process; worlds are isolated by ID so a message to
// world A never reaches a client connected to world B.
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool    *pgxpool.Pool
}

// New creates a Hub ready for use.
func New() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// SetPool wires a database pool for notification persistence.
// Call once at startup before any events are processed.
func (h *Hub) SetPool(pool *pgxpool.Pool) { h.pool = pool }

// BroadcastEvent is a convenience wrapper used by internal packages that must not
// import notify directly. It satisfies economy.Broadcaster and combat.Broadcaster.
func (h *Hub) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {
	h.Broadcast(worldID, Msg{Kind: kind, Payload: payload})
}

// NotifyPlayer persists a notification for a specific player and broadcasts
// the event to all clients in the world. If playerID is uuid.Nil or no pool
// is configured, only broadcasts.
func (h *Hub) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	if h.pool != nil && playerID != uuid.Nil {
		bodyJSON, err := json.Marshal(payload)
		if err == nil {
			if _, dbErr := h.pool.Exec(ctx,
				`INSERT INTO notifications (world_id, player_id, kind, level, body_json)
				 VALUES ($1, $2, $3, $4, $5)`,
				worldID, playerID, kind, level, bodyJSON,
			); dbErr != nil {
				slog.Error("persist notification", "kind", kind, "err", dbErr)
			}
		}
	}
	h.BroadcastEvent(worldID, kind, payload)
	return nil
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
