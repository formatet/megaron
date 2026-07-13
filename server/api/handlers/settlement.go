package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/loyalty"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/religion"
	"github.com/poleia/server/internal/tick"
	"github.com/poleia/server/internal/transport"
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
		"id":                  sett.ID,
		"province_id":         sett.ProvinceID,
		"name":                sett.Name,
		"culture":             sett.CultureID,
		"control_type":        sett.ControlType,
		"loyalty":             sett.Loyalty,
		"loyalty_trend":       sett.LoyaltyTrend,
		"wall_level":          sett.WallLevel,
		"is_capital":          sett.IsCapital,
		"state":               sett.State,
		"population":          sett.Population,
		"resources":           sett.Resources.Snapshot(now),
		"army":                sett.Army,
		"cult_level":          cultLevel,
		"divine_mood":         divineMood,
		"battle_frenzy_until": frenzyUntil,
		"updated_at":          sett.UpdatedAt,
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
	var giftCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&giftCurrentTick)
	giftDueTick := giftCurrentTick + messenger.TradeTravelTicks(dist)

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Deduct silver from source settlement good row.
	if req.Silver > 0 {
		tag, err2 := tx.Exec(r.Context(),
			`UPDATE settlement_goods
			   SET amount  = settled(amount, rate, calc_tick) - $1,
			       calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = 'silver'
			   AND settled(amount, rate, calc_tick) >= $1`,
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
			   amount  = settled(amount, rate, calc_tick) - $1,
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = 'grain'
			   AND settled(amount, rate, calc_tick) >= $1`,
			req.Grain, sourceID,
		)
		if err2 != nil || tag.RowsAffected() == 0 {
			writeError(w, http.StatusUnprocessableEntity, "insufficient grain")
			return
		}
	}

	// Dispatch the gift as a PHYSICAL caravan — a mover on the map (province route,
	// lazy-interpolated position), credited to the target on ARRIVAL, not instantly.
	// Internal supply line: exempt from the random trade-loss roll (interception is a
	// separate, deliberate mechanic — Del 3-fas-4).
	if _, err2 := transport.Dispatch(r.Context(), tx, h.scheduler, transport.DispatchParams{
		WorldID:       worldID,
		OwnerID:       playerID,
		Kind:          "transfer",
		OriginID:      sourceID,
		DestID:        targetID,
		Category:      "land",
		OriginQ:       sQ,
		OriginR:       sR,
		DestQ:         tQ,
		DestR:         tR,
		DepartsAt:     h.clk.Now(),
		ArrivesAt:     arrivesAt,
		DueTick:       giftDueTick,
		Manifest:      transport.Manifest{"silver": req.Silver, "grain": req.Grain},
		Interceptable: true,
	}); err2 != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch gift caravan")
		return
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
//
// NOTE (SB7): the restore SQL below still writes the retired settlements.* army
// columns; like its counterpart KingdomHandler.BorrowArmy it must be rebuilt on the
// units model when kingdoms are re-enabled. The route is gated off (kingdoms are
// POST-MVP), so this never runs live. See megaron_todo → "SB7 follow-up: borrow-army på units".
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
			"spearman": inf, "war_chariot": cha,
			"priest": pri, "ship": ship,
		},
	})
}

// riteFloor and riteCeil are the "heligt golv/tak" (holy floor/ceiling) — a rite
// never has a 0% or 100% chance; the gods are not machines (Timothy 2026-07-09
// kharis omdesign, temenos_kharis.md §"KANONISK OMDESIGN" FAS 1). Strawman —
// temenos_balans_spakar.md §9.
const (
	riteFloor = 0.10
	riteCeil  = 0.98
)

// Offer-multiplier bounds and the offerMod it produces at each end. A "fett
// offer" (offer_multiplier > 1, more goods than the prayer's baseline) nudges
// success up; a "snålt offer" (< 1, cheaper than baseline) nudges it down.
// offer_multiplier omitted or <= 0 defaults to 1.0 (exactly the baseline
// Offering, offerMod = 0) — fully backward compatible with callers that never
// send the field. Strawman constants — temenos_balans_spakar.md §9.
const (
	riteOfferMultiplierMin = 0.5
	riteOfferMultiplierMax = 2.0
	riteOfferModFat        = 0.10  // bonus at riteOfferMultiplierMax
	riteOfferModStingy     = -0.15 // penalty at riteOfferMultiplierMin
)

// riteOfferMultiplier clamps a requested offer multiplier into
// [riteOfferMultiplierMin, riteOfferMultiplierMax], defaulting to 1.0 (baseline,
// no modifier) for the JSON zero-value / omitted-field / invalid (<=0) case.
func riteOfferMultiplier(raw float64) float64 {
	if raw <= 0 {
		return 1.0
	}
	if raw < riteOfferMultiplierMin {
		return riteOfferMultiplierMin
	}
	if raw > riteOfferMultiplierMax {
		return riteOfferMultiplierMax
	}
	return raw
}

// riteOfferMod maps an offer multiplier to the success-chance modifier: linear
// on each side of 1.0 (baseline), hitting riteOfferModFat exactly at
// riteOfferMultiplierMax and riteOfferModStingy exactly at
// riteOfferMultiplierMin, continuous (0) at multiplier == 1.0.
func riteOfferMod(multiplier float64) float64 {
	switch {
	case multiplier > 1.0:
		return (multiplier - 1.0) / (riteOfferMultiplierMax - 1.0) * riteOfferModFat
	case multiplier < 1.0:
		return (multiplier - 1.0) / (1.0 - riteOfferMultiplierMin) * (-riteOfferModStingy)
	default:
		return 0
	}
}

// riteSuccessChance is the FAS 1 continuous rite formula: the kharis level
// (0-100) IS the success percentage, nudged by offerMod and clamped to the holy
// floor/ceiling. Replaces the old 4-tier lookup (95/80/60/25 at 800/400/100
// kharis) — "talet ÄR mätaren", not a tier.
func riteSuccessChance(kharisNow, offerMod float64) float64 {
	c := kharisNow/100.0 + offerMod
	if c < riteFloor {
		return riteFloor
	}
	if c > riteCeil {
		return riteCeil
	}
	return c
}

// Rite handles POST /worlds/:worldID/settlements/:settlementID/rite.
// Performs a cultural prayer — requires a temple, costs a material offering.
// Body: {"prayer":"<prayer_id>","target":"<optional uuid>","offer_multiplier":<optional float>}.
// Omitting prayer defaults to the culture's battle_frenzy prayer (backward compat).
// offer_multiplier (default 1.0, clamped to [0.5, 2.0]) scales the prayer's
// material offering up ("fett offer") or down ("snålt offer") and nudges the
// success chance accordingly — see riteOfferMod.
//
// Success probability is continuous, not tiered (FAS 1): the kharis level
// (0-100) IS the success percentage — kharis 95 → ~95%, kharis 40 → ~40% —
// nudged by offerMod and clamped to [riteFloor, riteCeil]. See riteSuccessChance.
//
// The prayer must belong to the settlement's culture (403 otherwise).
// The prayer must be off cooldown (409 otherwise).
// Outcome is rolled once in the handler and stored in the RiteCast event (Fas 2.3).
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

	// Decode optional body.
	var body struct {
		Prayer          string  `json:"prayer"`
		Target          string  `json:"target"`
		OfferMultiplier float64 `json:"offer_multiplier"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	// Lock the settlement row and read culture + battle_frenzy state.
	var cultureID string
	var alreadyFrenzied bool
	err = tx.QueryRow(r.Context(),
		`SELECT culture_id,
		        (battle_frenzy_until IS NOT NULL AND battle_frenzy_until > now())
		 FROM settlements
		 WHERE id = $1 AND world_id = $2 AND owner_id = $3
		 FOR UPDATE`,
		settlementID, worldID, playerID,
	).Scan(&cultureID, &alreadyFrenzied)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Resolve prayer: empty → default battle_frenzy for this culture.
	prayerID := body.Prayer
	if prayerID == "" {
		prayerID = religion.DefaultBattleFrenzyFor(cultureID)
	}

	// Validate prayer exists.
	spec, ok := religion.PrayerSpecs[prayerID]
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown prayer %q", prayerID))
		return
	}

	// Culture gate.
	if !religion.AllowedForCulture(cultureID, prayerID) {
		writeError(w, http.StatusForbidden,
			fmt.Sprintf("prayer %q is not available to culture %q", prayerID, cultureID))
		return
	}

	var hasTemple bool
	_ = tx.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'temple')`,
		settlementID,
	).Scan(&hasTemple)
	if !hasTemple {
		writeError(w, http.StatusBadRequest, "a temple must be built to perform rites")
		return
	}

	var kharisNow float64
	_ = tx.QueryRow(r.Context(),
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2
		 FOR UPDATE`,
		playerID, worldID,
	).Scan(&kharisNow)

	// Kharis tier gate.
	if kharisNow < spec.MinKharis {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("insufficient divine favour: prayer %q requires %.0f kharis (you have %.0f)",
				prayerID, spec.MinKharis, kharisNow))
		return
	}

	// Battle-frenzy-specific guard (can't stack).
	if spec.EffectType == religion.EffectBattleFrenzy && alreadyFrenzied {
		writeError(w, http.StatusConflict, "battle frenzy already active — wait for it to expire")
		return
	}

	// Cooldown check: query last successful RiteCast for this (player, prayer, temple) from events table.
	// Column-free: uses the existing event log, no new schema.
	// Keyed on stream_id = settlementID so the cooldown is per TEMPLE, not per Wanax:
	// a Wanax with temples in five cities can cast five prayers per cycle (one per city).
	if spec.CooldownTicks > 0 {
		var lastCast time.Time
		cooldownErr := h.pool.QueryRow(r.Context(),
			`SELECT created_at FROM events
			 WHERE world_id = $1
			   AND event_type = 'RiteCast'
			   AND payload->>'player_id' = $2
			   AND payload->>'prayer' = $3
			   AND (payload->>'success')::boolean = true
			   AND stream_id = $4
			 ORDER BY created_at DESC LIMIT 1`,
			worldID, playerID.String(), prayerID, settlementID,
		).Scan(&lastCast)
		if cooldownErr == nil {
			elapsed := h.clk.Now().Sub(lastCast)
			remaining := tick.RealUntil(spec.CooldownTicks, 0) - elapsed
			if remaining > 0 {
				writeError(w, http.StatusConflict,
					fmt.Sprintf("prayer %q is on cooldown for another %.0f minutes",
						prayerID, remaining.Minutes()))
				return
			}
		}
		// ErrNoRows = never cast before = allowed.
	}

	// Determine success probability — continuous (FAS 1), not tiered: the kharis
	// level IS the success percentage, nudged by how much was offered.
	offerMultiplier := riteOfferMultiplier(body.OfferMultiplier)
	offerMod := riteOfferMod(offerMultiplier)
	successChance := riteSuccessChance(kharisNow, offerMod)
	chance := int(successChance*100 + 0.5) // percentage, rounded — for the roll ONLY.
	// DESIGN INVARIANT (Timothy 2026-07-11, HARD): `chance` never leaves this handler —
	// the gods are not machines. It is used below purely to weight rand.Intn(100) and
	// must NOT be added to `resp`. Gynnsamhet is communicated via `mood` instead.
	mood := kharisToMood(kharisNow)

	// Affordability check + deduct the material offering, scaled by
	// offerMultiplier (a "fett offer" costs proportionally more goods; a "snålt
	// offer" costs less). The gods take the sacrifice regardless of outcome.
	// Kharis is never part of this — it is standing (gated above); the offering
	// is in trade goods (wine/oil/silver/…), the deliberate economic sink that
	// makes religion drive trade.
	scaledOffering := make(map[string]float64, len(spec.Offering))
	for good, baseline := range spec.Offering {
		scaledOffering[good] = baseline * offerMultiplier
	}
	for good, need := range scaledOffering {
		var have float64
		if scanErr := tx.QueryRow(r.Context(),
			`SELECT GREATEST(0, settled(amount, rate, calc_tick))
			 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
			settlementID, good,
		).Scan(&have); scanErr != nil || have < need {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("insufficient offering for %q: need %.1f %s (have %.1f)",
					prayerID, need, good, have))
			return
		}
	}
	for good, need := range scaledOffering {
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			    amount  = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			    calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, need, good,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not deduct offering")
			return
		}
	}

	// Roll outcome once (Fas 2.3 — result goes into the event, not "roll_pending").
	success := rand.Intn(100) < chance

	// Apply effect on success.
	effectPayload := map[string]any{}
	var message string

	if success {
		switch spec.EffectType {
		case religion.EffectBattleFrenzy:
			effectPayload, message, err = h.applyBattleFrenzy(r.Context(), tx, settlementID)
		case religion.EffectOracleRevealDeposits:
			effectPayload, message, err = h.applyOracleRevealDeposits(r.Context(), tx, settlementID, worldID, playerID, spec)
		case religion.EffectHarvestBlessing:
			effectPayload, message, err = h.applyHarvestBlessing(r.Context(), tx, settlementID, spec)
		default:
			effectPayload = map[string]any{"type": spec.EffectType}
			message = fmt.Sprintf("The gods accept your prayer — %s is granted.", spec.Name)
		}
		if err != nil {
			slog.Error("rite: apply effect failed", "prayer", prayerID,
				"effect", spec.EffectType, "settlement", settlementID, "err", err)
			writeError(w, http.StatusInternalServerError, "could not apply effect")
			return
		}
	} else {
		message = "The gods are silent. Your offering was received, but they do not answer."
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	// Emit RiteCast event AFTER commit (event store uses pool, not the now-committed TX).
	// Payload carries the full outcome (Fas 2.3).
	eventPayload := map[string]any{
		"player_id":        playerID.String(),
		"prayer":           prayerID,
		"effect_type":      spec.EffectType,
		"success":          success,
		"offering":         scaledOffering,
		"offer_multiplier": offerMultiplier,
		"effect":           effectPayload,
	}
	_, _ = h.eventStore.Append(r.Context(), settlementID, events.StreamReligion, "RiteCast",
		eventPayload, worldID, nil)

	resp := map[string]any{
		"success":          success,
		"mood":             mood,
		"offer_multiplier": offerMultiplier,
		"prayer":           prayerID,
		"message":          message,
	}
	if success {
		resp["effect_type"] = spec.EffectType
		resp["effect"] = effectPayload
	}
	writeJSON(w, http.StatusOK, resp)
}

// applyBattleFrenzy sets battle_frenzy_until for 6 scaled hours.
func (h *SettlementHandler) applyBattleFrenzy(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID) (map[string]any, string, error) {
	t := h.clk.Now().Add(tick.RealUntil(6, 0))
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET battle_frenzy_until = $1 WHERE id = $2`,
		t, settlementID,
	); err != nil {
		return nil, "", err
	}
	return map[string]any{"expires_at": t}, "The gods answer your plea — your warriors fight with divine fury!", nil
}

// applyHarvestBlessing boosts the settlement's grain by 25% (one-shot abundance).
// Mirrors the tick-level applyDivineBlessing harvest_blessing SQL.
func (h *SettlementHandler) applyHarvestBlessing(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, spec religion.PrayerSpec) (map[string]any, string, error) {
	if _, err := tx.Exec(ctx,
		`UPDATE settlement_goods SET
		    amount  = LEAST(cap, settled(amount, rate, calc_tick) * 1.25),
		    calc_tick = current_world_tick()
		 WHERE settlement_id = $1 AND good_key = 'grain'`,
		settlementID,
	); err != nil {
		return nil, "", err
	}
	msg := fmt.Sprintf("%s smiles upon your fields — grain stocks swell by a quarter.", spec.God)
	return map[string]any{"good": "grain", "multiplier": 1.25}, msg, nil
}

// applyOracleRevealDeposits reveals the nearest uncolonised tile(s) within 8 hexes
// of the settlement whose 6-hex catchment contains a tin, copper, or silver deposit.
// Tin is prioritised (rarest); copper next; silver last.
//
// FIX (Fas 1b, 2026-06-22): the previous query searched the `provinces` table (only
// already-settled hexes joined to map_tiles). Tin tiles are `mountain_limestone`
// (impassable, never settled), and their colonisable hills-neighbours were always
// unclaimed → zero results. The new query searches `map_tiles` directly for a
// **buildable** candidate tile (terrain eligible as a colony site) whose 6 axial
// neighbours include a deposit tile, skipping tiles the player already owns.
//
// Payload format returned in effectPayload (nested under "effect" in RiteCast event):
//
//	{
//	  "reveals": [
//	    { "q": 47, "r": 12, "ore": "tin" },
//	    { "q": 45, "r": 14, "ore": "copper" }   // optional second result
//	  ]
//	}
//
// Harness usage: read event payload["effect"]["reveals"][0]["q"/"r"] to get the
// colonisable tile coordinates, then issue a settle action there. colonize validation
// (unit.go:179) only requires a buildable unoccupied tile — FOW-visibility is NOT
// required to colonize it. The reveal is still written to player_scouted_tiles so the
// coordinate persists as map/FOW memory for the player (movement-motor Pass II).
//
// Idempotency: each reveal is INSERT ... ON CONFLICT DO NOTHING against
// player_scouted_tiles, so re-running (e.g. a retried TX) is safe.
func (h *SettlementHandler) applyOracleRevealDeposits(
	ctx context.Context,
	tx pgx.Tx,
	settlementID, worldID, playerID uuid.UUID,
	spec religion.PrayerSpec,
) (map[string]any, string, error) {
	const oracleRadius = 8

	// Find the settlement's province position (origin for radius search).
	var originQ, originR int
	if err := tx.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM provinces p
		 JOIN settlements s ON s.province_id = p.id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&originQ, &originR); err != nil {
		return nil, "", fmt.Errorf("oracle: could not find settlement province: %w", err)
	}

	// Find buildable tiles (colony candidates) in map_tiles:
	//   - terrain is eligible for settlement (not sea, impassable mountains, or semi_desert)
	//   - at least one of their 6 axial neighbours carries a deposit
	//   - not already owned by this player (no province row with owner = playerID)
	//   - within oracleRadius hex-distance from origin
	//
	// We generate the 6 neighbour offsets inline in SQL using LATERAL / VALUES so
	// this stays a single round-trip with no application-side loops.
	//
	// Bronze needs BOTH copper AND tin, but the oracle is gated by a long cooldown —
	// so a single cast must seed the whole chain, not roll one random metal (rolling
	// silver used to lock a Wanax out of the bronze metals for the whole cooldown).
	// Reveal the nearest TIN site and the nearest COPPER site (the two bronze metals),
	// plus the nearest silver-only site as a bonus when present.
	rows, err := tx.Query(ctx,
		`WITH sites AS (
		     SELECT site.q AS q, site.r AS r,
		            BOOL_OR(nb.tin_deposit)                     AS has_tin,
		            BOOL_OR(nb.copper_deposit)                   AS has_copper,
		            BOOL_OR(COALESCE(nb.silver_deposit, false))  AS has_silver,
		            (ABS(site.q - $2) + ABS((site.q - $2) + (site.r - $3)) + ABS(site.r - $3)) / 2 AS dist
		     FROM map_tiles site
		     JOIN LATERAL (VALUES
		         (1,0),(-1,0),(0,1),(0,-1),(1,-1),(-1,1)
		     ) AS d(dq, dr) ON true
		     JOIN map_tiles nb
		       ON nb.world_id = site.world_id
		      AND nb.q = site.q + d.dq
		      AND nb.r = site.r + d.dr
		      AND (nb.tin_deposit OR nb.copper_deposit OR COALESCE(nb.silver_deposit, false))
		     WHERE site.world_id = $1
		       AND site.terrain NOT IN
		           ('coastal_sea','deep_sea','mountain_limestone','mountain_red','semi_desert')
		       AND (ABS(site.q - $2) + ABS((site.q - $2) + (site.r - $3)) + ABS(site.r - $3)) / 2 <= $4
		       -- the revealed site must be COLONISABLE: no active settlement on it
		       -- (any owner). Revealing a hex someone else already holds is useless —
		       -- you can't colonise it. The exclusion is owner-agnostic, so playerID is
		       -- no longer a query parameter (passing an unreferenced $2 made Postgres
		       -- fail with "could not determine data type of parameter $2").
		       AND NOT EXISTS (
		           SELECT 1 FROM settlements s2
		           JOIN provinces p ON p.id = s2.province_id
		           WHERE p.world_id = site.world_id
		             AND p.map_q = site.q AND p.map_r = site.r
		             AND s2.state = 'active'
		       )
		     GROUP BY site.q, site.r
		 )
		 (SELECT q, r, has_tin, has_copper, has_silver FROM sites WHERE has_tin ORDER BY dist LIMIT 1)
		 UNION ALL
		 (SELECT q, r, has_tin, has_copper, has_silver FROM sites WHERE has_copper AND NOT has_tin ORDER BY dist LIMIT 1)
		 UNION ALL
		 (SELECT q, r, has_tin, has_copper, has_silver FROM sites WHERE has_silver AND NOT has_tin AND NOT has_copper ORDER BY dist LIMIT 1)`,
		worldID, originQ, originR, oracleRadius,
	)
	if err != nil {
		return nil, "", fmt.Errorf("oracle: query deposits: %w", err)
	}
	defer rows.Close()

	type revealedSite struct {
		Q, R   int
		Tin    bool
		Copper bool
		Silver bool
	}
	var revealed []revealedSite
	for rows.Next() {
		var rs revealedSite
		if err := rows.Scan(&rs.Q, &rs.R, &rs.Tin, &rs.Copper, &rs.Silver); err == nil {
			revealed = append(revealed, rs)
		}
	}
	rows.Close()

	if len(revealed) == 0 {
		msg := fmt.Sprintf("%s gazes into the distance — no ore deposits lie within reach to reveal.", spec.God)
		return map[string]any{"reveals": []any{}}, msg, nil
	}

	// Build human-readable message and structured payload.
	oreKey := func(rs revealedSite) string {
		switch {
		case rs.Tin:
			return "tin"
		case rs.Copper:
			return "copper"
		default:
			return "silver"
		}
	}

	var parts []string
	revealsPayload := make([]map[string]any, 0, len(revealed))
	foundTin, foundCopper, foundSilver := false, false, false
	for _, rs := range revealed {
		ore := oreKey(rs)
		parts = append(parts, fmt.Sprintf("%s at (%d,%d)", ore, rs.Q, rs.R))
		revealsPayload = append(revealsPayload, map[string]any{
			"q":   rs.Q,
			"r":   rs.R,
			"ore": ore,
		})
		// Persist the reveal as scouted-tile memory (R3): the coordinate stays on the
		// player's map/FOW after this rite even though colonize itself is not FOW-gated.
		if _, insErr := tx.Exec(ctx,
			`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
			 VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
			worldID, playerID, rs.Q, rs.R,
		); insErr != nil {
			slog.Warn("oracle: scouted-tile insert failed", "q", rs.Q, "r", rs.R, "err", insErr)
		}
		switch ore {
		case "tin":
			foundTin = true
		case "copper":
			foundCopper = true
		case "silver":
			foundSilver = true
		}
	}

	// Name absent metals explicitly so the player doesn't wait on cooldown hoping for more.
	var absent []string
	if !foundTin {
		absent = append(absent, "no tin")
	}
	if !foundCopper {
		absent = append(absent, "no copper")
	}
	if !foundSilver {
		absent = append(absent, "no silver")
	}
	absentNote := ""
	if len(absent) > 0 {
		absentNote = " (" + strings.Join(absent, ", ") + " within reach)"
	}

	var msg string
	if len(parts) == 1 {
		msg = fmt.Sprintf("%s reveals a site near hidden ore — %s%s.", spec.God, parts[0], absentNote)
	} else {
		msg = fmt.Sprintf("%s reveals sites near hidden ore — %s%s.", spec.God, strings.Join(parts, "; "), absentNote)
	}

	return map[string]any{
		"reveals": revealsPayload,
	}, msg, nil
}

// Abandon handles POST /worlds/:worldID/settlements/:settlementID/abandon.
//
// Voluntarily gives up a colony: the garrison is disbanded, the settlement's own
// province and any outpost provinces it fed are freed, and the row is marked
// state='abandoned'. This is the consolidation valve that pairs with the
// MaxSettlementsPerWanax cap — abandoning frees a slot (the cap counts state='active').
//
// Distinct from collapse: abandonment is peaceful (no warband spawns) and lighter
// (the owner keeps their capital and any kingdom membership). The capital itself
// cannot be abandoned — losing your seat is collapse, not a voluntary act.
func (h *SettlementHandler) Abandon(w http.ResponseWriter, r *http.Request) {
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

	// Lock the settlement; verify ownership, that it is active, and not the capital.
	var isCapital bool
	var state string
	var provinceID uuid.UUID
	var name string
	err = tx.QueryRow(r.Context(),
		`SELECT is_capital, state, province_id, name
		 FROM settlements
		 WHERE id = $1 AND world_id = $2 AND owner_id = $3
		 FOR UPDATE`,
		settlementID, worldID, playerID,
	).Scan(&isCapital, &state, &provinceID, &name)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}
	if state != "active" {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("settlement is already %q and cannot be abandoned", state))
		return
	}
	if isCapital {
		writeError(w, http.StatusUnprocessableEntity,
			"your capital cannot be abandoned — losing your seat is collapse, not a voluntary act")
		return
	}

	// Disband garrison units (and any embarked cargo) so no orphan rows remain.
	garrisonRows, err := tx.Query(r.Context(),
		`SELECT id, cargo_unit_id FROM units WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load garrison")
		return
	}
	var garrisonIDs, cargoIDs []uuid.UUID
	for garrisonRows.Next() {
		var gid uuid.UUID
		var cargoID *uuid.UUID
		if scanErr := garrisonRows.Scan(&gid, &cargoID); scanErr == nil {
			garrisonIDs = append(garrisonIDs, gid)
			if cargoID != nil {
				cargoIDs = append(cargoIDs, *cargoID)
			}
		}
	}
	garrisonRows.Close()
	for _, gid := range garrisonIDs {
		_, _ = tx.Exec(r.Context(),
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, gid)
	}
	for _, cid := range cargoIDs {
		_, _ = tx.Exec(r.Context(),
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1 AND status = 'embarked'`, cid)
	}

	// Free any outpost provinces this settlement fed, then drop the flows.
	if _, err := tx.Exec(r.Context(),
		`UPDATE provinces SET territory_state = 'free', owner_id = NULL,
		     outpost_feeds = NULL, garrison_strength = 0
		 WHERE id IN (SELECT province_id FROM outpost_flows WHERE settlement_id = $1)`,
		settlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not free outpost provinces")
		return
	}
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM outpost_flows WHERE settlement_id = $1`, settlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear outpost flows")
		return
	}

	// Free the settlement's own province so the hex is colonisable again.
	if _, err := tx.Exec(r.Context(),
		`UPDATE provinces SET territory_state = 'free', owner_id = NULL,
		     outpost_feeds = NULL, garrison_strength = 0
		 WHERE id = $1`,
		provinceID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not free province")
		return
	}

	// Mark the settlement abandoned (dispossessed, leaves any kingdom).
	if _, err := tx.Exec(r.Context(),
		`UPDATE settlements SET owner_id = NULL, kingdom_id = NULL,
		     state = 'abandoned', updated_at = now()
		 WHERE id = $1`,
		settlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not abandon settlement")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	_, _ = h.eventStore.Append(r.Context(), settlementID, events.StreamProvince, "SettlementAbandoned",
		map[string]any{"player_id": playerID.String(), "name": name}, worldID, nil)

	writeJSON(w, http.StatusOK, map[string]any{
		"abandoned": settlementID.String(),
		"name":      name,
		"message":   fmt.Sprintf("%s has been abandoned. Its people scatter and the hex falls quiet.", name),
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
		`SELECT id, source_region, category, text, generated_at, importance, hops
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
		Importance   string    `json:"importance"`
		Hops         int       `json:"hops"`
	}
	var result []item
	for rows.Next() {
		var g item
		if err := rows.Scan(&g.ID, &g.SourceRegion, &g.Category, &g.Text, &g.GeneratedAt, &g.Importance, &g.Hops); err == nil {
			result = append(result, g)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}
