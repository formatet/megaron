package economy

// fix/intern-overforing-forlustrisk (2026-07-30): DeliveryHandler.Handle rolled
// tradeRiskPct on every ScheduledTradeDelivery, including a wanax's own logistics
// (own settlement → own settlement) — contradicting CLAUDE.md's trade-lagret
// invariant #3 ("intern överföring ... fysisk karavan utan förlust") and
// province_trade_internal_test.go's comment describing that very roll as
// "pre-existing, unrelated behaviour this slice does not touch". This file
// proves the fix: internal transfers never roll the die, external trades still
// do, and the pre-existing interception veto (unrelated to ownership) is
// untouched by the change.

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mkTradeOwner creates a bare player row and returns its id.
func mkTradeOwner(t *testing.T, pool *pgxpool.Pool, ctx context.Context) uuid.UUID {
	t.Helper()
	var owner uuid.UUID
	name := "trisk-" + uuid.New().String()
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		name,
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}
	return owner
}

// mkTradeSettlement creates a settlement owned by owner at hex (q,0).
func mkTradeSettlement(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID, owner uuid.UUID, name string, q int) uuid.UUID {
	t.Helper()
	var prov, id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
		worldID, q,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5, 'active', 5000) RETURNING id`,
		worldID, prov, name, owner, q == 0,
	).Scan(&id); err != nil {
		t.Fatalf("create settlement %s: %v", name, err)
	}
	return id
}

// mkTradeWorld creates one active test world for this file's tests.
func mkTradeWorld(t *testing.T, pool *pgxpool.Pool, ctx context.Context) uuid.UUID {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-triskloss-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })
	return worldID
}

// mkTradeRoute inserts a resolved=false trade_routes row (the bookkeeping row
// province.go's internal-transfer /trade branch creates) and returns its id.
func mkTradeRoute(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID, origin, dest uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO trade_routes (world_id, origin_id, destination_id, good_key, quantity, arrives_at)
		 VALUES ($1, $2, $3, 'silver', 1, now()) RETURNING id`,
		worldID, origin, dest,
	).Scan(&id); err != nil {
		t.Fatalf("create trade_routes row: %v", err)
	}
	return id
}

// mkTradeTransport inserts an in_transit transports row and returns its id.
func mkTradeTransport(t *testing.T, pool *pgxpool.Pool, ctx context.Context, worldID, owner, origin, dest uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,3,0, now(), now(), 1, 'in_transit', true)
		 RETURNING id`,
		worldID, owner, origin, dest,
	).Scan(&id); err != nil {
		t.Fatalf("create transports row: %v", err)
	}
	return id
}

// TestIsInternalTransfer_FourCases is bevisplan (a): the ownership resolution
// isolated from the dice roll, in the four combinations the plan calls for —
// route-based vs transport-based origin lookup, crossed with same vs different
// owner. An unresolvable fifth case (no route, no transport) is included too,
// since AK4 requires "can't tell → treat as external", not "assume internal".
func TestIsInternalTransfer_FourCases(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)

	ownerA := mkTradeOwner(t, pool, ctx)
	ownerB := mkTradeOwner(t, pool, ctx)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	t.Run("route-based same owner", func(t *testing.T) {
		origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "R-Origin-Same", 0)
		dest := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "R-Dest-Same", 1)
		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		if !isInternalTransfer(ctx, tx, routeID, uuid.UUID{}, dest) {
			t.Errorf("route-based, same owner: got external, want internal")
		}
	})

	t.Run("route-based different owner", func(t *testing.T) {
		origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "R-Origin-Diff", 2)
		dest := mkTradeSettlement(t, pool, ctx, worldID, ownerB, "R-Dest-Diff", 3)
		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		if isInternalTransfer(ctx, tx, routeID, uuid.UUID{}, dest) {
			t.Errorf("route-based, different owner: got internal, want external")
		}
	})

	t.Run("transport-based same owner (no route — messenger-style payload)", func(t *testing.T) {
		origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "T-Origin-Same", 4)
		dest := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "T-Dest-Same", 5)
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerA, origin, dest)
		if !isInternalTransfer(ctx, tx, uuid.UUID{}, transportID, dest) {
			t.Errorf("transport-based, same owner: got external, want internal")
		}
	})

	t.Run("transport-based different owner", func(t *testing.T) {
		origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "T-Origin-Diff", 6)
		dest := mkTradeSettlement(t, pool, ctx, worldID, ownerB, "T-Dest-Diff", 7)
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerA, origin, dest)
		if isInternalTransfer(ctx, tx, uuid.UUID{}, transportID, dest) {
			t.Errorf("transport-based, different owner: got internal, want external")
		}
	})

	t.Run("unresolvable origin defaults to external", func(t *testing.T) {
		dest := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "U-Dest", 8)
		if isInternalTransfer(ctx, tx, uuid.UUID{}, uuid.UUID{}, dest) {
			t.Errorf("no route, no transport: got internal, want external (AK4 — never guess internal)")
		}
	})
}

// deliveryPayload builds the exact JSON shape DeliveryHandler.Handle consumes,
// mirroring province.go's internal-transfer emitter (trade_route_id + transport_id
// both set — the real internal-transfer shape).
func deliveryPayload(routeID, destID, transportID uuid.UUID, qty float64) []byte {
	b, _ := json.Marshal(map[string]any{
		"trade_route_id":     routeID,
		"destination_id":     destID,
		"good_key":           "silver",
		"quantity":           qty,
		"delivered_quantity": qty,
		"transport_id":       transportID,
	})
	return b
}

// TestDeliveryHandler_InternalTransferNeverLost is bevisplan (b): N=120 fresh
// internal-transfer deliveries (own settlement → own settlement), each its own
// trade_routes + transports row, run through the real DeliveryHandler.Handle.
// With the bug, each has an independent 5% chance of being marked 'lost';
// P(zero losses in 120 | bug present) = 0.95^120 ≈ 0.2%, so a green run here is
// strong evidence the fix works, not luck. N and the math are written down so
// a future reader can tell the bound was chosen, not stumbled on.
func TestDeliveryHandler_InternalTransferNeverLost(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	owner := mkTradeOwner(t, pool, ctx)
	origin := mkTradeSettlement(t, pool, ctx, worldID, owner, "N-Origin", 0)
	dest := mkTradeSettlement(t, pool, ctx, worldID, owner, "N-Dest", 1)

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))

	const n = 120
	lost := 0
	for i := 0; i < n; i++ {
		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		transportID := mkTradeTransport(t, pool, ctx, worldID, owner, origin, dest)
		payload := deliveryPayload(routeID, dest, transportID, 10)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i), WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("delivery handle #%d: %v", i, err)
		}
		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			lost++
		}
	}
	if lost != 0 {
		t.Fatalf("internal transfers lost = %d/%d, want 0 (P(this by chance if unfixed) ~= 0.2%%)", lost, n)
	}

	var destSilver float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, dest).Scan(&destSilver)
	if destSilver != float64(n)*10 {
		t.Fatalf("dest silver = %v, want %v (every one of %d deliveries credited)", destSilver, float64(n)*10, n)
	}
}

// TestDeliveryHandler_ExternalTradeStillRollsDice is bevisplan (c): the mirror
// of the above — N=120 external deliveries (different owners) through the same
// handler must still see the storm/pirate roll fire at least once, proving the
// fix gates the roll on ownership rather than silencing it globally.
// P(zero losses in 120 | correctly gated) ≈ 0.2%, same bound as above.
func TestDeliveryHandler_ExternalTradeStillRollsDice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	ownerA := mkTradeOwner(t, pool, ctx)
	ownerB := mkTradeOwner(t, pool, ctx)
	origin := mkTradeSettlement(t, pool, ctx, worldID, ownerA, "E-Origin", 0)
	dest := mkTradeSettlement(t, pool, ctx, worldID, ownerB, "E-Dest", 1)

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	// Seeded via the injected Dice rather than left on the global source: the
	// test still proves the roll fires for external trade, but deterministically.
	// Unpinned it carried P(zero losses) = 0.95^120 ~= 0.2% — the same lottery
	// this seam exists to remove (fix/forlusttarning-injicerbar, 2026-07-31).
	h.Dice = rand.New(rand.NewSource(1337))

	const n = 120
	lost := 0
	for i := 0; i < n; i++ {
		routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
		transportID := mkTradeTransport(t, pool, ctx, worldID, ownerA, origin, dest)
		payload := deliveryPayload(routeID, dest, transportID, 10)
		ev := events.ScheduledEvent{ID: time.Now().UnixNano() + int64(i) + 1_000_000, WorldID: worldID, Payload: payload}
		if err := h.Handle(ctx, ev); err != nil {
			t.Fatalf("delivery handle #%d: %v", i, err)
		}
		var status string
		_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
		if status != "delivered" {
			lost++
		}
	}
	if lost == 0 {
		t.Fatalf("external trades lost = 0/%d, want >=1 (the dice must still roll for real trade; P(this by chance) ~= 0.2%%)", n)
	}
}

// TestDeliveryHandler_InterceptedInternalTransferStillVetoed is bevisplan (d):
// the interception veto (pre-existing, ownership-independent) must keep firing
// for an internal transfer exactly as it does for external — an intercepted
// transport must never credit the destination, regardless of who owns both ends.
func TestDeliveryHandler_InterceptedInternalTransferStillVetoed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID := mkTradeWorld(t, pool, ctx)
	owner := mkTradeOwner(t, pool, ctx)
	origin := mkTradeSettlement(t, pool, ctx, worldID, owner, "I-Origin", 0)
	dest := mkTradeSettlement(t, pool, ctx, worldID, owner, "I-Dest", 1)

	routeID := mkTradeRoute(t, pool, ctx, worldID, origin, dest)
	transportID := mkTradeTransport(t, pool, ctx, worldID, owner, origin, dest)
	if _, err := pool.Exec(ctx,
		`UPDATE transports SET status = 'intercepted', updated_at = now() WHERE id = $1`, transportID,
	); err != nil {
		t.Fatalf("simulate interception: %v", err)
	}

	h := NewDeliveryHandler(pool, events.NewStore(pool), nil, events.NewScheduler(pool, clock.NewTestClock(time.Now())))
	payload := deliveryPayload(routeID, dest, transportID, 500)
	ev := events.ScheduledEvent{ID: time.Now().UnixNano(), WorldID: worldID, Payload: payload}
	if err := h.Handle(ctx, ev); err != nil {
		t.Fatalf("delivery handle: %v", err)
	}

	var destSilver float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'silver'`, dest).Scan(&destSilver)
	if destSilver != 0 {
		t.Fatalf("dest silver = %v, want 0 (intercepted internal transfer must not deliver)", destSilver)
	}

	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1`, transportID).Scan(&status)
	if status != "intercepted" {
		t.Errorf("transport status = %q, want intercepted (must stay, not flip to delivered)", status)
	}
}
