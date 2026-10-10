package handlers

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// megaron_plan_cedar_virke.md (Timothy 2026-10-10): cedar covers a timber shortfall 1:1
// in every cost paid through deductGoods (building, recruiting, ship repair).

func setStock(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, good string, amount float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, calc_tick)
		 VALUES ($1, $2, $3, 0, current_world_tick())
		 ON CONFLICT (settlement_id, good_key)
		 DO UPDATE SET amount = $3, rate = 0, calc_tick = current_world_tick()`,
		sid, good, amount); err != nil {
		t.Fatalf("set %s stock: %v", good, err)
	}
}

func stockOf(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, good string) float64 {
	t.Helper()
	var have float64
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(settled(amount, rate, calc_tick), 0) FROM settlement_goods
		  WHERE settlement_id = $1 AND good_key = $2`, sid, good).Scan(&have); err != nil {
		t.Fatalf("read %s stock: %v", good, err)
	}
	return have
}

func payCosts(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, costs map[string]float64) error {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := deductGoods(ctx, tx, sid, costs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestDeductGoods_CedarCoversMissingTimber(t *testing.T) {
	pool, sid := foundMetropolisFixture(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"})
	setStock(t, pool, sid, "timber", 2)
	setStock(t, pool, sid, "cedar", 4)

	if err := payCosts(t, pool, sid, map[string]float64{"timber": 6}); err != nil {
		t.Fatalf("6 timber with 2 timber + 4 cedar must be payable: %v", err)
	}
	if got := stockOf(t, pool, sid, "timber"); !near(got, 0) {
		t.Errorf("timber after pay = %v, want 0", got)
	}
	if got := stockOf(t, pool, sid, "cedar"); !near(got, 0) {
		t.Errorf("cedar after pay = %v, want 0", got)
	}
}

func TestDeductGoods_TimberPaidFirstCedarUntouched(t *testing.T) {
	pool, sid := foundMetropolisFixture(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"})
	setStock(t, pool, sid, "timber", 10)
	setStock(t, pool, sid, "cedar", 5)

	if err := payCosts(t, pool, sid, map[string]float64{"timber": 6}); err != nil {
		t.Fatal(err)
	}
	if got := stockOf(t, pool, sid, "cedar"); !near(got, 5) {
		t.Errorf("cedar = %v, want 5: enough timber must not spend cedar", got)
	}
}

func TestDeductGoods_InsufficientPotListsTimberWithPot(t *testing.T) {
	pool, sid := foundMetropolisFixture(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"})
	setStock(t, pool, sid, "timber", 2)
	setStock(t, pool, sid, "cedar", 3)

	err := payCosts(t, pool, sid, map[string]float64{"timber": 6})
	var ins *insufficientGoodsError
	if !errors.As(err, &ins) {
		t.Fatalf("want insufficientGoodsError, got %v", err)
	}
	if len(ins.Short) != 1 || ins.Short[0].Good != "timber" || !near(ins.Short[0].Have, 5) {
		t.Errorf("short = %+v, want timber with have 5 (2 timber + 3 cedar)", ins.Short)
	}
	if !near(stockOf(t, pool, sid, "timber"), 2) || !near(stockOf(t, pool, sid, "cedar"), 3) {
		t.Errorf("a refused payment must deduct nothing")
	}
}

func TestDeductGoods_OwnCedarCostIsNotSpentOnTimber(t *testing.T) {
	// War chariot style cost: timber AND cedar. Cedar owed as cedar is not also
	// available to cover timber.
	pool, sid := foundMetropolisFixture(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"})
	setStock(t, pool, sid, "timber", 0)
	setStock(t, pool, sid, "cedar", 1)

	err := payCosts(t, pool, sid, map[string]float64{"timber": 1, "cedar": 1})
	var ins *insufficientGoodsError
	if !errors.As(err, &ins) {
		t.Fatalf("want insufficientGoodsError, got %v", err)
	}
	if got := stockOf(t, pool, sid, "cedar"); !near(got, 1) {
		t.Errorf("cedar = %v, want 1 untouched", got)
	}
}

func TestDeductGoods_ChariotPaidWithCedarOnly(t *testing.T) {
	pool, sid := foundMetropolisFixture(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"})
	setStock(t, pool, sid, "timber", 0)
	setStock(t, pool, sid, "cedar", 10)

	// per man: timber 0.08, cedar 0.08 — 50 men
	if err := payCosts(t, pool, sid, map[string]float64{"timber": 4, "cedar": 4}); err != nil {
		t.Fatalf("a Wanax with cedar and no timber must afford chariots: %v", err)
	}
	if got := stockOf(t, pool, sid, "cedar"); !near(got, 2) {
		t.Errorf("cedar = %v, want 2 (10 - 4 own - 4 covering timber)", got)
	}
}
