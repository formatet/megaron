package economy

// fix/forlusttarning-en-regel (2026-07-31): TradeReturnHandler.Handle credited the
// buyer unconditionally, never rolling tradeRiskPct — unlike DeliveryHandler (the
// outbound leg), which already gates the same roll on isInternalTransfer
// (6dc2105). A negotiated trade between two wanaxes is external on BOTH legs
// (CLAUDE.md trade-lagret punkt 2/3: only a same-owner transfer is "fysisk
// karavan utan förlust"), so the return leg must carry the same risk as the
// outbound leg. AK1 (red baseline): TestTradeReturnHandler_ExternalTradeRollsDice
// fails on unmodified trade_return.go — 0/200 losses instead of the expected >=1.
//
// Statistical tests are seeded (math/rand.Seed) so a run is reproducible, not
// flaky — CLAUDE.md/bevisplan. Each test seeds independently at its own start,
// none call t.Parallel(), so seeding one test cannot perturb another's sequence.
// N=200 draws at tradeRiskPct=0.05: P(zero losses by chance) = 0.95^200 ≈ 0.0035%.

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"formatet/megaron/server/internal/events"
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

// TestTradeReturnHandler_ExternalTradeRollsDice is AK1 (red baseline on
// unmodified trade_return.go) and AK2/AK3 once fixed: a negotiated trade
// between two different owners must lose goods at ~tradeRiskPct, never credit
// the buyer on a loss, and notify exactly like the outbound leg's TradeLost.
func TestTradeReturnHandler_ExternalTradeRollsDice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	rand.Seed(1337)

	worldID := mkTradeWorld(t, pool, ctx)
	ownerSeller := mkTradeOwner(t, pool, ctx)
	ownerBuyer := mkTradeOwner(t, pool, ctx)
	buyer := mkTradeSettlement(t, pool, ctx, worldID, ownerBuyer, "RetExt-Buyer", 0)
	seller := mkTradeSettlement(t, pool, ctx, worldID, ownerSeller, "RetExt-Seller", 1)

	fb := &fakeReturnBroadcaster{}
	h := NewTradeReturnHandler(pool, events.NewStore(pool), fb)

	const n = 200
	const qty = 10.0
	lost := 0
	for i := 0; i < n; i++ {
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerSeller, seller, buyer)
		messengerID := mkReturnMessenger(t, pool, ctx, worldID, ownerBuyer, buyer, seller)

		pre := settlementGoodAmount(t, pool, ctx, buyer, "silver")
		payload := returnPayload(buyer, messengerID, transportID, "silver", qty)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("trade return handle #%d: %v", i, err)
		}
		post := settlementGoodAmount(t, pool, ctx, buyer, "silver")

		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		switch status {
		case "lost":
			lost++
			if post != pre {
				t.Errorf("iter %d: lost leg credited buyer anyway: pre=%v post=%v (AK2 violated)", i, pre, post)
			}
		case "delivered":
			if post != pre+qty {
				t.Errorf("iter %d: delivered leg did not credit qty: pre=%v post=%v want %v", i, pre, post, pre+qty)
			}
		default:
			t.Errorf("iter %d: unexpected transport status %q", i, status)
		}
	}

	// AK1: this is the assertion that fails on unmodified trade_return.go
	// (0/200 losses, since the handler never rolled). P(false negative once
	// fixed) ≈ 0.0035%.
	if lost == 0 {
		t.Fatalf("external trade returns lost = 0/%d, want >=1 — TradeReturnHandler never rolls tradeRiskPct (AK1)", n)
	}
	if lost == n {
		t.Fatalf("external trade returns lost = %d/%d, want <%d — every return was lost, risk roll looks unconditional rather than gated at tradeRiskPct", lost, n, n)
	}
	t.Logf("external trade returns lost = %d/%d", lost, n)

	// AK3: a loss must notify the buyer, same event kind as the outbound leg.
	foundNotify := false
	for _, k := range fb.notified {
		if k == "TradeLost" {
			foundNotify = true
			break
		}
	}
	if !foundNotify {
		t.Errorf("NotifyPlayer calls = %v, want a \"TradeLost\" among them (AK3 — a lost return must not be silent)", fb.notified)
	}

	var lostEvents int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'TradeLost'`, buyer,
	).Scan(&lostEvents)
	if lostEvents == 0 {
		t.Errorf("events table has no TradeLost row for buyer %s (AK3 — the audit trail must record the loss)", buyer)
	}
}

// TestTradeReturnHandler_InternalNeverLost is AK4 (same owner ⇒ never rolls):
// a return leg between two settlements owned by the same wanax must never be
// marked lost, however many times it runs.
func TestTradeReturnHandler_InternalNeverLost(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	rand.Seed(4242)

	worldID := mkTradeWorld(t, pool, ctx)
	owner := mkTradeOwner(t, pool, ctx)
	buyer := mkTradeSettlement(t, pool, ctx, worldID, owner, "RetInt-Buyer", 0)
	seller := mkTradeSettlement(t, pool, ctx, worldID, owner, "RetInt-Seller", 1)

	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)

	const n = 150
	const qty = 7.0
	for i := 0; i < n; i++ {
		transportID := mkTradeTransport(t, pool, ctx, worldID, owner, seller, buyer)
		messengerID := mkReturnMessenger(t, pool, ctx, worldID, owner, buyer, seller)

		payload := returnPayload(buyer, messengerID, transportID, "grain", qty)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("trade return handle #%d: %v", i, err)
		}

		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			t.Fatalf("iter %d: same-owner return leg status = %q, want delivered (AK4 — same owner must never roll)", i, status)
		}
	}

	got := settlementGoodAmount(t, pool, ctx, buyer, "grain")
	want := float64(n) * qty
	if got != want {
		t.Fatalf("buyer grain = %v, want %v (every one of %d same-owner returns credited)", got, want, n)
	}
}

// TestTradeReturnHandler_UnresolvableOriginDefaultsExternal is AK4's other half
// (fail-external): a return leg with no transport row (legacy event — the
// pre-physical-caravan shape trade_delivery_stale_cap_test.go's fixtures also
// use) can't resolve an origin at all, and must be treated as external — it
// must still be able to roll the loss die, not silently default to internal.
func TestTradeReturnHandler_UnresolvableOriginDefaultsExternal(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	rand.Seed(90210)

	worldID := mkTradeWorld(t, pool, ctx)
	ownerBuyer := mkTradeOwner(t, pool, ctx)
	ownerSeller := mkTradeOwner(t, pool, ctx)
	buyer := mkTradeSettlement(t, pool, ctx, worldID, ownerBuyer, "RetUnk-Buyer", 0)
	seller := mkTradeSettlement(t, pool, ctx, worldID, ownerSeller, "RetUnk-Seller", 1)

	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)

	const n = 200
	const qty = 3.0
	lost := 0
	for i := 0; i < n; i++ {
		messengerID := mkReturnMessenger(t, pool, ctx, worldID, ownerBuyer, buyer, seller)
		pre := settlementGoodAmount(t, pool, ctx, buyer, "timber")
		// transport_id omitted (zero UUID): no transports row exists to resolve
		// an origin from — isInternalTransfer must fail-external here (AK4).
		payload := returnPayload(buyer, messengerID, uuid.UUID{}, "timber", qty)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("trade return handle #%d: %v", i, err)
		}
		post := settlementGoodAmount(t, pool, ctx, buyer, "timber")
		if post == pre {
			lost++
		} else if post != pre+qty {
			t.Errorf("iter %d: unexpected credit delta pre=%v post=%v", i, pre, post)
		}
	}

	if lost == 0 {
		t.Fatalf("unresolvable-origin returns lost = 0/%d, want >=1 (AK4 — no transport row must fail-external, not silently internal)", n)
	}
	t.Logf("unresolvable-origin returns lost = %d/%d", lost, n)
}
