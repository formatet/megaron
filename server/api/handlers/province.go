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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/capabilities"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/kharis"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/religion"
	"github.com/poleia/server/internal/tick"
	"github.com/poleia/server/internal/unit"
	"github.com/poleia/server/internal/unit/shipnames"
)

// ProvinceHandler handles HTTP requests for province endpoints.
type ProvinceHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	clk        clock.Clock
	sitosCfg   economy.SitosConfig
	eventStore *events.Store // may be nil in tests that don't exercise Recruit's naval path
}

// NewProvinceHandler creates a ProvinceHandler.
func NewProvinceHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, clk clock.Clock, sitosCfg economy.SitosConfig, eventStore *events.Store) *ProvinceHandler {
	return &ProvinceHandler{pool: pool, scheduler: scheduler, clk: clk, sitosCfg: sitosCfg, eventStore: eventStore}
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
		// The CLI resolver (poleia --province) now catches this client-side, but direct
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

	// Collect deposit types present in the 7 catchment tiles (own hex + 6 adjacent)
	// so clients/agents can decide whether building a mine is worthwhile.
	var catchmentDeposits []string
	cdrows, _ := h.pool.Query(r.Context(),
		`SELECT DISTINCT
		    CASE WHEN copper_deposit THEN 'copper' END,
		    CASE WHEN tin_deposit    THEN 'tin'    END,
		    CASE WHEN COALESCE(silver_deposit,false) THEN 'silver' END,
		    CASE WHEN COALESCE(cedar_deposit, false) THEN 'cedar'  END
		 FROM map_tiles
		 WHERE world_id = $1
		   AND terrain NOT IN ('deep_sea','coastal_sea')
		   AND (
		       (q = $2   AND r = $3  ) OR
		       (q = $2+1 AND r = $3  ) OR (q = $2-1 AND r = $3  ) OR
		       (q = $2   AND r = $3+1) OR (q = $2   AND r = $3-1) OR
		       (q = $2+1 AND r = $3-1) OR (q = $2-1 AND r = $3+1)
		   )`,
		worldID, prov.MapTile.Q, prov.MapTile.R,
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

		// can_afford per building: all goods costs covered.
		type buildAffordRow struct {
			Type      string `json:"type"`
			CanAfford bool   `json:"can_afford"`
		}
		var buildAfford []buildAffordRow
		for bType, spec := range province.BuildingSpecs {
			afford := silverStock >= spec.CostSilver
			if afford {
				for goodKey, needed := range spec.Costs {
					if goodsStock[goodKey] < needed {
						afford = false
						break
					}
				}
			}
			buildAfford = append(buildAfford, buildAffordRow{Type: string(bType), CanAfford: afford})
		}

		// Index completed buildings for O(1) lookup in the can_recruit loop below.
		builtTypes := make(map[string]bool, len(buildings))
		for _, b := range buildings {
			builtTypes[b.Type] = true
		}

		// can_recruit per unit: goods + labor pool + building requirements (for 1 unit).
		// Mirrors the actual Recruit handler gates so can_recruit:false is trustworthy.
		type recruitAffordRow struct {
			Unit       string `json:"unit"`
			CanRecruit bool   `json:"can_recruit"`
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
			recruitAfford = append(recruitAfford, recruitAffordRow{Unit: unitType, CanRecruit: afford})
		}

		// Live kharis pool (per-Wanax, on player_world_records) — NOT the stale
		// settlement-level resources.kharis (≈0 since kharis moved to the pool).
		// The oracle/rite tier-gate (MinKharis) reads this, so the agent must see
		// it to know which prayers it can actually cast.
		var kharisNow, kharisRate float64
		if sett.OwnerID != nil {
			if k, kerr := loadPlayerKharis(r.Context(), h.pool, *sett.OwnerID, worldID); kerr == nil {
				kharisNow, kharisRate = k.Amount, k.Rate
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
		// cooldown_remaining_minutes is >0 when the prayer is on cooldown.
		// Effect (Plan A / A7, megaron_kult_legibilitet_plan.md) surfaces spec.Description
		// so a Wanax knows what a prayer does before casting it. DESIGN INVARIANT
		// (Timothy 2026-07-11, HARD): never add a success_chance/odds field here — the
		// gods are not machines. Gynnsamhet is read via the settlement's kharis_mood
		// sibling field (kharisToMood, web.go), not a computed percentage.
		type prayerRow struct {
			ID                    string             `json:"id"`
			Name                  string             `json:"name"`
			God                   string             `json:"god"`
			EffectType            string             `json:"effect_type"`
			Effect                string             `json:"effect"`
			MinKharis             float64            `json:"min_kharis"`
			Offering              map[string]float64 `json:"offering"`
			Affordable            bool               `json:"affordable"`
			CooldownRemainingMins float64            `json:"cooldown_remaining_minutes,omitempty"`
		}
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
			var cooldownRemainingMins float64
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
						cooldownRemainingMins = remaining.Minutes()
						afford = false
					}
				}
			}
			prayers = append(prayers, prayerRow{
				ID: spec.ID, Name: spec.Name, God: spec.God, EffectType: spec.EffectType,
				Effect:    spec.Description,
				MinKharis: spec.MinKharis, Offering: spec.Offering, Affordable: afford,
				CooldownRemainingMins: cooldownRemainingMins,
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

		// Sitos-fonden surface (always visible): the fund's silver + smoothed grain
		// reference price. Rate shown is the tax leg's "up to" amount per tick (the
		// applied tax is additionally silver-gated).
		var currentTick int
		_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&currentTick)
		var sitosFundSilver float64
		_ = h.pool.QueryRow(r.Context(),
			`SELECT GREATEST(0, sitos_fund_silver) FROM settlements WHERE id = $1`, sett.ID,
		).Scan(&sitosFundSilver)
		var grainBaseValue, grainAmount, grainRate, grainCap float64
		var grainCalcTick int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT g.base_value, sg.amount, sg.rate, sg.cap, sg.calc_tick
			 FROM settlement_goods sg JOIN goods g ON g.key = sg.good_key
			 WHERE sg.settlement_id = $1 AND sg.good_key = 'grain'`,
			sett.ID,
		).Scan(&grainBaseValue, &grainAmount, &grainRate, &grainCap, &grainCalcTick)
		sitosFundCap := economy.FundCap(sett.Population, grainBaseValue, h.sitosCfg)
		refPriceGrain := economy.RefPrice(grainBaseValue, grainAmount, grainRate,
			float64(grainCalcTick), currentTick, h.sitosCfg)
		taxRatePerTick := float64(sett.Population) * h.sitosCfg.TaxRate / float64(events.TicksPerDay)

		// Grain-netto-märkning (DEL C, megaron_ekonomi_legibilitet_plan.md): the
		// stored grain rate is already NET (production − consumption, folded in
		// RecomputeProduction) — reconstruct the two components from the same
		// consumption formula rather than re-running a recompute, so `status` can
		// show "prod X − konsum Y = netto Z" instead of one unmarked number.
		grainConsumRate := float64(laborPool) * economy.GrainConsumptionPerCitizenPerDay / float64(events.TicksPerDay)
		grainProdRate := grainRate + grainConsumRate

		// Break-even grain labor-weight for this settlement's catchment
		// (pop-independent — see DEL C step 4): the minimum grain weight that
		// keeps a citizen fed. basePot_grain comes from the same catchment-query
		// path RecomputeProduction uses (economy.CatchmentBasePotential) so it
		// never drifts from the real production formula. nil when the catchment
		// can't produce grain at all (no plains/farm tile) — no weight helps.
		var breakevenGrainWeight *float64
		if basePots, bperr := economy.CatchmentBasePotential(r.Context(), h.pool, sett.ID); bperr == nil {
			if basePotGrain := basePots["grain"]; basePotGrain > 0 {
				be := economy.GrainConsumptionPerCitizenPerDay * economy.REF_LABOR /
					(basePotGrain * float64(events.TicksPerDay))
				breakevenGrainWeight = &be
			}
		}

		// "Senaste tick" summary (Fas 2 point 8): derive prod/cons from the same
		// per-tick rates already in resSnap, and sum this tick's Sitos silver delta
		// from the events log. Summarizes the journal without replacing it.
		lastTickProd := map[string]float64{}
		lastTickCons := map[string]float64{}
		for k, rd := range resSnap {
			if rd.Rate > 0 {
				lastTickProd[k] = rd.Rate
			} else if rd.Rate < 0 {
				lastTickCons[k] = -rd.Rate
			}
		}
		// DEL A Sitos-delta-itemisering (megaron_ekonomi_legibilitet_plan.md):
		// beyond the net silver delta, tally the grain-moving legs separately so
		// `status` can say WHAT Sitos did for this settlement this tick, not just
		// the silver blob. "sell" = rescue leg (fund sells grain to the city:
		// GoodDelta positive = grain arrived, SilverDelta positive = city paid
		// the fund). "buy" = surplus-absorption leg (fund buys the city's excess
		// grain: GoodDelta negative = grain left, SilverDelta negative = fund
		// paid the city). "tax" legs have GoodDelta == 0 (silver-only, routine)
		// and are already folded into lastTickSitosDelta — not itemized here.
		var lastTickSitosDelta float64
		var sitosInterventions int
		var sitosGrainIn, sitosGrainOut, sitosSilverIn, sitosSilverOut float64
		if lrows, lerr := h.pool.Query(r.Context(),
			`SELECT payload FROM events
			 WHERE stream_id = $1 AND world_tick = $2 AND event_type = 'SitosTransaction'`,
			sett.ID, currentTick,
		); lerr == nil {
			for lrows.Next() {
				var pl []byte
				if lrows.Scan(&pl) == nil {
					var p economy.SitosTransactionPayload
					if json.Unmarshal(pl, &p) == nil {
						lastTickSitosDelta += p.SilverDelta
						switch p.Kind {
						case "sell":
							sitosInterventions++
							sitosGrainIn += p.GoodDelta
							sitosSilverIn += p.SilverDelta
						case "buy":
							sitosInterventions++
							sitosGrainOut += -p.GoodDelta
							sitosSilverOut += -p.SilverDelta
						}
					}
				}
			}
			lrows.Close()
		}

		armyUp, _, upErr := armyUpkeep(r.Context(), h.pool, sett.ID)
		if upErr != nil {
			writeError(w, http.StatusInternalServerError, "could not compute army upkeep")
			return
		}

		// Settlement cap: same "how many colonies do I hold vs. the per-Wanax
		// ceiling" figure `poleia actions` derives for the colonize gate
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

		resp["settlement"] = map[string]any{
			"id":                     sett.ID,
			"name":                   sett.Name,
			"owner_id":               sett.OwnerID,
			"kingdom_id":             sett.KingdomID,
			"culture":                sett.CultureID,
			"state":                  sett.State,
			"population":             sett.Population,
			"labor_pool":             laborPool,
			"walls":                  sett.WallLevel,
			"loyalty":                sett.Loyalty,
			"resources":              resSnap,
			"kharis":                 kharisNow,
			"kharis_rate":            kharisRate,
			"kharis_mood":            kharisToMood(kharisNow),
			"kharis_per_day":         kharisRate * float64(events.TicksPerDay),
			"temple_offers":          templeOffers,
			"grain_prod_rate":        grainProdRate,
			"grain_consum_rate":      grainConsumRate,
			"breakeven_grain_weight": breakevenGrainWeight,
			"army":                   sett.Army,
			"army_upkeep":            armyUp,
			"build_queue":            buildQueue,
			"training_units":         trainingUnits,
			"buildings":              buildings,
			"can_afford":             buildAfford,
			"can_recruit":            recruitAfford,
			"available_prayers":      prayers,
			"settlement_cap": map[string]any{
				"used": settlementsOwned,
				"max":  province.MaxSettlementsPerWanax,
			},
			"sitos": map[string]any{
				"fund_silver":        sitosFundSilver,
				"fund_cap":           sitosFundCap,
				"fund_rate_per_tick": taxRatePerTick,
				"ref_price_grain":    refPriceGrain,
				"ref_price_floor":    h.sitosCfg.RefPriceFloor,
				"ref_price_ceiling":  h.sitosCfg.RefPriceCeiling,
			},
			"last_tick": map[string]any{
				"tick":                currentTick,
				"production":          lastTickProd,
				"consumption":         lastTickCons,
				"sitos_delta":         lastTickSitosDelta,
				"sitos_interventions": sitosInterventions,
				"sitos_grain_in":      sitosGrainIn,
				"sitos_grain_out":     sitosGrainOut,
				"sitos_silver_in":     sitosSilverIn,
				"sitos_silver_out":    sitosSilverOut,
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

// armyUpkeep sums the per-period upkeep of a settlement's garrison from the units
// table (the SB7 source of truth), via combat.UnitUpkeep so the shown cost always
// matches what the daily upkeep tick actually debits. Returns the total plus a
// per-unit-type breakdown.
func armyUpkeep(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) (upkeepAmount, map[string]upkeepAmount, error) {
	total := upkeepAmount{}
	perType := map[string]upkeepAmount{}
	rows, err := pool.Query(ctx,
		`SELECT type, category, size FROM units
		 WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	)
	if err != nil {
		return total, perType, err
	}
	defer rows.Close()
	for rows.Next() {
		var unitType, category string
		var size int
		if err := rows.Scan(&unitType, &category, &size); err != nil {
			return total, perType, err
		}
		up := combat.UnitUpkeep(unitType, category, size)
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

	// Harbour requires the settlement to be adjacent to a sea hex (coast is a property, not a terrain).
	if req.BuildingType == "harbour" {
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
			     AND terrain IN ('coastal_sea','deep_sea')
			 )`,
			worldID, pq, pr,
		).Scan(&coastNeighbour)
		if !coastNeighbour {
			writeError(w, http.StatusUnprocessableEntity, "harbour requires a coastal or sea tile on an adjacent hex")
			return
		}
	}

	// Mines require the matching ore deposit in the 7-tile catchment (own hex + 6
	// adjacent) — production reads only the catchment, so a mine without a matching
	// deposit in reach would produce nothing. Gate it at build time.
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
		var hasDeposit bool
		_ = h.pool.QueryRow(r.Context(),
			fmt.Sprintf(`SELECT EXISTS(
			   SELECT 1 FROM map_tiles
			   WHERE world_id = $1
			     AND terrain NOT IN ('coastal_sea','deep_sea')
			     AND (q, r) IN (
			       ($2,$3),
			       ($2+1,$3), ($2-1,$3),
			       ($2,$3+1), ($2,$3-1),
			       ($2+1,$3-1), ($2-1,$3+1)
			     )
			     AND %s
			 )`, depositCond),
			worldID, pq, pr,
		).Scan(&hasDeposit)
		if !hasDeposit {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("a %s here would produce nothing — no %s deposit in this settlement's catchment (its own hex or the 6 surrounding hexes). Build it on or next to the ore.", req.BuildingType, oreName))
			return
		}
	}

	// Queue guards — block before we deduct resources.
	// 1. Walls/towers/bronze walls upgrade an existing wall_level; everything else
	//    is a one-instance building (production_rules use UPSERT, duplicates waste resources).
	// 2. No double-queueing the same building.
	// 3. Cap concurrent queue at maxParallelBuilds.
	const maxParallelBuilds = 2
	upgradeable := map[string]bool{"wall": true}

	if !upgradeable[req.BuildingType] {
		var alreadyBuilt bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = $2)`,
			settlementID, req.BuildingType,
		).Scan(&alreadyBuilt)
		if alreadyBuilt {
			writeError(w, http.StatusUnprocessableEntity, "building already exists")
			return
		}
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
			writeError(w, http.StatusUnprocessableEntity, "insufficient silver")
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
// all constructable buildings: costs, duration, gate (requires_coastal, requires_deposit)
// joined from production_rules, and a human purpose string.
// No world auth required — this is static reference data.
func (h *ProvinceHandler) BuildingCatalogue(w http.ResponseWriter, r *http.Request) {
	// Load requires_coastal and requires_deposit per building_type from production_rules.
	// A building may have multiple rules; we collect all deposit requirements and
	// collapse coastal to a single bool (any rule requiring coastal → true).
	type gateInfo struct {
		requiresCoastal  bool
		requiresDeposits []string
	}
	gates := map[string]*gateInfo{}

	rows, err := h.pool.Query(r.Context(),
		`SELECT DISTINCT building_type, requires_coastal, requires_deposit
		 FROM production_rules
		 WHERE building_type IS NOT NULL`,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var bt string
			var coastal bool
			var deposit *string
			if err := rows.Scan(&bt, &coastal, &deposit); err != nil {
				continue
			}
			if _, ok := gates[bt]; !ok {
				gates[bt] = &gateInfo{}
			}
			if coastal {
				gates[bt].requiresCoastal = true
			}
			if deposit != nil && *deposit != "" {
				gates[bt].requiresDeposits = append(gates[bt].requiresDeposits, *deposit)
			}
		}
	}

	type buildingEntry struct {
		Type             string             `json:"type"`
		Costs            map[string]float64 `json:"costs"`
		CostSilver       float64            `json:"cost_silver,omitempty"`
		DurationMinutes  float64            `json:"duration_minutes"`
		RequiresCoastal  bool               `json:"requires_coastal,omitempty"`
		RequiresDeposits []string           `json:"requires_deposits,omitempty"`
		Purpose          string             `json:"purpose"`
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
			Type:            bt,
			Costs:           spec.Costs,
			CostSilver:      spec.CostSilver,
			DurationMinutes: float64(spec.DurationTicks * tick.TickMinutes),
			Purpose:         province.BuildingPurposes[province.BuildingType(bt)],
		}
		if g, ok := gates[bt]; ok {
			entry.RequiresCoastal = g.requiresCoastal
			if len(g.requiresDeposits) > 0 {
				entry.RequiresDeposits = g.requiresDeposits
			}
		}
		result = append(result, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

// UnitCatalogue handles GET /api/v1/units — returns the static catalogue of all
// recruitable unit types: the resource cost for one recruit action (a 10-man
// batch for land units, one vessel for naval — the same quantities `recruitPerManCosts`
// scales up to deduct), the population-pool gate, training duration, and the
// building requirement. No world/auth required — static reference data,
// mirrors BuildingCatalogue.
func (h *ProvinceHandler) UnitCatalogue(w http.ResponseWriter, r *http.Request) {
	type unitEntry struct {
		Type             string             `json:"type"`
		Costs            map[string]float64 `json:"costs"`
		BatchMen         int                `json:"batch_men"` // men (land) or crew (naval) the Costs above pay for in one recruit call
		PopCost          int                `json:"pop_cost"`
		DurationMinutes  float64            `json:"duration_minutes"`
		RequiresBarracks bool               `json:"requires_barracks,omitempty"`
		RequiresStable   bool               `json:"requires_stable,omitempty"`
		RequiresHarbour  bool               `json:"requires_harbour,omitempty"`
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
		batchMen := 10
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
			DurationMinutes:  float64(spec.DurationTicks * tick.TickMinutes),
			RequiresBarracks: spec.RequiresBarracks,
			RequiresStable:   spec.RequiresStable,
			RequiresHarbour:  spec.RequiresHarbour,
			RequiresFoundry:  spec.RequiresFoundry,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// recruitPerManCosts returns the resource cost per man for a given unit type.
// Derived from Skalbeslut (2026-06-15): per-man = old UnitSpec cost / old PopCost.
// Batch = 10 men → total cost = per-man × 10. All siffror are tunable at reseed.
// recruitPerManCosts delegates to province.UnitSpecs so this handler and the
// capabilities recruit checker (poleia actions) read the exact same per-man
// cost table — before Fas 3 they were two separately hand-maintained copies
// that had already drifted apart (temenos_capabilities.md Fas 3). Note:
// "priest" is not a recruitable unit (removed mig 060) and is caught earlier
// by the UnitSpecs lookup in Recruit, so it never reaches this function.
func recruitPerManCosts(unitType string) map[string]float64 {
	spec, ok := province.UnitSpecs[unitType]
	if !ok {
		return nil
	}
	return spec.Costs
}

// recruitBatchTicks returns the training ticks for one batch of 10 men.
func recruitBatchTicks(unitType string) int {
	spec, ok := province.UnitSpecs[unitType]
	if !ok {
		return 1
	}
	return spec.DurationTicks
}

// Recruit handles POST /worlds/:worldID/provinces/:provinceID/recruit.
//
// C2 semantics: soldiers are drafted from the population in batches of 10 men.
// Request: {"unit_type": "spearman", "men": 30}  (men must be a multiple of 10, max 100).
// Population is decremented immediately; resources are deducted up-front.
// A forming unit is created (or grown) in the units table; one TrainComplete is
// scheduled per batch-of-10. At size == 100 the unit becomes deployable (garrison).
// Naval units (galley/war_galley/merchantman) skip the 100-forming gate: they are
// deployable as soon as their crew is drafted (one vessel = one unit, size always 1).
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
	// Land units keep the existing 10–100-men batch gate. `count` (1–20) still
	// lets a Wanax queue several vessels in one call, but the default (count
	// omitted) and the Web UI both build exactly one ship.
	uType := unit.Type(req.UnitType)
	cat := unit.CategoryOf(uType)
	if cat == unit.CategoryLand {
		if req.Men <= 0 || req.Men > 100 {
			writeError(w, http.StatusBadRequest, "men must be 1–100")
			return
		}
		if req.Men%10 != 0 {
			writeError(w, http.StatusBadRequest, "men must be a multiple of 10")
			return
		}
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

	// Coarse precondition — the same checker `poleia actions` uses
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
	if spec.RequiresFoundry {
		var exists bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = 'foundry')`,
			settlementID,
		).Scan(&exists)
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "foundry required")
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
			writeError(w, http.StatusUnprocessableEntity, "insufficient kharis")
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

			var unitID uuid.UUID
			if err := h.pool.QueryRow(r.Context(),
				`INSERT INTO units
				   (world_id, owner_id, type, category, size, crew, status, settlement_id, name, build_complete_at)
				 VALUES ($1, $2, $3, $4, 1, $5, 'forming', $6, $7, $8)
				 RETURNING id`,
				worldID, playerID, string(uType), string(cat),
				crew, settlementID, chosenName, completeAt,
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
		if unitSize > 100 {
			unitSize = 100 // cap; the remainder spills into a new forming unit below
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
			if err := h.pool.QueryRow(r.Context(),
				`INSERT INTO units
				   (world_id, owner_id, type, category, size, crew, status, settlement_id)
				 VALUES ($1, $2, $3, $4, $5, $6, 'forming', $7)
				 RETURNING id`,
				worldID, playerID, string(uType), string(cat),
				unitSize, crew, settlementID,
			).Scan(&unitID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not create forming unit")
				return
			}
		}
		unitIDs = append(unitIDs, unitID)
		finalSize = unitSize

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
				if _, err := h.pool.Exec(r.Context(),
					`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
					 VALUES ($1, $2, $3, $4, $5, $6, 'forming', $7)`,
					worldID, playerID, string(uType), string(cat), newSize-100, crew, settlementID,
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
	writeJSON(w, http.StatusCreated, map[string]any{
		"unit_id":      unitIDs[len(unitIDs)-1],
		"unit_ids":     unitIDs,
		"unit_type":    req.UnitType,
		"men":          menForResponse,
		"count":        effectiveCount,
		"batches":      batches,
		"complete_at":  lastCompleteAt,
		"forming_size": finalSize,
		"names":        unitNames,
	})
}

// Goods handles GET /worlds/:worldID/provinces/:provinceID/goods.
// Returns the settlement's goods inventory with lazy-eval amounts and local prices.
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
	// (same logic as RecomputeProduction — 7 catchment map_tiles: own hex + 6 adjacent).
	baseRows, _ := h.pool.Query(r.Context(),
		`SELECT pr.good_key, SUM(pr.rate_per_tick) AS base_potential
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 JOIN map_tiles mt ON mt.world_id = s.world_id
		     AND mt.terrain NOT IN ('deep_sea','coastal_sea')
		     AND (
		         (mt.q = prov.map_q   AND mt.r = prov.map_r  ) OR
		         (mt.q = prov.map_q+1 AND mt.r = prov.map_r  ) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r  ) OR
		         (mt.q = prov.map_q   AND mt.r = prov.map_r+1) OR (mt.q = prov.map_q   AND mt.r = prov.map_r-1) OR
		         (mt.q = prov.map_q+1 AND mt.r = prov.map_r-1) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r+1)
		     )
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit,false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 WHERE s.id = $1
		 GROUP BY pr.good_key`,
		settlementID,
	)
	basePotential := make(map[string]float64)
	if baseRows != nil {
		for baseRows.Next() {
			var k string
			var v float64
			_ = baseRows.Scan(&k, &v)
			basePotential[k] = v
		}
		baseRows.Close()
	}

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

	// Compute idle citizens from unallocated weight fraction.
	var totalWeight float64
	for _, w := range laborWeights {
		totalWeight += w
	}
	idleCitizens := int(math.Round((1.0 - totalWeight) * float64(laborPool)))
	if idleCitizens < 0 {
		idleCitizens = 0
	}

	type goodRow struct {
		Key            string  `json:"key"`
		Name           string  `json:"name"`
		Tier           string  `json:"tier"`
		Category       string  `json:"category"`
		Amount         float64 `json:"amount"`
		Rate           float64 `json:"rate_per_tick"`
		Cap            float64 `json:"cap"`
		Price          float64 `json:"price"`
		Citizens       int     `json:"citizens"`
		Percent        float64 `json:"percent"`
		YieldPerWorker float64 `json:"yield_per_worker"`
		Producible     bool    `json:"producible"`
		LaborPool      int     `json:"labor_pool"`
		IdleCitizens   int     `json:"idle_citizens"`
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
		result = append(result, goodRow{
			Key:            key,
			Name:           name,
			Tier:           tier,
			Category:       category,
			Amount:         current,
			Rate:           rate,
			Cap:            capV,
			Price:          economy.LocalPrice(baseValue, current, rate),
			Citizens:       int(math.Round(laborWeights[key] * float64(laborPool))),
			Percent:        laborWeights[key] * 100.0,
			YieldPerWorker: bp / economy.REF_LABOR,
			Producible:     bp > 0,
			LaborPool:      laborPool,
			IdleCitizens:   idleCitizens,
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
	production := map[string]float64{}
	consumption := map[string]float64{}
	if grows, gerr := h.pool.Query(r.Context(),
		`SELECT good_key, rate FROM settlement_goods WHERE settlement_id = $1`, sett.ID,
	); gerr == nil {
		for grows.Next() {
			var k string
			var rt float64
			if grows.Scan(&k, &rt) == nil {
				if rt > 0 {
					production[k] = rt
				} else if rt < 0 {
					consumption[k] = -rt
				}
			}
		}
		grows.Close()
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

	var weight float64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(weight, 2) FROM goods WHERE key = $1`,
		req.GoodKey,
	).Scan(&weight); err != nil {
		writeError(w, http.StatusBadRequest, "unknown good")
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

	// Internal transfer: no loss, no gain — delivered quantity equals what was sent.
	// Enqueue delivery within the same transaction — atomic with the deduction.
	if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
		map[string]any{
			"trade_route_id":     routeID,
			"destination_id":     req.DestinationID,
			"good_key":           req.GoodKey,
			"quantity":           req.Quantity,
			"delivered_quantity": req.Quantity,
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

// Craft handles POST /worlds/:worldID/provinces/:provinceID/craft.
// Body: { "recipe_id": 1, "quantity": 1.0 }
func (h *ProvinceHandler) Craft(w http.ResponseWriter, r *http.Request) {
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
		RecipeID int     `json:"recipe_id"`
		Quantity float64 `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "recipe_id and quantity > 0 required")
		return
	}

	// Find settlement and verify ownership.
	var settlementID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT s.id FROM settlements s
		 WHERE s.province_id = $1 AND s.world_id = $2 AND s.owner_id = $3`,
		provinceID, worldID, playerID,
	).Scan(&settlementID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Load recipe.
	var outputKey, buildingType string
	var outputQty float64
	err = h.pool.QueryRow(r.Context(),
		`SELECT output_key, output_qty, building_type FROM recipes WHERE id = $1`,
		req.RecipeID,
	).Scan(&outputKey, &outputQty, &buildingType)
	if err != nil {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}

	if req.RecipeID == 1 {
		// The load-bearing bronze recipe has a capabilities checker
		// (temenos_capabilities.md) — reuse it here so this 422 and
		// `poleia actions` can never show a different requirement for the
		// same gate (Fas 3 anti-drift). Covers both the foundry-presence
		// gate and "you don't hold enough of an ingredient to craft even
		// one unit" — sound as a precondition for ANY requested quantity,
		// since a smaller stock than one unit's worth cannot satisfy a
		// larger batch either.
		cc := capabilities.NewContext(r.Context(), h.pool, h.clk, worldID, provinceID, playerID, settlementID)
		if v := capabilities.CanCraft(cc); !v.Available {
			writeError(w, http.StatusUnprocessableEntity, capabilities.FirstUnsatisfied(v))
			return
		}
	} else {
		// Other recipes (e.g. luxury goods) have no capabilities checker yet —
		// fall back to the generic building-presence gate.
		var hasBuilding bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = $2)`,
			settlementID, buildingType,
		).Scan(&hasBuilding)
		if !hasBuilding {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s required", buildingType))
			return
		}
	}

	// Load recipe ingredients.
	ingRows, err := h.pool.Query(r.Context(),
		`SELECT good_key, quantity FROM recipe_ingredients WHERE recipe_id = $1`,
		req.RecipeID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load recipe")
		return
	}
	defer ingRows.Close()

	type ing struct {
		key string
		qty float64
	}
	var ingredients []ing
	for ingRows.Next() {
		var i ing
		if err := ingRows.Scan(&i.key, &i.qty); err == nil {
			ingredients = append(ingredients, i)
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Deduct each ingredient.
	for _, i := range ingredients {
		needed := i.qty * req.Quantity
		tag, err := tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			     amount = settled(amount, rate, calc_tick) - $1,
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = $3
			   AND settled(amount, rate, calc_tick) >= $1`,
			needed, settlementID, i.key,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not deduct ingredient")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("insufficient %s", i.key))
			return
		}
	}

	// Credit output. cap = 1_000_000: the non-binding technical ceiling shared
	// by all goods rows since the 2026-07-05 cap loosening (mirrors
	// economy.goodCap / combat.goodCap — each package hard-mirrors the value).
	// The old hard-coded 100 here, plus the genesis seed's ELSE=200 cap in
	// join.go, pinned every craft output (bronze, luxury, pottery, purple) at a
	// binding low ceiling — bronze could never be stored >200. Set cap via
	// EXCLUDED so an existing genesis-placeholder row is lifted on first craft,
	// and clamp to that same ceiling (pattern mirrors combat/arrival.go).
	produced := outputQty * req.Quantity
	_, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 1000000, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
		             + $3,
		         EXCLUDED.cap),
		     cap = EXCLUDED.cap,
		     calc_tick = current_world_tick()`,
		settlementID, outputKey, produced,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not credit output")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"output_key": outputKey,
		"produced":   produced,
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
		`SELECT id, target_id, intent, infantry, chariot, priest, ship, elite_infantry,
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
		Priest        int       `json:"priest"`
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
			&m.Spearman, &m.WarChariot, &m.Priest, &m.Ship, &m.EliteInfantry,
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
		Priest        int
		Ship          int
		EliteInfantry int
		WarGalley     int
		Merchantman   int
		Resolved      bool
		OriginID      uuid.UUID
		TargetID      uuid.UUID
	}
	err = tx.QueryRow(r.Context(),
		`SELECT infantry, chariot, priest, ship, elite_infantry,
		        war_galley, merchantman, resolved, origin_id, target_id
		 FROM marching_armies
		 WHERE id = $1 AND world_id = $2 AND origin_id = $3
		 FOR UPDATE`,
		marchID, worldID, provinceID,
	).Scan(&march.Spearman, &march.WarChariot, &march.Priest,
		&march.Ship, &march.EliteInfantry, &march.WarGalley, &march.Merchantman,
		&march.Resolved, &march.OriginID, &march.TargetID)
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
	// does the army turn around. If the army arrives and fights first, the recall simply misses.

	// Hex positions of origin (home) and target (where the army is heading).
	var oQ, oR, tQ, tR int
	_ = tx.QueryRow(r.Context(), `SELECT map_q, map_r FROM provinces WHERE id = $1`, march.OriginID).Scan(&oQ, &oR)
	_ = tx.QueryRow(r.Context(), `SELECT map_q, map_r FROM provinces WHERE id = $1`, march.TargetID).Scan(&tQ, &tR)

	// Dispatch a visible recall messenger toward the army's target province.
	// Assumption: it aims at the destination, not the army's mid-march position — no interpolation,
	// always physically safe (never faster than physics).
	dist := province.HexDistance(province.MapPosition{Q: oQ, R: oR}, province.MapPosition{Q: tQ, R: tR})
	messengerArrivesAt := h.clk.Now().Add(messenger.MessengerTravelDuration(dist))

	var marchRecallCurrentTick int
	_ = tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&marchRecallCurrentTick)
	marchRecallDueTick := marchRecallCurrentTick + messenger.MessengerTravelTicks(dist)

	var messengerID uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,$4,'outbound','recall',$5,$6,$7,$8,$9)
		 RETURNING id`,
		worldID, playerID, originSettlementID, "Recall order — return home.",
		oQ, oR, tQ, tR, messengerArrivesAt,
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
		Priest:        march.Priest,
		Ship:          march.Ship,
		EliteInfantry: march.EliteInfantry,
		WarGalley:     march.WarGalley,
		Merchantman:   march.Merchantman,
		OriginQ:       oQ,
		OriginR:       oR,
		TargetQ:       tQ,
		TargetR:       tR,
		OriginID:      march.OriginID,
		TargetID:      march.TargetID,
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
		"recalled":             true,
		"messenger_id":         messengerID,
		"messenger_arrives_at": messengerArrivesAt,
		"messenger_distance":   dist,
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
		Priest        int `json:"priest"`
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

	// priest is no longer a unit (removed mig 060), so it is never disbandable.
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

	disbanded := map[string]int{"priest": 0}
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
// Body: {"percent":{"timber":40,"grain":30,"silver":20}}
// Each value is the share of the population assigned to that good (0–100).
// Σ percent must not exceed 100; non-producible goods are rejected with a 422.
// Stored as weight = percent/100, so production auto-scales with population.
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
		writeError(w, http.StatusBadRequest, "invalid JSON — expected {\"percent\":{\"silver\":25,...}} (each value = % of population, Σ ≤ 100)")
		return
	}

	// Verify ownership and load labor_pool.
	// Part B: labor_pool = population (soldiers drawn from pop at recruit time).
	var settlementID uuid.UUID
	var population int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT s.id, s.population FROM settlements s WHERE s.province_id=$1 AND s.world_id=$2 AND s.owner_id=$3`,
		provinceID, worldID, playerID,
	).Scan(&settlementID, &population); err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	laborPool := population
	if laborPool < 0 {
		laborPool = 0
	}

	// Determine producible goods for this settlement using catchment tiles
	// (same logic as RecomputeProduction — 7 catchment map_tiles: own hex + 6 adjacent).
	producible := make(map[string]bool)
	prows, err := h.pool.Query(r.Context(),
		`SELECT DISTINCT pr.good_key
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 JOIN map_tiles mt ON mt.world_id = s.world_id
		     AND mt.terrain NOT IN ('deep_sea','coastal_sea')
		     AND (
		         (mt.q = prov.map_q   AND mt.r = prov.map_r  ) OR
		         (mt.q = prov.map_q+1 AND mt.r = prov.map_r  ) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r  ) OR
		         (mt.q = prov.map_q   AND mt.r = prov.map_r+1) OR (mt.q = prov.map_q   AND mt.r = prov.map_r-1) OR
		         (mt.q = prov.map_q+1 AND mt.r = prov.map_r-1) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r+1)
		     )
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit,  false)))
		 WHERE s.id = $1`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load producible goods")
		return
	}
	var producibleKeys []string
	for prows.Next() {
		var key string
		_ = prows.Scan(&key)
		producible[key] = true
		producibleKeys = append(producibleKeys, key)
	}
	prows.Close()
	sort.Strings(producibleKeys)

	// Validate: only producible goods, percent ∈ [0,100]; Σ ≤ 100.
	// Each value is a share of the population; weight = percent/100 is stored
	// directly so production auto-scales as population grows or shrinks.
	totalPct := 0.0
	filtered := make(map[string]float64)
	for key, pct := range req.Percent {
		if pct < 0 || pct > 100 {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("percent for %s must be between 0 and 100", key))
			return
		}
		if !producible[key] {
			hint := ""
			switch key {
			case "copper":
				hint = " (requires mine + hills catchment tile with copper deposit)"
			case "tin":
				hint = " (requires mine + mountain_limestone catchment tile with tin deposit)"
			case "silver":
				hint = " (requires silver_mine + catchment tile with silver deposit)"
			}
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("%s is not producible at this settlement%s — producible here: %s",
					key, hint, strings.Join(producibleKeys, ", ")))
			return
		}
		if pct > 0 {
			filtered[key] = pct
			totalPct += pct
		}
	}
	if totalPct == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no valid producible goods in percent")
		return
	}
	if totalPct > 100.0001 {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("labor over-allocated: percentages sum to %.1f%% but must not exceed 100%% — lower one or more shares",
				totalPct))
		return
	}

	// Break-even grain labor-weight guardrail (DEL D, megaron_ekonomi_legibilitet_plan.md).
	// Sparta-forensiken 2026-07-12: a re-allocation dropping grain below its break-even
	// weight silently starved the capital, and this DELETE+INSERT was entirely unaudited.
	// Build the weight map now, and compute the pop-independent break-even from the SAME
	// settlement-scoped catchment helper the status endpoint uses (economy.CatchmentBasePotential
	// — never a second formula), so the audit event and the warning read identical values.
	weights := make(map[string]float64, len(filtered))
	for key, pct := range filtered {
		weights[key] = pct / 100.0
	}
	var breakevenGrainWeight *float64
	if basePots, bperr := economy.CatchmentBasePotential(r.Context(), h.pool, settlementID); bperr == nil {
		if basePotGrain := basePots["grain"]; basePotGrain > 0 {
			be := economy.GrainConsumptionPerCitizenPerDay * economy.REF_LABOR /
				(basePotGrain * float64(events.TicksPerDay))
			breakevenGrainWeight = &be
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Clear existing allocations then upsert new ones as weights.
	// weight = percent/100 = fraction of population; production auto-scales with pop.
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM settlement_labor WHERE settlement_id = $1`, settlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear labor")
		return
	}
	for key, pct := range filtered {
		wt := pct / 100.0
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO settlement_labor (settlement_id, good_key, weight)
			 VALUES ($1,$2,$3) ON CONFLICT (settlement_id, good_key) DO UPDATE SET weight=$3`,
			settlementID, key, wt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save labor weight")
			return
		}
	}

	// Server-floor: always ensure a baseline cult weight (0.15) so temples are
	// never inert regardless of agent/Wanax allocation choices. This satisfies
	// the "starter city self-sufficient" and "kharis 5% floor always" invariants.
	// Cult weight is additive (does not compete with grain workers — 15% of pop
	// serves the temple alongside other duties), so grain self-sufficiency is
	// unaffected. Only applied when the settlement has a temple; no-op otherwise
	// (ON CONFLICT DO NOTHING skips the insert if agent already allocated cult ≥ 0.15).
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO settlement_labor (settlement_id, good_key, weight)
		 SELECT $1, 'cult', 0.15
		 WHERE EXISTS (SELECT 1 FROM buildings b WHERE b.settlement_id = $1 AND b.building_type = 'temple')
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		settlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not apply cult labor floor")
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

	// Audit the (previously silent) re-allocation + break-even so a future forensic
	// can attribute a starvation collapse to the labour lever that caused it. stream =
	// settlement (StreamProvince, keyed by settlementID — the settlement's own stream).
	if h.eventStore != nil {
		auditPayload := map[string]any{"weights": weights}
		if breakevenGrainWeight != nil {
			auditPayload["breakeven_grain_weight"] = *breakevenGrainWeight
		}
		_, _ = h.eventStore.Append(r.Context(), settlementID, events.StreamProvince, "LaborAllocated",
			auditPayload, worldID, nil)
	}

	// Guardrail: accept the allocation regardless (the Wanax's freedom), but if the
	// grain weight is below break-even, return a warning — the city will slowly starve
	// at this weight. Keryx renders it AFTER confirming the allocation.
	var warning string
	if breakevenGrainWeight != nil {
		grainWeight := weights["grain"] // 0 if grain not allocated at all
		if grainWeight < *breakevenGrainWeight {
			warning = fmt.Sprintf("grain-vikt %.2f < break-even ~%.2f för denna catchment → staden kommer svälta vid denna allokering",
				grainWeight, *breakevenGrainWeight)
		}
	}

	// Echo both the percent levers and the resulting citizen counts: the share
	// auto-scales with population, but real output depends on the absolute number
	// of citizens (more citizens produce more, even at a lower percent).
	citizens := make(map[string]int, len(filtered))
	for key, pct := range filtered {
		citizens[key] = int(pct / 100.0 * float64(laborPool))
	}
	resp := map[string]any{
		"percent":       filtered,
		"citizens":      citizens,
		"idle_percent":  100.0 - totalPct,
		"idle_citizens": int((100.0 - totalPct) / 100.0 * float64(laborPool)),
		"labor_pool":    laborPool,
		"message":       "labor allocation updated and production recomputed",
	}
	if breakevenGrainWeight != nil {
		resp["breakeven_grain_weight"] = *breakevenGrainWeight
	}
	if warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}

// MarketWants handles GET /worlds/{worldID}/market/wants.
// Returns, per settlement the player has price-knowledge of (market_snapshots),
// the goods that settlement WANTS to buy — goods in shortage (price > base × 1.1).
// Demand signal for traders and LLM agents. FOW-gated: only known settlements.
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
		`SELECT ms.settlement_id, s.name, ms.good_key, ms.price, ms.stock, g.base_value, ms.observed_at, ms.secondhand
		 FROM market_snapshots ms
		 JOIN goods g ON g.key = ms.good_key
		 JOIN settlements s ON s.id = ms.settlement_id
		 WHERE ms.player_id = $1 AND s.world_id = $2
		   AND ms.price > g.base_value * 1.1
		   AND g.category <> 'sacred'
		   AND ms.good_key <> 'silver'
		 ORDER BY ms.settlement_id, (ms.price / g.base_value) DESC`,
		playerID, worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type wantItem struct {
		Good          string  `json:"good"`
		Price         float64 `json:"price"`
		BaseValue     float64 `json:"base_value"`
		ShortageRatio float64 `json:"shortage_ratio"`
		WantLevel     string  `json:"want_level"`
		Stock         float64 `json:"stock"`
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
			settlementID            uuid.UUID
			name, goodKey           string
			price, stock, baseValue float64
			observedAt              time.Time
			secondhand              bool
		)
		if err := rows.Scan(&settlementID, &name, &goodKey, &price, &stock, &baseValue, &observedAt, &secondhand); err != nil {
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
		ratio := price / baseValue
		level := "low"
		if ratio >= 2.0 {
			level = "high"
		} else if ratio >= 1.5 {
			level = "medium"
		}
		byID[settlementID].Goods = append(byID[settlementID].Goods, wantItem{
			Good:          goodKey,
			Price:         price,
			BaseValue:     baseValue,
			ShortageRatio: ratio,
			WantLevel:     level,
			Stock:         stock,
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

	// Surplus: goods with price < base_value * 0.9 (export candidates).
	surplusRows, err := h.pool.Query(r.Context(),
		`SELECT ms.settlement_id, s.name, ms.good_key, ms.price, ms.stock, g.base_value, ms.observed_at, ms.secondhand
		 FROM market_snapshots ms
		 JOIN goods g ON g.key = ms.good_key
		 JOIN settlements s ON s.id = ms.settlement_id
		 WHERE ms.player_id = $1 AND s.world_id = $2
		   AND ms.price < g.base_value * 0.9
		   AND g.category <> 'sacred'
		   AND ms.good_key <> 'silver'
		 ORDER BY ms.settlement_id, ms.price / g.base_value ASC`,
		playerID, worldID,
	)

	type surplusItem struct {
		Good         string  `json:"good"`
		Price        float64 `json:"price"`
		BaseValue    float64 `json:"base_value"`
		SurplusRatio float64 `json:"surplus_ratio"`
		Stock        float64 `json:"stock"`
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
				settlementID            uuid.UUID
				name, goodKey           string
				price, stock, baseValue float64
				observedAt              time.Time
				secondhand              bool
			)
			if err := surplusRows.Scan(&settlementID, &name, &goodKey, &price, &stock, &baseValue, &observedAt, &secondhand); err != nil {
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
				Good:         goodKey,
				Price:        price,
				BaseValue:    baseValue,
				SurplusRatio: price / baseValue,
				Stock:        stock,
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
