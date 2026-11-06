package transport

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

// A caravan under way: T ticks left become ceil(0.75*T); the hex it stands in is the same
// before and after; its one pending delivery job moves in place; a second run does nothing.
func TestSpeedUpTransport_RemainingTimeIdempotentAndInPlace(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	ctx := context.Background()
	pool := testPool(t)
	f := newFixture(t, pool)
	start := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=103,last_tick_at=$2 WHERE id=$1`, f.worldID, start); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(start.Add(10 * time.Second))
	tx, _ := pool.Begin(ctx)
	defer tx.Rollback(ctx)
	id, err := Dispatch(ctx, tx, events.NewScheduler(pool, clk), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: clk.Now(), Manifest: Manifest{"grain": 10}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var dep, due int
	if err = pool.QueryRow(ctx, `SELECT journey,departed_tick,due_tick FROM transports WHERE id=$1`, id).Scan(&raw, &dep, &due); err != nil {
		t.Fatal(err)
	}
	now := start.Add(10*time.Second + 35*time.Second) // inside the first tick, so a second run could still gain a tick
	before, err := SavedPosition(raw, dep, due, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	var j0 province.TradeJourney
	_ = json.Unmarshal(raw, &j0)
	nowTick := int(j0.Route(0, 0).MilliAt(now) / 1000)
	left := due - nowTick
	want := nowTick + (3*left+3)/4

	bless := uuid.New()
	tx, _ = pool.Begin(ctx)
	defer tx.Rollback(ctx)
	got, err := SpeedUpTransport(ctx, tx, id, bless, now)
	if err != nil || got == nil {
		t.Fatalf("not sped: %v %v", got, err)
	}
	if got.OldArriveTick != due || got.NewArriveTick != want || want >= due {
		t.Fatalf("old %d new %d want %d (was due %d)", got.OldArriveTick, got.NewArriveTick, want, due)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var raw2 []byte
	var due2 int
	var arr time.Time
	if err = pool.QueryRow(ctx, `SELECT journey,due_tick,arrives_at FROM transports WHERE id=$1`, id).Scan(&raw2, &due2, &arr); err != nil {
		t.Fatal(err)
	}
	after, err := SavedPosition(raw2, dep, due2, 0, now)
	if err != nil || after != before {
		t.Fatalf("position moved %v -> %v (%v)", before, after, err)
	}
	var n, evDue int
	var evAt time.Time
	if err = pool.QueryRow(ctx, `SELECT count(*),min(due_tick),min(process_after) FROM scheduled_events WHERE payload->>'transport_id'=$1 AND processed_at IS NULL`, id.String()).Scan(&n, &evDue, &evAt); err != nil {
		t.Fatal(err)
	}
	if due2 != want || n != 1 || evDue != want || !evAt.Equal(arr) {
		t.Fatalf("due %d jobs %d jobDue %d jobAt %v eta %v", due2, n, evDue, evAt, arr)
	}
	// Idempotent: a marked journey is left alone, bit for bit; a second prayer does not stack.
	tx, _ = pool.Begin(ctx)
	defer tx.Rollback(ctx)
	again, err := SpeedUpTransport(ctx, tx, id, uuid.New(), now.Add(time.Second))
	if err != nil || again != nil {
		t.Fatalf("second run sped again: %v %v", again, err)
	}
	_ = tx.Commit(ctx)
	var raw3 []byte
	_ = pool.QueryRow(ctx, `SELECT journey FROM transports WHERE id=$1`, id).Scan(&raw3)
	if string(raw3) != string(raw2) {
		t.Fatal("journey changed on the second run")
	}
}

// A journey that left under a departure prayer (factor 0.75) is never sped again.
func TestSpeedUpTransport_DepartureSpedIsLeftAlone(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	f := newFixture(t, pool)
	tx, _ := pool.Begin(ctx)
	defer tx.Rollback(ctx)
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure')`, f.sourceID, f.owner); err != nil {
		t.Fatal(err)
	}
	id, err := Dispatch(ctx, tx, events.NewScheduler(pool, clock.NewTestClock(time.Now())), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: time.Now(), Manifest: Manifest{"grain": 1}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := SpeedUpTransport(ctx, tx, id, uuid.New(), time.Now())
	if err != nil || got != nil {
		t.Fatalf("a departure-sped caravan was sped again: %v %v", got, err)
	}
}

// A live speed prayer over the departing city shortens the whole journey; it does not
// stack on a pending departure prayer; after until_tick the time is normal.
func TestBlessDeparture_SpeedPrayerHoldsOnlyWhileItLasts(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	f := newFixture(t, pool)
	var tickNow int
	if err := pool.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, f.worldID).Scan(&tickNow); err != nil {
		t.Fatal(err)
	}
	lay := func(scope string, city *uuid.UUID, from, until int) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,settlement_id,from_tick,until_tick) VALUES($1,$2,'blessed_speed_land',$3,$4,$5,$6)`, f.worldID, f.owner, scope, city, from, until); err != nil {
			t.Fatal(err)
		}
	}
	plain := func() province.TradeJourney {
		return province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0}, {Q: 1}, {Q: 2}, {Q: 3}, {Q: 4}}, StepCosts: []int64{1000, 1000, 1000, 1000}, Distance: 4, TravelTicks: 6}
	}
	run := func(city uuid.UUID) province.TradeJourney {
		t.Helper()
		tx, _ := pool.Begin(ctx)
		defer tx.Rollback(ctx)
		defer tx.Rollback(ctx)
		j := plain()
		if err := BlessDeparture(ctx, tx, f.worldID, f.owner, city, &j, true); err != nil {
			t.Fatal(err)
		}
		return j
	}
	if j := run(f.sourceID); j.TravelTicks != 6 || j.SpeedBlessing != "" {
		t.Fatalf("no prayer, yet sped: %+v", j)
	}
	lay("city", &f.sourceID, tickNow, tickNow+10)
	if j := run(f.sourceID); j.TravelTicks != 5 || j.SpeedBlessing == "" {
		t.Fatalf("a prayer over the city must speed the departure (x0.75 of 6 = 5): %+v", j)
	}
	// Already sped (by this or another prayer): a second pass takes nothing more off.
	{
		tx, _ := pool.Begin(ctx)
		defer tx.Rollback(ctx)
		j := plain()
		for i := 0; i < 2; i++ {
			j.DepartureChecked = false
			if err := BlessDeparture(ctx, tx, f.worldID, f.owner, f.sourceID, &j, true); err != nil {
				t.Fatal(err)
			}
		}
		if j.TravelTicks != 5 {
			t.Fatalf("a journey already sped was sped again: %d ticks", j.TravelTicks)
		}
		_ = tx.Rollback(ctx) // release the city row before the next departure
	}
	if j := run(f.destID); j.TravelTicks != 6 {
		t.Fatalf("another city is not covered: %+v", j)
	}
	if _, err := pool.Exec(ctx, `UPDATE prayer_blessings SET until_tick=$1`, tickNow); err != nil {
		t.Fatal(err)
	}
	if j := run(f.sourceID); j.TravelTicks != 6 {
		t.Fatalf("after until_tick the time is normal: %+v", j)
	}
}
