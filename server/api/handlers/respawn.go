package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/religion"
)

// RespawnHandler processes Respawn scheduled events.
// When a Wanax loses their last settlement they are reborn on a new province
// with the same player_id, culture, and starter resources.
type RespawnHandler struct {
	pool *pgxpool.Pool
}

// NewRespawnHandler creates a RespawnHandler.
func NewRespawnHandler(pool *pgxpool.Pool) *RespawnHandler {
	return &RespawnHandler{pool: pool}
}

// Handle processes a single Respawn scheduled event.
func (h *RespawnHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload combat.RespawnPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal respawn payload: %w", err)
	}

	// Idempotency: skip if the player already has an active settlement.
	var existing int
	_ = h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlements WHERE world_id = $1 AND owner_id = $2`,
		payload.WorldID, payload.PlayerID,
	).Scan(&existing)
	if existing > 0 {
		return nil
	}

	if err := respawnPlayer(ctx, h.pool, payload.PlayerID, payload.WorldID, payload.Culture); err != nil {
		return fmt.Errorf("respawn player %s: %w", payload.PlayerID, err)
	}
	slog.Info("player respawned", "player", payload.PlayerID, "world", payload.WorldID, "culture", payload.Culture)
	return nil
}

// respawnPlayer finds a free province and plants a new capital settlement for the player.
// Uses the same starter goods as join.go so the player can begin again.
func respawnPlayer(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID, culture string) error {
	// Find an unclaimed tile.
	var q, r int
	var terrainType string
	var copperDeposit, tinDeposit bool
	err := pool.QueryRow(ctx,
		`SELECT mt.q, mt.r, mt.terrain, mt.copper_deposit, mt.tin_deposit
		 FROM map_tiles mt
		 LEFT JOIN provinces p ON p.world_id = mt.world_id AND p.map_q = mt.q AND p.map_r = mt.r
		 WHERE mt.world_id = $1 AND p.id IS NULL AND mt.terrain IN ('plains','coast','hills')
		 ORDER BY RANDOM() LIMIT 1`,
		worldID,
	).Scan(&q, &r, &terrainType, &copperDeposit, &tinDeposit)
	if err != nil {
		return fmt.Errorf("no free tiles for respawn: %w", err)
	}

	// Kharis rate from pantheon geography (same as join.go).
	regions := religion.DefaultPantheonRegions()
	var maxPower float64
	for _, reg := range regions {
		if p := religion.LocalPower(reg, q, r); p > maxPower {
			maxPower = p
		}
	}
	kharisRate := maxPower * 0.05

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var provinceID uuid.UUID
	if err = tx.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state, copper_deposit, tin_deposit)
		 VALUES ($1, $2, $3, $4, 'controlled', $5, $6) RETURNING id`,
		worldID, q, r, terrainType, copperDeposit, tinDeposit,
	).Scan(&provinceID); err != nil {
		return fmt.Errorf("create province: %w", err)
	}

	name := province.SettlementNameForCulture(culture)

	var settlementID uuid.UUID
	if err = tx.QueryRow(ctx,
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital,
		  kharis_rate, kharis_calc_at)
		 VALUES ($1,$2,$3,$4,$5,'capital',true,$6,now())
		 RETURNING id`,
		worldID, provinceID, name, culture, playerID, kharisRate,
	).Scan(&settlementID); err != nil {
		return fmt.Errorf("create settlement: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE provinces SET controller_id = $1 WHERE id = $2`,
		settlementID, provinceID,
	); err != nil {
		return fmt.Errorf("link province: %w", err)
	}

	// Seed goods: zero row for every good, then starter amounts + production rules.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, g.key,
		        CASE g.key WHEN 'grain' THEN 150 WHEN 'cedar' THEN 120 WHEN 'stone' THEN 120 ELSE 0 END,
		        0,
		        CASE g.key WHEN 'grain' THEN 1000 WHEN 'cedar' THEN 500 WHEN 'stone' THEN 1000
		                   WHEN 'copper' THEN 300 WHEN 'tin' THEN 300 ELSE 200 END,
		        now()
		 FROM goods g ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		settlementID,
	); err != nil {
		return fmt.Errorf("seed goods: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, pr.good_key, 0, pr.rate_per_min,
		        CASE pr.good_key WHEN 'grain' THEN 1000 WHEN 'cedar' THEN 500 WHEN 'stone' THEN 1000
		                        WHEN 'copper' THEN 300 WHEN 'tin' THEN 300 ELSE 200 END,
		        now()
		 FROM production_rules pr
		 WHERE pr.building_type IS NULL AND pr.terrain_type = $2
		   AND (pr.requires_deposit IS NULL
		        OR (pr.requires_deposit = 'copper' AND $3)
		        OR (pr.requires_deposit = 'tin'    AND $4))
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     rate = settlement_goods.rate + EXCLUDED.rate`,
		settlementID, terrainType, copperDeposit, tinDeposit,
	); err != nil {
		return fmt.Errorf("init production: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, settlement_id, status)
		 VALUES ($1, $2, $3, 'active')
		 ON CONFLICT (player_id, world_id) DO UPDATE SET settlement_id = EXCLUDED.settlement_id, status = 'active'`,
		playerID, worldID, settlementID,
	); err != nil {
		return fmt.Errorf("update records: %w", err)
	}

	return tx.Commit(ctx)
}
