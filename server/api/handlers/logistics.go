package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
)

// LogisticsArrivalHandler credits the destination of an internal physical transfer when the
// caravan arrives: a royal gift to one of your own settlements, or a tribute caravan to the
// kingdom treasury. Internal logistics are NOT subject to trade-caravan loss — they are your
// own supply lines, not external trade.
//
// Idempotent: claims the event in processed_deliveries before crediting (CLAUDE.md "Event handlers").
type LogisticsArrivalHandler struct {
	pool *pgxpool.Pool
}

// NewLogisticsArrivalHandler creates a LogisticsArrivalHandler.
func NewLogisticsArrivalHandler(pool *pgxpool.Pool) *LogisticsArrivalHandler {
	return &LogisticsArrivalHandler{pool: pool}
}

// Handle credits silver/goods to a settlement, or silver to a kingdom treasury, on arrival.
func (h *LogisticsArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		Kind        string    `json:"kind"`        // "settlement_good" | "treasury"
		Destination uuid.UUID `json:"destination"` // settlement id or kingdom id
		GoodKey     string    `json:"good_key"`    // "silver" (currency) or a good key (e.g. "grain")
		Quantity    float64   `json:"quantity"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal logistics arrival: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Exactly-once claim — the worker marks the event done after this commits, so a crash
	// in between would otherwise re-run the handler and double-credit.
	ct, err := tx.Exec(ctx,
		`INSERT INTO processed_deliveries (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, e.ID)
	if err != nil {
		return fmt.Errorf("claim logistics delivery: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil // already processed in an earlier run
	}

	switch p.Kind {
	case "treasury":
		// Silver into the kingdom treasury (kingdoms.silver_amount is the silver balance).
		if _, err = tx.Exec(ctx,
			`UPDATE kingdoms SET
			     silver_amount  = settled(silver_amount, silver_rate, silver_calc_at) + $1,
			     silver_calc_at = now()
			 WHERE id = $2`,
			p.Quantity, p.Destination,
		); err != nil {
			return fmt.Errorf("credit treasury: %w", err)
		}
	case "settlement_good":
		if p.GoodKey == "silver" {
			if _, err = tx.Exec(ctx,
				`UPDATE settlements SET
				     silver_amount  = LEAST(
				         settled(silver_amount, silver_rate, silver_calc_at) + $1,
				         silver_cap),
				     silver_calc_at = now()
				 WHERE id = $2`,
				p.Quantity, p.Destination,
			); err != nil {
				return fmt.Errorf("credit settlement silver: %w", err)
			}
		} else {
			if _, err = tx.Exec(ctx,
				`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
				 VALUES ($1, $2, $3, 0, 1000, now())
				 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
				     amount  = LEAST(
				         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_at)
				             + $3,
				         settlement_goods.cap),
				     calc_at = now()`,
				p.Destination, p.GoodKey, p.Quantity,
			); err != nil {
				return fmt.Errorf("credit settlement good: %w", err)
			}
		}
	default:
		return fmt.Errorf("unknown logistics kind: %q", p.Kind)
	}

	return tx.Commit(ctx)
}
