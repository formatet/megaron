package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
)

// TradeReturnHandler delivers goods to the buyer after a trade offer is accepted.
type TradeReturnHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
}

// NewTradeReturnHandler creates a TradeReturnHandler.
func NewTradeReturnHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster) *TradeReturnHandler {
	return &TradeReturnHandler{pool: pool, eventStore: eventStore, hub: hub}
}

// Handle credits goods to the buyer settlement when a negotiated trade return arrives.
func (h *TradeReturnHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		DestinationID uuid.UUID `json:"destination_id"` // buyer's settlement
		GoodKey       string    `json:"good_key"`
		Quantity      float64   `json:"quantity"`
		MessengerID   uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal trade return: %w", err)
	}

	// Idempotency: check messenger offer not already returned.
	var offerStatus string
	if err := h.pool.QueryRow(ctx,
		`SELECT trade_offer->>'status' FROM messengers WHERE id=$1`,
		p.MessengerID,
	).Scan(&offerStatus); err != nil {
		return nil // messenger gone
	}
	if offerStatus == "returned" {
		return nil
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Credit goods to buyer — silver is now a normal good in settlement_goods.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 VALUES ($1, $2, $3, 0, 100, now())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_at)
		             + $3,
		         settlement_goods.cap),
		     calc_at = now()`,
		p.DestinationID, p.GoodKey, p.Quantity,
	); err != nil {
		return fmt.Errorf("credit goods to buyer: %w", err)
	}

	// Mark as returned.
	if _, err = tx.Exec(ctx,
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"returned"}' WHERE id=$1`,
		p.MessengerID,
	); err != nil {
		return fmt.Errorf("mark returned: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if _, err = h.eventStore.Append(ctx, p.DestinationID, events.StreamProvince, "TradeReturn",
		map[string]any{"good_key": p.GoodKey, "quantity": p.Quantity, "messenger_id": p.MessengerID},
		e.WorldID, &e.ID,
	); err != nil {
		slog.Error("record TradeReturn event", "err", err)
	}

	if h.hub != nil {
		var ownerID uuid.UUID
		_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, p.DestinationID).Scan(&ownerID)
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, ownerID, "TradeReturn", 3, map[string]any{
			"destination_id": p.DestinationID,
			"good_key":       p.GoodKey,
			"quantity":       p.Quantity,
		})
	}

	slog.Info("trade return delivered", "buyer", p.DestinationID, "good", p.GoodKey, "qty", p.Quantity)
	return nil
}
