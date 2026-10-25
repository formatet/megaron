package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"formatet/megaron/server/internal/unit/shipnames"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxParallelBuilds caps how many buildings a single settlement may have
// queued at once (Build's queue guard, below). Hoisted to package scope
// (P10, soak 2026-07-18) so Get can also surface it as "build_queue_max"
// alongside "build_queue" — before this it only ever appeared in the 422
// error a player got AFTER trying to queue a 3rd building.
const maxParallelBuilds = 2

// ProvinceHandler handles HTTP requests for province endpoints.
type ProvinceHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock

	eventStore *events.Store // may be nil in tests that don't exercise Recruit's naval path
	hub        *notify.Hub   // nil-guarded; carries player-facing notifications (trade, arrivals, ...)
}

// NewProvinceHandler creates a ProvinceHandler.
func NewProvinceHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, clk clock.Clock, eventStore *events.Store, hub *notify.Hub) *ProvinceHandler {
	return &ProvinceHandler{pool: pool, scheduler: scheduler, clk: clk, eventStore: eventStore, hub: hub}
}

// Get handles GET /worlds/:worldID/provinces/:provinceID.
func (h *ProvinceHandler) Get(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}

	prov, err := loadTerrainProvince(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		// The CLI resolver (keryx --province) now catches this client-side, but direct
		// API callers (iOS client, curl) can still pass a settlement ID where a province
		// ID is expected — check for that before giving a bare 404.
		var sName string
		var sProvinceID uuid.UUID
		if sErr := h.pool.QueryRow(r.Context(),
			`SELECT name, province_id FROM settlements WHERE id = $1 AND world_id = $2`,
			provinceID, worldID,
		).Scan(&sName, &sProvinceID); sErr == nil {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("that is a settlement ID, not a province ID — settlement %q sits in province %s; retry with --province %s (or just the settlement name)",
					sName, sProvinceID, sProvinceID))
			return
		}
		writeError(w, http.StatusNotFound, "province not found")
		return
	}

	// Collect deposit types present in the productive catchment ring
	// (hexgrid.CatchmentRadius around the settlement's own hex, P1 — the own
	// hex is excluded, same as every production query) so clients/agents can
	// decide whether building a mine is worthwhile.
	var catchmentDeposits []string
	catchQ, catchR := hexgrid.QRArrays(hexgrid.Ring(hexgrid.Coord{Q: prov.MapTile.Q, R: prov.MapTile.R}, hexgrid.CatchmentRadius))
	cdrows, _ := h.pool.Query(r.Context(),
		`SELECT DISTINCT
		    CASE WHEN copper_deposit THEN 'copper' END,
		    CASE WHEN tin_deposit    THEN 'tin'    END,
		    CASE WHEN COALESCE(silver_deposit,false) THEN 'silver' END,
		    CASE WHEN COALESCE(cedar_deposit, false) THEN 'cedar'  END,
		    CASE WHEN COALESCE(incense_deposit, false) THEN 'incense' END
		 FROM map_tiles mt
		 JOIN unnest($2::int[], $3::int[]) AS catchment(q, r) ON mt.q = catchment.q AND mt.r = catchment.r
		 WHERE mt.world_id = $1
		   AND mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')`,
		worldID, catchQ, catchR,
	)
	if cdrows != nil {
		seen := make(map[string]bool)
		for cdrows.Next() {
			var copper, tin, silver, cedar, incense *string
			_ = cdrows.Scan(&copper, &tin, &silver, &cedar, &incense)
			for _, v := range []*string{copper, tin, silver, cedar, incense} {
				if v != nil && !seen[*v] {
					seen[*v] = true
					catchmentDeposits = append(catchmentDeposits, *v)
				}
			}
		}
		cdrows.Close()
	}
	if catchmentDeposits == nil {
		catchmentDeposits = []string{}
	}

	resp := map[string]any{
		"id":                 prov.ID,
		"world_id":           prov.WorldID,
		"map_tile":           prov.MapTile,
		"terrain_type":       prov.TerrainType,
		"territory_state":    prov.TerritoryState,
		"coastal":            prov.Coastal,
		"copper_deposit":     prov.CopperDeposit,
		"tin_deposit":        prov.TinDeposit,
		"silver_deposit":     prov.SilverDeposit,
		"cedar_deposit":      prov.CedarDeposit,
		"incense_deposit":    prov.IncenseDeposit,
		"catchment_deposits": catchmentDeposits,
	}

	sett, err := loadSettlementByProvince(r.Context(), h.pool, provinceID, worldID)
	if err == nil {
		now := h.clk.Now()

		// Build queue — include so API clients don't need a separate endpoint.
		type buildItem struct {
			ID         uuid.UUID `json:"id"`
			Type       string    `json:"type"`
			CreatedAt  time.Time `json:"created_at"`
			CompleteAt time.Time `json:"complete_at"`
			GodID      string    `json:"god_id,omitempty"`
			Name       string    `json:"name,omitempty"`
		}
		var buildQueue []buildItem
		qrows, _ := h.pool.Query(r.Context(),
			`SELECT id, building_type, created_at, complete_at, coalesce(god_id,'') FROM build_queue
			 WHERE settlement_id = $1 ORDER BY complete_at`,
			sett.ID,
		)
		if qrows != nil {
			for qrows.Next() {
				var bi buildItem
				_ = qrows.Scan(&bi.ID, &bi.Type, &bi.CreatedAt, &bi.CompleteAt, &bi.GodID)
				if bi.GodID != "" {
					bi.Name = religion.TempleName(bi.GodID)
				}
				buildQueue = append(buildQueue, bi)
			}
			qrows.Close()
		}
		if buildQueue == nil {
			buildQueue = []buildItem{}
		}

		// Units still maturing — the recruit pipeline before a unit is deployable.
		// One row per unit so the client can render the lifecycle directly:
		//   land forming   (size < 100)      → "80/100 forming" (gathering men)
		//   land training  (size = 100)      → "100/100 training — ready HH:MM"
		//   naval forming  (a vessel builds) → "building — ready HH:MM"
		// ready_at is the unit's build_complete_at (null for a still-gathering
		// forming land unit, which has no timer yet). Replaces the old per-batch
		// training_queue + forming_units fields (the per-10 batch model is gone).
		type trainingUnit struct {
			Unit                  string     `json:"unit"`
			Size                  int        `json:"size"`
			Status                string     `json:"status"`
			Category              string     `json:"category"`
			ReadyAt               *time.Time `json:"ready_at,omitempty"`
			ShipyardWaiting       bool       `json:"shipyard_waiting"`
			ShipyardWorkRemaining *int       `json:"shipyard_work_remaining,omitempty"`
		}
		var trainingUnits []trainingUnit
		if trows, terr := h.pool.Query(r.Context(),
			`SELECT id,type, size, status, category, build_complete_at
			 FROM units
			 WHERE settlement_id = $1 AND status IN ('forming', 'training')
			 ORDER BY category, status DESC, created_at`,
			sett.ID,
		); terr == nil {
			for trows.Next() {
				var tu trainingUnit
				var id uuid.UUID
				if trows.Scan(&id, &tu.Unit, &tu.Size, &tu.Status, &tu.Category, &tu.ReadyAt) == nil {
					work, err := economy.LoadShipyardWork(r.Context(), h.pool, id)
					if err != nil {
						trows.Close()
						writeError(w, http.StatusInternalServerError, "could not read shipyard work")
						return
					}
					if work.Remaining != nil {
						tu.ReadyAt = work.CompleteAt
						tu.ShipyardWaiting = work.Waiting
						tu.ShipyardWorkRemaining = work.Remaining
					}
					trainingUnits = append(trainingUnits, tu)
				}
			}
			trows.Close()
		}
		if trainingUnits == nil {
			trainingUnits = []trainingUnit{}
		}

		// Buildings — already completed (agents/clients use this to avoid re-queuing).
		// HexQ/HexR are set only for hex-bound types (province.HexBoundBuildings) —
		// null for a city building (megaron_plan_byggnad_pa_hex.md §A). This is
		// the field `keryx city`/`keryx status` read to show a hex-bound
		// building's location.
		type buildingItem struct {
			ID    uuid.UUID `json:"id"`
			Type  string    `json:"type"`
			Level int       `json:"level"`
			HexQ  *int      `json:"hex_q,omitempty"`
			HexR  *int      `json:"hex_r,omitempty"`
			// A temple's god and name ("Temple of Potnia"); empty otherwise.
			GodID string `json:"god_id,omitempty"`
			Name  string `json:"name,omitempty"`
		}
		var buildings []buildingItem
		brows, _ := h.pool.Query(r.Context(),
			`SELECT id, building_type, level, hex_q, hex_r, coalesce(god_id,'') FROM buildings WHERE settlement_id = $1 ORDER BY building_type, god_id, hex_q, hex_r`,
			sett.ID,
		)
		if brows != nil {
			for brows.Next() {
				var bi buildingItem
				_ = brows.Scan(&bi.ID, &bi.Type, &bi.Level, &bi.HexQ, &bi.HexR, &bi.GodID)
				if bi.GodID != "" {
					bi.Name = religion.TempleName(bi.GodID)
				}
				buildings = append(buildings, bi)
			}
			brows.Close()
		}
		if buildings == nil {
			buildings = []buildingItem{}
		}

		// Part B: labor_pool = population. Soldiers are extracted from population at
		// recruit time, so army columns are no longer a labor drain.
		laborPool := sett.Population
		if laborPool < 0 {
			laborPool = 0
		}

		// Load current goods amounts for affordability checks.
		goodsStock := make(map[string]float64)
		gsrows, _ := h.pool.Query(r.Context(),
			`SELECT good_key, settled(amount, rate, calc_tick)
			 FROM settlement_goods WHERE settlement_id = $1`, sett.ID,
		)
		if gsrows != nil {
			for gsrows.Next() {
				var k string
				var v float64
				_ = gsrows.Scan(&k, &v)
				if v < 0 {
					v = 0
				}
				goodsStock[k] = v
			}
			gsrows.Close()
		}
		silverStock := sett.Resources.Silver.Current(now)

		buildQuotes, qerr := loadBuildQuotes(r.Context(), h.pool, sett.ID)
		if qerr != nil {
			writeError(w, http.StatusInternalServerError, "could not load build quotes")
			return
		}

		// can_afford per building. For a producing building that already stands, the
		// next build raises its LEVEL, so affordability must be quoted against the
		// next level's cost (base + cedar) — quoting level 1 would tell a Wanax they
		// can afford an upgrade they cannot pay for.
		builtLevels := make(map[string]int, len(buildings))
		for _, b := range buildings {
			if b.Level > builtLevels[b.Type] {
				builtLevels[b.Type] = b.Level
			}
		}
		type buildAffordRow struct {
			Type      string `json:"type"`
			CanAfford bool   `json:"can_afford"`
			NextLevel int    `json:"next_level"`
			AtMax     bool   `json:"at_max_level,omitempty"`
		}
		var buildAfford []buildAffordRow
		for bType, spec := range province.BuildingSpecs {
			next := builtLevels[string(bType)] + 1
			atMax := false
			if province.LevelledBuildings[bType] {
				if next > province.MaxBuildingLevel {
					atMax = true
					next = province.MaxBuildingLevel
				}
				if levelled, lok := province.LevelledSpec(bType, next); lok {
					spec = levelled
				}
			} else if next > 1 {
				atMax = true
				next = 1
			}
			afford := !atMax && province.GoodsAffordable(silverStock, spec.CostSilver)
			if afford {
				afford = goodsAffordableWithCedar(goodsStock, spec.Costs, 1)
			}
			buildAfford = append(buildAfford, buildAffordRow{
				Type: string(bType), CanAfford: afford, NextLevel: next, AtMax: atMax,
			})
		}

		// Index completed buildings for O(1) lookup in the can_recruit loop below.
		builtTypes := make(map[string]bool, len(buildings))
		for _, b := range buildings {
			builtTypes[b.Type] = true
		}

		// Upkeep carrying-capacity (P6, soak 2026-07-18): a galley disbanded the
		// instant it garrisoned ("grain_shortage") and garrisoned spearmen starved
		// to death in a city whose grain rate LOOKED healthy — because army upkeep
		// is a separate once-daily discrete debit (combat/upkeep.go), never folded
		// into settlement_goods' continuous rate. Nothing warned a Wanax before
		// recruit/build that the city was already at, or about to go into, upkeep
		// deficit. Computed once here (before can_recruit) and reused below for the
		// settlement-level net-after-upkeep figures `status` shows.
		var upkeepGrainRate, upkeepSilverRate float64
		_ = h.pool.QueryRow(r.Context(),
			`SELECT
			    COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'), 0),
			    COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'), 0)`,
			sett.ID,
		).Scan(&upkeepGrainRate, &upkeepSilverRate)
		armyUp, circulatedSilver, upErr := settlementUpkeepDrain(r.Context(), h.pool, sett.ID)
		if upErr != nil {
			writeError(w, http.StatusInternalServerError, "could not compute army upkeep")
			return
		}
		// Since Utfodringsordningen D1 (megaron_plan_utfodringsordningen.md,
		// 2026-08-26) upkeepGrainRate is grain's RAW rate — the population's own
		// food is no longer netted into it, FoodTick debits that separately from
		// stock. Net it here via economy.GrainBalance (D6) BEFORE subtracting the
		// army's upkeep, or this chain-gate figure (can_recruit/sustainable below)
		// would silently stop accounting for the population's food at all.
		_, upkeepGrainNetOfPopulation := economy.GrainBalance(upkeepGrainRate, laborPool)
		netGrainPerDay, netSilverPerDay := upkeepNetPerDay(upkeepGrainNetOfPopulation, upkeepSilverRate, armyUp)

		// can_recruit per unit: goods + labor pool + building requirements (for 1 unit).
		// Mirrors the actual Recruit handler gates so can_recruit:false is trustworthy.
		// upkeep_grain_per_tick/upkeep_silver_per_tick + sustainable project what this
		// unit costs once it FINISHES TRAINING AND GARRISONS (forming/training units
		// draw no upkeep yet) against the city's current net-after-upkeep capacity —
		// answering "can this city carry one more of these" before the Wanax commits.
		type recruitAffordRow struct {
			Unit       string `json:"unit"`
			CanRecruit bool   `json:"can_recruit"`
			// Gross is the unit's own upkeep — what the affordability gate needs
			// liquid at debit time (the war-chest is still charged in full).
			UpkeepGrainPerDay  float64 `json:"upkeep_grain_per_tick"`
			UpkeepSilverPerDay float64 `json:"upkeep_silver_per_tick"`
			// Legacy net field equals gross; the owner has already applied home pay.
			UpkeepSilverNetPerDay float64 `json:"upkeep_silver_net_per_tick"`
			Sustainable           bool    `json:"sustainable"`
		}
		var recruitAfford []recruitAffordRow
		for unitType, spec := range province.UnitSpecs {
			afford := laborPool >= spec.PopCost
			if afford && spec.RequiresBarracks && !builtTypes["barracks"] {
				afford = false
			}
			if afford && spec.RequiresStable && !builtTypes["stable"] {
				afford = false
			}
			if afford && spec.RequiresHarbour && !builtTypes["harbour"] {
				afford = false
			}
			if afford && spec.RequiresShipyard && !builtTypes["shipyard"] {
				afford = false
			}
			if afford && spec.RequiresFoundry && !builtTypes["foundry"] {
				afford = false
			}
			if afford {
				// Recruit charges per man × the whole batch (a 100-man cohort or the crew).
				batch := 100.0
				if crew := unit.CrewFor(unit.Type(unitType)); crew > 0 {
					batch = float64(crew)
				}
				afford = goodsAffordableWithCedar(goodsStock, spec.Costs, batch)
			}
			fullSize := 100
			cat := string(unit.CategoryOf(unit.Type(unitType)))
			if cat != "land" {
				fullSize = 1 // naval food/pay uses actual crew, not hull size
			}
			newUnitUp := combat.UnitUpkeep(unitType, cat, fullSize, "garrison", unit.CrewFor(unit.Type(unitType)))
			// Home pay is already halved by the wage owner; gross equals net.

			newUnitNetSilver := newUnitUp.Silver
			sustainable := (netGrainPerDay-newUnitUp.Grain) >= 0 && (netSilverPerDay-newUnitNetSilver) >= 0
			recruitAfford = append(recruitAfford, recruitAffordRow{
				Unit: unitType, CanRecruit: afford,
				UpkeepGrainPerDay: newUnitUp.Grain, UpkeepSilverPerDay: newUnitUp.Silver,
				UpkeepSilverNetPerDay: newUnitNetSilver,
				Sustainable:           sustainable,
			})
		}

		// Live kharis pool (per-Wanax, on player_world_records) — NOT the stale
		// settlement-level resources.kharis (≈0 since kharis moved to the pool).
		// The oracle/rite tier-gate (MinKharis) reads this, so the agent must see
		// it to know which prayers it can actually cast.
		var kharisNow, kharisRate, kharisCap float64
		var maxTempleLevel int
		var kharisNetPerDay, kharisProduction, kharisBaseDecay, kharisCityDecay, kharisPopulationDecay float64
		var kharisCityCount, kharisPopulation int
		var kharisNetKnown, kharisDevotionIdle bool
		var kharisWrath *kharis.WrathStatus
		viewerID, authenticated := auth.PlayerIDFromContext(r.Context())
		ownCult := authenticated && sett.OwnerID != nil && *sett.OwnerID == viewerID
		if sett.OwnerID != nil {
			if k, kerr := loadPlayerKharis(r.Context(), h.pool, *sett.OwnerID, worldID); kerr == nil {
				kharisNow, kharisRate = k.Amount, k.Rate
				kharisCap, maxTempleLevel = k.Cap, k.MaxTempleLevel
			}
		}
		if ownCult {
			wrath, err := kharis.LoadWrathStatus(r.Context(), h.pool, *sett.OwnerID, worldID)
			if err != nil {
				writeError(w, 500, "could not load divine protection")
				return
			}
			kharisWrath = &wrath
			decay, decayErr := kharis.ProjectDecay(r.Context(), h.pool, *sett.OwnerID, worldID)
			kharisCityCount, kharisPopulation = decay.CityCount, decay.Population
			kharisBaseDecay, kharisCityDecay, kharisPopulationDecay = decay.Base, decay.Cities, decay.Residents
			if decayErr != nil {
				writeError(w, http.StatusInternalServerError, "could not load kharis decay")
				return
			}
			// Projected daily maintenance net (gain − decay) — the honest answer to
			// "will my kharis rise or fall", which the passive geographic rate alone
			// hides (soak 2026-07-24: a fading L1 Wanax saw "+0.1/dygn"). Read-only.
			if net, has, devSum, devCap, nerr := kharis.ProjectDailyNet(r.Context(), h.pool, *sett.OwnerID, worldID); nerr == nil {
				kharisNetPerDay, kharisNetKnown = net, true
				kharisProduction = net + decay.Total
				kharisDevotionIdle = has && devCap > devSum
			}
		}

		// Status displays the same local early-gift plan as the kharis tick.
		templeOffers := []kharis.TempleOffer{}
		if viewerID, authenticated := auth.PlayerIDFromContext(r.Context()); authenticated && sett.OwnerID != nil && *sett.OwnerID == viewerID {
			offers, err := kharis.LoadTempleOffers(r.Context(), h.pool, *sett.OwnerID, worldID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not load temple offerings")
				return
			}
			templeOffers = offers
		}

		// Temple presence — a prayer needs its own god's temple (ORDER 15); a
		// legacy prayer without a god takes any temple, as the rite handler does.
		var templeGods []string
		_ = h.pool.QueryRow(r.Context(),
			`SELECT coalesce(array_agg(god_id), '{}') FROM buildings WHERE settlement_id = $1 AND building_type = 'temple'`,
			sett.ID,
		).Scan(&templeGods)
		hasTemple := len(templeGods) > 0
		hasTempleFor := func(god string) bool { return (god == "" && hasTemple) || slices.Contains(templeGods, god) }

		// available_prayers: the settlement culture's prayers with full affordability:
		// material offering costs + kharis tier gate + temple presence.
		// All three gates mirror the real Rite handler so affordable:true is trustworthy.
		// cooldown_remaining_game_days is >0 when the prayer is on cooldown (game-days,
		// not wall clock — one tick IS one day).
		// Effect (Plan A / A7, megaron_kult_legibilitet_plan.md) surfaces spec.Description
		// so a Wanax knows what a prayer does before casting it. DESIGN INVARIANT
		// (Timothy 2026-07-11, HARD): never add a success_chance/odds field here — the
		// gods are not machines. Gynnsamhet is read via the settlement's kharis_mood
		// sibling field (kharisToMood, web.go), not a computed percentage.
		type prayerRow struct {
			CanPrepareOffering  bool               `json:"can_prepare_offering"`
			GodID               string             `json:"god_id,omitempty"`
			ID                  string             `json:"id"`
			Name                string             `json:"name"`
			God                 string             `json:"god"`
			Temple              string             `json:"temple,omitempty"` // the temple it is prayed at
			EffectType          string             `json:"effect_type"`
			Effect              string             `json:"effect"`
			MinKharis           float64            `json:"min_kharis"`
			Offering            map[string]float64 `json:"offering"`
			UnavailableReason   string             `json:"unavailable_reason,omitempty"`
			Affordable          bool               `json:"affordable"`
			Default             bool               `json:"is_default"`
			DeparturePending    bool               `json:"departure_pending,omitempty"`
			ActiveRemainingDays int                `json:"active_remaining_game_days,omitempty"`
			// Game-days, not wall-clock minutes: CooldownTicks is a TICK count
			// and one tick IS one day (megaron_plan_ticket_ar_dygnet). The old
			// cooldown_remaining_minutes reported 1440 for a 24-tick cooldown —
			// true, unreadable, and in the wrong unit. Rounds UP, so a cooldown
			// with any time left never reports 0.
			CooldownRemainingDays int `json:"cooldown_remaining_game_days,omitempty"`
			// The temple's reading of the gods (temenos_prayers_komposition_plan.md).
			// An offering is now composed, and its worth depends on world scarcity —
			// data no Wanax can observe through the fog. The temple is exactly the
			// institution that would know, so it tells you: what this god favours,
			// and what the traditional recipe is currently reckoned to be worth.
			// Without this the composition mechanic is unplayable noise.
			Favours          map[string]float64 `json:"favours,omitempty"`
			OfferingBaseline float64            `json:"offering_baseline,omitempty"`
		}
		// The gods' current reckoning — one read for the whole prayer list.
		divineValues, _ := religion.LoadDivineValues(r.Context(), h.pool, worldID)

		// Devotion + what this settlement's temple can employ.
		templeStaff, err := economy.LoadTempleStaffing(r.Context(), h.pool, sett.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load temple staffing")
			return
		}

		var prayerTick int
		if err := h.pool.QueryRow(r.Context(), `SELECT current_tick FROM worlds WHERE id=$1`, worldID).Scan(&prayerTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read prayer time")
			return
		}
		prayers := []prayerRow{}
		for _, pid := range religion.CulturePrayers[string(sett.CultureID)] {
			spec := religion.PrayerSpecs[pid]
			afford := hasTempleFor(spec.GodID) && kharisNow >= spec.MinKharis
			// Opening the offering form does not require the traditional recipe.
			canPrepareOffering := afford
			if afford {
				for g, need := range spec.Offering {
					if !province.GoodsAffordable(goodsStock[g], need) {
						afford = false
						break
					}
				}
			}
			// Check cooldown: same query as the Rite handler.
			// Use sett.OwnerID (not request auth) so spectators see the owner's cooldown.
			var cooldownRemainingDays int
			if spec.GodID == "" && spec.CooldownTicks > 0 && sett.OwnerID != nil {
				var lastCast time.Time
				if cdErr := h.pool.QueryRow(r.Context(),
					`SELECT created_at FROM events
					 WHERE world_id = $1
					   AND event_type = 'RiteCast'
					   AND payload->>'player_id' = $2
					   AND payload->>'prayer' = $3
					   AND (payload->>'success')::boolean = true
					   AND stream_id = $4
					 ORDER BY created_at DESC LIMIT 1`,
					worldID, sett.OwnerID.String(), pid, sett.ID,
				).Scan(&lastCast); cdErr == nil {
					elapsed := h.clk.Now().Sub(lastCast)
					remaining := tick.RealUntil(spec.CooldownTicks, 0) - elapsed
					if remaining > 0 {
						cooldownRemainingDays = tick.GameDaysLeft(remaining)
						afford = false
						canPrepareOffering = false
					}
				}
			}
			activeRemainingDays := 0
			if spec.GodID != "" && sett.OwnerID != nil {
				var cdErr error
				cooldownRemainingDays, cdErr = religion.GodCooldownRemaining(r.Context(), h.pool, worldID, *sett.OwnerID, spec.GodID, prayerTick)
				if cdErr != nil {
					writeError(w, http.StatusInternalServerError, "could not read divine waiting time")
					return
				}
				activeRemainingDays, cdErr = religion.PrayerActiveRemaining(r.Context(), h.pool, sett.ID, *sett.OwnerID, spec.EffectType, prayerTick)
				if cdErr != nil {
					writeError(w, http.StatusInternalServerError, "could not read active prayer")
					return
				}
				if cooldownRemainingDays > 0 || activeRemainingDays > 0 {
					afford = false
					canPrepareOffering = false
				}
			}
			departurePending := activeRemainingDays > 0 && (spec.EffectType == religion.EffectNextLandDeparture || spec.EffectType == religion.EffectNextShipDeparture)
			if departurePending {
				activeRemainingDays = 0
			}
			reason := ""
			temple := ""
			if spec.GodID != "" {
				temple = religion.TempleName(spec.GodID)
			}
			if !hasTempleFor(spec.GodID) {
				reason = "no temple"
				if temple != "" {
					reason = "no " + temple
				}
			} else if kharisNow < spec.MinKharis {
				reason = "insufficient kharis"
			} else if !afford && cooldownRemainingDays == 0 && activeRemainingDays == 0 && !departurePending {
				// Timed/pending states already have precise structured fields.
				// A generic reason would hide them in the prayer list.
				reason = "insufficient offering"
			}
			prayers = append(prayers, prayerRow{DeparturePending: departurePending, CanPrepareOffering: canPrepareOffering,
				GodID: spec.GodID, ID: spec.ID, Name: spec.Name, God: spec.God, Temple: temple, EffectType: spec.EffectType,
				Default: spec.ID == religion.DefaultBattleFrenzyFor(string(sett.CultureID)), ActiveRemainingDays: activeRemainingDays,
				Effect:    spec.Description,
				MinKharis: spec.MinKharis, Offering: spec.Offering, Affordable: afford, UnavailableReason: reason,
				CooldownRemainingDays: cooldownRemainingDays,
				Favours:               religion.FavoursFor(spec),
				OfferingBaseline:      religion.TraditionalBaseline(spec, divineValues),
			})
		}

		// Silver is authoritative in settlement_goods (mig 057 silver_unify); the
		// ResourceLedger.Silver column is stale (~0). Inject the live value for every
		// good the settlement holds — not a hard-coded subset — so the status view
		// shows the same stock as `goods` (previously it reported only silver+metals,
		// hiding timber/stone even when labor was actively allocated to them).
		resSnap := sett.Resources.SnapshotFull(now)
		if grows, gerr := h.pool.Query(r.Context(),
			`SELECT good_key, GREATEST(0, settled(amount, rate, calc_tick)), rate, cap
			 FROM settlement_goods
			 WHERE settlement_id = $1`,
			sett.ID,
		); gerr == nil {
			for grows.Next() {
				var k string
				var amt, rt, cp float64
				if grows.Scan(&k, &amt, &rt, &cp) == nil {
					// settled() extrapolates linearly with no ceiling — clamp to cap
					// here too (Goods already does this), otherwise a good that hasn't
					// been settled in a while shows an ever-growing uncapped number in
					// status while goods correctly reports it flat at cap.
					if amt > cp {
						amt = cp
					}
					resSnap[k] = province.ResourceDetail{Amount: amt, Rate: rt, Cap: cp}
				}
			}
			grows.Close()
		}

		// Current food stock and growth use the same shared summary as overview.
		var currentTick int
		var harvestBlessingUntil *int
		if err := h.pool.QueryRow(r.Context(),
			`SELECT current_world_tick(), harvest_blessing_until_tick FROM settlements WHERE id = $1`,
			sett.ID,
		).Scan(&currentTick, &harvestBlessingUntil); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read settlement status")
			return
		}
		// Extracted to settlementFoodSummary (megaron_plan_oversiktsendpoint.md,
		// 2026-09-06) so SettlementsOverview reads the exact same economy.* calls
		// — the two surfaces can never disagree about a settlement's food state.
		foodSummary := settlementFoodSummary(r.Context(), h.pool, sett.ID, sett.Population)

		nextCitizen, err := economy.LoadNextCitizen(r.Context(), h.pool, sett.ID)
		if err != nil {
			writeError(w, 500, "could not read next citizen growth")
			return
		}

		coverageDays := foodSummary.CoverageTicks

		foodNetPerDay := foodSummary.FoodNetPerTick
		grainConsumRate := foodSummary.GrainConsumRate
		grainProdRate := foodSummary.GrainProdRate
		foodGubbarRequired := foodSummary.FoodGubbarRequired
		foodSelfSufficient := foodSummary.FoodSelfSufficient
		// food_gubbar_placed counts ONLY grain/fish rows — the exact good set
		// rankSlotsFromOptions greedily places on (economy/founding_placement.go).
		// Never settlement_labor (the dead table) and never economy.FoodGoods
		// (a WIDER set used for diet-variety scoring elsewhere — livestock/
		// wine/oil included — that would compare two different quantities).
		var foodGubbarPlaced int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM settlement_placement WHERE settlement_id = $1 AND good_key IN ($2, $3)`,
			sett.ID, economy.GoodGrain, economy.GoodFish,
		).Scan(&foodGubbarPlaced)

		// Current production projection plus the real latest meal. Do not report
		// the entire population ration as grain after the shared proportional debit.
		lastTickProd := map[string]float64{}
		lastTickCons := map[string]float64{}
		for good, rd := range resSnap {
			if rd.Rate > 0 {
				lastTickProd[good] = rd.Rate
			} else if rd.Rate < 0 {
				lastTickCons[good] = -rd.Rate
			}
		}
		if foodSummary.ConsumedTick != nil && *foodSummary.ConsumedTick == currentTick {
			for good, n := range foodSummary.Consumed {
				lastTickCons[good] += n
			}
		}
		// Settlement cap: same "how many colonies do I hold vs. the per-Wanax
		// ceiling" figure `keryx actions` derives for the colonize gate
		// (capabilities.settlementCapRequirement / province.MaxSettlementsPerWanax),
		// surfaced here too so status doesn't require a second round-trip to see
		// it. Scoped by sett.OwnerID (not the requesting player) to match the
		// existing kharis/cooldown convention above — spectators see the owner's count.
		var settlementsOwned int
		if sett.OwnerID != nil {
			_ = h.pool.QueryRow(r.Context(),
				`SELECT count(*) FROM settlements WHERE world_id = $1 AND owner_id = $2 AND state = 'active'`,
				worldID, *sett.OwnerID,
			).Scan(&settlementsOwned)
		}

		// Belägring S1+S2 (megaron_plan_belagring.md): who's holding the
		// chokepoint, named for the client — ONLY queried when the settlement
		// is already besieged (the flag itself isn't FOW-gated; naming its
		// cause follows the same "you feel it, you're told what's doing it"
		// principle, Timothy 2026-08-08).
		var besiegedBy []map[string]any
		if sett.Besieged && sett.OwnerID != nil {
			besiegers, err := economy.LoadBesiegers(r.Context(), h.pool, worldID, *sett.OwnerID,
				hexgrid.Coord{Q: prov.MapTile.Q, R: prov.MapTile.R})
			if err == nil {
				for _, b := range besiegers {
					besiegedBy = append(besiegedBy, map[string]any{
						"owner_name": b.OwnerName,
						"unit_type":  b.UnitType,
						"size":       b.Size,
						"q":          b.Q,
						"r":          b.R,
					})
				}
			}
		}
		if besiegedBy == nil {
			besiegedBy = []map[string]any{}
		}

		var storage []economy.StorageForecast
		if viewer, ok := auth.PlayerIDFromContext(r.Context()); ok && sett.OwnerID != nil && viewer == *sett.OwnerID {
			tx, err := h.pool.Begin(r.Context())
			if err != nil {
				writeError(w, 500, "could not read storage forecast")
				return
			}
			storage, err = (combat.StorageForecaster{}).StorageForecast(r.Context(), tx, sett.ID)
			tx.Rollback(r.Context())
			if err != nil {
				writeError(w, 500, "could not read storage forecast")
				return
			}
		}
		resp["settlement"] = map[string]any{
			"id":                           sett.ID,
			"name":                         sett.Name,
			"owner_id":                     sett.OwnerID,
			"kingdom_id":                   sett.KingdomID,
			"culture":                      sett.CultureID,
			"state":                        sett.State,
			"besieged":                     sett.Besieged,
			"besieged_by":                  besiegedBy,
			"population":                   sett.Population,
			"next_citizen_progress":        nextCitizen.Progress,
			"next_citizen_growth_per_tick": nextCitizen.GrowthPerTick,
			"next_citizen_in_ticks":        nextCitizen.InTicks,
			"labor_pool":                   laborPool,
			"walls":                        sett.WallLevel,
			"population_cap":               economy.PopulationCap(sett.WallLevel),
			"silo_level":                   economy.SiloLevel(r.Context(), h.pool, sett.ID),
			"resources":                    resSnap,
			"kharis":                       kharisNow,
			"kharis_wrath":                 kharisWrath,
			"kharis_blessing_threshold":    kharis.BlessThreshold,
			"kharis_rate":                  kharisRate,
			"kharis_mood":                  kharisToMood(kharisNow),
			"kharis_per_tick":              kharisRate,
			"kharis_cap":                   kharisCap,
			"kharis_city_count":            kharisCityCount,
			"kharis_base_decay":            kharisBaseDecay,
			"kharis_city_decay":            kharisCityDecay,
			"kharis_daily_decay":           kharisBaseDecay + kharisCityDecay + kharisPopulationDecay,
			"kharis_population":            kharisPopulation,
			"kharis_population_decay":      kharisPopulationDecay,
			"kharis_production_per_tick":   kharisProduction,
			"kharis_per_temple_worker":     kharis.KharisPerTempleWorker,
			"max_temple_level":             maxTempleLevel,
			"kharis_net_per_tick":          kharisNetPerDay,
			"kharis_net_known":             kharisNetKnown,
			"kharis_devotion_idle":         kharisDevotionIdle,
			"temple_offers":                templeOffers,
			"grain_prod_rate":              grainProdRate,
			"grain_consum_rate":            grainConsumRate,
			"food_gubbar_required":         foodGubbarRequired,
			"automatic_food":               foodSummary.AutomaticFood,
			"food_gubbar_placed":           foodGubbarPlaced,
			"food_self_sufficient":         foodSelfSufficient,
			"harvest_blessing":             harvestBlessingStatus(harvestBlessingUntil, currentTick),
			"storm_no_fishing":             len(economy.StormNoFishingHexes(r.Context(), h.pool, sett.ID)),
			"army":                         sett.Army,
			"army_upkeep":                  armyUp,
			// Del C: the sold a garrison spends back into the town it holds. Without
			// this line the net below cannot be derived from the gross above, and the
			// mechanic would be invisible — a silver flow the Wanax cannot see or plan
			// against. Grain has no equivalent: soldiers eat their rations.
			"army_upkeep_circulated_silver":    circulatedSilver,
			"net_grain_per_tick_after_upkeep":  netGrainPerDay,
			"food_net_per_tick_after_upkeep":   foodNetAfterArmy(foodNetPerDay, armyUp.Grain),
			"net_silver_per_tick_after_upkeep": netSilverPerDay,
			"build_queue":                      buildQueue,
			"build_queue_max":                  maxParallelBuilds,
			"training_units":                   trainingUnits,
			"buildings":                        buildings,
			"build_quotes":                     buildQuotes,
			"can_afford":                       buildAfford,
			"can_recruit":                      recruitAfford,
			"available_prayers":                prayers,
			"divine_values":                    divineValues,
			// Physical temple workers share the settlement workforce.
			"temple_staffed":  templeStaff.Staffed,
			"temple_capacity": templeStaff.Capacity,
			// One row per god's temple; the two sums above are these added up.
			"temples": templeRows(templeStaff.Temples),
			"settlement_cap": map[string]any{
				"used": settlementsOwned,
				"max":  province.MaxSettlementsPerWanax,
			},
			"storage": storage,
			"food": map[string]any{

				"coverage_ticks":    coverageDays,
				"food_net_per_tick": foodNetPerDay,
				"growth_state":      foodSummary.GrowthState,
				"herd":              foodSummary.Herd,
				"goods":             foodSummary.Goods, "consumed": foodSummary.Consumed, "consumed_tick": foodSummary.ConsumedTick,
				"production_per_tick": foodSummary.ProductionPerTick, "consumption_per_tick": foodSummary.ConsumptionPerTick,
			},
			"last_tick": map[string]any{
				"tick":        currentTick,
				"production":  lastTickProd,
				"consumption": lastTickCons,
			},
		}
		// The population term and total/net can reveal residents in cities the
		// visitor has never seen. Cult upkeep is private, like local altar stocks.
		if !ownCult {
			private := resp["settlement"].(map[string]any)
			for _, key := range []string{"storm_no_fishing", "kharis_wrath", "kharis_city_count", "kharis_base_decay", "kharis_city_decay", "kharis_daily_decay", "kharis_population", "kharis_population_decay", "kharis_production_per_tick", "kharis_net_per_tick", "kharis_devotion_idle"} {
				delete(private, key)
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// FoodSummary is a settlement's single-inventory food snapshot as the economy.*
// functions compute it — shared by ProvinceHandler.Get and
// SettlementHandler.SettlementsOverview (megaron_plan_oversiktsendpoint.md,
// 2026-09-06) so the two surfaces read the same numbers from the same
// functions and can never disagree.
type FoodSummary struct {
	Goods              map[string]float64
	ConsumedTick       *int
	Consumed           map[string]float64
	Herd               map[string]int
	ProductionPerTick  float64
	ConsumptionPerTick float64
	AutomaticFood      economy.AutomaticFood
	GrainProdRate      float64
	GrainConsumRate    float64
	FoodSelfSufficient bool
	FoodGubbarRequired int

	CoverageTicks  float64
	FoodNetPerTick float64
	GrowthState    string
}

// settlementFoodSummary computes a settlement's food economy exactly as
// ProvinceHandler.Get always has (formerly inline in Get, extracted
// 2026-09-06 — same economy.* calls, same SQL, never a second formula).
// population is the caller's already-loaded sett.Population, passed
// explicitly so a caller that only loaded id/name/population (the overview
// handler) doesn't need a full settlement.Settlement.
func settlementFoodSummary(ctx context.Context, pool *pgxpool.Pool, settID uuid.UUID, population int) FoodSummary {
	// Part B: labor_pool = population. Soldiers are extracted from population at
	// recruit time, so army columns are no longer a labor drain.
	laborPool := population
	if laborPool < 0 {
		laborPool = 0
	}
	var wallLevel int
	_ = pool.QueryRow(ctx, `SELECT wall_level FROM settlements WHERE id = $1`, settID).Scan(&wallLevel)

	var grainAmount, grainRate, grainCap float64
	var grainCalcTick int
	_ = pool.QueryRow(ctx,
		`SELECT sg.amount, sg.rate, sg.cap, sg.calc_tick
		 FROM settlement_goods sg JOIN goods g ON g.key = sg.good_key
		 WHERE sg.settlement_id = $1 AND sg.good_key = 'grain'`,
		settID,
	).Scan(&grainAmount, &grainRate, &grainCap, &grainCalcTick)
	// Coverage is measured on the whole food basket (B6) — grain first, fish
	// for the remainder, one need (economy.FoodConsumptionSplit). Counting
	// grain alone would call a fish-fed city starving.
	var foodStock float64
	foodGoods := map[string]float64{}
	for _, good := range economy.FoodGoods {
		var s float64
		if pool.QueryRow(ctx,
			`SELECT GREATEST(0, settled(amount, rate, calc_tick))
			 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
			settID, good,
		).Scan(&s) == nil {
			foodStock += s
			foodGoods[good] = s
		}
	}
	var foodRatePerTick float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(rate), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = ANY($2)`,
		settID, economy.FoodGoods,
	).Scan(&foodRatePerTick)
	coverageDays := economy.FoodCoverageTicks(foodStock, population)

	// Coverage is a stock figure, so it says nothing about which way the city
	// is going — and a newly founded city legitimately starts near zero
	// coverage while producing a large surplus. Reported alone it reads as
	// famine for the first days of every city's life (eye-check 2026-08-03:
	// a surplus city showed only its small initial stock). The
	// net food rate is what separates "lean and climbing" from "starving",
	// and the surfaces need both to say either honestly.
	//
	// economy.FoodNet — production MINUS the population's ration, the same
	// balance growth is gated on (kharis/tick.go). Until 2026-09-26 this was
	// foodRatePerTick alone: since Utfodringsordningen D1 moved eating out of
	// the rate, that was raw production, so every city read "+" here whether
	// or not it fed itself.
	foodNetPerDay := economy.FoodNet(foodRatePerTick, population)
	// The growth gate's other input: did the city eat today (FoodTick).
	var foodUnmet float64
	consumed := map[string]float64{}
	var consumedTick *int
	_ = pool.QueryRow(ctx, `SELECT food_unmet_amount,food_consumed,food_consumed_tick FROM settlements WHERE id = $1`, settID).Scan(&foodUnmet, &consumed, &consumedTick)

	// Grain-netto-märkning (DEL C, megaron_ekonomi_legibilitet_plan.md).
	//
	// Since Utfodringsordningen D1 (megaron_plan_utfodringsordningen.md,
	// 2026-08-26) the stored grain rate is RAW production — the population's
	// food is debited once a day from STOCK by FoodTick, not folded into this
	// rate — so grainProdRate is grain's rate as-is, and grainConsumRate comes
	// from economy.GrainBalance (D6, the one shared reader every surface uses
	// instead of re-deriving laborPool × GrainConsumptionPerCitizenPerTick
	// itself — province.go used to do exactly that twice).
	grainConsumRate, _ := economy.GrainBalance(grainRate, laborPool)
	grainProdRate := grainRate

	// Gubbar krävda för föda (P4-arvet i province.go, replaces the old
	// pre-P4 weight-based figure — megaron_plan_p4_arvet_i_province.md
	// §2): how many gubbar the catchment's food (grain/fish) slots need for
	// the settlement's OWN production to cover the population's daily
	// ration, run through the SAME greedy loop founding/growth placement
	// use (economy.FoodGubbarRequired) — never a second formula.
	// foodSelfSufficient=false is not silent (arbetssätt §7): a catchment
	// that cannot feed its population even with every gubbe placed on food
	// (Gournia/Zakros in drift) must say so, not just drop the field.
	foodGubbarRequired, foodSelfSufficient, automatic, foodReqErr := economy.FoodGubbarRequiredWithAutomatic(ctx, pool, settID)
	if foodReqErr != nil {
		slog.Error("food gubbar required failed", "err", foodReqErr, "settlement", settID)
	}

	var herdNow float64
	_ = pool.QueryRow(ctx, `SELECT GREATEST(0, settled(amount, rate, calc_tick)) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'livestock'`, settID).Scan(&herdNow)
	herd := map[string]int{"count": int(herdNow), "cap": int(economy.HerdCap(economy.PenLevel(ctx, pool, settID)))}
	// The next calves (order 33): how many, how many grain they cost, on which tick.
	var grainNow float64
	var nowTick int
	_ = pool.QueryRow(ctx, `SELECT COALESCE((SELECT GREATEST(0, settled(amount, rate, calc_tick)) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'), 0), COALESCE(current_world_tick(), 0)`, settID).Scan(&grainNow, &nowTick)
	born, fed, at := economy.NextHerdBirth(herdNow, grainNow, nowTick)
	herd["births_next"], herd["births_grain"], herd["next_birth_tick"] = born, fed, at
	if at > 0 {
		herd["births_in"] = at - nowTick // days, for the player surfaces
	}

	return FoodSummary{
		Herd:  herd,
		Goods: foodGoods, Consumed: consumed, ConsumedTick: consumedTick, ProductionPerTick: foodRatePerTick, ConsumptionPerTick: economy.GrainConsumptionPerTick(population),
		AutomaticFood:      automatic,
		GrainProdRate:      grainProdRate,
		GrainConsumRate:    grainConsumRate,
		FoodSelfSufficient: foodSelfSufficient,
		FoodGubbarRequired: foodGubbarRequired,

		CoverageTicks:  coverageDays,
		FoodNetPerTick: foodNetPerDay,
		GrowthState:    economy.GrowthStateAtWall(foodUnmet, foodNetPerDay, population, wallLevel),
	}
}

// upkeepAmount is a grain+silver upkeep total per upkeep-period (the daily tick).
type upkeepAmount struct {
	Grain  float64 `json:"grain"`
	Silver float64 `json:"silver"`
}

// Grain remains the legacy wire alias for nominal ration demand. New clients
// use food; actual commodity debits live in UpkeepSettled.food_by_good.
func (u upkeepAmount) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Grain  float64 `json:"grain"`
		Food   float64 `json:"food"`
		Silver float64 `json:"silver"`
	}{u.Grain, u.Grain, u.Silver})
}

// armyUpkeep sums the per-period upkeep of the garrison STANDING IN a settlement,
// from the units table (the SB7 source of truth) via combat.UnitUpkeep. This is
// the composition figure /army reports next to the units it lists — "what the
// force here costs to keep".
//
// It is NOT what this settlement pays. Since mig 100 the payer is
// support_settlement_id, not settlement_id. Use settlementUpkeepDrain for anything that projects
// the city's own silver, or the projection will drift from the debit.
//
// The status filter is deliberately narrower than the tick's own billing set
// (checked 2026-08-05, embarkerad ranson): this is a composition figure for
// what stands garrisoned HERE, and an embarked unit is aboard a ship, not
// standing in the settlement — it is correctly excluded, not drifted.
func armyUpkeep(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) (upkeepAmount, map[string]upkeepAmount, error) {
	total := upkeepAmount{}
	perType := map[string]upkeepAmount{}
	rows, err := pool.Query(ctx,
		`SELECT type, category, size, status, crew FROM units
		 WHERE settlement_id = $1 AND status IN ('garrison','fitting_out')`,
		settlementID,
	)
	if err != nil {
		return total, perType, err
	}
	defer rows.Close()
	for rows.Next() {
		var unitType, category, status string
		var size, crew int
		if err := rows.Scan(&unitType, &category, &size, &status, &crew); err != nil {
			return total, perType, err
		}
		up := combat.UnitUpkeep(unitType, category, size, status, crew)
		if up.Grain == 0 && up.Silver == 0 {
			continue
		}
		total.Grain += up.Grain
		total.Silver += up.Silver
		agg := perType[unitType]
		agg.Grain += up.Grain
		agg.Silver += up.Silver
		perType[unitType] = agg
	}
	return total, perType, rows.Err()
}

// settlementUpkeepDrain is what the daily upkeep tick ACTUALLY debits this
// settlement. Two things separate it from armyUpkeep, and both arrived after
// that function was written:
//
//   - Payer, not location. Since mig 100 support_settlement_id is the sole payer
//     (the silent capital fallback is gone), so a unit marching across the map
//     still bills the town that raised it, while a unit garrisoning here may be
//     paid by someone else entirely. Grouping by settlement_id understates the
//     drain — the dangerous direction: it calls a city sustainable that isn't.
//
// Wages already contain the home/field factor; no pay is credited back.
// The legacy circulation return remains zero for compatible API fields.
func settlementUpkeepDrain(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) (gross upkeepAmount, circulatedSilver float64, err error) {
	rows, qerr := pool.Query(ctx,
		`SELECT u.type, u.category, u.size, u.status, u.crew
		 FROM units u
		 JOIN settlements s ON s.id = u.support_settlement_id AND s.owner_id = u.owner_id
		 WHERE u.support_settlement_id = $1
		   AND u.status IN ('garrison', 'marching', 'positioned', 'embarked', 'repairing', 'freighting', 'fitting_out')`,
		settlementID,
	)
	if qerr != nil {
		return gross, 0, qerr
	}
	defer rows.Close()
	for rows.Next() {
		var unitType, category, status string
		var size, crew int
		if serr := rows.Scan(&unitType, &category, &size, &status, &crew); serr != nil {
			return gross, 0, serr
		}
		up := combat.UnitUpkeep(unitType, category, size, status, crew)
		gross.Grain += up.Grain
		gross.Silver += up.Silver
	}
	return gross, circulatedSilver, rows.Err()
}

// upkeepNetPerDay converts a settlement's raw grain/silver production rates
// (settlement_goods.rate, per tick — production net of CITIZEN consumption
// only) into the carrying-capacity figure a Wanax needs before adding an
// upkeep-bearing unit: netted against the upkeep the city ALREADY pays.
// Army upkeep is a separate once-daily discrete debit (combat/upkeep.go),
// never folded into the continuous rate, so a settlement's displayed grain
// rate can look healthy right up until the daily upkeep tick disbands a unit
// (P6, soak 2026-07-18). Home/field wages are already resolved by UnitUpkeep.
// Pure function — no DB — so it's unit-testable without a database.
func upkeepNetPerDay(grainRatePerTick, silverRatePerTick float64, up upkeepAmount) (grainNetPerDay, silverNetPerDay float64) {
	grainNetPerDay = grainRatePerTick - up.Grain
	silverNetPerDay = silverRatePerTick - up.Silver
	return grainNetPerDay, silverNetPerDay
}

// GetArmy handles GET /worlds/:worldID/provinces/:provinceID/army.
func (h *ProvinceHandler) GetArmy(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}

	sett, err := loadSettlementByProvince(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no settlement in province")
		return
	}
	up, perType, err := armyUpkeep(r.Context(), h.pool, sett.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not compute army upkeep")
		return
	}
	// Embed ArmyComposition so its fields stay at the top level (non-breaking for
	// existing /army consumers); add the upkeep totals + per-type breakdown.
	writeJSON(w, http.StatusOK, struct {
		province.ArmyComposition
		UpkeepPerPeriod upkeepAmount            `json:"upkeep_per_period"`
		UpkeepPerType   map[string]upkeepAmount `json:"upkeep_per_type,omitempty"`
	}{
		ArmyComposition: sett.Army,
		UpkeepPerPeriod: up,
		UpkeepPerType:   perType,
	})
}

// Build handles POST /worlds/:worldID/provinces/:provinceID/build.
func (h *ProvinceHandler) Build(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		BuildingType string `json:"building_type"`
		HexQ         *int   `json:"hex_q"`
		HexR         *int   `json:"hex_r"`
		God          string `json:"god"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	spec, ok := province.BuildingSpecs[province.BuildingType(req.BuildingType)]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown building type")
		return
	}
	// A temple is built for one god; building it again raises that god's
	// temple (ORDER 15). godArg stays nil for every other building, which the
	// queries below match with IS NOT DISTINCT FROM.
	var godArg *string
	if req.BuildingType == string(province.BuildingTemple) {
		if _, ok := religion.TempleGodByID(req.God); !ok {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("a temple is built for one god — choose god: %s", religion.TempleGodIDs()))
			return
		}
		godArg = &req.God
	} else if req.God != "" {
		writeError(w, http.StatusBadRequest, "only a temple is built for a god")
		return
	}

	// Hex-bound types (farm/mine/lumbermill/stonequarry, province.HexBoundBuildings)
	// are placed on ONE catchment hex and only affect that hex — hex_q/hex_r
	// are required for them, and forbidden for every city building
	// (megaron_plan_byggnad_pa_hex.md §A, Timothy 2026-09-28).
	isHexBound := province.HexBoundBuildings[province.BuildingType(req.BuildingType)]
	if isHexBound {
		if req.HexQ == nil || req.HexR == nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is placed on a hex — hex_q and hex_r are required", req.BuildingType))
			return
		}
	} else if req.HexQ != nil || req.HexR != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is a city building and does not take a hex", req.BuildingType))
		return
	}

	// Verify ownership via settlement.
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	settlementID, err := resolveSettlementID(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no settlement in province")
		return
	}

	// Harbour and shipyard both require the settlement to be adjacent to a sea
	// hex (coast is a property, not a terrain) — shipyard split off from
	// harbour (megaron_plan_skeppsreparation.md Slice A, §Beslut B1) and
	// carries the same coastal gate BuildingPurposes promises for it.
	if province.NeedsWaterNeighbour(province.BuildingType(req.BuildingType)) {
		var pq, pr int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT p.map_q, p.map_r FROM provinces p WHERE p.id = $1`, provinceID,
		).Scan(&pq, &pr)
		var coastNeighbour bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(
			   SELECT 1 FROM map_tiles
			   WHERE world_id = $1
			     AND (q, r) IN (
			       ($2+1,$3), ($2-1,$3),
			       ($2,$3+1), ($2,$3-1),
			       ($2+1,$3-1), ($2-1,$3+1)
			     )
			     AND terrain IN ('coastal_sea','deep_sea','river','river_ford')
			 )`,
			worldID, pq, pr,
		).Scan(&coastNeighbour)
		if !coastNeighbour {
			writeError(w, http.StatusUnprocessableEntity,
				province.InlandBuildRefusal(province.BuildingType(req.BuildingType)))
			return
		}
	}

	// Hex-bound types are placed on ONE specific catchment hex — validate
	// THAT hex, not the catchment in aggregate (megaron_plan_byggnad_pa_hex.md
	// §A). silver_mine is retired: a mine on a silver-deposit hex now
	// produces silver, exactly like copper/tin — there is no longer a second
	// building type to hint the player toward.
	if isHexBound {
		var pq, pr int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT map_q, map_r FROM provinces WHERE id = $1`, provinceID,
		).Scan(&pq, &pr)
		center := hexgrid.Coord{Q: pq, R: pr}
		chosen := hexgrid.Coord{Q: *req.HexQ, R: *req.HexR}

		inRing := false
		for _, c := range hexgrid.Ring(center, hexgrid.CatchmentRadius) {
			if c == chosen {
				inRing = true
				break
			}
		}
		if !inRing {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("hex (%d,%d) is not within this settlement's production catchment (within %d steps)",
					chosen.Q, chosen.R, hexgrid.CatchmentRadius))
			return
		}

		supports, err := economy.HexSupportsBuilding(r.Context(), h.pool, worldID, chosen, req.BuildingType)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check hex")
			return
		}
		if !supports {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("a %s at (%d,%d) would produce nothing — the terrain/deposit there doesn't support it",
					req.BuildingType, chosen.Q, chosen.R))
			return
		}
		// One building per hex (Timothy 2026-09-30): any OTHER hex-bound
		// building, standing or queued, by this or a neighbouring settlement.
		occ, err := economy.HexOccupants(r.Context(), h.pool, worldID, []hexgrid.Coord{chosen})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not check hex")
			return
		}
		if o, taken := occ[chosen]; taken && (o.BuildingType != req.BuildingType || o.SettlementID != settlementID) {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("hex (%d,%d) already has a %s — one building per hex",
					chosen.Q, chosen.R, o.BuildingType))
			return
		}
		// A completed building of the SAME type already standing on this
		// exact hex is not rejected here — that is the upgrade path
		// ("Uppgradering adresserar byggnaden via (typ, hex)"), handled below
		// exactly like a city building's repeat-build-raises-level branch,
		// just keyed on (type, hex) instead of (type) alone. Only a build
		// ALREADY QUEUED for this same hex is rejected — the hex-scoped
		// dupQueued check further below.
	}

	// Queue guards — block before we deduct resources.
	// 1. Walls/towers/bronze walls upgrade an existing wall_level; everything else
	//    is a one-instance building (production_rules use UPSERT, duplicates waste resources).
	// 2. No double-queueing the same building.
	// 3. Cap concurrent queue at maxParallelBuilds (package-level const above).
	bType := province.BuildingType(req.BuildingType)
	isWall := req.BuildingType == "wall"

	currentLevel := 0
	var buildCity string
	if err := h.pool.QueryRow(r.Context(), `SELECT name FROM settlements WHERE id=$1`, settlementID).Scan(&buildCity); err != nil {
		writeError(w, 500, "could not load build city")
		return
	}
	if !isWall && !province.LevelledBuildings[bType] {
		var alreadyBuilt bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = $2)`,
			settlementID, req.BuildingType,
		).Scan(&alreadyBuilt)
		if alreadyBuilt {
			writeError(w, http.StatusUnprocessableEntity, "building already exists")
			return
		}
	} else if !isWall {
		// Producing building: repeat-building raises its level, and the level is how
		// many citizens the workplace can employ (economy.LaborCapacity). Cost for the
		// NEXT level comes from LevelledSpec — level 1 unchanged, level 2+ adds cedar.
		// Hex-bound types key this off (type, hex) — MAX(level) across the whole
		// settlement would otherwise level up the WRONG hex's farm (or read a
		// level that doesn't exist yet on THIS hex at all).
		var lvl int
		if isHexBound {
			_ = h.pool.QueryRow(r.Context(),
				`SELECT COALESCE(level, 0) FROM buildings WHERE settlement_id = $1 AND building_type = $2 AND hex_q = $3 AND hex_r = $4`,
				settlementID, req.BuildingType, *req.HexQ, *req.HexR,
			).Scan(&lvl)
		} else {
			_ = h.pool.QueryRow(r.Context(),
				`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = $2 AND god_id IS NOT DISTINCT FROM $3`,
				settlementID, req.BuildingType, godArg,
			).Scan(&lvl)
		}
		currentLevel = lvl
		if lvl >= province.MaxBuildingLevel {
			writeError(w, http.StatusUnprocessableEntity,
				(buildErrorContext{BuildQuote: quoteBuild(bType, lvl, req.HexQ, req.HexR, false).forGod(godArg), City: buildCity}).message(""))
			return
		}
		nextSpec, specOK := province.LevelledSpec(bType, lvl+1)
		if !specOK {
			writeError(w, http.StatusUnprocessableEntity, "this building has no further levels")
			return
		}
		spec = nextSpec
	} else {
		var wl int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT wall_level FROM settlements WHERE id = $1`, settlementID,
		).Scan(&wl)
		currentLevel = wl
		if wl >= 3 {
			writeError(w, http.StatusUnprocessableEntity, (buildErrorContext{BuildQuote: quoteBuild(bType, wl, nil, nil, false), City: buildCity}).message(""))
			return
		}
		// The wall is the population ceiling, and it may not outgrow the stores:
		// wall N needs a Granary of level N (province.SiloLevelForWall).
		if economy.SiloLevel(r.Context(), h.pool, settlementID) < province.SiloLevelForWall(wl+1) {
			writeError(w, http.StatusUnprocessableEntity, province.WallNeedsSiloRefusal(wl+1))
			return
		}
		// Use the cost/duration for the next wall level (1–3); wl is 0–2 here.
		spec = province.WallLevelSpecs[wl+1]
	}

	buildContext := buildErrorContext{BuildQuote: quoteBuild(bType, currentLevel, req.HexQ, req.HexR, false).forGod(godArg), City: buildCity}
	var queueDepth int
	var dupQueued bool
	if isHexBound {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT
			   COUNT(*),
			   COUNT(*) FILTER (WHERE building_type = $2 AND hex_q = $3 AND hex_r = $4) > 0
			 FROM build_queue WHERE settlement_id = $1`,
			settlementID, req.BuildingType, *req.HexQ, *req.HexR,
		).Scan(&queueDepth, &dupQueued)
	} else {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT
			   COUNT(*),
			   COUNT(*) FILTER (WHERE building_type = $2 AND god_id IS NOT DISTINCT FROM $3) > 0
			 FROM build_queue WHERE settlement_id = $1`,
			settlementID, req.BuildingType, godArg,
		).Scan(&queueDepth, &dupQueued)
	}
	if dupQueued {
		writeError(w, http.StatusUnprocessableEntity, "this building is already in the queue")
		return
	}
	if queueDepth >= maxParallelBuilds {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("build queue is full (fixed at %d concurrent buildings; population and buildings do not add slots — finish or cancel a build)", maxParallelBuilds))
		return
	}

	// Deduct building costs (goods + silver) atomically in one transaction so a
	// silver shortfall can't leave the goods already committed (partial-drain).
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct resources")
		return
	}
	defer tx.Rollback(r.Context())

	if err := deductGoods(r.Context(), tx, settlementID, spec.Costs); err != nil {
		var insErr *insufficientGoodsError
		if errors.As(err, &insErr) {
			writeGoodsError(w, insErr, buildContext)
		} else {
			writeError(w, http.StatusInternalServerError, "could not deduct resources")
		}
		return
	}

	// Deduct silver if required.
	if spec.CostSilver > 0 {
		tag, err2 := province.DeductGood(r.Context(), tx, settlementID, "silver", spec.CostSilver)
		if err2 != nil || tag.RowsAffected() == 0 {
			// Name the shortfall (need/have) like writeGoodsError does for goods —
			// a bare "insufficient silver" left a silver-poor Wanax guessing.
			var have float64
			_ = tx.QueryRow(r.Context(),
				`SELECT settled(amount, rate, calc_tick) FROM settlement_goods
				  WHERE settlement_id = $1 AND good_key = 'silver'`,
				settlementID).Scan(&have)
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": fmt.Sprintf("insufficient silver (%s)", shortfall(spec.CostSilver, have)), "build_context": buildContext, "message": buildContext.message(fmt.Sprintf("silver (%s)", shortfall(spec.CostSilver, have)))})
			return
		}
	}

	// Paid, queued and scheduled in this one transaction: a failed queue
	// row or schedule rolls the payment back with it.
	var buildCurrentTick int
	_ = tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&buildCurrentTick)
	buildDueTick := buildCurrentTick + spec.DurationTicks
	completeAt := tick.EtaAt(h.clk, buildDueTick, buildCurrentTick)
	var queueID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO build_queue (settlement_id, world_id, building_type, complete_at, hex_q, hex_r, god_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		settlementID, worldID, req.BuildingType, completeAt, req.HexQ, req.HexR, godArg,
	).Scan(&queueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not queue build")
		return
	}

	if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledBuildComplete,
		combat.BuildCompletePayload{
			SettlementID: settlementID,
			BuildQueueID: queueID,
			BuildingType: req.BuildingType,
			HexQ:         req.HexQ,
			HexR:         req.HexR,
		}, buildDueTick,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule build")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not queue build")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"queue_id":           queueID,
		"complete_at":        completeAt,
		"complete_tick":      buildDueTick,
		"duration_game_days": spec.DurationTicks,
	})
}

var (
	errQueuedBuildGone        = errors.New("build already completed or not found")
	errUnknownQueuedBuildType = errors.New("unknown building type in queue")
)

// cancelQueuedBuild cancels one queued build inside tx: it works out what the
// queued build was charged, deletes the build_queue row (the atomic "still
// pending" check), deletes the BuildComplete scheduled event so the worker
// never fires, and refunds the costs to the queueing settlement. Shared by
// CancelBuild (the player's own cancel) and PlaceGubbe's hex take
// (megaron_plan_delad_catchment.md — the loser's queued build on a taken hex
// is cancelled and refunded the same way). Returns the building type that
// was cancelled; the caller owns commit/rollback and any ownership check.
func cancelQueuedBuild(ctx context.Context, tx economy.Tx, queueID uuid.UUID) (string, error) {
	var settlementID uuid.UUID
	var buildingType string
	var hexQ, hexR *int
	var godID *string
	if err := tx.QueryRow(ctx,
		`SELECT settlement_id, building_type, hex_q, hex_r, god_id FROM build_queue WHERE id = $1`,
		queueID,
	).Scan(&settlementID, &buildingType, &hexQ, &hexR, &godID); err != nil {
		return "", errQueuedBuildGone
	}

	spec, ok := province.BuildingSpecs[province.BuildingType(buildingType)]
	if !ok {
		return "", errUnknownQueuedBuildType
	}

	// For wall, refund the cost of the queued level (wall_level+1 at time of cancel,
	// since wall_level is only incremented on completion).
	if buildingType == "wall" {
		var wl int
		_ = tx.QueryRow(ctx, `SELECT wall_level FROM settlements WHERE id = $1`, settlementID).Scan(&wl)
		next := wl + 1
		if next < 1 {
			next = 1
		}
		if next > 3 {
			next = 3
		}
		spec = province.WallLevelSpecs[next]
	} else if bt := province.BuildingType(buildingType); province.LevelledBuildings[bt] {
		// Same reasoning as the wall above: buildings.level is only incremented on
		// completion, so the queued level is current+1. Refund what was actually
		// charged — otherwise cancelling a level-2 workplace silently ate its cedar.
		// Hex-bound types (hexQ/hexR non-nil) key this off (type, hex) — MAX(level)
		// across the whole settlement would read a DIFFERENT hex's farm level.
		var lvl int
		if hexQ != nil && hexR != nil {
			_ = tx.QueryRow(ctx,
				`SELECT COALESCE(level, 0) FROM buildings WHERE settlement_id = $1 AND building_type = $2 AND hex_q = $3 AND hex_r = $4`,
				settlementID, buildingType, *hexQ, *hexR,
			).Scan(&lvl)
		} else {
			_ = tx.QueryRow(ctx,
				`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = $2 AND god_id IS NOT DISTINCT FROM $3`,
				settlementID, buildingType, godID,
			).Scan(&lvl)
		}
		next := lvl + 1
		if next > province.MaxBuildingLevel {
			next = province.MaxBuildingLevel
		}
		if levelled, lok := province.LevelledSpec(bt, next); lok {
			spec = levelled
		}
	}

	// Delete the queue entry (atomic check: still pending).
	ct, err := tx.Exec(ctx, `DELETE FROM build_queue WHERE id = $1`, queueID)
	if err != nil || ct.RowsAffected() == 0 {
		return "", errQueuedBuildGone
	}

	// Cancel the scheduled event so the worker never fires.
	_, _ = tx.Exec(ctx,
		`DELETE FROM scheduled_events
		 WHERE event_type = 'BuildComplete'
		   AND (payload->>'build_queue_id')::uuid = $1
		   AND processed_at IS NULL`,
		queueID,
	)

	// Refund costs.
	for goodKey, qty := range spec.Costs {
		if _, err = tx.Exec(ctx,
			`SELECT credit_settlement_good($2,$3,$1,'build_refund')`,
			qty, settlementID, goodKey,
		); err != nil {
			return "", err
		}
	}
	return buildingType, nil
}

// CancelBuild handles DELETE /worlds/:worldID/provinces/:provinceID/build-queue/:queueID.
// Cancels a pending build, deletes the scheduled event, and refunds the costs.
func (h *ProvinceHandler) CancelBuild(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	queueID, err := uuid.Parse(chi.URLParam(r, "queueID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid queue ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Verify ownership (the queue entry itself is read inside cancelQueuedBuild).
	var settlementID uuid.UUID
	var buildingType string
	err = h.pool.QueryRow(r.Context(),
		`SELECT bq.settlement_id, bq.building_type
		 FROM build_queue bq
		 JOIN settlements s ON s.id = bq.settlement_id
		 WHERE bq.id = $1 AND bq.world_id = $2
		   AND s.province_id = $3 AND s.owner_id = $4`,
		queueID, worldID, provinceID, playerID,
	).Scan(&settlementID, &buildingType)
	if err != nil {
		writeError(w, http.StatusNotFound, "build queue entry not found or not yours")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := cancelQueuedBuild(r.Context(), tx, queueID); err != nil {
		switch {
		case errors.Is(err, errUnknownQueuedBuildType):
			writeError(w, http.StatusInternalServerError, "unknown building type in queue")
		case errors.Is(err, errQueuedBuildGone):
			writeError(w, http.StatusConflict, "build already completed or not found")
		default:
			writeError(w, http.StatusInternalServerError, "could not refund goods")
		}
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"cancelled": buildingType})
}

// Buildings handles GET /worlds/:worldID/provinces/:provinceID/buildings.
func (h *ProvinceHandler) Buildings(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}

	settlementID, err := resolveSettlementID(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, building_type, level, built_at, hex_q, hex_r, coalesce(god_id,'') FROM buildings WHERE settlement_id = $1 ORDER BY built_at`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load buildings")
		return
	}
	defer rows.Close()

	type buildingRow struct {
		ID      uuid.UUID `json:"id"`
		Type    string    `json:"type"`
		Level   int       `json:"level"`
		BuiltAt time.Time `json:"built_at"`
		// HexQ/HexR are set only for hex-bound buildings (province.HexBoundBuildings)
		// — null for a city building (megaron_plan_byggnad_pa_hex.md §A).
		HexQ *int `json:"hex_q,omitempty"`
		HexR *int `json:"hex_r,omitempty"`
		// CurrentLevel mirrors Level.
		CurrentLevel     int                      `json:"current_level"`
		GodID            string                   `json:"god_id,omitempty"`
		Name             string                   `json:"name,omitempty"`
		DemolitionKharis *kharis.TempleDemolition `json:"demolition_kharis,omitempty"`
	}
	var result []buildingRow
	for rows.Next() {
		var b buildingRow
		if err := rows.Scan(&b.ID, &b.Type, &b.Level, &b.BuiltAt, &b.HexQ, &b.HexR, &b.GodID); err == nil {
			b.CurrentLevel = b.Level
			if b.GodID != "" {
				b.Name = religion.TempleName(b.GodID)
			}
			result = append(result, b)
		}
	}
	rows.Close()

	// Quotes contain realm standing and therefore belong only to the owner.
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if authenticated {
		var owner *uuid.UUID
		if err := h.pool.QueryRow(r.Context(), `SELECT owner_id FROM settlements WHERE id=$1`, settlementID).Scan(&owner); err != nil {
			writeError(w, 500, "could not load building owner")
			return
		}
		if owner != nil && *owner == playerID {
			for i := range result {
				if result[i].Type == "temple" {
					quote, err := kharis.ProjectTempleDemolition(r.Context(), h.pool, playerID, worldID, result[i].ID)
					if err != nil {
						writeError(w, 500, "could not quote temple demolition")
						return
					}
					result[i].DemolitionKharis = quote
				}
			}
		}
	}
	if result == nil {
		result = []buildingRow{}
	}
	writeJSON(w, http.StatusOK, result)
}

// BuildingCatalogue handles GET /api/v1/buildings — returns the static catalogue of
// all constructable buildings: costs, duration, gate (requires_coastal, requires_deposit,
// requires_terrain) joined from production_rules, and a human purpose string.
// No world auth required — this is static reference data.
func (h *ProvinceHandler) BuildingCatalogue(w http.ResponseWriter, r *http.Request) {
	// Load requires_coastal, requires_deposit and terrain_type per building_type
	// from production_rules. A building may have multiple rules; we collect all
	// deposit requirements, collapse coastal to a single bool (any rule requiring
	// coastal → true), and — for requires_terrain — only flag a terrain set when
	// EVERY rule for that building names a terrain (no NULL/"any terrain" row).
	// lumbermill och mine har terräng-konditionerade BONUS-rader vid sidan av en
	// terrängfri basrad, producerar alltså något överallt och flaggas inte.
	// farm, olive_press och winery har ENBART terrängrader och är därmed
	// genuint gateade — en farm på kalksten producerar ingenting alls, tyst,
	// vilket är exakt den dolda gate P10 (soak 2026-07-18) stänger.
	// (Kommentaren nämnde tidigare farm som exempel på MOTSATSEN. Fel sedan
	// migration 008 — farm har aldrig haft en NULL-terrängrad. Rättat 2026-07-26.)
	type gateInfo struct {
		requiresCoastal  bool
		requiresDeposits []string
		terrains         map[string]bool
		hasAnyTerrain    bool // saw a NULL terrain_type row (matches any terrain)
	}
	gates := map[string]*gateInfo{}

	rows, err := h.pool.Query(r.Context(),
		`SELECT building_type, terrain_type, requires_coastal, requires_deposit
		 FROM production_rules
		 WHERE building_type IS NOT NULL
		   -- P6's terrain-free refining rows (olive_press/winery/foundry — a
		   -- pressarbetare/vinmakare/gjutare's OWN capacity) are NOT the same
		   -- "produces regardless of terrain" shape lumbermill/mine's
		   -- unconditional trickle rows are: they still yield zero without a
		   -- terrain-side extraction gubbe elsewhere in the catchment
		   -- (economy.RecomputeProduction's weakest-link/stock-drain steps).
		   -- Counting them here would silently drop the winery/olive_press
		   -- terrain warning P10 exists for. Exclude by good_key, not building
		   -- type, so a future terrain-free good on the SAME building type
		   -- doesn't need this list touched.
		   AND NOT (terrain_type IS NULL AND good_key IN ('oil', 'wine', 'bronze'))`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var bt string
			var terrain *string
			var coastal bool
			var deposit *string
			if err := rows.Scan(&bt, &terrain, &coastal, &deposit); err != nil {
				continue
			}
			g, ok := gates[bt]
			if !ok {
				g = &gateInfo{terrains: map[string]bool{}}
				gates[bt] = g
			}
			if coastal {
				g.requiresCoastal = true
			}
			if deposit != nil && *deposit != "" {
				// No DISTINCT in the query above (terrain_type now varies per row for
				// the same building/deposit pair) — dedupe here instead.
				dup := false
				for _, d := range g.requiresDeposits {
					if d == *deposit {
						dup = true
						break
					}
				}
				if !dup {
					g.requiresDeposits = append(g.requiresDeposits, *deposit)
				}
			}
			if terrain == nil {
				g.hasAnyTerrain = true
			} else {
				g.terrains[*terrain] = true
			}
		}
	}

	// The rule-table buildings (byggnadsregeln) have no production_rules rows any
	// more: their gate comes from where the rule table lets them stand.
	for _, bt := range economy.RuleBuildingTypes {
		terrains, deposits := economy.RuleBuildingGate(bt)
		g := &gateInfo{terrains: map[string]bool{}, requiresDeposits: deposits}
		for _, t := range terrains {
			g.terrains[t] = true
		}
		gates[bt] = g
	}

	type buildingEntry struct {
		Type       string             `json:"type"`
		Costs      map[string]float64 `json:"costs"`
		CostSilver float64            `json:"cost_silver,omitempty"`
		// Game-days, not wall-clock minutes: DurationTicks is a TICK count and
		// one tick IS one game-day (megaron_plan_ticket_ar_dygnet). The old
		// duration_minutes field reported spec.DurationTicks*TickMinutes,
		// which floors to a 1-minute-per-tick granularity and silently gave
		// wrong figures at a sub-minute TICK_SECONDS dev cadence — same class
		// as the rite-cooldown fix (cli-sanning row K / commit 2769042).
		// Routed through tick.GameDaysLeft/RealUntil rather than assigned as
		// a bare int so the unit stays explicit and consistent with the
		// cooldown sibling; the round trip is exact here since DurationTicks
		// is already a whole tick count (no fractional remainder to round).
		DurationGameDays int      `json:"duration_game_days"`
		RequiresCoastal  bool     `json:"requires_coastal,omitempty"`
		RequiresDeposits []string `json:"requires_deposits,omitempty"`
		RequiresTerrain  []string `json:"requires_terrain,omitempty"`
		Purpose          string   `json:"purpose"`
		// MaxLevel is 1 for one-instance buildings and MaxBuildingLevel for producing
		// ones, whose level is how many citizens the workplace can employ.
		MaxLevel int `json:"max_level"`
		// UpgradeCosts maps level → full cost of taking the building to that level
		// (level 1 is Costs above). Empty for buildings with no level ladder.
		UpgradeCosts map[int]map[string]float64 `json:"upgrade_costs,omitempty"`
		// HexBound is true for farm/mine/lumbermill/stonequarry
		// (province.HexBoundBuildings) — the build request must name a hex_q/
		// hex_r, and a settlement may build several of this type, one per hex
		// (megaron_plan_byggnad_pa_hex.md §A).
		HexBound bool `json:"hex_bound,omitempty"`
		// WallLadder (wall only): what each wall level costs, which Granary level it
		// needs, and the population the walls then hold. The wall is the ceiling.
		WallLadder []wallStep `json:"wall_ladder,omitempty"`
	}

	// Stable ordering: sort building types alphabetically.
	order := make([]string, 0, len(province.BuildingSpecs))
	for bt := range province.BuildingSpecs {
		order = append(order, string(bt))
	}
	sort.Strings(order)

	result := make([]buildingEntry, 0, len(order))
	for _, bt := range order {
		spec := province.BuildingSpecs[province.BuildingType(bt)]
		entry := buildingEntry{
			Type:             bt,
			Costs:            spec.Costs,
			CostSilver:       spec.CostSilver,
			DurationGameDays: tick.GameDaysLeft(tick.RealUntil(spec.DurationTicks, 0)),
			Purpose:          province.BuildingPurposes[province.BuildingType(bt)],
			MaxLevel:         1,
			HexBound:         province.HexBoundBuildings[province.BuildingType(bt)],
		}
		if province.LevelledBuildings[province.BuildingType(bt)] {
			entry.MaxLevel = province.MaxBuildingLevel
			entry.UpgradeCosts = make(map[int]map[string]float64, province.MaxBuildingLevel-1)
			for level := 2; level <= province.MaxBuildingLevel; level++ {
				if levelled, lok := province.LevelledSpec(province.BuildingType(bt), level); lok {
					entry.UpgradeCosts[level] = levelled.Costs
				}
			}
		}
		if province.NeedsWaterNeighbour(province.BuildingType(bt)) {
			entry.RequiresCoastal = true
		}
		if province.BuildingType(bt) == province.BuildingWall {
			entry.WallLadder = wallLadder()
		}
		if g, ok := gates[bt]; ok {
			entry.RequiresCoastal = entry.RequiresCoastal || g.requiresCoastal
			if len(g.requiresDeposits) > 0 {
				sort.Strings(g.requiresDeposits)
				entry.RequiresDeposits = g.requiresDeposits
			}
			if !g.hasAnyTerrain && len(g.terrains) > 0 {
				terrains := make([]string, 0, len(g.terrains))
				for t := range g.terrains {
					terrains = append(terrains, t)
				}
				sort.Strings(terrains)
				entry.RequiresTerrain = terrains
			}
		}
		result = append(result, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

// wallStep is one rung of the wall ladder as the catalogue tells it.
type wallStep struct {
	Level         int                `json:"level"`
	Name          string             `json:"name"`
	Costs         map[string]float64 `json:"costs"`
	RequiresSilo  int                `json:"requires_silo"`
	PopulationCap int                `json:"population_cap"`
}

func wallLadder() []wallStep {
	out := make([]wallStep, 0, 3)
	for lvl := 1; lvl <= 3; lvl++ {
		out = append(out, wallStep{Level: lvl, Name: province.WallLevelNames[lvl], Costs: province.WallLevelSpecs[lvl].Costs,
			RequiresSilo: province.SiloLevelForWall(lvl), PopulationCap: economy.PopulationCap(lvl)})
	}
	return out
}

// UnitCatalogue handles GET /api/v1/units — returns the static catalogue of all
// recruitable unit types: the resource cost for one recruit action (a whole
// 100-man cohort for land units, one vessel for naval — the same quantities
// `recruitPerManCosts` scales up to deduct), the population-pool gate,
// training duration, and the building requirement. No world/auth required —
// static reference data, mirrors BuildingCatalogue.
func (h *ProvinceHandler) UnitCatalogue(w http.ResponseWriter, r *http.Request) {
	type unitEntry struct {
		Type     string             `json:"type"`
		Costs    map[string]float64 `json:"costs"`
		BatchMen int                `json:"batch_men"` // men (land) or crew (naval) the Costs above pay for in one recruit call
		PopCost  int                `json:"pop_cost"`
		// Game-days, not wall-clock minutes — see buildingEntry.DurationGameDays
		// above for the full rationale (same fix, same class of bug).
		DurationGameDays int  `json:"duration_game_days"`
		RequiresBarracks bool `json:"requires_barracks,omitempty"`
		RequiresStable   bool `json:"requires_stable,omitempty"`
		RequiresHarbour  bool `json:"requires_harbour,omitempty"`
		RequiresShipyard bool `json:"requires_shipyard,omitempty"`
		RequiresFoundry  bool `json:"requires_foundry,omitempty"`
	}

	// Stable ordering: sort unit types alphabetically (mirrors BuildingCatalogue).
	order := make([]string, 0, len(province.UnitSpecs))
	for ut := range province.UnitSpecs {
		order = append(order, ut)
	}
	sort.Strings(order)

	result := make([]unitEntry, 0, len(order))
	for _, ut := range order {
		spec := province.UnitSpecs[ut]
		// Kohort-rekrytering: one land recruit call always drafts the full
		// 100-man cohort (economy.MaxUnitSize), not a 10-man batch.
		batchMen := economy.MaxUnitSize
		if unit.CategoryOf(unit.Type(ut)) == unit.CategoryNaval {
			batchMen = unit.CrewFor(unit.Type(ut))
		}
		costs := make(map[string]float64, len(spec.Costs))
		for g, v := range spec.Costs {
			costs[g] = v * float64(batchMen)
		}
		result = append(result, unitEntry{
			Type:             ut,
			Costs:            costs,
			BatchMen:         batchMen,
			PopCost:          spec.PopCost,
			DurationGameDays: tick.GameDaysLeft(tick.RealUntil(spec.DurationTicks, 0)),
			RequiresBarracks: spec.RequiresBarracks,
			RequiresStable:   spec.RequiresStable,
			RequiresHarbour:  spec.RequiresHarbour,
			RequiresShipyard: spec.RequiresShipyard,
			RequiresFoundry:  spec.RequiresFoundry,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// RecipeCatalogue handles GET /api/v1/recipes — returns the static catalogue
// of all refining recipes: output good + quantity, the building type that
// refines it, and the full ingredient list. No world/auth required — static
// reference data, mirrors BuildingCatalogue/UnitCatalogue.
//
// Recipes live in the recipes/recipe_ingredients tables (not a Go map like
// BuildingSpecs/UnitSpecs) — economy.RecomputeProduction's bronze stock-drain
// step (P6, megaron_plan_fysisk_gubbemodell.md §P6) reads them generically by
// output_key at tick time, so this handler is a read-only mirror of that same
// data, not a second source of truth. It exists because the web client used
// to hardcode recipe strings and broke silently when a recipe changed (bronze's copper:tin
// ratio, mig 099, 2026-07-25): this lets the client ask the server instead.
func (h *ProvinceHandler) RecipeCatalogue(w http.ResponseWriter, r *http.Request) {
	type ingredientEntry struct {
		GoodKey  string  `json:"good_key"`
		Quantity float64 `json:"quantity"`
	}
	type recipeEntry struct {
		ID           int               `json:"id"`
		OutputKey    string            `json:"output_key"`
		OutputQty    float64           `json:"output_qty"`
		BuildingType string            `json:"building_type"`
		Ingredients  []ingredientEntry `json:"ingredients"`
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, output_key, output_qty, building_type FROM recipes ORDER BY id`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load recipes")
		return
	}
	defer rows.Close()

	// Stable ordering: DB id order (mirrors insertion/seed order — recipes
	// only ever grow via migration, so this is deterministic across requests).
	order := make([]int, 0, 8)
	byID := make(map[int]*recipeEntry, 8)
	for rows.Next() {
		var e recipeEntry
		if err := rows.Scan(&e.ID, &e.OutputKey, &e.OutputQty, &e.BuildingType); err != nil {
			continue
		}
		e.Ingredients = []ingredientEntry{}
		byID[e.ID] = &e
		order = append(order, e.ID)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load recipes")
		return
	}

	// One query for all recipes' ingredients, grouped by recipe_id in Go
	// (mirrors BuildingCatalogue's single-pass gather-by-key over production_rules).
	ingRows, err := h.pool.Query(r.Context(),
		`SELECT recipe_id, good_key, quantity FROM recipe_ingredients ORDER BY recipe_id, good_key`,
	)
	if err == nil {
		defer ingRows.Close()
		for ingRows.Next() {
			var recipeID int
			var ing ingredientEntry
			if err := ingRows.Scan(&recipeID, &ing.GoodKey, &ing.Quantity); err != nil {
				continue
			}
			if e, ok := byID[recipeID]; ok {
				e.Ingredients = append(e.Ingredients, ing)
			}
		}
	}

	result := make([]recipeEntry, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	writeJSON(w, http.StatusOK, result)
}

// recruitPerManCosts returns the resource cost per man for a given unit type.
// Derived from Skalbeslut (2026-06-15): per-man = old UnitSpec cost / old PopCost.
// A land cohort = 100 men → total cost = per-man × 100 (kohort-rekrytering,
// megaron_plan_rekryteringsmodell.md — was a caller-chosen 10-man batch
// before). All siffror are tunable at reseed. recruitPerManCosts delegates to
// province.UnitSpecs so this handler and the capabilities recruit checker
// (keryx actions) read the exact same per-man cost table — before Fas 3 they
// were two separately hand-maintained copies that had already drifted apart
// (temenos_capabilities.md Fas 3). The same table is also what
// economy.RecruitCostPerMan exposes to kharis/tick.go's reinforce trickle,
// which prices a partial refill pro-rata against these same per-man numbers.
func recruitPerManCosts(unitType string) map[string]float64 {
	spec, ok := province.UnitSpecs[unitType]
	if !ok {
		return nil
	}
	return spec.Costs
}

// recruitBatchTicks returns the training ticks for one 100-man cohort.
func recruitBatchTicks(unitType string) int {
	spec, ok := province.UnitSpecs[unitType]
	if !ok {
		return 1
	}
	return spec.DurationTicks
}

// Recruit handles POST /worlds/:worldID/provinces/:provinceID/recruit.
//
// Kohort-rekrytering: soldiers are drafted from the population a whole
// 100-man cohort at a time (1 gubbe) — `men` is ignored for land requests
// (forced to economy.MaxUnitSize above). Request: {"unit_type": "spearman"}.
// Population is decremented immediately; resources are deducted up-front.
// A forming unit is created (or grown, for a legacy partial straggler) in the
// units table; one TrainComplete is scheduled once it reaches 100. At
// size == 100 the unit becomes deployable (garrison). A cohort later
// decimated in battle regenerates over time via the reinforce trickle
// (POST .../units/{id}/reinforce, kharis/tick.go applyReinforcement) instead
// of a fresh partial recruit. Naval units (galley/war_galley/merchantman)
// skip the 100-forming gate: they are deployable as soon as their crew is
// drafted (one vessel = one unit, size always 1).
//
// Payment, unit creation and training jobs share one transaction.
func (h *ProvinceHandler) Recruit(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		UnitType string `json:"unit_type"`
		Men      int    `json:"men"`
		Count    int    `json:"count"`
		// Name is naval-only: an optional Wanax-chosen ship name. Ignored for
		// land units. If omitted, a name is suggested from a culture-appropriate
		// pool (ship-build overhaul 2026-07-09).
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Normalize legacy/alias unit-type strings ("ship"/"trireme"→"galley",
	// "chariot"→"war_chariot") so old clients (web, cached CLI) that still send
	// the pre-rename value keep recruiting instead of hitting "unknown unit
	// type" after namn-hygien A+B (mig 084) made galley/war_chariot the only
	// units.type values.
	req.UnitType = unit.Canonical(req.UnitType)

	spec, specOK := province.UnitSpecs[req.UnitType]
	if !specOK {
		writeError(w, http.StatusBadRequest, "unknown unit type")
		return
	}
	perManCosts := recruitPerManCosts(req.UnitType)
	if perManCosts == nil {
		writeError(w, http.StatusBadRequest, "unknown unit type")
		return
	}

	// Skepp-taxonomi (temenos_enheter.md "Flottdesign", Timothy 2026-07-02):
	// naval units are built ONE VESSEL AT A TIME with a fixed crew per type
	// (unit.CrewFor) — `men` never applies to them and is ignored if sent.
	//
	// Kohort-rekrytering (megaron_plan_rekryteringsmodell.md, Timothy
	// 2026-08-19): land recruitment is no longer a caller-chosen 10–100-men
	// batch — one Recruit call always drafts a whole 100-man cohort (1 gubbe).
	// Any `men` the client sends is ignored for land, exactly like naval
	// already ignores it for crew — ships and men both draft a fixed size per
	// call now, just different fixed sizes. A decimated cohort is topped back
	// up over time by the reinforce trickle (kharis/tick.go
	// applyReinforcement), not by recruiting partial batches.
	uType := unit.Type(req.UnitType)
	cat := unit.CategoryOf(uType)
	if cat == unit.CategoryLand {
		req.Men = economy.MaxUnitSize
	}
	if req.Count == 0 {
		req.Count = 1
	}
	if req.Count < 1 || req.Count > 20 {
		writeError(w, http.StatusBadRequest, "count must be 1–20")
		return
	}
	effectiveCount := 1
	if cat == unit.CategoryNaval {
		effectiveCount = req.Count
	}
	crew := unit.CrewFor(uType)

	// Verify ownership via settlement.
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	settlementID, err := resolveSettlementID(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no settlement in province")
		return
	}

	// Naval build-queue cap: at most ten vessels building per settlement.
	// Land TrainComplete jobs do not occupy the shipyard; legacy naval timers
	// and saved shipyard work count each vessel only once.
	if cat == unit.CategoryNaval {
		var pendingBuilds int
		_ = h.pool.QueryRow(r.Context(),
			navalBuildQueueCountSQL,
			worldID, settlementID,
		).Scan(&pendingBuilds)
		if pendingBuilds+effectiveCount > 10 {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("build queue would overflow: %d pending + %d new > 10", pendingBuilds, effectiveCount))
			return
		}
	}

	// C2/C-collapse: men are drawn from population at recruit time.
	// Allow draining population down to 1 (or below 100 → triggers city collapse).
	// No hard floor here: if the Wanax chooses to overmobilise, the city collapses.
	var population int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT population FROM settlements WHERE id = $1 AND state != 'collapsed'`,
		settlementID,
	).Scan(&population)

	// Validate the requested type below so a missing good is named, rather
	// than rejecting through the aggregate availability of any recruit.
	// totalMen is the actual head-count drafted from population — for naval
	// this is crew (fixed per type), never req.Men.
	totalMen := req.Men * effectiveCount
	if cat == unit.CategoryNaval {
		totalMen = crew * effectiveCount
	}
	if totalMen >= population {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("insufficient population: cannot draft %d men from a settlement of %d",
				totalMen, population))
		return
	}

	// Check building requirements.
	if spec.RequiresBarracks {
		var exists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'barracks')`,
			settlementID,
		).Scan(&exists)
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "barracks required")
			return
		}
	}
	if spec.RequiresStable {
		var exists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'stable')`,
			settlementID,
		).Scan(&exists)
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "stable required")
			return
		}
	}
	if spec.RequiresHarbour {
		var exists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'harbour')`,
			settlementID,
		).Scan(&exists)
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "harbour required")
			return
		}
	}
	if spec.RequiresShipyard {
		var exists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'shipyard')`,
			settlementID,
		).Scan(&exists)
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "shipyard required")
			return
		}
	}
	if spec.RequiresFoundry {
		var exists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'foundry')`,
			settlementID,
		).Scan(&exists)
		if !exists {
			// Distinguish "no foundry at all" from "foundry finishing" — the
			// latter is the surprise the player hits crafting right after a build
			// shows done: the BuildComplete event (which inserts the buildings row)
			// is a poll away. Point at the real cause instead of a bare "required".
			var queued bool
			_ = h.pool.QueryRow(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM build_queue WHERE settlement_id = $1 AND building_type = 'foundry')`,
				settlementID,
			).Scan(&queued)
			if queued {
				writeError(w, http.StatusUnprocessableEntity,
					"foundry is still finishing here — it becomes usable within a game day of completion; retry shortly")
			} else {
				writeError(w, http.StatusUnprocessableEntity,
					// Do NOT reintroduce "bronze is smelted at a foundry" here: war_galley
					// stopped costing bronze in bc1bbaa, and the stale parenthetical told a
					// playtester the two were still related (Antilokhos, 2026-07-23).
					"foundry required — build a foundry here first (a war galley is fitted out at a foundry)")
			}
			return
		}
	}

	// Compute total resource costs: per-man cost × number of men × count (naval batch).
	totalCosts := make(map[string]float64, len(perManCosts))
	for k, v := range perManCosts {
		totalCosts[k] = v * float64(totalMen)
	}
	totalKharis := spec.CostKharis * float64(totalMen)

	// The whole recruit is one transaction: payment, population, ordinal,
	// unit, durable events and training jobs either all commit or all roll back.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct resources")
		return
	}
	defer tx.Rollback(r.Context())

	// Serialize recruits for this settlement before locking goods. Re-read
	// ownership and population; the fast prechecks are not mutation authority.
	if err := tx.QueryRow(r.Context(),
		`SELECT population FROM settlements
		 WHERE id = $1 AND world_id = $2 AND owner_id = $3 AND state != 'collapsed'
		 FOR UPDATE`, settlementID, worldID, playerID,
	).Scan(&population); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "settlement is no longer available for recruitment")
		} else {
			writeError(w, http.StatusInternalServerError, "could not lock recruitment settlement")
		}
		return
	}
	if totalMen >= population {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("insufficient population: cannot draft %d men from a settlement of %d", totalMen, population))
		return
	}
	for _, building := range []struct {
		name     string
		required bool
	}{
		{"barracks", spec.RequiresBarracks}, {"stable", spec.RequiresStable},
		{"harbour", spec.RequiresHarbour}, {"shipyard", spec.RequiresShipyard},
		{"foundry", spec.RequiresFoundry},
	} {
		if !building.required {
			continue
		}
		var exists bool
		if err := tx.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = $2)`,
			settlementID, building.name).Scan(&exists); err != nil {
			writeError(w, http.StatusInternalServerError, "could not check recruitment buildings")
			return
		}
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, building.name+" required")
			return
		}
	}
	if cat == unit.CategoryNaval {
		var pendingBuilds int
		if err := tx.QueryRow(r.Context(),
			navalBuildQueueCountSQL,
			worldID, settlementID).Scan(&pendingBuilds); err != nil {
			writeError(w, http.StatusInternalServerError, "could not check ship build queue")
			return
		}
		if pendingBuilds+effectiveCount > 10 {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("build queue would overflow: %d pending + %d new > 10", pendingBuilds, effectiveCount))
			return
		}
	}

	if err := deductGoods(r.Context(), tx, settlementID, totalCosts); err != nil {
		var insErr *insufficientGoodsError
		if errors.As(err, &insErr) {
			writeGoodsError(w, insErr)
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not deduct resources: %v", err))
		}
		return
	}

	// Deduct kharis (if any).
	if totalKharis > 0 {
		tag, err2 := tx.Exec(r.Context(),
			`UPDATE player_world_records SET
			   kharis_amount = settled(kharis_amount, kharis_rate, kharis_calc_tick) - $1,
			   kharis_calc_tick = current_world_tick()
			 WHERE player_id = $2 AND world_id = $3
			   AND settled(kharis_amount, kharis_rate, kharis_calc_tick) >= $1`,
			totalKharis, playerID, worldID,
		)
		if err2 != nil {
			writeError(w, http.StatusInternalServerError, "could not deduct recruitment kharis")
			return
		}
		if tag.RowsAffected() == 0 {
			// Name the shortfall (need/have) — kharis lives on the per-Wanax pool.
			var have float64
			_ = tx.QueryRow(r.Context(),
				`SELECT settled(kharis_amount, kharis_rate, kharis_calc_tick)
				   FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
				playerID, worldID).Scan(&have)
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("insufficient kharis (need %.1f, have %.1f)", totalKharis, have))
			return
		}
	}

	// C2: deduct population immediately — men leave civilian life to form up.
	var popAfter int
	if err := tx.QueryRow(r.Context(),
		`UPDATE settlements SET population = population - $1 WHERE id = $2
		 RETURNING population`,
		totalMen, settlementID,
	).Scan(&popAfter); err != nil {
		writeError(w, http.StatusInternalServerError, "could not draft men")
		return
	}

	// Collapse is part of the same durable action as the draft.
	if popAfter < economy.CollapseBelowPopulation {
		var collapseCurrentTick int
		if err := tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&collapseCurrentTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read recruitment game day")
			return
		}
		if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledCollapseSettlement,
			combat.CollapseSettlementPayload{SettlementID: settlementID, WorldID: worldID, Cause: "overmobilisation"},
			collapseCurrentTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule settlement collapse")
			return
		}
	}

	// One training duration per full land cohort or vessel.
	batchTicks := recruitBatchTicks(req.UnitType)
	var trainCurrentTick int
	if err := tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&trainCurrentTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read recruitment game day")
		return
	}

	// Naval-only: resolve the Wanax's culture (from their capital settlement,
	// same pattern as the music player's capital.culture on the web client)
	// for a culture-appropriate name suggestion, and collect this Wanax's
	// existing ship names in this world so we can steer clear of a repeat.
	var culture string
	takenNames := make(map[string]bool)
	if cat == unit.CategoryNaval {
		if err := tx.QueryRow(r.Context(),
			`SELECT culture_id FROM settlements WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
			playerID, worldID,
		).Scan(&culture); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "could not read ship naming culture")
			return
		}
		nameRows, nameErr := tx.Query(r.Context(),
			`SELECT name FROM units WHERE owner_id = $1 AND world_id = $2 AND name IS NOT NULL`,
			playerID, worldID,
		)
		if nameErr != nil {
			writeError(w, http.StatusInternalServerError, "could not read existing ship names")
			return
		}
		for nameRows.Next() {
			var n string
			if err := nameRows.Scan(&n); err != nil {
				nameRows.Close()
				writeError(w, http.StatusInternalServerError, "could not read existing ship names")
				return
			}
			takenNames[n] = true
		}
		nameErr = nameRows.Err()
		nameRows.Close()
		if nameErr != nil {
			writeError(w, http.StatusInternalServerError, "could not read existing ship names")
			return
		}
	}

	var committedEvents []*events.Event
	var unitIDs []uuid.UUID
	var unitNames []string
	var lastCompleteAt time.Time
	var finalSize int
	// trainingStarted/menNeeded carry the forming→training truth into the
	// response (land only): a Recruit call that leaves a unit under
	// MaxUnitSize looks identical to a hung pipeline unless the client is told
	// explicitly what's missing and that nothing trains yet (see forming
	// legibility, 2026-07-30).
	var trainingStarted bool
	var menNeeded int

	for n := 0; n < effectiveCount; n++ {
		if cat == unit.CategoryNaval {
			// One vessel per iteration: always a new row, size fixed at 1, no
			// reinforcement of an existing forming unit (that batching model is
			// land-only). Build takes batchTicks (one event, not a batch loop).
			chosenName := strings.TrimSpace(req.Name)
			if n > 0 || chosenName == "" {
				chosenName = shipnames.Suggest(culture, takenNames)
			}
			takenNames[chosenName] = true

			dueTick := trainCurrentTick + batchTicks
			completeAt := tick.EtaAt(h.clk, dueTick, trainCurrentTick)
			lastCompleteAt = completeAt

			// Ordinalen delas ut ur den monotona räknaren. Skepp bär den inte i
			// sitt namn (de har egennamn) men den finns för fullständighetens
			// skull och för att räknaren aldrig ska hoppa.
			shipOrdinal, err := unit.AllocateOrdinal(r.Context(), tx, settlementID, string(uType))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not allocate ship ordinal")
				return
			}
			var unitID uuid.UUID
			if err := tx.QueryRow(r.Context(),
				`INSERT INTO units
				   (world_id, owner_id, type, category, size, crew, status, settlement_id,
				    support_settlement_id, ordinal, name, build_complete_at)
				 VALUES ($1, $2, $3, $4, 1, $5, 'forming', $6, $6, $7, $8, $9)
				 RETURNING id`,
				worldID, playerID, string(uType), string(cat),
				crew, settlementID, shipOrdinal, chosenName, completeAt,
			).Scan(&unitID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not create ship")
				return
			}
			if err := combat.RecordRecruitReservation(r.Context(), tx, unitID, totalCosts, totalKharis, crew, totalMen, true); err != nil {
				writeError(w, http.StatusInternalServerError, "could not reserve ship costs")
				return
			}
			unitIDs = append(unitIDs, unitID)
			unitNames = append(unitNames, chosenName)
			finalSize = 1

			if err := economy.StartShipyardWork(r.Context(), tx, unitID, batchTicks); err != nil {
				writeError(w, http.StatusInternalServerError, "could not start shipyard work")
				return
			}

			// Outcome (Fas 2.3: the name was already chosen above, once) + row
			// (Fas 2.2/2.4: UnitFormed already existed for starter units; this is
			// its first use from Recruit, adding the optional Name field it was
			// defined with).
			if h.eventStore != nil {
				formedEvent, err := h.eventStore.AppendTx(r.Context(), tx, unitID, events.StreamType(unit.StreamUnit), unit.EventUnitFormed,
					unit.UnitFormedPayload{
						UnitID:       unitID,
						OwnerID:      playerID,
						WorldID:      worldID,
						SettlementID: settlementID,
						UnitType:     req.UnitType,
						Category:     string(cat),
						InitialSize:  1,
						Crew:         crew,
						PopDrawn:     crew,
						Name:         chosenName,
					}, worldID, nil,
				)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not record ship formation")
					return
				}
				committedEvents = append(committedEvents, formedEvent)
			}
			continue
		}

		// Land: grow (or create) this settlement's forming unit of this type. A
		// unit gathers men until it reaches 100, then enters `training` for one
		// duration (batchTicks = the type's DurationTicks) before deploying to
		// garrison — a single TrainComplete, not per-10-men batches. Men beyond
		// 100 spill into a fresh forming unit. (forming units are always < 100;
		// at 100 they become training, so an existing forming row is safe to top up.)
		var existingUnitID *uuid.UUID
		var existingSize int
		row := tx.QueryRow(r.Context(),
			`SELECT id, size FROM units
			 WHERE settlement_id = $1 AND type = $2 AND status = 'forming'
			 ORDER BY created_at LIMIT 1`,
			settlementID, string(uType),
		)
		var eid uuid.UUID
		if scanErr := row.Scan(&eid, &existingSize); scanErr == nil {
			existingUnitID = &eid
		} else if !errors.Is(scanErr, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "could not load forming unit")
			return
		}

		newSize := existingSize + req.Men
		unitSize := newSize
		if unitSize > economy.MaxUnitSize {
			unitSize = economy.MaxUnitSize // cap; the remainder spills into a new forming unit below
		}

		var unitID uuid.UUID
		if existingUnitID != nil {
			if err := tx.QueryRow(r.Context(),
				`UPDATE units SET size = $1, updated_at = now() WHERE id = $2 RETURNING id`,
				unitSize, *existingUnitID,
			).Scan(&unitID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not grow forming unit")
				return
			}
		} else {
			ordinal, err := unit.AllocateOrdinal(r.Context(), tx, settlementID, string(uType))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not allocate unit ordinal")
				return
			}
			if err := tx.QueryRow(r.Context(),
				`INSERT INTO units
				   (world_id, owner_id, type, category, size, crew, status,
				    settlement_id, support_settlement_id, ordinal, origin_settlement_id)
				 VALUES ($1, $2, $3, $4, $5, $6, 'forming', $7, $7, $8, $7)
				 RETURNING id`,
				worldID, playerID, string(uType), string(cat),
				unitSize, crew, settlementID, ordinal,
			).Scan(&unitID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not create forming unit")
				return
			}
		}
		if err := combat.RecordRecruitReservation(r.Context(), tx, unitID, totalCosts, totalKharis, unitSize-existingSize, totalMen, existingUnitID == nil); err != nil {
			writeError(w, http.StatusInternalServerError, "could not reserve training costs")
			return
		}
		unitIDs = append(unitIDs, unitID)
		finalSize = unitSize
		trainingStarted = newSize >= 100
		if !trainingStarted {
			menNeeded = economy.MaxUnitSize - finalSize
		}

		if newSize >= 100 {
			// Full → enter training: one completion event at now + the type's
			// training duration. build_complete_at carries the ready ETA (shared
			// with naval). Not deployable until TrainComplete flips it to garrison.
			dueTick := trainCurrentTick + batchTicks
			completeAt := tick.EtaAt(h.clk, dueTick, trainCurrentTick)
			lastCompleteAt = completeAt
			if _, err := tx.Exec(r.Context(),
				`UPDATE units SET status = 'training', build_complete_at = $1, updated_at = now() WHERE id = $2`,
				completeAt, unitID,
			); err != nil {
				writeError(w, http.StatusInternalServerError, "could not start unit training")
				return
			}
			if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTrainComplete,
				combat.TrainCompletePayload{
					SettlementID: settlementID,
					UnitType:     req.UnitType,
					Count:        100,
					UnitID:       unitID,
				}, dueTick,
			); err != nil {
				writeError(w, http.StatusInternalServerError, "could not schedule training")
				return
			}
			// Spill the overflow into a new forming unit of the same type.
			if newSize > 100 {
				spillOrdinal, err := unit.AllocateOrdinal(r.Context(), tx, settlementID, string(uType))
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not allocate spill ordinal")
					return
				}
				var spillID uuid.UUID
				if err := tx.QueryRow(r.Context(),
					`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
					                    settlement_id, support_settlement_id, ordinal, origin_settlement_id)
					 VALUES ($1, $2, $3, $4, $5, $6, 'forming', $7, $7, $8, $7) RETURNING id`,
					worldID, playerID, string(uType), string(cat), newSize-100, crew, settlementID, spillOrdinal,
				).Scan(&spillID); err != nil {
					writeError(w, http.StatusInternalServerError, "could not create spill forming unit")
					return
				}
				if err := combat.RecordRecruitReservation(r.Context(), tx, spillID, totalCosts, totalKharis, newSize-100, totalMen, true); err != nil {
					writeError(w, http.StatusInternalServerError, "could not reserve spill training costs")
					return
				}
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not complete recruitment")
		return
	}
	for _, e := range committedEvents {
		h.eventStore.RecordCommitted(r.Context(), e)
	}

	menForResponse := req.Men
	if cat == unit.CategoryNaval {
		menForResponse = crew
	}

	// Upkeep warning (P6, soak 2026-07-18): this unit draws no upkeep until it
	// finishes training and garrisons (upkeep.go only charges 'garrison'/
	// 'marching'/'positioned'), so warn NOW about what happens THEN — a Wanax
	// otherwise had no signal before a galley disbanded the instant it
	// garrisoned. Best-effort: a failed capacity read never blocks the recruit
	// that already succeeded above.
	var upkeepWarning string
	if netGrainPerDay, netSilverPerDay, capErr := settlementUpkeepCapacity(r.Context(), h.pool, settlementID); capErr == nil {
		fullSize := 100
		if cat == unit.CategoryNaval {
			fullSize = 1
		}
		newUnitUp := combat.UnitUpkeep(req.UnitType, string(cat), fullSize, "garrison", unit.CrewFor(unit.Type(req.UnitType)))
		if (netGrainPerDay-newUnitUp.Grain) < 0 || (netSilverPerDay-newUnitUp.Silver) < 0 {
			upkeepWarning = fmt.Sprintf(
				"warning: once this unit garrisons it needs %.1f grain + %.1f silver per game day upkeep — "+
					"this settlement's current net after its existing army's upkeep is %+.1f grain per game day, %+.1f silver per game day; "+
					"it may starve/desert without more production or fewer units (`keryx status`)",
				newUnitUp.Grain, newUnitUp.Silver, netGrainPerDay, netSilverPerDay)
		}
	}

	resp := map[string]any{
		"unit_id":      unitIDs[len(unitIDs)-1],
		"unit_ids":     unitIDs,
		"unit_type":    req.UnitType,
		"men":          menForResponse,
		"count":        effectiveCount,
		"complete_at":  lastCompleteAt,
		"forming_size": finalSize,
		"names":        unitNames,
		// Hål 3 (megaron_plan_tysta_forluster.md): totalMen/population/popAfter
		// were already computed above (the insufficient-population check at
		// ~line 1898 uses them) — a successful recruit just never told the
		// client. pop_drawn is the actual head-count the settlement lost,
		// population_before/after let the client confirm the delta itself
		// (same "verify against resulting state" pattern place/allocate use).
		"pop_drawn":         totalMen,
		"population_before": population,
		"population_after":  popAfter,
	}
	// Land only: tell the client explicitly whether this unit is still
	// gathering men or has just entered training, and — if still forming —
	// exactly how many more men close it. Naval has no such gate (a vessel is
	// deployable once built, size always 1) so these fields are omitted for it
	// rather than reporting a meaningless "men_needed".
	if cat == unit.CategoryLand {
		resp["training_started"] = trainingStarted
		if !trainingStarted {
			resp["men_needed"] = menNeeded
		}
	}
	if upkeepWarning != "" {
		resp["upkeep_warning"] = upkeepWarning
	}
	if cat == unit.CategoryNaval {
		work, err := economy.LoadShipyardWork(r.Context(), h.pool, unitIDs[len(unitIDs)-1])
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not read shipyard work")
			return
		}
		resp["complete_at"] = work.CompleteAt
		resp["shipyard_waiting"] = work.Waiting
		resp["shipyard_work_remaining"] = work.Remaining
	}

	writeJSON(w, http.StatusCreated, resp)
}

// settlementUpkeepCapacity loads this settlement's current grain/silver
// production rate and the upkeep it already pays, returning the net-per-day
// surplus/deficit after upkeep. Thin DB-hitting wrapper around
// settlementUpkeepDrain + upkeepNetPerDay for callers (like Recruit) that
// don't already have the rate/drain values loaded from a broader
// province-status query — so the POST's warning and the GET's `sustainable`
// can never disagree about the same city.
func settlementUpkeepCapacity(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) (grainNetPerDay, silverNetPerDay float64, err error) {
	var grainRate, silverRate float64
	var population int
	if err = pool.QueryRow(ctx,
		`SELECT
		    COALESCE((SELECT SUM(rate) FROM settlement_goods WHERE settlement_id = $1 AND good_key=ANY($2)),0),
		    COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'), 0),
		    (SELECT population FROM settlements WHERE id = $1)`,
		settlementID, economy.FoodGoods,
	).Scan(&grainRate, &silverRate, &population); err != nil {
		return 0, 0, err
	}
	up, _, upErr := settlementUpkeepDrain(ctx, pool, settlementID)
	if upErr != nil {
		return 0, 0, upErr
	}
	laborPool := population
	if laborPool < 0 {
		laborPool = 0
	}
	// grainRate is raw production since D1 — see the identical note at the
	// GetStatus call site above. Net the population's own food out via
	// economy.GrainBalance (D6) before subtracting the army's upkeep, so this
	// figure (Recruit's own sustainability gate) doesn't silently start
	// ignoring the population's food the moment D1 landed.
	_, grainNetOfPopulation := economy.GrainBalance(grainRate, laborPool)
	grainNetPerDay, silverNetPerDay = upkeepNetPerDay(grainNetOfPopulation, silverRate, up)
	return grainNetPerDay, silverNetPerDay, nil
}

// Goods handles GET /worlds/:worldID/provinces/:provinceID/goods.
// Returns the settlement's goods inventory with lazy-eval amounts and local prices.
// placedCitizensByGood collapses a settlement's PlacementCounts into citizens
// employed per good — one placed gubbe = 100 citizens (Temenos_varutaxonomi_sol
// §1.1). This is the honest post-P4 replacement for the pre-P4 settlement_labor
// weights the goods surface used to read: production is driven by
// settlement_placement, including temple service; cult remains absent from the goods inventory.
func placedCitizensByGood(pc economy.PlacementCounts) map[string]int {
	out := make(map[string]int)
	for _, byGood := range pc.Hex {
		for good, n := range byGood {
			out[good] += n * 100
		}
	}
	for _, byGood := range pc.Building {
		for good, n := range byGood {
			out[good] += n * 100
		}
	}
	return out
}

func (h *ProvinceHandler) Goods(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}

	settlementID, err := resolveSettlementID(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Part B: labor_pool = population. Soldiers are extracted from population at recruit time.
	var population int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT population FROM settlements WHERE id = $1`, settlementID,
	).Scan(&population)
	laborPool := population
	if laborPool < 0 {
		laborPool = 0
	}

	// Load base_potential per good from production_rules using catchment tiles
	// (same logic as RecomputeProduction — the productive ring around the
	// settlement's own hex, hexgrid.CatchmentRadius, P1; the settlement's own
	// hex itself is excluded — not a production tile, economy/catchment.go).
	var provQ, provR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT prov.map_q, prov.map_r FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id WHERE s.id = $1`,
		settlementID,
	).Scan(&provQ, &provR)
	catchQ, catchR := hexgrid.QRArrays(hexgrid.Ring(hexgrid.Coord{Q: provQ, R: provR}, hexgrid.CatchmentRadius))
	baseRows, _ := h.pool.Query(r.Context(),
		`SELECT pr.good_key, SUM(pr.rate_per_tick) AS base_potential,
		        bool_or(pr.building_type IS NULL) AS has_field_path
		 FROM settlements s
		 JOIN unnest($2::int[], $3::int[]) AS catchment(q, r) ON true
		 JOIN map_tiles mt ON mt.world_id = s.world_id AND mt.q = catchment.q AND mt.r = catchment.r
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford') OR pr.terrain_type = mt.terrain)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit,false))
		          OR (pr.requires_deposit = 'incense' AND COALESCE(mt.incense_deposit,false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE s.id = $1
		 GROUP BY pr.good_key`,
		settlementID, catchQ, catchR,
	)
	basePotential := make(map[string]float64)
	hasFieldPath := make(map[string]bool)
	if baseRows != nil {
		for baseRows.Next() {
			var k string
			var v float64
			var field bool
			_ = baseRows.Scan(&k, &v, &field)
			basePotential[k] = v
			hasFieldPath[k] = field
		}
		baseRows.Close()
	}

	// Workplace levels per good — displayed as-is (informational: "how many
	// levels of building"). Actual employment capacity is now driven by
	// economy.WorkplaceSlots' absolute headcount (P2), loaded separately below.
	// Without either of these on the goods surface, over-allocating is silent: a
	// Wanax cannot tell "producing flat out from a level-1 harbour" from "half my
	// fishermen have no boat to crew". (Playtest 2026-07-23, Deiphobos:
	// "ingenting säger om detta är mättat".)
	// DISTINCT is on (good_key, b.id), not (good_key, building_type, level) —
	// same fix as db.go's loadLaborCapacities: a hex-bound type can now have
	// several same-level rows (one per hex), which the old DISTINCT would
	// have collapsed into one, undercounting this informational level sum.
	workplaceLevels := make(map[string]int)
	lvlRows, _ := h.pool.Query(r.Context(),
		`SELECT good_key, SUM(level)::int FROM (
		     SELECT DISTINCT pr.good_key, b.id, b.level
		     FROM production_rules pr
		     JOIN buildings b ON b.settlement_id = $1 AND b.building_type = pr.building_type
		 ) t GROUP BY good_key`,
		settlementID,
	)
	if lvlRows != nil {
		for lvlRows.Next() {
			var k string
			var lvl int
			_ = lvlRows.Scan(&k, &lvl)
			workplaceLevels[k] = lvl
		}
		lvlRows.Close()
	}
	workplaceSlots, _ := economy.LoadWorkplaceSlots(r.Context(), h.pool, settlementID)
	hexSlots, _ := economy.LoadHexCapacity(r.Context(), h.pool, settlementID)

	rows, err := h.pool.Query(r.Context(),
		`SELECT sg.good_key, settled(sg.amount, sg.rate, sg.calc_tick), sg.rate, sg.cap,
		        g.labor_value, g.name, g.tier, g.category, g.weight > 0
		 FROM settlement_goods sg
		 JOIN goods g ON g.key = sg.good_key
		 WHERE sg.settlement_id = $1
		 ORDER BY g.category, sg.good_key`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load goods")
		return
	}
	defer rows.Close()

	// Post-P4 truth: worker headcounts come from settlement_placement (the source
	// that actually drives production), NOT the pre-P4 settlement_labor weights —
	// those are inert dead writes for every good except cult, so reading them here
	// made every non-cult Workers count a lie (stone agent, 2026-08-24). One
	// placed gubbe = 100 citizens.
	placed, _ := economy.LoadPlacementCounts(r.Context(), h.pool, settlementID)
	employedByGood := placedCitizensByGood(placed)

	// marginal_yield (P4-arvet, replaces the old catchment-aggregate figure):
	// "what does my NEXT gubbe on this good give" — the same shared formula
	// PlacementOptions itemises per hex/building (economy.MarginalYieldForSlot),
	// aggregated here to the best available slot per good so /goods keeps its
	// one-row-per-good shape. Never a second formula.
	hexOptionsForYield, _ := economy.LoadHexProductionOptions(r.Context(), h.pool, settlementID, nil)
	buildingOptionsForYield, _ := economy.LoadBuildingProductionOptions(r.Context(), h.pool, settlementID)
	// Hexägarskap (megaron_plan_hexagarskap_och_stadsavstand.md §2): whether a
	// hex slot is "available" for the next gubbe must be judged against every
	// settlement's occupancy, not just this one's — see the identical note in
	// PlacementOptions (settlement_placement.go). A no-op today (no overlap
	// exists yet). employedByGood/idleCitizens above stay on the ORIGINAL
	// settlement-scoped `placed` — this is only about "is there room left",
	// never about whose citizens they are.
	hexCoordsForYield := make([]hexgrid.Coord, len(hexOptionsForYield))
	for i, opt := range hexOptionsForYield {
		hexCoordsForYield[i] = opt.Coord
	}
	globalHexOccupancyForYield, _ := economy.GlobalHexOccupancy(r.Context(), h.pool, worldID, hexCoordsForYield)
	// §2b (Timothy 2026-09-05): a hex held by ANOTHER settlement offers no
	// good at all — mirrors the identical adjustment in PlacementOptions
	// (settlement_placement.go) so /goods never reports a "next gubbe" yield
	// for a hex the write path would reject outright. Delad catchment: a hex
	// the placer may TAKE is not full (economy.ApplyHexHolds, the same helper
	// PlacementOptions uses — one rule, not two).
	var yieldOwner uuid.UUID
	_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'::uuid) FROM settlements WHERE id = $1`, settlementID).Scan(&yieldOwner)
	holdsForYield, _ := economy.LoadHexHolds(r.Context(), h.pool, worldID, settlementID, yieldOwner, hexCoordsForYield)
	globalHexOccupancyForYield = economy.ApplyHexHolds(hexOptionsForYield, holdsForYield, globalHexOccupancyForYield)
	marginalPlaced := economy.PlacementCounts{Hex: globalHexOccupancyForYield, Building: placed.Building}
	marginalYields := economy.MarginalYieldPerGood(hexOptionsForYield, buildingOptionsForYield, marginalPlaced)

	// Every placement, including temple service, consumes the same workforce.
	idleCitizens := laborPool - placed.Total*100
	if idleCitizens < 0 {
		idleCitizens = 0
	}

	type goodRow struct {
		Shippable     bool     `json:"shippable"`
		Key           string   `json:"key"`
		Name          string   `json:"name"`
		Tier          string   `json:"tier"`
		Category      string   `json:"category"`
		Amount        float64  `json:"amount"`
		Rate          float64  `json:"rate_per_tick"`
		Cap           float64  `json:"cap"`
		LaborValue    *float64 `json:"labor_value"`
		Citizens      int      `json:"citizens"`
		Percent       float64  `json:"percent"`
		MarginalYield float64  `json:"marginal_yield"`
		Producible    bool     `json:"producible"`
		LaborPool     int      `json:"labor_pool"`
		IdleCitizens  int      `json:"idle_citizens"`
		// Workplace capacity (2026-07-23). CapacityPercent is the share of the city
		// this good's fields + buildings can actually employ; EmployedCitizens is what
		// is really working it; UnservedCitizens is the allocation that has no
		// workplace to serve at and therefore produces nothing. Without these three,
		// over-allocating is completely silent — the goods table shows a rate and no
		// hint that half your fishermen have no boat to crew.
		CapacityPercent  float64 `json:"labor_capacity_percent"`
		EmployedCitizens int     `json:"employed_citizens"`
		UnservedCitizens int     `json:"unserved_citizens"`
		WorkplaceLevel   int     `json:"workplace_level"`
		WorkplaceSlots   int     `json:"workplace_slots"`
		HexSlots         int     `json:"hex_slots"`
	}
	var result []goodRow
	for rows.Next() {
		var key, name, tier, category string
		var current, rate, capV float64
		var laborValue *float64
		var shippable bool
		if err := rows.Scan(&key, &current, &rate, &capV, &laborValue, &name, &tier, &category, &shippable); err != nil {
			continue
		}
		if current < 0 {
			current = 0
		}
		if current > capV {
			current = capV
		}
		bp := basePotential[key]
		capacity := economy.LaborCapacity(key, hasFieldPath[key], hexSlots[key], workplaceSlots[key], laborPool)
		var employed, unserved int
		employed = employedByGood[key]
		unserved = 0

		percent := 0.0
		if laborPool > 0 {
			percent = float64(employed) / float64(laborPool) * 100.0
		}
		result = append(result, goodRow{
			Key:           key,
			Shippable:     shippable,
			Name:          name,
			Tier:          tier,
			Category:      category,
			Amount:        current,
			Rate:          rate,
			Cap:           capV,
			LaborValue:    laborValue,
			Citizens:      employed,
			Percent:       percent,
			MarginalYield: marginalYields[key],
			Producible:    bp > 0,
			LaborPool:     laborPool,
			IdleCitizens:  idleCitizens,

			CapacityPercent:  capacity * 100.0,
			EmployedCitizens: employed,
			UnservedCitizens: unserved,
			WorkplaceLevel:   workplaceLevels[key],
			WorkplaceSlots:   workplaceSlots[key],
			HexSlots:         hexSlots[key],
		})
	}
	if result == nil {
		result = []goodRow{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Ticklog handles GET /worlds/:worldID/provinces/:provinceID/ticklog?last=N&order=asc.
// Per-city tick-journal (temenos_sitos.md Fas 2): for each tick in
// [current−N+1, current] it derives production/consumption flows from the
// settlement_goods rates (lazy, per-tick) and buckets the discrete events
// (SitosTransaction, TradeDelivery, BuildComplete, …) stamped with that tick.
// Newest-first by default; ?order=asc for chronological.
func (h *ProvinceHandler) Ticklog(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	sett, err := loadSettlementByProvince(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no settlement here")
		return
	}

	last := 10
	if v := r.URL.Query().Get("last"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			last = n
		}
	}
	if last < 1 {
		last = 1
	}
	if last > 200 {
		last = 200
	}
	ascOrder := r.URL.Query().Get("order") == "asc"

	var currentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&currentTick)
	fromTick := currentTick - last + 1
	if fromTick < 0 {
		fromTick = 0
	}

	// Derive per-tick flows from current settlement_goods rates (per-tick).
	// Rate is assumed constant across the window (true between RecomputeProduction
	// calls). Positive rate = production; negative = consumption (shown positive).
	//
	// grain is a special case: since Utfodringsordningen D1
	// (megaron_plan_utfodringsordningen.md, 2026-08-26) its rate is RAW, un-netted
	// production — the population's food is debited once a day from STOCK by
	// FoodTick, not folded into this rate. Filing it through the same
	// positive/negative-rate loop as every other good would show the full gross
	// rate as `production` and nothing at all under `consumption`, hiding the
	// population's food demand entirely. economy.GrainBalance (D6, the one
	// shared reader every surface uses instead of re-deriving
	// laborPool × GrainConsumptionPerCitizenPerTick itself) supplies it here.
	production := map[string]float64{}
	consumption := map[string]float64{}
	var grainRate float64
	grainRateKnown := false
	if grows, gerr := h.pool.Query(r.Context(),
		`SELECT good_key, rate FROM settlement_goods WHERE settlement_id = $1`, sett.ID,
	); gerr == nil {
		for grows.Next() {
			var k string
			var rt float64
			if grows.Scan(&k, &rt) == nil {
				if k == "grain" {
					grainRate = rt
					grainRateKnown = true
					continue
				}
				if rt > 0 {
					production[k] = rt
				} else if rt < 0 {
					consumption[k] = -rt
				}
			}
		}
		grows.Close()
	}
	if grainRateKnown {
		grainLaborPool := sett.Population
		if grainLaborPool < 0 {
			grainLaborPool = 0
		}
		grainConsumRate, _ := economy.GrainBalance(grainRate, grainLaborPool)
		production["grain"] = grainRate // already raw production since D1 — no add-back needed
		consumption["grain"] = grainConsumRate
	}

	// Bucket discrete events by tick.
	type tickEvent struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	eventsByTick := map[int][]tickEvent{}
	if erows, eerr := h.pool.Query(r.Context(),
		`SELECT world_tick, event_type, payload FROM events
		 WHERE stream_id = $1 AND world_tick BETWEEN $2 AND $3
		 ORDER BY world_tick, id`,
		sett.ID, fromTick, currentTick,
	); eerr == nil {
		for erows.Next() {
			var tk int
			var et string
			var pl json.RawMessage
			if erows.Scan(&tk, &et, &pl) == nil {
				eventsByTick[tk] = append(eventsByTick[tk], tickEvent{Type: et, Payload: pl})
			}
		}
		erows.Close()
	}

	type tickRow struct {
		Tick        int                `json:"tick"`
		Production  map[string]float64 `json:"production"`
		Consumption map[string]float64 `json:"consumption"`
		Events      []tickEvent        `json:"events"`
	}
	ticks := make([]tickRow, 0, last)
	for tk := fromTick; tk <= currentTick; tk++ {
		evs := eventsByTick[tk]
		if evs == nil {
			evs = []tickEvent{}
		}
		ticks = append(ticks, tickRow{
			Tick: tk, Production: production, Consumption: consumption,
			Events: evs,
		})
	}
	if !ascOrder {
		for i, j := 0, len(ticks)-1; i < j; i, j = i+1, j-1 {
			ticks[i], ticks[j] = ticks[j], ticks[i]
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"settlement_id": sett.ID,
		"current_tick":  currentTick,
		"ticks":         ticks,
	})
}

// settlementHasHarbour reports whether a settlement has built a harbour —
// half of the "coastal OR harbour" naval-eligibility gate unit.go's
// embark/disembark checks already use (the other half, provinces.coastal, is
// read inline by callers since it's already in their settlement/province
// JOIN). A settlement's own coastal flag is checked by the caller; this only
// covers the building fallback for an inland settlement that dug a harbour.
func settlementHasHarbour(ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, settlementID uuid.UUID) (bool, error) {
	var hasHarbour bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'harbour')`,
		settlementID,
	).Scan(&hasHarbour)
	return hasHarbour, err
}

// Trade handles POST /worlds/:worldID/provinces/:provinceID/trade.
// Body: { "destination_id": "<settlement UUID>", "good_key": "grain", "quantity": 10.0 }
func (h *ProvinceHandler) Trade(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		DestinationID uuid.UUID           `json:"destination_id"`
		GoodKey       string              `json:"good_key"`
		Quantity      float64             `json:"quantity"`
		Items         []economy.CargoItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	listRequest := req.Items != nil
	if listRequest && (req.GoodKey != "" || req.Quantity != 0) {
		writeError(w, 400, "use items or good_key/quantity, not both")
		return
	}
	if !listRequest {
		req.Items = []economy.CargoItem{{GoodKey: req.GoodKey, Quantity: req.Quantity}}
	}
	items, total, err := economy.NormalizeCargo(req.Items)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	req.Items = items
	// The route row keeps the first item for legacy readers; the complete
	// authoritative manifest lives in transport_goods and the V2 timer.
	req.GoodKey, req.Quantity = items[0].GoodKey, items[0].Quantity

	// Find origin settlement and verify ownership.
	var originID uuid.UUID
	var originName string
	var originQ, originR int
	var originCoastal bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT s.id, s.name, prov.map_q, prov.map_r, COALESCE(prov.coastal, false)
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.province_id = $1 AND s.world_id = $2 AND s.owner_id = $3`,
		provinceID, worldID, playerID,
	).Scan(&originID, &originName, &originQ, &originR, &originCoastal)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// A foreign transfer is a gift, gated exactly like letters.
	var destQ, destR int
	var destCoastal bool
	var destOwnerID *uuid.UUID
	var destName, destOwnerName, destState, destTerrain string
	err = h.pool.QueryRow(r.Context(), `SELECT prov.map_q,prov.map_r,COALESCE(prov.coastal,false),s.owner_id,s.name,COALESCE(NULLIF(pl.wanax_name,''),pl.username,''),s.state,prov.terrain_type FROM settlements s JOIN provinces prov ON prov.id=s.province_id LEFT JOIN players pl ON pl.id=s.owner_id WHERE s.id=$1 AND s.world_id=$2`, req.DestinationID, worldID).Scan(&destQ, &destR, &destCoastal, &destOwnerID, &destName, &destOwnerName, &destState, &destTerrain)
	if err != nil {
		writeError(w, 404, "destination settlement not found")
		return
	}
	if destOwnerID == nil {
		writeError(w, 403, "destination has no Wanax to receive goods")
		return
	}
	isGift := *destOwnerID != playerID
	if isGift {
		if destState != "active" {
			writeError(w, 403, "destination settlement is not active")
			return
		}
		if !knownToPlayer(loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now()), loadRememberedTiles(r.Context(), h.pool, worldID, playerID), province.MapPosition{Q: destQ, R: destR}, destTerrain) {
			writeError(w, 403, "this city is not known on your map — discover or contact it before sending a gift")
			return
		}
	}

	for _, item := range items {
		_, shippable, err := economy.IsShippableGood(r.Context(), h.pool, item.GoodKey)
		if err != nil {
			writeError(w, 400, "unknown good")
			return
		}
		if !shippable {
			writeError(w, 400, "cannot transfer cult — cult is produced by temple labor and converted to kharis in place, it cannot be shipped")
			return
		}
	}

	// A navigable coastal route still needs a real ship; otherwise use a real land path.
	originHarboured, err := settlementHasHarbour(r.Context(), h.pool, originID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check origin harbour")
		return
	}
	destHarboured, err := settlementHasHarbour(r.Context(), h.pool, req.DestinationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check destination harbour")
		return
	}
	category, _, err := province.ResolveTradeRoute(r.Context(), h.pool, worldID,
		originCoastal || originHarboured, destCoastal || destHarboured,
		province.MapPosition{Q: originQ, R: originR},
		province.MapPosition{Q: destQ, R: destR},
	)
	if err != nil {
		writeTradeJourneyError(w, err)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Revalidate both endpoints under row locks before any debit or ship binding.
	endpointsLocked := false
	if isGift {
		endpointsLocked = lockGiftSettlements(r.Context(), tx, worldID, playerID, *destOwnerID, originID, req.DestinationID)
	} else {
		endpointsLocked = lockOwnedTradeSettlements(r.Context(), tx, worldID, playerID, originID, req.DestinationID)
	}
	if !endpointsLocked {
		writeError(w, http.StatusForbidden, "settlement ownership changed")
		return
	}
	// Sjöhandel kräver skepp (megaron_plan_sjohandel_kraver_skepp.md R3): a
	// resolved "naval" route only means a sea lane connects the two shores —
	// it says nothing about whether the Wanax actually owns a free hull to
	// sail it. Bound inside this same transaction (FindFreeShip's row lock
	// held through to commit) so two concurrent transfers can never bind the
	// same ship.
	var shipID *uuid.UUID
	var shipName, shipType string
	var cargoCapacity *float64
	if category == "naval" {
		ship, found, ferr := transport.FindFreeShip(r.Context(), tx, worldID, playerID, originID)
		if ferr != nil {
			writeError(w, http.StatusInternalServerError, "could not look for a free ship")
			return
		}
		if found {
			if total > ship.Capacity {
				writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf(
					"%s cargo %g / %g capacity — remove %g goods",
					unit.DisplayName(ship.Type), total, ship.Capacity, total-ship.Capacity))
				return
			}
			if err := transport.BindShip(r.Context(), tx, ship.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not bind ship")
				return
			}
			shipType = ship.Type
			capacity := ship.Capacity
			cargoCapacity = &capacity
			sid := ship.ID
			shipID = &sid
			shipOwnName := ""
			if ship.Name != nil {
				shipOwnName = *ship.Name
			}
			shipName = unit.ShipDisplayName(ship.Type, shipOwnName, originName).DisplayName
		} else {
			// No free galley/merchantman — fall back to a real land route if
			// one actually exists (a resolved-naval pair may have NO land
			// connection at all, e.g. two islands); otherwise there is
			// genuinely nothing to send this on.
			category = "land"
			if _, ferr := province.PlanTradeJourney(r.Context(), tx, worldID,
				province.MapPosition{Q: originQ, R: originR}, province.MapPosition{Q: destQ, R: destR}, category); ferr != nil {
				if errors.Is(ferr, province.ErrNoTradePath) {
					writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("no free galley or merchantman in %s to carry goods by sea and no land route — build one at a shipyard or wait for one to return", originName))
				} else {
					writeTradeJourneyError(w, ferr)
				}
				return
			}
		}
	}

	journey, err := province.PlanTradeJourney(r.Context(), tx, worldID,
		province.MapPosition{Q: originQ, R: originR}, province.MapPosition{Q: destQ, R: destR}, category)
	if err != nil {
		writeTradeJourneyError(w, err)
		return
	}
	var tradeCurrentTick int
	if err := tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&tradeCurrentTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read current game day")
		return
	}
	if err := transport.BlessDeparture(r.Context(), tx, worldID, playerID, originID, &journey, true); err != nil {
		writeError(w, 500, "could not prepare departure")
		return
	}
	tradeDispatchAt := h.clk.Now()
	if err := economy.ClockTradeJourney(r.Context(), tx, worldID, &journey, tradeDispatchAt); err != nil {
		writeError(w, 500, "could not anchor cargo journey")
		return
	}
	tradeCurrentTick = journey.Clock.StartTick
	tradeTravelTicks := journey.TravelTicks
	arrivesAt := journey.Route(0, 0).WallAt(int64(journey.Clock.EndTick)*1000 + journey.Clock.OffsetMilli)

	// Deduct from origin — silver is now a normal good in settlement_goods.
	manifest := transport.Manifest{}
	for _, item := range items {
		deductTag, err := province.DeductGood(r.Context(), tx, originID, item.GoodKey, item.Quantity)
		if err != nil {
			writeError(w, 500, "could not deduct goods")
			return
		}
		if deductTag.RowsAffected() == 0 {
			writeError(w, 422, "insufficient goods: "+item.GoodKey)
			return
		}
		manifest[item.GoodKey] = item.Quantity
	}

	var routeID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO trade_routes (world_id, origin_id, destination_id, good_key, quantity, arrives_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		worldID, originID, req.DestinationID, req.GoodKey, req.Quantity, arrivesAt,
	).Scan(&routeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create trade route")
		return
	}

	// Physical caravan: internal transfer is logistics without consent, but it is
	// still a mover on the map (CLAUDE.md trade-lagret punkt 3) — CreateShadow, not
	// Dispatch, because the arrival is already driven by ScheduledTradeDelivery
	// below; Dispatch would additionally schedule ScheduledTransportArrival and
	// double-credit the destination.
	transportKind := "transfer"
	if isGift {
		transportKind = "gift"
	}
	transportID, err := transport.CreateShadow(r.Context(), tx, transport.DispatchParams{
		WorldID:      worldID,
		OwnerID:      playerID,
		Kind:         transportKind,
		OriginID:     originID,
		DestID:       req.DestinationID,
		Category:     category,
		OriginQ:      originQ,
		OriginR:      originR,
		DestQ:        destQ,
		DestR:        destR,
		DepartsAt:    tradeDispatchAt,
		ArrivesAt:    arrivesAt,
		DueTick:      tradeCurrentTick + tradeTravelTicks,
		DepartedTick: tradeCurrentTick, Journey: &journey,
		Manifest:      manifest,
		Interceptable: true,
		ShipUnitID:    shipID,
	})
	if err != nil {
		writeTransportDispatchError(w, err, "could not dispatch transfer caravan")
		return
	}

	if _, err := tx.Exec(r.Context(), `UPDATE trade_routes SET transport_id=$2 WHERE id=$1`, routeID, transportID); err != nil {
		writeError(w, 500, "could not link shipment manifest")
		return
	}

	var dispatchedGift *events.Event
	if listRequest {
		var senderName string
		if err := tx.QueryRow(r.Context(), `SELECT COALESCE(NULLIF(wanax_name,''),username) FROM players WHERE id=$1`, playerID).Scan(&senderName); err != nil {
			writeError(w, 500, "could not read sender")
			return
		}
		shipment := economy.ShipmentV2{TransportID: transportID, RouteID: routeID, SenderID: playerID, RecipientID: *destOwnerID, OriginID: originID, DestinationID: req.DestinationID, OriginName: originName, DestinationName: destName, SenderName: senderName, RecipientName: destOwnerName, Items: items, Kind: transportKind}
		if err := economy.EnqueueFreightTx(r.Context(), tx, h.scheduler, worldID, events.ScheduledShipmentDeliveryV2, shipment, tradeCurrentTick+tradeTravelTicks); err != nil {
			writeError(w, 500, "could not schedule shipment")
			return
		}
		if isGift {
			dispatchedGift, err = h.eventStore.AppendTx(r.Context(), tx, originID, events.StreamProvince, "GiftDispatchedV2", shipment, worldID, nil)
			if err != nil {
				writeError(w, 500, "could not record gift dispatch")
				return
			}
		}
	} else if isGift {
		var senderName string
		if err := tx.QueryRow(r.Context(), `SELECT COALESCE(NULLIF(wanax_name,''),username) FROM players WHERE id=$1`, playerID).Scan(&senderName); err != nil {
			writeError(w, 500, "could not read sender")
			return
		}
		shipment := economy.GiftShipment{TransportID: transportID, RouteID: routeID, SenderID: playerID, RecipientID: *destOwnerID, OriginID: originID, DestinationID: req.DestinationID, OriginName: originName, DestinationName: destName, SenderName: senderName, RecipientName: destOwnerName, GoodKey: req.GoodKey, Quantity: req.Quantity}
		if err := economy.EnqueueFreightTx(r.Context(), tx, h.scheduler, worldID, events.ScheduledGiftDelivery, shipment, tradeCurrentTick+tradeTravelTicks); err != nil {
			writeError(w, 500, "could not schedule gift")
			return
		}
		dispatchedGift, err = h.eventStore.AppendTx(r.Context(), tx, originID, events.StreamProvince, "GiftDispatched", shipment, worldID, nil)
		if err != nil {
			writeError(w, 500, "could not record gift dispatch")
			return
		}
	} else {
		// Internal transfer: no loss, no gain — delivered quantity equals what was sent.
		// Enqueue delivery within the same transaction — atomic with the deduction.
		if err := economy.EnqueueFreightTx(r.Context(), tx, h.scheduler, worldID, events.ScheduledTradeDelivery,
			map[string]any{
				"trade_route_id":     routeID,
				"destination_id":     req.DestinationID,
				"good_key":           req.GoodKey,
				"quantity":           req.Quantity,
				"delivered_quantity": req.Quantity,
				"transport_id":       transportID.String(),
			}, tradeCurrentTick+tradeTravelTicks); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule delivery")
			return
		}

	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	if dispatchedGift != nil {
		h.eventStore.RecordCommitted(r.Context(), dispatchedGift)
	}
	resp := map[string]any{
		"route_id":              routeID,
		"kind":                  transportKind,
		"arrives_at":            arrivesAt,
		"distance":              journey.Distance,
		"travel_min":            float64(tradeTravelTicks) * 60,
		"travel_ticks":          tradeTravelTicks,
		"departure_tick":        tradeCurrentTick,
		"arrival_tick":          tradeCurrentTick + tradeTravelTicks,
		"route_version":         2,
		"dispatch_offset_milli": journey.Clock.OffsetMilli,
		"transport_id":          transportID,
		"delivered_qty":         req.Quantity,
		"category":              category,
	}
	if isGift {
		delete(resp, "delivered_qty")
		resp["sent_qty"] = req.Quantity
	}
	resp["items"] = items
	resp["cargo_quantity"] = total
	resp["cargo_capacity"] = cargoCapacity
	if listRequest {
		delete(resp, "delivered_qty")
		delete(resp, "sent_qty")
	}
	if shipID != nil {
		resp["ship_type"] = shipType
		resp["ship_id"] = *shipID
		resp["ship_name"] = shipName
	}
	writeJSON(w, http.StatusCreated, resp)
}

// Marches handles GET /worlds/:worldID/provinces/:provinceID/marches.
// Returns the last 20 marches from this province (owner only) including combat reports.
func (h *ProvinceHandler) Marches(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var ownerID *uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&ownerID)
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, target_id, intent, infantry, chariot, ship, elite_infantry,
		        war_galley, merchantman, resolved, arrives_at, combat_report,
		        origin_id = $1 AS outgoing
		 FROM marching_armies
		 WHERE (origin_id = $1 OR (target_id = $1 AND resolved = true))
		   AND world_id = $2
		 ORDER BY arrives_at DESC LIMIT 20`,
		provinceID, worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load marches")
		return
	}
	defer rows.Close()

	type marchItem struct {
		ID            uuid.UUID `json:"id"`
		TargetID      uuid.UUID `json:"target_id"`
		Intent        string    `json:"intent"`
		Spearman      int       `json:"spearman"`
		WarChariot    int       `json:"war_chariot"`
		Ship          int       `json:"ship"` // galley
		EliteInfantry int       `json:"elite_infantry"`
		WarGalley     int       `json:"war_galley"`
		Merchantman   int       `json:"merchantman"`
		Resolved      bool      `json:"resolved"`
		ArrivesAt     time.Time `json:"arrives_at"`
		CombatReport  *string   `json:"combat_report,omitempty"`
		Outgoing      bool      `json:"outgoing"`
	}
	var result []marchItem
	for rows.Next() {
		var m marchItem
		if err := rows.Scan(&m.ID, &m.TargetID, &m.Intent,
			&m.Spearman, &m.WarChariot, &m.Ship, &m.EliteInfantry,
			&m.WarGalley, &m.Merchantman, &m.Resolved, &m.ArrivesAt, &m.CombatReport, &m.Outgoing); err == nil {
			result = append(result, m)
		}
	}
	if result == nil {
		result = []marchItem{}
	}
	writeJSON(w, http.StatusOK, result)
}

// TradeRoutes handles GET /worlds/:worldID/provinces/:provinceID/trade.
// Returns active (unresolved) trade routes sent from this province.
func (h *ProvinceHandler) TradeRoutes(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var ownerID *uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&ownerID)
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT tr.id, ds.name, tr.good_key, tr.quantity, tr.departs_at, tr.arrives_at,
		        op.map_q, op.map_r, dp.map_q, dp.map_r
		 FROM trade_routes tr
		 JOIN settlements ds ON ds.id = tr.destination_id
		 JOIN settlements os ON os.id = tr.origin_id
		 JOIN provinces op ON op.id = os.province_id
		 JOIN provinces dp ON dp.id = ds.province_id
		 WHERE os.province_id = $1 AND tr.world_id = $2 AND tr.resolved = false
		 ORDER BY tr.arrives_at ASC`,
		provinceID, worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load trade routes")
		return
	}
	defer rows.Close()

	type routeItem struct {
		Items        []economy.CargoItem `json:"items"`
		ID           uuid.UUID           `json:"id"`
		PeerName     string              `json:"peer_name"` // destination for outgoing, origin for incoming
		GoodKey      string              `json:"good_key"`
		Quantity     float64             `json:"quantity"`
		DeliveredQty float64             `json:"delivered_qty"`
		Direction    string              `json:"direction"` // "outgoing" | "incoming"
		DepartsAt    time.Time           `json:"departs_at"`
		ArrivesAt    time.Time           `json:"arrives_at"`
	}
	var result []routeItem
	for rows.Next() {
		var ri routeItem
		var oq, or_, dq, dr int
		if err := rows.Scan(&ri.ID, &ri.PeerName, &ri.GoodKey, &ri.Quantity, &ri.DepartsAt, &ri.ArrivesAt,
			&oq, &or_, &dq, &dr); err == nil {
			ri.DeliveredQty = ri.Quantity
			ri.Direction = "outgoing"
			result = append(result, ri)
		}
	}
	rows.Close()

	// Also load incoming routes (addressed to this settlement).
	var settlementID uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&settlementID)

	if settlementID != (uuid.UUID{}) {
		inRows, err := h.pool.Query(r.Context(),
			`SELECT tr.id, os.name, tr.good_key, tr.quantity, tr.departs_at, tr.arrives_at,
			        op.map_q, op.map_r, dp.map_q, dp.map_r
			 FROM trade_routes tr
			 JOIN settlements os ON os.id = tr.origin_id
			 JOIN settlements ds ON ds.id = tr.destination_id
			 JOIN provinces op ON op.id = os.province_id
			 JOIN provinces dp ON dp.id = ds.province_id
			 WHERE tr.destination_id = $1 AND tr.world_id = $2 AND tr.resolved = false
			 ORDER BY tr.arrives_at ASC`,
			settlementID, worldID,
		)
		if err == nil {
			defer inRows.Close()
			for inRows.Next() {
				var ri routeItem
				var oq, or_, dq, dr int
				if err := inRows.Scan(&ri.ID, &ri.PeerName, &ri.GoodKey, &ri.Quantity, &ri.DepartsAt, &ri.ArrivesAt,
					&oq, &or_, &dq, &dr); err == nil {
					ri.DeliveredQty = ri.Quantity
					ri.Direction = "incoming"
					result = append(result, ri)
				}
			}
		}
	}

	if result == nil {
		result = []routeItem{}
	}
	for i := range result {
		var raw []byte
		err := h.pool.QueryRow(r.Context(), `SELECT COALESCE((SELECT jsonb_agg(jsonb_build_object('good_key',tg.good_key,'quantity',tg.quantity) ORDER BY tg.good_key) FROM transport_goods tg WHERE tg.transport_id=tr.transport_id),jsonb_build_array(jsonb_build_object('good_key',tr.good_key,'quantity',tr.quantity))) FROM trade_routes tr WHERE tr.id=$1`, result[i].ID).Scan(&raw)
		if err != nil {
			writeError(w, 500, "could not load shipment manifest")
			return
		}
		if err := json.Unmarshal(raw, &result[i].Items); err != nil {
			writeError(w, 500, "could not decode shipment manifest")
			return
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// harvestBlessingStatus reports the same strict expiry gate and tuning as
// economy's production recomputation. Clients translate remaining ticks to days.
func harvestBlessingStatus(until *int, currentTick int) any {
	if until == nil || *until <= currentTick {
		return nil
	}
	return map[string]any{
		"until_tick":      *until,
		"remaining_ticks": *until - currentTick,
		"yield_bonus":     economy.HarvestBlessingYieldBonus,
		"goods":           economy.HarvestBlessedGoods(),
	}
}

// Population food balance is also the growth gate; army drain is a separate
// projection, never a change to FoodTick or growth. Shared by both read APIs.
func foodNetAfterArmy(foodNet, grainUpkeep float64) float64 { return foodNet - grainUpkeep }

type templeRow struct {
	BuildingID uuid.UUID `json:"building_id"`
	GodID      string    `json:"god_id"`
	Name       string    `json:"name"`
	Level      int       `json:"level"`
	Staffed    int       `json:"staffed"`
	Capacity   int       `json:"capacity"`
}

// templeRows names each temple after its god for every surface (ORDER 15).
func templeRows(temples []economy.TempleWorkplace) []templeRow {
	out := make([]templeRow, 0, len(temples))
	for _, t := range temples {
		out = append(out, templeRow{BuildingID: t.BuildingID, GodID: t.GodID, Name: religion.TempleName(t.GodID),
			Level: t.Level, Staffed: t.Staffed, Capacity: t.Capacity})
	}
	return out
}
