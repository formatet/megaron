package combat

// ExecuteRecall: the recall/redirect order's execute core, extracted from
// messenger.MarchRecallHandler.Handle (temenos_orderlopare_plan.md "recall/redirect→kuvert-unifiering")
// so the new order-envelope "recall"/"redirect" verbs (delivered by
// messenger.OrderDeliveryHandler) and the frozen ScheduledMarchRecall path can
// eventually share one execution path. Verbatim logic move from
// march_recall.go's post-claim body — no behaviour change.
//
// The caller owns the messenger row's outbound→arrived idempotency claim
// (mirrors StartMarch/SetStance: this function opens its own transaction,
// separate from that claim — same relaxed two-commit pattern OrderDeliveryHandler
// already uses for march/stance). A nil, nil return means the unit is no longer
// marching by the time this runs (already arrived, or an earlier order already
// turned it) — a stale miss, not a rejection: the caller must treat it as a
// silent no-op, never an OrderFailed notice.

import (
	"context"
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
// to the hex it departed from, redirect sets a new target. Both re-interpolate
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
	var q, r int
	var targetQ, targetR *int // nulled once the unit is no longer marching
	var departsAt, arrivesAt *time.Time
	var marchIntent, colonyName *string
	var cargoUnitID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, type, category, status, q, r, target_q, target_r, departs_at, arrives_at, march_intent, colony_name, cargo_unit_id
		 FROM units WHERE id = $1 FOR UPDATE`,
		o.UnitID,
	).Scan(&ownerID, &utype, &category, &status, &q, &r, &targetQ, &targetR, &departsAt, &arrivesAt, &marchIntent, &colonyName, &cargoUnitID); err != nil {
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

	currentPos, posOK, err := province.InterpolatePosition(ctx, tx, o.WorldID, origin, target, category, *departsAt, *arrivesAt, now)
	if err != nil {
		return nil, fmt.Errorf("interpolate unit position: %w", err)
	}
	if !posOK {
		slog.Warn("recall/redirect: could not re-walk outbound path, using origin as current position", "unit", o.UnitID)
		currentPos = origin
	}

	newTarget := origin // recall: head home to where the unit departed from
	if o.Mode == "redirect" && o.NewTargetQ != nil && o.NewTargetR != nil {
		newTarget = province.MapPosition{Q: *o.NewTargetQ, R: *o.NewTargetR}
	}

	// Route over the same passability graph the outbound march used — never a
	// straight-line teleport across impassable terrain. The straight-line
	// fallback should not trigger in practice: for recall, currentPos lies on
	// the very path that proved origin↔target traversable; for redirect, the
	// new target was validated at dispatch time.
	_, pathHours, pathOK, pathErr := province.FindPath(ctx, tx, o.WorldID, currentPos, newTarget, category)
	var moveHours float64
	if pathErr == nil && pathOK {
		moveHours = pathHours
	} else {
		if pathErr != nil {
			slog.Warn("recall/redirect: FindPath error, falling back to straight line", "unit", o.UnitID, "err", pathErr)
		} else {
			slog.Warn("recall/redirect: no route found, falling back to straight line", "unit", o.UnitID)
		}
		dist := province.HexDistance(currentPos, newTarget)
		if dist < 1 {
			dist = 1
		}
		moveHours = province.TerrainMoveHours("plains") * float64(dist)
	}
	// Mirror the outbound leg's speed multipliers (march_start.go StartMarch) —
	// a war galley/merchantman/nomadic host recalled or redirected mid-march
	// must keep its own speed, not the unmultiplied path cost.
	moveHours *= NavalSpeedFactor(unit.Type(utype))
	moveHours *= unit.MarchHoursFactorFor(unit.Type(utype))
	if cargoUnitID != nil {
		moveHours *= 1.5
	}

	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	travelTicks := int(math.Round(moveHours))
	if travelTicks < 1 {
		travelTicks = 1
	}
	// arrivesAtNew mirrors the real tick-scheduled arrival (travelTicks × real
	// seconds/tick) — NOT moveHours-as-hours. Using moveHours*time.Hour here
	// (the bug: a several-hour wall-clock ETA for a march that the tick
	// substrate actually completes in a handful of ticks/seconds) made a
	// recalled or redirected unit's map position crawl almost imperceptibly
	// slowly and then snap to its destination the moment a poll caught the
	// tick-driven arrival that had already happened — "sailed there but
	// teleported home".
	arrivesAtNew := now.Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)

	// Recall clears any lingering colonize intent (heading home, not to found a
	// colony); redirect keeps it — the unit still tries to fulfil it at the new target.
	newIntent, newColonyName := marchIntent, colonyName
	if o.Mode == "recall" {
		newIntent, newColonyName = nil, nil
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
		   updated_at   = now()
		 WHERE id = $1`,
		o.UnitID, currentPos.Q, currentPos.R, newTarget.Q, newTarget.R, now, arrivesAtNew, newIntent, newColonyName,
		currentTick, currentTick+travelTicks,
	); err != nil {
		return nil, fmt.Errorf("turn unit toward new course: %w", err)
	}

	arrPayload := unit.ScheduledUnitArrivalPayload{UnitID: o.UnitID, WorldID: o.WorldID}
	if err := scheduler.EnqueueTickTx(ctx, tx, o.WorldID, events.ScheduledUnitArrival, arrPayload, currentTick+travelTicks); err != nil {
		return nil, fmt.Errorf("schedule new arrival: %w", err)
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
