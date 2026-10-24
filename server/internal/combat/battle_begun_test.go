package combat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestBattleBegunRecipientsAndRetry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE players SET wanax_name='Idomeneus' WHERE id=$1`, f.attacker); err != nil {
		t.Fatal(err)
	}
	a, d := mkFieldAttacker(t, pool, f, 100), mkFieldDefender(t, pool, f, 100)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{42}})
	h.clk = clock.NewTestClock(time.Date(2026, 10, 24, 18, 45, 0, 0, time.UTC))
	att := battleParticipant{unitID: a, ownerID: f.attacker, utype: "spearman", side: "attacker", currentSize: 100}
	def := battleParticipant{unitID: d, ownerID: f.defender, utype: "spearman", side: "defender", currentSize: 100}
	// A rollback must leave neither battle nor notice.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.initiateOrJoinBattle(ctx, tx, f.worldID, 1, 0, att, []battleParticipant{def}); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='BattleBegun'`, f.worldID).Scan(&n)
	if n != 0 {
		t.Fatalf("rollback left %d notices", n)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = h.initiateOrJoinBattle(ctx, tx, f.worldID, 1, 0, att, []battleParticipant{def}); err != nil {
		t.Fatal(err)
	}
	if err = h.initiateOrJoinBattle(ctx, tx, f.worldID, 1, 0, att, []battleParticipant{def}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT player_id,level,body_json FROM notifications WHERE world_id=$1 AND kind='BattleBegun'`, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n = 0
	for rows.Next() {
		n++
		var owner uuid.UUID
		var level int
		var raw []byte
		if err = rows.Scan(&owner, &level, &raw); err != nil {
			t.Fatal(err)
		}
		var b map[string]any
		if err = json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		if owner != f.defender || level != 2 || b["opponent_name"] != "Idomeneus" || b["q"] != float64(1) || b["r"] != float64(0) || b["battle_id"] == nil || b["occurred_at"] != "2026-10-24T18:45:00Z" {
			t.Fatalf("wrong recipient/notice: %s %d %s", owner, level, raw)
		}
		for _, key := range []string{"enemy_unit", "origin", "target", "stance", "seed"} {
			if _, ok := b[key]; ok {
				t.Fatalf("private/excess field %s", key)
			}
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("BattleBegun count=%d, want one defender notice after retry", n)
	}
}

func TestBattleBegunMultipleOwnersCityAndNewDefender(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{42}})
	a, d := mkFieldAttacker(t, pool, f, 100), mkFieldDefender(t, pool, f, 100)
	var cityOwner, otherOwner uuid.UUID
	for i, ptr := range []*uuid.UUID{&cityOwner, &otherOwner} {
		if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash,wanax_name) VALUES($1,'x',$2) RETURNING id`, uuid.NewString(), []string{"Nestor", "Myrine"}[i]).Scan(ptr); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE world_id=$1 AND owner_id=$3`, f.worldID, cityOwner, f.defender); err != nil {
		t.Fatal(err)
	}
	// City owner has no unit in this fight; unit owner is a separate Wanax.
	att := battleParticipant{unitID: a, ownerID: f.attacker, utype: "spearman", side: "attacker", currentSize: 100}
	def := battleParticipant{unitID: d, ownerID: f.defender, utype: "spearman", side: "defender", currentSize: 100}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = h.initiateOrJoinBattle(ctx, tx, f.worldID, 5, 5, att, []battleParticipant{def}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(DISTINCT player_id) FROM notifications WHERE world_id=$1 AND kind='BattleBegun'`, f.worldID).Scan(&n)
	if n != 2 {
		t.Fatalf("initial recipients=%d, want city and army owners", n)
	}
	// Reading the notice does not reset deduplication. Another unit belonging
	// to the same Wanax and a new Wanax join the already active battle.
	if _, err = pool.Exec(ctx, `UPDATE notifications SET read_at=now() WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []uuid.UUID{f.defender, otherOwner} {
		var u uuid.UUID
		if err = pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,'spearman','land',50,'positioned',5,5) RETURNING id`, f.worldID, owner).Scan(&u); err != nil {
			t.Fatal(err)
		}
		tx, err = pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		part := battleParticipant{unitID: u, ownerID: owner, utype: "spearman", side: "defender", currentSize: 50}
		if err = h.initiateOrJoinBattle(ctx, tx, f.worldID, 5, 5, part, nil); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='BattleBegun'`, f.worldID).Scan(&n)
	if n != 3 {
		t.Fatalf("recipients after reinforcements=%d, want three once each", n)
	}
	var joined bool
	if err = pool.QueryRow(ctx, `SELECT (body_json->>'joined')::boolean FROM notifications WHERE world_id=$1 AND player_id=$2 AND kind='BattleBegun'`, f.worldID, otherOwner).Scan(&joined); err != nil || !joined {
		t.Fatalf("new defender joined=%v err=%v", joined, err)
	}
}

type begunDeliverySpy struct {
	fakeBroadcaster
	check func(uuid.UUID, string)
}

func (s *begunDeliverySpy) DeliverCommittedNotification(_ context.Context, _ uuid.UUID, owner uuid.UUID, id, kind string, _ int, _ any) {
	if kind == "BattleBegun" {
		s.check(owner, id)
	}
}

func TestBattleBegunArrivalDeliversOnlyAfterCommit(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newBattleFixture(t, pool)
	a := mkFieldAttacker(t, pool, f, 100)
	mkFieldDefender(t, pool, f, 100)
	h := newArrivalHandler(pool, &sequenceDice{ints: []int{42}})
	deliveries := 0
	h.hub = &begunDeliverySpy{check: func(owner uuid.UUID, id string) {
		deliveries++
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications n JOIN battles b ON b.id=(n.body_json->>'battle_id')::uuid WHERE n.id=$1 AND n.player_id=$2 AND b.status='active'`, id, owner).Scan(&n); err != nil || n != 1 {
			t.Fatalf("delivery before committed battle+notice: %d %v", n, err)
		}
	}}
	payload, _ := json.Marshal(map[string]any{"unit_id": a, "world_id": f.worldID})
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 {
		t.Fatalf("deliveries=%d, want one", deliveries)
	}
}
