package kharis

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
)

// Same starting placements on both code versions, controlled DB fixtures, not live play.
func TestRestfolk_Soak60(t *testing.T) {
	for _, kind := range []string{"scheria1042", "hills", "coast"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			terrain := kind
			if kind == "scheria1042" {
				terrain = "coast"
			}
			pool, wid, sid, _ := balanceFixture(t, terrain)
			if kind == "scheria1042" {
				if _, err := pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, sid); err != nil {
					t.Fatal(err)
				}
				ring := hexgrid.Ring(hexgrid.Coord{}, hexgrid.CatchmentRadius)
				for i, c := range ring {
					terrain := "forest_olive_grove"
					if i < 9 {
						terrain = "plains"
					}
					if i >= 9 && i < 13 {
						terrain = "coastal_sea"
					}
					if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain=$4,coastal=$5 WHERE world_id=$1 AND q=$2 AND r=$3`, wid, c.Q, c.R, terrain, terrain == "coastal_sea"); err != nil {
						t.Fatal(err)
					}
				}
				// One fisher keeps this a production-shortage fixture even on a
				// final-rule bay (2 fish/worker). Three fishers may now sustain growth;
				// this test isolates stored meals versus production, not fishing balance.
				for n := 1; n <= 1; n++ {
					c := ring[9+(n-1)/2]
					if _, err := pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,$2,'hex',$3,$4,'fish')`, sid, n, c.Q, c.R); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := pool.Exec(ctx, `UPDATE settlements SET population=1042 WHERE id=$1`, sid); err != nil {
				t.Fatal(err)
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
				for _, h := range []interface {
					Handle(context.Context, events.ScheduledEvent) error
				}{food} {
					if err := h.Handle(ctx, events.ScheduledEvent{ID: atomic.AddInt64(&advanceOneDayEventID, 1), WorldID: wid, DueTick: tick}); err != nil {
						t.Fatal(err)
					}
				}
				growth.applyDecay(ctx, wid, atomic.AddInt64(&advanceOneDayEventID, 1))
				if tick%10 == 0 {
					records = append(records, readBalance(t, pool, sid))
				}
			}
			last := records[len(records)-1]
			if last.Unmet != 0 {
				t.Fatalf("stocks should feed these fixtures: %+v", last)
			}
			if kind == "scheria1042" && (last.Population != 1042 || last.FoodNet >= 0) {
				t.Fatalf("production shortage must hold growth even while stocks feed: %+v", last)
			}
			if kind != "scheria1042" && last.Population <= 1100 {
				t.Fatalf("healthy city must grow across a hundred: %+v", last)
			}
			raw, err := json.Marshal(map[string]any{"kind": kind, "states": records})
			if err != nil {
				t.Fatal(err)
			}
			t.Log("RESTFOLK_JSON " + string(raw))
		})
	}
}
