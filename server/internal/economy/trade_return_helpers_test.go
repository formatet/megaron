package economy

// Shared fixtures for TradeReturnHandler tests. The return leg's flat loss die
// (and the tests that pinned it) were removed in slice T (megaron_transportrisk.md);
// see trade_no_flat_die_test.go for the rule that replaced them.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeReturnBroadcaster records every NotifyPlayer call's kind + payload, for
// AK3 (a loss must notify the buyer the same way TradeDelivery's loss does).
type fakeReturnBroadcaster struct {
	notified []string
	payloads []map[string]any
}

func (f *fakeReturnBroadcaster) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (f *fakeReturnBroadcaster) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.notified = append(f.notified, kind)
	if m, ok := payload.(map[string]any); ok {
		f.payloads = append(f.payloads, m)
	}
	return nil
}

// mkReturnMessenger creates a messenger row with an "accepted" trade_offer —
// the state a return leg's messenger is in when ScheduledTradeReturn fires
// (mirrors trade_delivery_stale_cap_test.go's newMessenger closure).
func mkReturnMessenger(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID, sender, origin, dest uuid.UUID) uuid.UUID {
	t.Helper()
	var mid uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text,
		                         status, hex_q, hex_r, arrives_at, trade_offer, kind)
		 VALUES ($1, $2, $3, $4, 'trade offer', 'delivered', 0, 0, now(),
		         '{"status":"accepted"}'::jsonb, 'trade')
		 RETURNING id`,
		worldID, sender, origin, dest,
	).Scan(&mid); err != nil {
		t.Fatalf("create messenger: %v", err)
	}
	return mid
}

// returnPayload builds the exact JSON shape TradeReturnHandler.Handle consumes.
func returnPayload(destID, messengerID, transportID uuid.UUID, goodKey string, qty float64) []byte {
	b, _ := json.Marshal(map[string]any{
		"destination_id": destID,
		"good_key":       goodKey,
		"quantity":       qty,
		"messenger_id":   messengerID,
		"transport_id":   transportID,
	})
	return b
}

func settlementGoodAmount(t *testing.T, pool *pgxpool.Pool, ctx context.Context, settlementID uuid.UUID, goodKey string) float64 {
	t.Helper()
	var amt float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id=$1 AND good_key=$2`, settlementID, goodKey,
	).Scan(&amt)
	return amt
}
