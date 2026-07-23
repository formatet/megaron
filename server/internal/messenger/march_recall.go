package messenger

// MarchRecall: a recall or redirect messenger reaching a marching discrete unit
// (units table, C1-C8 model). Distinct from RecallArrivalHandler above, which
// turns around legacy marching_armies/outposts — see temenos_march_recall.md.
//
// Idempotent via an atomic conditional claim: the unit is locked FOR UPDATE and
// the handler no-ops if it is no longer marching (already arrived, or an earlier
// order already turned it).

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
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MarchRecallPayload is the ScheduledMarchRecall payload.
type MarchRecallPayload struct {
	WorldID     uuid.UUID `json:"world_id"`
	UnitID      uuid.UUID `json:"unit_id"`
	MessengerID uuid.UUID `json:"messenger_id"`
	Mode        string    `json:"mode"` // "recall" | "redirect"
	NewTargetQ  *int      `json:"new_target_q,omitempty"`
	NewTargetR  *int      `json:"new_target_r,omitempty"`
}

// MarchRecallHandler processes ScheduledMarchRecall events.
type MarchRecallHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	hub        combat.Broadcaster
	clk        clock.Clock
}

// NewMarchRecallHandler creates a MarchRecallHandler.
func NewMarchRecallHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store, hub combat.Broadcaster, clk clock.Clock) *MarchRecallHandler {
	return &MarchRecallHandler{pool: pool, scheduler: scheduler, eventStore: eventStore, hub: hub, clk: clk}
}

// Handle processes one ScheduledMarchRecall scheduled event.
func (h *MarchRecallHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p MarchRecallPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal march recall payload: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Atomic claim: a recalled unit stays status='marching' throughout (it's just
	// on a new leg), so unlike combat/colonize arrivals we can't rely on a status
	// flip to catch a re-delivered event. The messenger's own outbound→arrived
	// transition is the one-way claim instead — a replay of the same event finds
	// it already 'arrived' and no-ops before touching the unit.
	ct, err := tx.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1 AND status != 'arrived'`, p.MessengerID)
	if err != nil {
		return fmt.Errorf("claim messenger: %w", err)
	}
	if ct.RowsAffected() == 0 {
		slog.Info("march recall/redirect messenger already processed — idempotent replay skipped", "messenger", p.MessengerID)
		return tx.Commit(ctx)
	}

	var ownerID uuid.UUID
	var category, status string
	var q, r int
	var targetQ, targetR *int // nulled once the unit is no longer marching
	var departsAt, arrivesAt *time.Time
	var marchIntent, colonyName *string
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, category, status, q, r, target_q, target_r, departs_at, arrives_at, march_intent, colony_name
		 FROM units WHERE id = $1 FOR UPDATE`,
		p.UnitID,
	).Scan(&ownerID, &category, &status, &q, &r, &targetQ, &targetR, &departsAt, &arrivesAt, &marchIntent, &colonyName); err != nil {
		return fmt.Errorf("load recalled unit: %w", err)
	}

	// Too late / already handled: the unit finished its march (or an earlier
	// order on the same unit already turned it) before this one caught up.
	if status != string(unit.StatusMarching) || targetQ == nil || targetR == nil || departsAt == nil || arrivesAt == nil {
		slog.Info("recall/redirect messenger arrived but unit no longer marching — order missed",
			"unit", p.UnitID, "status", status, "mode", p.Mode)
		return tx.Commit(ctx)
	}

	origin := province.MapPosition{Q: q, R: r}
	target := province.MapPosition{Q: *targetQ, R: *targetR}
	now := h.clk.Now()

	currentPos, posOK, err := province.InterpolatePosition(ctx, tx, p.WorldID, origin, target, category, *departsAt, *arrivesAt, now)
	if err != nil {
		return fmt.Errorf("interpolate unit position: %w", err)
	}
	if !posOK {
		slog.Warn("march recall: could not re-walk outbound path, using origin as current position", "unit", p.UnitID)
		currentPos = origin
	}

	newTarget := origin // recall: head home to where the unit departed from
	if p.Mode == "redirect" && p.NewTargetQ != nil && p.NewTargetR != nil {
		newTarget = province.MapPosition{Q: *p.NewTargetQ, R: *p.NewTargetR}
	}

	// Route over the same passability graph the outbound march used — never a
	// straight-line teleport across impassable terrain. The straight-line
	// fallback should not trigger in practice: for recall, currentPos lies on
	// the very path that proved origin↔target traversable; for redirect, the
	// new target was validated at dispatch time.
	_, pathHours, pathOK, pathErr := province.FindPath(ctx, tx, p.WorldID, currentPos, newTarget, category)
	var moveHours float64
	if pathErr == nil && pathOK {
		moveHours = pathHours
	} else {
		if pathErr != nil {
			slog.Warn("march recall: FindPath error, falling back to straight line", "unit", p.UnitID, "err", pathErr)
		} else {
			slog.Warn("march recall: no route found, falling back to straight line", "unit", p.UnitID)
		}
		dist := province.HexDistance(currentPos, newTarget)
		if dist < 1 {
			dist = 1
		}
		moveHours = province.TerrainMoveHours("plains") * float64(dist)
	}

	arrivesAtNew := now.Add(time.Duration(moveHours * float64(time.Hour)))
	var currentTick int
	_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	travelTicks := int(math.Round(moveHours))
	if travelTicks < 1 {
		travelTicks = 1
	}

	// Recall clears any lingering colonize intent (heading home, not to found a
	// colony); redirect keeps it — the unit still tries to fulfil it at the new target.
	newIntent, newColonyName := marchIntent, colonyName
	if p.Mode == "recall" {
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
		p.UnitID, currentPos.Q, currentPos.R, newTarget.Q, newTarget.R, now, arrivesAtNew, newIntent, newColonyName,
		currentTick, currentTick+travelTicks,
	); err != nil {
		return fmt.Errorf("turn unit toward new course: %w", err)
	}

	arrPayload := unit.ScheduledUnitArrivalPayload{UnitID: p.UnitID, WorldID: p.WorldID}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, p.WorldID, events.ScheduledUnitArrival, arrPayload, currentTick+travelTicks); err != nil {
		return fmt.Errorf("schedule new arrival: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if p.Mode == "redirect" {
		_, _ = h.eventStore.Append(ctx, p.UnitID, events.StreamType(unit.StreamUnit), unit.EventUnitMarchRedirected,
			unit.MarchRedirectedPayload{
				UnitID: p.UnitID, FromQ: currentPos.Q, FromR: currentPos.R,
				NewTargetQ: newTarget.Q, NewTargetR: newTarget.R,
				ArrivesAt: arrivesAtNew.Format(time.RFC3339),
			}, p.WorldID, nil)
	} else {
		_, _ = h.eventStore.Append(ctx, p.UnitID, events.StreamType(unit.StreamUnit), unit.EventUnitMarchRecalled,
			unit.MarchRecalledPayload{
				UnitID: p.UnitID, FromQ: currentPos.Q, FromR: currentPos.R,
				OriginQ: newTarget.Q, OriginR: newTarget.R,
				ArrivesAt: arrivesAtNew.Format(time.RFC3339),
			}, p.WorldID, nil)
	}

	if h.hub != nil {
		notifKind := "UnitRecalled"
		if p.Mode == "redirect" {
			notifKind = "UnitRedirected"
		}
		_ = h.hub.NotifyPlayer(ctx, p.WorldID, ownerID, notifKind, 3, map[string]any{
			"unit_id":    p.UnitID,
			"q":          currentPos.Q,
			"r":          currentPos.R,
			"target_q":   newTarget.Q,
			"target_r":   newTarget.R,
			"arrives_at": arrivesAtNew,
		})
	}

	slog.Info("march order reached unit, new course set", "unit", p.UnitID, "mode", p.Mode,
		"from_q", currentPos.Q, "from_r", currentPos.R, "to_q", newTarget.Q, "to_r", newTarget.R, "arrives_at", arrivesAtNew)
	return nil
}
