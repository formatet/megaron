package combat

// UnitInterceptScanHandler is the unit-vs-unit counterpart of
// transport.InterceptScanHandler (megaron_plan_avsiktslagret.md §S3). Nothing
// currently makes two marching columns fight before one of them arrives at a
// settlement — this is the substrate KR2 ("marscherande härar möts") builds on:
// a periodic sweep that finds every marching unit's live interpolated position,
// and for each FOW-visible enemy sentry within reach whose reaction policy says
// "intercept", resolves combat between them.
//
// G1 placement: this lives in `combat`, not `transport` or `unit` — it calls
// resolveCombat's sibling math (unitStrength, rollFortune,
// ResolveStrengthsWithRout, routFractionForLoyalty, supplyingSettlement, all
// already package-private in this package) and combat is the one package
// downstream of both transport and unit allowed to depend on both plus
// province (CLAUDE.md G1). transport/unit must never import combat, so this
// scan cannot live there.
//
// Scope note (deliberately minimal, avsiktslagret §S3): a marching unit that
// SURVIVES an interception keeps marching on its existing course — this scan
// only applies losses to both sides, it never redirects, halts or turns a
// march around. Full march-interruption semantics (does the column retreat?
// does it have to re-path?) are KR2's job, not this substrate's.
//
// §S4 (2026-08-07, megaron_plan_avsiktslagret.md): both owners now get a
// notification, reusing the stridsrapport payload shape (BattleTickHandler.
// notifyBattleEnded's kind/fields) even though this handler resolves combat
// synchronously in one roll, not via initiateOrJoinBattle/ScheduledBattleTick
// — see notifyInterceptionResolved below for why cutting this entry point
// over to the KR3 substrate is deliberately NOT done here (scope note in that
// function's doc comment).

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/loyalty"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
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

// Handle scans every marching unit once, resolving combat against the
// strongest FOW-visible enemy sentry within reach that hasn't already fought
// this exact march instance, then re-enqueues itself.
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

		// An enemy sentry watching within reach whose policy says intercept.
		var sentryID, interceptorID uuid.UUID
		var sentryType string
		if qErr := h.pool.QueryRow(ctx,
			`SELECT id, owner_id, type FROM units
			 WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned' AND stance = 'sentry'
			   AND sentry_q IS NOT NULL AND sentry_r IS NOT NULL
			   AND (reaction_policy->>'foreign') = 'intercept'
			   AND (ABS(sentry_q - $3) + ABS(sentry_r - $4) + ABS((sentry_q + sentry_r) - ($3 + $4))) / 2 <= $5
			 ORDER BY size DESC
			 LIMIT 1`,
			e.WorldID, m.owner, pos.Q, pos.R, UnitInterceptRadius,
		).Scan(&sentryID, &interceptorID, &sentryType); qErr != nil {
			continue // no intercept-policy sentry in reach
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

		if err := h.intercept(ctx, e.WorldID, m, sentryID, sentryType, interceptorID, pos); err != nil {
			slog.Error("unit intercept scan: intercept failed", "unit", m.id, "sentry", sentryID, "err", err)
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledUnitInterceptScan,
		struct{}{}, e.DueTick, UnitInterceptScanIntervalTicks)
}

// intercept resolves one (marching unit, sentry) pair's combat, guarded so the
// same march instance can only be fought once by the same sentry.
func (h *UnitInterceptScanHandler) intercept(
	ctx context.Context, worldID uuid.UUID, m marchingUnit, sentryID uuid.UUID, sentryType string, interceptorID uuid.UUID, pos province.MapPosition,
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
	// for THIS march instance (depart_tick) may only fight once. A surviving
	// march keeps its size reduced but otherwise keeps marching unchanged, so
	// without this guard the same static sentry would re-fight it on every
	// subsequent scan tick it remains in range.
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
		return nil // already fought this sentry on this march instance
	}

	// Load both units FOR UPDATE — fresh state at combat time; either may have
	// changed since the outer scan read them (idempotency: a handler re-run
	// after a crash finds the guard row above already inserted and stops there).
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

	// ── Strength + fortune (mirrors resolveFieldCombat's math). ──
	attStr := unitStrength(m.utype, attSize)
	defStr := unitStrength(sentryType, defSize)

	var attackerKharis, defenderKharis float64
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		m.owner, worldID,
	).Scan(&attackerKharis)
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick))
		 FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		interceptorID, worldID,
	).Scan(&defenderKharis)
	fortune := rollFortune(attackerKharis, defenderKharis)
	attStrWithFortune := attStr * (1 + fortune)

	attSettleID, attLoyalty, attHasSettle := supplyingSettlement(ctx, tx, m.owner, nil, worldID)
	defSettleID, defLoyalty, defHasSettle := supplyingSettlement(ctx, tx, interceptorID, nil, worldID)

	result := ResolveStrengthsWithRout(attStrWithFortune, defStr, fortune,
		routFractionForLoyalty(attLoyalty), routFractionForLoyalty(defLoyalty))

	slog.Info("unit intercepted", "unit", m.id, "sentry", sentryID, "q", pos.Q, "r", pos.R,
		"att", attStr, "fortune", fortune, "def", defStr, "outcome", result.Outcome, "rounds", result.Rounds)

	attSizeBefore := attSize
	attSizeAfter := int(float64(attSize) * (1 - result.AttackerLosses))
	attPopLost := attSizeBefore - attSizeAfter
	defSizeBefore := defSize
	defSizeAfter := int(float64(defSize) * (1 - result.DefenderLosses))
	defPopLost := defSizeBefore - defSizeAfter

	// Apply losses to the marching (attacker) unit. A survivor keeps marching
	// unchanged (see file doc comment — full march-interruption is KR2's job).
	if attSizeAfter <= 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`, m.id,
		); err != nil {
			return fmt.Errorf("disband intercepted unit: %w", err)
		}
	} else if _, err := tx.Exec(ctx,
		`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, m.id, attSizeAfter,
	); err != nil {
		return fmt.Errorf("apply intercepted unit losses: %w", err)
	}
	if attPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			m.owner, attPopLost, worldID,
		); err != nil {
			slog.Warn("unit intercept scan: could not apply attacker pop loss", "unit", m.id, "err", err)
		}
	}

	// Apply losses to the sentry (defender).
	if defSizeAfter <= 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', size = 0, stance = NULL, sentry_q = NULL, sentry_r = NULL, updated_at = now()
			 WHERE id = $1`, sentryID,
		); err != nil {
			return fmt.Errorf("disband defeated sentry: %w", err)
		}
	} else if _, err := tx.Exec(ctx,
		`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, sentryID, defSizeAfter,
	); err != nil {
		return fmt.Errorf("apply sentry losses: %w", err)
	}
	if defPopLost > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET population = GREATEST(50, population - $2)
			 WHERE owner_id = $1 AND world_id = $3 AND is_capital = true`,
			interceptorID, defPopLost, worldID,
		); err != nil {
			slog.Warn("unit intercept scan: could not apply sentry pop loss", "sentry", sentryID, "err", err)
		}
	}

	// ── L2 battle loyalty (mirrors UnitArrivalHandler.applyBattleLoyalty). ──
	attackerWon := result.Outcome == OutcomeAttackerWins
	if attHasSettle {
		delta, evType, reason := -1, "battle_lost", "lost_battle"
		if attackerWon {
			delta, evType, reason = +1, "shared_victory", "won_battle"
		}
		if lErr := loyalty.AppendLoyaltyEventTx(ctx, tx, h.eventStore, attSettleID, worldID, evType, delta, reason); lErr != nil {
			slog.Warn("unit intercept scan: attacker battle loyalty failed", "settlement", attSettleID, "err", lErr)
		}
	}
	if !attackerWon && defHasSettle {
		if lErr := loyalty.AppendLoyaltyEventTx(ctx, tx, h.eventStore, defSettleID, worldID, "shared_victory", +1, "defended_settlement"); lErr != nil {
			slog.Warn("unit intercept scan: defender battle loyalty failed", "settlement", defSettleID, "err", lErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Outcome, not intent (CLAUDE.md events rule) — the roll already happened above.
	_, _ = h.eventStore.Append(ctx, m.id, events.StreamType(unit.StreamUnit), unit.EventUnitIntercepted,
		unit.UnitInterceptedPayload{
			SentryUnitID:      sentryID,
			InterceptedUnitID: m.id,
			Q:                 pos.Q,
			R:                 pos.R,
			Outcome:           string(result.Outcome),
		}, worldID, nil)
	_, _ = h.eventStore.Append(ctx, m.id, events.StreamType(unit.StreamUnit), unit.EventUnitCombatResolved,
		unit.UnitCombatResolvedPayload{
			UnitID:     m.id,
			Role:       "attacker",
			SizeBefore: attSizeBefore,
			SizeAfter:  attSizeAfter,
			Outcome:    string(result.Outcome),
			PopLost:    attPopLost,
		}, worldID, nil)

	h.notifyInterceptionResolved(ctx, worldID, pos.Q, pos.R,
		m.owner, m.utype, attSizeBefore, attSizeAfter,
		interceptorID, sentryType, defSizeBefore, defSizeAfter,
		result.Outcome)

	return nil
}

// notifyInterceptionResolved is avsiktslagret's §S4 (megaron_plan_avsiktslagret.md):
// "Notis till båda parter (återanvänd stridsrapportens payload)". Reuses
// BattleTickHandler.notifyBattleEnded's exact payload shape/kind
// (BattleWon/BattleLost, role/outcome/opponent_name/own_unit/enemy_unit/q/r/
// place) so keryx's printBattleReportLine and the web's notifText case render
// this identically to a KR3 battle report — a Wanax should not be able to
// tell "this notification came from a different code path" from the text.
//
// Deliberately NOT cut over to initiateOrJoinBattle (plan §8's "the other
// three entry points"): this handler resolves in one immediate roll, and a
// SURVIVING marching unit keeps marching unchanged on its existing course
// (this file's own header comment, "full march-interruption is KR2's job").
// Routing this through KR3 would mean the intercepted unit instead halts in
// place (status='positioned') for however many battle-ticks the fight takes,
// same as a field arrival — a real behaviour change to the march mechanic,
// not a pure refactor, and not specified by the locked plan. Left as a
// flagged gap for whoever picks up §8/KR2's march-interruption question,
// rather than decided here.
func (h *UnitInterceptScanHandler) notifyInterceptionResolved(
	ctx context.Context, worldID uuid.UUID, q, r int,
	attOwner uuid.UUID, attType string, attSizeBefore, attSizeAfter int,
	defOwner uuid.UUID, defType string, defSizeBefore, defSizeAfter int,
	outcome Outcome,
) {
	if h.hub == nil {
		return
	}
	attName := ownerNameOf(ctx, h.pool, attOwner)
	defName := ownerNameOf(ctx, h.pool, defOwner)
	var place *string
	if name, ok := settlementNameAt(ctx, h.pool, worldID, q, r); ok {
		place = &name
	}

	// outcomeStr maps resolver.go's Outcome ("attacker_wins"/"defender_wins")
	// onto the KR3 notification's own vocabulary ("attacker_wins"/
	// "defender_holds"/"mutual_wipe" — BattleTickHandler.notifyBattleEnded),
	// which keryx's printBattleReportLine and the web's notifText case switch
	// on for the trailer sentence. string(outcome) directly would silently
	// mismatch ("defender_wins" ≠ "defender_holds") and the trailer would
	// just fall through empty — not a crash, but a quietly worse message.
	outcomeStr := "attacker_wins"
	if outcome == OutcomeDefenderWins {
		outcomeStr = "defender_holds"
	}

	notify := func(recipient uuid.UUID, role, opponentName string, own, enemy battleReportUnit) {
		kind, level := "BattleLost", 2
		won := (role == "attacker" && outcome == OutcomeAttackerWins) || (role == "defender" && outcome == OutcomeDefenderWins)
		if won {
			kind, level = "BattleWon", 3
		}
		payload := map[string]any{
			"role": role, "outcome": outcomeStr, "opponent_name": opponentName,
			"own_unit": own, "enemy_unit": enemy, "q": q, "r": r,
		}
		if place != nil {
			payload["place"] = *place
		}
		if err := h.hub.NotifyPlayer(ctx, worldID, recipient, kind, level, payload); err != nil {
			slog.Warn("notify interception resolved", "recipient", recipient, "err", err)
		}
	}

	attUnit := battleReportUnit{Type: attType, SizeBefore: attSizeBefore, SizeAfter: attSizeAfter, PopLost: attSizeBefore - attSizeAfter}
	defUnit := battleReportUnit{Type: defType, SizeBefore: defSizeBefore, SizeAfter: defSizeAfter, PopLost: defSizeBefore - defSizeAfter}
	notify(attOwner, "attacker", defName, attUnit, defUnit)
	notify(defOwner, "defender", attName, defUnit, attUnit)
}
