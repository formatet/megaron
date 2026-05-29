package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
)

// MessengerHandler handles HTTP requests for messenger endpoints.
type MessengerHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
}

// NewMessengerHandler creates a MessengerHandler.
func NewMessengerHandler(pool *pgxpool.Pool, sched *events.Scheduler, clk clock.Clock) *MessengerHandler {
	return &MessengerHandler{pool: pool, scheduler: sched, clk: clk}
}

// Send handles POST /worlds/:worldID/settlements/:settlementID/messengers.
// Creates a messenger travelling from the caller's settlement to the destination.
func (h *MessengerHandler) Send(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	originID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		DestinationID string `json:"destination_id"`
		Message       string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	destID, err := uuid.Parse(req.DestinationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid destination ID")
		return
	}
	if originID == destID {
		writeError(w, http.StatusBadRequest, "cannot send messenger to own settlement")
		return
	}
	if len(req.Message) == 0 || len(req.Message) > 1000 {
		writeError(w, http.StatusBadRequest, "message must be 1–1000 characters")
		return
	}

	// Verify caller owns the origin settlement.
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		originID, worldID,
	).Scan(&ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Look up province hex coords for distance calculation.
	var oQ, oR int
	err = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p
		 JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		originID,
	).Scan(&oQ, &oR)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not find origin province")
		return
	}

	var dQ, dR int
	err = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p
		 JOIN settlements s ON s.province_id = p.id WHERE s.id = $1 AND s.world_id = $2`,
		destID, worldID,
	).Scan(&dQ, &dR)
	if err != nil {
		writeError(w, http.StatusNotFound, "destination settlement not found in this world")
		return
	}

	dist := province.HexDistance(province.MapPosition{Q: oQ, R: oR}, province.MapPosition{Q: dQ, R: dR})
	if dist == 0 {
		writeError(w, http.StatusBadRequest, "settlements are on the same province")
		return
	}
	arrivesAt := h.clk.Now().Add(time.Duration(dist) * time.Hour)

	var messengerID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, hex_q, hex_r, arrives_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		worldID, playerID, originID, destID, req.Message, dQ, dR, arrivesAt,
	).Scan(&messengerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create messenger")
		return
	}

	_ = h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledMessengerArrival,
		messenger.ArrivalPayload{MessengerID: messengerID}, arrivesAt)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         messengerID,
		"arrives_at": arrivesAt,
		"distance":   dist,
	})
}

// ListSent handles GET /worlds/:worldID/settlements/:settlementID/messengers.
// Returns the last 20 messengers sent from this settlement (owner only).
func (h *MessengerHandler) ListSent(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	originID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var ownerID *uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		originID, worldID,
	).Scan(&ownerID)
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, m.destination_id, s.name, m.status, m.reply_text, m.sent_at, m.arrives_at
		 FROM messengers m
		 JOIN settlements s ON s.id = m.destination_id
		 WHERE m.origin_id = $1
		 ORDER BY m.sent_at DESC LIMIT 20`,
		originID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load messengers")
		return
	}
	defer rows.Close()

	type item struct {
		ID          uuid.UUID  `json:"id"`
		DestID      uuid.UUID  `json:"destination_id"`
		DestName    string     `json:"destination_name"`
		Status      string     `json:"status"`
		ReplyText   *string    `json:"reply_text"`
		SentAt      time.Time  `json:"sent_at"`
		ArrivesAt   time.Time  `json:"arrives_at"`
	}
	var result []item
	for rows.Next() {
		var m item
		if err := rows.Scan(&m.ID, &m.DestID, &m.DestName, &m.Status, &m.ReplyText,
			&m.SentAt, &m.ArrivesAt); err == nil {
			result = append(result, m)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Inbox handles GET /worlds/:worldID/messengers/inbox.
// Returns all messengers delivered to any of the caller's settlements.
func (h *MessengerHandler) Inbox(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, m.origin_id, os.name, m.destination_id, ds.name, m.message_text, m.status, m.sent_at, m.arrives_at
		 FROM messengers m
		 JOIN settlements os ON os.id = m.origin_id
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.world_id = $1
		   AND ds.owner_id = $2
		   AND m.status = 'delivered'
		 ORDER BY m.arrives_at DESC LIMIT 30`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load inbox")
		return
	}
	defer rows.Close()

	type item struct {
		ID       uuid.UUID `json:"id"`
		FromID   uuid.UUID `json:"from_id"`
		FromName string    `json:"from_name"`
		ToID     uuid.UUID `json:"to_id"`
		ToName   string    `json:"to_name"`
		Message  string    `json:"message"`
		Status   string    `json:"status"`
		SentAt   time.Time `json:"sent_at"`
		ArrivedAt time.Time `json:"arrived_at"`
	}
	var result []item
	for rows.Next() {
		var m item
		if err := rows.Scan(&m.ID, &m.FromID, &m.FromName, &m.ToID, &m.ToName,
			&m.Message, &m.Status, &m.SentAt, &m.ArrivedAt); err == nil {
			result = append(result, m)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Reply handles POST /worlds/:worldID/messengers/:messengerID/reply.
// Sets the reply text, flips status to 'returning', and schedules the return trip.
func (h *MessengerHandler) Reply(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	messengerID, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Reply) > 1000 {
		writeError(w, http.StatusBadRequest, "reply must be at most 1000 characters")
		return
	}

	// Messenger must be delivered to one of the caller's settlements.
	var destID, originID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.destination_id, m.origin_id FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.id = $1 AND m.world_id = $2 AND ds.owner_id = $3 AND m.status = 'delivered'`,
		messengerID, worldID, playerID,
	).Scan(&destID, &originID)
	if err != nil {
		writeError(w, http.StatusForbidden, "messenger not found, not yours, or not yet arrived")
		return
	}

	// Calculate return trip distance.
	var dQ, dR, oQ, oR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		destID,
	).Scan(&dQ, &dR)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		originID,
	).Scan(&oQ, &oR)
	dist := province.HexDistance(province.MapPosition{Q: dQ, R: dR}, province.MapPosition{Q: oQ, R: oR})
	returnsAt := h.clk.Now().Add(time.Duration(dist) * time.Hour)

	_, err = h.pool.Exec(r.Context(),
		`UPDATE messengers SET reply_text = $1, status = 'returning' WHERE id = $2`,
		req.Reply, messengerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save reply")
		return
	}

	// Schedule return. The auto-return (48h from delivery) is harmless — ReturnHandler is idempotent.
	_ = h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledMessengerReturn,
		messenger.ReturnPayload{MessengerID: messengerID}, returnsAt)

	writeJSON(w, http.StatusOK, map[string]any{
		"returns_at": returnsAt,
		"distance":   dist,
	})
}
