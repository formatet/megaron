package combat

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// Exercise the real tick handler and persisted participation, including fortify
// and different realm balances. Neither city nor realm state scales commitment.
func TestBattleParticipationFixedAcrossCityAndKharis(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	for _, balance := range []int{1, 100} {
		f := newBattleFixture(t, pool)
		if _, err := pool.Exec(ctx, `INSERT INTO player_world_records (player_id,world_id,kharis_amount) VALUES ($1,$3,$4),($2,$3,$4)`, f.attacker, f.defender, f.worldID, balance); err != nil {
			t.Fatal(err)
		}
		if balance == 1 {
			// An army without a capital must still commit the same fraction.
			if _, err := pool.Exec(ctx, `UPDATE settlements SET is_capital=false WHERE world_id=$1`, f.worldID); err != nil {
				t.Fatal(err)
			}
		}
		defender := mkFieldDefender(t, pool, f, 100)
		if _, err := pool.Exec(ctx, `UPDATE units SET stance='fortify' WHERE id=$1`, defender); err != nil {
			t.Fatal(err)
		}
		attacker := mkFieldAttacker(t, pool, f, 100)
		h := newArrivalHandler(pool, &sequenceDice{ints: []int{424243, 909091}})
		runFieldArrival(t, pool, h, f.worldID, attacker)
		battle := loadBattleID(t, pool, f.worldID, 1, 0)
		runBattleToEnd(t, pool, NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk), f.worldID, battle, 200)
		rounds := loadBattleRounds(t, pool, battle)
		if len(rounds) == 0 {
			t.Fatal("no battle rounds resolved")
		}
		for _, raw := range []json.RawMessage{rounds[0].attacker, rounds[0].defender} {
			var result BattleSideRoundResult
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.ActiveCombatants != 90 || result.DiceRolled != 45 {
				t.Errorf("first round kharis%d result=%+v want90 active and45 dice", balance, result)
			}
		}
		rows, err := pool.Query(ctx, `SELECT side,last_participation FROM battle_participants WHERE battle_id=$1`, battle)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for rows.Next() {
			var side string
			var value float64
			if err := rows.Scan(&side, &value); err != nil {
				t.Fatal(err)
			}
			count++
			if value != .90 {
				t.Errorf("kharis=%d %s participation=%g, want .90", balance, side, value)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("participants=%d", count)
		}
	}
}

func TestFixedRoutThresholdAndHistoricalStandingOrders(t *testing.T) {
	for _, raw := range []string{`{}`, `{"by_loyalty":true}`, `{"when_losing":true}`} {
		for _, tc := range []struct {
			remaining int
			want      bool
		}{{21, false}, {20, true}, {19, true}, {0, false}} {
			if got := sideRouts([]int{0}, []int{100}, []int{tc.remaining}, [][]byte{[]byte(raw)}); got != tc.want {
				t.Errorf("orders=%s remaining=%d rout=%v want %v", raw, tc.remaining, got, tc.want)
			}
		}
	}
	if sideRouts([]int{0}, []int{100}, []int{1}, [][]byte{[]byte(`{"hold_to_last_man":true}`)}) {
		t.Error("hold order routed")
	}
}

func TestHistoricalStandingOrdersModeRename(t *testing.T) {
	for _, raw := range []string{`{"ByLoyalty":true}`, `{"by_loyalty":true}`, `{"when_losing":true}`} {
		var order StandingOrdersOrder
		if err := json.Unmarshal([]byte(raw), &order); err != nil {
			t.Fatal(err)
		}
		if !order.WhenLosing {
			t.Fatalf("historical order %s lost its mode", raw)
		}
		out, err := json.Marshal(order)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(bytes.ToLower(out), []byte("loyalty")) || !bytes.Contains(out, []byte(`"when_losing":true`)) {
			t.Errorf("noncanonical order %s", out)
		}
	}
}

func TestUnpaidUpkeepHasFixedDesertionStep(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "fixed-desertion")
	seedGoods(t, pool, f.capitalID, f.tick, 100000, 0)
	seedGoods(t, pool, f.townID, f.tick, 100000, 0)
	for _, sid := range []uuid.UUID{f.capitalID, f.townID} {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,settlement_id,support_settlement_id,unpaid_periods) VALUES($1,$2,'elite_infantry','land',50,'garrison',$3,$3,$4) RETURNING id`, f.worldID, f.owner, sid, upkeepDesertionTicks-1).Scan(&id); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Now())
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), &fakeBroadcaster{}, nil, events.NewScheduler(pool, clk).Clock())

	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM units WHERE world_id=$1 AND size=$2 AND unpaid_periods=$3`, f.worldID, 50-upkeepDesertionStep, upkeepDesertionTicks).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("units with fixed baseline desertion=%d want2", count)
	}
}
