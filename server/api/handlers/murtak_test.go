package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
)

// B1 "muren som befolkningstak": wall N needs a Granary (silo) of level N, and a
// Granary multiplies the store of every food except livestock.

func setSilo(t *testing.T, f *mineGateFixture, level int) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'silo', $2)
		 ON CONFLICT (settlement_id, building_type) WHERE hex_q IS NULL DO UPDATE SET level = EXCLUDED.level`,
		f.settlementID, level); err != nil {
		t.Fatal(err)
	}
}

// (c) Stone Wall is refused without a Granary of level 2, accepted with one.
func TestMurtak_StoneWallNeedsGranary2(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET wall_level = 1 WHERE id = $1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"building_type": "wall"}
	code, resp := f.do(t, http.MethodPost, f.buildPath(), body)
	if code != http.StatusUnprocessableEntity || !strings.Contains(resp["error"].(string), "Granary of level 2") {
		t.Fatalf("Stone Wall without any Granary = %d %v, want 422 naming Granary of level 2", code, resp)
	}
	setSilo(t, f, 1)
	code, resp = f.do(t, http.MethodPost, f.buildPath(), body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("Stone Wall with Granary 1 = %d %v, want 422", code, resp)
	}
	setSilo(t, f, 2)
	code, resp = f.do(t, http.MethodPost, f.buildPath(), body)
	if code != http.StatusCreated {
		t.Fatalf("Stone Wall with Granary 2 = %d %v, want 201", code, resp)
	}
}

// The first wall (Palisade) needs Granary 1.
func TestMurtak_PalisadeNeedsGranary1(t *testing.T) {
	f := buildHexFixture(t)
	body := map[string]any{"building_type": "wall"}
	if code, resp := f.do(t, http.MethodPost, f.buildPath(), body); code != http.StatusUnprocessableEntity {
		t.Fatalf("Palisade without Granary = %d %v, want 422", code, resp)
	}
	setSilo(t, f, 1)
	if code, resp := f.do(t, http.MethodPost, f.buildPath(), body); code != http.StatusCreated {
		t.Fatalf("Palisade with Granary 1 = %d %v, want 201", code, resp)
	}
}

func capOf(t *testing.T, f *mineGateFixture, good string) float64 {
	t.Helper()
	var c float64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT cap FROM settlement_goods WHERE settlement_id = $1 AND good_key = $2`, f.settlementID, good).Scan(&c); err != nil {
		t.Fatalf("cap of %s: %v", good, err)
	}
	return c
}

// (e) Granary 2 gives 5x the store for grain (existing rows AND rows made later)
// but nothing for livestock; levelling and losing the Granary follow.
func TestMurtak_GranaryMultipliesFoodStoreNotHerd(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	base := province.DefaultGoodStorageCap
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlement_goods WHERE settlement_id = $1 AND good_key IN ('grain','fish','wine','livestock')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	seed := func(good string) {
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, 0, 0, $3, 0) ON CONFLICT DO NOTHING`, f.settlementID, good, base); err != nil {
			t.Fatal(err)
		}
	}
	seed("grain")
	seed("livestock")
	setSilo(t, f, 2)
	if got := capOf(t, f, "grain"); got != province.FoodStorageCap(2) {
		t.Errorf("grain cap with Granary 2 = %v, want %v", got, province.FoodStorageCap(2))
	}
	if got := capOf(t, f, "livestock"); got != base {
		t.Errorf("livestock cap with Granary 2 = %v, want unchanged %v", got, base)
	}
	seed("fish") // a row born AFTER the Granary
	seed("wine")
	if got := capOf(t, f, "fish"); got != province.FoodStorageCap(2) {
		t.Errorf("fish row created after the Granary has cap %v, want %v", got, province.FoodStorageCap(2))
	}
	setSilo(t, f, 3)
	if got := capOf(t, f, "grain"); got != province.FoodStorageCap(3) {
		t.Errorf("grain cap with Granary 3 = %v, want %v", got, province.FoodStorageCap(3))
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'silo'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if got := capOf(t, f, "grain"); got != province.FoodStorageBase {
		t.Errorf("grain cap after the Granary is gone = %v, want %v", got, province.FoodStorageBase)
	}
}

// The SQL mirror (mig 197) and the Go owner agree on multiplier and on which goods.
func TestMurtak_SiloStorageSQLMirror(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	for lvl := 0; lvl <= 4; lvl++ {
		var m int
		if err := f.pool.QueryRow(ctx, `SELECT silo_food_mult($1)`, lvl).Scan(&m); err != nil {
			t.Fatal(err)
		}
		if float64(m) != province.SiloFoodStorageMult(lvl) {
			t.Errorf("silo_food_mult(%d) = %d, Go says %v", lvl, m, province.SiloFoodStorageMult(lvl))
		}
	}
	var b int
	if err := f.pool.QueryRow(ctx, `SELECT food_store_base()`).Scan(&b); err != nil || float64(b) != province.FoodStorageBase {
		t.Errorf("food_store_base() = %d (%v), Go says %v", b, err, province.FoodStorageBase)
	}
	food := map[string]bool{}
	for _, g := range economy.FoodGoods {
		food[g] = true
	}
	for _, g := range append(append([]string{}, economy.FoodGoods...), economy.GoodLivestock, "timber", "silver", "bronze") {
		var in bool
		if err := f.pool.QueryRow(ctx, `SELECT silo_stored_good($1)`, g).Scan(&in); err != nil {
			t.Fatal(err)
		}
		if in != food[g] {
			t.Errorf("silo_stored_good(%q) = %v, want %v (economy.FoodGoods, never livestock)", g, in, food[g])
		}
	}
}

// The cap table is the contract every surface reads.
func TestMurtak_PopulationCapTable(t *testing.T) {
	for wall, want := range map[int]int{-1: 2000, 0: 2000, 1: 5000, 2: 10000, 3: 20000, 9: 20000} {
		if got := economy.PopulationCap(wall); got != want {
			t.Errorf("PopulationCap(%d) = %d, want %d", wall, got, want)
		}
	}
}
