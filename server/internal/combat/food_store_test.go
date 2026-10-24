package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"math"
	"os"
	"testing"
	"time"
)

// Actual migration -> actual upkeep, with the city's grain initially zero and
// all available grain in the old reserve. No alternative feeding formula.
func TestFoodStore_MigratedGrainFeedsGarrison(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "returned-food")
	seedGoods(t, pool, f.capitalID, f.tick, 0, 10000)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, q := range []string{`CREATE TABLE settlement_granary(settlement_id uuid,good_key text,amount float8)`, `CREATE TABLE processed_sitos_ticks(event_id bigint)`} {
		if _, err := tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settlement_granary VALUES($1,'grain',10)`, f.capitalID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../db/migrations/184_remove_sitos.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var uid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,home_settlement_id,type,category,size,status) VALUES($1,$2,$3,$3,$3,'spearman','land',100,'garrison') RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))), events.NewStore(pool), nil, nil, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))).Clock())
	event := events.ScheduledEvent{ID: 1840091, WorldID: f.worldID, DueTick: f.tick}
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	var size, losses int
	var grain float64
	if err := pool.QueryRow(ctx, `SELECT size,(SELECT amount FROM settlement_goods WHERE settlement_id=$2 AND good_key='grain'),(SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='UnitAttrition') FROM units WHERE id=$1`, uid, f.capitalID).Scan(&size, &grain, &losses); err != nil {
		t.Fatal(err)
	}
	want := 10 - UnitUpkeep("spearman", "land", 100, "garrison", 0).Grain
	if size != 100 || losses != 0 || math.Abs(grain-want) > 1e-9 {
		t.Fatalf("migrated grain unavailable or double charged: size%d losses%d grain%f want%f", size, losses, grain, want)
	}
}
