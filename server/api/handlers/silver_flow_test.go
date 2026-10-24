package handlers

import (
	"context"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/capabilities"
	"formatet/megaron/server/internal/clock"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSilverFlowRecruitHTTP(t *testing.T) {
	for _, kind := range []string{"spearman", "galley", "merchantman"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			ctx := context.Background()
			if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1 AND good_key='silver'`, f.settlementID); err != nil {
				t.Fatal(err)
			}
			if kind == "merchantman" {
				before := snapshotRecruitState(t, f)
				rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": kind})
				if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(strings.ToLower(rec.Body.String()), "cedar") {
					t.Fatalf("missing cedar: %d %s", rec.Code, rec.Body.String())
				}
				if after := snapshotRecruitState(t, f); !reflect.DeepEqual(before, after) {
					t.Fatal("rejection changed state")
				}
				if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'cedar',90,0,1000,0)`, f.settlementID); err != nil {
					t.Fatal(err)
				}
			}
			var woodBefore float64
			if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'`, f.settlementID).Scan(&woodBefore); err != nil {
				t.Fatal(err)
			}
			rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": kind})
			if rec.Code != http.StatusCreated {
				t.Fatalf("recruit: %d %s", rec.Code, rec.Body.String())
			}
			var wood, silver float64
			if err := f.pool.QueryRow(ctx, `SELECT (SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'),(SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver')`, f.settlementID).Scan(&wood, &silver); err != nil {
				t.Fatal(err)
			}
			wantWood := woodBefore
			if kind == "galley" {
				wantWood -= 30
			}
			if wood != wantWood || silver != 0 {
				t.Fatalf("wood=%g want%g silver=%g", wood, wantWood, silver)
			}
			if kind == "merchantman" {
				var cedar float64
				if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, f.settlementID).Scan(&cedar); err != nil || cedar != 0 {
					t.Fatalf("cedar=%g %v", cedar, err)
				}
			}
		})
	}
}

// Every conjunct of the old aggregate gate remains a concrete refusal. An
// aggregate affordability failure has no unique case beyond these checks.
func TestSilverFlowRecruitAggregateRefusalsRemain(t *testing.T) {
	for _, cause := range []string{"building", "goods", "population"} {
		t.Run(cause, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			ctx := context.Background()
			query := map[string]string{
				"building":   `DELETE FROM buildings WHERE settlement_id=$1`,
				"goods":      `UPDATE settlement_goods SET amount=0,rate=0 WHERE settlement_id=$1`,
				"population": `UPDATE settlements SET population=0 WHERE id=$1`,
			}[cause]
			if _, err := f.pool.Exec(ctx, query, f.settlementID); err != nil {
				t.Fatal(err)
			}
			cc := capabilities.NewContext(ctx, f.pool, clock.NewTestClock(time.Now()), f.worldID, f.provinceID, f.playerID, f.settlementID)
			if capabilities.CanRecruit(cc).Available {
				t.Fatal("fixture must fail old aggregate gate")
			}
			before := snapshotRecruitState(t, f)
			rec, _ := f.post(t, recruitPath(f), map[string]any{"unit_type": "galley"})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%s: %d %s", cause, rec.Code, rec.Body.String())
			}
			if after := snapshotRecruitState(t, f); !reflect.DeepEqual(before, after) {
				t.Fatal("refusal mutated state")
			}
		})
	}
}

func TestSilverFlowJoinHTTPStartsAt120(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	wid := seedJoinableWorld(t, pool)
	svc := auth.NewService(pool, "silver-flow")
	player, token := registerViewer(t, ctx, svc, "silver-flow-join")
	router := joinCultureRouter(pool, svc, clock.NewTestClock(time.Now()))
	request := func() int {
		req := httptest.NewRequest("POST", "/worlds/"+wid.String()+"/join", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := request(); code != 201 {
		t.Fatalf("join %d", code)
	}
	var silver float64
	if err := pool.QueryRow(ctx, `SELECT silver_amount FROM founder_phase WHERE world_id=$1 AND owner_id=$2 AND active`, wid, player).Scan(&silver); err != nil || silver != 120 {
		t.Fatalf("silver=%g %v", silver, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE founder_phase SET silver_amount=7 WHERE world_id=$1 AND owner_id=$2`, wid, player); err != nil {
		t.Fatal(err)
	}
	if code := request(); code != 200 {
		t.Fatalf("repeat join %d", code)
	}
	if err := pool.QueryRow(ctx, `SELECT silver_amount FROM founder_phase WHERE world_id=$1 AND owner_id=$2 AND active`, wid, player).Scan(&silver); err != nil || silver != 7 {
		t.Fatalf("repeat topped up silver=%g %v", silver, err)
	}
}
