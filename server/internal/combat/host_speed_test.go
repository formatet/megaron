package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"testing"
	"time"
)

// Longer routes expose fractional rounding that a one-hex march masks.
func TestHostSpeed_TerrainRouteRounding(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, tc := range []struct {
		terrain     string
		host, spear []int
	}{
		{"plains", []int{1, 2, 6, 10}, []int{1, 2, 5, 9}},
		{"forest_cedar", []int{2, 4, 11, 20}, []int{2, 4, 10, 18}},
		{"forest_olive_grove", []int{2, 3, 8, 15}, []int{2, 3, 8, 14}},
		{"hills", []int{2, 4, 11, 20}, []int{2, 4, 10, 18}},
		{"semi_desert", []int{3, 7, 17, 30}, []int{3, 6, 15, 27}},
	} {
		if _, err := pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
			t.Fatal(err)
		}
		var world, owner uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES($1,'active',5) RETURNING id`, "host-speed-"+uuid.NewString()).Scan(&world); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, world) })
		if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "host-speed-"+uuid.NewString()).Scan(&owner); err != nil {
			t.Fatal(err)
		}
		for q := 0; q <= 9; q++ {
			if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, world, q, tc.terrain); err != nil {
				t.Fatal(err)
			}
		}
		for _, ty := range []unit.Type{unit.TypeNomadicHost, unit.TypeSpearman} {
			var id uuid.UUID
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,$3,'land',100,'positioned',0,0) RETURNING id`, world, owner, ty).Scan(&id); err != nil {
				t.Fatal(err)
			}
			for i, d := range []int{1, 2, 5, 9} {
				p, err := PreviewMarch(ctx, pool, clock.NewTestClock(time.Now()), MarchOrder{WorldID: world, PlayerID: owner, UnitID: id, TargetQ: d}, nil)
				if err != nil {
					t.Fatal(err)
				}
				want := tc.spear[i]
				if ty == unit.TypeNomadicHost {
					want = tc.host[i]
				}
				if p.DurationTicks != want || p.ArrivalTick != 5+want {
					t.Fatalf("%s %s %d hex: %+v want %d ticks", ty, tc.terrain, d, p, want)
				}
			}
		}
	}
}
