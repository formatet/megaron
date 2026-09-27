package combat

// R6 (megaron_plan_skeppsuppdrag_landsatt.md): the one-time deploy transition.
//
// Before this slice, a naval unit could sit status='positioned' at sea
// indefinitely and still take fresh orders (march/stance/recall/…). R3 closes
// that: a ship not docked at its own port (status='garrison') takes no
// orders at all. Any ship already 'positioned' at sea when this deploys would
// otherwise be stranded forever — unreachable by any order, with no mission
// of its own driving it home. This sweep runs once at server startup and
// sends each one home, exactly like any other return leg (dispatchReturnHome,
// reusing the explore_return machinery) — except a ship actively on patrol
// (a pending ScheduledSentryReturn) is left alone; its own timer already
// returns it home on schedule, unchanged.
//
// R5 (megaron_plan_ordna_passage.md, 3b-3): a ship holding march_intent=
// 'passage_wait' is ALSO left alone — it is not stranded, it is waiting in a
// port it is already standing off, for a runner whose homeward leg is
// pending. Every restart would otherwise send it home before that runner
// ever gets its ride, silently breaking "ordna passage" on every deploy.
// PassageScanHandler's own release phase (R4) is its only path home.
//
// Idempotent: it only ever selects status='positioned' naval units, and
// dispatchReturnHome flips that to 'marching' — a second run (a restart, or
// calling this twice) finds nothing left to do.

import (
	"context"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SweepShipsAtSeaOnDeploy finds every naval unit still 'positioned' at sea
// with no active patrol timer and sends each one home once. Returns how many
// it swept; a per-unit failure is logged and skipped rather than aborting the
// whole sweep (a rare unit should never block every other one from coming
// home at startup).
func SweepShipsAtSeaOnDeploy(ctx context.Context, pool *pgxpool.Pool, h *UnitArrivalHandler) (swept int, err error) {
	// world_id must be an ACTIVE world (one_active_world) — a test suite (or a
	// world reseed) leaves plenty of positioned naval units behind in
	// archived worlds, and this is a one-time startup sweep for the live
	// world, not a historical cleanup.
	rows, qErr := pool.Query(ctx,
		`SELECT u.id FROM units u
		 JOIN worlds w ON w.id = u.world_id
		 WHERE w.status = 'active' AND u.category = 'naval' AND u.status = 'positioned'
		   AND (u.march_intent IS DISTINCT FROM 'passage_wait')
		   AND NOT EXISTS (
		     SELECT 1 FROM scheduled_events se
		     WHERE se.event_type = $1 AND se.processed_at IS NULL
		       AND (se.payload->>'unit_id')::uuid = u.id)`,
		string(events.ScheduledSentryReturn),
	)
	if qErr != nil {
		return 0, fmt.Errorf("sweep ships at sea: query: %w", qErr)
	}
	var shipIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("sweep ships at sea: scan: %w", scanErr)
		}
		shipIDs = append(shipIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sweep ships at sea: rows: %w", err)
	}

	for _, id := range shipIDs {
		sent, swErr := sweepOneShip(ctx, pool, h, id)
		if swErr != nil {
			slog.Error("sweep ships at sea: unit failed, continuing", "unit", id, "err", swErr)
			continue
		}
		if sent {
			swept++
		}
	}
	if swept > 0 || len(shipIDs) > 0 {
		slog.Info("sweep ships at sea on deploy: complete", "candidates", len(shipIDs), "sent_home", swept)
	}
	return swept, nil
}

// sweepOneShip re-reads and locks one candidate ship inside its own
// transaction, then either dispatches it home or — for the R6 stranded
// exception (transport.StrandShip: the owner holds no active settlement at
// all, so there is nowhere to route it) — leaves it exactly as it is. sent
// reports whether a return leg was actually dispatched.
func sweepOneShip(ctx context.Context, pool *pgxpool.Pool, h *UnitArrivalHandler, unitID uuid.UUID) (sent bool, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var u unitRow
	var curQ, curR *int
	var worldID uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id, world_id, owner_id, type, category, size, crew, cargo_unit_id,
		        status, q, r, target_q, target_r, stance, march_intent, colony_name, home_settlement_id, capture_mode,
		        carried_silver, provisions, land_target_q, land_target_r, land_cargo_intent
		 FROM units WHERE id = $1 AND status = 'positioned' AND category = 'naval'
		   AND (march_intent IS DISTINCT FROM 'passage_wait') FOR UPDATE`,
		unitID,
	).Scan(&u.id, &worldID, &u.ownerID, &u.utype, &u.category, &u.size, &u.crew, &u.cargoUnitID,
		&u.status, &curQ, &curR, &u.targetQ, &u.targetR, &u.stance, &u.marchIntent, &u.colonyName, &u.homeSettlementID, &u.captureMode,
		&u.carriedSilver, &u.provisions, &u.landTargetQ, &u.landTargetR, &u.landCargoIntent); err != nil {
		return false, nil // already changed since the candidate scan — idempotent no-op
	}
	if curQ == nil || curR == nil {
		return false, fmt.Errorf("positioned naval unit %s has no q/r", unitID)
	}
	u.q, u.r = *curQ, *curR

	home, found, hErr := nearestOwnedSettlement(ctx, tx, worldID, u.ownerID, u.q, u.r)
	if hErr != nil {
		return false, fmt.Errorf("resolve home settlement: %w", hErr)
	}
	if !found {
		// R6's stranded exception: nowhere to route home to — leave it be.
		// RequireShipInPort's own owner-has-no-settlements exception is what
		// keeps this ship orderable going forward (R3), the sole exception to
		// "ships at sea take no orders" — otherwise it would be dead forever.
		slog.Info("sweep ships at sea: owner has no settlement, leaving ship in place (stranded exception)", "unit", unitID)
		return false, tx.Commit(ctx)
	}
	u.homeSettlementID = &home

	if err := h.dispatchReturnHome(ctx, tx, u, u.q, u.r, worldID, returnReasonSweptFromSea); err != nil {
		return false, fmt.Errorf("dispatch return home: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
