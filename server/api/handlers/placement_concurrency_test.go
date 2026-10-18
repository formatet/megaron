package handlers

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"testing"
	"time"
)

func TestPlacementConcurrentPOSTsSerialize(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population=100 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err = blocker.Exec(ctx, `SELECT 1 FROM map_tiles WHERE world_id=$1 AND q=1 AND r=0 FOR UPDATE`, f.worldID); err != nil {
		t.Fatal(err)
	}
	const n = 8
	start := make(chan struct{})
	results := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			<-start
			code, _ := f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
			results <- code
		}()
	}
	close(start)
	// Both versions must have all requests inside PostgreSQL before release.
	// Old code waits on the tile after choosing ordinal1; fixed code waits on
	// the settlement before even reading placements. No artificial response delay.
	deadline := time.Now().Add(8 * time.Second)
	for {
		var waiting int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only%d/%d requests entered DB lock barrier", waiting, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	counts := map[int]int{}
	for i := 0; i < n; i++ {
		select {
		case code := <-results:
			counts[code]++
		case <-time.After(10 * time.Second):
			t.Fatal("placement deadlock/timeout")
		}
	}
	if counts[201] != 1 || counts[409] != n-1 {
		t.Fatalf("one citizen must yield1x201 and7x409, never500: %v", counts)
	}
	var placed, maxOrdinal int
	if err = pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(gubbe_ordinal),0) FROM settlement_placement WHERE settlement_id=$1`, f.settlementID).Scan(&placed, &maxOrdinal); err != nil {
		t.Fatal(err)
	}
	if placed != 1 || maxOrdinal != 1 {
		t.Fatalf("pool overflow %d max%d", placed, maxOrdinal)
	}
	t.Log(fmt.Sprintf("parallel HTTP statuses=%v, placed=%d", counts, placed))
}

func TestPlacementUsesPopulationAfterSettlementLock(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains", {-1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()
	if code, body := f.do(t, "POST", f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"}); code != 201 {
		t.Fatalf("%d %v", code, body)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE settlements SET population=100 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		code, _ := f.do(t, "POST", f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": -1, "hex_r": 0, "good_key": "grain"})
		result <- code
	}()
	waitPlacementLock(t, pool)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != 409 {
			t.Fatalf("stale population allowed placement: HTTP%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("placement timeout")
	}
}

func TestPlacementDELETEWaitsBeforeLockingPlacement(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()
	if code, _ := f.do(t, "POST", f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"}); code != 201 {
		t.Fatal(code)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM settlements WHERE id=$1 FOR UPDATE`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() { code, _ := f.do(t, "DELETE", f.placementsPath()+"/1", nil); result <- code }()
	waitPlacementLock(t, pool)
	observer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Rollback(ctx)
	if _, err = observer.Exec(ctx, `SELECT id FROM settlement_placement WHERE settlement_id=$1 FOR UPDATE NOWAIT`, f.settlementID); err != nil {
		t.Fatalf("DELETE locked child placements before the settlement: %v", err)
	}
	_ = observer.Rollback(ctx)
	_ = tx.Rollback(ctx)
	select {
	case code := <-result:
		if code != 200 {
			t.Fatal(code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DELETE timeout")
	}
}

func waitPlacementLock(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("request never reached lock barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPlacementTakeBusyHolderReturnsConflict(t *testing.T) {
	f := setupTwoSettlementHexFixture(t, "plains", [2]int{takeHexQ, takeHexR})
	pool := p10TestPool(t)
	ctx := context.Background()
	f.aHoldsHex(t, 1)
	f.unit(t, f.owner(t, f.settlementB), "positioned", str("sentry"))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM settlements WHERE id=$1 FOR UPDATE`, f.settlementA); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() { code, _ := f.place(t, f.tokenB, f.provinceB, "grain"); result <- code }()
	select {
	case code := <-result:
		if code != 409 {
			t.Fatalf("busy take HTTP%d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("take waited for holder while holding taker — lock cycle possible")
	}
	if f.placementCount(t, f.settlementA) != 1 || f.placementCount(t, f.settlementB) != 0 {
		t.Fatal("busy take partially changed placements")
	}
	_ = tx.Rollback(ctx)
	if code, body := f.place(t, f.tokenB, f.provinceB, "grain"); code != 201 {
		t.Fatalf("retry after unlock %d %v", code, body)
	}
}
