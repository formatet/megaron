package combat

// collapseSettlement — C-collapse: a city whose population reaches ≤ 100 ceases
// to exist. The last 100 inhabitants leave as a single infantry warband (garrison
// unit, 100 men) placed on the city's hex. The city is then torn down.
//
// Teardown sequence:
//   1. Idempotency guard: if settlement.state == 'collapsed' → return nil.
//   2. Spawn warband: INSERT into units (infantry, 100 men, status=garrison, same q/r).
//   3. Disband existing garrison units (they join the warband as stragglers; simplest
//      approach that leaves no orphan rows). Men were already drawn from pop at recruit
//      time so no pop is returned — they are "part of the collapse".
//   4. Tear down outpost_flows for the settlement's province (settlement_id match).
//   5. Remove kingdom membership for the owner if this was their only city, or
//      remove them from kingdom_members if they remain in one.
//   6. Dispossess: owner_id = NULL, control_type = 'occupied', kingdom_id = NULL,
//      state = 'collapsed' on the settlement row.
//   7. Mark player dispossessed in player_world_records IF this was their last city;
//      schedule Respawn if it was.
//   8. Emit CityCollapsed event.
//
// Garrison-unit decision: existing garrison units are disbanded (status='disbanded')
// so no orphan rows remain. Their men are "subsumed into" the new warband narratively;
// no pop is credited back (they were already removed from pop at recruit time).
//
// Last-city respawn decision: if this was the player's last settlement, we schedule
// a Respawn event (fire immediately) rather than calling respawnPlayer inline,
// because respawnPlayer uses pool (not tx) and creates a new TX — nesting would
// risk deadlock. The Respawn handler is idempotent (checks for existing capital).

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/gossip"
	"github.com/poleia/server/internal/unit"
)

// CollapseSettlementPayload is the scheduled_events payload for CollapseSettlement.
// Cause is "starvation" or "overmobilisation".
type CollapseSettlementPayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	WorldID      uuid.UUID `json:"world_id"`
	Cause        string    `json:"cause"`
}

// CollapseSettlementHandler processes CollapseSettlement scheduled events.
type CollapseSettlementHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	scheduler  *events.Scheduler
}

// NewCollapseSettlementHandler creates a CollapseSettlementHandler.
func NewCollapseSettlementHandler(pool *pgxpool.Pool, store *events.Store, scheduler *events.Scheduler) *CollapseSettlementHandler {
	return &CollapseSettlementHandler{pool: pool, eventStore: store, scheduler: scheduler}
}

// Handle processes a single CollapseSettlement scheduled event.
func (h *CollapseSettlementHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload CollapseSettlementPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal collapse payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := collapseSettlement(ctx, tx, h.eventStore, h.scheduler,
		payload.SettlementID, payload.WorldID, payload.Cause); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// collapseSettlement performs the full teardown of a city reduced to ≤ 100 pop.
// All DB writes use ctx and tx (G2). Respawn is enqueued via scheduler (atomic with TX).
//
// Idempotent: returns nil immediately if state == 'collapsed'.
func collapseSettlement(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	scheduler *events.Scheduler,
	settlementID, worldID uuid.UUID,
	cause string,
) error {
	// ── 1. Load settlement with FOR UPDATE (idempotency guard) ─────────────────
	var ownerID *uuid.UUID
	var cultureID, name string
	var provinceID uuid.UUID
	var state string

	if err := tx.QueryRow(ctx,
		`SELECT owner_id, culture_id, province_id, state, name
		 FROM settlements WHERE id = $1 FOR UPDATE`,
		settlementID,
	).Scan(&ownerID, &cultureID, &provinceID, &state, &name); err != nil {
		return fmt.Errorf("load settlement for collapse: %w", err)
	}

	// Idempotency: already collapsed.
	if state == "collapsed" {
		return nil
	}

	// Load province coordinates for the warband's spawn point.
	var q, r int
	if err := tx.QueryRow(ctx,
		`SELECT map_q, map_r FROM provinces WHERE id = $1`,
		provinceID,
	).Scan(&q, &r); err != nil {
		return fmt.Errorf("load province coords for collapse: %w", err)
	}

	effectiveOwnerID := uuid.Nil
	if ownerID != nil {
		effectiveOwnerID = *ownerID
	}

	// ── 2. Spawn warband: 100 infantry, positioned on city's hex ──────────────
	// status='positioned': the unit is on the map but not inside a functioning
	// settlement (the city is about to be collapsed). settlement_id stays NULL
	// so no FK to a dead settlement is created.
	var warbandID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO units
		   (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'infantry', 'land', 100, 0, 'positioned', $3, $4)
		 RETURNING id`,
		worldID, ownerID, q, r,
	).Scan(&warbandID); err != nil {
		return fmt.Errorf("spawn warband unit: %w", err)
	}

	// ── 3. Disband existing garrison units ─────────────────────────────────────
	// Garrison units are disbanded (status → disbanded) so no orphan rows remain.
	// The warband was just inserted with status='positioned' and no settlement_id,
	// so it is not in this result set.
	// Naval units with cargo: disband cargo units too.
	garrisonRows, err := tx.Query(ctx,
		`SELECT id, cargo_unit_id FROM units
		 WHERE settlement_id = $1 AND status = 'garrison'`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("load garrison units for disband: %w", err)
	}
	var garrisonIDs []uuid.UUID
	var cargoIDs []uuid.UUID
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
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1`,
			gid,
		); err != nil {
			slog.Warn("collapse: disband garrison unit", "unit", gid, "err", err)
		}
	}
	for _, cid := range cargoIDs {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', updated_at = now()
			 WHERE id = $1 AND status = 'embarked'`,
			cid,
		); err != nil {
			slog.Warn("collapse: disband cargo unit", "unit", cid, "err", err)
		}
	}

	// ── 4. Tear down outpost_flows for this settlement ─────────────────────────
	// outpost_flows.settlement_id points to the feeding settlement.
	// We need to subtract the rates from settlement_goods before deleting.
	flowRows, err := tx.Query(ctx,
		`SELECT province_id, good_key, rate FROM outpost_flows WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("load outpost flows for collapse: %w", err)
	}
	type outpostFlow struct {
		provinceID uuid.UUID
		goodKey    string
		rate       float64
	}
	var outpostFlows []outpostFlow
	for flowRows.Next() {
		var f outpostFlow
		if scanErr := flowRows.Scan(&f.provinceID, &f.goodKey, &f.rate); scanErr == nil {
			outpostFlows = append(outpostFlows, f)
		}
	}
	flowRows.Close()

	for _, f := range outpostFlows {
		// Settle-then-subtract (same pattern as teardownOutpost in arrival.go).
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET
			     amount  = LEAST(cap, settled(amount, rate, calc_tick)),
			     rate    = GREATEST(0, rate - $3),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $2`,
			settlementID, f.goodKey, f.rate,
		); err != nil {
			slog.Warn("collapse: subtract outpost flow from settlement goods",
				"settlement", settlementID, "good", f.goodKey, "err", err)
		}
		// Free the province.
		if _, err := tx.Exec(ctx,
			`UPDATE provinces SET
			     territory_state = 'free',
			     owner_id        = NULL,
			     outpost_feeds   = NULL,
			     garrison_strength = 0
			 WHERE id = $1`,
			f.provinceID,
		); err != nil {
			slog.Warn("collapse: free outpost province",
				"province", f.provinceID, "err", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM outpost_flows WHERE settlement_id = $1`,
		settlementID,
	); err != nil {
		return fmt.Errorf("delete outpost flows for collapse: %w", err)
	}

	// ── 5. Kingdom membership ──────────────────────────────────────────────────
	// Remove the owner from kingdom_members if they were in one. The settlement's
	// kingdom_id is nulled in step 6; here we clean the membership table.
	if effectiveOwnerID != uuid.Nil {
		if _, err := tx.Exec(ctx,
			`DELETE FROM kingdom_members WHERE player_id = $1`,
			effectiveOwnerID,
		); err != nil {
			slog.Warn("collapse: remove kingdom membership", "player", effectiveOwnerID, "err", err)
		}
	}

	// ── 6. Dispossess settlement ───────────────────────────────────────────────
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET
		   owner_id     = NULL,
		   control_type = 'occupied',
		   kingdom_id   = NULL,
		   state        = 'collapsed',
		   updated_at   = now()
		 WHERE id = $1`,
		settlementID,
	); err != nil {
		return fmt.Errorf("dispossess collapsed settlement: %w", err)
	}

	// Update province: release controller.
	if _, err := tx.Exec(ctx,
		`UPDATE provinces SET
		   territory_state = 'free',
		   controller_id   = NULL
		 WHERE id = $1`,
		provinceID,
	); err != nil {
		slog.Warn("collapse: free province", "province", provinceID, "err", err)
	}

	// Recompute production (owner changed, rates will be stale).
	_ = economy.RecomputeProduction(ctx, tx, settlementID)

	// Rumor: a city collapsing is major news — hearsay, several hops
	// (temenos_gossip.md PASS 2b). Best-effort — never fail the collapse over gossip.
	if err := gossip.Broadcast(ctx, tx, worldID, settlementID, "military",
		name+" has fallen ("+cause+").", 6,
		gossip.ImportanceMajor, settlementID, ""); err != nil {
		slog.Warn("collapse: broadcast gossip", "settlement", settlementID, "err", err)
	}

	// ── 7. Last-city check → Respawn ───────────────────────────────────────────
	isLastCity := false
	if effectiveOwnerID != uuid.Nil {
		var remaining int
		_ = tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM settlements
			 WHERE owner_id = $1 AND world_id = $2 AND state != 'collapsed'`,
			effectiveOwnerID, worldID,
		).Scan(&remaining)
		isLastCity = (remaining == 0)

		// Mark dispossessed regardless — the player has lost this city.
		if _, err := tx.Exec(ctx,
			`UPDATE player_world_records
			   SET status = 'dispossessed', settlement_id = NULL
			 WHERE player_id = $1 AND world_id = $2`,
			effectiveOwnerID, worldID,
		); err != nil {
			slog.Warn("collapse: mark dispossessed", "player", effectiveOwnerID, "err", err)
		}

		if isLastCity {
			// Schedule Respawn after 12 ticks (~12 game hours). Atomic with the collapse TX.
			var currentTick int
			_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
			if err := scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledRespawn,
				RespawnPayload{
					PlayerID: effectiveOwnerID,
					WorldID:  worldID,
					Culture:  cultureID,
				},
				currentTick+12,
			); err != nil {
				slog.Warn("collapse: could not schedule respawn",
					"player", effectiveOwnerID, "err", err)
			}
		}
	}

	// ── 8. Emit CityCollapsed event ────────────────────────────────────────────
	_, _ = eventStore.Append(ctx, settlementID, events.StreamProvince, unit.EventCityCollapsed,
		unit.CityCollapsedPayload{
			SettlementID:   settlementID,
			OwnerID:        effectiveOwnerID,
			WorldID:        worldID,
			WarbandUnitID:  warbandID,
			Q:              q,
			R:              r,
			Cause:          cause,
			LastSettlement: isLastCity,
		}, worldID, nil)

	// Notify in logs for observability.
	slog.Info("city collapsed",
		"settlement", settlementID,
		"owner", effectiveOwnerID,
		"warband", warbandID,
		"q", q, "r", r,
		"cause", cause,
		"last_city", isLastCity,
	)

	return nil
}
