package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
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

	RiteDice economy.Dice       // injectable outcome seam; nil uses the production roll
	Hub      combat.Broadcaster // RiteAnswered notices; nil sends none
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
		`SELECT id, province_id, name, culture_id, control_type,
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
		ID          uuid.UUID `json:"id"`
		ProvinceID  uuid.UUID `json:"province_id"`
		Name        string    `json:"name"`
		Culture     string    `json:"culture"`
		ControlType string    `json:"control_type"`
		WallLevel   int       `json:"wall_level"`
		IsCapital   bool      `json:"is_capital"`
		State       string    `json:"state"`
		Population  int       `json:"population"`
		UpdatedAt   time.Time `json:"updated_at"`
	}
	var result []item
	for rows.Next() {
		var s item
		if err := rows.Scan(&s.ID, &s.ProvinceID, &s.Name, &s.Culture, &s.ControlType,
			&s.WallLevel, &s.IsCapital, &s.State,
			&s.Population, &s.UpdatedAt); err == nil {
			result = append(result, s)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}

// SettlementsOverview handles GET /worlds/:worldID/settlements/overview —
// every field the economy drawer's settlement list needs, per settlement the
// caller OWNS, in one call (megaron_plan_oversiktsendpoint.md, 2026-09-06).
// Replaces the old N+1 (one /provinces/{id} fetch per owned settlement) that
// backed web/static/js/megaron/ui/drawers/economy.js's loadEconomyGoods.
//
// Values come from settlementFoodSummary — the exact same economy.* calls
// ProvinceHandler.Get uses — so this endpoint and /provinces/{id} can never
// disagree about a settlement's food state (the invariant the parity test
// checks).
//
// FOW/ownership (CLAUDE.md §Trade & messenger layer, §1): only the caller's
// OWN settlements, never another Wanax's — the query is scoped by owner_id.
func (h *SettlementHandler) SettlementsOverview(w http.ResponseWriter, r *http.Request) {
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
		`SELECT id, name, is_capital, population
		 FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND state = 'active'
		 ORDER BY is_capital DESC, name`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settlements")
		return
	}

	type foodRow struct {
		ConsumedTick       *int               `json:"consumed_tick"`
		Goods              map[string]float64 `json:"goods"`
		Consumed           map[string]float64 `json:"consumed"`
		Herd               map[string]int     `json:"herd"`
		ProductionPerTick  float64            `json:"production_per_tick"`
		ConsumptionPerTick float64            `json:"consumption_per_tick"`
		CoverageTicks      float64            `json:"coverage_ticks"`

		FoodNetPerTick float64 `json:"food_net_per_tick"`
		GrowthState    string  `json:"growth_state"`
	}
	type item struct {
		AutomaticFood      economy.AutomaticFood `json:"automatic_food"`
		ID                 uuid.UUID             `json:"id"`
		Name               string                `json:"name"`
		IsCapital          bool                  `json:"is_capital"`
		Population         int                   `json:"population"`
		GrainProdRate      float64               `json:"grain_prod_rate"`
		GrainConsumRate    float64               `json:"grain_consum_rate"`
		FoodSelfSufficient bool                  `json:"food_self_sufficient"`
		ArmyUpkeep         upkeepAmount          `json:"army_upkeep"`
		FoodNetAfterUpkeep float64               `json:"food_net_per_tick_after_upkeep"`
		Food               foodRow               `json:"food"`
	}

	type settRow struct {
		id         uuid.UUID
		name       string
		isCapital  bool
		population int
	}
	var settRows []settRow
	for rows.Next() {
		var s settRow
		if scanErr := rows.Scan(&s.id, &s.name, &s.isCapital, &s.population); scanErr == nil {
			settRows = append(settRows, s)
		}
	}
	rows.Close()

	result := make([]item, 0, len(settRows))
	for _, s := range settRows {
		fs := settlementFoodSummary(r.Context(), h.pool, s.id, s.population)
		up, _, err := settlementUpkeepDrain(r.Context(), h.pool, s.id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not compute army upkeep")
			return
		}
		result = append(result, item{
			ID:                 s.id,
			Name:               s.name,
			IsCapital:          s.isCapital,
			Population:         s.population,
			GrainProdRate:      fs.GrainProdRate,
			GrainConsumRate:    fs.GrainConsumRate,
			FoodSelfSufficient: fs.FoodSelfSufficient,
			AutomaticFood:      fs.AutomaticFood,
			ArmyUpkeep:         up,
			FoodNetAfterUpkeep: foodNetAfterArmy(fs.FoodNetPerTick, up.Grain),
			Food: foodRow{Herd: fs.Herd, ConsumedTick: fs.ConsumedTick, Goods: fs.Goods, Consumed: fs.Consumed, ProductionPerTick: fs.ProductionPerTick, ConsumptionPerTick: fs.ConsumptionPerTick,
				CoverageTicks: fs.CoverageTicks,

				FoodNetPerTick: fs.FoodNetPerTick,
				GrowthState:    fs.GrowthState,
			},
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// PlacementRoster handles GET /worlds/:worldID/settlements/placement-roster —
// a realm-wide roll of every placed gubbe grouped by settlement, so a Wanax
// can see where their citizens work (and how many sit idle) without opening
// each settlement one at a time. Player reports 19ed51f1/54f2b747, 2026-09-04:
// "se en lista över våra 'gubbar' och var de är och arbetar utan att behöva
// gå in på särskilda hexar … för att minska bemanningen och frigöra gubbar."
//
// This is the aggregate substrate under both `keryx roster` and (a later
// slice) the web roster — one call, not the N+1 a client would otherwise make
// hitting /provinces/{id}/placements per settlement (same reason
// SettlementsOverview exists).
//
// Ownership/FOW (CLAUDE.md §Trade & messenger layer, §1): owner_id-scoped —
// only the caller's OWN settlements, never another Wanax's gubbar. Hex
// ordinals are computed here (hexgrid.RingOrdinal against each settlement's
// own centre) so the numbers match `keryx city`/place exactly — no client
// duplicates the ring math. Temple service is an ordinary building assignment.
func (h *SettlementHandler) PlacementRoster(w http.ResponseWriter, r *http.Request) {
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

	type settRow struct {
		id         uuid.UUID
		provinceID uuid.UUID
		name       string
		isCapital  bool
		population int
		center     hexgrid.Coord
	}
	rows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.province_id, s.name, s.is_capital, s.population, p.map_q, p.map_r
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.state = 'active'
		 ORDER BY s.is_capital DESC, s.name`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settlements")
		return
	}
	var settRows []settRow
	settIdx := make(map[uuid.UUID]int)
	for rows.Next() {
		var s settRow
		if scanErr := rows.Scan(&s.id, &s.provinceID, &s.name, &s.isCapital, &s.population, &s.center.Q, &s.center.R); scanErr == nil {
			settIdx[s.id] = len(settRows)
			settRows = append(settRows, s)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settlements")
		return
	}

	// One pass over every owned settlement's placements (2 queries total, not
	// N+1). Group by (target_kind, location, good) into a count — the same
	// shape `keryx city` shows per hex/building row.
	prows, err := h.pool.Query(r.Context(),
		`SELECT sp.settlement_id, sp.target_kind, sp.hex_q, sp.hex_r, sp.building_type, sp.good_key
		 FROM settlement_placement sp
		 JOIN settlements s ON s.id = sp.settlement_id
		 WHERE s.world_id = $1 AND s.owner_id = $2`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load placements")
		return
	}
	type assignKey struct {
		kind         string
		hexOrdinal   int
		buildingType string
		good         string
	}
	type assignAgg struct {
		key   assignKey
		hexQ  int
		hexR  int
		count int
	}
	// aggregation per settlement, keyed by assignKey, plus placed count.
	perSett := make(map[uuid.UUID]map[assignKey]*assignAgg, len(settRows))
	placedCount := make(map[uuid.UUID]int, len(settRows))
	for prows.Next() {
		var sid uuid.UUID
		var kind, good string
		var hexQ, hexR *int
		var buildingType *string
		if scanErr := prows.Scan(&sid, &kind, &hexQ, &hexR, &buildingType, &good); scanErr != nil {
			prows.Close()
			writeError(w, http.StatusInternalServerError, "could not read placement")
			return
		}
		idx, known := settIdx[sid]
		if !known {
			continue // placement for a non-active settlement — skip
		}
		placedCount[sid]++
		k := assignKey{kind: kind, good: good}
		var q, rr int
		if kind == "hex" && hexQ != nil && hexR != nil {
			q, rr = *hexQ, *hexR
			if ord, found := hexgrid.RingOrdinal(settRows[idx].center, hexgrid.CatchmentRadius, hexgrid.Coord{Q: q, R: rr}); found {
				k.hexOrdinal = ord
			}
		}
		if kind == "building" && buildingType != nil {
			k.buildingType = *buildingType
		}
		bucket := perSett[sid]
		if bucket == nil {
			bucket = make(map[assignKey]*assignAgg)
			perSett[sid] = bucket
		}
		if agg := bucket[k]; agg != nil {
			agg.count++
		} else {
			bucket[k] = &assignAgg{key: k, hexQ: q, hexR: rr, count: 1}
		}
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read placements")
		return
	}

	type assignOut struct {
		GoodKey      string `json:"good_key"`
		TargetKind   string `json:"target_kind"`
		HexOrdinal   *int   `json:"hex_ordinal,omitempty"`
		HexQ         *int   `json:"hex_q,omitempty"`
		HexR         *int   `json:"hex_r,omitempty"`
		BuildingType string `json:"building_type,omitempty"`
		Count        int    `json:"count"`
	}
	type settOut struct {
		ID          uuid.UUID   `json:"id"`
		ProvinceID  uuid.UUID   `json:"province_id"`
		Name        string      `json:"name"`
		IsCapital   bool        `json:"is_capital"`
		TotalGubbar int         `json:"total_gubbar"`
		Placed      int         `json:"placed"`
		Idle        int         `json:"idle"`
		Assignments []assignOut `json:"assignments"`
	}

	result := make([]settOut, 0, len(settRows))
	for _, s := range settRows {
		total := s.population / 100
		placed := placedCount[s.id]
		assigns := make([]assignOut, 0, len(perSett[s.id]))
		for _, agg := range perSett[s.id] {
			a := assignOut{
				GoodKey:      agg.key.good,
				TargetKind:   agg.key.kind,
				BuildingType: agg.key.buildingType,
				Count:        agg.count,
			}
			if agg.key.kind == "hex" {
				q, rr := agg.hexQ, agg.hexR
				a.HexQ, a.HexR = &q, &rr
				if agg.key.hexOrdinal > 0 {
					ord := agg.key.hexOrdinal
					a.HexOrdinal = &ord
				}
			}
			assigns = append(assigns, a)
		}
		// Deterministic order: buildings after hexes, then by ordinal /
		// building, then good — so the roster (and its test) reads the same
		// on every call regardless of map iteration order.
		sort.Slice(assigns, func(i, j int) bool {
			ai, aj := assigns[i], assigns[j]
			if ai.TargetKind != aj.TargetKind {
				return ai.TargetKind < aj.TargetKind // "building" < "hex"
			}
			if ai.TargetKind == "hex" {
				oi, oj := 0, 0
				if ai.HexOrdinal != nil {
					oi = *ai.HexOrdinal
				}
				if aj.HexOrdinal != nil {
					oj = *aj.HexOrdinal
				}
				if oi != oj {
					return oi < oj
				}
			} else if ai.BuildingType != aj.BuildingType {
				return ai.BuildingType < aj.BuildingType
			}
			return ai.GoodKey < aj.GoodKey
		})
		result = append(result, settOut{
			ID:          s.id,
			ProvinceID:  s.provinceID,
			Name:        s.name,
			IsCapital:   s.isCapital,
			TotalGubbar: total,
			Placed:      placed,
			Idle:        total - placed,
			Assignments: assigns,
		})
	}
	writeJSONETag(w, r, result)
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
		"wall_level":          sett.WallLevel,
		"population_cap":      economy.PopulationCap(sett.WallLevel),
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

	own := sett.OwnerID != nil && *sett.OwnerID == playerID
	if board := marketBoardView(r.Context(), h.pool, worldID, sett.ID, playerID, own); board != nil {
		resp["market_board"] = board
	}

	// Only owner sees the full resources; others see limited info.
	if !own {
		delete(resp, "resources")
		delete(resp, "army")
		resp["owner_id"] = sett.OwnerID
	}

	writeJSON(w, http.StatusOK, resp)
}

// Gift is a tombstone for the retired own-colony loyalty gesture.
// Logistics uses province trade/transfer; foreign gifts use the same physical transport.
func (h *SettlementHandler) Gift(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusGone, "Gifts to your own colonies have been removed; use a transfer to move goods between your cities.")
}

// ReturnArmy handles POST /worlds/:worldID/settlements/:settlementID/return-army.
// The king returns a borrowed army to its settlement.
//
// NOTE (SB7): the restore SQL below still writes the retired settlements.* army
// columns; like its counterpart KingdomHandler.BorrowArmy it must be rebuilt on the
// units model BEFORE kingdoms are re-enabled. The server registers this endpoint
// behind requireKingdomsEnabled (default off; kingdoms are POST-MVP). A nonempty
// KINGDOMS_ENABLED exposes this legacy handler, so enabling the switch is not a
// substitute for rebuilding it. See megaron_arkitekturprogram, step 0.
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
		writeError(w, http.StatusInternalServerError, "could not return army forces")
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
// Everyday prayers (GodID set) answer when standing and offering requirements
// are met. Legacy prayers retain the continuous, hidden success probability.
// Standing is a requirement for every prayer, never a payment.
//
// The prayer must belong to the settlement's culture (403 otherwise).
// The prayer must be off cooldown (409 otherwise).
// Legacy outcomes are rolled once; every outcome is stored in the RiteCast event.
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
	var nowTick int
	err = tx.QueryRow(r.Context(),
		`SELECT culture_id, (battle_frenzy_until IS NOT NULL AND battle_frenzy_until > $4)
		 FROM settlements
		 WHERE id = $1 AND world_id = $2 AND owner_id = $3
		 FOR UPDATE`,
		settlementID, worldID, playerID, h.clk.Now(),
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

	// A prayer is said at its own god's temple (ORDER 15); a legacy prayer
	// without a god (empty GodID) still takes any temple.
	var hasTemple bool
	_ = tx.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'temple' AND ($2 = '' OR god_id = $2))`,
		settlementID, spec.GodID,
	).Scan(&hasTemple)
	if !hasTemple {
		// Distinguish "no temple at all" from "temple finishing" — the latter is
		// the surprise a Wanax hits performing a rite right after a temple build
		// shows done: the BuildComplete event (which inserts the buildings row) is
		// a poll away. Mirror the foundry-422 fix (df3a77b) so the error names the
		// real cause instead of a bare "temple required".
		var queued bool
		_ = tx.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM build_queue WHERE settlement_id = $1 AND building_type = 'temple' AND ($2 = '' OR god_id = $2))`,
			settlementID, spec.GodID,
		).Scan(&queued)
		if queued {
			writeError(w, http.StatusBadRequest,
				"temple is still finishing here — it becomes usable within a game day of completion; retry shortly")
		} else if spec.GodID != "" {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("%s is prayed at the %s — build it here first (`keryx build --type temple --god %s`)",
					spec.Name, religion.TempleName(spec.GodID), spec.GodID))
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

	if err := tx.QueryRow(r.Context(), `SELECT current_tick FROM worlds WHERE id=$1`, worldID).Scan(&nowTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read world time")
		return
	}
	if spec.GodID != "" {
		active, activeErr := religion.PrayerActiveRemaining(r.Context(), tx, settlementID, playerID, spec.EffectType, nowTick)
		if activeErr != nil {
			writeError(w, http.StatusInternalServerError, "could not read active prayer")
			return
		}
		if active > 0 {
			if spec.EffectType == religion.EffectNextLandDeparture || spec.EffectType == religion.EffectNextShipDeparture {
				writeError(w, http.StatusConflict, "prayer awaits your next eligible departure from this city")
				return
			}
			writeError(w, http.StatusConflict, "prayer effect already active — wait for it to end")
			return
		}
	}

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

	// The temple rests, not the god or the Wanax (bönegrunden): the temple this
	// prayer is said at, and only it, must be ready.
	var templeID uuid.UUID
	var templeLevel int
	if spec.GodID != "" {
		if scanErr := tx.QueryRow(r.Context(),
			`SELECT id, level FROM buildings WHERE settlement_id = $1 AND building_type = 'temple' AND god_id = $2 ORDER BY id LIMIT 1`,
			settlementID, spec.GodID,
		).Scan(&templeID, &templeLevel); scanErr != nil {
			writeError(w, http.StatusInternalServerError, "could not read the temple")
			return
		}
		remaining, cdErr := religion.TempleRestRemaining(r.Context(), tx, templeID)
		if cdErr != nil {
			writeError(w, http.StatusInternalServerError, "could not read the temple's rest")
			return
		}
		if remaining > 0 {
			staff, staffErr := economy.LoadTempleStaffing(r.Context(), tx, settlementID)
			if staffErr != nil {
				writeError(w, http.StatusInternalServerError, "could not read temple staffing")
				return
			}
			staffed, capacity := 0, 0
			for _, t := range staff.Temples {
				if t.BuildingID == templeID {
					staffed, capacity = t.Staffed, t.Capacity
				}
			}
			writeError(w, http.StatusConflict, templeRestMessage(religion.TempleName(spec.GodID), remaining, staffed, capacity))
			return
		}
	}
	// Cooldown check: query last successful RiteCast for this (player, prayer, temple) from events table.
	// Column-free: uses the existing event log, no new schema.
	// Keyed on stream_id = settlementID so the cooldown is per TEMPLE, not per Wanax:
	// a Wanax with temples in five cities can cast five prayers per cycle (one per city).
	if spec.GodID == "" && spec.CooldownTicks > 0 {
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
						prayerID, tick.FormatDays(tick.GameDaysLeft(remaining))))
				return
			}
		}
		// ErrNoRows = never cast before = allowed.
	}

	// Determine success probability — continuous (FAS 1), not tiered: the kharis
	// level IS the success percentage, nudged by how much was offered.
	offerMultiplier := riteOfferMultiplier(body.OfferMultiplier)
	// An everyday prayer (one with a god, R4 Timothy 2026-10-22) is answered
	// whenever its kharis requirement is met: it takes its traditional gift or
	// one worth as much, never a fatter or stingier one.
	everyday := spec.GodID != ""
	if everyday {
		offerMultiplier = 1.0
	}
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
	divineValues, dvErr := religion.LoadDivineValues(r.Context(), h.pool, worldID)
	if dvErr != nil {
		writeError(w, http.StatusInternalServerError, "could not read the gods' reckoning")
		return
	}
	composed := len(body.Offering) > 0 && len(divineValues) > 0
	if everyday && len(body.Offering) > 0 {
		if len(divineValues) == 0 {
			writeError(w, http.StatusServiceUnavailable, "the temple cannot value a composed offering yet — use the traditional offering")
			return
		}
		for good, amount := range body.Offering {
			if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || divineValues[good] <= 0 {
				writeError(w, http.StatusBadRequest, "a composed offering needs positive amounts of goods the temple can value")
				return
			}
		}
	}
	if composed {
		offering = body.Offering
	}
	favours := religion.FavoursFor(spec)
	offeringWorth := religion.OfferingWorth(offering, divineValues, favours)
	offeringBaseline := religion.TraditionalBaseline(spec, divineValues)
	offerCap := riteOfferModFat
	if composed && !everyday {
		offerMod, offerCap = religion.ComposedOfferMod(offering, divineValues, favours, offeringBaseline)
	}
	// r: how much larger than the traditional gift this one is. The gift sets
	// how long a lasting prayer holds and how long the temple rests.
	giftFactor := 1.0
	if everyday && composed {
		giftFactor = religion.GiftFactor(offeringWorth, offeringBaseline)
	}
	if everyday && composed && offeringWorth < offeringBaseline*(1-1e-9) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("%s asks for a gift worth at least the traditional offering (%s)", spec.God, formatOffering(spec.Offering)))
		return
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
	// sink that makes religion drive trade. The MinKharis gate above checks
	// eligibility; neither a success nor a refusal spends standing.
	// `offering` was resolved above: either the composed one the Wanax brought or
	// the prayer's traditional recipe scaled by offer_multiplier.
	for good, need := range offering {
		var have float64
		if scanErr := tx.QueryRow(r.Context(),
			`SELECT GREATEST(0, settled(amount, rate, calc_tick))
			 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2 FOR UPDATE`,
			settlementID, good,
		).Scan(&have); scanErr != nil || !province.GoodsAffordable(have, need) {
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

	// Kharis is the relationship, not a currency (R1 Timothy 2026-10-22): a
	// prayer pays in goods and waiting time; standing is a requirement, never spent.

	// Everyday prayers never consult the dice. Legacy outcomes are rolled once.
	success := everyday
	if !everyday {
		var roll int
		if h.RiteDice != nil {
			roll = h.RiteDice.Intn(100)
		} else {
			roll = rand.Intn(100)
		}
		success = roll < chance
	}

	// Apply effect on success.
	effectPayload := map[string]any{}
	var message string
	durationTicks := 0 // set by the lasting prayers (garrison defence, harvest blessing)

	if success {
		switch spec.EffectType {
		case religion.EffectOracleMemory:
			effectPayload, message, err = h.applyOracleMemory(r.Context(), tx, worldID, playerID, spec)
		case religion.EffectNextLandDeparture, religion.EffectNextShipDeparture:
			_, err = tx.Exec(r.Context(), `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,$3) ON CONFLICT(settlement_id,effect) DO UPDATE SET owner_id=EXCLUDED.owner_id`, settlementID, playerID, spec.EffectType)
			effectPayload = map[string]any{"departure_pending": true, "time_factor": religion.DepartureTimeFactor}
			message = "Asterion's thread waits for your next land caravan or Runner from this city."
			if spec.EffectType == religion.EffectNextShipDeparture {
				message = "The Sea Daimons of Stillness shelter the next ship departing this port — faster sailing and no storm damage on its outward leg."
			}
		case religion.EffectGarrisonDefense:
			durationTicks = religion.DurationTicks(combat.GarrisonDefenseTicks, giftFactor)
			until := nowTick + durationTicks
			_, err = tx.Exec(r.Context(), `UPDATE settlements SET garrison_defense_until_tick=$2,garrison_defense_owner_id=$3 WHERE id=$1`, settlementID, until, playerID)
			effectPayload = map[string]any{"until_tick": until, "strength_factor": combat.GarrisonDefenseFactor, "duration_ticks": durationTicks}
			message = fmt.Sprintf("Potnia shelters your walls — your own garrison defends this city a quarter stronger for %s.", tick.FormatDays(durationTicks))
		case religion.EffectBattleFrenzy:
			effectPayload, message, err = h.applyBattleFrenzy(r.Context(), tx, settlementID)
		case religion.EffectOracleRevealDeposits:
			effectPayload, message, err = h.applyOracleRevealDeposits(r.Context(), tx, settlementID, worldID, playerID, spec)
		case religion.EffectHarvestBlessing:
			durationTicks = religion.DurationTicks(economy.HarvestBlessingTicks, giftFactor)
			effectPayload, message, err = h.applyHarvestBlessing(r.Context(), tx, settlementID, spec, durationTicks)
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

	// Effect, successful-only waiting time and outcome are one transaction.
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
		"offering_work":          offeringWorth,
		"offering_work_baseline": offeringBaseline,
		"offering_value_unit":    religion.LaborValueUnit,
		"valuation_version":      religion.LaborValuationVersion,
		"offer_mod":              offerMod,
		"kharis_spent":           0.0,
	}
	restTicks := 0
	if success && spec.GodID != "" {
		restTicks = religion.RestTicks(spec.CooldownTicks, giftFactor, templeLevel)
		if err = religion.SetTempleRest(r.Context(), tx, templeID, restTicks, nowTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not record the temple's rest")
			return
		}
		eventPayload["temple_building_id"] = templeID.String()
		eventPayload["rest_ticks"] = restTicks
		eventPayload["gift_factor"] = giftFactor
		if durationTicks > 0 {
			eventPayload["duration_ticks"] = durationTicks
		}
	}
	eventPayload["god_id"] = spec.GodID
	eventPayload["cast_tick"] = nowTick
	riteEvent, err := h.eventStore.AppendTx(r.Context(), tx, settlementID, events.StreamReligion, "RiteCast", eventPayload, worldID, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record prayer outcome")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	h.eventStore.RecordCommitted(r.Context(), riteEvent)
	h.notifyRiteAnswered(r.Context(), worldID, playerID, settlementID, spec, success, message, effectPayload)

	resp := map[string]any{
		"success":          success,
		"mood":             mood,
		"offer_multiplier": offerMultiplier,
		"prayer":           prayerID,
		"message":          message,
		"kharis_spent":     0.0,
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
		resp["offering_work"] = offeringWorth
		resp["offering_value_unit"] = religion.LaborValueUnit
		resp["valuation_version"] = religion.LaborValuationVersion
		resp["offering_expected"] = offeringBaseline
		switch {
		case everyday:
			if durationTicks > 0 {
				resp["offering_verdict"] = "worthy — a larger gift lasts longer, and the temple rests longer"
			} else {
				resp["offering_verdict"] = "worthy — meets the prayer's requirement; more will not improve the answer, only lengthen the temple's rest"
			}
		case offerMod >= offerCap:
			resp["offering_verdict"] = "as generous as the gods will notice — more would be wasted"
		case math.Abs(offeringWorth-offeringBaseline) < 1e-9:
			resp["offering_verdict"] = "worthy — meets what this god expects"
		case offeringWorth > offeringBaseline:
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
		if restTicks > 0 {
			resp["rest_game_days"] = restTicks
			resp["temple_level"] = templeLevel
		}
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

// applyHarvestBlessing raises the yield of every food good but fish by a
// quarter for economy.HarvestBlessingTicks (Timothy 2026-10-10). It was a
// one-shot ×1.25 on the grain store, which gave the most to the fullest
// food store and nothing to an empty one. Shared with the divine favour
// (kharis/tick.go "bountiful_harvest") through economy.ApplyHarvestBlessing.
func (h *SettlementHandler) applyHarvestBlessing(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, spec religion.PrayerSpec, ticks int) (map[string]any, string, error) {
	untilTick, err := economy.ApplyHarvestBlessingFor(ctx, tx, settlementID, ticks)
	if err != nil {
		return nil, "", err
	}
	msg := fmt.Sprintf("%s smiles upon your fields — grain, oil and wine yield a quarter more for %s.",
		spec.God, tick.FormatDays(ticks))
	return map[string]any{
		"goods":          economy.HarvestBlessedGoods(),
		"yield_bonus":    economy.HarvestBlessingYieldBonus,
		"days":           ticks,
		"duration_ticks": ticks,
		"until_tick":     untilTick,
	}, msg, nil
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
	var revealedHexes []province.MapPosition
	for rows.Next() {
		var q, r int
		if scanErr := rows.Scan(&q, &r); scanErr == nil {
			revealedTiles = append(revealedTiles, map[string]any{"q": q, "r": r})
			revealedHexes = append(revealedHexes, province.MapPosition{Q: q, R: r})
		}
	}
	rows.Close()
	if err := province.RecordSeenSettlements(ctx, tx, worldID, playerID, revealedHexes); err != nil {
		return nil, "", fmt.Errorf("oracle: record seen cities: %w", err)
	}
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

// notifyRiteAnswered tells the one who prayed what the god answered
// (byggplan A.2). It is their own act and the omen exception covers a find
// (Timothy 2026-10-12), so it goes at once. q/r is the find when there is one;
// otherwise the dispatch resolves to the city. revealed_tiles lets the client
// show the reveal. Best-effort after commit, like the other notices.
func (h *SettlementHandler) notifyRiteAnswered(ctx context.Context, worldID, playerID, settlementID uuid.UUID,
	spec religion.PrayerSpec, success bool, message string, effect map[string]any) {
	if h.Hub == nil {
		return
	}
	body := map[string]any{
		"settlement_id": settlementID,
		"god_id":        spec.GodID,
		"god":           spec.God,
		"prayer":        spec.ID,
		"prayer_name":   spec.Name,
		"success":       success,
		"message":       message,
	}
	var name string
	if err := h.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id=$1`, settlementID).Scan(&name); err == nil {
		body["name"] = name
	}
	if ore, ok := effect["revealed_ore"].(map[string]any); ok {
		body["q"], body["r"], body["ore"] = ore["q"], ore["r"], ore["ore"]
		body["revealed_tiles"] = effect["revealed_tiles"]
	}
	_ = h.Hub.NotifyPlayer(ctx, worldID, playerID, "RiteAnswered", 3, body)
}

// Abandon handles POST /worlds/:worldID/settlements/:settlementID/abandon.
//
// Voluntarily gives up a colony: the garrison is disbanded, the settlement's own
// province is freed, and the row is marked state='abandoned'. This is the
// consolidation valve that pairs with the MaxSettlementsPerWanax cap —
// abandoning frees a slot (the cap counts state='active').
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
		`SELECT ship.id, passenger.id FROM units ship LEFT JOIN units passenger ON passenger.carried_by_ship_id = ship.id WHERE ship.settlement_id = $1 AND ship.status = 'garrison'`,
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

	// Free the settlement's own province so the hex is colonisable again.
	if _, err := tx.Exec(r.Context(),
		`UPDATE provinces SET territory_state = 'free'
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
		`SELECT id, source_region, category, text, generated_at, importance, hops,fact_data
		 FROM gossip_events
		 WHERE world_id = $1 AND recipient_id = $2
 AND (category<>'omen' OR fact_data IS NOT NULL)
 AND (transport_owner_id IS NULL OR transport_owner_id<>$2)
 AND NOT (transport_owner_id IS NULL AND category='economy' AND text LIKE '% flows from % to %.' AND EXISTS(SELECT 1 FROM settlements s WHERE s.id=subject_settlement_id AND s.owner_id=$2))
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
		ID           uuid.UUID       `json:"id"`
		SourceRegion string          `json:"source_region"`
		Category     string          `json:"category"`
		Text         string          `json:"text"`
		GeneratedAt  time.Time       `json:"generated_at"`
		Importance   string          `json:"importance"`
		Hops         int             `json:"hops"`
		Omen         json.RawMessage `json:"omen,omitempty"`
	}
	var result []item
	for rows.Next() {
		var g item
		if err := rows.Scan(&g.ID, &g.SourceRegion, &g.Category, &g.Text, &g.GeneratedAt, &g.Importance, &g.Hops, &g.Omen); err == nil {
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

// formatOffering writes a recipe as "10 grain + 3 silver" in a stable order.
func formatOffering(recipe map[string]float64) string {
	goods := make([]string, 0, len(recipe))
	for good := range recipe {
		goods = append(goods, good)
	}
	sort.Strings(goods)
	parts := make([]string, len(goods))
	for i, good := range goods {
		parts[i] = fmt.Sprintf("%g %s", recipe[good], good)
	}
	return strings.Join(parts, " + ")
}

// templeRestMessage is the 409 text for a resting temple: which temple, how
// long, and — when it is short of servants — that the rest stands still.
func templeRestMessage(temple string, days, staffed, capacity int) string {
	msg := fmt.Sprintf("The %s rests for another %s", temple, tick.FormatDays(days))
	if capacity > 0 && staffed < capacity {
		msg += fmt.Sprintf(" — its rest stands still until it has all its servants (%d of %d servants)", staffed, capacity)
	}
	return msg
}
