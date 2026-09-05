package messenger

// RecallArrival: when a recall-order messenger reaches the unit, this handler
// fires and starts the actual return march.
//
// The event payload carries a Kind field for future sub-types; today only
// "march" — turn an in-flight army around and march it home — exists (an
// "outpost" sub-type existed here until migration 138 dropped the outpost
// data model, 2026-09-02: no code path ever established an outpost, so
// handleOutpost never ran).
//
// Idempotent via an atomic conditional claim (CLAUDE.md "Event handlers"):
// the return work only happens if the unit is still recallable when the messenger arrives.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TicksPerHex is the travel speed of a messenger (ticks per hex). Shared by diplomatic
// messengers (api/handlers messenger send) and recall messengers so the rate lives in one place.
// A future "blessed messengers" rite would turn this into a multiplier-driven value (see thalassa_todo).
const TicksPerHex = 0.5

// armyArrivalPayload is the ScheduledArmyArrival payload for a legacy return march.
// The legacy aggregate-army combat handler that consumed this event was retired with
// the SB7 dual-write drop (its handler was never registered on the worker); the type
// lived in combat/arrival.go. Kept locally so the legacy recall path still compiles and
// behaves exactly as before (the event has no registered consumer → it dead-letters,
// same as pre-SB7). Retiring the legacy recall path entirely is separate cleanup.
type armyArrivalPayload struct {
	MarchingArmyID uuid.UUID `json:"marching_army_id"`
}

// RecallMarchPayload is the RecallArrival payload for an in-flight army recall.
type RecallMarchPayload struct {
	Kind          string    `json:"kind"` // "march"
	WorldID       uuid.UUID `json:"world_id"`
	MessengerID   uuid.UUID `json:"messenger_id"` // the visible recall messenger, marked arrived on fire
	MarchID       uuid.UUID `json:"march_id"`
	Spearman      int       `json:"spearman"`
	WarChariot    int       `json:"war_chariot"`
	Ship          int       `json:"ship"` // galley
	EliteInfantry int       `json:"elite_infantry"`
	WarGalley     int       `json:"war_galley"`
	Merchantman   int       `json:"merchantman"`
	// The messenger is aimed at an honestly-computed INTERCEPTION point along
	// the army's outbound path (InterceptCourierTarget, resolved once at
	// dispatch — api/handlers.ProvinceHandler.RecallMarch), not the army's
	// full original destination (megaron_todo.md Aggregatarmé-recall slice,
	// 2026-07-30 — the old "aims at the destination, always physically safe"
	// assumption was exactly the silent-miss bug: an army intercepted early
	// still had its return trip costed from the far destination). The return
	// march this event's handler creates departs from THIS point.
	OriginQ  int       `json:"origin_q"`
	OriginR  int       `json:"origin_r"`
	TargetQ  int       `json:"target_q"` // interception hex, not the army's original target
	TargetR  int       `json:"target_r"`
	OriginID uuid.UUID `json:"origin_id"` // province the return march goes back to (home)
	TargetID uuid.UUID `json:"target_id"` // province at the interception hex — where the return march departs from
}

// RecallArrivalHandler processes RecallArrival scheduled events.
// It is registered with events.Worker and must be idempotent.
type RecallArrivalHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	hub       combat.Broadcaster // OrderFailed notice when a recall genuinely loses the race (handleMarch)
	clk       clock.Clock
}

// NewRecallArrivalHandler creates a RecallArrivalHandler. hub may be nil in
// tests that don't need to observe notifications.
func NewRecallArrivalHandler(pool *pgxpool.Pool, sched *events.Scheduler, hub combat.Broadcaster, clk clock.Clock) *RecallArrivalHandler {
	return &RecallArrivalHandler{pool: pool, scheduler: sched, hub: hub, clk: clk}
}

// Handle dispatches to the correct sub-handler based on the Kind field.
func (h *RecallArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var peek struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(e.Payload, &peek); err != nil {
		return fmt.Errorf("unmarshal recall arrival kind: %w", err)
	}
	switch peek.Kind {
	case "march":
		return h.handleMarch(ctx, e)
	default:
		return fmt.Errorf("unknown recall kind: %q", peek.Kind)
	}
}

// handleMarch turns a recalled in-flight army around once the recall messenger reaches it.
// Idempotent + "command isn't instant": the army keeps marching until this fires. We claim the
// outbound march with an atomic conditional UPDATE; if it was already resolved (the army arrived
// and fought first, or this event already fired), the recall is a harmless no-op.
func (h *RecallArrivalHandler) handleMarch(ctx context.Context, e events.ScheduledEvent) error {
	var p RecallMarchPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal recall march payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// The messenger's one-way outbound→arrived flip is the idempotency claim
	// for this whole event (mirrors OrderDeliveryHandler/MarchRecallHandler):
	// 0 rows affected means an earlier delivery of this exact event already
	// ran to completion — whatever it decided (turned the army, or recorded a
	// miss) already happened and must never be repeated, in particular never
	// re-notify the owner of a miss that was already reported once.
	alreadyProcessed := false
	if p.MessengerID != uuid.Nil {
		mct, err := tx.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1 AND status != 'arrived'`, p.MessengerID)
		if err != nil {
			return fmt.Errorf("claim recall messenger: %w", err)
		}
		alreadyProcessed = mct.RowsAffected() == 0
	}

	// Atomic claim: only turn the army around if its outbound march is still unresolved.
	ct, err := tx.Exec(ctx,
		`UPDATE marching_armies SET resolved=true WHERE id=$1 AND resolved=false`, p.MarchID)
	if err != nil {
		return fmt.Errorf("claim outbound march: %w", err)
	}
	if ct.RowsAffected() == 0 {
		if alreadyProcessed {
			slog.Info("recall arrival event replayed — already processed, no-op", "march_id", p.MarchID)
			return tx.Commit(ctx)
		}
		// First (and only) processing of this event, and the army resolved
		// (combat, colonization, another order…) before the recall messenger
		// caught up — a genuine race loss, distinguishable from a replay only
		// via the messenger claim above. Never silent (megaron_arbetssatt.md:
		// "a silent fallback that guesses a value is a bug that does not
		// crash") — the owner is told what happened and why.
		var ownerID uuid.UUID
		_ = tx.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE province_id = $1 AND world_id = $2`,
			p.OriginID, p.WorldID).Scan(&ownerID)
		slog.Info("recall messenger arrived but army already resolved — recall missed", "march_id", p.MarchID)
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		if h.hub != nil && ownerID != uuid.Nil {
			_ = h.hub.NotifyPlayer(ctx, p.WorldID, ownerID, "OrderFailed", 2, map[string]any{
				"march_id": p.MarchID,
				"verb":     "recall",
				"q":        p.TargetQ, // interception hex — where the recall messenger caught up
				"r":        p.TargetR,
				"reason": "your recall messenger reached the army, but it had already resolved (combat, colonization, " +
					"or another order) before the messenger arrived — the army was not turned back; check its outcome " +
					"and issue fresh orders if it still needs them",
			})
		}
		return nil
	}

	// Return trip = full distance origin↔target × terrain cost (army turns around at the target).
	var targetTerrain string
	if err := tx.QueryRow(ctx, `SELECT terrain_type FROM provinces WHERE id=$1`, p.TargetID).Scan(&targetTerrain); err != nil || targetTerrain == "" {
		targetTerrain = "plains"
	}
	dist := province.HexDistance(
		province.MapPosition{Q: p.OriginQ, R: p.OriginR},
		province.MapPosition{Q: p.TargetQ, R: p.TargetR},
	)
	now := h.clk.Now()
	returnsAt := now.Add(returnDuration(dist, targetTerrain))

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	dueTick := currentTick + returnTicks(dist, targetTerrain)

	var returnMarchID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO marching_armies
		 (world_id, origin_id, target_id, infantry, chariot, ship, elite_infantry,
		  war_galley, merchantman, intent, departs_at, arrives_at, depart_tick, arrive_tick)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'return',$10,$11,$12,$13)
		 RETURNING id`,
		p.WorldID, p.TargetID, p.OriginID,
		p.Spearman, p.WarChariot, p.Ship, p.EliteInfantry,
		p.WarGalley, p.Merchantman,
		now, returnsAt, currentTick, dueTick,
	).Scan(&returnMarchID); err != nil {
		return fmt.Errorf("create return march: %w", err)
	}

	// Atomic with the claim — no orphan return march if we crash before the worker marks done.
	if err := h.scheduler.EnqueueTickTx(ctx, tx, p.WorldID, events.ScheduledArmyArrival,
		armyArrivalPayload{MarchingArmyID: returnMarchID}, dueTick); err != nil {
		return fmt.Errorf("schedule army arrival: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	slog.Info("recall messenger reached army, return march started",
		"march_id", p.MarchID, "return_march_id", returnMarchID, "returns_at", returnsAt, "dist", dist)
	return nil
}

// returnDuration is the wall-clock travel time of a return march over dist hexes of the given
// terrain, for the marching_armies.arrives_at DISPLAY column. Actual scheduling uses returnTicks
// (1 tick = 1 game hour); this must convert those same ticks through the real tick cadence
// (tick.RealUntil, exact via TickSeconds) rather than assuming "1 game hour = 1 real hour" — that
// assumption only holds at the default 60 min/tick cadence, and was silently wrong on any faster
// world (and, with the since-retired TickMinutes conversion, still wrong on a sub-minute
// TICK_SECONDS cadence — TickMinutes floors to 1 minute), producing arrives_at timestamps ahead of
// the real (tick-driven) delivery and negative "ago"/ETA once the CLI compared them to wall time.
func returnDuration(dist int, terrain string) time.Duration {
	return tick.RealUntil(returnTicks(dist, terrain), 0)
}

// returnTicks converts a terrain-weighted march distance to world ticks (1 tick = 1 game hour).
func returnTicks(dist int, terrain string) int {
	hours := float64(dist) * province.TerrainMoveTicks(terrain)
	t := int(math.Round(hours))
	if t < 1 {
		return 1
	}
	return t
}

// MessengerTravelDuration returns the wall-clock travel time for a messenger over dist hexes,
// for the messengers.arrives_at DISPLAY column. Scheduling uses MessengerTravelTicks (1 tick =
// 1 game hour); converted through tick.RealUntil for the same reason as returnDuration above —
// a fixed "1 game hour = 1 real hour" assumption drifts from the actual tick-driven delivery on
// any world not running the default 60 min/tick cadence, and TickMinutes' 1-minute floor drifts
// again on a sub-minute TICK_SECONDS dev cadence.
func MessengerTravelDuration(dist int) time.Duration {
	return tick.RealUntil(MessengerTravelTicks(dist), 0)
}

// MessengerTravelTicks returns the world-tick travel time for a messenger over dist hexes.
func MessengerTravelTicks(dist int) int {
	t := int(math.Round(float64(dist) * TicksPerHex))
	if t < 1 {
		return 1
	}
	return t
}

// CourierTravel returns the world-tick and wall-clock travel time for a
// runner from 'from' to 'to' (temenos_orderlopare_plan.md Fas 4): A*
// over the courier graph — land at half a land unit's terrain ticks (2×
// spearman speed), sea legs at the flat boat rate province.CourierSeaTicks,
// mountains routed around. Falls back to the legacy straight-line rate when no
// route exists (should be unreachable with sea passable — e.g. a target walled
// in by mountains) so an order is never stranded by the pathfinder.
// One speed model for ALL messengers: diplomatic, recall/redirect and order
// runners alike (trade CARAVANS keep their own TradeTicksPerHex seam below).
func CourierTravel(ctx context.Context, db province.Queryer, worldID uuid.UUID, from, to province.MapPosition) (ticks int, dur time.Duration) {
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		dist := province.HexDistance(from, to)
		return MessengerTravelTicks(dist), MessengerTravelDuration(dist)
	}
	return CourierTravelOnGraph(g, from, to)
}

// CourierTravelOnGraph is CourierTravel over an already-loaded TileGraph —
// avoids re-querying every map_tile per call. Used by InterceptAlongPath,
// which evaluates many candidate hexes against the same courier origin within
// one request; loading the graph once instead of once per candidate is the
// difference between one DB round-trip and dozens.
func CourierTravelOnGraph(g province.TileGraph, from, to province.MapPosition) (ticks int, dur time.Duration) {
	if _, hours, ok := g.FindPath(from, to, province.CategoryCourier); ok {
		t := int(math.Round(hours))
		if t < 1 {
			t = 1
		}
		return t, tick.RealUntil(t, 0)
	}
	dist := province.HexDistance(from, to)
	return MessengerTravelTicks(dist), MessengerTravelDuration(dist)
}

// AggregateArmyCategory guesses the province.FindPath category ("land" or
// "naval") a marching_armies aggregate row moves under, from its unit
// composition. marching_armies has no "embarked" flag to tell a mixed
// land+naval force apart, so this is a heuristic, not a certainty: any naval
// hull present (ship/war_galley/merchantman) means the whole force's motion
// is bound to water passability — land troops cannot cross open water on
// their own — so naval wins whenever a hull is present. Callers that need
// certainty (e.g. RecallMarch, before trusting this for pathfinding) verify
// with province.FindPath and fall back to the other category if this guess
// finds no route.
func AggregateArmyCategory(spearman, warChariot, ship, eliteInfantry, warGalley, merchantman int) string {
	if ship+warGalley+merchantman > 0 {
		return "naval"
	}
	return "land"
}

// InterceptAlongPath finds the earliest hex along a marching unit's
// already-validated outbound path (as returned by province.FindPath) where a
// courier departing courierOrigin at now can reach the unit BEFORE it does —
// an honest moving-target intercept, rather than aiming at the unit's
// position at the dispatch instant (temenos_orderlopare_plan.md interception
// fix, 2026-07-30). Aiming at that instant's snapshot is always stale: the
// courier takes time to reach even that point, and the unit keeps moving
// while it travels — precisely the silent "runner arrived, unit long gone"
// miss this replaces.
//
// Scans path indices from the start (skipping every hex the unit has already
// passed) to the last (the march's own destination). CategoryCourier runs at
// 2× a land unit's speed (sea legs at the flat CourierSeaTicks rate), so an
// intercept normally exists somewhere along the remaining path — the earlier
// on the path it is found, the sooner the order is delivered.
//
// ok=false means no hex on the remaining path — including the destination
// itself — is reachable before the unit passes it: courierOrigin is too far
// (or the remaining march too short) for any physically-honest courier to
// catch this unit. Callers must treat that as a genuine, visible dispatch
// failure; queuing a courier anyway would only reproduce the silent miss this
// function exists to prevent.
func InterceptAlongPath(
	g province.TileGraph, courierOrigin province.MapPosition, path []province.MapPosition,
	departsAt, arrivesAt, now time.Time,
) (target province.MapPosition, ok bool) {
	n := len(path)
	if n < 2 {
		return province.MapPosition{}, false
	}
	total := arrivesAt.Sub(departsAt)
	for i := 0; i < n; i++ {
		// unitTime is the instant the unit reaches path[i], on the same
		// index-fraction-of-total model province.InterpolatePosition uses for
		// "where is the unit right now" — the last hex is pinned to the exact
		// stored arrivesAt to avoid float round-trip drift.
		unitTime := arrivesAt
		if i < n-1 {
			frac := float64(i) / float64(n-1)
			unitTime = departsAt.Add(time.Duration(frac * float64(total)))
		}
		if !unitTime.After(now) {
			continue // the unit has already passed (or is exactly at) this hex
		}
		// <= (not strict <): an exact tie — the courier's rounded-to-the-tick
		// ETA lands on the same instant the unit reaches this hex — is a real
		// 50/50 race at that tick (whichever scheduled event a poll happens to
		// process first), not a provable miss. Rejecting ties outright would
		// wrongly fail the ordinary "recall from the very city the unit
		// marched out of, right at the march's halfway point" case: a courier
		// at 2× a land unit's speed departing from the SAME origin always
		// needs exactly half the march's total duration to reach the
		// destination, which ties the remaining time exactly at the halfway
		// mark. Ties are let through; a genuine loss of that race still ends
		// in a visible OrderFailed (order_delivery.go), never a silent one.
		_, courierDur := CourierTravelOnGraph(g, courierOrigin, path[i])
		if !now.Add(courierDur).After(unitTime) {
			return path[i], true
		}
	}
	return province.MapPosition{}, false
}

// InterceptCourierTarget loads the marching unit's outbound path and the
// world's terrain graph, then delegates to InterceptAlongPath. err is
// non-nil only for DB/scan failures; ok=false (err==nil) covers both "no
// route exists" (mirrors province.InterpolatePosition's own ok=false) and
// "a route exists but no courier can catch the unit on it" — see
// InterceptAlongPath's doc comment for the latter.
func InterceptCourierTarget(
	ctx context.Context, db province.Queryer, worldID uuid.UUID,
	courierOrigin, marchOrigin, marchTarget province.MapPosition, category string,
	departsAt, arrivesAt, now time.Time,
) (target province.MapPosition, ok bool, err error) {
	path, _, pathOK, err := province.FindPath(ctx, db, worldID, marchOrigin, marchTarget, category)
	if err != nil {
		return province.MapPosition{}, false, err
	}
	if !pathOK {
		return province.MapPosition{}, false, nil
	}
	g, err := province.LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return province.MapPosition{}, false, err
	}
	t, ok := InterceptAlongPath(g, courierOrigin, path, departsAt, arrivesAt, now)
	return t, ok, nil
}

// TradeTicksPerHex is the travel speed of a trade caravan (the silver/goods legs of a messenger trade).
// Kept as a separate seam from messengers so caravans can later be tuned slower than runners
// without affecting messenger/recall speed.
const TradeTicksPerHex = 0.5

// TradeTravelDuration returns the wall-clock travel time for a trade caravan over dist hexes,
// for display columns only (scheduling uses TradeTravelTicks) — same tick.RealUntil conversion
// as MessengerTravelDuration/returnDuration above.
func TradeTravelDuration(dist int) time.Duration {
	return tick.RealUntil(TradeTravelTicks(dist), 0)
}

// TradeTravelTicks returns the world-tick travel time for a trade caravan over dist hexes.
func TradeTravelTicks(dist int) int {
	t := int(math.Round(float64(dist) * TradeTicksPerHex))
	if t < 1 {
		return 1
	}
	return t
}
