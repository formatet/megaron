package handlers

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/province"
	"net/http"
	"strings"
	"testing"
)

func TestBuildShipyardRefusalNamesExistingAndTargetLevels(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'shipyard',1)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1 AND good_key='cedar'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	code, resp := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "shipyard"})
	if code != 422 || resp["error"] != "insufficient_goods" {
		t.Fatalf("%d %v", code, resp)
	}
	msg, _ := resp["message"].(string)
	for _, s := range []string{"Raising Shipyard to level 2", "cedar (need 4, have 0", "Georgopolis already has Shipyard level 1"} {
		if !strings.Contains(msg, s) {
			t.Fatalf("missing %q: %v", s, resp)
		}
	}
	if len(resp["missing"].([]any)) != 1 {
		t.Fatal(resp)
	}
	var stock float64
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'`, f.settlementID).Scan(&stock); err != nil || stock != 200 {
		t.Fatalf("failed build drained timber %g %v", stock, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE buildings SET level=3 WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	code, resp = f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "shipyard"})
	if code != 422 || !strings.Contains(fmt.Sprint(resp["error"]), "Shipyard level 3, the maximum") {
		t.Fatalf("max %d %v", code, resp)
	}
}
func TestBuildQuotesKeepEachHexLevelAndExactNextSpec(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	for _, row := range []struct{ q, r, level int }{{1, 0, 1}, {-1, 0, 2}, {0, -1, 3}} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,hex_q,hex_r) VALUES($1,'farm',$2,$3,$4)`, f.settlementID, row.level, row.q, row.r); err != nil {
			t.Fatal(err)
		}
	}
	quotes, err := loadBuildQuotes(ctx, f.pool, f.settlementID)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, q := range quotes {
		if q.Type != "farm" {
			continue
		}
		seen++
		if q.CurrentLevel == 3 {
			if !q.AtMax || q.NextLevel != nil || q.Costs != nil {
				t.Fatal(q)
			}
			continue
		}
		spec, _ := province.LevelledSpec(province.BuildingFarm, q.CurrentLevel+1)
		if q.NextLevel == nil || *q.NextLevel != q.CurrentLevel+1 || q.Costs["timber"] != spec.Costs["timber"] || q.Duration != spec.DurationTicks {
			t.Fatalf("quote %+v spec%+v", q, spec)
		}
		if q.NewSite && q.CurrentLevel != 0 {
			t.Fatal(q)
		}
	}
	if seen != 4 {
		t.Fatalf("want new site + three separate hexes; got%d", seen)
	}
}
func TestBuildQuotesMatchEveryLevelledSpec(t *testing.T) {
	for bt := range province.LevelledBuildings {
		for level := 0; level < 3; level++ {
			q := quoteBuild(bt, level, nil, nil, false)
			spec, _ := province.LevelledSpec(bt, level+1)
			if q.NextLevel == nil || q.Duration != spec.DurationTicks || q.CostSilver != spec.CostSilver {
				t.Fatalf("%s/%d %+v", bt, level, q)
			}
			for g, n := range spec.Costs {
				if q.Costs[g] != n {
					t.Fatalf("%s L%d %s: %g want%g", bt, level+1, g, q.Costs[g], n)
				}
			}
		}
	}
}
