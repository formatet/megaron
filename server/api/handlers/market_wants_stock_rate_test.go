package handlers

// Pins the post-PR1 wants/exports semantics (megaron_plan_riv_prismekanismen.md,
// [[megaron_valtabell]] PR1 repealed 2026-08-19): MarketWants used to select
// on a system-computed local price (economy.LocalPrice, now deleted). The
// discovery signal survives, rerooted onto market_snapshots' observed
// stock+rate (economy.WantsDaysCover / economy.ExportsDaysCover /
// economy.MinFlowForCover — internal/economy/discovery.go):
//
//   want:    rate <= 0 AND stock < WantsDaysCover   * max(-rate, MinFlowForCover)
//   surplus: rate >  0 AND stock > ExportsDaysCover * rate
//
// Same rig as marches_messengers_fow_test.go / foreign_units_fow_test.go: real
// Postgres (DATABASE_URL-gated), citiesTestPool, a real auth.Service-minted
// token, and a chi.Mux running ProvinceHandler.MarketWants directly. Seeds
// market_snapshots rows DIRECTLY (bypassing RecordMarketSnapshot) so each
// case's stock/rate is exact and deterministic — this is also the FOW
// invariant in miniature: the handler must read only the frozen
// market_snapshots row, never live settlement_goods.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type wantsRespGood struct {
	Good  string  `json:"good"`
	Stock float64 `json:"stock"`
	Rate  float64 `json:"rate"`
}

type wantsRespSettlement struct {
	SettlementID string          `json:"settlement_id"`
	Name         string          `json:"name"`
	Secondhand   bool            `json:"secondhand"`
	Goods        []wantsRespGood `json:"goods"`
}

type wantsResp struct {
	Wants   []wantsRespSettlement `json:"wants"`
	Surplus []wantsRespSettlement `json:"surplus"`
}

// flatGoods collects every good key that shows up anywhere in a wants/surplus
// response, across all settlements — the tests below only seed one
// settlement, so a flat set is enough to assert presence/absence without
// caring which settlementWants bucket it landed in.
func flatGoods(list []wantsRespSettlement) map[string]wantsRespGood {
	out := map[string]wantsRespGood{}
	for _, sw := range list {
		for _, g := range sw.Goods {
			out[g.Good] = g
		}
	}
	return out
}

func TestMarketWants_StockRateSemantics(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "wants-viewer")

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"wants-owner-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create settlement owner: %v", err)
	}

	provinceID := insertProvince(t, ctx, pool, worldID, 5, 5)
	settlementID := insertSettlement(t, ctx, pool, worldID, provinceID, "Tradeburg", ownerID, true)
	// Secondhand case gets its OWN settlement: settlementWants dedupes on
	// settlement_id and stamps Secondhand from whichever row the ORDER BY
	// visits first for that settlement, so mixing a secondhand row into
	// Tradeburg above (which also holds firsthand rows) would let a firsthand
	// row win the flag depending on sort order — not what this case is
	// isolating.
	secondhandProvinceID := insertProvince(t, ctx, pool, worldID, 6, 6)
	secondhandSettlementID := insertSettlement(t, ctx, pool, worldID, secondhandProvinceID, "Rumourburg", ownerID, false)

	seedAt := func(sett uuid.UUID, good string, stock, rate float64, secondhand bool) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO market_snapshots (player_id, settlement_id, good_key, stock, rate, observed_at, secondhand)
			 VALUES ($1, $2, $3, $4, $5, now(), $6)`,
			viewerID, sett, good, stock, rate, secondhand,
		); err != nil {
			t.Fatalf("seed market_snapshots(%s): %v", good, err)
		}
	}
	seed := func(good string, stock, rate float64, secondhand bool) {
		seedAt(settlementID, good, stock, rate, secondhand)
	}

	// Case 1: draining, low stock → want, not surplus. rate=-2, stock=6:
	// threshold = WantsDaysCover(5) * max(2, MinFlowForCover(1)) = 10; 6 < 10.
	seed("tin", 6, -2, false)
	// Case 2: producing, built-up stock → surplus, not want. rate=8, stock=500:
	// threshold = ExportsDaysCover(20) * 8 = 160; 500 > 160.
	seed("timber", 500, 8, false)
	// Case 3: never produced, nothing held → want (the good it doesn't do at
	// all). rate=0, stock=0: threshold = 5 * max(0,1) = 5; 0 < 5.
	seed("oil", 0, 0, false)
	// Case 4a: silver would otherwise qualify as a want (draining, empty) but
	// must be excluded categorically.
	seed("silver", 0, -5, false)
	// Case 4b: a sacred good (cult) would otherwise qualify as a want but
	// must be excluded by category.
	seed("cult", 0, -5, false)
	// Case 5: FOW secondhand row still passes the gate and carries
	// secondhand=true through to the response. rate=-3, stock=2:
	// threshold = 5 * max(3,1) = 15; 2 < 15 → want. Own settlement (see note
	// above on why this can't share Tradeburg's rows).
	seedAt(secondhandSettlementID, "grain", 2, -3, true)
	// Control: a self-sufficient good with a healthy but not overflowing
	// buffer must land in NEITHER bucket. rate=1, stock=5:
	// want check fails (rate>0); surplus threshold = 20*1=20; 5 is not > 20.
	seed("stone", 5, 1, false)

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/market/wants", ph.MarketWants)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/market/wants", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /market/wants = %d %q, want 200", rec.Code, rec.Body.String())
	}

	var resp wantsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode /market/wants: %v (body: %s)", err, rec.Body.String())
	}

	wants := flatGoods(resp.Wants)
	surplus := flatGoods(resp.Surplus)

	// Case 1: tin is a want, not a surplus.
	if g, ok := wants["tin"]; !ok {
		t.Errorf("tin (stock=6, rate=-2) missing from wants")
	} else if g.Stock != 6 || g.Rate != -2 {
		t.Errorf("tin want = %+v, want stock=6 rate=-2", g)
	}
	if _, ok := surplus["tin"]; ok {
		t.Errorf("tin must not appear in surplus")
	}

	// Case 2: timber is a surplus, not a want.
	if g, ok := surplus["timber"]; !ok {
		t.Errorf("timber (stock=500, rate=8) missing from surplus")
	} else if g.Stock != 500 || g.Rate != 8 {
		t.Errorf("timber surplus = %+v, want stock=500 rate=8", g)
	}
	if _, ok := wants["timber"]; ok {
		t.Errorf("timber must not appear in wants")
	}

	// Case 3: oil (never produced, nothing held) is a want.
	if _, ok := wants["oil"]; !ok {
		t.Errorf("oil (stock=0, rate=0) missing from wants")
	}

	// Case 4: silver and cult (sacred) excluded from both, even though their
	// stock/rate would otherwise qualify as a want.
	if _, ok := wants["silver"]; ok {
		t.Errorf("silver must never appear in wants")
	}
	if _, ok := surplus["silver"]; ok {
		t.Errorf("silver must never appear in surplus")
	}
	if _, ok := wants["cult"]; ok {
		t.Errorf("cult (sacred) must never appear in wants")
	}
	if _, ok := surplus["cult"]; ok {
		t.Errorf("cult (sacred) must never appear in surplus")
	}

	// Case 5: grain is a want and carries secondhand=true on its settlement row.
	if _, ok := wants["grain"]; !ok {
		t.Errorf("grain (stock=2, rate=-3) missing from wants")
	}
	foundSecondhand := false
	for _, sw := range resp.Wants {
		for _, g := range sw.Goods {
			if g.Good == "grain" {
				if !sw.Secondhand {
					t.Errorf("grain want row secondhand=false, want true")
				}
				foundSecondhand = true
			}
		}
	}
	if !foundSecondhand {
		t.Errorf("grain want row not found to check secondhand flag")
	}

	// Control: stone lands in neither bucket.
	if _, ok := wants["stone"]; ok {
		t.Errorf("stone (self-sufficient, stock=5 rate=1) must not appear in wants")
	}
	if _, ok := surplus["stone"]; ok {
		t.Errorf("stone (stock=5, rate=1, below ExportsDaysCover) must not appear in surplus")
	}
}
