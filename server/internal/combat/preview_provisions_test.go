package combat

import (
	"context"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// GPT19: a landing previewed at 17 days, then the order was refused for 35
// grain the port did not hold. The preview must refuse the same way.
func TestPreviewMarch_RefusesWhenThePortCannotProvision(t *testing.T) {
	pool, world, owner, city := setupLandMarchWorldWithHome(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1`, world); err != nil {
		t.Fatal(err)
	}
	for q := 0; q <= 12; q++ {
		terrain := "coastal_sea"
		if q == 0 {
			terrain = "plains"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, world, q, terrain); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', 0, 0, 1000, 0)
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = 0, rate = 0`, city); err != nil {
		t.Fatal(err)
	}
	var ship uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,settlement_id,support_settlement_id)
		 VALUES($1,$2,'galley','naval',1,20,'garrison',0,0,$3,$3) RETURNING id`, world, owner, city).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	order := MarchOrder{WorldID: world, PlayerID: owner, UnitID: ship, TargetQ: 12, Intent: "patrol"}

	_, dispatchErr := StartMarch(ctx, pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, order, nil)
	if dispatchErr == nil || !strings.Contains(dispatchErr.Error(), "not enough food to provision") {
		t.Fatalf("dispatch from an empty port = %v, want the provisions refusal", dispatchErr)
	}
	_, previewErr := PreviewMarch(ctx, pool, clk, order, nil)
	if previewErr == nil || previewErr.Error() != dispatchErr.Error() {
		t.Fatalf("preview = %v, want the same refusal as dispatch (%v)", previewErr, dispatchErr)
	}
}
