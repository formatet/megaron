package combat

// UnitInterceptScanHandler is the unit-vs-unit counterpart of
// transport.InterceptScanHandler (megaron_plan_avsiktslagret.md §S3). A
// periodic sweep that finds every marching unit's live interpolated position,
// and for each FOW-visible enemy sentry within reach whose reaction policy
// says "intercept" or "alert", acts on it (KR3 §7, megaron_plan_kr3_stridssystem.md).
//
// G1 placement: this lives in `combat`, not `transport` or `unit` — it calls
// resolveCombat's sibling math (unitStrength, rollFortune,
// ResolveStrengthsWithRout, routFractionForLoyalty, supplyingSettlement, all
// already package-private in this package) and combat is the one package
// downstream of both transport and unit allowed to depend on both plus
// province (CLAUDE.md G1). transport/unit must never import combat, so this
// scan cannot live there.
//
// §7 cutover (2026-08-07, Timothy's march-interruption decision): an
// "intercept"-policy sentry now calls initiateOrJoinBattle instead of
// resolving one immediate roll — the marching unit HALTS at its interpolated
// position (status flips to 'positioned', same as a field arrival) and fights
// the persistent multi-tick battle there. A surviving unit does NOT resume
// its march on its own; the player must re-issue a march order once the
// battle ends. This retires the old one-shot resolve for this entry point —
// it was the last of the four named in plan §8, so unitStrength/rollFortune/
// ResolveStrengthsWithRout's direct callers for the interception path are
// gone (those functions remain — resolver.go, still used inside battle.go's
// own math and by any not-yet-audited one-shot code elsewhere).
//
// avsiktslagret §7 verbs, read off the SENTRY's reaction_policy.foreign:
//   - "intercept": fight (initiateOrJoinBattle, as above).
//   - "alert": no combat — the sentry's owner is notified a foreign unit
//     passed within reach, nothing else happens. "larma bara."
//   - "escort"/"ignore": excluded from the query entirely below — a sentry
//     posted to escort or ignore never triggers this scan at all.
//
// The scan itself is still the substrate KR2 ("marscherande härar möts")
// builds on — this file only makes a MARCHING unit vs a STATIONARY sentry
// fight; two marching columns meeting each other is still unbuilt (own
// contract round, per megaron_todo.md).
//
// §S4 (2026-08-07, megaron_plan_avsiktslagret.md): the "intercept" branch's
// notification is now BattleTickHandler.notifyBattleEnded's own (fired when
// the battle concludes, possibly several ticks later) — no separate
// notification is sent at the moment of interception itself, matching every
// other initiateOrJoinBattle entry point.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Unit-vs-unit interception tuning. Deliberately combat's OWN copy, not shared
// with transport.interceptRadius/interceptScanIntervalTicks — transport and
// combat sit at different G1 layers (combat depends on transport, never the
// reverse) so a shared constant isn't reachable from here; the plan asks for
// the same DEFAULT value, not a single source of truth across the two.
const (
	UnitInterceptRadius            = 2
	UnitInterceptScanIntervalTicks = 1
)

// UnitInterceptScanHandler processes the recurring ScheduledUnitInterceptScan sweep.
type UnitInterceptScanHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	clk        clock.Clock
	hub        Broadcaster
}

// NewUnitInterceptScanHandler creates a UnitInterceptScanHandler. hub may be
// nil in tests (every NotifyPlayer call below is nil-guarded, matching the
// other combat handlers — see collapse.go).
func NewUnitInterceptScanHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, clk clock.Clock, hub Broadcaster) *UnitInterceptScanHandler {
	return &UnitInterceptScanHandler{pool: pool, scheduler: sched, eventStore: store, clk: clk, hub: hub}
}

type marchingUnit struct {
	id         uuid.UUID
	owner      uuid.UUID
	utype      string
	category   string
	size       int
	q, r       int
	targetQ    int
	targetR    int
	departsAt  time.Time
	arrivesAt  time.Time
	departTick *int
}

// Handle scans every marching unit once, acting against the strongest
// FOW-visible enemy sentry within reach (intercept or alert policy) that
// hasn't already been resolved against this exact march instance, then
// re-enqueues itself.
func (h *UnitInterceptScanHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	now := h.clk.Now()

	// Terrain + per-owner live eyes, loaded/memoised once for the whole sweep —
	// mirrors transport/intercept.go's FOW gate exactly (same gate /foreign-units
	// uses: LoadLiveEyes + AnyEyeSees against the target's live interpolated
	// position, avsiktslagret §4).
	graph, err := province.LoadTileGraph(ctx, h.pool, e.WorldID)
	if err != nil {
		return fmt.Errorf("unit intercept scan: load terrain: %w", err)
	}
	eyesByOwner := map[uuid.UUID][]province.Eye{}

	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, type, category, size, q, r, target_q, target_r, departs_at, arrives_at, depart_tick
		 FROM units
		 WHERE world_id = $1 AND status = 'marching'
		   AND q IS NOT NULL AND r IS NOT NULL
		   AND target_q IS NOT NULL AND target_r IS NOT NULL
		   AND departs_at IS NOT NULL AND arrives_at IS NOT NULL`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("unit intercept scan: load marching units: %w", err)
	}
	var fleet []marchingUnit
	for rows.Next() {
		var m marchingUnit
		if scanErr := rows.Scan(&m.id, &m.owner, &m.utype, &m.category, &m.size,
			&m.q, &m.r, &m.targetQ, &m.targetR, &m.departsAt, &m.arrivesAt, &m.departTick); scanErr != nil {
			rows.Close()
			return fmt.Errorf("unit intercept scan: scan unit: %w", scanErr)
		}
		fleet = append(fleet, m)
	}
	rows.Close()

	for _, m := range fleet {
		origin := province.MapPosition{Q: m.q, R: m.r}
		dest := province.MapPosition{Q: m.targetQ, R: m.targetR}
		pos, ok, posErr := province.InterpolatePosition(ctx, h.pool, e.WorldID, origin, dest, m.category, m.departsAt, m.arrivesAt, now)
		if posErr != nil {
			continue // DB error is not the same as "no route" — never guess a position off the back of one
		}
		if !ok {
			// No traversable route for this category (origin/target split by sea
			// or a river). Unlike transport/intercept.go this does NOT fall back
			// to a straight-line position — known, accepted gap for this slice
			// (avsiktslagret §S3 is the substrate scan, not full parity with the
			// caravan path); a unit on an unpathable course is simply not
			// interceptable this sweep instead of guessing at a position.
			continue
		}

		// The strongest enemy sentry watching within reach whose policy is
		// "intercept" or "alert" — "escort"/"ignore" never match, so a sentry
		// posted that way never triggers this scan at all (§7).
		var sentryID, interceptorID uuid.UUID
		var sentryType, verb string
		if qErr := h.pool.QueryRow(ctx,
			`SELECT id, owner_id, type, reaction_policy->>'foreign' FROM units
			 WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned' AND stance = 'sentry'
			   AND sentry_q IS NOT NULL AND sentry_r IS NOT NULL
			   AND (reaction_policy->>'foreign') IN ('intercept', 'alert')
			   AND (ABS(sentry_q - $3) + ABS(sentry_r - $4) + ABS((sentry_q + sentry_r) - ($3 + $4))) / 2 <= $5
			 ORDER BY size DESC
			 LIMIT 1`,
			e.WorldID, m.owner, pos.Q, pos.R, UnitInterceptRadius,
		).Scan(&sentryID, &interceptorID, &sentryType, &verb); qErr != nil {
			continue // no intercept/alert-policy sentry in reach
		}

		// FOW gate (avsiktslagret §4): the sentry's owner must actually SEE the
		// marching unit's live position right now. Fail closed: no terrain row
		// for the hex → treat as unseen, never guess.
		terrain, known := graph[[2]int{pos.Q, pos.R}]
		if !known {
			continue
		}
		eyes, cached := eyesByOwner[interceptorID]
		if !cached {
			eyes = province.LoadLiveEyes(ctx, h.pool, e.WorldID, interceptorID, now)
			eyesByOwner[interceptorID] = eyes
		}
		if !province.AnyEyeSees(eyes, pos, terrain) {
			continue // the sentry's owner has never laid eyes on this march
		}

		if err := h.intercept(ctx, e.WorldID, m, sentryID, sentryType, interceptorID, verb, pos); err != nil {
			slog.Error("unit intercept scan: intercept failed", "unit", m.id, "sentry", sentryID, "err", err)
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledUnitInterceptScan,
		struct{}{}, e.DueTick, UnitInterceptScanIntervalTicks)
}

// intercept acts on one (marching unit, sentry) pair, guarded so the same
// march instance can only be resolved once by the same sentry — regardless
// of whether that resolution was a fight (verb "intercept") or a sighting
// (verb "alert").
func (h *UnitInterceptScanHandler) intercept(
	ctx context.Context, worldID uuid.UUID, m marchingUnit, sentryID uuid.UUID, sentryType string, interceptorID uuid.UUID, verb string, pos province.MapPosition,
) error {
	if m.departTick == nil {
		// Should not happen — march dispatch always sets depart_tick alongside
		// departs_at (unit/model.go). Fail closed rather than guess a guard key.
		slog.Warn("unit intercept scan: marching unit has no depart_tick, skipping", "unit", m.id)
		return nil
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Idempotency / double-avskärning guard (G2 + avsiktslagret §S3 design,
	// unit_arrival.go's original TODO): this exact (marching unit, sentry) pair
	// for THIS march instance (depart_tick) may only be resolved once. Without
	// this guard the same static sentry would re-trigger on every subsequent
	// scan tick the marching unit remains in range.
	tag, err := tx.Exec(ctx,
		`INSERT INTO unit_interceptions (unit_id, sentry_unit_id, depart_tick, world_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		m.id, sentryID, *m.departTick, worldID,
	)
	if err != nil {
		return fmt.Errorf("intercept guard insert: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil // already resolved against this sentry on this march instance
	}

	// Load both units FOR UPDATE — fresh state at resolution time; either may
	// have changed since the outer scan read them (idempotency: a handler
	// re-run after a crash finds the guard row above already inserted and
	// stops there).
	var attSize int
	var attStatus string
	if err := tx.QueryRow(ctx,
		`SELECT size, status FROM units WHERE id = $1 FOR UPDATE`, m.id,
	).Scan(&attSize, &attStatus); err != nil {
		return fmt.Errorf("load marching unit: %w", err)
	}
	if attStatus != "marching" {
		return nil // already arrived/recalled/destroyed since the scan read it
	}

	var defSize int
	var defStatus string
	var defStance *string
	if err := tx.QueryRow(ctx,
		`SELECT size, status, stance FROM units WHERE id = $1 FOR UPDATE`, sentryID,
	).Scan(&defSize, &defStatus, &defStance); err != nil {
		return fmt.Errorf("load sentry unit: %w", err)
	}
	if defStatus != "positioned" || defStance == nil || *defStance != "sentry" {
		return nil // sentry moved/disbanded/changed stance since the scan read it
	}

	if verb == "alert" {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit alert guard: %w", err)
		}
		slog.Info("unit intercept scan: sentry alerted (no combat)", "unit", m.id, "sentry", sentryID, "q", pos.Q, "r", pos.R)
		if h.hub != nil {
			marchOwnerName := ownerNameOf(ctx, h.pool, m.owner)
			if err := h.hub.NotifyPlayer(ctx, worldID, interceptorID, "SentryAlerted", 1, map[string]any{
				"sentry_unit_id": sentryID,
				"foreign_type":   m.utype,
				"foreign_owner":  marchOwnerName,
				"q":              pos.Q,
				"r":              pos.R,
			}); err != nil {
				slog.Warn("notify sentry alerted", "recipient", interceptorID, "err", err)
			}
		}
		return nil
	}

	// verb == "intercept": KR3 §7 cutover — halt the marching unit at its
	// interpolated position and initiate/join a persistent battle there,
	// exactly like resolveFieldCombat (unit_arrival_field.go). No immediate
	// win/lose roll here anymore; ScheduledBattleTick resolves it over
	// subsequent ticks and its own notifyBattleEnded tells both owners.
	arriving := battleParticipant{unitID: m.id, ownerID: m.owner, utype: m.utype, side: "attacker", currentSize: attSize}
	defender := battleParticipant{unitID: sentryID, ownerID: interceptorID, utype: sentryType, side: "defender", currentSize: defSize}

	// initiateOrJoinBattle is a UnitArrivalHandler method; built inline rather
	// than threading a shared instance through NewUnitInterceptScanHandler's
	// constructor (and its 5 call sites) — sitosCfg/Dice are the only fields
	// that differ from this zero-value build, and neither is read by
	// initiateOrJoinBattle/startBattle/joinBattle (sitosCfg: unit_arrival.go's
	// genesis-silver path only; Dice: nil-safe, falls back to
	// economy.NewWallDice(), the same production default).
	uah := &UnitArrivalHandler{pool: h.pool, eventStore: h.eventStore, scheduler: h.scheduler, hub: h.hub, clk: h.clk}
	if err := uah.initiateOrJoinBattle(ctx, tx, worldID, pos.Q, pos.R, arriving, []battleParticipant{defender}); err != nil {
		return fmt.Errorf("intercept: initiate/join battle: %w", err)
	}

	// The marching unit halts at the interception point — a Timothy decision
	// (2026-08-07): it does NOT keep marching. A survivor must be given a new
	// march order once the battle ends; nothing resumes it automatically.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status        = 'positioned',
		   q             = $2,
		   r             = $3,
		   target_q      = NULL,
		   target_r      = NULL,
		   departs_at    = NULL,
		   arrives_at    = NULL,
		   depart_tick   = NULL,
		   arrive_tick   = NULL,
		   updated_at    = now()
		 WHERE id = $1`,
		m.id, pos.Q, pos.R,
	); err != nil {
		return fmt.Errorf("intercept: halt marching unit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("unit intercepted — battle initiated/joined", "unit", m.id, "sentry", sentryID, "q", pos.Q, "r", pos.R)
	return nil
}
