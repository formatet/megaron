package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"github.com/google/uuid"
	"math"
	"testing"
	"time"
)

func TestSitosArmy_GrainReserveFeedsWithoutDoubleCharge(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "sitos-army")
	seedGoods(t, pool, f.capitalID, f.tick, 0, 10000)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'fish',10000,34,100000,$2)`, f.capitalID, f.tick); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_granary(settlement_id,good_key,amount) VALUES($1,'grain',10),($1,'fish',20)`, f.capitalID); err != nil {
		t.Fatal(err)
	}
	var uid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,home_settlement_id,type,category,size,status) VALUES($1,$2,$3,$3,$3,'spearman','land',100,'garrison') RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Unix(0, 0))
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	// Correct priority: Sitos first, then upkeep; large fish stock must not hide
	// the army's need for grain in its own paying city's reserve.
	sitos := economy.NewSitosTickHandler(pool, sched, store, nil, economy.LoadSitosConfig())
	if err := sitos.Handle(ctx, events.ScheduledEvent{ID: 1313131, WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatal(err)
	}
	var before float64
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'`, f.capitalID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	h := NewUpkeepHandler(pool, sched, store, nil, nil)
	event := events.ScheduledEvent{ID: 1313132, WorldID: f.worldID, DueTick: f.tick}
	for n := 0; n < 2; n++ {
		if err := h.Handle(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	var size int
	var held float64
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id=$1`, uid).Scan(&size); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'`, f.capitalID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	need := UnitUpkeep("spearman", "land", 100, "garrison").Grain
	if size != 100 || math.Abs(held-(before-need)) > 1e-9 {
		t.Fatalf("army starved or reserve not conserved/charged once: size%d grain%f before%f need%f", size, held, before, need)
	}
	var source float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum((payload->>'grain_from_sitos')::float8),0) FROM events WHERE stream_id=$1 AND event_type='UpkeepSettled'`, f.capitalID).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if math.Abs(source-need) > 1e-9 {
		t.Fatalf("upkeep outcome lost its reserve source: %f want%f", source, need)
	}
	var losses int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='UnitAttrition'`, uid).Scan(&losses); err != nil {
		t.Fatal(err)
	}
	if losses != 0 {
		t.Fatalf("loss events%d despite own grain reserve", losses)
	}
}

// Controlled stored-rate scenario in actual tick priority, for BEFORE/AFTER
// comparison. No growth/decay/rites: isolate grain supply rather than mask it.
func TestSitosArmy_Soak20(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "sitos-soak")
	seedGoods(t, pool, f.capitalID, f.tick, 0, 10000)
	for _, q := range []string{
		`UPDATE settlement_goods SET rate=.25 WHERE settlement_id=$1 AND good_key='grain'`,
		`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) SELECT $1,'fish',10000,34,100000,current_world_tick()`,
		`INSERT INTO settlement_granary(settlement_id,good_key,amount) VALUES($1,'grain',10),($1,'fish',290)`,
	} {
		if _, err := pool.Exec(ctx, q, f.capitalID); err != nil {
			t.Fatal(err)
		}
	}
	var uid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,home_settlement_id,type,category,size,status) VALUES($1,$2,$3,$3,$3,'spearman','land',100,'garrison') RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Unix(0, 0))
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	sitos := economy.NewSitosTickHandler(pool, sched, store, nil, economy.LoadSitosConfig())
	upkeep := NewUpkeepHandler(pool, sched, store, nil, nil)
	food := economy.NewFoodTickHandler(pool, sched, store, nil)
	type sample struct {
		Tick, Size                           int
		CityGrain, ReserveGrain, ReserveFish float64
		Losses                               int
	}
	samples := []sample{}
	for tick := 1; tick <= 20; tick++ {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, f.tick+tick); err != nil {
			t.Fatal(err)
		}
		for n, h := range []interface {
			Handle(context.Context, events.ScheduledEvent) error
		}{sitos, upkeep, food} {
			if err := h.Handle(ctx, events.ScheduledEvent{ID: int64(13000000 + tick*3 + n), WorldID: f.worldID, DueTick: f.tick + tick}); err != nil {
				t.Fatal(err)
			}
		}
		row := sample{Tick: tick}
		if err := pool.QueryRow(ctx, `SELECT u.size,settled(g.amount,g.rate,g.calc_tick),(SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'),(SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='fish'),(SELECT count(*) FROM events WHERE stream_id=$2 AND event_type='UnitAttrition') FROM units u JOIN settlement_goods g ON g.settlement_id=$1 AND g.good_key='grain' WHERE u.id=$2`, f.capitalID, uid).Scan(&row.Size, &row.CityGrain, &row.ReserveGrain, &row.ReserveFish, &row.Losses); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, row)
	}
	raw, err := json.Marshal(samples)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("SITOS_ARMY_SOAK " + string(raw))
}

func TestSitosArmy_SeaStillNeedsPhysicalProvisions(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "sitos-sea")
	seedGoods(t, pool, f.capitalID, f.tick, 0, 10000)
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_granary(settlement_id,good_key,amount) VALUES($1,'grain',100)`, f.capitalID); err != nil {
		t.Fatal(err)
	}
	var uid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,home_settlement_id,support_settlement_id,type,category,size,crew,status,q,r,provisions) VALUES($1,$2,$3,$3,'war_galley','naval',1,50,'positioned',9,0,0) RETURNING id`, f.worldID, f.owner, f.capitalID).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0))), events.NewStore(pool), nil, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 13001231, WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatal(err)
	}
	var crew int
	var reserve float64
	if err := pool.QueryRow(ctx, `SELECT crew FROM units WHERE id=$1`, uid).Scan(&crew); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_granary WHERE settlement_id=$1 AND good_key='grain'`, f.capitalID).Scan(&reserve); err != nil {
		t.Fatal(err)
	}
	if crew >= 50 || reserve != 100 {
		t.Fatalf("distant Sitos teleported food: crew%d reserve%f", crew, reserve)
	}
}

// Exercise the actual producer and consumer in the production tick order,
// including extinction of the final garrison. No synthetic loss event.
func TestSitosArmy_ActualLossSignalsEvenLastGarrison(t *testing.T) {
	for _, size := range []int{100, 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			f := newSupportFixture(t, pool, "sitos-actual-loss")
			seedGoods(t, pool, f.capitalID, f.tick, 0, 10000)
			var uid uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,settlement_id,support_settlement_id,home_settlement_id,type,category,size,status) VALUES($1,$2,$3,$3,$3,'spearman','land',$4,'garrison') RETURNING id`, f.worldID, f.owner, f.capitalID, size).Scan(&uid); err != nil {
				t.Fatal(err)
			}
			sched := events.NewScheduler(pool, clock.NewTestClock(time.Unix(0, 0)))
			store := events.NewStore(pool)
			if events.TickPriority(events.ScheduledUpkeepTick) >= events.TickPriority(events.ScheduledKharisTick) {
				t.Fatal("upkeep must precede kharis")
			}
			if err := NewUpkeepHandler(pool, sched, store, nil, nil).Handle(ctx, events.ScheduledEvent{ID: 13009901, WorldID: f.worldID, DueTick: f.tick}); err != nil {
				t.Fatal(err)
			}
			var recorded string
			if err := pool.QueryRow(ctx, `SELECT payload->>'garrison_settlement_id' FROM events WHERE stream_id=$1 AND event_type='UnitAttrition'`, uid).Scan(&recorded); err != nil {
				t.Fatal(err)
			}
			if recorded != f.capitalID.String() {
				t.Fatalf("loss location %s want %s", recorded, f.capitalID)
			}
			if size == 1 {
				var status string
				if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, uid).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if status != "disbanded" {
					t.Fatalf("last garrison status %s", status)
				}
				// The outcome must survive later cleanup/location changes as well.
				if _, err := pool.Exec(ctx, `UPDATE units SET settlement_id=NULL WHERE id=$1`, uid); err != nil {
					t.Fatal(err)
				}
			}
			if err := kharis.NewTickHandler(pool, sched, store, nil).Handle(ctx, events.ScheduledEvent{ID: 13009902, WorldID: f.worldID, DueTick: f.tick}); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='StarvationDamage' AND world_tick=current_world_tick()`, f.capitalID).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("actual current-tick attrition must signal hunger, got %d", n)
			}
		})
	}
}
