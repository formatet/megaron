package messenger

// OrderDelivery: an order courier physically reaching its unit
// (temenos_orderlopare_plan.md Fas 2). The runner carries the order envelope;
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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/combat"
	"github.com/poleia/server/internal/events"
)

// OrderDeliveryPayload is the ScheduledOrderDelivery payload.
type OrderDeliveryPayload struct {
	WorldID     uuid.UUID          `json:"world_id"`
	PlayerID    uuid.UUID          `json:"player_id"`
	UnitID      uuid.UUID          `json:"unit_id"`
	MessengerID uuid.UUID          `json:"messenger_id"`
	Verb        string             `json:"verb"` // "march" (Fas 2; more verbs in Fas 3)
	March       *combat.MarchOrder `json:"march,omitempty"`
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
			var rej *combat.MarchReject
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
	default:
		return fmt.Errorf("order delivery %s: unknown verb %q", p.MessengerID, p.Verb)
	}
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
