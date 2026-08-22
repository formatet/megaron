package combat

// KR2 — fientliga härar i samma hex möts (megaron_plan_kr2_motet.md,
// Timothy 2026-08-22).
//
// Before this file, two marching armies whose paths crossed passed straight
// through each other. Combat only ever triggered at ARRIVAL
// (unit_arrival.go, once status has already flipped to 'positioned') or
// against a STATIONARY sentry (unit_intercept_scan.go, avsiktslagret §S3) —
// neither entry point noticed two columns that are BOTH still marching when
// their paths cross mid-route. The old model also assigned "attacker"
// arbitrarily to whichever unit happened to move last, which made a
// deliberate interception march pointless: you could never reach an
// oncoming army in time, only its empty former position.
//
// Scope (plan §2) — read this before touching the grouping logic: "samma
// hex" is a GROUP BY on the (q,r) each marching unit's INTERPOLATED position
// resolves to THIS tick. It is NOT a pairwise distance check and NOT a
// radius — that would be the O(n²) sweep the plan explicitly rejected in
// favour of this linear one. If a meeting radius >0 is ever wanted, that is
// its own decision with its own follow-up questions (who "gets there
// first"?), not a generalisation to smuggle in here.
//
// Substrate reused, none of it rebuilt here:
//   - position: province.LoadTileGraph + TileGraph.FindPath, preloaded ONCE
//     per sweep, then walked per unit — the exact pattern
//     unit_intercept_scan.go and api/handlers/foreign_units.go:159 already
//     use, so the per-tick cost is n×A*, never n².
//   - combat: UnitArrivalHandler.initiateOrJoinBattle (KR3) — this file never
//     rolls dice itself.
//   - reaction: units.reaction_policy (avsiktslagret) — read straight off
//     the row, no new column.
//
// Detection vs. reaction (plan §4 point 5, "KR2 = B skärpt"): a shared hex is
// always noticed, but only units whose reaction_policy.foreign is
// "intercept" actually fight — "escort"/"ignore"/"alert" pass through
// untouched, same axis unit_intercept_scan.go already reads off a sentry.
// This slice adds no new notification: unlike the sentry scan's "alert" verb
// (which fires a standalone SentryAlerted), a march-vs-march sighting that
// produces no fight has nothing new to say — MarchSightingHandler already
// tells a Wanax the moment a foreign march becomes visible, independently of
// whether it ever collides with anything. So filtering to intercept-only
// units AT THE QUERY is behaviourally identical to loading everyone and
// discarding the rest afterward, and considerably cheaper — it is not a
// skipped detection step, just an early exit with no observable difference.
//
// Two known traps (plan §4):
//  1. A unit due to ARRIVE this exact tick (arrive_tick <= current tick) must
//     be left for the arrival handler (ScheduledUnitArrival), never resolved
//     here — "ankomst vinner". The marching-units query below excludes any
//     unit whose arrive_tick is due or overdue, so the two handlers can never
//     both resolve the same unit's fate on the same tick regardless of which
//     one the worker happens to dispatch first.
//  2. Interpolation is continuous, the hex is discrete: two units that cross
//     paths BETWEEN two hexes (never sharing an integer (q,r) at the same
//     tick) are not detected. Accepted, not a bug — Timothy's rule is
//     literally "same hex".

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// marchEncounterScanIntervalTicks: every tick, like the sentry scan and the
// sighting sweep — a collision must be caught the tick it happens, not
// batched up.
const marchEncounterScanIntervalTicks = 1

// MarchEncounterHandler processes the recurring ScheduledMarchEncounterScan sweep.
type MarchEncounterHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	clk        clock.Clock
	hub        Broadcaster // may be nil in tests; unused directly (see resolveHex's uah build)
}

// NewMarchEncounterHandler creates a MarchEncounterHandler.
func NewMarchEncounterHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, clk clock.Clock, hub Broadcaster) *MarchEncounterHandler {
	return &MarchEncounterHandler{pool: pool, scheduler: sched, eventStore: store, clk: clk, hub: hub}
}

// crossingUnit is one marching, intercept-postured unit resolved to the hex
// it occupies right now.
type crossingUnit struct {
	id    uuid.UUID
	owner uuid.UUID
	utype string
	size  int
	pos   province.MapPosition
}

// isForeign reports whether b is hostile to a. Kingdoms are post-MVP and
// gated (CLAUDE.md) — today every other owner is foreign, no ally carve-out.
// A single point so the future kingdoms slice only has to correct this one
// function, not every owner_id<>owner_id comparison scattered through the
// scan — the same shortcut /foreign-units already takes
// (megaron_todo.md, "Allierade behandlas som främmande"). Not fixed here,
// deliberately not repeated as a second copy either.
func isForeign(a, b uuid.UUID) bool {
	return a != b
}

// hasForeignPair reports whether group contains at least two units under
// mutually foreign owners — the condition that turns a shared hex into a
// contested one (plan §2/§4's whole detection model). Delegates the relation
// test to isForeign so a future kingdoms fix has one function to correct,
// not a scattered set of owner_id<>owner_id comparisons.
func hasForeignPair(group []crossingUnit) bool {
	for i := range group {
		for j := i + 1; j < len(group); j++ {
			if isForeign(group[i].owner, group[j].owner) {
				return true
			}
		}
	}
	return false
}

// Handle scans every marching unit once, groups them by their interpolated
// current hex, and initiates/joins a battle on every hex held by ≥2 foreign
// intercept-postured owners, then re-enqueues itself.
func (h *MarchEncounterHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	now := h.clk.Now()

	var currentTick int
	if err := h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick); err != nil {
		return fmt.Errorf("march encounter: read current tick: %w", err)
	}

	units, err := h.loadCandidates(ctx, e.WorldID, now, currentTick)
	if err != nil {
		return err
	}

	if len(units) > 0 {
		groups := make(map[[2]int][]crossingUnit)
		for _, u := range units {
			key := [2]int{u.pos.Q, u.pos.R}
			groups[key] = append(groups[key], u)
		}

		for hex, group := range groups {
			if !hasForeignPair(group) {
				continue // one owner (or a lone unit) — nothing foreign here
			}
			if err := h.resolveHex(ctx, e, hex, group); err != nil {
				slog.Error("march encounter: resolve hex failed", "q", hex[0], "r", hex[1], "err", err)
			}
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledMarchEncounterScan,
		struct{}{}, e.DueTick, marchEncounterScanIntervalTicks)
}

// loadCandidates returns every marching, intercept-postured unit in the
// world, resolved to its live interpolated position — exactly the same
// position model /foreign-units and march_sighting.go use, computed off ONE
// preloaded tile graph (never a second copy of the pathfinding rules).
//
// Filters applied in SQL:
//   - status='marching' with a complete march (q/r/target/departs/arrives all
//     set) — the same completeness guard unit_intercept_scan.go and
//     march_sighting.go both use.
//   - reaction_policy.foreign = 'intercept' — trap 2's "detection is free,
//     reaction is policy-gated" collapses to this filter (see file header).
//   - arrive_tick IS NULL OR arrive_tick > currentTick — trap 1, "ankomst
//     vinner": a unit due this tick or overdue is left for
//     ScheduledUnitArrival.
func (h *MarchEncounterHandler) loadCandidates(ctx context.Context, worldID uuid.UUID, now time.Time, currentTick int) ([]crossingUnit, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, type, size, q, r, target_q, target_r, departs_at, arrives_at, category
		 FROM units
		 WHERE world_id = $1 AND status = 'marching'
		   AND q IS NOT NULL AND r IS NOT NULL
		   AND target_q IS NOT NULL AND target_r IS NOT NULL
		   AND departs_at IS NOT NULL AND arrives_at IS NOT NULL
		   AND (reaction_policy->>'foreign') = $2
		   AND (arrive_tick IS NULL OR arrive_tick > $3)`,
		worldID, string(unit.ReactionIntercept), currentTick,
	)
	if err != nil {
		return nil, fmt.Errorf("march encounter: load marching units: %w", err)
	}

	type rawUnit struct {
		crossingUnit
		originQ, originR, targetQ, targetR int
		departsAt, arrivesAt               time.Time
		category                           string
	}
	var raw []rawUnit
	for rows.Next() {
		var m rawUnit
		if scanErr := rows.Scan(&m.id, &m.owner, &m.utype, &m.size,
			&m.originQ, &m.originR, &m.targetQ, &m.targetR,
			&m.departsAt, &m.arrivesAt, &m.category); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("march encounter: scan unit: %w", scanErr)
		}
		raw = append(raw, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("march encounter: load marching units: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}

	g, err := province.LoadTileGraph(ctx, h.pool, worldID)
	if err != nil {
		return nil, fmt.Errorf("march encounter: load tile graph: %w", err)
	}

	out := make([]crossingUnit, 0, len(raw))
	for _, m := range raw {
		pos := province.MapPosition{Q: m.originQ, R: m.originR}
		path, _, ok := g.FindPath(pos, province.MapPosition{Q: m.targetQ, R: m.targetR}, m.category)
		if !ok || len(path) == 0 {
			// No traversable route this sweep (split by sea/river). Same
			// accepted gap as unit_intercept_scan.go: no straight-line
			// fallback, the unit is simply not detectable this tick rather
			// than guessing a position.
			continue
		}
		pos = province.InterpolateAlongPath(now, m.departsAt, m.arrivesAt, path)

		u := m.crossingUnit
		u.pos = pos
		out = append(out, u)
	}
	return out, nil
}

// resolveHex claims (event, hex) via processed_tick_claims — a retry of the
// SAME scheduled event must never start a second battle at a hex it already
// resolved (G2) — then, if the claim is fresh, initiates/joins one battle
// covering every unit in group, split attacker/defender by owner.
//
// group is guaranteed (by Handle's caller) to span ≥2 distinct owners, all
// already filtered to reaction_policy.foreign='intercept' by loadCandidates.
func (h *MarchEncounterHandler) resolveHex(ctx context.Context, e events.ScheduledEvent, hex [2]int, group []crossingUnit) error {
	q, r := hex[0], hex[1]

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// Idempotency (G2, plan §4): one claim per (event_id, hex) — same
	// processed_tick_claims table UpkeepHandler and loyalty/decay.go use,
	// migration 098. The scope UUID is deterministic from (worldID, q, r) so
	// the SAME hex claims the SAME slot every time this exact event is
	// replayed; a genuinely NEW recurring tick always gets a fresh event_id
	// (EnqueueTickRecurring inserts a new scheduled_events row), so this only
	// suppresses a true replay of this event, never a later tick's collision.
	scope := uuid.NewSHA1(e.WorldID, []byte(fmt.Sprintf("march_encounter:%d:%d", q, r)))
	claim, err := tx.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, e.ID, scope)
	if err != nil {
		return fmt.Errorf("claim march encounter tick: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // this exact event already resolved this hex
	}

	// Re-verify every unit is still 'marching' under lock — another handler
	// (recall, a courier-delivered order) may have changed it between the
	// unlocked scan above and now. Units that changed are dropped from this
	// battle rather than aborting the whole hex.
	live := make([]crossingUnit, 0, len(group))
	for _, u := range group {
		var status string
		if qErr := tx.QueryRow(ctx, `SELECT status FROM units WHERE id = $1 FOR UPDATE`, u.id).Scan(&status); qErr != nil {
			continue
		}
		if status != "marching" {
			continue
		}
		live = append(live, u)
	}

	if !hasForeignPair(live) {
		return tx.Commit(ctx) // the claim stands; nothing left to fight
	}
	owners := map[uuid.UUID][]crossingUnit{}
	for _, u := range live {
		owners[u.owner] = append(owners[u.owner], u)
	}

	// Deterministic attacker/defender split: sort owner IDs, first = attacker
	// side, every other owner = defender side. With exactly two owners (the
	// overwhelmingly common case — kingdoms disabled, so a third mutually
	// hostile army on the exact same hex the exact same tick is a rare
	// edge) this is a plain two-sided battle; with three or more it lumps
	// the rest together, a known simplification (this system has no 3-way
	// free-for-all combat model to begin with).
	ownerIDs := make([]string, 0, len(owners))
	byStr := map[string]uuid.UUID{}
	for o := range owners {
		s := o.String()
		ownerIDs = append(ownerIDs, s)
		byStr[s] = o
	}
	sort.Strings(ownerIDs)
	attackerOwner := byStr[ownerIDs[0]]

	var participants []battleParticipant
	for _, o := range ownerIDs {
		owner := byStr[o]
		side := "defender"
		if owner == attackerOwner {
			side = "attacker"
		}
		for _, u := range owners[owner] {
			participants = append(participants, battleParticipant{
				unitID: u.id, ownerID: u.owner, utype: u.utype, side: side, currentSize: u.size,
			})
		}
	}

	// initiateOrJoinBattle is a UnitArrivalHandler method; built inline
	// exactly as unit_intercept_scan.go's intercept() does — sitosCfg/Dice
	// are the only fields that differ from this zero-value build and neither
	// is read by initiateOrJoinBattle/startBattle/joinBattle.
	uah := &UnitArrivalHandler{pool: h.pool, eventStore: h.eventStore, scheduler: h.scheduler, hub: h.hub, clk: h.clk}

	// Call once per participant: the FIRST call (no battle yet at this hex)
	// seeds the battle with every OTHER participant already tagged in
	// `defenders`, in one startBattle insert; every subsequent call finds the
	// battle it just created and joins idempotently
	// (ON CONFLICT (battle_id, unit_id) DO NOTHING in joinBattle) — so this
	// is correct whether the battle is brand new this tick or already
	// existed from a prior tick (e.g. this same scan, an earlier hour).
	for i, p := range participants {
		others := make([]battleParticipant, 0, len(participants)-1)
		for j, o := range participants {
			if j != i {
				others = append(others, o)
			}
		}
		if err := uah.initiateOrJoinBattle(ctx, tx, e.WorldID, q, r, p, others); err != nil {
			return fmt.Errorf("march encounter: initiate/join battle for %s: %w", p.unitID, err)
		}
	}

	// Halt every participant at the shared hex — a marching unit that just
	// found a battle does not keep walking, same rule §7's sentry
	// interception already established (unit_intercept_scan.go).
	for _, p := range participants {
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
			p.unitID, q, r,
		); err != nil {
			return fmt.Errorf("march encounter: halt unit %s: %w", p.unitID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("march encounter: crossing armies collided — battle initiated/joined",
		"q", q, "r", r, "units", len(participants))
	return nil
}
