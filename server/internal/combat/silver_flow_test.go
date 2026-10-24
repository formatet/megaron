package combat

import (
	"context"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"strconv"
	"testing"
	"time"
)

func TestSilverFlowRoundDice(t *testing.T) {
	for _, tc := range []struct {
		kind       string
		size, want int
	}{
		{"spearman", 100, 50}, {"spearman", 1, 1}, {"spearman", 3, 2}, {"war_galley", 1, 5}, {"galley", 1, 1}, {"elite_infantry", 100, 300}, {"war_chariot", 100, 500},
	} {
		t.Run(tc.kind+strconv.Itoa(tc.size), func(t *testing.T) {
			p := []battleParticipant{{utype: tc.kind, hull: hullMax}}
			active, dice, _ := rollSide(p, []int{tc.size}, []int{0}, 1, economy.NewSeededDice(42))
			if active != tc.size || dice != tc.want {
				t.Fatalf("active=%d dice=%d want=%d", active, dice, tc.want)
			}
		})
	}
}

func TestSilverFlowBasicShipsNeverDesertForSilver(t *testing.T) {
	for _, kind := range []string{"galley", "merchantman"} {
		for _, status := range []string{"garrison", "positioned"} {
			t.Run(kind+"/"+status, func(t *testing.T) {
				pool := testPool(t)
				ctx := context.Background()
				f := newSupportFixture(t, pool, "free-ship")
				seedGoods(t, pool, f.capitalID, f.tick, 100000, 0)
				var id uuid.UUID
				if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id,support_settlement_id,provisions,provision_goods,unpaid_periods) VALUES($1,$2,$3,'naval',1,10,$4,$5,$5,10000,'{"grain":10000}',$6) RETURNING id`, f.worldID, f.owner, kind, status, f.capitalID, upkeepDesertionTicks-1).Scan(&id); err != nil {
					t.Fatal(err)
				}
				clk := clock.NewTestClock(time.Now())
				h := NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, nil, clk)
				for n := 0; n < upkeepDesertionTicks+2; n++ {
					if err := h.Handle(ctx, events.ScheduledEvent{ID: int64(900000 + n), WorldID: f.worldID, DueTick: f.tick + n}); err != nil {
						t.Fatal(err)
					}
				}
				var crew, unpaid int
				var state string
				if err := pool.QueryRow(ctx, `SELECT crew,unpaid_periods,status FROM units WHERE id=$1`, id).Scan(&crew, &unpaid, &state); err != nil {
					t.Fatal(err)
				}
				if crew != 10 || unpaid != 0 || state == "disbanded" {
					t.Fatalf("crew=%d unpaid=%d state=%s", crew, unpaid, state)
				}
				var silver float64
				if err := pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, f.capitalID).Scan(&silver); err != nil || silver != 0 {
					t.Fatalf("silver=%g %v", silver, err)
				}
			})
		}
	}
}
