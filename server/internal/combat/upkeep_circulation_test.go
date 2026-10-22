package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"math"
	"testing"
	"time"
)

// Real payroll, not a second formula: damaged cohorts/rowers, home away from
// payer, field statuses, insufficient liquidity, legacy audit zeros and replay.
func TestWagesChargeActualPeopleWithoutCirculation(t *testing.T) {
	t.Setenv("UPKEEP_SOLD_SHARE", "1") // Retired environment setting cannot restore the credit.
	cases := []struct {
		typ, category, status string
		size, crew            int
		stock, want           float64
		away                  bool
	}{
		{"elite_infantry", "land", "garrison", 100, 0, 10, .75, false},
		{"elite_infantry", "land", "garrison", 50, 0, 10, .375, true},
		{"elite_infantry", "land", "marching", 50, 0, 10, .75, true},
		{"war_chariot", "land", "positioned", 25, 0, 10, .5, true},
		{"war_galley", "naval", "garrison", 1, 7, 10, .035, false},
		{"war_galley", "naval", "freighting", 1, 7, 10, .07, true},
		{"galley", "naval", "repairing", 1, 2, 10, .01, false},
		{"merchantman", "naval", "fitting_out", 1, 2, 10, .01, false},
		{"elite_infantry", "land", "garrison", 100, 0, .74, .75, false},
	}
	for i, c := range cases {
		t.Run(c.typ+"/"+c.status+"/"+string(rune('a'+i)), func(t *testing.T) {
			ctx := context.Background()
			pool := testPool(t)
			f := newSupportFixture(t, pool, "wage")
			seedGoods(t, pool, f.capitalID, f.tick, 1000, 10)
			if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=$2 WHERE settlement_id=$1 AND good_key='silver'`, f.capitalID, c.stock); err != nil {
				t.Fatal(err)
			}
			var uid uuid.UUID
			location := f.capitalID
			if c.away {
				location = f.townID
			}
			if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,status,size,crew,settlement_id,support_settlement_id,provisions,provision_goods) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,100,'{"grain":100}') RETURNING id`, f.worldID, f.owner, c.typ, c.category, c.status, c.size, c.crew, location, f.capitalID).Scan(&uid); err != nil {
				t.Fatal(err)
			}
			h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), nil, nil)
			e := events.ScheduledEvent{ID: int64(876000 + i), WorldID: f.worldID, DueTick: f.tick}
			if err := h.Handle(ctx, e); err != nil {
				t.Fatal(err)
			}
			wantStock := c.stock
			paid := c.stock >= c.want
			if paid {
				wantStock -= c.want
			}
			if got := settledAmount(t, pool, f.capitalID, "silver"); math.Abs(got-wantStock) > 1e-9 {
				t.Fatalf("paid=%v debit gives%v want%v", paid, got, wantStock)
			}
			var gross, circ, destroyed, unpaid float64
			var circMap string
			if err := pool.QueryRow(ctx, `SELECT (payload->>'silver_gross')::float,(payload->>'silver_circulated')::float,(payload->>'silver_destroyed')::float,(payload->>'silver_unpaid')::float,(payload->'circulated_to')::text FROM events WHERE world_id=$1 AND event_type='UpkeepSettled' AND stream_id=$2`, f.worldID, f.capitalID).Scan(&gross, &circ, &destroyed, &unpaid, &circMap); err != nil {
				t.Fatal(err)
			}
			if circ != 0 || circMap != "{}" || math.Abs(gross-destroyed) > 1e-9 {
				t.Fatalf("audit gross/circ/destroyed/map=%v/%v/%v/%s", gross, circ, destroyed, circMap)
			}
			if paid && (math.Abs(gross-c.want) > 1e-9 || unpaid != 0) {
				t.Fatalf("paid audit=%v/%v", gross, unpaid)
			}
			if !paid && (gross != 0 || math.Abs(unpaid-c.want) > 1e-9) {
				t.Fatalf("unpaid audit=%v/%v", gross, unpaid)
			}
			if err := h.Handle(ctx, e); err != nil {
				t.Fatal(err)
			}
			if got := settledAmount(t, pool, f.capitalID, "silver"); math.Abs(got-wantStock) > 1e-9 {
				t.Fatalf("replay debit=%v want%v", got, wantStock)
			}
		})
	}
}
