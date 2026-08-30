package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/capabilities"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"formatet/megaron/server/internal/unit/shipnames"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	clk        clock.Clock
	sitosCfg   economy.SitosConfig
	eventStore *events.Store // may be nil in tests that don't exercise Recruit's naval path
	hub        *notify.Hub   // nil-guarded; carries player-facing notifications (trade, arrivals, ...)
}

// NewProvinceHandler creates a ProvinceHandler.
func NewProvinceHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, clk clock.Clock, sitosCfg economy.SitosConfig, eventStore *events.Store, hub *notify.Hub) *ProvinceHandler {
	return &ProvinceHandler{pool: pool, scheduler: scheduler, clk: clk, sitosCfg: sitosCfg, eventStore: eventStore, hub: hub}
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
		    CASE WHEN COALESCE(cedar_deposit, false) THEN 'cedar'  END
		 FROM map_tiles mt
		 JOIN unnest($2::int[], $3::int[]) AS catchment(q, r) ON mt.q = catchment.q AND mt.r = catchment.r
		 WHERE mt.world_id = $1
		   AND mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')`,
		worldID, catchQ, catchR,
	)
	if cdrows != nil {
		seen := make(map[string]bool)
		for cdrows.Next() {
			var copper, tin, silver, cedar *string
			_ = cdrows.Scan(&copper, &tin, &silver, &cedar)
			for _, v := range []*string{copper, tin, silver, cedar} {
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
		}
		var buildQueue []buildItem
		qrows, _ := h.pool.Query(r.Context(),
			`SELECT id, building_type, created_at, complete_at FROM build_queue
			 WHERE settlement_id = $1 ORDER BY complete_at`,
			sett.ID,
		)
		if qrows != nil {
			for qrows.Next() {
				var bi buildItem
				_ = qrows.Scan(&bi.ID, &bi.Type, &bi.CreatedAt, &bi.CompleteAt)
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
			Unit     string     `json:"unit"`
			Size     int        `json:"size"`
			Status   string     `json:"status"`
			Category string     `json:"category"`
			ReadyAt  *time.Time `json:"ready_at,omitempty"`
		}
		var trainingUnits []trainingUnit
		if trows, terr := h.pool.Query(r.Context(),
			`SELECT type, size, status, category, build_complete_at
			 FROM units
			 WHERE settlement_id = $1 AND status IN ('forming', 'training')
			 ORDER BY category, status DESC, created_at`,
			sett.ID,
		); terr == nil {
			for trows.Next() {
				var tu trainingUnit
				if trows.Scan(&tu.Unit, &tu.Size, &tu.Status, &tu.Category, &tu.ReadyAt) == nil {
					trainingUnits = append(trainingUnits, tu)
				}
			}
			trows.Close()
		}
		if trainingUnits == nil {
			trainingUnits = []trainingUnit{}
		}

		// Buildings — already completed (agents/clients use this to avoid re-queuing).
		type buildingItem struct {
			Type  string `json:"type"`
			Level int    `json:"level"`
		}
		var buildings []buildingItem
		brows, _ := h.pool.Query(r.Context(),
			`SELECT building_type, level FROM buildings WHERE settlement_id = $1 ORDER BY building_type`,
			sett.ID,
		)
		if brows != nil {
			for brows.Next() {
				var bi buildingItem
				_ = brows.Scan(&bi.Type, &bi.Level)
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
			afford := !atMax && silverStock >= spec.CostSilver
			if afford {
				for goodKey, needed := range spec.Costs {
					if goodsStock[goodKey] < needed {
						afford = false
						break
					}
				}
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
		soldShare := combat.UpkeepSoldShare()
		armyUp, circulatedSilver, upErr := settlementUpkeepDrain(r.Context(), h.pool, sett.ID, soldShare)
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
		netGrainPerDay, netSilverPerDay := upkeepNetPerDay(upkeepGrainNetOfPopulation, upkeepSilverRate, armyUp, circulatedSilver)

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
			// Net is what the city actually loses per tick while the unit garrisons
			// at home: the sold share circulates back (Del C). March it out and the
			// cost is the gross figure again.
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
				for goodKey, needed := range spec.Costs {
					if goodsStock[goodKey] < needed {
						afford = false
						break
					}
				}
			}
			fullSize := 100
			cat := string(unit.CategoryOf(unit.Type(unitType)))
			if cat != "land" {
				fullSize = 1 // naval upkeep is flat, independent of size
			}
			newUnitUp := combat.UnitUpkeep(unitType, cat, fullSize, "garrison")
			// A unit recruited here garrisons here and is paid from here, so both
			// unit columns point at this city and Del C's sold share circulates
			// back the same tick. Judging sustainability on the gross would call a
			// city that can carry the unit unsustainable — a false negative sitting
			// directly on the recruit surface, i.e. on the chain gate.
			newUnitNetSilver := newUnitUp.Silver * (1 - soldShare)
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
		var kharisNetPerDay float64
		var kharisNetKnown, kharisDevotionIdle bool
		if sett.OwnerID != nil {
			if k, kerr := loadPlayerKharis(r.Context(), h.pool, *sett.OwnerID, worldID); kerr == nil {
				kharisNow, kharisRate = k.Amount, k.Rate
				kharisCap, maxTempleLevel = k.Cap, k.MaxTempleLevel
			}
			// Projected daily maintenance net (gain − decay) — the honest answer to
			// "will my kharis rise or fall", which the passive geographic rate alone
			// hides (soak 2026-07-24: a fading L1 Wanax saw "+0.1/dygn"). Read-only.
			if net, has, devSum, devCap, nerr := kharis.ProjectDailyNet(r.Context(), h.pool, *sett.OwnerID, worldID); nerr == nil {
				kharisNetPerDay, kharisNetKnown = net, has
				// Idle temple capacity: the Wanax raised a temple's level but hasn't
				// allocated the cult labor to fill it, so it can't lift kharis. 0.02 =
				// a hair above float noise, well under one 0.15 devotion step.
				kharisDevotionIdle = has && (devCap-devSum) > 0.02
			}
		}

		// Kult-block (PLAN B §3, megaron_kult_legibilitet_plan.md): per temple-city,
		// today's offer requirement vs current oil/wine stock — a read-only mirror of
		// kharis.applyTempleOffering's own query (same gate, same numbers), so "will
		// my kharis climb today" is answerable from `status` without waiting for the
		// daily tick to run. Scoped by owner (not requesting player) to match the
		// kharis/cooldown convention above — spectators see the owner's temples.
		type templeOfferRow struct {
			SettlementID uuid.UUID `json:"settlement_id"`
			Name         string    `json:"name"`
			Oil          float64   `json:"oil"`
			Wine         float64   `json:"wine"`
			OilNeeded    float64   `json:"oil_needed"`
			WineNeeded   float64   `json:"wine_needed"`
			Fed          bool      `json:"fed"`
		}
		var templeOffers []templeOfferRow
		if sett.OwnerID != nil {
			if trows, terr := h.pool.Query(r.Context(),
				`SELECT s.id, s.name,
				    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
				              FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'), 0) AS oil,
				    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
				              FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'), 0) AS wine
				 FROM settlements s
				 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.state NOT IN ('sunk', 'collapsed')
				   AND EXISTS (SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'temple')
				 ORDER BY s.name`,
				*sett.OwnerID, worldID,
			); terr == nil {
				for trows.Next() {
					var t templeOfferRow
					if trows.Scan(&t.SettlementID, &t.Name, &t.Oil, &t.Wine) == nil {
						t.OilNeeded = kharis.OfferOilPerTemple
						t.WineNeeded = kharis.OfferWinePerTemple
						t.Fed = t.Oil >= kharis.OfferOilPerTemple && t.Wine >= kharis.OfferWinePerTemple
						templeOffers = append(templeOffers, t)
					}
				}
				trows.Close()
			}
		}
		if templeOffers == nil {
			templeOffers = []templeOfferRow{}
		}

		// Temple presence — required by the rite handler for any prayer.
		var hasTemple bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'temple')`,
			sett.ID,
		).Scan(&hasTemple)

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
			ID         string             `json:"id"`
			Name       string             `json:"name"`
			God        string             `json:"god"`
			EffectType string             `json:"effect_type"`
			Effect     string             `json:"effect"`
			MinKharis  float64            `json:"min_kharis"`
			Offering   map[string]float64 `json:"offering"`
			Affordable bool               `json:"affordable"`
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
		var devotionWeight, devotionCapacity float64
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE((SELECT weight FROM settlement_labor
			                  WHERE settlement_id = $1 AND good_key = 'cult'), 0),
			        COALESCE((SELECT $2::float8 * GREATEST(1, level) FROM buildings
			                  WHERE settlement_id = $1 AND building_type = 'temple'
			                  ORDER BY level DESC LIMIT 1), 0)`,
			sett.ID, kharis.TempleDevotionPerLevel,
		).Scan(&devotionWeight, &devotionCapacity)
		prayers := []prayerRow{}
		for _, pid := range religion.CulturePrayers[string(sett.CultureID)] {
			spec := religion.PrayerSpecs[pid]
			afford := hasTemple && kharisNow >= spec.MinKharis
			if afford {
				for g, need := range spec.Offering {
					if goodsStock[g] < need {
						afford = false
						break
					}
				}
			}
			// Check cooldown: same query as the Rite handler.
			// Use sett.OwnerID (not request auth) so spectators see the owner's cooldown.
			var cooldownRemainingDays int
			if spec.CooldownTicks > 0 && sett.OwnerID != nil {
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
					}
				}
			}
			prayers = append(prayers, prayerRow{
				ID: spec.ID, Name: spec.Name, God: spec.God, EffectType: spec.EffectType,
				Effect:    spec.Description,
				MinKharis: spec.MinKharis, Offering: spec.Offering, Affordable: afford,
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

		// Sitos granary surface (always visible): what the city has set aside, and
		// — the figure the whole mechanic turns on — how many days of food it is
		// standing on right now. Coverage is what triggers both legs (B1), so
		// showing the reserve without it would show the answer and hide the
		// question.
		var currentTick int
		_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&currentTick)
		granaryPerGood, granaryTotal, granErr := economy.GranaryTotals(r.Context(), h.pool, sett.ID)
		if granErr != nil {
			slog.Error("granary read failed", "err", granErr, "settlement", sett.ID)
		}
		var grainBaseValue, grainAmount, grainRate, grainCap float64
		var grainCalcTick int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT g.base_value, sg.amount, sg.rate, sg.cap, sg.calc_tick
			 FROM settlement_goods sg JOIN goods g ON g.key = sg.good_key
			 WHERE sg.settlement_id = $1 AND sg.good_key = 'grain'`,
			sett.ID,
		).Scan(&grainBaseValue, &grainAmount, &grainRate, &grainCap, &grainCalcTick)
		// Coverage is measured on the whole food basket (B6) — grain first, fish
		// for the remainder, one need (economy.FoodConsumptionSplit). Counting
		// grain alone would call a fish-fed city starving.
		var foodStock, foodRatePerTick float64
		for _, good := range h.sitosCfg.SubsistenceGoods {
			var s, rate float64
			if h.pool.QueryRow(r.Context(),
				`SELECT GREATEST(0, settled(amount, rate, calc_tick)), rate
				 FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`,
				sett.ID, good,
			).Scan(&s, &rate) == nil {
				foodStock += s
				foodRatePerTick += rate
			}
		}
		coverageDays := economy.CoverageDays(foodStock, sett.Population)
		granaryCap := economy.GranaryCap(sett.Population, h.sitosCfg)
		// Coverage is a stock figure, so it says nothing about which way the city
		// is going — and a newly founded city legitimately starts near zero
		// coverage while producing a large surplus. Reported alone it reads as
		// famine for the first days of every city's life (eye-check 2026-08-03:
		// a city with +21 000 grain/day showed "0.1 days, granary empty"). The
		// net food rate is what separates "lean and climbing" from "starving",
		// and the surfaces need both to say either honestly.
		foodNetPerDay := foodRatePerTick

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
		foodGubbarRequired, foodSelfSufficient, foodReqErr := economy.FoodGubbarRequired(r.Context(), h.pool, sett.ID)
		if foodReqErr != nil {
			slog.Error("food gubbar required failed", "err", foodReqErr, "settlement", sett.ID)
		}
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

		// "Senaste tick" summary (Fas 2 point 8): derive prod/cons from the same
		// per-tick rates already in resSnap, and sum this tick's Sitos silver delta
		// from the events log. Summarizes the journal without replacing it.
		lastTickProd := map[string]float64{}
		lastTickCons := map[string]float64{}
		for k, rd := range resSnap {
			if k == "grain" {
				continue // handled below — grain's consumption isn't in its rate (D1)
			}
			if rd.Rate > 0 {
				lastTickProd[k] = rd.Rate
			} else if rd.Rate < 0 {
				lastTickCons[k] = -rd.Rate
			}
		}
		// Grain is the one good whose rate carries no consumption term at all
		// since D1, so the loop above would file a self-sufficient city's whole
		// gross rate under "produced" and leave consumption empty. The keryx
		// one-liner then reported "2 varor produceras, 0 förbrukas" for a city
		// eating 500 grain a tick — the same lie P4 hål 2 removed from
		// `keryx ticklog`, one surface over (found in the acceptance sweep
		// 2026-08-24). Reuses grainProdRate/grainConsumRate derived above, so the
		// two surfaces cannot drift apart again.
		if _, hasGrain := resSnap["grain"]; hasGrain {
			if grainProdRate > 0 {
				lastTickProd["grain"] = grainProdRate
			}
			if grainConsumRate > 0 {
				lastTickCons["grain"] = grainConsumRate
			}
		}
		// DEL A Sitos-delta-itemisering (megaron_ekonomi_legibilitet_plan.md):
		// What the granary did for this settlement this tick. There is no silver
		// leg left to report (B3) — the itemisation is food in and food out.
		// Reads the NEW event types; the frozen SitosTransaction rows still in the
		// log belong to the fund and are deliberately not reinterpreted here.
		var sitosInterventions int
		var sitosFoodIn, sitosFoodOut float64
		if lrows, lerr := h.pool.Query(r.Context(),
			`SELECT event_type, payload FROM events
			 WHERE stream_id = $1 AND world_tick = $2
			   AND event_type IN ('SitosGranaryStored', 'SitosGranaryReleased')`,
			sett.ID, currentTick,
		); lerr == nil {
			for lrows.Next() {
				var etype string
				var pl []byte
				if lrows.Scan(&etype, &pl) == nil {
					var p economy.SitosGranaryPayload
					if json.Unmarshal(pl, &p) == nil {
						sitosInterventions++
						if etype == economy.EventSitosGranaryReleased {
							sitosFoodIn += p.Total
						} else {
							sitosFoodOut += p.Total
						}
					}
				}
			}
			lrows.Close()
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

		resp["settlement"] = map[string]any{
			"id":                   sett.ID,
			"name":                 sett.Name,
			"owner_id":             sett.OwnerID,
			"kingdom_id":           sett.KingdomID,
			"culture":              sett.CultureID,
			"state":                sett.State,
			"besieged":             sett.Besieged,
			"besieged_by":          besiegedBy,
			"population":           sett.Population,
			"labor_pool":           laborPool,
			"walls":                sett.WallLevel,
			"loyalty":              sett.Loyalty,
			"resources":            resSnap,
			"kharis":               kharisNow,
			"kharis_rate":          kharisRate,
			"kharis_mood":          kharisToMood(kharisNow),
			"kharis_per_tick":      kharisRate,
			"kharis_cap":           kharisCap,
			"max_temple_level":     maxTempleLevel,
			"rite_kharis_cost":     riteKharisCost,
			"kharis_net_per_tick":  kharisNetPerDay,
			"kharis_net_known":     kharisNetKnown,
			"kharis_devotion_idle": kharisDevotionIdle,
			"temple_offers":        templeOffers,
			"grain_prod_rate":      grainProdRate,
			"grain_consum_rate":    grainConsumRate,
			"food_gubbar_required": foodGubbarRequired,
			"food_gubbar_placed":   foodGubbarPlaced,
			"food_self_sufficient": foodSelfSufficient,
			"army":                 sett.Army,
			"army_upkeep":          armyUp,
			// Del C: the sold a garrison spends back into the town it holds. Without
			// this line the net below cannot be derived from the gross above, and the
			// mechanic would be invisible — a silver flow the Wanax cannot see or plan
			// against. Grain has no equivalent: soldiers eat their rations.
			"army_upkeep_circulated_silver":    circulatedSilver,
			"net_grain_per_tick_after_upkeep":  netGrainPerDay,
			"net_silver_per_tick_after_upkeep": netSilverPerDay,
			"build_queue":                      buildQueue,
			"build_queue_max":                  maxParallelBuilds,
			"training_units":                   trainingUnits,
			"buildings":                        buildings,
			"can_afford":                       buildAfford,
			"can_recruit":                      recruitAfford,
			"available_prayers":                prayers,
			// Devotion: the share of the city serving the temple. Mig 094 made cult
			// a labor weight that produces nothing, which removed it from the goods
			// list — and with it the only place a Wanax could see or tend it. A
			// mechanic you cannot see is a mechanic you cannot tend, so it is
			// reported here with the capacity that bounds it (a temple employs
			// kharis.TempleDevotionPerLevel of the city per level; beyond that the
			// devotion has no altar to serve at).
			"devotion":          devotionWeight,
			"devotion_capacity": devotionCapacity,
			"settlement_cap": map[string]any{
				"used": settlementsOwned,
				"max":  province.MaxSettlementsPerWanax,
			},
			"sitos": map[string]any{
				"granary_total":     granaryTotal,
				"granary_per_good":  granaryPerGood,
				"granary_cap":       granaryCap,
				"coverage_ticks":    coverageDays,
				"food_net_per_tick": foodNetPerDay,
				"low_ticks":         h.sitosCfg.LowDays,
				"high_ticks":        h.sitosCfg.HighDays,
			},
			"last_tick": map[string]any{
				"tick":                currentTick,
				"production":          lastTickProd,
				"consumption":         lastTickCons,
				"sitos_interventions": sitosInterventions,
				"sitos_food_in":       sitosFoodIn,
				"sitos_food_out":      sitosFoodOut,
			},
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// upkeepAmount is a grain+silver upkeep total per upkeep-period (the daily tick).
type upkeepAmount struct {
	Grain  float64 `json:"grain"`
	Silver float64 `json:"silver"`
}

// armyUpkeep sums the per-period upkeep of the garrison STANDING IN a settlement,
// from the units table (the SB7 source of truth) via combat.UnitUpkeep. This is
// the composition figure /army reports next to the units it lists — "what the
// force here costs to keep".
//
// It is NOT what this settlement pays. Since mig 100 the payer is
// support_settlement_id, not settlement_id, and since Del C a garrison's sold
// partly circulates back. Use settlementUpkeepDrain for anything that projects
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
		`SELECT type, category, size, status FROM units
		 WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	)
	if err != nil {
		return total, perType, err
	}
	defer rows.Close()
	for rows.Next() {
		var unitType, category, status string
		var size int
		if err := rows.Scan(&unitType, &category, &size, &status); err != nil {
			return total, perType, err
		}
		up := combat.UnitUpkeep(unitType, category, size, status)
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
//   - Sold circulation (silver-plan Del C). Soldiers standing in the town that
//     pays them spend soldShare of their silver back into it, so that town's net
//     silver drain is (1−share)·gross. Projecting gross overstates it — a false
//     negative on the recruit surface, which sits on the chain gate.
//
// Mirrors the tick's own filters: the four upkeep-bearing statuses (garrison,
// marching, positioned, embarked — the last added 2026-08-05, embarkerad
// ranson: an embarked cohort is billed exactly like a marching one, so a read
// surface that forgot it would show a settlement as solvent while the tick
// was already charging it in full, or vice versa), and the payer must still
// exist AND still be owned by the unit's owner (a fallen or captured town
// pays nothing — combat/upkeep.go step 2). Returns the gross grain+silver
// and, separately, the silver that comes back, so the caller can show the
// player an arithmetic they can follow.
func settlementUpkeepDrain(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID, soldShare float64) (gross upkeepAmount, circulatedSilver float64, err error) {
	rows, qerr := pool.Query(ctx,
		`SELECT u.type, u.category, u.size, u.status,
		        COALESCE(u.settlement_id = s.id, false) AS at_home
		 FROM units u
		 JOIN settlements s ON s.id = u.support_settlement_id AND s.owner_id = u.owner_id
		 WHERE u.support_settlement_id = $1
		   AND u.status IN ('garrison', 'marching', 'positioned', 'embarked')`,
		settlementID,
	)
	if qerr != nil {
		return gross, 0, qerr
	}
	defer rows.Close()
	for rows.Next() {
		var unitType, category, status string
		var size int
		var atHome bool
		if serr := rows.Scan(&unitType, &category, &size, &status, &atHome); serr != nil {
			return gross, 0, serr
		}
		up := combat.UnitUpkeep(unitType, category, size, status)
		gross.Grain += up.Grain
		gross.Silver += up.Silver
		if atHome {
			circulatedSilver += soldShare * up.Silver
		}
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
// (P6, soak 2026-07-18). circulatedSilver is the Del C sold share that returns
// to the town the same tick — it never leaves, so it must not count as drain.
// Pure function — no DB — so it's unit-testable without a database.
func upkeepNetPerDay(grainRatePerTick, silverRatePerTick float64, up upkeepAmount, circulatedSilver float64) (grainNetPerDay, silverNetPerDay float64) {
	grainNetPerDay = grainRatePerTick - up.Grain
	silverNetPerDay = silverRatePerTick - (up.Silver - circulatedSilver)
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
	if req.BuildingType == "harbour" || req.BuildingType == "shipyard" {
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
				fmt.Sprintf("%s requires a coastal, sea, river or river ford tile on an adjacent hex", req.BuildingType))
			return
		}
	}

	// Mines require the matching ore deposit in the settlement's production
	// catchment (own hex + hexgrid.CatchmentRadius ring, same set
	// LoadHexProductionOptions reads — a mine without a matching deposit in
	// reach would produce nothing). Gate it at build time.
	//
	// Was a hand-copied "own hex + 6 neighbours" (radius 1) list — stale since
	// P1 (megaron_plan_fysisk_gubbemodell.md, 2026-08-07) doubled the
	// catchment to radius 2 for production and megaron_plan_gruvgrinden.md's
	// keryx-visible catchment_deposits field, but this write gate never
	// followed. Now reads hexgrid.Disk (own hex included, matching that a
	// mine may still be built directly on the ore, as before).
	if req.BuildingType == "mine" || req.BuildingType == "silver_mine" {
		var pq, pr int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT map_q, map_r FROM provinces WHERE id = $1`, provinceID,
		).Scan(&pq, &pr)
		var depositCond, oreName string
		if req.BuildingType == "silver_mine" {
			depositCond = "COALESCE(silver_deposit,false)"
			oreName = "silver"
		} else {
			depositCond = "(copper_deposit OR tin_deposit)"
			oreName = "copper or tin"
		}
		catchQ, catchR := hexgrid.QRArrays(hexgrid.Disk(hexgrid.Coord{Q: pq, R: pr}, hexgrid.CatchmentRadius))
		var hasDeposit bool
		_ = h.pool.QueryRow(r.Context(),
			fmt.Sprintf(`SELECT EXISTS(
			   SELECT 1 FROM map_tiles mt
			   JOIN unnest($2::int[], $3::int[]) AS catchment(q, r) ON mt.q = catchment.q AND mt.r = catchment.r
			   WHERE mt.world_id = $1
			     AND mt.terrain NOT IN ('coastal_sea','deep_sea','river','river_ford')
			     AND %s
			 )`, depositCond),
			worldID, catchQ, catchR,
		).Scan(&hasDeposit)
		if !hasDeposit {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("a %s here would produce nothing — no %s deposit within this settlement's production catchment (its own hex plus every hex within %d steps). Build it on or in reach of the ore.",
					req.BuildingType, oreName, hexgrid.CatchmentRadius))
			return
		}
	}

	// Queue guards — block before we deduct resources.
	// 1. Walls/towers/bronze walls upgrade an existing wall_level; everything else
	//    is a one-instance building (production_rules use UPSERT, duplicates waste resources).
	// 2. No double-queueing the same building.
	// 3. Cap concurrent queue at maxParallelBuilds (package-level const above).
	bType := province.BuildingType(req.BuildingType)
	isWall := req.BuildingType == "wall"

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
		var lvl int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = $2`,
			settlementID, req.BuildingType,
		).Scan(&lvl)
		if lvl >= province.MaxBuildingLevel {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("%s is already at maximum level (%d)", req.BuildingType, province.MaxBuildingLevel))
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
		if wl >= 3 {
			writeError(w, http.StatusUnprocessableEntity, "walls are already at maximum (level 3)")
			return
		}
		// Use the cost/duration for the next wall level (1–3); wl is 0–2 here.
		spec = province.WallLevelSpecs[wl+1]
	}

	var queueDepth int
	var dupQueued bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT
		   COUNT(*),
		   COUNT(*) FILTER (WHERE building_type = $2) > 0
		 FROM build_queue WHERE settlement_id = $1`,
		settlementID, req.BuildingType,
	).Scan(&queueDepth, &dupQueued)
	if dupQueued {
		writeError(w, http.StatusUnprocessableEntity, "this building is already in the queue")
		return
	}
	if queueDepth >= maxParallelBuilds {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("build queue is full (max %d concurrent — finish or wait)", maxParallelBuilds))
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
			writeGoodsError(w, insErr)
		} else {
			writeError(w, http.StatusInternalServerError, "could not deduct resources")
		}
		return
	}

	// Deduct silver if required.
	if spec.CostSilver > 0 {
		tag, err2 := tx.Exec(r.Context(),
			`UPDATE settlement_goods
			   SET amount  = settled(amount, rate, calc_tick) - $1,
			       calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = 'silver'
			   AND settled(amount, rate, calc_tick) >= $1`,
			spec.CostSilver, settlementID,
		)
		if err2 != nil || tag.RowsAffected() == 0 {
			// Name the shortfall (need/have) like writeGoodsError does for goods —
			// a bare "insufficient silver" left a silver-poor Wanax guessing.
			var have float64
			_ = tx.QueryRow(r.Context(),
				`SELECT settled(amount, rate, calc_tick) FROM settlement_goods
				  WHERE settlement_id = $1 AND good_key = 'silver'`,
				settlementID).Scan(&have)
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("insufficient silver (%s)", shortfall(spec.CostSilver, have)))
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct resources")
		return
	}

	var buildCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&buildCurrentTick)
	buildDueTick := buildCurrentTick + spec.DurationTicks
	completeAt := tick.EtaAt(h.clk, buildDueTick, buildCurrentTick)
	var queueID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO build_queue (settlement_id, world_id, building_type, complete_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		settlementID, worldID, req.BuildingType, completeAt,
	).Scan(&queueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not queue build")
		return
	}

	if err := h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledBuildComplete,
		combat.BuildCompletePayload{
			SettlementID: settlementID,
			BuildQueueID: queueID,
			BuildingType: req.BuildingType,
		}, buildDueTick,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule build")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"queue_id":    queueID,
		"complete_at": completeAt,
	})
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

	// Verify ownership and fetch the build entry.
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

	spec, ok := province.BuildingSpecs[province.BuildingType(buildingType)]
	if !ok {
		writeError(w, http.StatusInternalServerError, "unknown building type in queue")
		return
	}

	// For wall, refund the cost of the queued level (wall_level+1 at time of cancel,
	// since wall_level is only incremented on completion).
	if buildingType == "wall" {
		var wl int
		_ = h.pool.QueryRow(r.Context(), `SELECT wall_level FROM settlements WHERE id = $1`, settlementID).Scan(&wl)
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
		var lvl int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = $2`,
			settlementID, buildingType,
		).Scan(&lvl)
		next := lvl + 1
		if next > province.MaxBuildingLevel {
			next = province.MaxBuildingLevel
		}
		if levelled, lok := province.LevelledSpec(bt, next); lok {
			spec = levelled
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Delete the queue entry (atomic check: still pending).
	ct, err := tx.Exec(r.Context(), `DELETE FROM build_queue WHERE id = $1`, queueID)
	if err != nil || ct.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "build already completed or not found")
		return
	}

	// Cancel the scheduled event so the worker never fires.
	_, _ = tx.Exec(r.Context(),
		`DELETE FROM scheduled_events
		 WHERE event_type = 'BuildComplete'
		   AND (payload->>'build_queue_id')::uuid = $1
		   AND processed_at IS NULL`,
		queueID,
	)

	// Refund costs.
	for goodKey, qty := range spec.Costs {
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			     amount  = LEAST(settled(amount, rate, calc_tick) + $1, cap),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = $3`,
			qty, settlementID, goodKey,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not refund goods")
			return
		}
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
		`SELECT building_type, level, built_at FROM buildings WHERE settlement_id = $1 ORDER BY built_at`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load buildings")
		return
	}
	defer rows.Close()

	type buildingRow struct {
		Type    string    `json:"type"`
		Level   int       `json:"level"`
		BuiltAt time.Time `json:"built_at"`
	}
	var result []buildingRow
	for rows.Next() {
		var b buildingRow
		if err := rows.Scan(&b.Type, &b.Level, &b.BuiltAt); err == nil {
			result = append(result, b)
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
	// farm, olive_press, silver_mine och winery har ENBART terrängrader och är
	// därmed genuint gateade — en farm på kalksten producerar ingenting alls,
	// tyst, vilket är exakt den dolda gate P10 (soak 2026-07-18) stänger.
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
		DurationGameDays int  `json:"duration_game_days"`
		RequiresCoastal  bool `json:"requires_coastal,omitempty"`
		RequiresDeposits []string           `json:"requires_deposits,omitempty"`
		RequiresTerrain  []string           `json:"requires_terrain,omitempty"`
		Purpose          string             `json:"purpose"`
		// MaxLevel is 1 for one-instance buildings and MaxBuildingLevel for producing
		// ones, whose level is how many citizens the workplace can employ.
		MaxLevel int `json:"max_level"`
		// UpgradeCosts maps level → full cost of taking the building to that level
		// (level 1 is Costs above). Empty for buildings with no level ladder.
		UpgradeCosts map[int]map[string]float64 `json:"upgrade_costs,omitempty"`
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
			MaxLevel:        1,
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
		if g, ok := gates[bt]; ok {
			entry.RequiresCoastal = g.requiresCoastal
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

// UnitCatalogue handles GET /api/v1/units — returns the static catalogue of all
// recruitable unit types: the resource cost for one recruit action (a whole
// 100-man cohort for land units, one vessel for naval — the same quantities
// `recruitPerManCosts` scales up to deduct), the population-pool gate,
// training duration, and the building requirement. No world/auth required —
// static reference data, mirrors BuildingCatalogue.
func (h *ProvinceHandler) UnitCatalogue(w http.ResponseWriter, r *http.Request) {
	type unitEntry struct {
		Type             string             `json:"type"`
		Costs            map[string]float64 `json:"costs"`
		BatchMen         int                `json:"batch_men"` // men (land) or crew (naval) the Costs above pay for in one recruit call
		PopCost          int                `json:"pop_cost"`
		// Game-days, not wall-clock minutes — see buildingEntry.DurationGameDays
		// above for the full rationale (same fix, same class of bug).
		DurationGameDays int  `json:"duration_game_days"`
		RequiresBarracks bool `json:"requires_barracks,omitempty"`
		RequiresStable   bool               `json:"requires_stable,omitempty"`
		RequiresHarbour  bool               `json:"requires_harbour,omitempty"`
		RequiresShipyard bool               `json:"requires_shipyard,omitempty"`
		RequiresFoundry  bool               `json:"requires_foundry,omitempty"`
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
// DUAL-WRITE: the old integer army column is also incremented so existing
// combat/display code continues to work until C4/C8.
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

	// Naval build-queue cap: at most 10 vessels building per settlement (one
	// TrainComplete per vessel = its build time). Land no longer schedules
	// per-10-men batches — a unit trains as a single job when it reaches 100
	// (see the create-loop below) — so this cap is naval-only now.
	if cat == unit.CategoryNaval {
		var pendingBuilds int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM scheduled_events
			 WHERE world_id = $1 AND event_type = 'TrainComplete'
			   AND processed_at IS NULL AND failed_at IS NULL
			   AND (payload->>'settlement_id')::uuid = $2`,
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

	// Coarse precondition — the same checker `keryx actions` uses
	// (temenos_capabilities.md Fas 3): population > 0, and at least one unit
	// type affordable at the 10-man minimum batch. Sound as a full gate here
	// (not just population): if NO type is affordable even at the smallest
	// valid batch (10 men — the floor enforced above), no larger request for
	// ANY type can succeed either, so this cannot false-reject a request that
	// would otherwise go through. The finer per-type building/goods checks
	// below stay handler-specific — they depend on exactly which type and
	// how many men this specific request asks for.
	cc := capabilities.NewContext(r.Context(), h.pool, h.clk, worldID, provinceID, playerID, settlementID)
	if v := capabilities.CanRecruit(cc); !v.Available {
		writeError(w, http.StatusUnprocessableEntity, capabilities.FirstUnsatisfied(v))
		return
	}
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
					"foundry is still finishing here — it becomes usable within a tick of completion; retry shortly")
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

	// Deduct payment (goods + kharis + population) atomically in one transaction so
	// a kharis/population shortfall can't leave goods already committed (partial-drain).
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct resources")
		return
	}
	defer tx.Rollback(r.Context())

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
		if err2 != nil || tag.RowsAffected() == 0 {
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

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not draft men")
		return
	}

	// C-collapse: overmobilisation — city drained to ≤ 100 → schedule collapse.
	if popAfter <= 100 {
		var collapseCurrentTick int
		_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&collapseCurrentTick)
		if err := h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledCollapseSettlement,
			combat.CollapseSettlementPayload{
				SettlementID: settlementID,
				WorldID:      worldID,
				Cause:        "overmobilisation",
			},
			collapseCurrentTick,
		); err != nil {
			// Non-fatal: log and continue — the collapse will be picked up by the daily tick.
			slog.Warn("recruit: could not schedule collapse event",
				"settlement", settlementID, "pop_after", popAfter, "err", err)
		} else {
			slog.Info("recruit: overmobilisation collapse scheduled",
				"settlement", settlementID, "pop_after", popAfter)
		}
	}

	// batchTicks: for land this is the per-10-men batch duration (looped);
	// for naval it is reused, unlooped, as the single vessel's build time —
	// same UnitSpecs[type].DurationTicks tunable, just a different multiplier
	// below (ship-build overhaul 2026-07-09).
	batchTicks := recruitBatchTicks(req.UnitType)
	var trainCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&trainCurrentTick)

	// Naval-only: resolve the Wanax's culture (from their capital settlement,
	// same pattern as the music player's capital.culture on the web client)
	// for a culture-appropriate name suggestion, and collect this Wanax's
	// existing ship names in this world so we can steer clear of a repeat.
	var culture string
	takenNames := make(map[string]bool)
	if cat == unit.CategoryNaval {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT culture_id FROM settlements WHERE owner_id = $1 AND world_id = $2 AND is_capital = true`,
			playerID, worldID,
		).Scan(&culture)
		if nameRows, nameErr := h.pool.Query(r.Context(),
			`SELECT name FROM units WHERE owner_id = $1 AND world_id = $2 AND name IS NOT NULL`,
			playerID, worldID,
		); nameErr == nil {
			for nameRows.Next() {
				var n string
				if nameRows.Scan(&n) == nil {
					takenNames[n] = true
				}
			}
			nameRows.Close()
		}
	}

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
			shipOrdinal, err := unit.AllocateOrdinal(r.Context(), h.pool, settlementID, string(uType))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not allocate ship ordinal")
				return
			}
			var unitID uuid.UUID
			if err := h.pool.QueryRow(r.Context(),
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
			unitIDs = append(unitIDs, unitID)
			unitNames = append(unitNames, chosenName)
			finalSize = 1

			if err := h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledTrainComplete,
				combat.TrainCompletePayload{
					SettlementID: settlementID,
					UnitType:     req.UnitType,
					Count:        1,
					UnitID:       unitID,
				}, dueTick,
			); err != nil {
				writeError(w, http.StatusInternalServerError, "could not schedule ship build")
				return
			}

			// Outcome (Fas 2.3: the name was already chosen above, once) + row
			// (Fas 2.2/2.4: UnitFormed already existed for starter units; this is
			// its first use from Recruit, adding the optional Name field it was
			// defined with).
			if h.eventStore != nil {
				_, _ = h.eventStore.Append(r.Context(), unitID, events.StreamType(unit.StreamUnit), unit.EventUnitFormed,
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
		row := h.pool.QueryRow(r.Context(),
			`SELECT id, size FROM units
			 WHERE settlement_id = $1 AND type = $2 AND status = 'forming'
			 ORDER BY created_at LIMIT 1`,
			settlementID, string(uType),
		)
		var eid uuid.UUID
		if scanErr := row.Scan(&eid, &existingSize); scanErr == nil {
			existingUnitID = &eid
		}

		newSize := existingSize + req.Men
		unitSize := newSize
		if unitSize > economy.MaxUnitSize {
			unitSize = economy.MaxUnitSize // cap; the remainder spills into a new forming unit below
		}

		var unitID uuid.UUID
		if existingUnitID != nil {
			if err := h.pool.QueryRow(r.Context(),
				`UPDATE units SET size = $1, updated_at = now() WHERE id = $2 RETURNING id`,
				unitSize, *existingUnitID,
			).Scan(&unitID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not grow forming unit")
				return
			}
		} else {
			ordinal, err := unit.AllocateOrdinal(r.Context(), h.pool, settlementID, string(uType))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not allocate unit ordinal")
				return
			}
			if err := h.pool.QueryRow(r.Context(),
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
			if _, err := h.pool.Exec(r.Context(),
				`UPDATE units SET status = 'training', build_complete_at = $1, updated_at = now() WHERE id = $2`,
				completeAt, unitID,
			); err != nil {
				writeError(w, http.StatusInternalServerError, "could not start unit training")
				return
			}
			if err := h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledTrainComplete,
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
				spillOrdinal, err := unit.AllocateOrdinal(r.Context(), h.pool, settlementID, string(uType))
				if err != nil {
					writeError(w, http.StatusInternalServerError, "could not allocate spill ordinal")
					return
				}
				if _, err := h.pool.Exec(r.Context(),
					`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
					                    settlement_id, support_settlement_id, ordinal, origin_settlement_id)
					 VALUES ($1, $2, $3, $4, $5, $6, 'forming', $7, $7, $8, $7)`,
					worldID, playerID, string(uType), string(cat), newSize-100, crew, settlementID, spillOrdinal,
				); err != nil {
					writeError(w, http.StatusInternalServerError, "could not create spill forming unit")
					return
				}
			}
		}
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
		newUnitUp := combat.UnitUpkeep(req.UnitType, string(cat), fullSize, "garrison")
		if (netGrainPerDay-newUnitUp.Grain) < 0 || (netSilverPerDay-newUnitUp.Silver) < 0 {
			upkeepWarning = fmt.Sprintf(
				"warning: once this unit garrisons it needs %.1f grain + %.1f silver/tick upkeep — "+
					"this settlement's current net after its existing army's upkeep is %+.1f grain/tick, %+.1f silver/tick; "+
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
		    COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'), 0),
		    COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'), 0),
		    (SELECT population FROM settlements WHERE id = $1)`,
		settlementID,
	).Scan(&grainRate, &silverRate, &population); err != nil {
		return 0, 0, err
	}
	up, circulatedSilver, upErr := settlementUpkeepDrain(ctx, pool, settlementID, combat.UpkeepSoldShare())
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
	grainNetPerDay, silverNetPerDay = upkeepNetPerDay(grainNetOfPopulation, silverRate, up, circulatedSilver)
	return grainNetPerDay, silverNetPerDay, nil
}

// Goods handles GET /worlds/:worldID/provinces/:provinceID/goods.
// Returns the settlement's goods inventory with lazy-eval amounts and local prices.
// placedCitizensByGood collapses a settlement's PlacementCounts into citizens
// employed per good — one placed gubbe = 100 citizens (Temenos_varutaxonomi_sol
// §1.1). This is the honest post-P4 replacement for the pre-P4 settlement_labor
// weights the goods surface used to read: production is driven by
// settlement_placement, so the worker counts shown beside it must be too. Cult
// is NOT here — it stays on its settlement_labor devotion path (the one good P4
// left on weights).
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

	// Load labor weights (weight ∈ [0,1] = fraction of labor_pool).
	wrows, _ := h.pool.Query(r.Context(),
		`SELECT good_key, weight FROM settlement_labor WHERE settlement_id = $1`, settlementID,
	)
	laborWeights := make(map[string]float64)
	if wrows != nil {
		for wrows.Next() {
			var k string
			var w float64
			_ = wrows.Scan(&k, &w)
			laborWeights[k] = w
		}
		wrows.Close()
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
	workplaceLevels := make(map[string]int)
	lvlRows, _ := h.pool.Query(r.Context(),
		`SELECT good_key, SUM(level)::int FROM (
		     SELECT DISTINCT pr.good_key, b.building_type, b.level
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
		        g.base_value, g.name, g.tier, g.category
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
	// Cult is the one good still staffed by settlement_labor devotion weight
	// (temple labor, untouched by P4) — keep its live path here.
	cultCitizens := int(math.Round(laborWeights[economy.GoodCult] * float64(laborPool)))

	// marginal_yield (P4-arvet, replaces the old catchment-aggregate figure):
	// "what does my NEXT gubbe on this good give" — the same shared formula
	// PlacementOptions itemises per hex/building (economy.MarginalYieldForSlot),
	// aggregated here to the best available slot per good so /goods keeps its
	// one-row-per-good shape. Never a second formula.
	hexOptionsForYield, _ := economy.LoadHexProductionOptions(r.Context(), h.pool, settlementID, nil)
	buildingOptionsForYield, _ := economy.LoadBuildingProductionOptions(r.Context(), h.pool, settlementID)
	marginalYields := economy.MarginalYieldPerGood(hexOptionsForYield, buildingOptionsForYield, placed)

	// Idle = population neither placed on a good nor devoted to the temple.
	// placed.Total counts only non-cult gubbar (cult never enters
	// settlement_placement), so cult citizens are subtracted separately.
	idleCitizens := laborPool - placed.Total*100 - cultCitizens
	if idleCitizens < 0 {
		idleCitizens = 0
	}

	type goodRow struct {
		Key           string  `json:"key"`
		Name          string  `json:"name"`
		Tier          string  `json:"tier"`
		Category      string  `json:"category"`
		Amount        float64 `json:"amount"`
		Rate          float64 `json:"rate_per_tick"`
		Cap           float64 `json:"cap"`
		BaseValue     float64 `json:"base_value"`
		Citizens      int     `json:"citizens"`
		Percent       float64 `json:"percent"`
		MarginalYield float64 `json:"marginal_yield"`
		Producible    bool    `json:"producible"`
		LaborPool     int     `json:"labor_pool"`
		IdleCitizens  int     `json:"idle_citizens"`
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
		var baseValue float64
		if err := rows.Scan(&key, &current, &rate, &capV, &baseValue, &name, &tier, &category); err != nil {
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
		if key == economy.GoodCult {
			// Cult is unchanged from the pre-P4 path: devotion weight clamped to
			// temple capacity, with any over-allocation reported as unserved.
			// It is the one good still driven by settlement_labor.
			allocated := laborWeights[key]
			served := allocated
			if served > capacity {
				served = capacity
			}
			employed = int(math.Round(served * float64(laborPool)))
			unserved = int(math.Round((allocated - served) * float64(laborPool)))
			if unserved < 0 {
				unserved = 0
			}
		} else {
			// Every other good is staffed by real placements. Placement enforces
			// caps at write time, so there is no "unserved" over-allocation state.
			employed = employedByGood[key]
			unserved = 0
		}
		percent := 0.0
		if laborPool > 0 {
			percent = float64(employed) / float64(laborPool) * 100.0
		}
		result = append(result, goodRow{
			Key:           key,
			Name:          name,
			Tier:          tier,
			Category:      category,
			Amount:        current,
			Rate:          rate,
			Cap:           capV,
			BaseValue:     baseValue,
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
// Newest-first by default; ?order=asc for chronological. The loyalty row is the
// placeholder "—" until Fas 3.
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
		Loyalty     string             `json:"loyalty"` // "—" until Fas 3
	}
	ticks := make([]tickRow, 0, last)
	for tk := fromTick; tk <= currentTick; tk++ {
		evs := eventsByTick[tk]
		if evs == nil {
			evs = []tickEvent{}
		}
		ticks = append(ticks, tickRow{
			Tick: tk, Production: production, Consumption: consumption,
			Events: evs, Loyalty: "—",
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
		DestinationID uuid.UUID `json:"destination_id"`
		GoodKey       string    `json:"good_key"`
		Quantity      float64   `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "quantity must be positive")
		return
	}
	if req.GoodKey == "" {
		writeError(w, http.StatusBadRequest, "good_key required")
		return
	}

	// Find origin settlement and verify ownership.
	var originID uuid.UUID
	var originQ, originR int
	err = h.pool.QueryRow(r.Context(),
		`SELECT s.id, prov.map_q, prov.map_r
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.province_id = $1 AND s.world_id = $2 AND s.owner_id = $3`,
		provinceID, worldID, playerID,
	).Scan(&originID, &originQ, &originR)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Get destination — also verify it's owned by the same player (internal transfer only).
	// External trade requires messenger-based negotiation.
	var destQ, destR int
	var destOwnerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT prov.map_q, prov.map_r, s.owner_id
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1 AND s.world_id = $2`,
		req.DestinationID, worldID,
	).Scan(&destQ, &destR, &destOwnerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "destination settlement not found")
		return
	}
	if destOwnerID == nil || *destOwnerID != playerID {
		writeError(w, http.StatusForbidden,
			"use messenger trade offers to trade with other players — /trade is for internal transfers only")
		return
	}

	weight, shippable, err := economy.IsShippableGood(r.Context(), h.pool, req.GoodKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown good")
		return
	}
	if !shippable {
		writeError(w, http.StatusBadRequest,
			"cannot transfer cult — cult is produced by temple labor and converted to kharis in place, it cannot be shipped")
		return
	}

	dist := province.HexDistance(
		province.MapPosition{Q: originQ, R: originR},
		province.MapPosition{Q: destQ, R: destR},
	)
	base := 30.0 + float64(dist)*2.0
	weightPenalty := 0.0
	if weight > 1.0 {
		weightPenalty = (weight - 1.0) * 0.1
	}
	travelMins := base * (1.0 + weightPenalty)

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Deduct from origin — silver is now a normal good in settlement_goods.
	deductTag, err := tx.Exec(r.Context(),
		`UPDATE settlement_goods SET
		     amount = settled(amount, rate, calc_tick) - $1,
		     calc_tick = current_world_tick()
		 WHERE settlement_id = $2 AND good_key = $3
		   AND settled(amount, rate, calc_tick) >= $1`,
		req.Quantity, originID, req.GoodKey,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct goods")
		return
	}
	if deductTag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "insufficient goods")
		return
	}

	arrivesAt := h.clk.Now().Add(time.Duration(travelMins * float64(time.Minute)))
	var tradeCurrentTick int
	_ = tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&tradeCurrentTick)
	tradeTravelTicks := int(math.Round(travelMins / 60))
	if tradeTravelTicks < 1 {
		tradeTravelTicks = 1
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
	transportID, err := transport.CreateShadow(r.Context(), tx, transport.DispatchParams{
		WorldID:       worldID,
		OwnerID:       playerID,
		Kind:          "transfer",
		OriginID:      originID,
		DestID:        req.DestinationID,
		Category:      "land",
		OriginQ:       originQ,
		OriginR:       originR,
		DestQ:         destQ,
		DestR:         destR,
		DepartsAt:     h.clk.Now(),
		ArrivesAt:     arrivesAt,
		DueTick:       tradeCurrentTick + tradeTravelTicks,
		Manifest:      transport.Manifest{req.GoodKey: req.Quantity},
		Interceptable: true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch transfer caravan")
		return
	}

	// Internal transfer: no loss, no gain — delivered quantity equals what was sent.
	// Enqueue delivery within the same transaction — atomic with the deduction.
	if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
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

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"route_id":      routeID,
		"arrives_at":    arrivesAt,
		"distance":      dist,
		"travel_min":    travelMins,
		"delivered_qty": req.Quantity,
	})
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

// RecallMarch handles DELETE /worlds/:worldID/provinces/:provinceID/marches/:marchID.
// Issues a recall order: a messenger is dispatched from the home settlement to the
// army's destination. The return march begins only when the messenger arrives.
// Total recall time = messenger travel out + return march home.
func (h *ProvinceHandler) RecallMarch(w http.ResponseWriter, r *http.Request) {
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
	marchID, err := uuid.Parse(chi.URLParam(r, "marchID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid march ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer tx.Rollback(r.Context())

	// Load march and verify ownership via FOR UPDATE (prevents race with arrival handler).
	var march struct {
		Spearman      int
		WarChariot    int
		Ship          int
		EliteInfantry int
		WarGalley     int
		Merchantman   int
		Resolved      bool
		OriginID      uuid.UUID
		TargetID      uuid.UUID
		DepartsAt     time.Time
		ArrivesAt     time.Time
	}
	err = tx.QueryRow(r.Context(),
		`SELECT infantry, chariot, ship, elite_infantry,
		        war_galley, merchantman, resolved, origin_id, target_id, departs_at, arrives_at
		 FROM marching_armies
		 WHERE id = $1 AND world_id = $2 AND origin_id = $3
		 FOR UPDATE`,
		marchID, worldID, provinceID,
	).Scan(&march.Spearman, &march.WarChariot,
		&march.Ship, &march.EliteInfantry, &march.WarGalley, &march.Merchantman,
		&march.Resolved, &march.OriginID, &march.TargetID, &march.DepartsAt, &march.ArrivesAt)
	if err != nil {
		writeError(w, http.StatusNotFound, "march not found or not yours")
		return
	}
	if march.Resolved {
		writeError(w, http.StatusConflict, "march already resolved")
		return
	}

	// Verify player owns the origin province; capture the commanding settlement for the messenger.
	var ownerID *uuid.UUID
	var originSettlementID uuid.UUID
	_ = tx.QueryRow(r.Context(),
		`SELECT id, owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&originSettlementID, &ownerID)
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	// The army keeps marching — command is not instant. We do NOT resolve the outbound march here;
	// a recall messenger is dispatched, and only when it reaches the army (ScheduledRecallArrival)
	// does the army turn around. If the army arrives and fights first, the recall arrives too late —
	// handled explicitly below (dispatch-time gate) and in RecallArrivalHandler.handleMarch (the
	// residual delivery-time race), never as a silent miss.

	// Hex positions of origin (home) and target (where the army is heading).
	var oQ, oR, tQ, tR int
	_ = tx.QueryRow(r.Context(), `SELECT map_q, map_r FROM provinces WHERE id = $1`, march.OriginID).Scan(&oQ, &oR)
	_ = tx.QueryRow(r.Context(), `SELECT map_q, map_r FROM provinces WHERE id = $1`, march.TargetID).Scan(&tQ, &tR)
	originPos := province.MapPosition{Q: oQ, R: oR}
	targetPos := province.MapPosition{Q: tQ, R: tR}

	// The army's own movement category (needed to re-walk its outbound path).
	// marching_armies is an aggregate row — mixed land+naval composition is
	// possible (a garrison riding its own ships) and there is no "embarked"
	// flag to disambiguate it, so this is a documented heuristic, not a
	// certainty: any naval hull present means the whole force's motion is
	// bound to water passability (land troops cannot cross open water on
	// their own), so naval wins whenever ship+war_galley+merchantman > 0.
	category := messenger.AggregateArmyCategory(
		march.Spearman, march.WarChariot,
		march.Ship, march.EliteInfantry, march.WarGalley, march.Merchantman)

	// Verify the category actually connects origin→target before trusting it.
	// Unlike an individual unit's march (validated by FindPath at StartMarch
	// dispatch), a marching_armies row reaching this handler may itself be a
	// RETURN march created by an earlier recall (RecallArrivalHandler.handleMarch,
	// below) — created from a flat hex-distance + terrain-cost estimate, never
	// path-validated. A composition guess can also simply be wrong for a mixed
	// force. Try the other category before concluding no route exists at all:
	// this is what separates a genuine data problem (fall b — the route itself
	// cannot be verified) from an honest "no courier can catch it in time"
	// (fall a) — the two must never collapse into the same silent 422.
	_, _, pathOK, pathErr := province.FindPath(r.Context(), tx, worldID, originPos, targetPos, category)
	if pathErr != nil {
		writeError(w, http.StatusInternalServerError, "could not verify the army's route")
		return
	}
	if !pathOK {
		altCategory := "land"
		if category == "land" {
			altCategory = "naval"
		}
		_, _, altOK, altErr := province.FindPath(r.Context(), tx, worldID, originPos, targetPos, altCategory)
		if altErr != nil {
			writeError(w, http.StatusInternalServerError, "could not verify the army's route")
			return
		}
		if altOK {
			category, pathOK = altCategory, true
		}
	}
	if !pathOK {
		slog.Error("recall: no route connects march origin to target under either category — march data may be stale or corrupt",
			"march_id", marchID, "origin", originPos, "target", targetPos)
		writeError(w, http.StatusUnprocessableEntity,
			"could not verify a route for this army between its recorded start and destination under either land or naval movement — this is a data problem, not a timing problem; the recall was not sent")
		return
	}

	// Aim the recall messenger at an honest interception point along the
	// army's route — never blindly at its destination (the old bug this
	// handler carried: see the doc comment on RecallMarchPayload below).
	// Command is never instant: the messenger must be physically ABLE to
	// catch the army before it completes its march, or the order is refused
	// now, visibly, instead of racing to a silent miss.
	interceptPos, interceptOK, err := messenger.InterceptCourierTarget(r.Context(), tx, worldID,
		originPos, originPos, targetPos, category, march.DepartsAt, march.ArrivesAt, h.clk.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve recall interception")
		return
	}
	if !interceptOK {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("no messenger can catch this army before it completes its march (arrives %s) — wait for it to arrive, then issue a fresh order",
				march.ArrivesAt.Local().Format(time.RFC3339)))
		return
	}

	// marching_armies.target_id is a NOT NULL FK to provinces, and every map
	// hex has a province row — resolve the interception hex's province id so
	// the return march RecallArrivalHandler.handleMarch creates can use it as
	// its departure point.
	var interceptProvinceID uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`SELECT id FROM provinces WHERE world_id = $1 AND map_q = $2 AND map_r = $3`,
		worldID, interceptPos.Q, interceptPos.R,
	).Scan(&interceptProvinceID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not resolve interception province")
		return
	}

	// Dispatch a visible recall messenger toward the interception hex, not the
	// army's full destination — messenger_travel_ticks below is honestly the
	// courier's time to THAT point.
	recallTravelTicks, recallTravelDur := messenger.CourierTravel(r.Context(), h.pool, worldID,
		originPos, interceptPos)
	messengerArrivesAt := h.clk.Now().Add(recallTravelDur)

	var marchRecallCurrentTick int
	_ = tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&marchRecallCurrentTick)
	marchRecallDueTick := marchRecallCurrentTick + recallTravelTicks

	var messengerID uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,$4,'outbound','recall',$5,$6,$7,$8,$9)
		 RETURNING id`,
		worldID, playerID, originSettlementID, "Recall order — return home.",
		oQ, oR, interceptPos.Q, interceptPos.R, messengerArrivesAt,
	).Scan(&messengerID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch recall messenger")
		return
	}

	payload := messenger.RecallMarchPayload{
		Kind:          "march",
		WorldID:       worldID,
		MessengerID:   messengerID,
		MarchID:       marchID,
		Spearman:      march.Spearman,
		WarChariot:    march.WarChariot,
		Ship:          march.Ship,
		EliteInfantry: march.EliteInfantry,
		WarGalley:     march.WarGalley,
		Merchantman:   march.Merchantman,
		OriginQ:       oQ,
		OriginR:       oR,
		TargetQ:       interceptPos.Q,
		TargetR:       interceptPos.R,
		OriginID:      march.OriginID,
		TargetID:      interceptProvinceID,
	}
	// Messenger row + recall-arrival event committed atomically.
	if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledRecallArrival,
		payload, marchRecallDueTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule recall arrival")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"recalled":               true,
		"messenger_id":           messengerID,
		"messenger_arrives_at":   messengerArrivesAt,
		"messenger_travel_ticks": recallTravelTicks,
	})
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
		ID           uuid.UUID `json:"id"`
		PeerName     string    `json:"peer_name"` // destination for outgoing, origin for incoming
		GoodKey      string    `json:"good_key"`
		Quantity     float64   `json:"quantity"`
		DeliveredQty float64   `json:"delivered_qty"`
		Direction    string    `json:"direction"` // "outgoing" | "incoming"
		DepartsAt    time.Time `json:"departs_at"`
		ArrivesAt    time.Time `json:"arrives_at"`
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
	writeJSON(w, http.StatusOK, result)
}

// disbandPlan decides how to satisfy a disband of `men` from a set of garrison
// units of one type, given their sizes in the order they should be consumed
// (callers pass them smallest-first, so leftover fragments clear before
// full-strength units are touched). It returns, per input unit, how many men to
// remove from it: 0 = untouched, ==size = disband the whole unit, in between =
// shrink it. Never removes more than a unit holds, and stops once `men` is met,
// so asking to disband more than the garrison has simply disbands what exists.
func disbandPlan(sizes []int, men int) []int {
	plan := make([]int, len(sizes))
	remaining := men
	for i, size := range sizes {
		if remaining <= 0 {
			break
		}
		take := size
		if take > remaining {
			take = remaining
		}
		plan[i] = take
		remaining -= take
	}
	return plan
}

// Disband handles POST /worlds/:worldID/provinces/:provinceID/disband.
// Releases garrison units of the requested types back into civilian life,
// consuming them from the units table (the SB7 source of truth). Variant B:
// disband does not restore population directly; labor rises as army pop-cost falls.
func (h *ProvinceHandler) Disband(w http.ResponseWriter, r *http.Request) {
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
		Spearman      int `json:"spearman"`
		WarChariot    int `json:"war_chariot"`
		Ship          int `json:"ship"` // galley
		EliteInfantry int `json:"elite_infantry"`
		WarGalley     int `json:"war_galley"`
		Merchantman   int `json:"merchantman"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Verify ownership.
	var settlementID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT s.id FROM settlements s WHERE s.province_id=$1 AND s.world_id=$2 AND s.owner_id=$3`,
		provinceID, worldID, playerID,
	).Scan(&settlementID); err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Variant B: disband does NOT restore population.
	// The army lives in the units table (single source of truth since the SB7
	// drop of the settlements.* army columns). Disbanding consumes garrison units
	// of the requested type; labor_pool rises automatically because the army's
	// pop-cost decreases. RecomputeProduction updates the rates.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	wanted := []struct {
		men int
		typ string
		key string
	}{
		{req.Spearman, "spearman", "spearman"},
		{req.WarChariot, "war_chariot", "war_chariot"},
		{req.Ship, "galley", "ship"},
		{req.EliteInfantry, "elite_infantry", "elite_infantry"},
		{req.WarGalley, "war_galley", "war_galley"},
		{req.Merchantman, "merchantman", "merchantman"},
	}

	disbanded := map[string]int{}
	for _, want := range wanted {
		disbanded[want.key] = 0
		if want.men <= 0 {
			continue
		}
		// Load the garrison units of this type, smallest first, so a disband
		// clears leftover fragments before biting into full-strength units.
		rows, err := tx.Query(r.Context(),
			`SELECT id, size FROM units
			 WHERE settlement_id = $1 AND status = 'garrison' AND type = $2
			 ORDER BY size ASC`,
			settlementID, want.typ,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "disband failed")
			return
		}
		var ids []uuid.UUID
		var sizes []int
		for rows.Next() {
			var id uuid.UUID
			var size int
			if scanErr := rows.Scan(&id, &size); scanErr != nil {
				rows.Close()
				writeError(w, http.StatusInternalServerError, "disband failed")
				return
			}
			ids = append(ids, id)
			sizes = append(sizes, size)
		}
		rows.Close()

		// Decide the per-unit consumption, then apply it: a unit consumed in full
		// is disbanded, a partially-consumed one is shrunk.
		plan := disbandPlan(sizes, want.men)
		for i, take := range plan {
			if take <= 0 {
				continue
			}
			if take >= sizes[i] {
				if _, err := tx.Exec(r.Context(),
					`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`, ids[i],
				); err != nil {
					writeError(w, http.StatusInternalServerError, "disband failed")
					return
				}
			} else {
				if _, err := tx.Exec(r.Context(),
					`UPDATE units SET size = size - $1, updated_at = now() WHERE id = $2`, take, ids[i],
				); err != nil {
					writeError(w, http.StatusInternalServerError, "disband failed")
					return
				}
			}
			disbanded[want.key] += take
		}
	}

	if err := economy.RecomputeProduction(r.Context(), tx, settlementID); err != nil {
		writeError(w, http.StatusInternalServerError, "recompute production failed")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	// Report what was actually disbanded (may be less than requested if the
	// garrison held fewer men of a type than asked).
	writeJSON(w, http.StatusOK, map[string]any{
		"disbanded": disbanded,
	})
}

// LaborAlloc handles PUT /worlds/:worldID/provinces/:provinceID/labor.
// Body: {"percent":{"cult":30}}
//
// This is a CULT-DEVOTION endpoint, not a production lever. Production goods
// were moved to gubbe placement in P4 (2026-08-08, settlement_placement) —
// every other good's settlement_labor weight has been a dead write since
// then. This handler used to accept a percent per good; it now accepts and
// persists only "cult" and explicitly reports every other key as ignored,
// rather than silently no-opping them (megaron_plan_riv_procentallokeringen.md).
// `settlement_labor` itself is not going anywhere: cult (temple devotion)
// still lives there and is the one row still read live (kharis/tick.go).
func (h *ProvinceHandler) LaborAlloc(w http.ResponseWriter, r *http.Request) {
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
		Percent map[string]float64 `json:"percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Percent) == 0 {
		writeError(w, http.StatusBadRequest, "invalid JSON — expected {\"percent\":{\"cult\":25}} (temple devotion, % of population; production is set with placements — see `keryx place`)")
		return
	}

	// Verify ownership.
	var settlementID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT s.id FROM settlements s WHERE s.province_id=$1 AND s.world_id=$2 AND s.owner_id=$3`,
		provinceID, worldID, playerID,
	).Scan(&settlementID); err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	var templeLevel int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(level), 0) FROM buildings WHERE settlement_id = $1 AND building_type = 'temple'`,
		settlementID,
	).Scan(&templeLevel)
	// Temple devotion capacity: a temple of level L can employ up to
	// kharis.TempleDevotionPerLevel x L of the population at the altar (Timothy
	// 2026-07-24). Cult labor is allocatable up to this cap, ADDITIVE, so devoting
	// more never starves the producing jobs. Before this, cult was pinned at the
	// 0.15 floor and unallocatable, so a level-2 temple's extra capacity could never
	// be filled and its kharis could never climb (sondrunda 2026-07-24).
	cultCapacity := kharis.TempleDevotionPerLevel * float64(templeLevel)

	// Accept only "cult"; every other key is reported back as ignored rather
	// than silently dropped — the whole point of this slice is that a Wanax
	// who names grain/timber/… gets told plainly that it does nothing here.
	cultWeight := -1.0 // sentinel: player did not name cult → KH1 preserves existing devotion below
	pct, named := req.Percent["cult"]
	var ignoredKeys []string
	for key := range req.Percent {
		if key != "cult" {
			ignoredKeys = append(ignoredKeys, key)
		}
	}
	sort.Strings(ignoredKeys)

	if named {
		if pct < 0 || pct > 100 {
			writeError(w, http.StatusUnprocessableEntity, "percent for cult must be between 0 and 100")
			return
		}
		if templeLevel == 0 {
			writeError(w, http.StatusUnprocessableEntity,
				"cult (devotion) needs a temple here — build one first")
			return
		}
		cw := pct / 100.0
		if cw < kharis.TempleDevotionPerLevel {
			cw = kharis.TempleDevotionPerLevel // never below the holy floor
		}
		if cw > cultCapacity {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("cult capped at %.0f%% by your level-%d temple — build a higher temple to devote more of the city",
					cultCapacity*100, templeLevel))
			return
		}
		cultWeight = cw
	}

	// KH1 (decision A, locked 2026-08-07): when the client omits cult, the server
	// PRESERVES the settlement's current devotion instead of resetting it to the
	// floor. Keryx/agents that don't resend cult must behave identically to a
	// web client that does. Read the live value now and carry it forward,
	// clamped to the temple's capacity (in case the temple was downgraded
	// since the value was set).
	if cultWeight < 0 && templeLevel > 0 {
		var existing float64
		if err := h.pool.QueryRow(r.Context(),
			`SELECT weight FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'cult'`,
			settlementID,
		).Scan(&existing); err == nil {
			if existing > cultCapacity {
				existing = cultCapacity
			}
			if existing > kharis.TempleDevotionPerLevel {
				cultWeight = existing // above the floor → worth preserving
			}
		}
	}

	explainer := "labor percentages only apply to cult (temple devotion); production is set with placements — see `keryx place`"
	if len(ignoredKeys) > 0 {
		explainer = fmt.Sprintf("%s (ignored: %s)", explainer, strings.Join(ignoredKeys, ", "))
	}

	if templeLevel == 0 {
		// No temple → no devotion lever at all; nothing to persist.
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "this settlement has no temple — " + explainer,
		})
		return
	}

	cw := cultWeight
	if cw < 0 {
		cw = kharis.TempleDevotionPerLevel // player didn't name cult → hold the floor
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Targeted UPSERT on the cult row only. This replaces the old
	// DELETE-all-then-reinsert, which is what made KH1's preservation logic
	// necessary in the first place — touching only the cult row means there
	// is no other row to lose.
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO settlement_labor (settlement_id, good_key, weight)
		 VALUES ($1, 'cult', $2)
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET weight = $2`,
		settlementID, cw,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not apply cult labor")
		return
	}

	if err := economy.RecomputeProduction(r.Context(), tx, settlementID); err != nil {
		writeError(w, http.StatusInternalServerError, "recompute production failed")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	// Audit the devotion change. LaborAllocated's semantics ("the labour
	// weights changed") are unchanged and frozen forever (CLAUDE.md §Events) —
	// only the payload shrank to the one weight that is still real.
	if h.eventStore != nil {
		auditPayload := map[string]any{"weights": map[string]float64{"cult": cw}}
		_, _ = h.eventStore.Append(r.Context(), settlementID, events.StreamProvince, "LaborAllocated",
			auditPayload, worldID, nil)
	}

	message := "cult devotion updated and production recomputed"
	if len(ignoredKeys) > 0 {
		message += "; " + explainer
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cult_percent": cw * 100.0,
		"message":      message,
	})
}

// MarketWants handles GET /worlds/{worldID}/market/wants.
// Returns, per settlement the player has FOW-knowledge of (market_snapshots),
// the goods that settlement WANTS to buy (draining or empty, see
// economy.WantsDaysCover) and the goods it has a sellable SURPLUS of (built up
// past economy.ExportsDaysCover). PR1 (system-computed local price) was
// repealed 2026-08-19 — this discovery signal survives, rerooted from price
// onto the observed stock+rate market_snapshots already carries (never live
// settlement_goods — that would leak present-tense state through FOW).
// Demand/supply signal for traders and LLM agents. FOW-gated: only known settlements.
func (h *ProvinceHandler) MarketWants(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT ms.settlement_id, s.name, ms.good_key, ms.stock, ms.rate, ms.observed_at, ms.secondhand
		 FROM market_snapshots ms
		 JOIN goods g ON g.key = ms.good_key
		 JOIN settlements s ON s.id = ms.settlement_id
		 WHERE ms.player_id = $1 AND s.world_id = $2
		   AND ms.rate <= 0 AND ms.stock < $3 * GREATEST(-ms.rate, $4)
		   AND g.category <> 'sacred'
		   AND ms.good_key <> 'silver'
		 ORDER BY ms.settlement_id, ms.stock / GREATEST(-ms.rate, $4) ASC`,
		playerID, worldID, economy.WantsDaysCover, economy.MinFlowForCover,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type wantItem struct {
		Good  string  `json:"good"`
		Stock float64 `json:"stock"`
		Rate  float64 `json:"rate"`
	}
	type settlementWants struct {
		SettlementID uuid.UUID  `json:"settlement_id"`
		Name         string     `json:"name"`
		ObservedAt   time.Time  `json:"observed_at"`
		Secondhand   bool       `json:"secondhand"` // learned via a contact's gossip, not observed directly
		Goods        []wantItem `json:"goods"`
	}

	order := []uuid.UUID{}
	byID := map[uuid.UUID]*settlementWants{}

	for rows.Next() {
		var (
			settlementID  uuid.UUID
			name, goodKey string
			stock, rate   float64
			observedAt    time.Time
			secondhand    bool
		)
		if err := rows.Scan(&settlementID, &name, &goodKey, &stock, &rate, &observedAt, &secondhand); err != nil {
			continue
		}
		if _, seen := byID[settlementID]; !seen {
			byID[settlementID] = &settlementWants{
				SettlementID: settlementID,
				Name:         name,
				ObservedAt:   observedAt,
				Secondhand:   secondhand,
				Goods:        []wantItem{},
			}
			order = append(order, settlementID)
		}
		byID[settlementID].Goods = append(byID[settlementID].Goods, wantItem{
			Good:  goodKey,
			Stock: stock,
			Rate:  rate,
		})
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	wants := make([]settlementWants, 0, len(order))
	for _, id := range order {
		wants = append(wants, *byID[id])
	}

	// Surplus: producing goods with a built-up sellable stock (export candidates).
	surplusRows, err := h.pool.Query(r.Context(),
		`SELECT ms.settlement_id, s.name, ms.good_key, ms.stock, ms.rate, ms.observed_at, ms.secondhand
		 FROM market_snapshots ms
		 JOIN goods g ON g.key = ms.good_key
		 JOIN settlements s ON s.id = ms.settlement_id
		 WHERE ms.player_id = $1 AND s.world_id = $2
		   AND ms.rate > 0 AND ms.stock > $3 * ms.rate
		   AND g.category <> 'sacred'
		   AND ms.good_key <> 'silver'
		 ORDER BY ms.settlement_id, ms.stock / GREATEST(ms.rate, $4) DESC`,
		playerID, worldID, economy.ExportsDaysCover, economy.MinFlowForCover,
	)

	type surplusItem struct {
		Good  string  `json:"good"`
		Stock float64 `json:"stock"`
		Rate  float64 `json:"rate"`
	}
	type settlementSurplus struct {
		SettlementID uuid.UUID     `json:"settlement_id"`
		Name         string        `json:"name"`
		ObservedAt   time.Time     `json:"observed_at"`
		Secondhand   bool          `json:"secondhand"` // learned via a contact's gossip, not observed directly
		Goods        []surplusItem `json:"goods"`
	}

	var surplusList []settlementSurplus
	if err == nil {
		defer surplusRows.Close()
		surplusOrder := []uuid.UUID{}
		surplusByID := map[uuid.UUID]*settlementSurplus{}
		for surplusRows.Next() {
			var (
				settlementID  uuid.UUID
				name, goodKey string
				stock, rate   float64
				observedAt    time.Time
				secondhand    bool
			)
			if err := surplusRows.Scan(&settlementID, &name, &goodKey, &stock, &rate, &observedAt, &secondhand); err != nil {
				continue
			}
			if _, seen := surplusByID[settlementID]; !seen {
				surplusByID[settlementID] = &settlementSurplus{
					SettlementID: settlementID,
					Name:         name,
					ObservedAt:   observedAt,
					Secondhand:   secondhand,
					Goods:        []surplusItem{},
				}
				surplusOrder = append(surplusOrder, settlementID)
			}
			surplusByID[settlementID].Goods = append(surplusByID[settlementID].Goods, surplusItem{
				Good:  goodKey,
				Stock: stock,
				Rate:  rate,
			})
		}
		surplusList = make([]settlementSurplus, 0, len(surplusOrder))
		for _, id := range surplusOrder {
			surplusList = append(surplusList, *surplusByID[id])
		}
	}
	if surplusList == nil {
		surplusList = []settlementSurplus{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"wants": wants, "surplus": surplusList})
}
