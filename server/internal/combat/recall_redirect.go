package combat

// ExecuteRecall is the shared recall/redirect execution core for personal
// orders and messenger.OrderDeliveryHandler's recall/redirect envelopes.
//
// The caller owns the messenger row's outbound→arrived idempotency claim
// (mirrors StartMarch/SetStance: this function opens its own transaction,
// separate from that claim — same relaxed two-commit pattern OrderDeliveryHandler
// already uses for march/stance). A nil, nil return means the unit is no longer
// marching by the time this runs (already arrived, or an earlier order already
// turned it). The caller reports this miss to the player.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RecallOrder is one recall/redirect command against one marching unit.
type RecallOrder struct {
	WorldID    uuid.UUID
	UnitID     uuid.UUID
	Mode       string // "recall" | "redirect"
	NewTargetQ *int   // only set when Mode == "redirect"
	NewTargetR *int
}

// RecallApplied describes the unit's new course (for the caller's notify +
// log — mirrors MarchStarted/StanceApplied).
type RecallApplied struct {
	UnitID     uuid.UUID
	OwnerID    uuid.UUID
	Mode       string
	FromQ      int
	FromR      int
	NewTargetQ int
	NewTargetR int
	ArrivesAt  time.Time
}

// ExecuteRecall turns a marching unit onto a new course: recall heads it home
// to its expedition home (otherwise the departure hex); redirect sets a new
// target. Both re-interpolate
// the unit's actual current position along the path it already proved
// traversable, then route from there over the same passability graph — never
// a straight-line teleport. Returns (nil, nil) when the unit is no longer
// marching (too late — see doc comment above).
func ExecuteRecall(ctx context.Context, pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store, clk clock.Clock, o RecallOrder) (*RecallApplied, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var ownerID uuid.UUID
	var utype, category, status string
	var q, r, crew int
	var targetQ, targetR *int // nulled once the unit is no longer marching
	var departsAt, arrivesAt *time.Time
	var marchIntent, colonyName *string
	var cargoUnitID, homeSettlementID *uuid.UUID
	var departTick, arriveTick *int
	var storedRouteRaw []byte
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, type, category, status, q, r, crew, target_q, target_r, departs_at, arrives_at, march_intent, colony_name, cargo_unit_id, depart_tick, arrive_tick, march_route, home_settlement_id
		 FROM units WHERE id = $1 FOR UPDATE`,
		o.UnitID,
	).Scan(&ownerID, &utype, &category, &status, &q, &r, &crew, &targetQ, &targetR, &departsAt, &arrivesAt, &marchIntent, &colonyName, &cargoUnitID, &departTick, &arriveTick, &storedRouteRaw, &homeSettlementID); err != nil {
		return nil, fmt.Errorf("load recalled unit: %w", err)
	}

	// Too late / already handled: the unit finished its march (or an earlier
	// order on the same unit already turned it) before this one caught up.
	if status != string(unit.StatusMarching) || targetQ == nil || targetR == nil || departsAt == nil || arrivesAt == nil {
		slog.Info("recall/redirect order arrived but unit no longer marching — order missed",
			"unit", o.UnitID, "status", status, "mode", o.Mode)
		return nil, tx.Commit(ctx)
	}

	origin := province.MapPosition{Q: q, R: r}
	target := province.MapPosition{Q: *targetQ, R: *targetR}
	now := clk.Now()

	// movement 2a, R6.a: read via the saved route when it applies to this
	// march (invariant 2); otherwise fall back to the old re-walk, unchanged.
	var currentPos province.MapPosition
	posOK := false
	if activeRoute, ok := LoadActiveRoute(storedRouteRaw, status, departTick, arriveTick); ok {
		if anchor, aErr := tick.LoadAnchor(ctx, tx, o.WorldID); aErr == nil {
			if p, rErr := RoutePositionAt(activeRoute, anchor.MilliAt(now)); rErr == nil {
				currentPos, posOK = p, true
			}
		}
	}
	if !posOK {
		var ipErr error
		currentPos, posOK, ipErr = province.InterpolatePosition(ctx, tx, o.WorldID, origin, target, category, *departsAt, *arrivesAt, now)
		if ipErr != nil {
			return nil, fmt.Errorf("interpolate unit position: %w", ipErr)
		}
		if !posOK {
			return nil, reject(422, "cannot resolve the unit's current position: no passable outbound route; check this unit and reissue the order")
		}
	}

	newTarget := origin // Ordinary recall returns to the departure hex.
	expeditionRecall := o.Mode == "recall" && marchIntent != nil && (*marchIntent == "explore" || *marchIntent == "explore_return")
	if expeditionRecall {
		home, err := expeditionReturnDestination(ctx, tx, unitRow{ownerID: ownerID, category: category, homeSettlementID: homeSettlementID}, currentPos.Q, currentPos.R, o.WorldID)
		if err != nil {
			return nil, fmt.Errorf("resolve recalled expedition home: %w", err)
		}
		if home == nil {
			return nil, reject(422, "no reachable home settlement for this expedition; check this unit and choose a reachable destination")
		}
		newTarget = province.MapPosition{Q: home.q, R: home.r}
		homeSettlementID = &home.id
	}
	if o.Mode == "redirect" && o.NewTargetQ != nil && o.NewTargetR != nil {
		newTarget = province.MapPosition{Q: *o.NewTargetQ, R: *o.NewTargetR}
	}

	// Delivery rechecks the real route; a dispatch precheck is not authority
	// to move across a route that no longer exists when the order arrives.
	path, pathTicks, pathOK, pathErr := province.FindPath(ctx, tx, o.WorldID, currentPos, newTarget, category)
	if pathErr != nil {
		return nil, fmt.Errorf("resolve recall/redirect route: %w", pathErr)
	}
	if !pathOK {
		return nil, reject(422, "no passable route from the unit's current position (%d,%d) to (%d,%d); check this unit and choose a reachable destination", currentPos.Q, currentPos.R, newTarget.Q, newTarget.R)
	}
	moveTicks := pathTicks
	// Mirror the outbound leg's speed multipliers (march_start.go's
	// TravelFactor) — a war galley/merchantman/nomadic host recalled or
	// redirected mid-march must keep its own speed, not the unmultiplied
	// path cost.
	moveTicks *= TravelFactor(unit.Type(utype), crew, cargoUnitID != nil)

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	travelTicks := int(math.Round(moveTicks))
	if travelTicks < 1 {
		travelTicks = 1
	}
	// arrivesAtNew mirrors the real tick-scheduled arrival (travelTicks × real
	// seconds/tick) — NOT moveTicks-as-hours. Using moveTicks*time.Hour here
	// (the bug: a several-hour wall-clock ETA for a march that the tick
	// substrate actually completes in a handful of ticks/seconds) made a
	// recalled or redirected unit's map position crawl almost imperceptibly
	// slowly and then snap to its destination the moment a poll caught the
	// tick-driven arrival that had already happened — "sailed there but
	// teleported home".
	arrivesAtNew := now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

	// movement 2a, R1/R5: save the NEW leg's own path — never re-search it at
	// read time. A trivial path needs no stored movement route.
	var newMarchRoute []byte
	if len(path) >= 2 {
		if stepHours, shErr := province.StepHoursDB(ctx, tx, o.WorldID, path, category); shErr == nil {
			if route, ok := BuildRoute(path, stepHours, currentTick, currentTick+travelTicks); ok {
				if raw, mErr := json.Marshal(route); mErr == nil {
					newMarchRoute = raw
				}
			}
		}
	}

	// Recall clears any lingering colonize intent (heading home, not to found a
	// colony); redirect keeps it — the unit still tries to fulfil it at the new target.
	newIntent, newColonyName := marchIntent, colonyName
	if o.Mode == "recall" {
		newIntent, newColonyName = nil, nil
		if expeditionRecall {
			// The mission is cancelled below, but its return leg must still use the
			// home-settlement arrival path (especially a ship's adjacent sea hex).
			returnIntent := "explore_return"
			newIntent = &returnIntent
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   q            = $2,
		   r            = $3,
		   target_q     = $4,
		   target_r     = $5,
		   departs_at   = $6,
		   arrives_at   = $7,
		   march_intent = $8,
		   colony_name  = $9,
		   depart_tick  = $10,
		   arrive_tick  = $11,
		   march_route  = $12,
		   home_settlement_id = $13,
		   updated_at   = now()
		 WHERE id = $1`,
		o.UnitID, currentPos.Q, currentPos.R, newTarget.Q, newTarget.R, now, arrivesAtNew, newIntent, newColonyName,
		currentTick, currentTick+travelTicks, newMarchRoute, homeSettlementID,
	); err != nil {
		return nil, fmt.Errorf("turn unit toward new course: %w", err)
	}

	// Arrival ticks can coincide; cancellation is explicit, not inferred from time.
	if _, err := tx.Exec(ctx, `DELETE FROM unit_expeditions WHERE unit_id = $1`, o.UnitID); err != nil {
		return nil, fmt.Errorf("close recalled expedition: %w", err)
	}

	newArriveTick := currentTick + travelTicks
	arrPayload := unit.ScheduledUnitArrivalPayload{UnitID: o.UnitID, WorldID: o.WorldID, ArriveTick: &newArriveTick}
	// The identical pending arrival already resolves the current course at
	// this tick. Reuse it under the unit lock rather than failing queue dedup.
	rawArrival, _ := json.Marshal(arrPayload)
	var arrivalQueued bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scheduled_events WHERE world_id=$1 AND event_type=$2 AND due_tick=$3 AND payload=$4::jsonb AND processed_at IS NULL AND failed_at IS NULL)`,
		o.WorldID, string(events.ScheduledUnitArrival), newArriveTick, rawArrival).Scan(&arrivalQueued); err != nil {
		return nil, fmt.Errorf("check existing arrival: %w", err)
	}
	if !arrivalQueued {
		if err := scheduler.EnqueueTickTx(ctx, tx, o.WorldID, events.ScheduledUnitArrival, arrPayload, newArriveTick); err != nil {
			return nil, fmt.Errorf("schedule new arrival: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	if o.Mode == "redirect" {
		_, _ = eventStore.Append(ctx, o.UnitID, events.StreamType(unit.StreamUnit), unit.EventUnitMarchRedirected,
			unit.MarchRedirectedPayload{
				UnitID: o.UnitID, FromQ: currentPos.Q, FromR: currentPos.R,
				NewTargetQ: newTarget.Q, NewTargetR: newTarget.R,
				ArrivesAt: arrivesAtNew.Format(time.RFC3339),
			}, o.WorldID, nil)
	} else {
		_, _ = eventStore.Append(ctx, o.UnitID, events.StreamType(unit.StreamUnit), unit.EventUnitMarchRecalled,
			unit.MarchRecalledPayload{
				UnitID: o.UnitID, FromQ: currentPos.Q, FromR: currentPos.R,
				OriginQ: newTarget.Q, OriginR: newTarget.R,
				ArrivesAt: arrivesAtNew.Format(time.RFC3339),
			}, o.WorldID, nil)
	}

	slog.Info("recall/redirect order reached unit, new course set", "unit", o.UnitID, "mode", o.Mode,
		"from_q", currentPos.Q, "from_r", currentPos.R, "to_q", newTarget.Q, "to_r", newTarget.R, "arrives_at", arrivesAtNew)

	return &RecallApplied{
		UnitID: o.UnitID, OwnerID: ownerID, Mode: o.Mode,
		FromQ: currentPos.Q, FromR: currentPos.R,
		NewTargetQ: newTarget.Q, NewTargetR: newTarget.R,
		ArrivesAt: arrivesAtNew,
	}, nil
}
