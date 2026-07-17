// Package notify provides a lightweight WebSocket broadcast hub.
// One Hub per process; worlds are isolated by ID so a message to
// world A never reaches a client connected to world B.
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebSocket liveness timings. These are TRANSPORT deadlines: the OS netpoller
// compares them against the real monotonic/wall clock, so they use the time
// package directly and NOT clock.Clock. The pause-aware game clock must never
// drive a socket deadline — its accumulated pause offset would push the deadline
// into the past after any absorbed downtime and kill every connection instantly.
// Time here is plumbing, not game logic (same reasoning as auth token expiry in
// internal/auth/service.go).
const (
	// pingPeriod — how often the write pump emits a protocol ping + app heartbeat.
	// Chosen well under Cloudflare's ~100 s idle-WS cap and common proxy defaults.
	pingPeriod = 25 * time.Second
	// pongWait — the read side gives up if no pong (or any frame) arrives within
	// this. Must exceed pingPeriod so a healthy peer always refreshes it first.
	pongWait = 60 * time.Second
	// writeWait — max time a single write may block before the peer is dead.
	writeWait = 10 * time.Second
	// readLimit — we never expect large client→server frames; cap to bound memory.
	readLimit = 512
)

// heartbeatFrame is the app-level liveness message. Browser JS cannot observe
// protocol pings, so this is the client's only way to measure liveness (its
// watchdog refreshes on any inbound frame; the onmessage if-chain ignores it).
var heartbeatFrame, _ = json.Marshal(Msg{Kind: "Heartbeat"})

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

// unregister removes c from the hub and tears its connection down exactly once.
// Ordering is load-bearing: delete-then-close under the same lock guarantees no
// in-flight Broadcast can send on a closed channel — Broadcast holds RLock while
// iterating, so it either sees c with an open send channel or does not see c at
// all. Idempotent via the membership check: both pumps call it on exit, and
// whichever loses the race is a no-op.
func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	_, ok := h.clients[c]
	if ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
	if ok {
		c.conn.Close() // unblocks the other pump's in-flight Read/Write
	}
}

// Register adds a new WebSocket client and runs its read pump, blocking until
// the connection is closed. A companion write pump goroutine drains broadcasts
// and emits periodic pings + heartbeats. Either pump exiting calls unregister,
// which closes the connection and so unblocks the other pump — no goroutine
// leak, no double close, no send on a closed channel.
func (h *Hub) Register(conn *websocket.Conn, worldID uuid.UUID) {
	c := &client{conn: conn, worldID: worldID, send: make(chan []byte, 32), hub: h}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	defer h.unregister(c) // covers normal read-pump exit and any panic

	// Write pump — the ONLY goroutine that writes to conn (gorilla requires a
	// single concurrent writer), so broadcasts, pings and heartbeats serialise here.
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case raw, ok := <-c.send:
				if !ok {
					return // read pump tore the client down
				}
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
					h.unregister(c)
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					h.unregister(c)
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.TextMessage, heartbeatFrame); err != nil {
					h.unregister(c)
					return
				}
			}
		}
	}()

	// Read pump — detects disconnect and keeps the read deadline fresh. A browser
	// auto-pongs our pings, so the pong handler is what keeps a healthy connection
	// alive; silence past pongWait ⇒ dead peer and ReadMessage errors out.
	conn.SetReadLimit(readLimit)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
