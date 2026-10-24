package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"testing"
	"time"
)

// The old trigger waits for crew loss. A full crew with just enough food to
// return must instead leave now, before the next stationary day eats its way home.
func TestReturnFoodLeavesBeforeCrewStarves(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newStarvationFixture(t, pool, "food-return")
	seedGoods(t, pool, f.capitalID, 100, 100000, 100000)
	ship := mkStarvingShip(t, pool, f.worldID, f.ownerID, f.capitalID, f.capitalID)
	if _, err := pool.Exec(ctx, `UPDATE units SET provisions=0.6 WHERE id=$1`, ship); err != nil {
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
}
