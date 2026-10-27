package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"net/http"
	"testing"
	"time"

	"formatet/megaron/server/internal/kharis"
)

func TestProvinceStandingMatchesTickOwner(t *testing.T) {
	f := setupMineGateFixture(t, nil)
	ctx := context.Background()
	f.router.Get("/worlds/{worldID}/provinces/{provinceID}", NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), nil, nil).Get)
	var playerID string
	if err := f.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&playerID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick) VALUES($1,$2,$3,40,0,0) ON CONFLICT DO NOTHING`, playerID, f.worldID, f.settlementID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		amount float64
		temple bool
	}{{4, false}, {15, false}, {59.9, true}, {60, true}, {90, false}, {90, true}} {
		if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='temple'`, f.settlementID); err != nil {
			t.Fatal(err)
		}
		if c.temple {
			if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',2,'potnia')`, f.settlementID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=$1,kharis_rate=0,kharis_calc_tick=current_world_tick() WHERE player_id=$2 AND world_id=$3`, c.amount, playerID, f.worldID); err != nil {
			t.Fatal(err)
		}
		code, resp := f.do(t, http.MethodGet, fmt.Sprintf("/worlds/%s/provinces/%s", f.worldID, f.provinceID), nil)
		if code != 200 {
			t.Fatalf("%d %v", code, resp)
		}
		b, _ := json.Marshal(resp["settlement"])
		var got struct {
			Standing kharis.Standing `json:"kharis_standing"`
		}
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if want := kharis.DescribeStanding(c.amount, c.temple, 0); got.Standing != want {
			t.Fatalf("amount=%g temple=%v: %+v want %+v", c.amount, c.temple, got.Standing, want)
		}
		if got.Standing.BlessingProbability != kharis.BlessingProbability(c.amount, c.temple) || got.Standing.PunishmentProbability != kharis.PunishmentProbability(c.amount, c.temple) {
			t.Fatal("API probability differs from tick owner")
		}
	}
}
