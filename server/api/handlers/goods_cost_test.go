package handlers

import (
	"context"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/province"
)

func TestBuildTemple_FloatingPointExactCost(t *testing.T) {
	f := buildHexFixture(t)
	const stored = 23.999999999999996
	setStock(t, f.pool, f.settlementID, "stone", stored)
	var raw float64
	if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='stone'`, f.settlementID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != stored || raw >= 24 {
		t.Fatalf("raw stone = %.17g, want literal under 24", raw)
	}
	code, body := f.do(t, http.MethodPost, f.buildPath(), map[string]any{"building_type": "temple", "god": "potnia"})
	if code != http.StatusCreated {
		t.Fatalf("build temple at stored %.17g: HTTP %d: %v", raw, code, body)
	}
	if got := stockOf(t, f.pool, f.settlementID, "stone"); got != 0 {
		t.Fatalf("remaining stone = %.17g, want zero", got)
	}
	var queued int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM build_queue WHERE settlement_id=$1 AND building_type='temple'`, f.settlementID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued temples = %d", queued)
	}
}

func TestGoodsPayment_ResidueAndRealShortage(t *testing.T) {
	f := buildHexFixture(t)
	for _, tc := range []struct {
		name       string
		have, need float64
		ok         bool
	}{
		{"stone residue", 23.999999999999996, 24, true},
		{"mine residue", 15.999999999999998, 16, true},
		{"real shortage", 23.99999, 24, false},
		{"real fractional shortage", 23.9, 24, false},
		{"surplus", 25, 24, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"multi", "atomic"} {
				setStock(t, f.pool, f.settlementID, "stone", tc.have)
				tx, err := f.pool.Begin(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				var paid bool
				if mode == "multi" {
					paid = deductGoods(context.Background(), tx, f.settlementID, map[string]float64{"stone": tc.need}) == nil
				} else {
					tag, e := province.DeductGood(context.Background(), tx, f.settlementID, "stone", tc.need)
					if e != nil {
						t.Fatal(e)
					}
					paid = tag.RowsAffected() == 1
				}
				if paid != tc.ok {
					tx.Rollback(context.Background())
					t.Errorf("%s paid=%v want %v", mode, paid, tc.ok)
					continue
				}
				if err := tx.Commit(context.Background()); err != nil {
					t.Fatal(err)
				}
				want := tc.have
				if tc.ok {
					want = tc.have - tc.need
					if want < 0 {
						want = 0
					}
				}
				if got := stockOf(t, f.pool, f.settlementID, "stone"); got != want {
					t.Errorf("%s stock %.17g want %.17g", mode, got, want)
				}
			}
		})
	}
	if !province.GoodsAffordable(23.999999999999996, 24) {
		t.Fatal("capability comparison must accept residue")
	}
}
