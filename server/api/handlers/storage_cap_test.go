package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/events"
)

func TestDefaultGoodStorageCapMetropolis(t *testing.T) {
	terrains := [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"}
	pool, sid := foundMetropolisFixture(t, terrains)
	var cap float64
	if err := pool.QueryRow(context.Background(), `SELECT cap FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, sid).Scan(&cap); err != nil {
		t.Fatal(err)
	}
	if cap != 1_000_000 {
		t.Fatalf("metropolis cedar cap=%g want unchanged 1000000", cap)
	}
}

func TestDefaultGoodStorageCapLogistics(t *testing.T) {
	terrains := [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"}
	pool, sid := foundMetropolisFixture(t, terrains)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, sid); err != nil {
		t.Fatal(err)
	}
	h := NewLogisticsArrivalHandler(pool)
	id := time.Now().UnixNano()
	deliver := func(n int64) {
		payload, _ := json.Marshal(map[string]any{"kind": "settlement_good", "destination": sid, "good_key": "cedar", "quantity": 5})
		if err := h.Handle(ctx, events.ScheduledEvent{ID: id + n, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	deliver(0)
	var cap, amount float64
	if err := pool.QueryRow(ctx, `SELECT cap,amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, sid).Scan(&cap, &amount); err != nil {
		t.Fatal(err)
	}
	if cap != 1_000_000 || amount != 5 {
		t.Fatalf("new logistics row cap=%g amount=%g", cap, amount)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET cap=7 WHERE settlement_id=$1 AND good_key='cedar'`, sid); err != nil {
		t.Fatal(err)
	}
	deliver(1)
	if err := pool.QueryRow(ctx, `SELECT cap,amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='cedar'`, sid).Scan(&cap, &amount); err != nil {
		t.Fatal(err)
	}
	if cap != 7 || amount != 7 {
		t.Fatalf("existing logistics row cap=%g amount=%g want 7/7", cap, amount)
	}
}
