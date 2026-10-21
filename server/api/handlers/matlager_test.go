package handlers

// Food store 2 000 per good (mig 198, Timothy 2026-10-21): a stock already above
// the ceiling is kept and can be eaten, never refilled; the city eats from the
// capped stock; production lost to a full store is told once, not every tick.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
)

func foodRow(t *testing.T, f *mineGateFixture, good string, amount, rate float64, ticksAgo int) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlement_goods WHERE settlement_id = $1 AND good_key = ANY($2)`,
		f.settlementID, append(append([]string{}, economy.FoodGoods...), economy.GoodLivestock)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
	        VALUES ($1, $2, $3, $4, $5, current_world_tick() - $6)`,
		f.settlementID, good, amount, rate, province.DefaultGoodStorageCap, ticksAgo); err != nil {
		t.Fatal(err)
	}
}

func TestMatlager_KeptSurplusIsEatenNeverRefilled(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	foodRow(t, f, "fish", 5000, 0, 0)
	if got := capOf(t, f, "fish"); got != 5000 {
		t.Fatalf("a 5 000 stock keeps its fish: cap %v, want 5000", got)
	}
	// The city eats 1 000: the ceiling follows the stock down.
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount = amount - 1000 WHERE settlement_id = $1 AND good_key = 'fish'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if got := capOf(t, f, "fish"); got != 4000 {
		t.Errorf("after eating 1 000: cap %v, want 4000", got)
	}
	// A catch of 3 000 only fills back to 4 000.
	var stock float64
	if err := f.pool.QueryRow(ctx, `SELECT credit_settlement_good($1,'fish',3000,'transfer_delivery')`, f.settlementID).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	var amount float64
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'fish'`, f.settlementID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 4000 {
		t.Errorf("refill above the kept stock: amount %v, want 4000", amount)
	}
	// Eaten down below the ceiling: the ordinary ceiling holds from then on.
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount = 500 WHERE settlement_id = $1 AND good_key = 'fish'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if got := capOf(t, f, "fish"); got != province.FoodStorageBase {
		t.Errorf("eaten below the ceiling: cap %v, want %v", got, province.FoodStorageBase)
	}
	// A Granary built later raises it; losing it never cuts below the stock.
	setSilo(t, f, 1)
	if got := capOf(t, f, "fish"); got != province.FoodStorageCap(1) {
		t.Errorf("with Granary 1: cap %v, want %v", got, province.FoodStorageCap(1))
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount = 3500 WHERE settlement_id = $1 AND good_key = 'fish'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'silo'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if got := capOf(t, f, "fish"); got != 3500 {
		t.Errorf("Granary razed with 3 500 in store: cap %v, want 3500 (kept)", got)
	}
}

// Production accrued past the ceiling is not food the city has.
func TestMatlager_CityEatsFromCappedStock(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	foodRow(t, f, "grain", 1900, 100, 3) // settled 2 200 > ceiling 2 000
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := economy.TakeCivilianFood(ctx, tx, f.settlementID, 100); err != nil {
		t.Fatal(err)
	}
	var amount float64
	if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, f.settlementID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 1900 {
		t.Errorf("after eating 100 from a full store: %v, want 1900 (2 000 − 100)", amount)
	}
}

// A store that stays full loses production every tick; its owner hears it once.
func TestMatlager_ProductionOverflowToldOnce(t *testing.T) {
	f := buildHexFixture(t)
	ctx := context.Background()
	count := func(table string) int {
		var n int
		q := `SELECT count(*) FROM notifications WHERE kind='StorageOverflow' AND body_json->>'settlement_id' = $1`
		if table == "events" {
			q = `SELECT count(*) FROM events WHERE event_type='StorageOverflow' AND stream_id::text = $1`
		}
		if err := f.pool.QueryRow(ctx, q, f.settlementID.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for i := 0; i < 3; i++ {
		if _, err := f.pool.Exec(ctx, `SELECT storage_capped($1,'grain',2500,2000,'production')`, f.settlementID); err != nil {
			t.Fatal(err)
		}
	}
	if n := count("notifications"); n != 1 {
		t.Errorf("three overflowing productions: %d notices, want 1", n)
	}
	if n := count("events"); n != 3 {
		t.Errorf("three overflowing productions: %d events, want 3 (the outcome is always written)", n)
	}
	if _, err := f.pool.Exec(ctx, `SELECT storage_capped($1,'grain',2500,2000,'transfer_delivery')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if n := count("notifications"); n != 2 {
		t.Errorf("a delivery spilling over is always told: %d notices, want 2", n)
	}
}
