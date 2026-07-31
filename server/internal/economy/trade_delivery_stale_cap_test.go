package economy

// Investigation (megaron_todo.md §Kalibrering & mätning, "Cedar 'have 0' direkt
// efter TradeDelivery/bygge", sond r4): while verifying whether Build/Craft/Recruit
// could see a stale "have 0" right after a trade delivery, deductGoods (helpers.go)
// and the silver/ingredient UPDATE guards in province.go were confirmed atomic
// (FOR UPDATE inside one tx, or a single guarded UPDATE statement) — no race window
// there. But DeliveryHandler.Handle (trade.go) and TradeReturnHandler.Handle
// (trade_return.go) both still hard-code cap=100 on the INSERT branch of their
// settlement_goods upsert — a leftover from before the 2026-07-05 cap loosening
// (see province.go's own Craft comment: "the old hard-coded 100 here... pinned
// every craft output... at a binding low ceiling", and arrival.go's "cap 1_000_000
// for a brand-new row matches economy.goodCap ... never reintroduce the cap-1000
// bug"). goodCap() (recompute.go) always returns 1_000_000 — trade.go/trade_return.go
// were missed in that sweep. This doesn't zero out the FIRST delivery (the INSERT
// sets amount directly, uncapped), but it silently caps the settlement's supply of
// that good at 100 forever after — a second delivery (a second trade, or a later
// recompute() pass for a locally-producible good) clamps the real total down to
// 100, silently discarding the rest. That is the DB-provable, narrow bug found here;
// the exact "have 0" mechanism from sond r4 was not reproduced (see write-up).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// TestTradeDelivery_StaleCapTruncatesSecondDelivery shows DeliveryHandler crediting
// a good a settlement has never held (e.g. cedar bought from a seller, "sell" kind,
// leg 1) creates the settlement_goods row with cap=100. A second, independent trade
// delivery of the same good then gets silently truncated at 100 instead of summing.
func TestTradeDelivery_StaleCapTruncatesSecondDelivery(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-stalecap-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-stalecap-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"stalecap-"+uuid.New().String(), "stalecap-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner)

	var prov, buyer uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID).Scan(&prov)
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Buyer', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&buyer); err != nil {
		t.Fatalf("create buyer settlement: %v", err)
	}

	// Sanity: buyer has never held cedar before.
	var preExists bool
	_ = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar')`,
		buyer,
	).Scan(&preExists)
	if preExists {
		t.Fatalf("test setup invalid: buyer already has a cedar row")
	}

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	// This test's subject is the stale hard-coded cap, not the loss die — pin
	// the dice to "never loses" so an unlucky roll can't turn this into an
	// intermittent failure (fix/forlusttarning-injicerbar, 2026-07-31).
	h.Dice = neverLosesDice()

	deliver := func(qty float64, n int) {
		payload, _ := json.Marshal(map[string]any{
			"destination_id":     buyer,
			"good_key":           "cedar",
			"quantity":           qty,
			"delivered_quantity": qty,
			// transport_id omitted (zero UUID) — "legacy event", skips the
			// interception veto so the test doesn't need a transports row.
		})
		if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano() + int64(n), WorldID: worldID, Payload: payload}); err != nil {
			t.Fatalf("delivery %d handle: %v", n, err)
		}
	}

	// First delivery: 60 cedar, never held before → INSERT branch.
	deliver(60, 1)

	var amount, cap float64
	_ = pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick), cap FROM settlement_goods
		 WHERE settlement_id=$1 AND good_key='cedar'`, buyer,
	).Scan(&amount, &cap)
	if amount != 60 {
		t.Errorf("after 1st delivery: cedar amount = %v, want 60", amount)
	}
	// This is the actual bug: cap should be economy's shared 1_000_000 ceiling
	// (goodCap("cedar")), matching every other credit path (arrival.go
	// TransferDelivered, province.go Craft output). trade.go's INSERT hard-codes
	// 100 instead.
	if cap != goodCap("cedar") {
		t.Errorf("after 1st delivery: cedar cap = %v, want %v (goodCap) — stale hard-coded cap in DeliveryHandler.Handle (trade.go)", cap, goodCap("cedar"))
	}

	// Second, independent delivery: another 60 cedar (e.g. a second trade). With a
	// correct 1_000_000 cap this sums to 120. With the stale cap=100 it silently
	// truncates to 100 — 20 cedar vanish with no error, no event, no trace.
	deliver(60, 2)

	_ = pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods
		 WHERE settlement_id=$1 AND good_key='cedar'`, buyer,
	).Scan(&amount)
	if amount != 120 {
		t.Errorf("after 2nd delivery: cedar amount = %v, want 120 (60+60) — stale cap=100 in DeliveryHandler.Handle silently discarded %v cedar", amount, 120-amount)
	}
}

// TestTradeReturn_StaleCapTruncatesSecondDelivery mirrors the above for
// TradeReturnHandler (trade_return.go) — the "buy" kind's leg 2, which delivers the
// purchased good back to the ORIGINAL buyer. Same stale cap=100 literal, same bug.
func TestTradeReturn_StaleCapTruncatesSecondDelivery(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-stalecapret-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-stalecapret-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"stalecapret-"+uuid.New().String(), "stalecapret-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner)

	var provSeller, provBuyer, seller, buyer uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID).Scan(&provSeller)
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 3, 0, 'plains') RETURNING id`,
		worldID).Scan(&provBuyer)
	_ = pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Seller', 'achaean', $3, 'capital', true, 'active', 5000) RETURNING id`,
		worldID, provSeller, owner,
	).Scan(&seller)
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Buyer', 'achaean', $3, 'capital', false, 'active', 5000) RETURNING id`,
		worldID, provBuyer, owner,
	).Scan(&buyer); err != nil {
		t.Fatalf("create buyer settlement: %v", err)
	}

	newMessenger := func() uuid.UUID {
		var mid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text,
			                         status, hex_q, hex_r, arrives_at, trade_offer, kind)
			 VALUES ($1, $2, $3, $4, 'trade offer', 'delivered', 0, 0, now(),
			         '{"status":"accepted"}'::jsonb, 'trade')
			 RETURNING id`,
			worldID, owner, buyer, seller,
		).Scan(&mid); err != nil {
			t.Fatalf("create messenger: %v", err)
		}
		return mid
	}

	h := NewTradeReturnHandler(pool, events.NewStore(pool), nil)
	// Same as the DeliveryHandler test above: this test's subject is the
	// stale hard-coded cap, not the loss die.
	h.Dice = neverLosesDice()

	deliver := func(qty float64, n int) {
		payload, _ := json.Marshal(map[string]any{
			"destination_id": buyer,
			"good_key":       "cedar",
			"quantity":       qty,
			"messenger_id":   newMessenger(),
			// transport_id omitted (zero UUID) — skips the interception veto.
		})
		if err := h.Handle(ctx, events.ScheduledEvent{ID: time.Now().UnixNano() + int64(n), WorldID: worldID, Payload: payload}); err != nil {
			t.Fatalf("trade return %d handle: %v", n, err)
		}
	}

	deliver(60, 1)

	var amount, cap float64
	_ = pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick), cap FROM settlement_goods
		 WHERE settlement_id=$1 AND good_key='cedar'`, buyer,
	).Scan(&amount, &cap)
	if amount != 60 {
		t.Errorf("after 1st return: cedar amount = %v, want 60", amount)
	}
	if cap != goodCap("cedar") {
		t.Errorf("after 1st return: cedar cap = %v, want %v (goodCap) — stale hard-coded cap in TradeReturnHandler.Handle (trade_return.go)", cap, goodCap("cedar"))
	}

	deliver(60, 2)

	_ = pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods
		 WHERE settlement_id=$1 AND good_key='cedar'`, buyer,
	).Scan(&amount)
	if amount != 120 {
		t.Errorf("after 2nd return: cedar amount = %v, want 120 (60+60) — stale cap=100 in TradeReturnHandler.Handle silently discarded %v cedar", amount, 120-amount)
	}
}
