package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/notify"
)

// DeliveryHandler processes ScheduledTradeDelivery events.
type DeliveryHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        *notify.Hub
}

// NewDeliveryHandler creates a DeliveryHandler.
func NewDeliveryHandler(pool *pgxpool.Pool, eventStore *events.Store, hub *notify.Hub) *DeliveryHandler {
	return &DeliveryHandler{pool: pool, eventStore: eventStore, hub: hub}
}

// Handle delivers goods to the destination settlement.
func (h *DeliveryHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		TradeRouteID      uuid.UUID `json:"trade_route_id"`
		DestinationID     uuid.UUID `json:"destination_id"`
		GoodKey           string    `json:"good_key"`
		Quantity          float64   `json:"quantity"`
		DeliveredQuantity float64   `json:"delivered_quantity"` // includes distance bonus; falls back to Quantity if zero
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal trade delivery: %w", err)
	}
	// Backward-compat: old events without delivered_quantity use raw quantity.
	delivered := p.DeliveredQuantity
	if delivered <= 0 {
		delivered = p.Quantity
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Idempotency: check route not already resolved (FOR UPDATE prevents races).
	var resolved bool
	if err := tx.QueryRow(ctx,
		`SELECT resolved FROM trade_routes WHERE id = $1 FOR UPDATE`,
		p.TradeRouteID,
	).Scan(&resolved); err != nil {
		return nil // Route gone or already cleaned up.
	}
	if resolved {
		return nil
	}

	// Credit goods to destination settlement (lazy-eval aware).
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 VALUES ($1, $2, $3, 0, 100, now())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settlement_goods.amount
		             + EXTRACT(EPOCH FROM (now() - settlement_goods.calc_at))/60 * settlement_goods.rate
		             + $3,
		         settlement_goods.cap),
		     calc_at = now()`,
		p.DestinationID, p.GoodKey, delivered,
	); err != nil {
		return fmt.Errorf("credit goods: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE trade_routes SET resolved = true WHERE id = $1`,
		p.TradeRouteID,
	); err != nil {
		return fmt.Errorf("mark resolved: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if _, err := h.eventStore.Append(ctx, p.DestinationID, events.StreamProvince, "TradeDelivery",
		map[string]any{"good_key": p.GoodKey, "quantity": delivered, "route_id": p.TradeRouteID},
		e.WorldID, &e.ID,
	); err != nil {
		slog.Error("record TradeDelivery event", "err", err)
	}

	if h.hub != nil {
		h.hub.Broadcast(e.WorldID, notify.Msg{
			Kind: "TradeDelivery",
			Payload: map[string]any{
				"destination_id": p.DestinationID,
				"good_key":       p.GoodKey,
				"quantity":       delivered,
			},
		})
	}

	// Record market snapshot: the caravan owner now knows the destination's prices.
	var ownerID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT owner_id FROM trade_routes WHERE id = $1`, p.TradeRouteID,
	).Scan(&ownerID); err == nil {
		if snapErr := RecordMarketSnapshot(ctx, h.pool, ownerID, p.DestinationID); snapErr != nil {
			slog.Error("market snapshot on delivery", "err", snapErr)
		}
	}

	slog.Info("trade delivery", "destination", p.DestinationID, "good", p.GoodKey, "qty", delivered)
	return nil
}
