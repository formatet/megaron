package combat

// collapseSettlement — C-collapse: a city whose population reaches ≤ 100 ceases
// to exist. The last 100 inhabitants leave as a single spearman warband (100 men)
// placed on the city's hex. The city is then torn down.
//
// Teardown sequence:
//   1. Idempotency guard: if settlement.state == 'collapsed' → return nil.
//   2. Spawn warband: INSERT into units (spearman, 100 men, positioned, same q/r).
//   3. Disband existing garrison units (they join the warband as stragglers; simplest
//      approach that leaves no orphan rows). Men were already drawn from pop at recruit
//      time so no pop is returned — they are "part of the collapse".
//   4. Remove kingdom membership for the owner if this was their only city, or
//      remove them from kingdom_members if they remain in one.
//   5. Dispossess: owner_id = NULL, control_type = 'occupied', kingdom_id = NULL,
//      state = 'collapsed' on the settlement row.
//   6. Succession: promote the highest-loyalty survivor to capital, or — if this
//      was the player's last city — mark them dispossessed for the epitaph.
//   7. Emit CityCollapsed event.
//
// Garrison-unit decision: existing garrison units are disbanded (status='disbanded')
// so no orphan rows remain. Their men are "subsumed into" the new warband narratively;
// no pop is credited back (they were already removed from pop at recruit time).
//
// Succession decision: metropolis-succession and game-over are handled by the
// shared handleOwnerCityLoss (succession.go), so all three loss paths (collapse and
// the two conquest paths) behave identically. Respawn was removed 2026-07-10.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	hub        Broadcaster
}

// NewCollapseSettlementHandler creates a CollapseSettlementHandler. hub may be
// nil (tests) — every NotifyPlayer call is nil-guarded, matching the other
// combat handlers.
func NewCollapseSettlementHandler(pool *pgxpool.Pool, store *events.Store, scheduler *events.Scheduler, hub Broadcaster) *CollapseSettlementHandler {
	return &CollapseSettlementHandler{pool: pool, eventStore: store, scheduler: scheduler, hub: hub}
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

	if err := collapseSettlement(ctx, tx, h.eventStore, h.scheduler, h.hub,
		payload.SettlementID, payload.WorldID, payload.Cause); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// collapseSettlement performs the full teardown of a city reduced to ≤ 100 pop.
// All DB writes use ctx and tx (G2). Succession/game-over is handled inline in
// step 6 via handleOwnerCityLoss (succession.go).
//
// Idempotent: returns nil immediately if state == 'collapsed'.
func collapseSettlement(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	scheduler *events.Scheduler,
	hub Broadcaster,
	settlementID, worldID uuid.UUID,
	cause string,
) error {
	// ── 1. Load settlement with FOR UPDATE (idempotency guard) ─────────────────
	var ownerID *uuid.UUID
	var name string
	var provinceID uuid.UUID
	var state string

	if err := tx.QueryRow(ctx,
		`SELECT owner_id, province_id, state, name
		 FROM settlements WHERE id = $1 FOR UPDATE`,
		settlementID,
	).Scan(&ownerID, &provinceID, &state, &name); err != nil {
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

	// ── 2. Spawn warband: 100 spearmen, positioned on city's hex ──────────────
	// status='positioned': the unit is on the map but not inside a functioning
	// settlement (the city is about to be collapsed). settlement_id stays NULL
	// so no FK to a dead settlement is created.
	// Type 'spearman', NOT 'infantry': the canonical taxonomy (unit/model.go)
	// has no bare-infantry type — a phantom type is invisible in the army
	// aggregate (db.go), undisbandable (the disband handler's type list), and
	// unknown to strength/upkeep tables. Found live 2026-07-13.
	var warbandID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO units
		   (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', $3, $4)
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

	// ── 4. Kingdom membership ──────────────────────────────────────────────────
	// Remove the owner from kingdom_members ONLY if this was their last city.
	// Losing one city while others survive triggers succession (step 6 promotes a
	// new capital) — the player lives on and must KEEP their kingdom seat. The
	// NOT EXISTS excludes the collapsing settlement (still owned here; owner_id is
	// nulled in step 5) so a survivor's membership is untouched. Latent until
	// kingdoms are re-enabled, but the unconditional delete was simply wrong.
	if effectiveOwnerID != uuid.Nil {
		if _, err := tx.Exec(ctx,
			`DELETE FROM kingdom_members WHERE player_id = $1
			   AND NOT EXISTS (
			       SELECT 1 FROM settlements s
			       WHERE s.owner_id = $1 AND s.id <> $2
			         AND s.state NOT IN ('collapsed', 'razed')
			   )`,
			effectiveOwnerID, settlementID,
		); err != nil {
			slog.Warn("collapse: remove kingdom membership", "player", effectiveOwnerID, "err", err)
		}
	}

	// ── 5. Dispossess settlement ───────────────────────────────────────────────
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
	// NOT best-effort despite the discarded error it used to be: this runs inside
	// the caller's transaction, so a failed statement in here poisons every
	// statement after it (25P02, "current transaction is aborted") and the real
	// cause is invisible. Log the cause; the caller still fails, but now it says why.
	if err := economy.RecomputeProduction(ctx, tx, settlementID); err != nil {
		slog.Error("collapse: recompute production failed — this transaction is now aborted",
			"settlement", settlementID, "err", err)
	}

	// Rumor: a city collapsing is major news — hearsay, several hops
	// (temenos_gossip.md PASS 2b). Best-effort — never fail the collapse over gossip.
	if err := gossip.Broadcast(ctx, tx, worldID, settlementID, "military",
		name+" has fallen ("+cause+").", 6,
		gossip.ImportanceMajor, settlementID, ""); err != nil {
		slog.Warn("collapse: broadcast gossip", "settlement", settlementID, "err", err)
	}

	// ── 6. Succession / game-over ──────────────────────────────────────────────
	// owner_id was cleared in step 5, so the fallen city no longer counts as the
	// player's. handleOwnerCityLoss decides between metropolis succession (promote
	// the highest-loyalty survivor) and game-over (last city → dispossessed, with
	// last_settlement_id anchored for the epitaph crawl).
	isLastCity := false
	if effectiveOwnerID != uuid.Nil {
		gameOver, lossErr := handleOwnerCityLoss(ctx, tx, effectiveOwnerID, worldID, settlementID)
		if lossErr != nil {
			return fmt.Errorf("handle owner city loss: %w", lossErr)
		}
		isLastCity = gameOver
	}

	// ── 7. Emit CityCollapsed event ────────────────────────────────────────────
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

	// Owner notification (r6 audit, 2026-07-24): before this, a collapse's only
	// player-reachable signals were the gossip.Broadcast above — which reaches
	// NEARBY settlement owners, not necessarily the affected Wanax, and reaches
	// no one at all when this was their last city — and the CityCollapsed event,
	// which is audit-only (chronicle/province-stream, never surfaced to a
	// client). A Wanax whose garrison was just disbanded into the warband (step
	// 3) and whose city vanished (step 5) had no notification connecting either
	// to this. level 1 (top priority): losing a settlement outranks losing a
	// single unit (upkeep.go's UnitAttrition/UnitDeserted use level 2/3).
	if hub != nil && effectiveOwnerID != uuid.Nil {
		_ = hub.NotifyPlayer(ctx, worldID, effectiveOwnerID, unit.EventCityCollapsed, 1, map[string]any{
			"settlement_id":   settlementID,
			"name":            name,
			"cause":           cause,
			"warband_unit_id": warbandID,
			"q":               q,
			"r":               r,
			"last_settlement": isLastCity,
		})
	}

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
