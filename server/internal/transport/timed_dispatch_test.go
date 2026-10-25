package transport

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestTimedFreightDispatchKeepsStartHexAndDeadline(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	ctx := context.Background()
	pool := testPool(t)
	f := newFixture(t, pool)
	a := tick.Anchor{Tick: 103, At: time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=103,last_tick_at=$2 WHERE id=$1`, f.worldID, a.At); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(a.At.Add(44 * time.Second))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id, err := Dispatch(ctx, tx, events.NewScheduler(pool, clk), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: clk.Now(), Manifest: Manifest{"grain": 10}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var start, end int
	var eta time.Time
	if err = pool.QueryRow(ctx, `SELECT journey,departed_tick,due_tick,arrives_at FROM transports WHERE id=$1`, id).Scan(&raw, &start, &end, &eta); err != nil {
		t.Fatal(err)
	}
	pos, err := SavedPosition(raw, start, end, a.ExactMilliAt(clk.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if pos.Q != 0 || pos.R != 0 {
		t.Errorf("caravan consumed pre-dispatch phase: at%v want origin", pos)
	}
	var saved map[string]any
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["clock"] == nil {
		t.Error("new freight has no movement-owned saved clock")
	}
	if end-start != 5 || !eta.Equal(clk.Now().Add(5*time.Minute)) {
		t.Errorf("duration changed: %d/%d eta%v", start, end, eta)
	}
}

// The advertised ETA and scheduler must be the same saved instant even when
// world.current_tick is already at the destination tick's start.
func TestTimedFreightDeadlineMatchesFrozenRoute(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 7, 0, 44, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=103,last_tick_at=$2 WHERE id=$1`, f.worldID, now.Add(-44*time.Second)); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id, err := Dispatch(ctx, tx, events.NewScheduler(pool, clock.NewTestClock(now)), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: now, Manifest: Manifest{"grain": 10}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var dep, due int
	var arrival, deadline time.Time
	var typ string
	if err = pool.QueryRow(ctx, `SELECT t.journey,t.departed_tick,t.due_tick,t.arrives_at,e.process_after,e.event_type FROM transports t JOIN scheduled_events e ON e.payload->>'transport_id'=t.id::text WHERE t.id=$1`, id).Scan(&raw, &dep, &due, &arrival, &deadline, &typ); err != nil {
		t.Fatal(err)
	}
	if typ != string(events.ScheduledTransportArrivalV2) || !deadline.Equal(arrival) {
		t.Fatalf("type %s deadline%v ETA%v", typ, deadline, arrival)
	}
	for _, tt := range []struct {
		elapsed time.Duration
		want    int
	}{{99 * time.Second, 0}, {100*time.Second + 20*time.Millisecond, 1}, {199 * time.Second, 1}, {200 * time.Second, 2}, {299 * time.Second, 2}, {300 * time.Second, 3}} {
		p, err := SavedPosition(raw, dep, due, 0, now.Add(tt.elapsed))
		if err != nil || p.Q != tt.want {
			t.Errorf("at %v got%v err%v wantq%d", tt.elapsed, p, err, tt.want)
		}
	}
}

// Ordered catch-up sees physical rows dispatched later than its historical
// cutoff; those rows must not be intercepted before dispatch or after arrival.
func TestTimedFreightCatchupOnlyInterceptsActiveWindow(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=103,last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
		t.Fatal(err)
	}
	var raider uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&raider); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=$2,is_capital=true WHERE id=$1`, f.destID, raider); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,stance,q,r,sentry_q,sentry_r) VALUES($1,$2,'spearman','land',80,0,'positioned','sentry',1,0,1,0)`, f.worldID, raider); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(now)
	sched := events.NewScheduler(pool, clk)
	ids := make([]uuid.UUID, 0, 3)
	for _, offset := range []time.Duration{time.Minute, -6 * time.Minute, 0} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		id, err := Dispatch(ctx, tx, sched, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: now.Add(offset), Manifest: Manifest{"silver": 10}, Interceptable: true})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	h := NewInterceptScanHandler(pool, sched, events.NewStore(pool), nil, clk)
	if err := h.ScanAt(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 103}); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		want := "in_transit"
		if i == 2 {
			want = "intercepted"
		}
		if status != want {
			t.Errorf("window%d status%s want%s", i, status, want)
		}
	}
}
