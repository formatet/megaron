package handlers

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/economy"
	"net/http"
	"testing"
)

func TestTempleUsesRealWorkforceAndActualLevelCapacity(t *testing.T) {
	for _, level := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
			pool := p10TestPool(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `UPDATE settlements SET population=1000 WHERE id=$1`, f.settlementID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',$2,'potnia')`, f.settlementID, level); err != nil {
				t.Fatal(err)
			}
			body := map[string]any{"target_kind": "building", "building_type": "temple", "good_key": "cult"}
			for n := 1; n <= 2*level; n++ {
				code, resp := f.do(t, http.MethodPost, f.placementsPath(), body)
				if code != 201 {
					t.Fatalf("worker%d: %d %v", n, code, resp)
				}
			}
			code, resp := f.do(t, http.MethodPost, f.placementsPath(), body)
			if code != 409 {
				t.Fatalf("over capacity %d %v", code, resp)
			}
			_, list := f.do(t, http.MethodGet, f.placementsPath(), nil)
			if list["pool_size"] != float64(10-2*level) {
				t.Fatalf("additive worker pool: %v", list)
			}
			staff, err := economy.LoadTempleStaffing(ctx, pool, f.settlementID)
			if err != nil || staff.Staffed != 2*level || staff.Capacity != 2*level {
				t.Fatalf("staff %+v %v", staff, err)
			}
			code, resp = f.do(t, http.MethodDelete, f.placementsPath()+"/1", nil)
			if code != 200 {
				t.Fatalf("remove %d %v", code, resp)
			}
			staff, err = economy.LoadTempleStaffing(ctx, pool, f.settlementID)
			if err != nil || staff.Staffed != 2*level-1 {
				t.Fatalf("remove staffing %+v %v", staff, err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_goods WHERE settlement_id=$1 AND good_key='cult'`, f.settlementID).Scan(&count); err != nil || count != 0 {
				t.Fatalf("cult stock minted %d %v", count, err)
			}
		})
	}
}
func TestTempleCannotEmployAWorkerAlreadyWorkingFood(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population=100 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	code, resp := f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
	if code != 201 {
		t.Fatalf("food %d %v", code, resp)
	}
	code, resp = f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "building", "building_type": "temple", "good_key": "cult"})
	if code != 409 {
		t.Fatalf("double employed %d %v", code, resp)
	}
}
