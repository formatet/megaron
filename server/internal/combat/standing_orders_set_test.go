package combat

// KR3 §5 tests: SetStandingOrders, the mid-battle retreat-order setter.
//
// RED BEFORE this slice: SetStandingOrders did not exist — standing_orders
// had no write path at all (battle.go's own comment: "no player-facing SET
// path yet"). GREEN after: the tests below.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestSetStandingOrders_AppliesToActiveParticipant(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)

	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)

	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	hold := true
	res, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: defenderUnitID,
		HoldToLastMan: &hold,
	})
	if err != nil {
		t.Fatalf("SetStandingOrders: %v", err)
	}
	if res.BattleID != battleID {
		t.Errorf("BattleID = %v, want %v", res.BattleID, battleID)
	}
	if res.RetreatAtLoss != nil {
		t.Errorf("RetreatAtLoss = %v, want nil", res.RetreatAtLoss)
	}
	if !res.HoldToLastMan {
		t.Error("HoldToLastMan = false, want true")
	}

	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT standing_orders FROM battle_participants WHERE battle_id = $1 AND unit_id = $2`,
		battleID, defenderUnitID,
	).Scan(&raw); err != nil {
		t.Fatalf("read participant: %v", err)
	}
	var so standingOrdersFields
	if err := json.Unmarshal(raw, &so); err != nil {
		t.Fatalf("unmarshal standing_orders: %v", err)
	}
	if so.RetreatAtLoss != nil || !so.HoldToLastMan {
		t.Errorf("standing_orders row = %+v, want hold_to_last_man=true only", so)
	}

	var eventCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'StandingOrdersChanged'`, battleID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count StandingOrdersChanged events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("StandingOrdersChanged event count = %d, want 1", eventCount)
	}
}

func TestSetStandingOrders_ReplacesModeClearsLegacyThreshold(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)

	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)

	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	hold := true
	if _, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: defenderUnitID, HoldToLastMan: &hold,
	}); err != nil {
		t.Fatalf("first SetStandingOrders: %v", err)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE battle_participants SET standing_orders=standing_orders || '{"retreat_at_loss":0.2,"other":"keep"}'::jsonb WHERE battle_id=$1 AND unit_id=$2`, battleID, defenderUnitID); err != nil {
		t.Fatal(err)
	}
	res, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: defenderUnitID, ByLoyalty: true,
	})
	if err != nil {
		t.Fatalf("second SetStandingOrders: %v", err)
	}
	if res.HoldToLastMan {
		t.Error("loyalty replacement retained legacy hold")
	}
	if res.RetreatAtLoss != nil {
		t.Errorf("RetreatAtLoss = %v, want nil", res.RetreatAtLoss)
	}
	var raw []byte
	if err := pool.QueryRow(context.Background(), `SELECT standing_orders FROM battle_participants WHERE battle_id=$1 AND unit_id=$2`, battleID, defenderUnitID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["other"] != "keep" || stored["retreat_at_loss"] != nil || stored["hold_to_last_man"] != nil {
		t.Fatalf("mode replacement lost unrelated JSON or retained legacy fields: %s", raw)
	}
}

func TestSetStandingOrders_RejectsUnitNotInActiveBattle(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)

	var idleUnitID uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 50, 0, 'positioned', 9, 9) RETURNING id`,
		f.worldID, f.defender,
	).Scan(&idleUnitID); err != nil {
		t.Fatalf("create idle unit: %v", err)
	}

	_, err := SetStandingOrders(context.Background(), pool, events.NewStore(pool), StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: idleUnitID, ByLoyalty: true,
	})
	if err == nil {
		t.Fatal("SetStandingOrders on a unit with no active battle: want error, got nil")
	}
	rej, ok := err.(*OrderReject)
	if !ok {
		t.Fatalf("err = %T, want *OrderReject", err)
	}
	if rej.Status != 422 {
		t.Errorf("Status = %d, want 422", rej.Status)
	}
}

func TestSetStandingOrders_RejectsWrongOwner(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)

	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)

	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)

	_, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.attacker, UnitID: defenderUnitID, ByLoyalty: true,
	})
	if err == nil {
		t.Fatal("SetStandingOrders on someone else's unit: want error, got nil")
	}
	rej, ok := err.(*OrderReject)
	if !ok {
		t.Fatalf("err = %T, want *OrderReject", err)
	}
	if rej.Status != 403 {
		t.Errorf("Status = %d, want 403", rej.Status)
	}
}
