package messenger

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

func speedFixture(t *testing.T) (*passageFixture, uuid.UUID, time.Time) {
	t.Helper()
	old := tick.TickSeconds
	tick.TickSeconds = 60
	t.Cleanup(func() { tick.TickSeconds = old })
	ctx := context.Background()
	f := setupLandFixture(t)
	now := time.Date(2026, 10, 25, 7, 0, 0, 0, time.UTC)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
		t.Fatal(err)
	}
	id := f.outboundMessenger(t)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET arrives_at=$2 WHERE id=$1`, id, now.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return f, id, now
}

// A Runner five ticks from home: ceil(0.75*5)=4 left; the hex it stands in is the same; the one
// pending arrival job moves in place; a second prayer does nothing.
func TestSpeedUpMessenger_RemainingTimeIdempotentAndInPlace(t *testing.T) {
	ctx := context.Background()
	f, id, now := speedFixture(t)
	sched := events.NewScheduler(f.pool, clock.NewTestClock(now))
	tx, _ := f.pool.Begin(ctx)
	if err := SaveLandRouteTx(ctx, tx, f.worldID, id, province.MapPosition{}, province.MapPosition{Q: 3}, now, now.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := EnqueueLandTx(ctx, tx, sched, f.worldID, events.ScheduledMessengerArrival, ArrivalPayload{MessengerID: id}, 508); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	at := now.Add(3*time.Minute + 10*time.Second)
	var raw []byte
	_ = f.pool.QueryRow(ctx, `SELECT land_route FROM messengers WHERE id=$1`, id).Scan(&raw)
	before, _, _ := province.RunnerLandPosition(raw, nil, at)

	tx, _ = f.pool.Begin(ctx)
	got, err := SpeedUpMessenger(ctx, tx, id, uuid.New(), at)
	if err != nil || got == nil {
		t.Fatalf("not sped: %v %v", got, err)
	}
	_ = tx.Commit(ctx)
	if got.OldArriveTick != 508 || got.NewArriveTick != 503+4 {
		t.Fatalf("arrival %d -> %d, want 508 -> 507", got.OldArriveTick, got.NewArriveTick)
	}
	var raw2 []byte
	var arr time.Time
	_ = f.pool.QueryRow(ctx, `SELECT land_route,arrives_at FROM messengers WHERE id=$1`, id).Scan(&raw2, &arr)
	if after, _, _ := province.RunnerLandPosition(raw2, nil, at); after != before {
		t.Fatalf("position moved %v -> %v", before, after)
	}
	var n, due int
	var pa time.Time
	if err := f.pool.QueryRow(ctx, `SELECT count(*),min(due_tick),min(process_after) FROM scheduled_events WHERE payload->>'messenger_id'=$1 AND processed_at IS NULL`, id.String()).Scan(&n, &due, &pa); err != nil {
		t.Fatal(err)
	}
	if n != 1 || due != 507 || !pa.Equal(arr) {
		t.Fatalf("jobs %d due %d at %v eta %v", n, due, pa, arr)
	}
	tx, _ = f.pool.Begin(ctx)
	again, err := SpeedUpMessenger(ctx, tx, id, uuid.New(), at.Add(time.Second))
	_ = tx.Commit(ctx)
	if err != nil || again != nil {
		t.Fatalf("sped twice: %v %v", again, err)
	}
}

// A Runner aboard a carrier follows its ship: it is not sped up on its own.
func TestSpeedUpMessenger_AboardACarrierIsLeftAlone(t *testing.T) {
	ctx := context.Background()
	f, id, now := speedFixture(t)
	tx, _ := f.pool.Begin(ctx)
	if err := SaveLandRouteTx(ctx, tx, f.worldID, id, province.MapPosition{}, province.MapPosition{Q: 3}, now, now.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	tx, _ = f.pool.Begin(ctx)
	got, err := SpeedUpMessenger(ctx, tx, id, uuid.New(), now.Add(2*time.Minute))
	_ = tx.Commit(ctx)
	if err != nil || got != nil {
		t.Fatalf("an aboard Runner was sped alone: %v %v", got, err)
	}
}

// A Runner sent while a speed prayer holds over its city takes three quarters of the time;
// the prayer over another city does nothing.
func TestSaveDeparture_SpeedPrayerOverTheCity(t *testing.T) {
	ctx := context.Background()
	f, id, now := speedFixture(t)
	lay := func(city uuid.UUID) {
		if _, err := f.pool.Exec(ctx, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,settlement_id,from_tick,until_tick) VALUES($1,$2,'blessed_speed_land','city',$3,490,520)`, f.worldID, f.ownerID, city); err != nil {
			t.Fatal(err)
		}
	}
	end := func() int {
		tx, _ := f.pool.Begin(ctx)
		if err := SaveDepartureTx(ctx, tx, f.worldID, id, now); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(ctx)
		var raw []byte
		_ = f.pool.QueryRow(ctx, `SELECT land_route FROM messengers WHERE id=$1`, id).Scan(&raw)
		return routeEnd(t, raw)
	}
	lay(f.destID)
	if e := end(); e != 508 {
		t.Fatalf("a prayer over another city sped the Runner: end %d", e)
	}
	lay(f.originID)
	if e := end(); e != 506 {
		t.Fatalf("end %d, want 500+ceil(0.75*8)=506", e)
	}
}

func routeEnd(t *testing.T, raw []byte) int {
	t.Helper()
	var r struct {
		EndTick int `json:"end_tick"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r.EndTick
}
