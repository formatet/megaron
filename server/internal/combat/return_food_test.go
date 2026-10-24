package combat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The old trigger waits for crew loss. A full crew with just enough food to
// return must instead leave now, before the next stationary day eats its way home.
func TestReturnFoodLeavesBeforeCrewStarves(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "food-return")
	seedGoods(t, pool, f.capitalID, 100, 100000, 100000)
	ship := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, f.capitalID)
	if _, err := pool.Exec(ctx, `UPDATE units SET provisions=0.4,provision_goods='{"grain":0.4}' WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	arrivals := NewUnitArrivalHandler(pool, events.NewStore(pool), nil, sched, clk)
	h := NewUpkeepHandler(pool, sched, events.NewStore(pool), nil, arrivals, sched.Clock())
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1610001, WorldID: f.worldID}); err != nil {
		t.Fatal(err)
	}
	var status string
	var crew int
	if err := pool.QueryRow(ctx, `SELECT status,crew FROM units WHERE id=$1`, ship).Scan(&status, &crew); err != nil {
		t.Fatal(err)
	}
	if status != "marching" || crew != 20 {
		t.Fatalf("home reserve should turn full crew home before starvation: status=%s crew=%d", status, crew)
	}
	// Nothing reaches the owner from the distant sea; the report comes home with the ship.
	if n := streamEvents(t, pool, ship, "ShipCameHomeForFood"); n != 0 {
		t.Fatalf("homecoming report before the ship is home: %d", n)
	}
	var tick int
	if err := pool.QueryRow(ctx, `SELECT arrive_tick FROM units WHERE id=$1`, ship).Scan(&tick); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, tick); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: ship, WorldID: f.worldID, ArriveTick: &tick})
	if err := arrivals.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status,crew FROM units WHERE id=$1`, ship).Scan(&status, &crew); err != nil {
		t.Fatal(err)
	}
	if status != "garrison" || crew != 20 || streamEvents(t, pool, ship, "ShipCameHomeForFood") != 1 {
		t.Fatalf("ship home: status=%s crew=%d reports=%d", status, crew, streamEvents(t, pool, ship, "ShipCameHomeForFood"))
	}
}

func streamEvents(t *testing.T, pool *pgxpool.Pool, stream uuid.UUID, kind string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type=$2`, stream, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
