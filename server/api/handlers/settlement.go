package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/loyalty"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		"besieged":            sett.Besieged,
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
	var inf, cha, ship int
	var lenderID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT ba.id, ba.lender_id, ba.infantry, ba.chariot, ba.ship
		 FROM borrowed_armies ba
		 JOIN settlements s ON s.owner_id = ba.lender_id AND s.id = $1
		 WHERE ba.kingdom_id = $2 AND ba.returned_at IS NULL
		 LIMIT 1`,
		settlementID, kingdomID,
	).Scan(&baID, &lenderID, &inf, &cha, &ship)
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
		   ship     = ship     + $3
		 WHERE id = $4`,
		inf, cha, ship, settlementID,
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
			"ship": ship,
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

// riteKharisCost is the standing a single rite draws down on cast — win or lose
// (Timothy 2026-07-24, "make the rite a wager"). It aligns the kharis tick's stated
// intent ("rites SPEND standing, they don't add kharis") which the handler had
// never actually implemented: before this a rite cost only material goods and left
// kharis untouched, so a Wanax pinned at their ceiling cast indefinitely at a fixed
// high chance — the rite was a fee, not a wager (soak 2026-07-23: 594/837 = 71 %
// success, 152 casts in one 5 h window). Now each petition draws down standing
// (floored at the holy floor so it never zeroes), making rites a depleting resource
// a modest (L1) temple cannot sustain and a grander, climbing one slowly can.
// Strawman — temenos_balans_spakar.md §9.
const riteKharisCost = 4.0

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

// insufficientKharisMessage formats the kharis-tier-gate refusal for Rite.
// Rad D, megaron_plan_cli_sanning.md: the old format (%.0f on both numbers)
// let a Wanax at 4.6 kharis against a 5.0 requirement read "requires 5 (you
// have 5)" and not understand why the rite was refused. Tiers
// (religion.MoodSuspicious etc.) are whole numbers, but current kharis is
// not — show it at one-decimal precision and name the shortfall explicitly
// instead of making the reader subtract two rounded numbers.
func insufficientKharisMessage(prayerID string, required, current float64) string {
	return fmt.Sprintf("insufficient divine favour: prayer %q requires %.0f kharis (you have %.1f, %.1f short)",
		prayerID, required, current, required-current)
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
		// Offering is the composed sacrifice: good_key→amount, the Wanax's own
		// choice of what to carry to the altar. Omitted → the prayer's
		// traditional recipe scaled by OfferMultiplier (unchanged behaviour).
		Offering map[string]float64 `json:"offering"`
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
		// Distinguish "no temple at all" from "temple finishing" — the latter is
		// the surprise a Wanax hits performing a rite right after a temple build
		// shows done: the BuildComplete event (which inserts the buildings row) is
		// a poll away. Mirror the foundry-422 fix (df3a77b) so the error names the
		// real cause instead of a bare "temple required".
		var queued bool
		_ = tx.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM build_queue WHERE settlement_id = $1 AND building_type = 'temple')`,
			settlementID,
		).Scan(&queued)
		if queued {
			writeError(w, http.StatusBadRequest,
				"temple is still finishing here — it becomes usable within a tick of completion; retry shortly")
		} else {
			writeError(w, http.StatusBadRequest,
				"temple required — build a temple here first (rites are performed at a temple)")
		}
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
		writeError(w, http.StatusBadRequest, insufficientKharisMessage(prayerID, spec.MinKharis, kharisNow))
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
					// Game-days, not wall-clock minutes: PrayerSpec.CooldownTicks is
					// a TICK count and one tick IS one day, so "another 1440
					// minutes" was both unreadable and hid the unit the player
					// counts in. Same class as cli-sanning row K's ETA fix, one
					// surface that round missed.
					fmt.Sprintf("prayer %q is on cooldown for another %s",
						prayerID, tick.FormatGameDays(tick.GameDaysLeft(remaining))))
				return
			}
		}
		// ErrNoRows = never cast before = allowed.
	}

	// Determine success probability — continuous (FAS 1), not tiered: the kharis
	// level IS the success percentage, nudged by how much was offered.
	offerMultiplier := riteOfferMultiplier(body.OfferMultiplier)
	// Composed offering (Timothy 2026-07-22): the Wanax may bring whatever they
	// judge worthy instead of the prayer's inherited recipe. What it is worth is
	// the world's scarcity (divine_valuations, repriced daily by the kharis tick)
	// times this god's taste — and it is weighed against the SAME god's
	// traditional recipe valued the same way, so bringing exactly the old recipe
	// still lands on offerMod 0. Omitting `offering` keeps the old
	// offer_multiplier path untouched: no client breaks.
	// temenos_prayers_komposition_plan.md
	offering := scaleOffering(spec.Offering, offerMultiplier)
	offerMod := riteOfferMod(offerMultiplier)
	var offeringWorth, offeringBaseline float64
	if len(body.Offering) > 0 {
		divineValues, dvErr := religion.LoadDivineValues(r.Context(), h.pool, worldID)
		if dvErr != nil {
			writeError(w, http.StatusInternalServerError, "could not read the gods' reckoning")
			return
		}
		if len(divineValues) == 0 {
			// The daily tick has not priced this world yet (fresh world, or the
			// first day). Fall back rather than value every gift at zero, which
			// would read as an empty altar and floor the odds.
			offering = scaleOffering(spec.Offering, offerMultiplier)
		} else {
			favours := religion.FavoursFor(spec)
			offering = body.Offering
			offeringWorth = religion.OfferingWorth(offering, divineValues, favours)
			offeringBaseline = religion.TraditionalBaseline(spec, divineValues)
			offerMod = religion.OfferMod(offeringWorth, offeringBaseline,
				religion.DistinctGoods(offering))
		}
	}
	successChance := riteSuccessChance(kharisNow, offerMod)
	chance := int(successChance*100 + 0.5) // percentage, rounded — for the roll ONLY.
	// DESIGN INVARIANT (Timothy 2026-07-11, HARD): `chance` never leaves this handler —
	// the gods are not machines. It is used below purely to weight rand.Intn(100) and
	// must NOT be added to `resp`. Gynnsamhet is communicated via `mood` instead.
	mood := kharisToMood(kharisNow)

	// Affordability check + deduct the material offering, scaled by
	// offerMultiplier (a "fett offer" costs proportionally more goods; a "snålt
	// offer" costs less). The gods take the sacrifice regardless of outcome.
	// The offering is in trade goods (wine/oil/silver/…), the deliberate economic
	// sink that makes religion drive trade. The rite ALSO draws down kharis itself
	// (riteKharisCost, deducted below after the goods clear) — a rite is a wager on
	// standing, not just a goods-fee (Timothy 2026-07-24). The MinKharis gate above
	// still guards eligibility on the pre-cast level.
	// `offering` was resolved above: either the composed one the Wanax brought or
	// the prayer's traditional recipe scaled by offer_multiplier.
	for good, need := range offering {
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
	for good, need := range offering {
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

	// The petition itself costs divine standing — a rite is a wager, not a fee
	// (Timothy 2026-07-24). Spent whether or not the god answers (like the material
	// offering above), floored at the holy floor so it never zeroes out. The row was
	// locked FOR UPDATE when kharisNow was read, so this settled() reads the same
	// stored tuple.
	if _, err = tx.Exec(r.Context(),
		`UPDATE player_world_records SET
		    kharis_amount    = GREATEST(1.0, settled(kharis_amount, kharis_rate, kharis_calc_tick) - $1),
		    kharis_calc_tick = current_world_tick()
		 WHERE player_id = $2 AND world_id = $3`,
		riteKharisCost, playerID, worldID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not draw down divine standing")
		return
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
		"offering":         offering,
		"offer_multiplier": offerMultiplier,
		"effect":           effectPayload,
		// What the gods made of the gift. New fields on an existing event
		// (Fas 2.4: old fields keep their meaning) — without them a composed
		// offering leaves no trace of WHY the odds were what they were, and the
		// whole system becomes uncalibratable after the fact.
		"offering_worth":    offeringWorth,
		"offering_baseline": offeringBaseline,
		"offer_mod":         offerMod,
		"kharis_spent":      riteKharisCost,
	}
	_, _ = h.eventStore.Append(r.Context(), settlementID, events.StreamReligion, "RiteCast",
		eventPayload, worldID, nil)

	resp := map[string]any{
		"success":          success,
		"mood":             mood,
		"offer_multiplier": offerMultiplier,
		"prayer":           prayerID,
		"message":          message,
		"kharis_spent":     riteKharisCost,
	}
	// What the offering achieved. A playtester tripled a gift (oil 150 → 500)
	// across three casts and saw the identical failure line every time, because
	// offerMod was already at its ceiling on the first — 950 oil spent learning
	// nothing (soak 2026-07-23). The odds themselves stay hidden by design
	// (Timothy 2026-07-11: "the gods are not machines"), but whether MORE would
	// help is a fact about the offering, not about the gods, and withholding it
	// just burns the Wanax's stores.
	if offeringBaseline > 0 {
		resp["offering_worth"] = offeringWorth
		resp["offering_expected"] = offeringBaseline
		switch {
		case offerMod >= riteOfferModFat:
			resp["offering_verdict"] = "as generous as the gods will notice — more would be wasted"
		case offeringWorth >= offeringBaseline:
			resp["offering_verdict"] = "worthy — beyond what this god expects"
		default:
			// Same rounding lie as row D and the need/have pairs (helpers.go
			// shortfall): with %.0f on both figures an offering worth 4,6
			// against an expected 5,0 read "worth 5 of the ~5" and the verdict
			// "short" looked like a bug. Name the gap.
			resp["offering_verdict"] = fmt.Sprintf("short — worth %.1f of the ~%.0f this god expects, %.1f short",
				offeringWorth, offeringBaseline, offeringBaseline-offeringWorth)
		}
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
// Shares its form with the tick-level applyDivineBlessing harvest_blessing branch
// (internal/kharis/tick.go) — both read the actual RETURNING delta rather than
// reporting the 1.25 multiplier as if it were the outcome (megaron_plan_ritens_
// utfall.md: events store outcomes, not intentions, CLAUDE.md §Events).
func (h *SettlementHandler) applyHarvestBlessing(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, spec religion.PrayerSpec) (map[string]any, string, error) {
	var gained, grainNow float64
	if err := tx.QueryRow(ctx,
		`WITH old AS (
		     SELECT settled(amount, rate, calc_tick) AS grain_now, cap
		     FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'
		 ), upd AS (
		     UPDATE settlement_goods sg SET
		        amount    = LEAST(o.cap, o.grain_now * 1.25),
		        calc_tick = current_world_tick()
		     FROM old o
		     WHERE sg.settlement_id = $1 AND sg.good_key = 'grain'
		     RETURNING sg.amount - o.grain_now AS gained, o.grain_now AS grain_now
		 )
		 SELECT COALESCE(SUM(gained), 0)::float8, COALESCE(MAX(grain_now), 0)::float8 FROM upd`,
		settlementID,
	).Scan(&gained, &grainNow); err != nil {
		return nil, "", err
	}

	var msg string
	switch {
	case gained > 0:
		msg = fmt.Sprintf("%s smiles upon your fields — grain stocks swell by a quarter (+%.0f grain).", spec.God, gained)
	case grainNow <= 0:
		msg = fmt.Sprintf("%s smiles upon your fields — but your granaries stand empty, and a quarter of nothing is nothing.", spec.God)
	default:
		msg = fmt.Sprintf("%s smiles upon your fields — but your granaries are already full; there is nowhere to put the abundance.", spec.God)
	}
	return map[string]any{"good": "grain", "multiplier": 1.25, "gained": gained}, msg, nil
}

// applyOracleRevealDeposits reveals SEVEN hexes of map — an ore hex somewhere in
// the world plus its six neighbours — and lifts the fog from all of them.
//
// Canon (Timothy 2026-09-02), replacing the mechanic this function carried until
// then: the oracle no longer searches for a colonisable SITE within reach of the
// casting settlement. It picks an ore hex anywhere on the map and shows it to
// you, whether or not settling there is remotely practical. Timothy: *"Den
// kunskapen är värdefull, kan bytas även om man inte ser det som reellt möjligt
// att kolonisera där."* Knowing where the world's copper lies is worth having
// and worth trading, and the map is free text away from any other Wanax — the
// value of the reveal is not bounded by your own reach.
//
// Why the old shape had to go, measured 2026-09-02 against the acceptance world
// 198135bb: the rite searched within oracleRadius=20 hexes. All six player
// cities sat on one landmass carrying tin and no copper; the nearest copper was
// 28-41 hexes away across open water, because copper and tin never share a
// landmass by design (world.TestGenerateMap_CopperTinSeaSeparated — "bronze must
// require sea trade"). The rite was therefore STRUCTURALLY incapable of ever
// mentioning copper to any of them, and answered six casts with "no ore deposits
// lie within reach to reveal". The playtest recorded that as the agents failing
// to explore. They had not failed; their only discovery tool could not see.
//
// Knowledge does not move an army: the voyage, the colony, the mine and the
// shipping home are all unchanged, and travel time remains the real gate. What
// the radius protected was nothing; what it cost was the chain gate's bronze
// step (CLAUDE.md §Gate 1).
//
// An already-known deposit is never handed back. A rite costs kharis, an
// offering and a long cooldown, so returning something the player already has is
// a paid no-op — the failure mode ritens_utfall closed for the harvest blessing
// on 2026-09-01 and this function's own empty answer showed again. When every
// ore hex is already known the rite says exactly that, rather than pretending.
//
// Payload (nested under "effect" in the RiteCast event):
//
//	{
//	  "revealed_ore":   {"q": 1, "r": 67, "ore": "copper"},
//	  "revealed_tiles": [{"q": 0, "r": 67}, {"q": 1, "r": 67}, ...]
//	}
//
// The old key "reveals" is deliberately NOT reused. Its "q"/"r" meant the
// colonisable site; here they would mean the ore hex itself. Renaming rather than
// reinterpreting keeps every historical RiteCast event readable as what it meant
// when it was written (CLAUDE.md §Events: semantics are frozen forever). A doc
// comment here used to promise "harness/agent.py" read reveals[0] — no such
// consumer exists in the repo or the vault any more, and neither keryx nor the
// web read the effect payload at all; both render the message string.
//
// Idempotency: the reveal is INSERT ... ON CONFLICT DO NOTHING against
// player_scouted_tiles, so a retried TX is safe. A retry re-rolls the ore hex,
// which is correct — the roll's outcome is what the event records.
func (h *SettlementHandler) applyOracleRevealDeposits(
	ctx context.Context,
	tx pgx.Tx,
	settlementID, worldID, playerID uuid.UUID,
	spec religion.PrayerSpec,
) (map[string]any, string, error) {
	// The reveal patch: centre + its six neighbours = 7 hexes. This is a SIGHT
	// radius and has nothing to do with hexgrid.CatchmentRadius (the economic
	// catchment, currently 2) — they are different concepts that happen to be
	// small numbers, and unifying them later would silently change the rite.
	// Do not replace this with CatchmentRadius.
	const oracleRevealRadius = 1

	// Pick an ore hex the player does not already know, uniformly at random over
	// the whole map. Uniform over HEXES, not over metals: a metal with more hexes
	// is correspondingly likelier, which is the honest reading of "somewhere on
	// the map" and keeps an abundant metal from being as rare a find as a scarce
	// one. A hex carrying more than one deposit reports a single ore by the
	// priority below; no world generated so far produces such a hex.
	var oreQ, oreR int
	var ore string
	err := tx.QueryRow(ctx,
		`SELECT mt.q, mt.r,
		        CASE WHEN mt.copper_deposit THEN 'copper'
		             WHEN mt.tin_deposit    THEN 'tin'
		             ELSE 'silver' END
		   FROM map_tiles mt
		  WHERE mt.world_id = $1
		    AND (mt.copper_deposit OR mt.tin_deposit OR COALESCE(mt.silver_deposit, false))
		    AND NOT EXISTS (
		        SELECT 1 FROM player_scouted_tiles pst
		         WHERE pst.world_id = mt.world_id AND pst.player_id = $2
		           AND pst.q = mt.q AND pst.r = mt.r
		    )
		  ORDER BY random()
		  LIMIT 1`,
		worldID, playerID,
	).Scan(&oreQ, &oreR, &ore)
	if errors.Is(err, pgx.ErrNoRows) {
		msg := fmt.Sprintf("%s searches the world for you and finds nothing new — every vein of ore it can see is already marked on your map.", spec.God)
		return map[string]any{"revealed_ore": nil, "revealed_tiles": []any{}}, msg, nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("oracle: pick ore hex: %w", err)
	}

	// Lift the fog from the ore hex and its six neighbours. INSERT ... SELECT
	// against map_tiles so only hexes that actually exist are written, rather
	// than phantom coordinates entering the player's map memory.
	//
	// In practice that means seven hexes almost always. The sea is NOT a hole in
	// the map — coastal_sea and deep_sea are ordinary map_tiles rows (1893 of the
	// acceptance world's 2500), so a coastal deposit reveals its water too, which
	// is exactly what you want for a metal you have to sail to: the six
	// neighbours show you the approach. Only the literal outermost ring of the
	// grid yields fewer, because there is no row beyond the map's edge.
	//
	// The q/r BETWEEN bounds are redundant with the cube distance but keep the
	// (world_id, q, r) primary key usable.
	if _, err := tx.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
		 SELECT $1, $2, mt.q, mt.r
		   FROM map_tiles mt
		  WHERE mt.world_id = $1
		    AND mt.q BETWEEN $3::int - $5::int AND $3::int + $5::int
		    AND mt.r BETWEEN $4::int - $5::int AND $4::int + $5::int
		    AND (ABS(mt.q - $3::int) + ABS((mt.q - $3::int) + (mt.r - $4::int)) + ABS(mt.r - $4::int)) / 2 <= $5::int
		 ON CONFLICT DO NOTHING`,
		worldID, playerID, oreQ, oreR, oracleRevealRadius,
	); err != nil {
		return nil, "", fmt.Errorf("oracle: reveal tiles: %w", err)
	}

	// Read back what the patch actually covers, so the payload reports the reveal
	// that happened rather than the one that was intended (CLAUDE.md §Events:
	// outcomes, not intentions). ON CONFLICT DO NOTHING above means a hex the
	// player already knew is not re-inserted, but it IS part of the patch and
	// belongs in the report.
	rows, err := tx.Query(ctx,
		`SELECT mt.q, mt.r
		   FROM map_tiles mt
		  WHERE mt.world_id = $1
		    AND mt.q BETWEEN $2::int - $4::int AND $2::int + $4::int
		    AND mt.r BETWEEN $3::int - $4::int AND $3::int + $4::int
		    AND (ABS(mt.q - $2::int) + ABS((mt.q - $2::int) + (mt.r - $3::int)) + ABS(mt.r - $3::int)) / 2 <= $4::int
		  ORDER BY mt.q, mt.r`,
		worldID, oreQ, oreR, oracleRevealRadius,
	)
	if err != nil {
		return nil, "", fmt.Errorf("oracle: read revealed tiles: %w", err)
	}
	revealedTiles := []map[string]any{}
	for rows.Next() {
		var q, r int
		if scanErr := rows.Scan(&q, &r); scanErr == nil {
			revealedTiles = append(revealedTiles, map[string]any{"q": q, "r": r})
		}
	}
	rows.Close()
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", fmt.Errorf("oracle: read revealed tiles: %w", rowsErr)
	}

	msg := fmt.Sprintf("%s draws back the veil over a distant land — %s at (%d,%d), and %d hexes of map around it.",
		spec.God, ore, oreQ, oreR, len(revealedTiles))

	return map[string]any{
		"revealed_ore":   map[string]any{"q": oreQ, "r": oreR, "ore": ore},
		"revealed_tiles": revealedTiles,
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

// scaleOffering returns a prayer's traditional recipe scaled by the legacy
// offer_multiplier. Kept as the fallback path so every client that never learns
// about composed offerings keeps working exactly as before.
func scaleOffering(recipe map[string]float64, multiplier float64) map[string]float64 {
	scaled := make(map[string]float64, len(recipe))
	for good, amount := range recipe {
		scaled[good] = amount * multiplier
	}
	return scaled
}
