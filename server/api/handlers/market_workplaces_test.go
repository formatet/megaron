package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/economy"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestMarketWorkplacesMigrationFreesWorkersWithoutLosingPeople(t *testing.T) {
	for run := 0; run < 2; run++ {
		f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
		pool := p10TestPool(t)
		ctx := context.Background()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		down, err := os.ReadFile("../../db/migrations/205_remove_market_workplaces.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		up, err := os.ReadFile("../../db/migrations/205_remove_market_workplaces.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, string(down)); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'market',2);`, f.settlementID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','market','pottery')`, f.settlementID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,2,'hex',1,0,'grain')`, f.settlementID); err != nil {
			t.Fatal(err)
		}
		var before int
		tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, f.settlementID).Scan(&before)
		if _, err = tx.Exec(ctx, string(up)); err != nil {
			t.Fatal(err)
		}
		var people, market, other, rules, standing int
		if err = tx.QueryRow(ctx, `SELECT population,(SELECT count(*) FROM settlement_placement WHERE settlement_id=$1 AND building_type='market'),(SELECT count(*) FROM settlement_placement WHERE settlement_id=$1 AND good_key='grain'),(SELECT count(*) FROM production_rules WHERE building_type='market'),(SELECT count(*) FROM buildings WHERE settlement_id=$1 AND building_type='market') FROM settlements WHERE id=$1`, f.settlementID).Scan(&people, &market, &other, &rules, &standing); err != nil {
			t.Fatal(err)
		}
		if people != before || market != 0 || other != 1 || rules != 0 || standing != 1 {
			t.Fatal(people, before, market, other, rules, standing)
		}
		// Physical idle = unchanged workforce minus remaining placements; no new
		// assignment is invented. Down restores the rule, never the old worker.
		if _, err = tx.Exec(ctx, string(down)); err != nil {
			t.Fatal(err)
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1 AND building_type='market'`, f.settlementID).Scan(&market); err != nil || market != 0 {
			t.Fatal(market, err)
		}
		tx.Rollback(ctx)
	}
}
func TestMarketCannotBecomeAWorkplaceEvenIfPotteryIsActive(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'market',3)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	// Isolated rollback fixture: even a stray future production-rule row must
	// not resurrect a market menu. The normal migration already deletes it.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE goods SET status='active' WHERE key='pottery'; INSERT INTO production_rules(terrain_type,building_type,good_key,rate_per_tick) VALUES(NULL,'market','pottery',1)`); err != nil {
		t.Fatal(err)
	}
	opts, err := economy.LoadBuildingProductionOptions(ctx, tx, f.settlementID)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range opts {
		if b.BuildingType == "market" {
			t.Fatal("market reappeared", b)
		}
	}
	tx.Rollback(ctx)
	code, body := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
	if code != 200 {
		t.Fatal(code, body)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), `"building_type":"market"`) {
		t.Fatal(string(raw))
	}
	code, body = f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "building", "building_type": "market", "good_key": "pottery"})
	if code != 422 {
		t.Fatal(code, body)
	}
	var pop, placed int
	pool.QueryRow(ctx, `SELECT population,(SELECT count(*) FROM settlement_placement WHERE settlement_id=$1) FROM settlements WHERE id=$1`, f.settlementID).Scan(&pop, &placed)
	if pop != 500 || placed != 0 {
		t.Fatal(pop, placed)
	}
	code, body = f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
	if code != 201 || body["gubbe_ordinal"] != float64(1) {
		t.Fatal("market refusal must leave the citizen free for real work", code, body)
	}
}
