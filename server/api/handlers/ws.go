package handlers

import (
	"net/http"

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
	h.hub.Register(conn, worldID) // blocks until disconnect
}
