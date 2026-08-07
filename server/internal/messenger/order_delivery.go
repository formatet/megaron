package messenger

// OrderDelivery: an order courier — a Runner (Timothy 2026-07-26) —
// physically reaching its unit (temenos_orderlopare_plan.md Fas 2). The runner carries the order envelope;
// the order executes only at delivery — command is never instant.
//
// Latest-delivered-wins (Timothy 2026-07-16): dispatch never guards on a
// pending order, so several couriers may race to the same unit; each delivery
// is evaluated against the unit's state at that moment. A delivery that is no
// longer valid (unit moved, died, started marching…) fails FINALLY with an
// OrderFailed notice to the owner — never silently.
//
// Idempotency: the messenger row's one-way outbound→arrived flip is the claim
// (same pattern as MarchRecallHandler). StartMarch runs its own transaction,
// so claim and execution are two commits; a crash between them drops the order
// after an ERROR log (visible), never doubles it — the unit's own status gate
// in StartMarch rejects a replayed execution.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderDeliveryPayload is the ScheduledOrderDelivery payload.
type OrderDeliveryPayload struct {
	WorldID     uuid.UUID           `json:"world_id"`
	PlayerID    uuid.UUID           `json:"player_id"`
	UnitID      uuid.UUID           `json:"unit_id"`
	MessengerID uuid.UUID           `json:"messenger_id"`
	Verb        string              `json:"verb"` // "march" | "stance" | "recall" | "redirect" | "occupy_action" | "standing_orders"
	March       *combat.MarchOrder  `json:"march,omitempty"`
	Stance      *combat.StanceOrder `json:"stance,omitempty"`
	Recall      *combat.RecallOrder `json:"recall,omitempty"`
	// StandingOrders carries the KR3 §5 mid-battle retreat order — see
	// combat.SetStandingOrders.
	StandingOrders *combat.StandingOrdersOrder `json:"standing_orders,omitempty"`
	// Occupy carries the S3 erövring choice (megaron_plan_erovring.md) — sack/
	// burn/annex an occupied city. Targets a SETTLEMENT, not a unit (UnitID is
	// left zero-value for this verb); the runner is dispatched to the city's
	// hex the same way as any other order (command is never instant, even for
	// an army already standing there).
	Occupy *combat.OccupyActionOrder `json:"occupy,omitempty"`
}

// OrderDeliveryHandler processes ScheduledOrderDelivery events.
type OrderDeliveryHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	hub        combat.Broadcaster
	clk        clock.Clock
}

// NewOrderDeliveryHandler creates an OrderDeliveryHandler.
func NewOrderDeliveryHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store, hub combat.Broadcaster, clk clock.Clock) *OrderDeliveryHandler {
	return &OrderDeliveryHandler{pool: pool, scheduler: scheduler, eventStore: eventStore, hub: hub, clk: clk}
}

// Handle processes one ScheduledOrderDelivery scheduled event.
func (h *OrderDeliveryHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p OrderDeliveryPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal order delivery payload: %w", err)
	}

	// Atomic claim: outbound→arrived is one-way; a replay no-ops here.
	ct, err := h.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1 AND status != 'arrived'`, p.MessengerID)
	if err != nil {
		return fmt.Errorf("claim order messenger: %w", err)
	}
	if ct.RowsAffected() == 0 {
		slog.Info("order messenger already processed — idempotent replay skipped", "messenger", p.MessengerID)
		return nil
	}

	switch p.Verb {
	case "march":
		if p.March == nil {
			return fmt.Errorf("order delivery %s: march verb without march payload", p.MessengerID)
		}
		// FOW was checked at dispatch (knowledge-at-order-time) — nil here.
		res, err := combat.StartMarch(ctx, h.pool, h.scheduler, h.eventStore, h.clk, *p.March, nil)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				// Game-rule rejection at delivery: final, never silent.
				h.notifyOrderFailed(ctx, p, rej.Reason)
				return nil
			}
			slog.Error("order delivery: march execution failed after claim — order dropped",
				"messenger", p.MessengerID, "unit", p.UnitID, "err", err)
			return nil
		}
		slog.Info("order delivered — march started",
			"unit", p.UnitID, "target_q", res.TargetQ, "target_r", res.TargetR, "arrival_tick", res.ArrivalTick)
		return nil
	case "stance":
		if p.Stance == nil {
			return fmt.Errorf("order delivery %s: stance verb without stance payload", p.MessengerID)
		}
		res, err := combat.SetStance(ctx, h.pool, h.eventStore, *p.Stance)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				h.notifyOrderFailed(ctx, p, rej.Reason)
				return nil
			}
			slog.Error("order delivery: stance execution failed after claim — order dropped",
				"messenger", p.MessengerID, "unit", p.UnitID, "err", err)
			return nil
		}
		slog.Info("order delivered — stance applied", "unit", p.UnitID, "stance", res.Stance)
		return nil
	case "recall", "redirect":
		if p.Recall == nil {
			return fmt.Errorf("order delivery %s: %s verb without recall payload", p.MessengerID, p.Verb)
		}
		res, err := combat.ExecuteRecall(ctx, h.pool, h.scheduler, h.eventStore, h.clk, *p.Recall)
		if err != nil {
			slog.Error("order delivery: recall/redirect execution failed after claim — order dropped",
				"messenger", p.MessengerID, "unit", p.UnitID, "verb", p.Verb, "err", err)
			return nil
		}
		if res == nil {
			// Unit no longer marching by the time the runner arrived (already
			// completed its march, or an earlier order already turned it) — a
			// genuine miss, not a game-rule rejection, but never silent
			// (temenos_orderlopare_plan.md interception fix, 2026-07-30):
			// dispatch now aims the Runner at a mathematically honest
			// interception point along the unit's path (unit.go's Recall
			// handler + messenger.InterceptCourierTarget), so this should be
			// a rare residual race rather than the everyday silent tap it
			// used to be — but "should be rare" is not "never", so the owner
			// still needs to hear about it.
			h.notifyOrderFailed(ctx, p, fmt.Sprintf(
				"your %s Runner reached the unit, but it was no longer marching there — it likely completed its march or was turned by an earlier order before this one caught up; check the unit's current status and reissue the order if it still needs one",
				p.Verb))
			return nil
		}
		notifKind := "UnitRecalled"
		if p.Verb == "redirect" {
			notifKind = "UnitRedirected"
		}
		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, p.WorldID, res.OwnerID, notifKind, 3, map[string]any{
				"unit_id":    res.UnitID,
				"q":          res.FromQ,
				"r":          res.FromR,
				"target_q":   res.NewTargetQ,
				"target_r":   res.NewTargetR,
				"arrives_at": res.ArrivesAt,
			})
		}
		slog.Info("order delivered — unit turned onto new course", "unit", p.UnitID, "verb", p.Verb, "arrives_at", res.ArrivesAt)
		return nil
	case "standing_orders":
		if p.StandingOrders == nil {
			return fmt.Errorf("order delivery %s: standing_orders verb without standing_orders payload", p.MessengerID)
		}
		res, err := combat.SetStandingOrders(ctx, h.pool, h.eventStore, *p.StandingOrders)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				// Most common case: the battle ended before the Runner arrived —
				// the order has nothing left to apply to, and the owner should
				// hear that rather than wonder why nothing changed.
				h.notifyOrderFailed(ctx, p, rej.Reason)
				return nil
			}
			slog.Error("order delivery: standing orders execution failed after claim — order dropped",
				"messenger", p.MessengerID, "unit", p.UnitID, "err", err)
			return nil
		}
		slog.Info("order delivered — standing orders applied",
			"unit", p.UnitID, "battle", res.BattleID, "hold_to_last_man", res.HoldToLastMan)
		return nil
	case "occupy_action":
		if p.Occupy == nil {
			return fmt.Errorf("order delivery %s: occupy_action verb without occupy payload", p.MessengerID)
		}
		res, err := combat.ExecuteOccupyAction(ctx, h.pool, h.scheduler, h.eventStore, h.clk, h.hub, *p.Occupy)
		if err != nil {
			var rej *combat.OrderReject
			if errors.As(err, &rej) {
				h.notifyOrderFailed(ctx, p, rej.Reason)
				return nil
			}
			slog.Error("order delivery: occupy action execution failed after claim — order dropped",
				"messenger", p.MessengerID, "settlement", p.Occupy.SettlementID, "action", p.Occupy.Action, "err", err)
			return nil
		}
		slog.Info("order delivered — occupation choice executed",
			"settlement", res.SettlementID, "action", res.Action)
		return nil
	default:
		return fmt.Errorf("order delivery %s: unknown verb %q", p.MessengerID, p.Verb)
	}
}

// NotifyDeadLetter is the events.Worker dead-letter hook for
// ScheduledOrderDelivery (P3 soak fix, 2026-07-19): if a courier's delivery
// keeps failing (transient DB/load errors — a game-rule OrderReject already
// notifies via notifyOrderFailed and never reaches dead-lettering) until it is
// dead-lettered, the owner is told instead of the order silently vanishing —
// the dispatch's 202 said a runner was en route, and without this the
// player would never learn it never arrived.
func (h *OrderDeliveryHandler) NotifyDeadLetter(ctx context.Context, e events.ScheduledEvent) error {
	var p OrderDeliveryPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("dead-letter: unmarshal order delivery payload: %w", err)
	}
	if h.hub == nil {
		return nil
	}
	_ = h.hub.NotifyPlayer(ctx, p.WorldID, p.PlayerID, "OrderFailed", 1, map[string]any{
		"unit_id": p.UnitID,
		"verb":    p.Verb,
		"reason":  fmt.Sprintf("your %s order's Runner could not deliver it after repeated attempts (system fault) — the unit never received it; check its status and reissue the order", p.Verb),
	})
	slog.Warn("dead-letter: notified owner of stalled order delivery", "messenger", p.MessengerID, "unit", p.UnitID, "verb", p.Verb)
	return nil
}

// notifyOrderFailed tells the owner their delivered order could not be carried
// out, and why — an explained failure, not a silent fizzle.
func (h *OrderDeliveryHandler) notifyOrderFailed(ctx context.Context, p OrderDeliveryPayload, reason string) {
	slog.Info("order delivered but no longer executable", "unit", p.UnitID, "verb", p.Verb, "reason", reason)
	if h.hub == nil {
		return
	}
	_ = h.hub.NotifyPlayer(ctx, p.WorldID, p.PlayerID, "OrderFailed", 2, map[string]any{
		"unit_id": p.UnitID,
		"verb":    p.Verb,
		"reason":  reason,
	})
}
