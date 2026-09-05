package handlers

// Standing orders (megaron_plan_staende_leverans.md) — the player-facing CRUD
// surface for the self-running caravan routes the sweep in
// internal/combat/standing_orders.go drives. Web-facing home is an
// "automation" tab in the economy view (megaron_plan_stad_vs_ekonomi.md §1: a
// flow between two settlements belongs to neither city's own drawer); this
// file only owns create/list/pause/resume/delete — the sweep itself is not
// reachable over HTTP.

import (
	"encoding/json"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StandingOrderHandler serves the standing-order CRUD surface.
type StandingOrderHandler struct {
	pool *pgxpool.Pool
}

// NewStandingOrderHandler creates a StandingOrderHandler.
func NewStandingOrderHandler(pool *pgxpool.Pool) *StandingOrderHandler {
	return &StandingOrderHandler{pool: pool}
}

type goodThreshold struct {
	GoodKey   string  `json:"good_key"`
	Threshold float64 `json:"threshold"`
}

type goodFloor struct {
	GoodKey string  `json:"good_key"`
	Floor   float64 `json:"floor"`
}

// ownSettlement verifies settlementID exists in worldID and is owned by playerID.
func (h *StandingOrderHandler) ownSettlement(r *http.Request, worldID, settlementID, playerID uuid.UUID) bool {
	var ownerID uuid.UUID
	err := h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		settlementID, worldID,
	).Scan(&ownerID)
	return err == nil && ownerID == playerID
}

// Create handles POST /worlds/:worldID/standing-orders.
// Body: { "from_settlement_id", "to_settlement_id", "crewed_by_settlement_id",
//
//	"outbound": [{"good_key","threshold"}], "return": [{"good_key","floor"}] }
func (h *StandingOrderHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		FromSettlementID   uuid.UUID       `json:"from_settlement_id"`
		ToSettlementID     uuid.UUID       `json:"to_settlement_id"`
		CrewedBySettlement uuid.UUID       `json:"crewed_by_settlement_id"`
		Outbound           []goodThreshold `json:"outbound"`
		Return             []goodFloor     `json:"return"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.FromSettlementID == req.ToSettlementID {
		writeError(w, http.StatusBadRequest, "from and to must be different settlements")
		return
	}
	if req.CrewedBySettlement != req.FromSettlementID && req.CrewedBySettlement != req.ToSettlementID {
		writeError(w, http.StatusBadRequest, "crewed_by_settlement_id must be either the source or the destination")
		return
	}
	if len(req.Outbound) == 0 {
		writeError(w, http.StatusBadRequest, "outbound must name at least one good and threshold")
		return
	}
	// Egen→egen only (CLAUDE.md trade-lagret punkt 3, plan §6) — both ends must
	// belong to the requesting Wanax, same rule api/handlers/province.go's
	// Trade handler enforces for a one-off transfer.
	if !h.ownSettlement(r, worldID, req.FromSettlementID, playerID) ||
		!h.ownSettlement(r, worldID, req.ToSettlementID, playerID) {
		writeError(w, http.StatusForbidden, "both settlements must be your own")
		return
	}
	for _, g := range req.Outbound {
		if _, shippable, err := economy.IsShippableGood(r.Context(), h.pool, g.GoodKey); err != nil || !shippable {
			writeError(w, http.StatusBadRequest, "unknown or unshippable outbound good: "+g.GoodKey)
			return
		}
	}
	for _, g := range req.Return {
		if _, shippable, err := economy.IsShippableGood(r.Context(), h.pool, g.GoodKey); err != nil || !shippable {
			writeError(w, http.StatusBadRequest, "unknown or unshippable return good: "+g.GoodKey)
			return
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	var orderID uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO standing_orders (world_id, owner_id, from_settlement_id, to_settlement_id, crewed_by_settlement_id)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		worldID, playerID, req.FromSettlementID, req.ToSettlementID, req.CrewedBySettlement,
	).Scan(&orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create standing order")
		return
	}
	for _, g := range req.Outbound {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO standing_order_outbound_goods (standing_order_id, good_key, threshold) VALUES ($1,$2,$3)`,
			orderID, g.GoodKey, g.Threshold,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save outbound goods")
			return
		}
	}
	for _, g := range req.Return {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO standing_order_return_goods (standing_order_id, good_key, floor) VALUES ($1,$2,$3)`,
			orderID, g.GoodKey, g.Floor,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save return goods")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": orderID, "status": "active"})
}

type standingOrderOut struct {
	ID          uuid.UUID       `json:"id"`
	FromID      uuid.UUID       `json:"from_settlement_id"`
	FromName    string          `json:"from_name"`
	ToID        uuid.UUID       `json:"to_settlement_id"`
	ToName      string          `json:"to_name"`
	CrewedByID  uuid.UUID       `json:"crewed_by_settlement_id"`
	Status      string          `json:"status"`
	PauseReason *string         `json:"pause_reason,omitempty"`
	Outbound    []goodThreshold `json:"outbound"`
	Return      []goodFloor     `json:"return"`
}

// List handles GET /worlds/:worldID/standing-orders — every route the
// requesting Wanax owns (as either end — a colony crewed_by itself still
// belongs to the same owner as everything else, since egen→egen requires
// both ends owned by the same player).
func (h *StandingOrderHandler) List(w http.ResponseWriter, r *http.Request) {
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
		`SELECT so.id, so.from_settlement_id, sf.name, so.to_settlement_id, st.name,
		        so.crewed_by_settlement_id, so.status, so.pause_reason
		 FROM standing_orders so
		 JOIN settlements sf ON sf.id = so.from_settlement_id
		 JOIN settlements st ON st.id = so.to_settlement_id
		 WHERE so.world_id = $1 AND so.owner_id = $2
		 ORDER BY so.created_at`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load standing orders")
		return
	}
	var out []standingOrderOut
	for rows.Next() {
		var o standingOrderOut
		if err := rows.Scan(&o.ID, &o.FromID, &o.FromName, &o.ToID, &o.ToName,
			&o.CrewedByID, &o.Status, &o.PauseReason); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "could not read standing order")
			return
		}
		out = append(out, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read standing orders")
		return
	}

	for i := range out {
		out[i].Outbound = h.loadOutbound(r, out[i].ID)
		out[i].Return = h.loadReturn(r, out[i].ID)
	}
	if out == nil {
		out = []standingOrderOut{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *StandingOrderHandler) loadOutbound(r *http.Request, orderID uuid.UUID) []goodThreshold {
	rows, err := h.pool.Query(r.Context(),
		`SELECT good_key, threshold FROM standing_order_outbound_goods WHERE standing_order_id = $1`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []goodThreshold
	for rows.Next() {
		var g goodThreshold
		if rows.Scan(&g.GoodKey, &g.Threshold) == nil {
			out = append(out, g)
		}
	}
	return out
}

func (h *StandingOrderHandler) loadReturn(r *http.Request, orderID uuid.UUID) []goodFloor {
	rows, err := h.pool.Query(r.Context(),
		`SELECT good_key, floor FROM standing_order_return_goods WHERE standing_order_id = $1`, orderID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []goodFloor
	for rows.Next() {
		var g goodFloor
		if rows.Scan(&g.GoodKey, &g.Floor) == nil {
			out = append(out, g)
		}
	}
	return out
}

// ownOrder verifies the standing order exists in worldID and belongs to playerID.
func (h *StandingOrderHandler) ownOrder(r *http.Request, worldID, orderID, playerID uuid.UUID) bool {
	var ownerID uuid.UUID
	err := h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM standing_orders WHERE id = $1 AND world_id = $2`,
		orderID, worldID,
	).Scan(&ownerID)
	return err == nil && ownerID == playerID
}

// Pause handles POST /worlds/:worldID/standing-orders/:orderID/pause — a
// player-initiated pause (status/pause_reason share the same column the
// sweep itself writes to when it can't find a spendable surplus or a spare
// gubbe; a manual pause just supplies its own reason).
func (h *StandingOrderHandler) Pause(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "paused", stringPtr("paused by Wanax"))
}

// Resume handles POST /worlds/:worldID/standing-orders/:orderID/resume.
func (h *StandingOrderHandler) Resume(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, "active", nil)
}

func stringPtr(s string) *string { return &s }

func (h *StandingOrderHandler) setStatus(w http.ResponseWriter, r *http.Request, status string, reason *string) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	orderID, err := uuid.Parse(chi.URLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.ownOrder(r, worldID, orderID, playerID) {
		writeError(w, http.StatusForbidden, "not your standing order")
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE standing_orders SET status = $1, pause_reason = $2 WHERE id = $3`,
		status, reason, orderID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update standing order")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": orderID, "status": status})
}

// Delete handles DELETE /worlds/:worldID/standing-orders/:orderID. A caravan
// already in flight is untouched (transports.standing_order_id ON DELETE SET
// NULL, migration 140) — it keeps flying as an ordinary untagged transfer.
func (h *StandingOrderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	orderID, err := uuid.Parse(chi.URLParam(r, "orderID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.ownOrder(r, worldID, orderID, playerID) {
		writeError(w, http.StatusForbidden, "not your standing order")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `DELETE FROM standing_orders WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete standing order")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
