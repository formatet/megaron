package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/poleia/server/internal/timescale"
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

	resp := map[string]any{
		"id":              prov.ID,
		"world_id":        prov.WorldID,
		"map_tile":        prov.MapTile,
		"terrain_type":    prov.TerrainType,
		"territory_state": prov.TerritoryState,
		"copper_deposit":  prov.CopperDeposit,
		"tin_deposit":     prov.TinDeposit,
		"silver_deposit":  prov.SilverDeposit,
		"cedar_deposit":   prov.CedarDeposit,
	}

	sett, err := loadSettlementByProvince(r.Context(), h.pool, provinceID, worldID)
	if err == nil {
		now := h.clk.Now()

		// Build queue — include so API clients don't need a separate endpoint.
		type buildItem struct {
			Type       string    `json:"type"`
			CompleteAt time.Time `json:"complete_at"`
		}
		var buildQueue []buildItem
		qrows, _ := h.pool.Query(r.Context(),
			`SELECT building_type, complete_at FROM build_queue
			 WHERE settlement_id = $1 ORDER BY complete_at`,
			sett.ID,
		)
		if qrows != nil {
			for qrows.Next() {
				var bi buildItem
				_ = qrows.Scan(&bi.Type, &bi.CompleteAt)
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

		// labor_pool = population − army pop-cost − transit pop-cost (variant B).
		homePop := sett.Army.Infantry*economy.PopCosts["infantry"] +
			sett.Army.Chariot*economy.PopCosts["chariot"] +
			sett.Army.Priest*economy.PopCosts["priest"] +
			sett.Army.Ship*economy.PopCosts["ship"] +
			sett.Army.EliteInfantry*economy.PopCosts["elite_infantry"] +
			sett.Army.WarGalley*economy.PopCosts["war_galley"] +
			sett.Army.Merchantman*economy.PopCosts["merchantman"]
		var transitPop int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(
			     m.infantry*$2+m.chariot*$3+m.priest*$4+m.ship*$5+m.elite_infantry*$6
			     +m.war_galley*$7+m.merchantman*$8
			 ),0)
			 FROM marching_armies m
			 JOIN settlements s ON s.province_id=m.origin_id
			 WHERE s.id=$1 AND m.resolved=false`,
			sett.ID,
			economy.PopCosts["infantry"], economy.PopCosts["chariot"],
			economy.PopCosts["priest"], economy.PopCosts["ship"], economy.PopCosts["elite_infantry"],
			economy.PopCosts["war_galley"], economy.PopCosts["merchantman"],
		).Scan(&transitPop)
		laborPool := sett.Population - homePop - transitPop
		if laborPool < 0 {
			laborPool = 0
		}

		// Load current goods amounts for affordability checks.
		goodsStock := make(map[string]float64)
		gsrows, _ := h.pool.Query(r.Context(),
			`SELECT good_key, amount + EXTRACT(EPOCH FROM (now()-calc_at))/60 * rate
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

		// can_recruit per unit: goods + labor pool (for 1 unit).
		type recruitAffordRow struct {
			Unit       string `json:"unit"`
			CanRecruit bool   `json:"can_recruit"`
		}
		var recruitAfford []recruitAffordRow
		for unitType, spec := range province.UnitSpecs {
			afford := laborPool >= spec.PopCost
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

		resp["settlement"] = map[string]any{
			"id":             sett.ID,
			"name":           sett.Name,
			"owner_id":       sett.OwnerID,
			"kingdom_id":     sett.KingdomID,
			"culture":        sett.CultureID,
			"state":          sett.State,
			"population":     sett.Population,
			"labor_pool":     laborPool,
			"walls":          sett.WallLevel,
			"loyalty":        sett.Loyalty,
			"resources":      sett.Resources.SnapshotFull(now),
			"army":           sett.Army,
			"build_queue":    buildQueue,
			"training_queue": trainQueue,
			"buildings":      buildings,
			"can_afford":     buildAfford,
			"can_recruit":    recruitAfford,
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
		Infantry      int    `json:"infantry"`
		Chariot       int    `json:"chariot"`
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
			writeError(w, http.StatusUnprocessableEntity, "cannot target mountain terrain")
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
		Infantry:      req.Infantry,
		Chariot:       req.Chariot,
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
	hasLandUnits := army.Infantry > 0 || army.Chariot > 0 || army.EliteInfantry > 0
	if hasNaval {
		// Embarkation: origin must be coast_beach OR have a harbour building.
		if src.TerrainType != "coast_beach" {
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
			if dst.TerrainType != "coast_beach" && !isSea {
				writeError(w, http.StatusUnprocessableEntity, "explore can only target coastal or sea provinces")
				return
			}
		} else if hasLandUnits {
			// Naval expedition: troops must land at a coast.
			if dst.TerrainType != "coast_beach" {
				writeError(w, http.StatusUnprocessableEntity, "naval expedition must land at a coastal province")
				return
			}
		} else {
			// Ships only (not explore): coast or sea destinations allowed.
			if dst.TerrainType != "coast_beach" && !isSea {
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
	arrivesAt := now.Add(timescale.Apply(time.Duration(moveHours * float64(time.Hour))))

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
		).Scan(&garrison.Infantry, &garrison.Chariot,
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
		).Scan(&defGarrison.Infantry, &defGarrison.Chariot,
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
		army.Infantry, army.Chariot, army.Priest, army.Ship, army.EliteInfantry,
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
		).Scan(&have.Infantry, &have.Chariot,
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
		army.Infantry, army.Chariot, army.Priest, army.Ship, army.EliteInfantry,
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

	if err := h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledArmyArrival,
		combat.ArmyArrivalPayload{MarchingArmyID: marchID},
		arrivesAt,
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
		`SELECT s.id, p.map_q, p.map_r FROM provinces p
		 JOIN settlements s ON s.province_id = p.id WHERE p.id=$1`,
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
		Infantry:       gInf,
		Chariot:        gCha,
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
	if err := h.scheduler.EnqueueTx(r.Context(), tx, worldID, events.ScheduledRecallArrival,
		payload, messengerArrivesAt); err != nil {
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

	// Harbour requires the settlement to be on coast_beach OR adjacent to a coastal/sea hex.
	if req.BuildingType == "harbour" {
		var pq, pr int
		var terrain string
		_ = h.pool.QueryRow(r.Context(),
			`SELECT p.map_q, p.map_r, p.terrain_type FROM provinces p WHERE p.id = $1`, provinceID,
		).Scan(&pq, &pr, &terrain)
		if terrain != "coast_beach" {
			// Check all 6 axial neighbours for coastal/sea terrain.
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
				     AND terrain IN ('coast_beach','coastal_sea','deep_sea')
				 )`,
				worldID, pq, pr,
			).Scan(&coastNeighbour)
			if !coastNeighbour {
				writeError(w, http.StatusUnprocessableEntity, "harbour requires a coastal or sea tile on an adjacent hex")
				return
			}
		}
	}

	// Queue guards — block before we deduct resources.
	// 1. Walls/towers/bronze walls upgrade an existing wall_level; everything else
	//    is a one-instance building (production_rules use UPSERT, duplicates waste resources).
	// 2. No double-queueing the same building.
	// 3. Cap concurrent queue at maxParallelBuilds.
	const maxParallelBuilds = 2
	upgradeable := map[string]bool{"wall": true, "tower": true, "bronze_wall": true}

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

	// Deduct all building costs from settlement_goods atomically.
	if err := deductGoods(r.Context(), h.pool, settlementID, spec.Costs); err != nil {
		var insErr *insufficientGoodsError
		if errors.As(err, &insErr) {
			writeError(w, http.StatusUnprocessableEntity, insErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "could not deduct resources")
		}
		return
	}

	// Deduct silver if required.
	if spec.CostSilver > 0 {
		tag, err2 := h.pool.Exec(r.Context(),
			`UPDATE settlements SET
			   silver_amount = silver_amount
			     + EXTRACT(EPOCH FROM (now() - silver_calc_at))/60 * silver_rate - $1,
			   silver_calc_at = now()
			 WHERE id = $2
			   AND silver_amount + EXTRACT(EPOCH FROM (now() - silver_calc_at))/60 * silver_rate >= $1`,
			spec.CostSilver, settlementID,
		)
		if err2 != nil || tag.RowsAffected() == 0 {
			writeError(w, http.StatusUnprocessableEntity, "insufficient silver")
			return
		}
	}

	completeAt := h.clk.Now().Add(timescale.Apply(spec.Duration))
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

	if err := h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledBuildComplete,
		combat.BuildCompletePayload{
			SettlementID: settlementID,
			BuildQueueID: queueID,
			BuildingType: req.BuildingType,
		}, completeAt,
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
			     amount  = LEAST(amount + EXTRACT(EPOCH FROM (now()-calc_at))/60*rate + $1, cap),
			     calc_at = now()
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

// Recruit handles POST /worlds/:worldID/provinces/:provinceID/recruit.
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
		Count    int    `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Count <= 0 || req.Count > 500 {
		writeError(w, http.StatusBadRequest, "count must be 1-500")
		return
	}

	spec, ok := province.UnitSpecs[req.UnitType]
	if !ok {
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

	// Training queue cap: max 2 pending jobs per settlement (mirrors build queue).
	var pendingTraining int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM scheduled_events
		 WHERE world_id = $1 AND event_type = 'TrainComplete'
		   AND processed_at IS NULL AND failed_at IS NULL
		   AND (payload->>'settlement_id')::uuid = $2`,
		worldID, settlementID,
	).Scan(&pendingTraining)
	if pendingTraining >= 2 {
		writeError(w, http.StatusUnprocessableEntity, "training queue is full (max 2 concurrent jobs)")
		return
	}

	// Variant-B labor check: population is the demographic total (unchanged by
	// recruit/disband). labor_pool = population − existing army pop-cost − transit pop-cost.
	// Recruit reduces labor_pool (army takes workers from production); population stays.
	totalPopCost := spec.PopCost * req.Count
	if totalPopCost > 0 {
		var population, infantry, chariot, priest, ship, eliteInfantry, warGalley, merchantman int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT population, infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
			 FROM settlements WHERE id = $1`,
			settlementID,
		).Scan(&population, &infantry, &chariot, &priest, &ship, &eliteInfantry, &warGalley, &merchantman)

		homePop := infantry*economy.PopCosts["infantry"] +
			chariot*economy.PopCosts["chariot"] +
			priest*economy.PopCosts["priest"] +
			ship*economy.PopCosts["ship"] +
			eliteInfantry*economy.PopCosts["elite_infantry"] +
			warGalley*economy.PopCosts["war_galley"] +
			merchantman*economy.PopCosts["merchantman"]

		var transitPop int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(
			     m.infantry       * $2 +
			     m.chariot        * $3 +
			     m.priest         * $4 +
			     m.ship           * $5 +
			     m.elite_infantry * $6 +
			     m.war_galley     * $7 +
			     m.merchantman    * $8
			 ), 0)
			 FROM marching_armies m
			 JOIN settlements s ON s.province_id = m.origin_id
			 WHERE s.id = $1 AND m.resolved = false`,
			settlementID,
			economy.PopCosts["infantry"], economy.PopCosts["chariot"],
			economy.PopCosts["priest"], economy.PopCosts["ship"], economy.PopCosts["elite_infantry"],
			economy.PopCosts["war_galley"], economy.PopCosts["merchantman"],
		).Scan(&transitPop)

		laborPool := population - homePop - transitPop
		if laborPool < 0 {
			laborPool = 0
		}
		if laborPool-totalPopCost < 0 {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("inte nog arbetskraft: behöver %d, ledig %d", totalPopCost, laborPool))
			return
		}
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

	// Scale per-unit costs to total for the batch.
	totalCosts := make(map[string]float64, len(spec.Costs))
	for k, v := range spec.Costs {
		totalCosts[k] = v * float64(req.Count)
	}
	totalKharis := spec.CostKharis * float64(req.Count)

	// Deduct all goods costs from settlement_goods atomically.
	if err := deductGoods(r.Context(), h.pool, settlementID, totalCosts); err != nil {
		var insErr *insufficientGoodsError
		if errors.As(err, &insErr) {
			writeError(w, http.StatusUnprocessableEntity, insErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not deduct resources: %v", err))
		}
		return
	}

	// Deduct kharis from the player's world record pool.
	if totalKharis > 0 {
		tag, err2 := h.pool.Exec(r.Context(),
			`UPDATE player_world_records SET
			   kharis_amount = kharis_amount
			     + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) - $1,
			   kharis_calc_at = now()
			 WHERE player_id = $2 AND world_id = $3
			   AND kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) >= $1`,
			totalKharis, playerID, worldID,
		)
		if err2 != nil || tag.RowsAffected() == 0 {
			writeError(w, http.StatusUnprocessableEntity, "insufficient kharis")
			return
		}
	}

	// Variant B: population is NOT decremented on recruit.
	// Army units are "taken workers" — they remain in demographics but exit labor_pool.
	// RecomputeProduction is called inside the train handler when units land in the army.
	// Here we only need a recompute if we later add the army count synchronously,
	// but since TrainComplete does the army col update, we skip it here.

	completeAt := h.clk.Now().Add(timescale.Apply(spec.Duration * time.Duration(req.Count)))

	if err := h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledTrainComplete,
		combat.TrainCompletePayload{
			SettlementID: settlementID,
			UnitType:     req.UnitType,
			Count:        req.Count,
		}, completeAt,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule training")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"unit_type":   req.UnitType,
		"count":       req.Count,
		"complete_at": completeAt,
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

	// Load labor pool for this settlement (pop − army pop-cost − transit pop-cost).
	var population, sInfantry, sChariot, sPriest, sShip, sEliteInfantry, sWarGalley, sMerchantman int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT population, infantry, chariot, priest, ship, elite_infantry, war_galley, merchantman
		 FROM settlements WHERE id = $1`, settlementID,
	).Scan(&population, &sInfantry, &sChariot, &sPriest, &sShip, &sEliteInfantry, &sWarGalley, &sMerchantman)
	homePop := sInfantry*economy.PopCosts["infantry"] +
		sChariot*economy.PopCosts["chariot"] +
		sPriest*economy.PopCosts["priest"] +
		sShip*economy.PopCosts["ship"] +
		sEliteInfantry*economy.PopCosts["elite_infantry"] +
		sWarGalley*economy.PopCosts["war_galley"] +
		sMerchantman*economy.PopCosts["merchantman"]
	var transitPop int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(
		     m.infantry*$2+m.chariot*$3+m.priest*$4+m.ship*$5+m.elite_infantry*$6
		     +m.war_galley*$7+m.merchantman*$8
		 ),0)
		 FROM marching_armies m
		 JOIN settlements s ON s.province_id=m.origin_id
		 WHERE s.id=$1 AND m.resolved=false`,
		settlementID,
		economy.PopCosts["infantry"], economy.PopCosts["chariot"],
		economy.PopCosts["priest"], economy.PopCosts["ship"], economy.PopCosts["elite_infantry"],
		economy.PopCosts["war_galley"], economy.PopCosts["merchantman"],
	).Scan(&transitPop)
	laborPool := population - homePop - transitPop
	if laborPool < 0 {
		laborPool = 0
	}

	// Load citizen allocations.
	wrows, _ := h.pool.Query(r.Context(),
		`SELECT good_key, citizens FROM settlement_labor WHERE settlement_id = $1`, settlementID,
	)
	laborCitizens := make(map[string]int)
	if wrows != nil {
		for wrows.Next() {
			var k string
			var c int
			_ = wrows.Scan(&k, &c)
			laborCitizens[k] = c
		}
		wrows.Close()
	}

	// Load base_potential per good from production_rules.
	baseRows, _ := h.pool.Query(r.Context(),
		`SELECT pr.good_key, SUM(pr.rate_per_min) AS base_potential
		 FROM production_rules pr
		 JOIN settlements s ON s.id = $1
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE (pr.terrain_type IS NULL OR pr.terrain_type = prov.terrain_type)
		   AND (pr.building_type IS NULL OR EXISTS (
		           SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
		   AND (pr.requires_deposit IS NULL
		        OR (pr.requires_deposit = 'copper' AND prov.copper_deposit)
		        OR (pr.requires_deposit = 'tin'    AND prov.tin_deposit)
		        OR (pr.requires_deposit = 'silver' AND COALESCE(prov.silver_deposit,false))
		        OR (pr.requires_deposit = 'cedar'  AND COALESCE(prov.cedar_deposit,false)))
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
		`SELECT sg.good_key, sg.amount, sg.rate, sg.cap, sg.calc_at,
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

	// Sum allocated citizens for idle_citizens calculation.
	var totalAllocated int
	for _, c := range laborCitizens {
		totalAllocated += c
	}
	idleCitizens := laborPool - totalAllocated
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
			Citizens:       laborCitizens[key],
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

	// Get good weight. Special case: 'silver' draws from silver_amount column, not settlement_goods.
	const silverKey = "silver"
	isSilver := req.GoodKey == silverKey

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

	// Deduct from origin — silver draws from silver_amount column, others from settlement_goods.
	var deductTag interface{ RowsAffected() int64 }
	if isSilver {
		deductTag, err = tx.Exec(r.Context(),
			`UPDATE settlements SET
			     silver_amount = GREATEST(0, silver_amount + EXTRACT(EPOCH FROM (now()-silver_calc_at))/60*silver_rate - $1),
			     silver_calc_at = now()
			 WHERE id = $2
			   AND silver_amount + EXTRACT(EPOCH FROM (now()-silver_calc_at))/60*silver_rate >= $1`,
			req.Quantity, originID,
		)
	} else {
		deductTag, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			     amount = amount + EXTRACT(EPOCH FROM (now()-calc_at))/60*rate - $1,
			     calc_at = now()
			 WHERE settlement_id = $2 AND good_key = $3
			   AND amount + EXTRACT(EPOCH FROM (now()-calc_at))/60*rate >= $1`,
			req.Quantity, originID, req.GoodKey,
		)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct goods")
		return
	}
	if deductTag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "insufficient goods")
		return
	}

	arrivesAt := h.clk.Now().Add(timescale.Apply(time.Duration(travelMins * float64(time.Minute))))
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
	if err := h.scheduler.EnqueueTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
		map[string]any{
			"trade_route_id":     routeID,
			"destination_id":     req.DestinationID,
			"good_key":           req.GoodKey,
			"quantity":           req.Quantity,
			"delivered_quantity": deliveredQty,
		}, arrivesAt); err != nil {
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
			     amount = amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate - $1,
			     calc_at = now()
			 WHERE settlement_id = $2 AND good_key = $3
			   AND amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate >= $1`,
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
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 VALUES ($1, $2, $3, 0, 100, now())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settlement_goods.amount
		             + EXTRACT(EPOCH FROM (now() - settlement_goods.calc_at))/60 * settlement_goods.rate
		             + $3,
		         settlement_goods.cap),
		     calc_at = now()`,
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
		ID            uuid.UUID  `json:"id"`
		TargetID      uuid.UUID  `json:"target_id"`
		Intent        string     `json:"intent"`
		Infantry      int        `json:"infantry"`
		Chariot       int        `json:"chariot"`
		Priest        int        `json:"priest"`
		Ship          int        `json:"ship"` // galley
		EliteInfantry int        `json:"elite_infantry"`
		WarGalley     int        `json:"war_galley"`
		Merchantman   int        `json:"merchantman"`
		Resolved      bool       `json:"resolved"`
		ArrivesAt     time.Time  `json:"arrives_at"`
		CombatReport  *string    `json:"combat_report,omitempty"`
		Outgoing      bool       `json:"outgoing"`
	}
	var result []marchItem
	for rows.Next() {
		var m marchItem
		if err := rows.Scan(&m.ID, &m.TargetID, &m.Intent,
			&m.Infantry, &m.Chariot, &m.Priest, &m.Ship, &m.EliteInfantry,
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
		Infantry      int
		Chariot       int
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
	).Scan(&march.Infantry, &march.Chariot, &march.Priest,
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
		Infantry:      march.Infantry,
		Chariot:       march.Chariot,
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
	if err := h.scheduler.EnqueueTx(r.Context(), tx, worldID, events.ScheduledRecallArrival,
		payload, messengerArrivesAt); err != nil {
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
		Infantry      int `json:"infantry"`
		Chariot       int `json:"chariot"`
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
		req.Infantry, req.Chariot, req.Priest, req.Ship, req.EliteInfantry,
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
			"infantry": req.Infantry, "chariot": req.Chariot,
			"priest": req.Priest, "ship": req.Ship, "elite_infantry": req.EliteInfantry,
			"war_galley": req.WarGalley, "merchantman": req.Merchantman,
		},
	})
}

// LaborAlloc handles PUT /worlds/:worldID/provinces/:provinceID/labor.
// Body: {"citizens":{"timber":40,"grain":30,"stone":20}}
// Each value is the number of citizens assigned to that good.
// Σ citizens must not exceed labor_pool; non-producible goods are rejected with a 422.
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
		Citizens map[string]int `json:"citizens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Citizens) == 0 {
		writeError(w, http.StatusBadRequest, "invalid JSON — expected {\"citizens\":{...}}")
		return
	}

	// Verify ownership and load labor_pool.
	var settlementID uuid.UUID
	var population, sInfantry, sChariot2, sPriest, sShip, sEliteInfantry, sWarGalley2, sMerchantman2 int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT s.id, s.population, s.infantry, s.chariot, s.priest, s.ship, s.elite_infantry,
		        s.war_galley, s.merchantman
		 FROM settlements s WHERE s.province_id=$1 AND s.world_id=$2 AND s.owner_id=$3`,
		provinceID, worldID, playerID,
	).Scan(&settlementID, &population, &sInfantry, &sChariot2, &sPriest, &sShip, &sEliteInfantry,
		&sWarGalley2, &sMerchantman2); err != nil {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	homePop := sInfantry*economy.PopCosts["infantry"] +
		sChariot2*economy.PopCosts["chariot"] +
		sPriest*economy.PopCosts["priest"] +
		sShip*economy.PopCosts["ship"] +
		sEliteInfantry*economy.PopCosts["elite_infantry"] +
		sWarGalley2*economy.PopCosts["war_galley"] +
		sMerchantman2*economy.PopCosts["merchantman"]

	var transitPop int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(
		     m.infantry*$2+m.chariot*$3+m.priest*$4+m.ship*$5+m.elite_infantry*$6
		     +m.war_galley*$7+m.merchantman*$8
		 ),0)
		 FROM marching_armies m
		 JOIN settlements s ON s.province_id=m.origin_id
		 WHERE s.id=$1 AND m.resolved=false`,
		settlementID,
		economy.PopCosts["infantry"], economy.PopCosts["chariot"],
		economy.PopCosts["priest"], economy.PopCosts["ship"], economy.PopCosts["elite_infantry"],
		economy.PopCosts["war_galley"], economy.PopCosts["merchantman"],
	).Scan(&transitPop)

	laborPool := population - homePop - transitPop
	if laborPool < 0 {
		laborPool = 0
	}

	// Determine producible goods for this settlement (single source of truth).
	producible := make(map[string]bool)
	prows, err := h.pool.Query(r.Context(),
		`SELECT DISTINCT pr.good_key
		 FROM production_rules pr
		 JOIN settlements s ON s.id = $1
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE
		     (pr.terrain_type IS NULL OR pr.terrain_type = prov.terrain_type)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND prov.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND prov.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(prov.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(prov.cedar_deposit,  false)))`,
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

	// Validate: only producible goods, non-negative values; compute total.
	total := 0
	filtered := make(map[string]int)
	for key, count := range req.Citizens {
		if count < 0 {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("citizens for %s must be non-negative", key))
			return
		}
		if !producible[key] {
			writeError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("%s is not producible at this settlement (terrain/building gap) — producible here: %s",
					key, strings.Join(producibleKeys, ", ")))
			return
		}
		if count > 0 {
			filtered[key] = count
			total += count
		}
	}
	if total == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no valid producible goods in citizens")
		return
	}
	if total > laborPool {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("insufficient labor: requested %d citizens but only %d are available (labor_pool=%d, %d already committed to army/buildings/marching) — lower the total or recall/disband units first",
				total, laborPool, laborPool, population-laborPool))
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Clear existing allocations then upsert new ones.
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM settlement_labor WHERE settlement_id = $1`, settlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear labor")
		return
	}
	for key, count := range filtered {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO settlement_labor (settlement_id, good_key, citizens)
			 VALUES ($1,$2,$3) ON CONFLICT (settlement_id, good_key) DO UPDATE SET citizens=$3`,
			settlementID, key, count,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save citizens")
			return
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

	idleCitizens := laborPool - total
	writeJSON(w, http.StatusOK, map[string]any{
		"citizens":      filtered,
		"labor_pool":    laborPool,
		"idle_citizens": idleCitizens,
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
		ProvinceID          uuid.UUID `json:"province_id"`
		GoodKey             string    `json:"good_key"`
		Rate                float64   `json:"rate_per_min"`
		HomeSettlementID    uuid.UUID `json:"home_settlement_id"`
		HomeSettlementName  string    `json:"home_settlement_name"`
		Terrain             string    `json:"terrain"`
		Q                   int       `json:"q"`
		R                   int       `json:"r"`
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
