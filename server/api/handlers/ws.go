package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/poleia/server/internal/notify"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  256,
	WriteBufferSize: 4096,
	CheckOrigin:    func(r *http.Request) bool { return true },
}

// WSHandler handles WebSocket upgrade requests.
type WSHandler struct{ hub *notify.Hub }

// NewWSHandler creates a WSHandler.
func NewWSHandler(hub *notify.Hub) *WSHandler { return &WSHandler{hub: hub} }

// Connect handles GET /ws/:worldID — upgrades to WebSocket and blocks until closed.
func (h *WSHandler) Connect(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		http.Error(w, "invalid world ID", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// The http.Server's ReadTimeout/WriteTimeout set short deadlines on the raw
	// connection for a normal request/response. A WebSocket is long-lived, so
	// those deadlines would kill it after a few seconds (→ 504 at the proxy, no
	// realtime push reaching the client). Clear them on the hijacked connection —
	// this exempts only the WS; every normal HTTP route keeps its timeouts. The
	// hub manages the connection's lifetime instead.
	if nc := conn.NetConn(); nc != nil {
		_ = nc.SetDeadline(time.Time{})
	}
	h.hub.Register(conn, worldID) // blocks until disconnect
}
