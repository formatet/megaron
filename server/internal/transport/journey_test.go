package transport

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
)

func TestSavedPositionUsesEnteringCostsWithoutTerrain(t *testing.T) {
	j := province.TradeJourney{Category: "land", Path: []province.MapPosition{{}, {Q: 1}, {Q: 2}}, StepCosts: []int64{2500, 750}, TravelTicks: 5, Distance: 2}
	raw, _ := json.Marshal(j)
	for _, test := range []struct {
		at   int64
		want int
	}{{9999, 0}, {10000, 1}, {13000, 1}, {13846, 2}, {15000, 2}} {
		pos, err := SavedPosition(raw, 10, 15, test.at)
		if err != nil || pos.Q != test.want {
			t.Fatalf("at%d got%+v err%v wantQ%d", test.at, pos, err, test.want)
		}
	}
	if _, err := SavedPosition(raw, 10, 14, 10000); err == nil {
		t.Fatal("wrong tick duration accepted")
	}
	if _, err := SavedPosition([]byte(`{"path":[]}`), 10, 15, 10000); err == nil {
		t.Fatal("corrupt saved route accepted")
	}
}

func TestDispatchPersistsJourneyAndRejectsNoPathAtomically(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	id, err := Dispatch(ctx, tx, sched, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: clk.Now(), Manifest: Manifest{"grain": 10}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var departed, due int
	if err := pool.QueryRow(ctx, `SELECT journey,departed_tick,due_tick FROM transports WHERE id=$1`, id).Scan(&raw, &departed, &due); err != nil {
		t.Fatal(err)
	}
	var j province.TradeJourney
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Path) != 4 || len(j.StepCosts) != 3 || due-departed != 3 {
		t.Fatalf("saved %+v ticks %d/%d", j, departed, due)
	}
	// Mutate terrain after dispatch: frozen position does not change or query it.
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := SavedPosition(raw, departed, due, int64(departed)*1000+1); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Dispatch(ctx, tx, sched, DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "land", DestQ: 3, DepartsAt: clk.Now(), Manifest: Manifest{"grain": 10}})
	if !errors.Is(err, province.ErrNoTradePath) {
		t.Fatalf("disconnected dispatch %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rejected dispatch mutated movers %d %v", count, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='TransportArrival'`, f.worldID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rejected dispatch mutated jobs %d %v", count, err)
	}
}

func TestDispatchNavalRejectsNoCarrier(t *testing.T) {
	pool := testPool(t)
	f := newNavalFixture(t, pool)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = Dispatch(ctx, tx, events.NewScheduler(pool, clk), DispatchParams{WorldID: f.worldID, OwnerID: f.owner, OriginID: f.sourceID, DestID: f.destID, Kind: "transfer", Category: "naval", DestQ: 3, DepartsAt: clk.Now()})
	if err == nil {
		t.Fatal("naval dispatch accepted without real ship")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("shipless dispatch left rows %d %v", count, err)
	}
}
