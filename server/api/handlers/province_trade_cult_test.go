package handlers

// Regression test for fix/cult-ar-inte-last (megaron_plan_kult_ar_inte_last.md).
//
// ProvinceHandler.Trade (the own→own internal-transfer endpoint) validated a
// good_key with nothing but `SELECT COALESCE(weight, 2) FROM goods WHERE key =
// $1` — cult IS a row in the goods table (kept as the FK anchor
// settlement_labor.good_key needs, migration 094's comment), so that lookup
// always succeeded and cult passed the same gate as any real good. Migration
// 094 deleted every settlement_goods row with good_key='cult' and nothing
// since has recreated one with a positive amount — cult is devotion, produced
// by temple labor and converted to kharis in place (internal/kharis/tick.go),
// never a stock a settlement carries — so the request would normally die
// later with a misleading 422 "insufficient goods" (or, if a stray positive
// row ever existed — e.g. a leftover from before mig 094, or any future bug
// that credits one — would succeed outright and actually move cult between
// settlements). Both are wrong: the rejection must be a 400, before any
// stock check, and it must never depend on the current stock level.
//
// Reuses setupTradeInternalFixture from province_trade_internal_test.go (same
// package, same DB harness) — two settlements, one player, 3 hexes apart.
//
// Real Postgres, gated by DATABASE_URL — same harness as recruit_ship_test.go.

import (
	"context"
	"strings"
	"testing"
)

// TestTrade_CultRejectedWithoutStock is the fail-fast case (AK1): a settlement
// with NO cult row at all must still get 400 with an actionable message, not
// the generic 422 "insufficient goods" produced-by-accident behaviour the
// deduct step would otherwise give it. Proves the gate fires before the
// stock check ever runs.
func TestTrade_CultRejectedWithoutStock(t *testing.T) {
	f := setupTradeInternalFixture(t)

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade",
		map[string]any{"destination_id": f.destID.String(), "good_key": "cult", "quantity": 10.0})

	if code != 400 {
		t.Fatalf("trade good_key=cult (no stock) returned %d: %v, want 400", code, resp)
	}
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "cult") || !strings.Contains(msg, "temple labor") {
		t.Errorf("error message = %q, want it to name cult and explain temple labor (actionable for human and LLM agent)", msg)
	}

	// No route/transport bookkeeping should exist for this rejected attempt.
	var count int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM trade_routes WHERE origin_id = $1 AND good_key = 'cult'`,
		f.originID,
	).Scan(&count); err != nil {
		t.Fatalf("count trade_routes: %v", err)
	}
	if count != 0 {
		t.Errorf("trade_routes rows for rejected cult transfer = %d, want 0", count)
	}
}

// TestTrade_CultRejectedWithStock is the exploit case (AK2): if a settlement
// somehow DOES carry a positive cult amount — a stray row surviving from
// before migration 094, or any future bug that credits one — the transfer
// must still be rejected, and the stock must be left exactly as it was (no
// partial deduction before the rejection). This is what "cult passes the
// same gate as every other good" looked like before the fix: with stock
// present, the request used to succeed (201) and actually move cult between
// settlements.
func TestTrade_CultRejectedWithStock(t *testing.T) {
	f := setupTradeInternalFixture(t)
	ctx := context.Background()

	if _, err := f.pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'cult', 50, 0, 1000000, 0)
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = 50`,
		f.originID,
	); err != nil {
		t.Fatalf("seed stray cult stock: %v", err)
	}

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade",
		map[string]any{"destination_id": f.destID.String(), "good_key": "cult", "quantity": 10.0})

	if code != 400 {
		t.Fatalf("trade good_key=cult (50 in stock) returned %d: %v, want 400", code, resp)
	}
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "cult") || !strings.Contains(msg, "temple labor") {
		t.Errorf("error message = %q, want it to name cult and explain temple labor", msg)
	}

	var amount float64
	if err := f.pool.QueryRow(ctx,
		`SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'cult'`,
		f.originID,
	).Scan(&amount); err != nil {
		t.Fatalf("read back origin cult stock: %v", err)
	}
	if amount != 50 {
		t.Errorf("origin cult stock after rejected transfer = %v, want unchanged 50 (no deduction should have happened)", amount)
	}
}

// TestTrade_UnknownGoodStillRejected pins the pre-existing "unknown good"
// behaviour (regression guard for folding the weight lookup into
// economy.IsShippableGood) — a key that isn't in the goods catalog at all
// must give the generic message, not the cult-specific one.
func TestTrade_UnknownGoodStillRejected(t *testing.T) {
	f := setupTradeInternalFixture(t)

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade",
		map[string]any{"destination_id": f.destID.String(), "good_key": "unobtainium", "quantity": 10.0})

	if code != 400 {
		t.Fatalf("trade good_key=unobtainium returned %d: %v, want 400", code, resp)
	}
	msg, _ := resp["error"].(string)
	if msg != "unknown good" {
		t.Errorf("error message = %q, want exactly %q", msg, "unknown good")
	}
}
