package handlers

// The last open scenario of Utfodringsordningen (megaron_plan_utfodringsordningen.md
// §7.6, kanon Timothy 2026-08-25): "ALLT SOM STADEN FÖRSÖRJER ÄTER FÖRE
// BEFOLKNINGEN." A city whose grain income covers its garrison's own upkeep
// but falls far short of feeding its population as well must, over many days,
// lose population to starvation while its garrison stays completely intact.
//
// This is the one integration this scenario needs that no existing test
// provides: internal/combat/food_ordering_test.go proves the Plikt(50)/
// Föda(55) ordering for ONE day (garrison fed in full, population takes the
// shortfall) but never runs Tillväxt(60) — so it never proves population
// actually SHRINKS. internal/kharis/grain_growth_test.go proves population
// shrinks under starvation but its fixtures never garrison a unit — so it
// never proves the garrison survives that shrinkage. Combining all three
// steps in one test is only possible from a package that can see combat,
// economy AND kharis at once: kharis may not import combat, and combat may
// not import kharis (CLAUDE.md G1 — package dependency order). api/handlers
// already imports all three in production code (province.go, db.go, web.go),
// which is why this lives here rather than being invented as a new rig.
//
// Execution order for each simulated day is looked up dynamically via
// events.TickPriority — NOT hardcoded as "Upkeep, then Food, then Kharis" —
// specifically so that a future change to internal/events/priority.go's
// tickPriorityFood/tickPriorityUpkeep constants is something this test can
// actually catch (see the mutation-grind note in the session report).

import (
	"context"
	"sort"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"github.com/google/uuid"
)

func TestFamine_GarrisonSurvivesWhilePopulationStarves(t *testing.T) {
	pool := armyDisplayTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	const startTick = 5000
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-famine-"+uuid.New().String(), startTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"famine-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	const startPop = 5000
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Limenaria', 'achaean', $3, 'capital', true, 'active', $4) RETURNING id`,
		worldID, provinceID, ownerID, startPop,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// The scenario's arithmetic, derived from the REAL constants rather than
	// guessed: a garrison of 100 spearmen needs combat.UnitUpkeep's grain per
	// day; the population of startPop needs economy.GrainConsumptionPerTick
	// per day. grainRate is set to comfortably clear the garrison's tiny need
	// (Plikt, priority 50, draws first and requires the FULL amount or the
	// unit attrits) while sitting far below what the population alone needs
	// (Föda, priority 55, draws whatever remains) — so every single day the
	// garrison eats in full and the population goes hungry, without the
	// stock ever fully drying up under the garrison's feet.
	armyGrainNeed := combat.UnitUpkeep("spearman", "land", 100, "garrison").Grain
	popDemand := economy.GrainConsumptionPerTick(startPop)
	grainRate := armyGrainNeed * 3
	if popDemand < grainRate*10 {
		t.Fatalf("fixture sanity: population demand %.4f is not far above garrison need %.4f (rate %.4f) — "+
			"the scenario needs a population starved every day while the garrison is always fed; adjust startPop",
			popDemand, armyGrainNeed, grainRate)
	}
	t.Logf("armyGrainNeed=%.4f popDemand=%.4f grainRate=%.4f", armyGrainNeed, popDemand, grainRate)

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES
		   ($1, 'grain',  0,      $2, 1000000, $3),
		   ($1, 'silver', 100000, 0,  1000000, $3)`,
		settlementID, grainRate, startTick,
	); err != nil {
		t.Fatalf("seed goods: %v", err)
	}

	var garrisonID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, settlement_id, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', 0, 0, $3, $3, 0)
		 RETURNING id`,
		worldID, ownerID, settlementID,
	).Scan(&garrisonID); err != nil {
		t.Fatalf("create garrison unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(pool, clk)
	store := events.NewStore(pool)
	upkeepH := combat.NewUpkeepHandler(pool, sched, store, nil, nil)
	foodH := economy.NewFoodTickHandler(pool, sched, store, nil)
	kharisH := kharis.NewTickHandler(pool, sched, store, nil)

	garrisonSize := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, garrisonID).Scan(&n); err != nil {
			t.Fatalf("read garrison size: %v", err)
		}
		return n
	}
	population := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id = $1`, settlementID).Scan(&n); err != nil {
			t.Fatalf("read population: %v", err)
		}
		return n
	}
	foodUnmet := func() float64 {
		t.Helper()
		var v float64
		if err := pool.QueryRow(ctx, `SELECT food_unmet_amount FROM settlements WHERE id = $1`, settlementID).Scan(&v); err != nil {
			t.Fatalf("read food_unmet_amount: %v", err)
		}
		return v
	}

	// Day order is looked up from events.TickPriority — not hardcoded — so a
	// swap of tickPriorityFood/tickPriorityUpkeep in internal/events/priority.go
	// actually changes what this loop does (the mutation grind pins this).
	type step struct {
		priority int
		run      func(ctx context.Context, tick int, eventID int64) error
	}
	steps := []step{
		{events.TickPriority(events.ScheduledUpkeepTick), func(ctx context.Context, tick int, id int64) error {
			return upkeepH.Handle(ctx, events.ScheduledEvent{ID: id, WorldID: worldID, DueTick: tick})
		}},
		{events.TickPriority(events.ScheduledFoodTick), func(ctx context.Context, tick int, id int64) error {
			return foodH.Handle(ctx, events.ScheduledEvent{ID: id, WorldID: worldID, DueTick: tick})
		}},
		{events.TickPriority(events.ScheduledKharisTick), func(ctx context.Context, tick int, id int64) error {
			return kharisH.Handle(ctx, events.ScheduledEvent{ID: id, WorldID: worldID, DueTick: tick})
		}},
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].priority < steps[j].priority })

	var nextEventID int64 = 1
	const days = 20
	sawUnmet := false
	for day := 1; day <= days; day++ {
		var tick int
		if err := pool.QueryRow(ctx,
			`UPDATE worlds SET current_tick = current_tick + 1 WHERE id = $1 RETURNING current_tick`,
			worldID,
		).Scan(&tick); err != nil {
			t.Fatalf("day %d: advance tick: %v", day, err)
		}

		for _, s := range steps {
			nextEventID++
			if err := s.run(ctx, tick, nextEventID); err != nil {
				t.Fatalf("day %d: handler at priority %d failed: %v", day, s.priority, err)
			}
		}

		size := garrisonSize()
		pop := population()
		unmet := foodUnmet()
		t.Logf("day %d (tick %d): garrison=%d population=%d food_unmet=%.4f", day, tick, size, pop, unmet)

		if size != 100 {
			t.Fatalf("day %d: garrison size = %d, want 100 — the population's hunger must never reach back and cost the army men", day, size)
		}
		if unmet > 0 {
			sawUnmet = true
		}
	}

	if !sawUnmet {
		t.Error("food_unmet_amount was never > 0 over the run — the fixture never actually starved the population, this test proves nothing")
	}
	if finalUnmet := foodUnmet(); finalUnmet <= 0 {
		t.Errorf("final food_unmet_amount = %.4f, want > 0", finalUnmet)
	}
	if finalPop := population(); finalPop >= startPop {
		t.Errorf("population after %d days of famine = %d, want < %d (start) — starvation must actually shrink the city", days, finalPop, startPop)
	}
	if size := garrisonSize(); size != 100 {
		t.Errorf("garrison size after %d days = %d, want 100 (intact) — kanon 2026-08-25: the garrison eats before the population and must never pay for its hunger", days, size)
	}
}
