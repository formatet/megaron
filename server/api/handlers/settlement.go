package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/loyalty"
)

// SettlementHandler handles HTTP requests for settlement endpoints.
type SettlementHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
}

// NewSettlementHandler creates a SettlementHandler.
func NewSettlementHandler(pool *pgxpool.Pool, store *events.Store) *SettlementHandler {
	return &SettlementHandler{pool: pool, eventStore: store}
}

// List handles GET /worlds/:worldID/settlements — returns the caller's settlements.
func (h *SettlementHandler) List(w http.ResponseWriter, r *http.Request) {
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
		`SELECT id, province_id, name, culture_id, control_type, loyalty, loyalty_trend,
		        wall_level, is_capital, state, population, updated_at
		 FROM settlements
		 WHERE world_id = $1 AND owner_id = $2
		 ORDER BY is_capital DESC, name`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settlements")
		return
	}
	defer rows.Close()

	type item struct {
		ID           uuid.UUID `json:"id"`
		ProvinceID   uuid.UUID `json:"province_id"`
		Name         string    `json:"name"`
		Culture      string    `json:"culture"`
		ControlType  string    `json:"control_type"`
		Loyalty      int       `json:"loyalty"`
		LoyaltyTrend string    `json:"loyalty_trend"`
		WallLevel    int       `json:"wall_level"`
		IsCapital    bool      `json:"is_capital"`
		State        string    `json:"state"`
		Population   int       `json:"population"`
		UpdatedAt    time.Time `json:"updated_at"`
	}
	var result []item
	for rows.Next() {
		var s item
		if err := rows.Scan(&s.ID, &s.ProvinceID, &s.Name, &s.Culture, &s.ControlType,
			&s.Loyalty, &s.LoyaltyTrend, &s.WallLevel, &s.IsCapital, &s.State,
			&s.Population, &s.UpdatedAt); err == nil {
			result = append(result, s)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /worlds/:worldID/settlements/:settlementID.
func (h *SettlementHandler) Get(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	settlementID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	sett, err := loadSettlement(r.Context(), h.pool, settlementID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "settlement not found")
		return
	}

	now := time.Now()
	resp := map[string]any{
		"id":            sett.ID,
		"province_id":   sett.ProvinceID,
		"name":          sett.Name,
		"culture":       sett.CultureID,
		"control_type":  sett.ControlType,
		"loyalty":       sett.Loyalty,
		"loyalty_trend": sett.LoyaltyTrend,
		"wall_level":    sett.WallLevel,
		"is_capital":    sett.IsCapital,
		"state":         sett.State,
		"population":    sett.Population,
		"resources":     sett.Resources.Snapshot(now),
		"army":          sett.Army,
		"updated_at":    sett.UpdatedAt,
	}

	// Only owner sees the full resources; others see limited info.
	if sett.OwnerID == nil || *sett.OwnerID != playerID {
		delete(resp, "resources")
		delete(resp, "army")
		resp["owner_id"] = sett.OwnerID
	}

	writeJSON(w, http.StatusOK, resp)
}

// Gift handles POST /worlds/:worldID/settlements/:settlementID/gift.
// The caller sends gold and food from their capital to a target colony to boost loyalty.
func (h *SettlementHandler) Gift(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
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
		Gold float64 `json:"gold"`
		Food float64 `json:"food"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Gold < 0 || req.Food < 0 || (req.Gold == 0 && req.Food == 0) {
		writeError(w, http.StatusBadRequest, "gift must include gold or food")
		return
	}

	// Verify target is owned by caller.
	var targetOwner *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		targetID, worldID,
	).Scan(&targetOwner)
	if err != nil || targetOwner == nil || *targetOwner != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Find caller's capital.
	var sourceID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, playerID,
	).Scan(&sourceID)
	if err != nil {
		writeError(w, http.StatusForbidden, "no capital to send gift from")
		return
	}

	// Deduct from source.
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE settlements SET
		   gold_amount = gold_amount
		     + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60 * gold_rate) - $1,
		   gold_calc_at = now(),
		   food_amount = food_amount
		     + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60 * food_rate) - $2,
		   food_calc_at = now()
		 WHERE id = $3
		   AND gold_amount + (EXTRACT(EPOCH FROM (now() - gold_calc_at))/60 * gold_rate) >= $1
		   AND food_amount + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60 * food_rate) >= $2`,
		req.Gold, req.Food, sourceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct resources")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "insufficient resources")
		return
	}

	// Add to target.
	_, _ = h.pool.Exec(r.Context(),
		`UPDATE settlements SET
		   gold_amount = gold_amount + $1,
		   food_amount = food_amount + $2
		 WHERE id = $3`,
		req.Gold, req.Food, targetID,
	)

	// Apply loyalty event — significant gift (50+ gold equivalent) gives +1 loyalty.
	loyaltyDelta := 0
	if req.Gold+req.Food*0.5 >= 50 {
		loyaltyDelta = 1
	}

	if err := loyalty.AppendLoyaltyEvent(r.Context(), h.pool, h.eventStore,
		targetID, worldID, "gift", loyaltyDelta,
		"wanax_gift",
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record gift")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"loyalty_delta": loyaltyDelta,
		"gold_sent":     req.Gold,
		"food_sent":     req.Food,
	})
}

// LoyaltyLog handles GET /worlds/:worldID/settlements/:settlementID/loyalty-log.
func (h *SettlementHandler) LoyaltyLog(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	settlementID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Verify ownership.
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		settlementID, worldID,
	).Scan(&ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, event_type, loyalty_delta, reason, created_at
		 FROM loyalty_events
		 WHERE settlement_id = $1
		 ORDER BY created_at DESC
		 LIMIT 50`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load loyalty log")
		return
	}
	defer rows.Close()

	type entry struct {
		ID           int64     `json:"id"`
		EventType    string    `json:"event_type"`
		LoyaltyDelta int       `json:"loyalty_delta"`
		Reason       string    `json:"reason"`
		CreatedAt    time.Time `json:"created_at"`
	}
	var log []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.EventType, &e.LoyaltyDelta, &e.Reason, &e.CreatedAt); err == nil {
			log = append(log, e)
		}
	}
	if log == nil {
		log = []entry{}
	}
	writeJSON(w, http.StatusOK, log)
}

// ReturnArmy handles POST /worlds/:worldID/settlements/:settlementID/return-army.
// The king returns a borrowed army to its settlement.
func (h *SettlementHandler) ReturnArmy(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	settlementID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Verify caller is king of a kingdom the settlement owner belongs to.
	var kingdomID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT km.kingdom_id
		 FROM kingdom_members km
		 WHERE km.player_id = $1 AND km.role = 'king'
		   AND km.kingdom_id IN (
		       SELECT km2.kingdom_id FROM kingdom_members km2
		       JOIN settlements s ON s.owner_id = km2.player_id
		       WHERE s.id = $2 AND s.world_id = $3
		   )`,
		playerID, settlementID, worldID,
	).Scan(&kingdomID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not the king for this settlement's kingdom")
		return
	}

	// Find the borrowed army row for this kingdom with lender = settlement owner.
	var baID uuid.UUID
	var inf, cav, cat, pri, ship int
	var lenderID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT ba.id, ba.lender_id, ba.infantry, ba.cavalry, ba.catapult, ba.priest, ba.ship
		 FROM borrowed_armies ba
		 JOIN settlements s ON s.owner_id = ba.lender_id AND s.id = $1
		 WHERE ba.kingdom_id = $2 AND ba.returned_at IS NULL
		 LIMIT 1`,
		settlementID, kingdomID,
	).Scan(&baID, &lenderID, &inf, &cav, &cat, &pri, &ship)
	if err != nil {
		writeError(w, http.StatusNotFound, "no borrowed army to return")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Mark returned.
	_, err = tx.Exec(r.Context(),
		`UPDATE borrowed_armies SET returned_at = now() WHERE id = $1`,
		baID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not mark army returned")
		return
	}

	// Return army to lender's settlement.
	_, err = tx.Exec(r.Context(),
		`UPDATE settlements SET
		   infantry = infantry + $1,
		   cavalry  = cavalry  + $2,
		   catapult = catapult + $3,
		   priest   = priest   + $4,
		   ship     = ship     + $5
		 WHERE id = $6`,
		inf, cav, cat, pri, ship, settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not return army units")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"returned": map[string]int{
			"infantry": inf, "cavalry": cav, "catapult": cat,
			"priest": pri, "ship": ship,
		},
	})
}

// Gossip handles GET /worlds/:worldID/gossip — the player's gossip feed.
func (h *SettlementHandler) Gossip(w http.ResponseWriter, r *http.Request) {
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
		`SELECT id, source_region, category, text, generated_at
		 FROM gossip_events
		 WHERE world_id = $1 AND recipient_id = $2
		 ORDER BY generated_at DESC
		 LIMIT 30`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load gossip")
		return
	}
	defer rows.Close()

	type item struct {
		ID           uuid.UUID `json:"id"`
		SourceRegion string    `json:"source_region"`
		Category     string    `json:"category"`
		Text         string    `json:"text"`
		GeneratedAt  time.Time `json:"generated_at"`
	}
	var result []item
	for rows.Next() {
		var g item
		if err := rows.Scan(&g.ID, &g.SourceRegion, &g.Category, &g.Text, &g.GeneratedAt); err == nil {
			result = append(result, g)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}
