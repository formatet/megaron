package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/religion"
	"github.com/poleia/server/internal/tick"
	"github.com/poleia/server/internal/unit"
)

// ProvinceHandler handles HTTP requests for province endpoints.
type ProvinceHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
}

// NewProvinceHandler creates a ProvinceHandler.
func NewProvinceHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, clk clock.Clock) *ProvinceHandler {
	return &ProvinceHandler{pool: pool, scheduler: scheduler, clk: clk}
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
		writeError(w, http.StatusNotFound, "province not found")
		return
	}

	// Collect deposit types present in the 6 catchment tiles so clients/agents
	// can decide whether building a mine is worthwhile.
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
			Type       string    `json:"type"`
			CreatedAt  time.Time `json:"created_at"`
			CompleteAt time.Time `json:"complete_at"`
		}
		var buildQueue []buildItem
		qrows, _ := h.pool.Query(r.Context(),
			`SELECT building_type, created_at, complete_at FROM build_queue
			 WHERE settlement_id = $1 ORDER BY complete_at`,
			sett.ID,
		)
		if qrows != nil {
			for qrows.Next() {
				var bi buildItem
				_ = qrows.Scan(&bi.Type, &bi.CreatedAt, &bi.CompleteAt)
				buildQueue = append(buildQueue, bi)
			}
			qrows.Close()
		}
		if buildQueue == nil {
			buildQueue = []buildItem{}
		}

		// Training queue — pending recruits from the scheduled-events queue.
		type trainItem struct {
			Unit       string    `json:"unit"`
			Count      int       `json:"count"`
			CompleteAt time.Time `json:"complete_at"`
		}
		var trainQueue []trainItem
		trrows, _ := h.pool.Query(r.Context(),
			`SELECT (payload->>'unit_type')::text, (payload->>'count')::int, process_after
			 FROM scheduled_events
			 WHERE world_id = $1 AND event_type = 'TrainComplete'
			   AND processed_at IS NULL
			   AND (payload->>'settlement_id')::uuid = $2
			 ORDER BY process_after`,
			worldID, sett.ID,
		)
		if trrows != nil {
			for trrows.Next() {
				var ti trainItem
				_ = trrows.Scan(&ti.Unit, &ti.Count, &ti.CompleteAt)
				trainQueue = append(trainQueue, ti)
			}
			trrows.Close()
		}
		if trainQueue == nil {
			trainQueue = []trainItem{}
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
		type prayerRow struct {
			ID                      string             `json:"id"`
			Name                    string             `json:"name"`
			God                     string             `json:"god"`
			EffectType              string             `json:"effect_type"`
			MinKharis               float64            `json:"min_kharis"`
			Offering                map[string]float64 `json:"offering"`
			Affordable              bool               `json:"affordable"`
			CooldownRemainingMins   float64            `json:"cooldown_remaining_minutes,omitempty"`
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
					remaining := time.Duration(spec.CooldownTicks*tick.TickMinutes)*time.Minute - elapsed
					if remaining > 0 {
						cooldownRemainingMins = remaining.Minutes()
						afford = false
					}
				}
			}
			prayers = append(prayers, prayerRow{
				ID: spec.ID, Name: spec.Name, God: spec.God, EffectType: spec.EffectType,
				MinKharis: spec.MinKharis, Offering: spec.Offering, Affordable: afford,
				CooldownRemainingMins: cooldownRemainingMins,
			})
		}

		// Silver is authoritative in settlement_goods (mig 057 silver_unify); the
		// ResourceLedger.Silver column is stale (~0). Inject the live value — plus
		// grain and the bronze-chain metals — so the status view shows the same stock
		// as `goods` (previously it reported only silver, hiding a colony's tin output).
		resSnap := sett.Resources.SnapshotFull(now)
		if grows, gerr := h.pool.Query(r.Context(),
			`SELECT good_key, GREATEST(0, settled(amount, rate, calc_tick)), rate, cap
			 FROM settlement_goods
			 WHERE settlement_id = $1
			   AND good_key IN ('silver','grain','copper','tin','bronze')`,
			sett.ID,
		); gerr == nil {
			for grows.Next() {
				var k string
				var amt, rt, cp float64
				if grows.Scan(&k, &amt, &rt, &cp) == nil {
					resSnap[k] = province.ResourceDetail{Amount: amt, Rate: rt, Cap: cp}
				}
			}
			grows.Close()
		}

		resp["settlement"] = map[string]any{
			"id":                sett.ID,
			"name":              sett.Name,
			"owner_id":          sett.OwnerID,
			"kingdom_id":        sett.KingdomID,
			"culture":           sett.CultureID,
			"state":             sett.State,
			"population":        sett.Population,
			"labor_pool":        laborPool,
			"walls":             sett.WallLevel,
			"loyalty":           sett.Loyalty,
			"resources":         resSnap,
			"kharis":            kharisNow,
			"kharis_rate":       kharisRate,
			"army":              sett.Army,
			"build_queue":       buildQueue,
			"training_queue":    trainQueue,
			"buildings":         buildings,
			"can_afford":        buildAfford,
			"can_recruit":       recruitAfford,
			"available_prayers": prayers,
		}
	}

	writeJSON(w, http.StatusOK, resp)
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
	writeJSON(w, http.StatusOK, sett.Army)
}

// March handles POST /worlds/:worldID/provinces/:provinceID/march.
func (h *ProvinceHandler) March(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	sourceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
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
		TargetID      string `json:"target_id"`
		TargetQ       *int   `json:"target_q"` // for colonize: (q,r) of unclaimed tile
		TargetR       *int   `json:"target_r"`
		ColonyName    string `json:"colony_name"` // optional player-chosen name for colonize
		Intent        string `json:"intent"`
		Spearman      int    `json:"spearman"`
		WarChariot    int    `json:"war_chariot"`
		Priest        int    `json:"priest"`
		Ship          int    `json:"ship"` // galley
		EliteInfantry int    `json:"elite_infantry"`
		WarGalley     int    `json:"war_galley"`
		Merchantman   int    `json:"merchantman"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	validIntents := map[string]bool{"attack": true, "reinforce": true, "support": true, "colonize": true, "outpost": true, "scout": true, "explore": true}
	if !validIntents[req.Intent] {
		writeError(w, http.StatusBadRequest, "invalid intent")
		return
	}

	var targetID uuid.UUID
	if (req.Intent == "colonize" || req.Intent == "outpost" || req.Intent == "scout" || req.Intent == "explore") && req.TargetQ != nil && req.TargetR != nil {
		// Coordinate-targeted intents: find or create a province row for the target tile.
		// colonize/outpost reject settled targets; scout allows it (reconnaissance).
		q, r2 := *req.TargetQ, *req.TargetR
		var terrain string
		if err = h.pool.QueryRow(r.Context(),
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			worldID, q, r2,
		).Scan(&terrain); err != nil {
			writeError(w, http.StatusNotFound, "tile not found")
			return
		}
		if terrain == "mountain_limestone" || terrain == "mountain_red" {
			if req.Intent == "colonize" || req.Intent == "outpost" {
				// Check whether the target tile has ore deposits — if so, give the
				// player the actionable hint about adjacent colonization.
				var hasOre bool
				_ = h.pool.QueryRow(r.Context(),
					`SELECT copper_deposit OR tin_deposit
					        OR COALESCE(silver_deposit,false) OR COALESCE(cedar_deposit,false)
					 FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
					worldID, q, r2,
				).Scan(&hasOre)
				if hasOre {
					writeError(w, http.StatusUnprocessableEntity,
						"cannot settle impassable mountain — found a colony on an adjacent passable hex instead: the ore deposit will fall in the new colony's catchment and be mineable from there")
				} else {
					writeError(w, http.StatusUnprocessableEntity,
						"cannot settle impassable mountain terrain — target an adjacent passable hex instead")
				}
			} else {
				writeError(w, http.StatusUnprocessableEntity, "cannot target mountain terrain")
			}
			return
		}
		if req.Intent != "explore" && (terrain == "deep_sea" || terrain == "coastal_sea") {
			writeError(w, http.StatusUnprocessableEntity, "cannot target sea terrain")
			return
		}
		// Province may or may not exist; find or create it.
		err = h.pool.QueryRow(r.Context(),
			`SELECT id FROM provinces WHERE world_id = $1 AND map_q = $2 AND map_r = $3`,
			worldID, q, r2,
		).Scan(&targetID)
		if err != nil {
			// No province yet — create one so the march can reference it.
			var copperDeposit, tinDeposit, silverDeposit, cedarDeposit bool
			_ = h.pool.QueryRow(r.Context(),
				`SELECT copper_deposit, tin_deposit,
				        COALESCE(silver_deposit,false), COALESCE(cedar_deposit,false)
				 FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
				worldID, q, r2,
			).Scan(&copperDeposit, &tinDeposit, &silverDeposit, &cedarDeposit)
			if err2 := h.pool.QueryRow(r.Context(),
				`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state,
				                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit)
				 VALUES ($1,$2,$3,$4,'free',$5,$6,$7,$8) RETURNING id`,
				worldID, q, r2, terrain, copperDeposit, tinDeposit, silverDeposit, cedarDeposit,
			).Scan(&targetID); err2 != nil {
				writeError(w, http.StatusInternalServerError, "could not create target province")
				return
			}
		}
		// colonize and outpost require an empty province (no settlement).
		if req.Intent == "colonize" || req.Intent == "outpost" {
			var existingSett uuid.UUID
			if scanErr := h.pool.QueryRow(r.Context(),
				`SELECT id FROM settlements WHERE province_id = $1`, targetID,
			).Scan(&existingSett); scanErr == nil {
				writeError(w, http.StatusUnprocessableEntity, "province already settled")
				return
			}
		}
	} else {
		targetID, err = uuid.Parse(req.TargetID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid target ID")
			return
		}
	}

	// Verify ownership via settlement.
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		sourceID, worldID,
	).Scan(&ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your province")
		return
	}

	// Load source and target terrain for distance calculation.
	src, err := loadTerrainProvince(r.Context(), h.pool, sourceID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source province not found")
		return
	}
	dst, err := loadTerrainProvince(r.Context(), h.pool, targetID, worldID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target province not found")
		return
	}

	dist := province.HexDistance(src.MapTile, dst.MapTile)
	if dist == 0 {
		writeError(w, http.StatusBadRequest, "cannot march to own province")
		return
	}

	army := province.ArmyComposition{
		Spearman:      req.Spearman,
		WarChariot:    req.WarChariot,
		Priest:        req.Priest,
		Ship:          req.Ship, // galley
		EliteInfantry: req.EliteInfantry,
		WarGalley:     req.WarGalley,
		Merchantman:   req.Merchantman,
	}

	hasNaval := army.Ship > 0 || army.WarGalley > 0 || army.Merchantman > 0
	if combat.Strength(army) == 0 && !hasNaval && army.Priest == 0 {
		writeError(w, http.StatusBadRequest, "must send at least one unit")
		return
	}

	if req.Intent == "explore" && !hasNaval {
		writeError(w, http.StatusBadRequest, "explore requires at least one ship (galley, war galley, or merchantman)")
		return
	}

	// Mountains are impassable for all units.
	if dst.TerrainType == "mountain_limestone" || dst.TerrainType == "mountain_red" {
		writeError(w, http.StatusUnprocessableEntity, "mountain terrain is impassable")
		return
	}

	// Naval gating.
	// Alla tre skeppstyper (galley/war_galley/merchantman) räknas som naval.
	isSea := dst.TerrainType == "coastal_sea" || dst.TerrainType == "deep_sea"
	hasLandUnits := army.Spearman > 0 || army.WarChariot > 0 || army.EliteInfantry > 0
	if hasNaval {
		// Embarkation: origin must be coastal OR have a harbour building.
		if !src.Coastal {
			var hasHarbour bool
			_ = h.pool.QueryRow(r.Context(),
				`SELECT EXISTS(
				   SELECT 1 FROM buildings b
				   JOIN settlements s ON s.id = b.settlement_id
				   WHERE s.province_id = $1 AND b.building_type = 'harbour'
				 )`,
				sourceID,
			).Scan(&hasHarbour)
			if !hasHarbour {
				writeError(w, http.StatusUnprocessableEntity, "ships can only embark from coastal settlements or harbours")
				return
			}
		}
		if req.Intent == "explore" {
			// Explore: ships only, any coastal or sea destination, auto-returns.
			if hasLandUnits {
				writeError(w, http.StatusUnprocessableEntity, "explore requires ships only — remove land units")
				return
			}
			if !dst.Coastal && !isSea {
				writeError(w, http.StatusUnprocessableEntity, "explore can only target coastal or sea provinces")
				return
			}
		} else if hasLandUnits {
			// Naval expedition: troops must land at a coast.
			if !dst.Coastal {
				writeError(w, http.StatusUnprocessableEntity, "naval expedition must land at a coastal province")
				return
			}
		} else {
			// Ships only (not explore): coast or sea destinations allowed.
			if !dst.Coastal && !isSea {
				writeError(w, http.StatusUnprocessableEntity, "ships can only sail to coastal or sea provinces")
				return
			}
		}
	} else if isSea {
		writeError(w, http.StatusUnprocessableEntity, "sea provinces require ships to reach")
		return
	}

	now := h.clk.Now()
	moveHours := province.TerrainMoveHours(dst.TerrainType) * float64(dist)
	arrivesAt := now.Add(time.Duration(moveHours * float64(time.Hour)))
	var marchCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&marchCurrentTick)
	marchTravelTicks := int(math.Round(moveHours))
	if marchTravelTicks < 1 {
		marchTravelTicks = 1
	}

	// Deduct units from source and insert march atomically — prevents sending
	// units you don't have or using the same units in multiple marches.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// 40% garrison + 75% attacker rules (attack only — skip for colonize/reinforce/support).
	if req.Intent == "attack" {
		var garrison province.ArmyComposition
		if err := tx.QueryRow(r.Context(),
			`SELECT infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
			 FROM settlements WHERE province_id = $1 AND world_id = $2`,
			sourceID, worldID,
		).Scan(&garrison.Spearman, &garrison.WarChariot,
			&garrison.Priest, &garrison.Ship, &garrison.EliteInfantry,
			&garrison.WarGalley, &garrison.Merchantman,
		); err == nil {
			garrisonDP := combat.Strength(garrison)
			sentDP := combat.Strength(army)
			if garrisonDP > 0 && sentDP > 0 && sentDP > 0.6*garrisonDP {
				writeError(w, http.StatusUnprocessableEntity, "cannot send more than 60% of garrison strength — you must defend your home")
				return
			}
		}

		// Must send at least 75% of the defender's DP.
		var defGarrison province.ArmyComposition
		if err := tx.QueryRow(r.Context(),
			`SELECT infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
			 FROM settlements WHERE province_id = $1 AND world_id = $2`,
			targetID, worldID,
		).Scan(&defGarrison.Spearman, &defGarrison.WarChariot,
			&defGarrison.Priest, &defGarrison.Ship, &defGarrison.EliteInfantry,
			&defGarrison.WarGalley, &defGarrison.Merchantman,
		); err == nil {
			defDP := combat.Strength(defGarrison)
			sentDP := combat.Strength(army)
			if defDP > 0 && sentDP < 0.75*defDP {
				writeError(w, http.StatusUnprocessableEntity, "must send at least 75% of the defender's strength to mount a serious attack")
				return
			}
		}
	}

	tag, err := tx.Exec(r.Context(),
		`UPDATE settlements SET
		   infantry       = GREATEST(0, infantry       - $1),
		   chariot        = GREATEST(0, chariot        - $2),
		   priest         = GREATEST(0, priest         - $3),
		   ship           = GREATEST(0, ship           - $4),
		   elite_infantry = GREATEST(0, elite_infantry - $5),
		   war_galley     = GREATEST(0, war_galley     - $6),
		   merchantman    = GREATEST(0, merchantman    - $7)
		 WHERE province_id = $8 AND world_id = $9
		   AND infantry       >= $1
		   AND chariot        >= $2
		   AND priest         >= $3
		   AND ship           >= $4
		   AND elite_infantry >= $5
		   AND war_galley     >= $6
		   AND merchantman    >= $7`,
		army.Spearman, army.WarChariot, army.Priest, army.Ship, army.EliteInfantry,
		army.WarGalley, army.Merchantman, sourceID, worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct army")
		return
	}
	if tag.RowsAffected() == 0 {
		// The atomic UPDATE matched no row because at least one unit type was
		// short. Report which units fell short and by how much so the caller
		// can scale the march down instead of looping a blind 422.
		var have province.ArmyComposition
		_ = tx.QueryRow(r.Context(),
			`SELECT infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
			 FROM settlements WHERE province_id = $1 AND world_id = $2`,
			sourceID, worldID,
		).Scan(&have.Spearman, &have.WarChariot,
			&have.Priest, &have.Ship, &have.EliteInfantry,
			&have.WarGalley, &have.Merchantman)
		writeError(w, http.StatusUnprocessableEntity, insufficientUnitsMsg(army, have))
		return
	}

	var colonyName *string
	if req.Intent == "colonize" && req.ColonyName != "" {
		n := req.ColonyName
		colonyName = &n
	}

	var marchID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO marching_armies
		 (world_id, origin_id, target_id, infantry, chariot, priest, ship, elite_infantry,
		  war_galley, merchantman, intent, departs_at, arrives_at, colony_name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING id`,
		worldID, sourceID, targetID,
		army.Spearman, army.WarChariot, army.Priest, army.Ship, army.EliteInfantry,
		army.WarGalley, army.Merchantman,
		req.Intent, now, arrivesAt, colonyName,
	).Scan(&marchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not send army")
		return
	}

	// Recompute production: army cols decremented + new transit march affects labor_pool.
	var srcSettlementID uuid.UUID
	if err2 := tx.QueryRow(r.Context(),
		`SELECT id FROM settlements WHERE province_id=$1 AND world_id=$2`, sourceID, worldID,
	).Scan(&srcSettlementID); err2 == nil {
		_ = economy.RecomputeProduction(r.Context(), tx, srcSettlementID)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	if err := h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledArmyArrival,
		combat.ArmyArrivalPayload{MarchingArmyID: marchID},
		marchCurrentTick+marchTravelTicks,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule arrival")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"march_id":   marchID,
		"arrives_at": arrivesAt,
		"distance":   dist,
	})
}

// RecallOutpost handles DELETE /worlds/:worldID/provinces/:provinceID/outpost.
// Tears down the player's outpost at provinceID, subtracts the production flows,
// and returns the garrison to the feeding (origin) settlement.
func (h *ProvinceHandler) RecallOutpost(w http.ResponseWriter, r *http.Request) {
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

	// Verify player owns this outpost.
	var outpostFeeds uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT outpost_feeds FROM provinces
		 WHERE id=$1 AND world_id=$2 AND owner_id=$3 AND controller_id IS NULL AND owner_id IS NOT NULL`,
		provinceID, worldID, playerID,
	).Scan(&outpostFeeds); err != nil {
		writeError(w, http.StatusNotFound, "outpost not found or not yours")
		return
	}

	// Garrison size comes from the resolved outpost march that established it.
	var gInf, gCha, gPri, gShip, gElite, gWarGalley, gMerchantman int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
		 FROM marching_armies
		 WHERE target_id=$1 AND intent='outpost' AND resolved=true
		 ORDER BY created_at DESC LIMIT 1`,
		provinceID,
	).Scan(&gInf, &gCha, &gPri, &gShip, &gElite, &gWarGalley, &gMerchantman)

	// Outpost position + terrain; home (feeding) settlement position + commanding settlement id.
	var outpostTerrain string
	var oQ, oR, hQ, hR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT terrain_type, map_q, map_r FROM provinces WHERE id=$1`, provinceID,
	).Scan(&outpostTerrain, &oQ, &oR)
	if outpostTerrain == "" {
		outpostTerrain = "plains"
	}
	var homeSettlementID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT s.id, p.map_q, p.map_r FROM settlements s
		 JOIN provinces p ON p.id = s.province_id WHERE s.id=$1`,
		outpostFeeds,
	).Scan(&homeSettlementID, &hQ, &hR); err != nil {
		writeError(w, http.StatusInternalServerError, "could not locate home settlement")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Dispatch a visible recall messenger from home toward the outpost. The outpost keeps producing
	// until the order arrives (ScheduledRecallArrival); only then is it torn down and the garrison sent home.
	dist := province.HexDistance(province.MapPosition{Q: hQ, R: hR}, province.MapPosition{Q: oQ, R: oR})
	messengerArrivesAt := h.clk.Now().Add(messenger.MessengerTravelDuration(dist))

	var recallCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&recallCurrentTick)
	recallDueTick := recallCurrentTick + messenger.MessengerTravelTicks(dist)

	var messengerID uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,$4,'outbound','recall',$5,$6,$7,$8,$9)
		 RETURNING id`,
		worldID, playerID, homeSettlementID, "Recall order — abandon the outpost and return home.",
		hQ, hR, oQ, oR, messengerArrivesAt,
	).Scan(&messengerID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not dispatch recall messenger")
		return
	}

	payload := messenger.RecallOutpostPayload{
		Kind:           "outpost",
		WorldID:        worldID,
		MessengerID:    messengerID,
		ProvinceID:     provinceID,
		HomeID:         outpostFeeds,
		Spearman:       gInf,
		WarChariot:     gCha,
		Priest:         gPri,
		Ship:           gShip,
		EliteInfantry:  gElite,
		WarGalley:      gWarGalley,
		Merchantman:    gMerchantman,
		OutpostTerrain: outpostTerrain,
		OutpostQ:       oQ,
		OutpostR:       oR,
		HomeQ:          hQ,
		HomeR:          hR,
	}
	// Messenger row + recall-arrival event committed atomically.
	if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledRecallArrival,
		payload, recallDueTick); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule recall arrival")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "recall_messenger_sent",
		"province_id":          provinceID,
		"messenger_id":         messengerID,
		"messenger_arrives_at": messengerArrivesAt,
		"messenger_distance":   dist,
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

	// Mines require the matching ore deposit in the 6-tile catchment — production
	// reads only the catchment (not the settlement's own tile), so a mine without a
	// matching deposit nearby would produce nothing. Gate it at build time.
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
				fmt.Sprintf("a %s here would produce nothing — no %s deposit in this settlement's catchment (the 6 surrounding hexes). Build it at a settlement adjacent to the ore.", req.BuildingType, oreName))
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
	completeAt := h.clk.Now().Add(time.Duration(spec.DurationTicks*tick.TickMinutes) * time.Minute)
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

// recruitPerManCosts returns the resource cost per man for a given unit type.
// Derived from Skalbeslut (2026-06-15): per-man = old UnitSpec cost / old PopCost.
// Batch = 10 men → total cost = per-man × 10. All siffror are tunable at reseed.
func recruitPerManCosts(unitType string) map[string]float64 {
	switch unitType {
	case "spearman":
		return map[string]float64{"grain": 3, "silver": 0.2}
	case "elite_infantry":
		return map[string]float64{"grain": 2.5, "bronze": 0.2, "silver": 0.4}
	case "war_chariot":
		return map[string]float64{"grain": 3.75, "timber": 0.625, "bronze": 0.375, "silver": 0.5}
	case "ship": // galley; crew 20
		return map[string]float64{"timber": 9, "silver": 0.3}
	case "war_galley": // crew 50
		return map[string]float64{"cedar": 5, "bronze": 0.33, "silver": 0.6}
	case "merchantman": // crew 10
		return map[string]float64{"timber": 8.75, "silver": 0.2}
	case "priest": // stationary; same cost structure
		return map[string]float64{"grain": 5}
	}
	return nil
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Men <= 0 || req.Men > 100 {
		writeError(w, http.StatusBadRequest, "men must be 1–100")
		return
	}
	if req.Men%10 != 0 {
		writeError(w, http.StatusBadRequest, "men must be a multiple of 10")
		return
	}

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

	// Training queue cap: max 10 pending batches per settlement.
	var pendingTraining int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'TrainComplete'
		   AND processed_at IS NULL AND failed_at IS NULL
		   AND (payload->>'settlement_id')::uuid = $2`,
		worldID, settlementID,
	).Scan(&pendingTraining)
	batches := req.Men / 10
	if pendingTraining+batches > 10 {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("training queue would overflow: %d pending + %d new > 10", pendingTraining, batches))
		return
	}

	// C2/C-collapse: men are drawn from population at recruit time.
	// Allow draining population down to 1 (or below 100 → triggers city collapse).
	// No hard floor here: if the Wanax chooses to overmobilise, the city collapses.
	var population int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT population FROM settlements WHERE id = $1 AND state != 'collapsed'`,
		settlementID,
	).Scan(&population)
	if population == 0 {
		writeError(w, http.StatusUnprocessableEntity, "settlement has already collapsed")
		return
	}
	if req.Men >= population {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("insufficient population: cannot draft %d men from a settlement of %d",
				req.Men, population))
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

	// Compute total resource costs: per-man cost × number of men.
	totalCosts := make(map[string]float64, len(perManCosts))
	for k, v := range perManCosts {
		totalCosts[k] = v * float64(req.Men)
	}
	totalKharis := spec.CostKharis * float64(req.Men)

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
		req.Men, settlementID,
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

	// Determine unit category and crew.
	uType := unit.Type(req.UnitType)
	cat := unit.CategoryOf(uType)
	crew := unit.CrewFor(uType)

	// Naval note: for naval units, "men" = crew of ONE vessel. Size stays at 1
	// and the unit is immediately garrison-ready upon completion.
	// Land units: size grows in batches of 10; deployable at 100.

	// Create or grow the forming unit in the units table.
	// Find an existing forming unit of same type in this settlement.
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

	var unitID uuid.UUID
	if existingUnitID != nil {
		// Reinforce existing forming unit.
		if err := h.pool.QueryRow(r.Context(),
			`UPDATE units SET size = size + $1, updated_at = now()
			 WHERE id = $2
			 RETURNING id`,
			req.Men, *existingUnitID,
		).Scan(&unitID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not grow forming unit")
			return
		}
	} else {
		// Create new forming unit.
		initialStatus := "forming"
		if cat == unit.CategoryNaval {
			// Naval: directly garrison-ready (one vessel, no 100-man gate).
			initialStatus = "garrison"
		}
		if err := h.pool.QueryRow(r.Context(),
			`INSERT INTO units
			   (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 RETURNING id`,
			worldID, playerID, string(uType), string(cat),
			req.Men, crew, initialStatus, settlementID,
		).Scan(&unitID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not create forming unit")
			return
		}
	}

	// Determine the final size after this recruitment.
	finalSize := existingSize + req.Men

	// Schedule one TrainComplete per batch-of-10, staggered.
	batchTicks := recruitBatchTicks(req.UnitType)
	var trainCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&trainCurrentTick)
	now := h.clk.Now()
	var lastCompleteAt time.Time
	for i := 0; i < batches; i++ {
		dueTick := trainCurrentTick + batchTicks*(i+1)
		completeAt := now.Add(time.Duration(batchTicks*(i+1)*tick.TickMinutes) * time.Minute)
		lastCompleteAt = completeAt
		if err := h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledTrainComplete,
			combat.TrainCompletePayload{
				SettlementID: settlementID,
				UnitType:     req.UnitType,
				Count:        10, // always 10 per batch
				UnitID:       unitID,
			}, dueTick,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule training batch")
			return
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"unit_id":      unitID,
		"unit_type":    req.UnitType,
		"men":          req.Men,
		"batches":      batches,
		"complete_at":  lastCompleteAt,
		"forming_size": finalSize,
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

	now := h.clk.Now()

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
	// (same logic as RecomputeProduction — 6 adjacent map_tiles, not the own province tile).
	baseRows, _ := h.pool.Query(r.Context(),
		`SELECT pr.good_key, SUM(pr.rate_per_min) AS base_potential
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 JOIN map_tiles mt ON mt.world_id = s.world_id
		     AND mt.terrain NOT IN ('deep_sea','coastal_sea')
		     AND (
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
		`SELECT sg.good_key, sg.amount, sg.rate, sg.cap, sg.calc_tick,
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
		Rate           float64 `json:"rate_per_min"`
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
		var amount, rate, capV float64
		var calcAt time.Time
		var baseValue float64
		if err := rows.Scan(&key, &amount, &rate, &capV, &calcAt, &baseValue, &name, &tier, &category); err != nil {
			continue
		}
		elapsed := now.Sub(calcAt).Minutes()
		current := amount + elapsed*rate
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
			Price:          goodLocalPrice(baseValue, current, rate, capV),
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

	// Distance bonus: caravans that travel farther deliver more.
	// bonus = 1 + sqrt(dist) × 0.1  (d=1→1.1×, d=4→1.2×, d=9→1.3×, d=16→1.4×)
	distBonus := 1.0 + math.Sqrt(float64(dist))*0.1
	deliveredQty := req.Quantity * distBonus

	// Enqueue delivery within the same transaction — atomic with the deduction.
	if err := h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
		map[string]any{
			"trade_route_id":     routeID,
			"destination_id":     req.DestinationID,
			"good_key":           req.GoodKey,
			"quantity":           req.Quantity,
			"delivered_quantity": deliveredQty,
		}, tradeCurrentTick+tradeTravelTicks); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule delivery")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"route_id":       routeID,
		"arrives_at":     arrivesAt,
		"distance":       dist,
		"travel_min":     travelMins,
		"distance_bonus": distBonus,
		"delivered_qty":  deliveredQty,
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

	// Check required building is present.
	var hasBuilding bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM buildings WHERE settlement_id = $1 AND building_type = $2)`,
		settlementID, buildingType,
	).Scan(&hasBuilding)
	if !hasBuilding {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%s required", buildingType))
		return
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

	// Credit output.
	produced := outputQty * req.Quantity
	_, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 100, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
		             + $3,
		         settlement_goods.cap),
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
		ID            uuid.UUID `json:"id"`
		PeerName      string    `json:"peer_name"` // destination for outgoing, origin for incoming
		GoodKey       string    `json:"good_key"`
		Quantity      float64   `json:"quantity"`
		DeliveredQty  float64   `json:"delivered_qty"`
		DistanceBonus float64   `json:"distance_bonus"`
		Direction     string    `json:"direction"` // "outgoing" | "incoming"
		DepartsAt     time.Time `json:"departs_at"`
		ArrivesAt     time.Time `json:"arrives_at"`
	}
	var result []routeItem
	for rows.Next() {
		var ri routeItem
		var oq, or_, dq, dr int
		if err := rows.Scan(&ri.ID, &ri.PeerName, &ri.GoodKey, &ri.Quantity, &ri.DepartsAt, &ri.ArrivesAt,
			&oq, &or_, &dq, &dr); err == nil {
			dist := province.HexDistance(province.MapPosition{Q: oq, R: or_}, province.MapPosition{Q: dq, R: dr})
			ri.DistanceBonus = 1.0 + math.Sqrt(float64(dist))*0.1
			ri.DeliveredQty = ri.Quantity * ri.DistanceBonus
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
					dist := province.HexDistance(province.MapPosition{Q: oq, R: or_}, province.MapPosition{Q: dq, R: dr})
					ri.DistanceBonus = 1.0 + math.Sqrt(float64(dist))*0.1
					ri.DeliveredQty = ri.Quantity * ri.DistanceBonus
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

// goodLocalPrice mirrors economy.LocalPrice for use within this package.
func goodLocalPrice(baseValue, stock, ratePerMin, cap float64) float64 {
	const lookahead = 60.0 * 24.0
	projected := stock + ratePerMin*lookahead
	if projected < 0 {
		projected = 0
	}
	if projected > cap {
		projected = cap
	}
	reference := cap * 0.3
	if reference <= 0 {
		return baseValue
	}
	shortage := math.Max(0, (reference-projected)/reference)
	surplus := 0.0
	if cap-reference > 0 {
		surplus = math.Max(0, (projected-reference)/(cap-reference))
	}
	price := baseValue * (1 + 2.0*shortage) * (1 - 0.5*surplus)
	if price < 0 {
		price = 0
	}
	return price
}

// Disband handles POST /worlds/:worldID/provinces/:provinceID/disband.
// Releases units back into the population. Soldiers return to civilian life:
// each disbanded unit adds PopCost back to population.
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
	// Disbanded units leave the army columns; labor_pool rises automatically
	// because army pop-cost decreases. RecomputeProduction updates the rates.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	tag, err := tx.Exec(r.Context(),
		`UPDATE settlements SET
		     infantry       = GREATEST(0, infantry       - $1),
		     chariot        = GREATEST(0, chariot        - $2),
		     priest         = GREATEST(0, priest         - $3),
		     ship           = GREATEST(0, ship           - $4),
		     elite_infantry = GREATEST(0, elite_infantry - $5),
		     war_galley     = GREATEST(0, war_galley     - $6),
		     merchantman    = GREATEST(0, merchantman    - $7)
		 WHERE id = $8`,
		req.Spearman, req.WarChariot, req.Priest, req.Ship, req.EliteInfantry,
		req.WarGalley, req.Merchantman, settlementID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusInternalServerError, "disband failed")
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

	writeJSON(w, http.StatusOK, map[string]any{
		"disbanded": map[string]int{
			"spearman": req.Spearman, "war_chariot": req.WarChariot,
			"priest": req.Priest, "ship": req.Ship, "elite_infantry": req.EliteInfantry,
			"war_galley": req.WarGalley, "merchantman": req.Merchantman,
		},
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
	// (same logic as RecomputeProduction — 6 adjacent map_tiles, not the own province tile).
	producible := make(map[string]bool)
	prows, err := h.pool.Query(r.Context(),
		`SELECT DISTINCT pr.good_key
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 JOIN map_tiles mt ON mt.world_id = s.world_id
		     AND mt.terrain NOT IN ('deep_sea','coastal_sea')
		     AND (
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

	// Echo both the percent levers and the resulting citizen counts: the share
	// auto-scales with population, but real output depends on the absolute number
	// of citizens (more citizens produce more, even at a lower percent).
	citizens := make(map[string]int, len(filtered))
	for key, pct := range filtered {
		citizens[key] = int(pct / 100.0 * float64(laborPool))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"percent":       filtered,
		"citizens":      citizens,
		"idle_percent":  100.0 - totalPct,
		"idle_citizens": int((100.0 - totalPct) / 100.0 * float64(laborPool)),
		"labor_pool":    laborPool,
		"message":       "labor allocation updated and production recomputed",
	})
}

// OutpostFlows handles GET /worlds/:worldID/outpost-flows.
// Returns all outpost_flows rows for the authenticated Wanax's settlements.
func (h *ProvinceHandler) OutpostFlows(w http.ResponseWriter, r *http.Request) {
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
		`SELECT of.province_id, of.good_key, of.rate, of.settlement_id,
		        s.name AS home_settlement_name,
		        prov.terrain_type, prov.map_q, prov.map_r
		 FROM outpost_flows of
		 JOIN settlements s ON s.id = of.settlement_id
		 JOIN provinces prov ON prov.id = of.province_id
		 WHERE of.world_id = $1
		   AND s.owner_id = $2
		 ORDER BY s.name, of.good_key`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load outpost flows")
		return
	}
	defer rows.Close()

	type flowRow struct {
		ProvinceID         uuid.UUID `json:"province_id"`
		GoodKey            string    `json:"good_key"`
		Rate               float64   `json:"rate_per_min"`
		HomeSettlementID   uuid.UUID `json:"home_settlement_id"`
		HomeSettlementName string    `json:"home_settlement_name"`
		Terrain            string    `json:"terrain"`
		Q                  int       `json:"q"`
		R                  int       `json:"r"`
	}
	var result []flowRow
	for rows.Next() {
		var fr flowRow
		if err := rows.Scan(
			&fr.ProvinceID, &fr.GoodKey, &fr.Rate,
			&fr.HomeSettlementID, &fr.HomeSettlementName,
			&fr.Terrain, &fr.Q, &fr.R,
		); err != nil {
			continue
		}
		result = append(result, fr)
	}
	if result == nil {
		result = []flowRow{}
	}
	writeJSON(w, http.StatusOK, result)
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
