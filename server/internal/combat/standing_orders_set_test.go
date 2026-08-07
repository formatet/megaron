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

	loss := 0.75
	hold := true
	res, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: defenderUnitID,
		RetreatAtLoss: &loss, HoldToLastMan: &hold,
	})
	if err != nil {
		t.Fatalf("SetStandingOrders: %v", err)
	}
	if res.BattleID != battleID {
		t.Errorf("BattleID = %v, want %v", res.BattleID, battleID)
	}
	if res.RetreatAtLoss == nil || *res.RetreatAtLoss != 0.75 {
		t.Errorf("RetreatAtLoss = %v, want 0.75", res.RetreatAtLoss)
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
	if so.RetreatAtLoss == nil || *so.RetreatAtLoss != 0.75 || !so.HoldToLastMan {
		t.Errorf("standing_orders row = %+v, want retreat_at_loss=0.75 hold_to_last_man=true", so)
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

func TestSetStandingOrders_PartialUpdatePreservesOtherField(t *testing.T) {
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

	loss := 0.2
	res, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: defenderUnitID, RetreatAtLoss: &loss,
	})
	if err != nil {
		t.Fatalf("second SetStandingOrders: %v", err)
	}
	if !res.HoldToLastMan {
		t.Error("HoldToLastMan reverted to false — setting retreat_at_loss alone must not clobber it")
	}
	if res.RetreatAtLoss == nil || *res.RetreatAtLoss != 0.2 {
		t.Errorf("RetreatAtLoss = %v, want 0.2", res.RetreatAtLoss)
	}
	_ = battleID
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

	loss := 0.5
	_, err := SetStandingOrders(context.Background(), pool, events.NewStore(pool), StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: idleUnitID, RetreatAtLoss: &loss,
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

	loss := 0.5
	_, err := SetStandingOrders(context.Background(), pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.attacker, UnitID: defenderUnitID, RetreatAtLoss: &loss,
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
