package loyalty

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

// Drive actual stock debits before welfare, including an exactly exhausted meal.
func TestWelfareFoodTick_FishAndReserves(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		grain, fish, livestock, grainRate, fishRate float64
		starving                                    bool
	}{
		{"fish covers consumption", 0, 5, 0, 0, 5, false},
		{"grain and fish insufficient", 1, 2, 0, 1, 2, true},
		{"grain reserve covers production deficit", 5, 0, 0, 0, 0, false},
		{"fish reserve covers production deficit", 0, 5, 0, 0, 0, false},
		{"livestock fallback follows FoodTick", 0, 0, 2, 0, 0, false},
		{"no food", 0, 0, 0, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := loyaltyTestPool(t)
			ctx := context.Background()

			if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
				t.Fatalf("archive leftover active test worlds: %v", err)
			}
			var worldID uuid.UUID
			const worldTick = 500
			if err := pool.QueryRow(ctx,
				`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
				"welfare-idem-"+uuid.NewString(), worldTick,
			).Scan(&worldID); err != nil {
				t.Fatalf("create world: %v", err)
			}
			t.Cleanup(func() {
				_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
			})

			var owner uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
				"welfare-idem-"+uuid.NewString(),
			).Scan(&owner); err != nil {
				t.Fatalf("create player: %v", err)
			}
			if _, err := pool.Exec(ctx,
				`INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate, kharis_cap, kharis_calc_tick)
		 VALUES ($1, $2, 0, 0, 100, 0)`,
				owner, worldID,
			); err != nil {
				t.Fatalf("create player_world_record: %v", err)
			}

			var prov, settlementID uuid.UUID
			if err := pool.QueryRow(ctx,
				`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
				worldID,
			).Scan(&prov); err != nil {
				t.Fatalf("create province: %v", err)
			}
			if err := pool.QueryRow(ctx,
				`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, loyalty, loyalty_points)
		 VALUES ($1, $2, 'WelfareTown', 'achaean', $3, 'capital', true, 'active', 1000, 3, 37) RETURNING id`,
				worldID, prov, owner,
			).Scan(&settlementID); err != nil {
				t.Fatalf("create settlement: %v", err)
			}

			for _, good := range []struct {
				key          string
				amount, rate float64
			}{
				{"grain", tc.grain, tc.grainRate}, {"fish", tc.fish, tc.fishRate}, {"livestock", tc.livestock, 0},
			} {
				if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods (settlement_id,good_key,amount,rate,cap,calc_tick) VALUES ($1,$2,$3,$4,1000000,$5)`, settlementID, good.key, good.amount, good.rate, worldTick); err != nil {
					t.Fatal(err)
				}
			}
			sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
			store := events.NewStore(pool)
			evt := events.ScheduledEvent{ID: 987300, WorldID: worldID, DueTick: worldTick}
			food := economy.NewFoodTickHandler(pool, sched, store, nil)
			if err := food.Handle(ctx, evt); err != nil {
				t.Fatal(err)
			}
			var unmet float64
			if err := pool.QueryRow(ctx, `SELECT food_unmet_amount FROM settlements WHERE id=$1`, settlementID).Scan(&unmet); err != nil {
				t.Fatal(err)
			}
			if (unmet > 0) != tc.starving {
				t.Fatalf("FoodTick unmet=%v expected starving=%v", unmet, tc.starving)
			}
			welfare := NewWelfareHandler(pool, sched, store)
			if err := welfare.Handle(ctx, evt); err != nil {
				t.Fatal(err)
			}
			var kind, reason string
			var points float64
			if err := pool.QueryRow(ctx, `SELECT event_type,reason FROM loyalty_events WHERE settlement_id=$1 ORDER BY created_at DESC LIMIT 1`, settlementID).Scan(&kind, &reason); err != nil {
				t.Fatal(err)
			}
			want := "well_fed"
			if tc.starving {
				want = "starving"
			}
			if kind != want {
				t.Fatalf("FoodTick unmet=%v but welfare=%s (%s), want %s", unmet, kind, reason, want)
			}
			if strings.Contains(reason, "grain store") || strings.Contains(reason, "grain production") {
				t.Fatal(reason)
			}
			if err := pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id=$1`, settlementID).Scan(&points); err != nil {
				t.Fatal(err)
			}
			if err := welfare.Handle(ctx, evt); err != nil {
				t.Fatal(err)
			}
			var replay float64
			_ = pool.QueryRow(ctx, `SELECT loyalty_points FROM settlements WHERE id=$1`, settlementID).Scan(&replay)
			if replay != points {
				t.Fatalf("replay moved loyalty: %v -> %v", points, replay)
			}
		})
	}
}
