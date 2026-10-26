package handlers

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// deliverFollowingCourier advances a test world's actual clock to each saved
// Runner deadline and executes the CURRENT scheduled generation. A moving unit
// may have left the first rendezvous, so one callback need not deliver it. The
// world anchor advances with the clock, as the real tick worker would, rather
// than freezing v1 tick-position readers at the original dispatch instant.
func deliverFollowingCourier(t *testing.T, pool *pgxpool.Pool, clk *clock.TestClock, handler *messenger.OrderDeliveryHandler, id uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	for leg := 0; leg < 20; leg++ {
		var status string
		var deadline time.Time
		var world uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT world_id,status,arrives_at FROM messengers WHERE id=$1`, id).Scan(&world, &status, &deadline); err != nil {
			t.Fatal(err)
		}
		if status == "arrived" {
			return
		}
		if status != "outbound" {
			t.Fatalf("Runner ended without delivery: %s", status)
		}
		if deadline.Before(clk.Now()) {
			deadline = clk.Now()
		}
		advanceOrderTestClock(t, pool, clk, world, deadline)
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT e.payload FROM scheduled_events e JOIN messengers m ON m.id=$1
 WHERE e.event_type IN ('OrderDelivery','OrderDeliveryV2') AND e.payload->>'messenger_id'=m.id::text
 AND COALESCE((e.payload->>'passage_generation')::int,0)=m.passage_generation
 AND e.processed_at IS NULL AND e.failed_at IS NULL ORDER BY e.id DESC LIMIT 1`, id).Scan(&raw); err != nil {
			t.Fatalf("current Runner generation missing: %v", err)
		}
		if err := handler.Handle(ctx, events.ScheduledEvent{WorldID: world, Payload: raw}); err != nil {
			t.Fatalf("physical Runner leg %d: %v", leg, err)
		}
	}
	t.Fatal("Runner did not reach a physical meeting within 20 scheduled legs")
}

// Advance time without changing any mover's saved arrival timestamp.
func advanceOrderTestClock(t *testing.T, pool *pgxpool.Pool, clk *clock.TestClock, world uuid.UUID, at time.Time) {
	t.Helper()
	ctx := context.Background()
	if at.Before(clk.Now()) {
		t.Fatal("test clock must not move backwards")
	}
	a, err := tick.LoadAnchor(ctx, pool, world)
	if err != nil {
		t.Fatal(err)
	}
	cadence := time.Duration(tick.TickSeconds) * time.Second
	elapsed := int(at.Sub(a.At) / cadence)
	if elapsed > 0 {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2,last_tick_at=$3 WHERE id=$1`, world, a.Tick+elapsed, a.At.Add(time.Duration(elapsed)*cadence)); err != nil {
			t.Fatal(err)
		}
	}
	clk.Set(at)
}
