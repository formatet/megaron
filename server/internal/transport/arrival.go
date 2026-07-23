package transport

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArrivalHandler credits a transport's manifest to its destination settlement when
// the caravan/ship arrives. It is the physical-mover successor to LogisticsArrivalHandler.
//
// Idempotent (CLAUDE.md "Event handlers"): claims the event in processed_deliveries,
// then re-checks the transport is still in transit under FOR UPDATE — so an
// interception (Del 3-fas-4) that flipped status to 'intercepted' cancels delivery,
// and a re-run never double-credits.
type ArrivalHandler struct {
	pool *pgxpool.Pool
}

// NewArrivalHandler creates an ArrivalHandler.
func NewArrivalHandler(pool *pgxpool.Pool) *ArrivalHandler {
	return &ArrivalHandler{pool: pool}
}

// Handle delivers the manifest to the destination, or no-ops if the transport was
// already delivered, intercepted, or lost.
func (h *ArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		TransportID uuid.UUID `json:"transport_id"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal transport arrival: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Exactly-once claim.
	ct, err := tx.Exec(ctx,
		`INSERT INTO processed_deliveries (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, e.ID)
	if err != nil {
		return fmt.Errorf("claim transport arrival: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil // already processed
	}

	// Re-check the mover is still in transit; interception/loss cancels delivery.
	var status string
	var destID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT status, dest_id FROM transports WHERE id = $1 FOR UPDATE`, p.TransportID,
	).Scan(&status, &destID); err != nil {
		return fmt.Errorf("load transport: %w", err)
	}
	if status != "in_transit" {
		return nil // intercepted, lost, or already delivered
	}
	if destID == nil {
		// Destination vanished (settlement removed). Nothing to credit — close it out.
		if _, err := tx.Exec(ctx,
			`UPDATE transports SET status = 'lost', updated_at = now() WHERE id = $1`, p.TransportID,
		); err != nil {
			return fmt.Errorf("mark transport lost: %w", err)
		}
		return tx.Commit(ctx)
	}

	// Credit each manifest good to the destination. cap 1_000_000 for a brand-new
	// row matches economy.goodCap (post-00d0722; never reintroduce the cap-1000 bug).
	rows, err := tx.Query(ctx,
		`SELECT good_key, quantity FROM transport_goods WHERE transport_id = $1`, p.TransportID)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	type item struct {
		good string
		qty  float64
	}
	var manifest []item
	for rows.Next() {
		var it item
		if scanErr := rows.Scan(&it.good, &it.qty); scanErr != nil {
			rows.Close()
			return fmt.Errorf("scan manifest: %w", scanErr)
		}
		manifest = append(manifest, it)
	}
	rows.Close()

	for _, it := range manifest {
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, 1000000, current_world_tick())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount = LEAST(
			         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick) + $3,
			         settlement_goods.cap),
			     calc_tick = current_world_tick()`,
			*destID, it.good, it.qty,
		); err != nil {
			return fmt.Errorf("credit good %q: %w", it.good, err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE transports SET status = 'delivered', updated_at = now() WHERE id = $1`, p.TransportID,
	); err != nil {
		return fmt.Errorf("mark transport delivered: %w", err)
	}

	return tx.Commit(ctx)
}
