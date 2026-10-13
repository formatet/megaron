package combat

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestSpearmanZeroSilverStillEatsAndNeverDeserts(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "spear-subsistence")
	seedGoods(t, pool, f.capitalID, f.tick, 100, 0)
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units (world_id,owner_id,type,category,size,status,settlement_id,support_settlement_id,unpaid_periods)
	 VALUES($1,$2,'spearman','land',100,'garrison',$3,$3,71) RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))), events.NewStore(pool), &fakeBroadcaster{}, nil)
	for i := 0; i < 80; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: int64(970000 + i), WorldID: f.worldID, DueTick: f.tick + i}); err != nil {
			t.Fatal(err)
		}
	}
	var size, unpaid int
	if err := pool.QueryRow(ctx, `SELECT size,unpaid_periods FROM units WHERE id=$1`, id).Scan(&size, &unpaid); err != nil {
		t.Fatal(err)
	}
	if size != 100 || unpaid != 0 {
		t.Fatalf("ration-only spearman size/unpaid=%d/%d, want100/0", size, unpaid)
	}
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 60 {
		t.Fatalf("grain=%v, want60 (80 ticks of .5 ration)", got)
	}
	if got := settledAmount(t, pool, f.capitalID, "silver"); got != 0 {
		t.Fatalf("silver=%v, want0", got)
	}
	if n := countEvents(t, pool, f.worldID, "UnitDeserted") + countEvents(t, pool, f.worldID, "UpkeepUnpaid"); n != 0 {
		t.Fatalf("silver-shortage events=%d, want0", n)
	}
}

func TestSpearmanFieldRationAndElitePayRemain(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "spear-field-elite")
	seedGoods(t, pool, f.capitalID, f.tick, 100, 10)
	if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,support_settlement_id)
	 VALUES($1,$2,'spearman','land',100,'positioned',$3),($1,$2,'elite_infantry','land',100,'positioned',$3)`, f.worldID, f.owner, f.capitalID); err != nil {
		t.Fatal(err)
	}
	runUpkeep(t, pool, f)
	if got := settledAmount(t, pool, f.capitalID, "grain"); got != 97.8 {
		t.Fatalf("field grain=%v, want97.8 (1 spear+1.2 elite)", got)
	}
	if got := settledAmount(t, pool, f.capitalID, "silver"); got != 8 {
		t.Fatalf("elite pay leaves=%v, want8 (10-2)", got)
	}
}
