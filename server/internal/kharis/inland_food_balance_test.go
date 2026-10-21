package kharis

import (
	"context"
	"encoding/json"
	"math"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Read-only live catchments, translated to origin; the coast/plains controls
// are synthetic. Fixture SQL controls starting state; these are NOT live play.
func balanceTerrain(kind string, c hexgrid.Coord) string {
	if kind == "hills" {
		if c.Q > 0 || c.Q == 0 && c.R > 0 {
			return "hills"
		}
		if c.R > 0 {
			if c.R == 2 {
				return "forest_cedar"
			}
			return "forest_olive_grove"
		}
		return "mountain_limestone"
	}
	if kind == "amnissos" {
		if c.Q == 0 && c.R < 0 || c.Q == 1 && c.R == -1 {
			return "river"
		}
		if c.Q == 2 && c.R == -1 {
			return "river_ford"
		}
		if c.Q == -2 && c.R > 0 || c.Q == 0 && c.R == 2 {
			return "plains"
		}
		if c.Q == -2 && c.R == 0 || c.Q == -1 && c.R > 0 || c.Q == 0 && c.R == 1 {
			return "hills"
		}
		if c.Q == 1 && c.R == 1 {
			return "scrub_maquis"
		}
		return "river_valley"
	}
	if kind == "coast" && c.Q > 0 {
		return "coastal_sea"
	}
	if c.Q < 0 && c.R > 0 {
		return "forest_olive_grove"
	}
	if c.Q < 0 && c.R <= 0 {
		return "mountain_limestone"
	}
	return "plains"
}

func balanceFixture(t *testing.T, kind string) (*pgxpool.Pool, uuid.UUID, uuid.UUID, economy.FoundingFoodForecast) {
	t.Helper()
	ctx := context.Background()
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	for _, q := range []string{"DELETE FROM settlement_placement WHERE settlement_id=$1", "DELETE FROM buildings WHERE settlement_id=$1", "DELETE FROM settlement_labor WHERE settlement_id=$1", "DELETE FROM settlement_goods WHERE settlement_id=$1"} {
		if _, err := pool.Exec(ctx, q, sid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "DELETE FROM map_tiles WHERE world_id=$1", wid); err != nil {
		t.Fatal(err)
	}
	for _, c := range hexgrid.Ring(hexgrid.Coord{}, hexgrid.CatchmentRadius) {
		terrain := balanceTerrain(kind, c)
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain,coastal) VALUES($1,$2,$3,$4,$5)`, wid, c.Q, c.R, terrain, terrain == "coastal_sea"); err != nil {
			t.Fatal(err)
		}
	}
	inland := kind == "hills" || kind == "plains"
	forecast, err := economy.FoundingFoodForecastAt(ctx, pool, wid, hexgrid.Coord{}, inland, nil, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if inland {
		c, ok, err := economy.ChooseFarmHex(ctx, pool, wid, hexgrid.Coord{}, nil)
		if err != nil || !ok {
			t.Fatalf("farm: %v %v", ok, err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,hex_q,hex_r) VALUES($1,'farm',1,$2,$3)`, sid, c.Q, c.R); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := economy.PlaceStartingWorkforce(ctx, pool, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'grain',1200,0,3600,0)`, sid); err != nil {
		t.Fatal(err)
	}
	if err := economy.RecomputeProduction(ctx, pool, sid); err != nil {
		t.Fatal(err)
	}
	return pool, wid, sid, forecast
}

type balanceState struct {
	Tick        int     `json:"tick"`
	Population  int     `json:"population"`
	FoodWorkers int     `json:"food_workers"`
	Grain       float64 `json:"grain"`
	Fish        float64 `json:"fish"`
	FoodNet     float64 `json:"food_net"`
	GrainStock  float64 `json:"grain_stock"`

	Timber float64 `json:"timber"`
	Stone  float64 `json:"stone"`
	Unmet  float64 `json:"unmet"`
}

func readBalance(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID) balanceState {
	t.Helper()
	ctx := context.Background()
	var s balanceState
	err := pool.QueryRow(ctx, `SELECT current_world_tick(),population,food_unmet_amount,
 (SELECT count(*) FROM settlement_placement WHERE settlement_id=s.id AND good_key IN ('grain','fish')),
 COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id=s.id AND good_key='grain'),0),
 COALESCE((SELECT rate FROM settlement_goods WHERE settlement_id=s.id AND good_key='fish'),0),
 COALESCE((SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=s.id AND good_key='grain'),0),
 COALESCE((SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=s.id AND good_key='timber'),0),
 COALESCE((SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=s.id AND good_key='stone'),0)
 FROM settlements s WHERE id=$1`, sid).Scan(&s.Tick, &s.Population, &s.Unmet, &s.FoodWorkers, &s.Grain, &s.Fish, &s.GrainStock, &s.Timber, &s.Stone)
	if err != nil {
		t.Fatal(err)
	}
	s.FoodNet = economy.FoodNet(s.Grain+s.Fish, s.Population)
	return s
}

func TestInlandFoodBalance_SevenOfTen(t *testing.T) {
	for _, kind := range []string{"hills", "plains", "coast", "amnissos"} {
		t.Run(kind, func(t *testing.T) {
			pool, _, sid, forecast := balanceFixture(t, kind)
			s := readBalance(t, pool, sid)
			if s.FoodWorkers > 7 || s.FoodNet < 0 {
				t.Fatalf("%s needs %d food workers, net %.12f; want <=7 and >=0", kind, s.FoodWorkers, s.FoodNet)
			}
			if math.Abs(s.Grain-forecast.GrainProduction) > 1e-9 || math.Abs(s.Fish-forecast.FishProduction) > 1e-9 {
				t.Fatalf("forecast %+v != actual %+v", forecast, s)
			}
			t.Logf("%s %+v", kind, s)
		})
	}
}

// Runs without changing rates: execute on separate fresh DBs before/after the
// migration. Fixed initial placements, then real FoodTick -> growth.
// No building/recruitment/trade or new-citizen placement policy is simulated.
func TestInlandFoodBalance_Soak60(t *testing.T) {
	for _, kind := range []string{"hills", "plains", "coast", "amnissos"} {
		t.Run(kind, func(t *testing.T) {
			pool, wid, sid, _ := balanceFixture(t, kind)
			ctx := context.Background()
			initial := readBalance(t, pool, sid)
			// Allocate available workers to timber then stone, within each hex's slots.
			options, err := economy.LoadHexProductionOptions(ctx, pool, sid, nil)
			if err != nil {
				t.Fatal(err)
			}
			next := initial.FoodWorkers + 1
			for _, good := range []string{"timber", "stone"} {
				wanted := 1
				if good == "timber" {
					wanted = 2
				}
				for _, opt := range options {
					for n := 0; n < opt.PlaceCapPerGood[good] && wanted > 0 && next <= 10; n++ {
						if _, err := pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,$2,'hex',$3,$4,$5)`, sid, next, opt.Coord.Q, opt.Coord.R, good); err != nil {
							t.Fatal(err)
						}
						next++
						wanted--
					}
				}
			}
			if err := economy.RecomputeProduction(ctx, pool, sid); err != nil {
				t.Fatal(err)
			}
			records := []balanceState{readBalance(t, pool, sid)}

			food := newTestFoodTickHandler(pool)
			growth := newTestTickHandler(pool)
			for tick := 1; tick <= 60; tick++ {
				if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, wid, tick); err != nil {
					t.Fatal(err)
				}
				if err := food.Handle(ctx, events.ScheduledEvent{ID: atomic.AddInt64(&advanceOneDayEventID, 1), WorldID: wid, DueTick: tick}); err != nil {
					t.Fatal(err)
				}
				growth.applyDecay(ctx, wid, atomic.AddInt64(&advanceOneDayEventID, 1))
				s := readBalance(t, pool, sid)
				if s.Unmet > 0 {
					t.Fatalf("unexpected actual hunger: %+v", s)
				}
				if tick%10 == 0 {
					records = append(records, s)
				}
			}
			raw, err := json.Marshal(map[string]any{"kind": kind, "states": records})
			if err != nil {
				t.Fatal(err)
			}
			t.Log("BALANCE_JSON " + string(raw))
		})
	}
}

// Catalogue edits cannot rewrite lazy history or silently lower another good.
// This models the required cmd/recompute-all deployment on an existing city.
func TestInlandFoodBalance_ExistingCityRecompute(t *testing.T) {
	for _, kind := range []string{"hills", "plains", "coast", "amnissos"} {
		t.Run(kind, func(t *testing.T) {
			pool, wid, sid, _ := balanceFixture(t, kind)
			ctx := context.Background()
			var current float64
			if err := pool.QueryRow(ctx, `SELECT rate_per_tick FROM production_rules WHERE terrain_type='hills' AND good_key='grain' AND building_type IS NULL`).Scan(&current); err != nil {
				t.Fatal(err)
			}
			defer pool.Exec(ctx, `UPDATE production_rules SET rate_per_tick=$1 WHERE terrain_type='hills' AND good_key='grain' AND building_type IS NULL`, current)
			if _, err := pool.Exec(ctx, `UPDATE production_rules SET rate_per_tick=1.0/3.0 WHERE terrain_type='hills' AND good_key='grain' AND building_type IS NULL`); err != nil {
				t.Fatal(err)
			}
			if err := economy.RecomputeProduction(ctx, pool, sid); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=10 WHERE id=$1`, wid); err != nil {
				t.Fatal(err)
			}
			before := readBalance(t, pool, sid)
			if _, err := pool.Exec(ctx, `UPDATE production_rules SET rate_per_tick=0.5 WHERE terrain_type='hills' AND good_key='grain' AND building_type IS NULL`); err != nil {
				t.Fatal(err)
			}
			stale := readBalance(t, pool, sid)
			if stale.Grain != before.Grain {
				t.Fatal("catalogue magically changed stored rate")
			}
			if err := economy.RecomputeProduction(ctx, pool, sid); err != nil {
				t.Fatal(err)
			}
			after := readBalance(t, pool, sid)
			if math.Abs(after.GrainStock-before.GrainStock) > 1e-8 || after.Grain < before.Grain || after.Fish != before.Fish || after.Timber != before.Timber || after.Stone != before.Stone {
				t.Fatalf("recompute lost stock/output: before=%+v after=%+v", before, after)
			}
			t.Logf("%s existing before=%+v after=%+v", kind, before, after)
		})
	}
}
