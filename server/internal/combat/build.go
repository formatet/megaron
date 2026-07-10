package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/gossip"
	"github.com/poleia/server/internal/province"
)

// BuildCompletePayload is the scheduled event payload for a completed building.
type BuildCompletePayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	BuildQueueID uuid.UUID `json:"build_queue_id"`
	BuildingType string    `json:"building_type"`
}

// BuildCompleteHandler resolves a completed building construction.
type BuildCompleteHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
}

// NewBuildCompleteHandler creates a BuildCompleteHandler.
func NewBuildCompleteHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster) *BuildCompleteHandler {
	return &BuildCompleteHandler{pool: pool, eventStore: eventStore, hub: hub}
}

// Handle processes a ScheduledBuildComplete event.
func (h *BuildCompleteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p BuildCompletePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal build payload: %w", err)
	}

	spec, ok := province.BuildingSpecs[province.BuildingType(p.BuildingType)]
	if !ok {
		return fmt.Errorf("unknown building type: %s", p.BuildingType)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Verify queue entry still exists (idempotency guard).
	var existingID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT id FROM build_queue WHERE id = $1 AND settlement_id = $2`,
		p.BuildQueueID, p.SettlementID,
	).Scan(&existingID)
	if err != nil {
		return nil // Already resolved.
	}

	// Insert completed building into settlement.
	_, err = tx.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, $2, 1)
		 ON CONFLICT (settlement_id, building_type)
		 DO UPDATE SET level = buildings.level + 1`,
		p.SettlementID, p.BuildingType,
	)
	if err != nil {
		return fmt.Errorf("insert building: %w", err)
	}

	// Determine goods this building just unlocked that are present in the
	// catchment but have no labor assigned yet. We do this BEFORE RecomputeProduction
	// so auto-allocation is reflected in the rates written by that call.
	var unlockedGoods []string
	urows, uerr := tx.Query(ctx,
		`SELECT DISTINCT pr.good_key
		 FROM production_rules pr
		 JOIN settlements s ON s.id = $1
		 JOIN provinces prov ON prov.id = s.province_id
		 JOIN map_tiles mt ON mt.world_id = s.world_id
		     AND mt.terrain NOT IN ('deep_sea','coastal_sea')
		     AND (
		         (mt.q = prov.map_q   AND mt.r = prov.map_r  ) OR
		         (mt.q = prov.map_q+1 AND mt.r = prov.map_r  ) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r  ) OR
		         (mt.q = prov.map_q   AND mt.r = prov.map_r+1) OR (mt.q = prov.map_q   AND mt.r = prov.map_r-1) OR
		         (mt.q = prov.map_q+1 AND mt.r = prov.map_r-1) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r+1)
		     )
		 WHERE pr.building_type = $2
		   AND (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		   AND (NOT pr.requires_coastal OR mt.coastal)
		   AND (pr.requires_deposit IS NULL
		        OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		        OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		        OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		        OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit,  false)))
		   AND NOT EXISTS (
		        SELECT 1 FROM settlement_labor sl
		        WHERE sl.settlement_id = s.id AND sl.good_key = pr.good_key AND sl.weight > 0)`,
		p.SettlementID, p.BuildingType,
	)
	if uerr == nil {
		for urows.Next() {
			var k string
			if urows.Scan(&k) == nil {
				unlockedGoods = append(unlockedGoods, k)
			}
		}
		urows.Close()
	}

	// Auto-allocate a starting labor slice to newly unlocked goods (skims grain if needed).
	// This runs before RecomputeProduction so the new weights are reflected in the rates.
	autoAllocated, autoErr := economy.AutoAllocateUnlocked(ctx, tx, p.SettlementID, unlockedGoods)
	if autoErr != nil {
		slog.Warn("auto-allocate labor after build", "settlement", p.SettlementID, "building", p.BuildingType, "err", autoErr)
		// Non-fatal: fall through so the build still completes.
		autoAllocated = nil
	}

	// Update settlement_goods production rates via the central labor-allocation helper.
	// This DRYs up the rate-UPSERT that was previously duplicated here and in join.go.
	if err := economy.RecomputeProduction(ctx, tx, p.SettlementID); err != nil {
		return fmt.Errorf("recompute production after build: %w", err)
	}

	// Apply kharis rate bonus to settlement columns.
	if spec.KharisRate > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE player_world_records SET
			   kharis_amount = settled(kharis_amount, kharis_rate, kharis_calc_tick),
			   kharis_rate = kharis_rate + $1,
			   kharis_calc_tick = current_world_tick()
			 WHERE player_id = (SELECT owner_id FROM settlements WHERE id = $2)
			   AND world_id = (SELECT world_id FROM settlements WHERE id = $2)`,
			spec.KharisRate, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update kharis rate: %w", err)
		}
	}
	if spec.WallsBonus > 0 {
		_, err = tx.Exec(ctx,
			`UPDATE settlements SET wall_level = LEAST(wall_level + $1, 3) WHERE id = $2`,
			spec.WallsBonus, p.SettlementID)
		if err != nil {
			return fmt.Errorf("update wall level: %w", err)
		}
	}

	// Rumor: a completed mine is minor news — witnessed only by nearby owners
	// (temenos_gossip.md PASS 2b). Subject = this settlement, hint = the ore, so
	// it registers as rumour-known ("rich in copper") for anyone who hears of it
	// without having seen it. Best-effort — never fail the build over gossip.
	if p.BuildingType == string(province.BuildingMine) || p.BuildingType == string(province.BuildingSilverMine) {
		ore := "silver"
		if p.BuildingType == string(province.BuildingMine) {
			// "mine" gates on copper OR tin present in the catchment (see
			// province.go's build-time deposit gate) — prefer copper as the hint
			// when both are present.
			ore = "tin"
			var hasCopper bool
			if err := tx.QueryRow(ctx,
				`SELECT EXISTS(
				   SELECT 1 FROM map_tiles mt
				   JOIN settlements s ON s.id = $1
				   JOIN provinces prov ON prov.id = s.province_id
				   WHERE mt.world_id = s.world_id
				     AND mt.terrain NOT IN ('deep_sea','coastal_sea')
				     AND (
				         (mt.q = prov.map_q   AND mt.r = prov.map_r  ) OR
				         (mt.q = prov.map_q+1 AND mt.r = prov.map_r  ) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r  ) OR
				         (mt.q = prov.map_q   AND mt.r = prov.map_r+1) OR (mt.q = prov.map_q   AND mt.r = prov.map_r-1) OR
				         (mt.q = prov.map_q+1 AND mt.r = prov.map_r-1) OR (mt.q = prov.map_q-1 AND mt.r = prov.map_r+1)
				     )
				     AND mt.copper_deposit)`,
				p.SettlementID,
			).Scan(&hasCopper); err == nil && hasCopper {
				ore = "copper"
			}
		}
		if err := gossip.Broadcast(ctx, tx, e.WorldID, p.SettlementID, "economy",
			"A "+ore+" mine has opened.", 6,
			gossip.ImportanceMinor, p.SettlementID, ore); err != nil {
			slog.Warn("build complete: broadcast gossip", "settlement", p.SettlementID, "err", err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM build_queue WHERE id = $1`, p.BuildQueueID); err != nil {
		return fmt.Errorf("delete build queue entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if _, err := h.eventStore.Append(ctx, p.SettlementID, events.StreamProvince, "BuildComplete", map[string]any{
		"building_type": p.BuildingType,
	}, e.WorldID, nil); err != nil {
		slog.Error("record BuildComplete event", "err", err)
	}

	if h.hub != nil {
		var ownerID uuid.UUID
		_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, p.SettlementID).Scan(&ownerID)
		body := map[string]any{
			"settlement_id": p.SettlementID,
			"building_type": p.BuildingType,
		}
		if len(autoAllocated) > 0 {
			body["unlocked_goods"] = autoAllocated
			body["hint"] = fmt.Sprintf(
				"%s is built — 15%% labor auto-allocated to %s to start production. Adjust with /labor if you want more.",
				p.BuildingType, strings.Join(autoAllocated, ", "))
		} else if len(unlockedGoods) > 0 {
			// Auto-alloc failed (no capacity) — give the old manual hint.
			body["unlocked_goods"] = unlockedGoods
			body["hint"] = fmt.Sprintf(
				"%s is built but idle. How large a share of the population should work it? Set a labor percent for %s to begin production.",
				p.BuildingType, strings.Join(unlockedGoods, ", "))
		}
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, ownerID, "BuildComplete", 4, body)
	}
	slog.Info("build complete", "settlement", p.SettlementID, "building", p.BuildingType)
	return nil
}
