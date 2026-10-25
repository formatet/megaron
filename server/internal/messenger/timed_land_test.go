package messenger

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"testing"
	"time"
)

func TestTimedRunnerReturnWaitsForReachedHex(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	ctx := context.Background()
	f := setupLandFixture(t)
	a := tick.Anchor{Tick: 500, At: time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)}
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET last_tick_at=$2 WHERE id=$1`, f.worldID, a.At); err != nil {
		t.Fatal(err)
	}
	id := f.outboundMessenger(t)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(a.At.Add(44 * time.Second))
	res, err := StartReturnLeg(ctx, f.pool, events.NewScheduler(f.pool, clk), f.worldID, id, clk.Now(), 500, nil)
	if err != nil || !res.Started {
		t.Fatalf("return start %+v err%v", res, err)
	}
	if !res.ReturnsAt.Equal(clk.Now().Add(2 * time.Minute)) {
		t.Fatalf("return duration changed: %v", res.ReturnsAt)
	}
	clk.Advance(30 * time.Second)
	eyes := province.LoadLiveEyes(ctx, f.pool, f.worldID, f.ownerID, clk.Now(), a.MilliAt(clk.Now()))
	found := false
	for _, e := range eyes {
		if e.Kind == province.EyeLandUnit {
			found = true
			if e.Pos.Q != 3 || e.Pos.R != 0 {
				t.Errorf("runner sees from a hex before reaching it: %+v want(3,0)", e.Pos)
			}
		}
	}
	if !found {
		t.Fatal("runner has no land eye")
	}
}

func TestTimedRunnerSightRemembersReachedPrefixWithoutPolling(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	ctx := context.Background()
	f := setupLandFixture(t)
	now := time.Date(2026, 10, 25, 7, 0, 44, 0, time.UTC)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET last_tick_at=$2 WHERE id=$1`, f.worldID, now.Add(-44*time.Second)); err != nil {
		t.Fatal(err)
	}
	id := f.outboundMessenger(t)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = SaveDepartureTx(ctx, tx, f.worldID, id, now); err != nil {
		t.Fatal(err)
	}
	// Explicitly freeze a two-tick leg: the fixture's old sent_at is historical.
	if err = SaveLandRouteTx(ctx, tx, f.worldID, id, province.MapPosition{}, province.MapPosition{Q: 3}, now, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `DELETE FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2`, f.worldID, f.ownerID); err != nil {
		t.Fatal(err)
	}
	if err = province.SweepRunnerRoutes(ctx, f.pool, f.worldID, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	var seen bool
	if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=3 AND r=0)`, f.worldID, f.ownerID).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen {
		t.Fatal("runner remembered unreached far hex")
	}
	if err = province.SweepRunnerRoutes(ctx, f.pool, f.worldID, now.Add(120*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=3 AND r=0)`, f.worldID, f.ownerID).Scan(&seen); err != nil || !seen {
		t.Fatalf("reached far hex not remembered: %v %v", seen, err)
	}
}

func TestTimedCallBackTurnsFromReachedHex(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	f := setupLandFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
		t.Fatal(err)
	}
	id := f.outboundMessenger(t)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='awaiting_passage',passage_port_id=$2,arrives_at=$3 WHERE id=$1`, id, f.destID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = SaveLandRouteTx(ctx, tx, f.worldID, id, province.MapPosition{}, province.MapPosition{Q: 3}, now, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(50 * time.Second)
	sched := events.NewScheduler(f.pool, clock.NewTestClock(now))
	result, err := CallBack(ctx, f.pool, sched, f.worldID, id, f.ownerID, now, 500)
	if err != nil || !result.Started {
		t.Fatalf("callback %+v err%v", result, err)
	}
	var raw []byte
	if err = f.pool.QueryRow(ctx, `SELECT land_route FROM messengers WHERE id=$1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	p, active, timed := province.RunnerLandPosition(raw, nil, now)
	if !active || !timed || p.Q != 1 {
		t.Fatalf("runner turned from %v active%v timed%v, want reached hex1", p, active, timed)
	}
}

func TestTimedRunnerCannotBoardBeforePortOrCatchDepartedShip(t *testing.T) {
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	f := setupPassageFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET last_tick_at=$2 WHERE id=$1;`, f.worldID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,-1,0,'plains')`, f.worldID); err != nil {
		t.Fatal(err)
	}
	id := f.waitingMessenger(t, f.originID, 501)
	ship := f.ship(t, f.originID, "merchantman")
	tid := f.departingTransport(t, f.originID, ship, 503)
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET departs_at=$2,arrives_at=$3 WHERE id=$1`, tid, now, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = SaveLandRouteTx(ctx, tx, f.worldID, id, province.MapPosition{Q: -1}, province.MapPosition{}, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(now)
	h := NewPassageScanHandler(f.pool, events.NewScheduler(f.pool, clk), nil, clk, nil)
	c := boardedCarrier{transportID: &tid, name: "carrier"}
	for _, elapsed := range []time.Duration{0, 62 * time.Second} {
		clk.Set(now.Add(elapsed))
		if err = h.boardOne(ctx, f.worldID, id, province.MapPosition{Q: 5}, 5, 0, 503, now.Add(3*time.Minute), c); err != nil {
			t.Fatal(err)
		}
		_, passage, _, _ := f.messengerRow(t, id)
		if passage == nil || *passage != "awaiting_passage" {
			t.Fatalf("boarded ship that left before runner reached port, elapsed%v status%v", elapsed, passage)
		}
	}
}
