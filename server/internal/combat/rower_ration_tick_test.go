package combat

import (
	"context"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"math"
	"testing"
)

// Exercise the real recurring charging loop, including a freight ship still
// bound in port: food access does not turn its mission into home rations.
func TestRowerCrewRationChargedFromOnboardGoods(t *testing.T) {
	for _, kind := range []string{"galley", "war_galley", "merchantman"} {
		for _, status := range []string{"garrison", "marching", "positioned", "freighting", "repairing", "fitting_out"} {
			t.Run(kind+"/"+status, func(t *testing.T) {
				pool := testPool(t)
				ctx := context.Background()
				f := newStarvationFixture(t, pool, "rower-meal")
				var ship uuid.UUID
				if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id,support_settlement_id,provisions,provision_goods)
 VALUES($1,$2,$3,'naval',1,2,$4,3,0,$5,$5,1,'{"fish":1}') RETURNING id`, f.worldID, f.ownerID, kind, status, f.capitalID).Scan(&ship); err != nil {
					t.Fatal(err)
				}
				h := newStarvationUpkeepHandler(pool, nil)
				e := events.ScheduledEvent{ID: 812012, WorldID: f.worldID, DueTick: 1}
				if err := h.Handle(ctx, e); err != nil {
					t.Fatal(err)
				}
				// A replay must not take a second meal.
				if err := h.Handle(ctx, e); err != nil {
					t.Fatal(err)
				}
				var total, fish float64
				var crew int
				if err := pool.QueryRow(ctx, `SELECT provisions,(provision_goods->>'fish')::float8,crew FROM units WHERE id=$1`, ship).Scan(&total, &fish, &crew); err != nil {
					t.Fatal(err)
				}
				want := 0.99
				if status == "marching" || status == "positioned" || status == "freighting" {
					want = 0.98
				}
				if math.Abs(total-want) > 1e-9 || math.Abs(fish-want) > 1e-9 || crew != 2 {
					t.Fatalf("crew2 %s %s: total%g fish%g crew%d want%g and no loss", kind, status, total, fish, crew, want)
				}
			})
		}
	}
}
