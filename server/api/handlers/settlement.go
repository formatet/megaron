package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/loyalty"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/timescale"
)

// SettlementHandler handles HTTP requests for settlement endpoints.
type SettlementHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	scheduler  *events.Scheduler
	clk        clock.Clock
}

// NewSettlementHandler creates a SettlementHandler.
func NewSettlementHandler(pool *pgxpool.Pool, store *events.Store, sched *events.Scheduler, clk clock.Clock) *SettlementHandler {
	return &SettlementHandler{pool: pool, eventStore: store, scheduler: sched, clk: clk}
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

	now := h.clk.Now()

	// Load cult_level and live kharis from the player's world record.
	// battle_frenzy_until is still on the settlement (it's a per-settlement combat state).
	var frenzyUntil *time.Time
	_ = h.pool.QueryRow(r.Context(),
		`SELECT battle_frenzy_until FROM settlements WHERE id = $1`,
		sett.ID,
	).Scan(&frenzyUntil)

	var cultLevel string
	var kharisNow float64
	if sett.OwnerID != nil {
		k, _ := loadPlayerKharis(r.Context(), h.pool, *sett.OwnerID, worldID)
		cultLevel = k.CultLevel
		kharisNow = k.Amount
		if cultLevel == "" {
			cultLevel = "enkel"
		}
	}

	divineMood := kharisToMood(kharisNow)

	resp := map[string]any{
		"id":                   sett.ID,
		"province_id":          sett.ProvinceID,
		"name":                 sett.Name,
		"culture":              sett.CultureID,
		"control_type":         sett.ControlType,
		"loyalty":              sett.Loyalty,
		"loyalty_trend":        sett.LoyaltyTrend,
		"wall_level":           sett.WallLevel,
		"is_capital":           sett.IsCapital,
		"state":                sett.State,
		"population":           sett.Population,
		"resources":            sett.Resources.Snapshot(now),
		"army":                 sett.Army,
		"cult_level":           cultLevel,
		"divine_mood":          divineMood,
		"battle_frenzy_until":  frenzyUntil,
		"updated_at":           sett.UpdatedAt,
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
		Silver float64 `json:"silver"`
		Grain  float64 `json:"grain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Silver < 0 || req.Grain < 0 || (req.Silver == 0 && req.Grain == 0) {
		writeError(w, http.StatusBadRequest, "gift must include silver or grain")
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

	// Caravan travel time: source capital → target settlement (both your own — internal supply line).
	var sQ, sR, tQ, tR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		sourceID).Scan(&sQ, &sR)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		targetID).Scan(&tQ, &tR)
	dist := province.HexDistance(province.MapPosition{Q: sQ, R: sR}, province.MapPosition{Q: tQ, R: tR})
	arrivesAt := h.clk.Now().Add(messenger.TradeTravelDuration(dist))

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Deduct silver from source settlement column.
	if req.Silver > 0 {
		tag, err2 := tx.Exec(r.Context(),
			`UPDATE settlements SET
			   silver_amount = silver_amount
			     + EXTRACT(EPOCH FROM (now() - silver_calc_at))/60 * silver_rate - $1,
			   silver_calc_at = now()
			 WHERE id = $2
			   AND silver_amount + EXTRACT(EPOCH FROM (now() - silver_calc_at))/60 * silver_rate >= $1`,
			req.Silver, sourceID,
		)
		if err2 != nil || tag.RowsAffected() == 0 {
			writeError(w, http.StatusUnprocessableEntity, "insufficient silver")
			return
		}
	}

	// Deduct grain from source settlement_goods.
	if req.Grain > 0 {
		tag, err2 := tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			   amount  = amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate - $1,
			   calc_at = now()
			 WHERE settlement_id = $2 AND good_key = 'grain'
			   AND amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate >= $1`,
			req.Grain, sourceID,
		)
		if err2 != nil || tag.RowsAffected() == 0 {
			writeError(w, http.StatusUnprocessableEntity, "insufficient grain")
			return
		}
	}

	// Dispatch the gift as a caravan — it is credited to the target on ARRIVAL, not instantly.
	// Internal supply line: no caravan-loss.
	if req.Silver > 0 {
		if err2 := h.scheduler.EnqueueTx(r.Context(), tx, worldID, events.ScheduledLogisticsArrival,
			map[string]any{"kind": "settlement_good", "destination": targetID, "good_key": "silver", "quantity": req.Silver},
			arrivesAt); err2 != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule gift silver")
			return
		}
	}
	if req.Grain > 0 {
		if err2 := h.scheduler.EnqueueTx(r.Context(), tx, worldID, events.ScheduledLogisticsArrival,
			map[string]any{"kind": "settlement_good", "destination": targetID, "good_key": "grain", "quantity": req.Grain},
			arrivesAt); err2 != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule gift grain")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	// Apply loyalty event — significant gift (50+ silver equivalent) gives +1 loyalty.
	// Applied at send (the gesture is committed); goods themselves arrive after travel.
	loyaltyDelta := 0
	if req.Silver+req.Grain*0.5 >= 50 {
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
		"silver_sent":   req.Silver,
		"grain_sent":    req.Grain,
		"arrives_at":    arrivesAt,
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
		 WHERE km.player_id = $1 AND km.role = 'basileus'
		   AND km.kingdom_id IN (
		       SELECT km2.kingdom_id FROM kingdom_members km2
		       JOIN settlements s ON s.owner_id = km2.player_id
		       WHERE s.id = $2 AND s.world_id = $3
		   )`,
		playerID, settlementID, worldID,
	).Scan(&kingdomID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not the basileus for this settlement's kingdom")
		return
	}

	// Find the borrowed army row for this kingdom with lender = settlement owner.
	var baID uuid.UUID
	var inf, cha, pri, ship int
	var lenderID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT ba.id, ba.lender_id, ba.infantry, ba.chariot, ba.priest, ba.ship
		 FROM borrowed_armies ba
		 JOIN settlements s ON s.owner_id = ba.lender_id AND s.id = $1
		 WHERE ba.kingdom_id = $2 AND ba.returned_at IS NULL
		 LIMIT 1`,
		settlementID, kingdomID,
	).Scan(&baID, &lenderID, &inf, &cha, &pri, &ship)
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
		   chariot  = chariot  + $2,
		   priest   = priest   + $3,
		   ship     = ship     + $4
		 WHERE id = $5`,
		inf, cha, pri, ship, settlementID,
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
			"infantry": inf, "chariot": cha,
			"priest": pri, "ship": ship,
		},
	})
}

// SetCultLevel handles PATCH /worlds/:worldID/settlements/:settlementID/cult-level.
// The Wanax chooses how generously to maintain the temple.
func (h *SettlementHandler) SetCultLevel(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		CultLevel string `json:"cult_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	valid := map[string]bool{
		"forsummad": true, "enkel": true, "vardig": true,
		"praktfull": true, "overdadig": true,
	}
	if !valid[req.CultLevel] {
		writeError(w, http.StatusBadRequest, "cult_level must be forsummad, enkel, vardig, praktfull, or overdadig")
		return
	}

	// Verify the player owns a settlement in this world before updating cult level.
	var ownerCheck int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT 1 FROM settlements WHERE id = $1 AND world_id = $2 AND owner_id = $3`,
		settlementID, worldID, playerID,
	).Scan(&ownerCheck); err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE player_world_records SET cult_level = $1
		 WHERE player_id = $2 AND world_id = $3`,
		req.CultLevel, playerID, worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update cult level")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "world record not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"cult_level": req.CultLevel})
}

// Rite handles POST /worlds/:worldID/settlements/:settlementID/rite.
// Performs a ritual intercession — requires ≥1 stationed Hiereus, costs 5 grain.
// Success probability is determined by divine mood (kharis level):
//   Favorable (≥800 kharis): 80% · Indifferent (≥400): 50% · Suspicious (≥100): 20% · Wrathful: 5%
// On success: sets battle_frenzy for 6 hours — attacker infantry fights at ×1.5 strength.
func (h *SettlementHandler) Rite(w http.ResponseWriter, r *http.Request) {
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

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var priestCount int
	var alreadyFrenzied bool
	err = tx.QueryRow(r.Context(),
		`SELECT priest,
		        (battle_frenzy_until IS NOT NULL AND battle_frenzy_until > now())
		 FROM settlements
		 WHERE id = $1 AND world_id = $2 AND owner_id = $3
		 FOR UPDATE`,
		settlementID, worldID, playerID,
	).Scan(&priestCount, &alreadyFrenzied)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	var kharisNow float64
	_ = tx.QueryRow(r.Context(),
		`SELECT GREATEST(0, kharis_amount + (EXTRACT(EPOCH FROM (now()-kharis_calc_at))/60 * kharis_rate))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2
		 FOR UPDATE`,
		playerID, worldID,
	).Scan(&kharisNow)

	if priestCount < 1 {
		writeError(w, http.StatusBadRequest, "a Hiereus must be stationed to perform a rite")
		return
	}
	if alreadyFrenzied {
		writeError(w, http.StatusConflict, "battle frenzy already active — wait for it to expire")
		return
	}

	// Determine success probability from divine mood.
	var chance int
	var mood string
	switch {
	case kharisNow >= 800:
		chance, mood = 80, "Favorable"
	case kharisNow >= 400:
		chance, mood = 50, "Indifferent"
	case kharisNow >= 100:
		chance, mood = 20, "Suspicious"
	default:
		chance, mood = 5, "Wrathful"
	}

	// Deduct 5 grain as an offering (regardless of outcome).
	_, err = tx.Exec(r.Context(),
		`UPDATE settlement_goods SET
		    amount  = GREATEST(0, amount + (EXTRACT(EPOCH FROM (now()-calc_at))/60 * rate) - 5),
		    calc_at = now()
		 WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct offering")
		return
	}

	success := rand.Intn(100) < chance
	var expiresAt *time.Time
	if success {
		t := h.clk.Now().Add(timescale.Apply(6 * time.Hour))
		expiresAt = &t
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlements SET battle_frenzy_until = $1 WHERE id = $2`,
			*expiresAt, settlementID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not set frenzy")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	resp := map[string]any{
		"success": success,
		"mood":    mood,
		"chance":  chance,
	}
	if success {
		resp["effect"]     = "battle_frenzy"
		resp["expires_at"] = expiresAt
		resp["message"]    = "The gods answer your plea — your warriors fight with divine fury!"
	} else {
		resp["message"] = "The gods are silent. Your offering was received, but they do not answer."
	}
	writeJSON(w, http.StatusOK, resp)
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
