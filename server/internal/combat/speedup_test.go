package combat

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

func marchingShip(t *testing.T, f expeditionFixture, start, end int, sped string) (movement.StoredRoute, time.Time) {
	t.Helper()
	ctx, pool := context.Background(), testPool(t)
	at := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	hexes := [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}}
	r := movement.StoredRoute{DepartureAt: at, TickNanoseconds: int64(60 * time.Second), Version: 2, StartTick: start, EndTick: end, Hexes: hexes, Costs: []int64{1000, 1000, 1000, 1000, 1000}, Sped: sped}
	raw, _ := json.Marshal(r)
	if _, err := pool.Exec(ctx, `UPDATE units SET type='galley',category='naval',size=1,crew=100,status='marching',depart_tick=$2,arrive_tick=$3,march_route=$4,target_q=5,target_r=0,q=0,r=0 WHERE id=$1`, f.unitID, start, end, raw); err != nil {
		t.Fatal(err)
	}
	return r, at
}

// A ship ten ticks out with eight to go: arrival tick -> now + ceil(0.75*left); the hex it stands
// in is the same; a new UnitArrivalV2 carries the new route key (the old one no longer matches);
// a second prayer does nothing.
func TestSpeedUpMarchingUnit_RemainingTimeNewArrivalAndIdempotent(t *testing.T) {
	f := newExpeditionFixture(t, -2, 25, nil)
	ctx, pool := context.Background(), testPool(t)
	r, at := marchingShip(t, f, 100, 110, "")
	now := at.Add(2*time.Minute + 20*time.Second) // tick 102, 8 left
	before, _ := movement.PositionAt(r.Move(), movement.Milli(r.MilliAt(now)))
	var oldRaw []byte
	_ = pool.QueryRow(ctx, `SELECT march_route FROM units WHERE id=$1`, f.unitID).Scan(&oldRaw)

	tx, _ := pool.Begin(ctx)

	defer tx.Rollback(ctx)
	got, err := SpeedUpMarchingUnit(ctx, tx, f.scheduler, f.worldID, f.unitID, uuid.New(), now)
	if err != nil || got == nil {
		t.Fatalf("not sped: %v %v", got, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got.OldArriveTick != 110 || got.NewArriveTick != 102+6 {
		t.Fatalf("arrival %d -> %d, want 110 -> 108", got.OldArriveTick, got.NewArriveTick)
	}
	var raw []byte
	var arrive int
	if err = pool.QueryRow(ctx, `SELECT march_route,arrive_tick FROM units WHERE id=$1`, f.unitID).Scan(&raw, &arrive); err != nil {
		t.Fatal(err)
	}
	var nr movement.StoredRoute
	_ = json.Unmarshal(raw, &nr)
	after, _ := movement.PositionAt(nr.Move(), movement.Milli(nr.MilliAt(now)))
	if arrive != 108 || nr.EndTick != 108 || after != before {
		t.Fatalf("arrive %d end %d position %v -> %v", arrive, nr.EndTick, before, after)
	}
	key := fmt.Sprintf("%x", sha256.Sum256(raw))
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE event_type='UnitArrivalV2' AND payload->>'unit_id'=$1 AND payload->>'route_key'=$2 AND due_tick=108 AND processed_at IS NULL`, f.unitID.String(), key).Scan(&n); err != nil || n != 1 {
		t.Fatalf("new arrival job: %d %v", n, err)
	}
	// The old arrival job (old route key, old tick) runs and does nothing: the ship is still at sea.
	oldTick := 110
	body, _ := json.Marshal(arrivalV2Payload{ScheduledUnitArrivalPayload: unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.worldID, ArriveTick: &oldTick}, RouteKey: fmt.Sprintf("%x", sha256.Sum256(oldRaw))})
	if _, err = pool.Exec(ctx, `UPDATE worlds SET current_tick=110 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if err = f.h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, EventType: events.ScheduledUnitArrivalV2, DueTick: 110, Payload: body}); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.unitID).Scan(&status)
	if status != "marching" {
		t.Fatalf("the old arrival job moved the ship: %s", status)
	}
	tx, _ = pool.Begin(ctx)
	defer tx.Rollback(ctx)
	again, err := SpeedUpMarchingUnit(ctx, tx, f.scheduler, f.worldID, f.unitID, uuid.New(), now.Add(time.Minute))
	_ = tx.Commit(ctx)
	if err != nil || again != nil {
		t.Fatalf("sped twice: %v %v", again, err)
	}
}

// A ship whose voyage already carries the mark is left bit for bit.
func TestSpeedUpMarchingUnit_MarkedVoyageIsLeftAlone(t *testing.T) {
	f := newExpeditionFixture(t, -2, 25, nil)
	ctx, pool := context.Background(), testPool(t)
	_, at := marchingShip(t, f, 100, 110, uuid.NewString())
	var before []byte
	_ = pool.QueryRow(ctx, `SELECT march_route FROM units WHERE id=$1`, f.unitID).Scan(&before)
	tx, _ := pool.Begin(ctx)
	defer tx.Rollback(ctx)
	got, err := SpeedUpMarchingUnit(ctx, tx, f.scheduler, f.worldID, f.unitID, uuid.New(), at.Add(2*time.Minute))
	_ = tx.Commit(ctx)
	if err != nil || got != nil {
		t.Fatalf("marked voyage sped: %v %v", got, err)
	}
	var after []byte
	_ = pool.QueryRow(ctx, `SELECT march_route FROM units WHERE id=$1`, f.unitID).Scan(&after)
	if string(before) != string(after) {
		t.Fatal("route changed")
	}
}
