package messenger

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// returnDeclinedReservationTx gives a declined offer's held goods or silver
// back to the sender's city once the runner that carried the answer is home —
// or is lost, since the sender learns that at once (Timothy 2026-10-24,
// temenos_settlement "Svar och kartkunskap" p.3). TradeDecline marks the offer
// refund_on_return; the guarded flip makes a replay a no-op. Offers declined
// before the marker existed were refunded on the click and carry no marker.
func returnDeclinedReservationTx(ctx context.Context, tx pgx.Tx, messengerID uuid.UUID) error {
	var originID *uuid.UUID
	var kind, good string
	var silver, qty float64
	err := tx.QueryRow(ctx,
		`UPDATE messengers SET trade_offer = trade_offer || '{"refund_on_return":false}'
		  WHERE id = $1 AND trade_offer->>'status' = 'declined'
		    AND COALESCE((trade_offer->>'refund_on_return')::boolean, false)
		  RETURNING origin_id,
		            COALESCE(trade_offer->>'kind', 'buy'),
		            COALESCE((trade_offer->>'offer_silver')::float, 0),
		            COALESCE(trade_offer->>'offer_good', ''),
		            COALESCE((trade_offer->>'offer_qty')::float, 0)`,
		messengerID,
	).Scan(&originID, &kind, &silver, &good, &qty)
	if err == pgx.ErrNoRows || (err == nil && originID == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	// buy → silver back to the buyer; sell → goods back to the seller.
	if kind == "sell" {
		_, err = tx.Exec(ctx, `SELECT credit_settlement_good($2,$3,$1,'escrow_refund')`, qty, *originID, good)
	} else {
		_, err = tx.Exec(ctx, `SELECT credit_settlement_good($2,'silver',$1,'escrow_refund')`, silver, *originID)
	}
	return err
}
