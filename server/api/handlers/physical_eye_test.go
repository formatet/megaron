package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestMarchEyeUsesSavedTickRouteAtMarkerPosition(t *testing.T) {
	for _, category := range []string{"land", "naval"} {
		t.Run(category, func(t *testing.T) {
			terrain := map[[2]int]string{}
			for q := 0; q <= 9; q++ {
				for r := 0; r <= 3; r++ {
					if q == 0 && r == 0 {
						continue
					}
					terrain[[2]int{q, r}] = "plains"
					if category == "naval" {
						terrain[[2]int{q, r}] = "coastal_sea"
					}
				}
			}
			f := setupPlacementFixture(t, terrain)
			pool := p10TestPool(t)
			ctx := context.Background()
			var owner uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&owner); err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=15,last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
				t.Fatal(err)
			}
			// The persisted detour and uneven costs deliberately disagree with both
			// A* recomputation and wall timestamps (whose departure has not happened).
			route := combat.StoredRoute{StartTick: 10, EndTick: 20, Hexes: [][2]int{{4, 0}, {4, 1}, {5, 1}, {6, 1}}, Costs: []int64{1, 8, 1}}
			raw, err := json.Marshal(route)
			if err != nil {
				t.Fatal(err)
			}
			kind := "spearman"
			if category == "naval" {
				kind = "galley"
			}
			if _, err := pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r,target_q,target_r,departs_at,arrives_at,depart_tick,arrive_tick,march_route) VALUES($1,$2,$3,$4,1,'marching',4,0,6,1,$5,$6,10,20,$7)`, f.worldID, owner, kind, category, now.Add(time.Hour), now.Add(2*time.Hour), raw); err != nil {
				t.Fatal(err)
			}
			for _, at := range []struct {
				tick int
				want province.MapPosition
			}{{10, province.MapPosition{Q: 4, R: 1}}, {11, province.MapPosition{Q: 5, R: 1}}, {15, province.MapPosition{Q: 5, R: 1}}, {19, province.MapPosition{Q: 6, R: 1}}, {20, province.MapPosition{Q: 6, R: 1}}} {
				if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=$2 WHERE id=$1`, f.worldID, at.tick); err != nil {
					t.Fatal(err)
				}
				marker, err := combat.RoutePositionAt(route, int64(at.tick)*1000)
				if err != nil {
					t.Fatal(err)
				}
				if marker != at.want {
					t.Fatalf("fixture tick%d marker%v want%v", at.tick, marker, at.want)
				}
				eyes := loadLiveEyes(ctx, pool, f.worldID, owner, now)
				seen := false
				for _, eye := range eyes {
					if (category == "naval" && eye.Kind == province.EyeShip) || (category == "land" && eye.Kind == province.EyeLandUnit) {
						seen = true
						if eye.Pos != marker {
							t.Fatalf("tick%d eye%v marker%v", at.tick, eye.Pos, marker)
						}
					}
				}
				if !seen {
					t.Fatal("moving eye missing")
				}
			}
			// The same read works inside one transaction; no pool-only hidden read.
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if len(loadLiveEyes(ctx, tx, f.worldID, owner, now)) == 0 {
				t.Fatal("transaction eyes missing")
			}
		})
	}
}
