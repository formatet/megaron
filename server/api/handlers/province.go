package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
)

// ProvinceHandler handles HTTP requests for province endpoints.
type ProvinceHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
}

// NewProvinceHandler creates a ProvinceHandler.
func NewProvinceHandler(pool *pgxpool.Pool, scheduler *events.Scheduler) *ProvinceHandler {
	return &ProvinceHandler{pool: pool, scheduler: scheduler}
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
	}

	sett, err := loadSettlementByProvince(r.Context(), h.pool, provinceID, worldID)
	if err == nil {
		now := time.Now()
		resp["settlement"] = map[string]any{
			"id":         sett.ID,
			"name":       sett.Name,
			"owner_id":   sett.OwnerID,
			"kingdom_id": sett.KingdomID,
			"culture":    sett.CultureID,
			"state":      sett.State,
			"population": sett.Population,
			"walls":      sett.WallLevel,
			"loyalty":    sett.Loyalty,
			"resources":  sett.Resources.Snapshot(now),
			"army":       sett.Army,
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
		TargetID string `json:"target_id"`
		Intent   string `json:"intent"`
		Infantry int    `json:"infantry"`
		Cavalry  int    `json:"cavalry"`
		Catapult int    `json:"catapult"`
		Priest   int    `json:"priest"`
		Ship     int    `json:"ship"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	targetID, err := uuid.Parse(req.TargetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid target ID")
		return
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

	now := time.Now()
	arrivesAt := now.Add(time.Duration(dist) * time.Hour)

	army := province.ArmyComposition{
		Infantry: req.Infantry,
		Cavalry:  req.Cavalry,
		Catapult: req.Catapult,
		Priest:   req.Priest,
		Ship:     req.Ship,
	}

	var marchID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO marching_armies
		 (world_id, origin_id, target_id, infantry, cavalry, catapult, priest, ship, intent, departs_at, arrives_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		worldID, sourceID, targetID,
		army.Infantry, army.Cavalry, army.Catapult, army.Priest, army.Ship,
		req.Intent, now, arrivesAt,
	).Scan(&marchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not send army")
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

	// Deduct resources atomically from settlement.
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE settlements SET
		   lumber_amount = lumber_amount
		     + (EXTRACT(EPOCH FROM (now() - lumber_calc_at))/60 * lumber_rate) - $1,
		   lumber_calc_at = now(),
		   stone_amount = stone_amount
		     + (EXTRACT(EPOCH FROM (now() - stone_calc_at))/60 * stone_rate) - $2,
		   stone_calc_at = now(),
		   iron_amount = iron_amount
		     + (EXTRACT(EPOCH FROM (now() - iron_calc_at))/60 * iron_rate) - $3,
		   iron_calc_at = now()
		 WHERE id = $4
		   AND lumber_amount + (EXTRACT(EPOCH FROM (now() - lumber_calc_at))/60 * lumber_rate) >= $1
		   AND stone_amount  + (EXTRACT(EPOCH FROM (now() - stone_calc_at))/60  * stone_rate)  >= $2
		   AND iron_amount   + (EXTRACT(EPOCH FROM (now() - iron_calc_at))/60   * iron_rate)   >= $3`,
		spec.CostLumber, spec.CostStone, spec.CostIron, settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct resources")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "insufficient resources")
		return
	}

	completeAt := time.Now().Add(spec.Duration)
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

	totalFood := spec.CostFood * float64(req.Count)
	totalIron := spec.CostIron * float64(req.Count)
	totalLumber := spec.CostLumber * float64(req.Count)
	totalKharis := spec.CostKharis * float64(req.Count)

	// Deduct resources atomically from settlement.
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE settlements SET
		   food_amount = food_amount
		     + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60 * food_rate) - $1,
		   food_calc_at = now(),
		   iron_amount = iron_amount
		     + (EXTRACT(EPOCH FROM (now() - iron_calc_at))/60 * iron_rate) - $2,
		   iron_calc_at = now(),
		   lumber_amount = lumber_amount
		     + (EXTRACT(EPOCH FROM (now() - lumber_calc_at))/60 * lumber_rate) - $3,
		   lumber_calc_at = now(),
		   kharis_amount = kharis_amount
		     + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) - $4,
		   kharis_calc_at = now()
		 WHERE id = $5
		   AND food_amount   + (EXTRACT(EPOCH FROM (now() - food_calc_at))/60   * food_rate)   >= $1
		   AND iron_amount   + (EXTRACT(EPOCH FROM (now() - iron_calc_at))/60   * iron_rate)   >= $2
		   AND lumber_amount + (EXTRACT(EPOCH FROM (now() - lumber_calc_at))/60 * lumber_rate) >= $3
		   AND kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate) >= $4`,
		totalFood, totalIron, totalLumber, totalKharis, settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not deduct resources: %v", err))
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "insufficient resources")
		return
	}

	completeAt := time.Now().Add(spec.Duration * time.Duration(req.Count))

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
