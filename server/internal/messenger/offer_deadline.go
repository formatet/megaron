package messenger

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// pendingOfferExpiryTick reads the already scheduled deadline, including offers
// created under an older lifetime. Existing timers are immutable: arrival and
// passage replanning must not shorten or extend them. If historical replanning
// left several timers, the earliest will actually expire the offer.
func pendingOfferExpiryTick(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*int, error) {
	var due int
	err := tx.QueryRow(ctx, `SELECT due_tick FROM scheduled_events
 WHERE event_type='OfferExpiry' AND payload->>'messenger_id'=$1
 AND processed_at IS NULL AND failed_at IS NULL ORDER BY due_tick,id LIMIT 1`, id.String()).Scan(&due)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &due, nil
}
