package combat

// Realm-wide retreat default (mig 145, retreat_default.go): it starts as
// "by loyalty" (today's behaviour), can be set and read back, refuses
// nonsense, is copied onto every participant row a Wanax's unit gets when it
// enters a battle (start AND join), and the mid-battle per-unit override
// still edits that copy without touching the default.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func joinWorld(t *testing.T, pool *pgxpool.Pool, worldID, playerID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO player_world_records (player_id, world_id, status) VALUES ($1, $2, 'active')`,
		playerID, worldID,
	); err != nil {
		t.Fatalf("join world: %v", err)
	}
}

func participantOrders(t *testing.T, pool *pgxpool.Pool, unitID uuid.UUID) standingOrdersFields {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT standing_orders FROM battle_participants WHERE unit_id = $1`, unitID,
	).Scan(&raw); err != nil {
		t.Fatalf("read participant %s: %v", unitID, err)
	}
	var so standingOrdersFields
	if err := json.Unmarshal(raw, &so); err != nil {
		t.Fatalf("unmarshal standing_orders %q: %v", raw, err)
	}
	return so
}

func f64(v float64) *float64 { return &v }

func TestRetreatDefault_FreshWanaxIsByLoyalty(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)
	joinWorld(t, pool, f.worldID, f.defender)

	d, err := LoadRetreatDefault(context.Background(), pool, f.worldID, f.defender)
	if err != nil {
		t.Fatalf("LoadRetreatDefault: %v", err)
	}
	if !d.ByLoyalty() {
		t.Errorf("fresh default = %+v, want by loyalty (no threshold, no hold)", d)
	}
}

func TestRetreatDefault_SetThenLoadAndValidation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	joinWorld(t, pool, f.worldID, f.defender)

	if _, err := SetRetreatDefault(ctx, pool, f.worldID, f.defender, RetreatDefaultOrder{RetreatAtLoss: f64(0.5)}); err != nil {
		t.Fatalf("set 0.5: %v", err)
	}
	d, err := LoadRetreatDefault(ctx, pool, f.worldID, f.defender)
	if err != nil || d.RetreatAtLoss == nil || *d.RetreatAtLoss != 0.5 || d.HoldToLastMan {
		t.Fatalf("after set 0.5: %+v, %v", d, err)
	}

	if _, err := SetRetreatDefault(ctx, pool, f.worldID, f.defender, RetreatDefaultOrder{HoldToLastMan: true}); err != nil {
		t.Fatalf("set hold: %v", err)
	}
	d, _ = LoadRetreatDefault(ctx, pool, f.worldID, f.defender)
	if !d.HoldToLastMan || d.RetreatAtLoss != nil {
		t.Fatalf("after set hold: %+v, want hold only (the old threshold must not linger)", d)
	}

	if _, err := SetRetreatDefault(ctx, pool, f.worldID, f.defender, RetreatDefaultOrder{ByLoyalty: true}); err != nil {
		t.Fatalf("set by loyalty: %v", err)
	}
	d, _ = LoadRetreatDefault(ctx, pool, f.worldID, f.defender)
	if !d.ByLoyalty() {
		t.Fatalf("after reset: %+v, want by loyalty", d)
	}

	bad := []struct {
		name string
		o    RetreatDefaultOrder
		want int
	}{
		{"nothing set", RetreatDefaultOrder{}, http.StatusBadRequest},
		{"two set", RetreatDefaultOrder{RetreatAtLoss: f64(0.5), HoldToLastMan: true}, http.StatusBadRequest},
		{"over 1", RetreatDefaultOrder{RetreatAtLoss: f64(1.5)}, http.StatusBadRequest},
		{"negative", RetreatDefaultOrder{RetreatAtLoss: f64(-0.1)}, http.StatusBadRequest},
	}
	for _, tc := range bad {
		_, err := SetRetreatDefault(ctx, pool, f.worldID, f.defender, tc.o)
		rej, ok := err.(*OrderReject)
		if !ok || rej.Status != tc.want {
			t.Errorf("%s: err = %v, want %d reject", tc.name, err, tc.want)
		}
	}
	// A refused write must leave the stored value alone.
	d, _ = LoadRetreatDefault(ctx, pool, f.worldID, f.defender)
	if !d.ByLoyalty() {
		t.Errorf("after refused writes: %+v, want still by loyalty", d)
	}

	// Not joined: no guessed default, a 404.
	_, err = SetRetreatDefault(ctx, pool, f.worldID, f.attacker, RetreatDefaultOrder{HoldToLastMan: true})
	if rej, ok := err.(*OrderReject); !ok || rej.Status != http.StatusNotFound {
		t.Errorf("set for a Wanax not in the world: err = %v, want 404", err)
	}
	_, err = LoadRetreatDefault(ctx, pool, f.worldID, f.attacker)
	if rej, ok := err.(*OrderReject); !ok || rej.Status != http.StatusNotFound {
		t.Errorf("load for a Wanax not in the world: err = %v, want 404", err)
	}
}

func TestRetreatDefault_SeedsParticipantsOnStartAndJoin(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	joinWorld(t, pool, f.worldID, f.attacker)
	joinWorld(t, pool, f.worldID, f.defender)

	if _, err := SetRetreatDefault(ctx, pool, f.worldID, f.attacker, RetreatDefaultOrder{RetreatAtLoss: f64(0.6)}); err != nil {
		t.Fatalf("set attacker default: %v", err)
	}
	if _, err := SetRetreatDefault(ctx, pool, f.worldID, f.defender, RetreatDefaultOrder{HoldToLastMan: true}); err != nil {
		t.Fatalf("set defender default: %v", err)
	}

	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	_ = loadBattleID(t, pool, f.worldID, 1, 0)

	if so := participantOrders(t, pool, attackerUnitID); so.RetreatAtLoss == nil || *so.RetreatAtLoss != 0.6 || so.HoldToLastMan {
		t.Errorf("attacker seeded with %+v, want retreat_at_loss=0.6", so)
	}
	if so := participantOrders(t, pool, defenderUnitID); !so.HoldToLastMan || so.RetreatAtLoss != nil {
		t.Errorf("defender seeded with %+v, want hold_to_last_man", so)
	}

	// A second attacker reaching the same hex JOINS the battle under way —
	// it is entering a battle too, and must carry the default.
	joinerID := mkFieldAttacker(t, pool, f, 40)
	runFieldArrival(t, pool, h, f.worldID, joinerID)
	if so := participantOrders(t, pool, joinerID); so.RetreatAtLoss == nil || *so.RetreatAtLoss != 0.6 {
		t.Errorf("joiner seeded with %+v, want retreat_at_loss=0.6", so)
	}
}

func TestRetreatDefault_OwnerWithoutRecordGetsByLoyalty(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool) // fixture players never join: no player_world_records row

	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)

	for _, id := range []uuid.UUID{attackerUnitID, defenderUnitID} {
		if so := participantOrders(t, pool, id); so.RetreatAtLoss != nil || so.HoldToLastMan {
			t.Errorf("unit %s seeded with %+v, want empty (by loyalty)", id, so)
		}
	}
}

func TestRetreatDefault_MidBattleOverrideEditsTheCopyOnly(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	joinWorld(t, pool, f.worldID, f.defender)
	if _, err := SetRetreatDefault(ctx, pool, f.worldID, f.defender, RetreatDefaultOrder{HoldToLastMan: true}); err != nil {
		t.Fatalf("set defender default: %v", err)
	}

	defenderUnitID := mkFieldDefender(t, pool, f, 60)
	attackerUnitID := mkFieldAttacker(t, pool, f, 62)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)

	hold := false
	if _, err := SetStandingOrders(ctx, pool, h.eventStore, StandingOrdersOrder{
		WorldID: f.worldID, PlayerID: f.defender, UnitID: defenderUnitID,
		RetreatAtLoss: f64(0.3), HoldToLastMan: &hold,
	}); err != nil {
		t.Fatalf("SetStandingOrders: %v", err)
	}
	if so := participantOrders(t, pool, defenderUnitID); so.HoldToLastMan || so.RetreatAtLoss == nil || *so.RetreatAtLoss != 0.3 {
		t.Errorf("after override: %+v, want retreat_at_loss=0.3 without hold", so)
	}
	d, _ := LoadRetreatDefault(ctx, pool, f.worldID, f.defender)
	if !d.HoldToLastMan {
		t.Errorf("realm default changed by a per-battle override: %+v, want still hold", d)
	}
}
