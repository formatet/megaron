package combat

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRetreatTwoModesMigrationNormalizesLegacyPreservesHoldAndLetters(t *testing.T) {
	pool := testPool(t)
	f := newBattleFixture(t, pool)
	ctx := context.Background()
	var battle uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO battles(world_id,q,r,started_tick,current_tick,status,seed) VALUES($1,8,0,0,0,'active',1) RETURNING id`, f.worldID).Scan(&battle); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ before, want string }{{`{"retreat_at_loss":0.5,"other":"keep"}`, `{"other":"keep"}`}, {`{"hold_to_last_man":true,"other":7}`, `{"hold_to_last_man":true,"other":7}`}, {`{}`, `{}`}, {`{"retreat_at_loss":0.75,"hold_to_last_man":true,"other":"held"}`, `{"hold_to_last_man":true,"other":"held"}`}}
	var players, units []uuid.UUID
	for _, c := range cases {
		var p, u uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "retreat-mig-"+uuid.New().String()).Scan(&p); err != nil {
			t.Fatal(err)
		}
		joinWorld(t, pool, f.worldID, p)
		if _, err := pool.Exec(ctx, `UPDATE player_world_records SET retreat_default=$3 WHERE world_id=$1 AND player_id=$2`, f.worldID, p, c.before); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,status,q,r) VALUES($1,$2,'spearman','land',100,'positioned',8,0) RETURNING id`, f.worldID, p).Scan(&u); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO battle_participants(battle_id,unit_id,owner_id,side,joined_tick,initial_size,current_size,standing_orders) VALUES($1,$2,$3,'attacker',0,100,100,$4)`, battle, u, p, c.before); err != nil {
			t.Fatal(err)
		}
		players = append(players, p)
		units = append(units, u)
	}
	sealed := `{"verb":"standing_orders","standing_orders":{"RetreatAtLoss":0.75,"HoldToLastMan":false}}`
	var letter uuid.UUID
	var scheduled int64
	if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_unit_id,origin_q,origin_r,kind,status,message_text,hex_q,hex_r,arrives_at,order_payload) VALUES($1,$2,$4,8,0,'order','outbound','sealed',0,0,now(),$3) RETURNING id`, f.worldID, players[0], sealed, units[0]).Scan(&letter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES($1,'OrderDelivery',$2,now(),1) RETURNING id`, f.worldID, sealed).Scan(&scheduled); err != nil {
		t.Fatal(err)
	}
	var letterBefore, queueBefore []byte
	_ = pool.QueryRow(ctx, `SELECT order_payload FROM messengers WHERE id=$1`, letter).Scan(&letterBefore)
	_ = pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE id=$1`, scheduled).Scan(&queueBefore)
	migration, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "173_retreat_two_modes.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		for _, where := range []struct {
			sql string
			id  uuid.UUID
		}{{`SELECT retreat_default FROM player_world_records WHERE player_id=$1`, players[i]}, {`SELECT standing_orders FROM battle_participants WHERE unit_id=$1`, units[i]}} {
			var raw []byte
			if err := pool.QueryRow(ctx, where.sql, where.id).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var got, want any
			_ = json.Unmarshal(raw, &got)
			_ = json.Unmarshal([]byte(c.want), &want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("migration changed wrong mode/unrelated data: before=%s got=%s want=%s", c.before, raw, c.want)
			}
		}
	}
	var letterAfter, queueAfter []byte
	_ = pool.QueryRow(ctx, `SELECT order_payload FROM messengers WHERE id=$1`, letter).Scan(&letterAfter)
	_ = pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE id=$1`, scheduled).Scan(&queueAfter)
	if string(letterAfter) != string(letterBefore) || string(queueAfter) != string(queueBefore) {
		t.Fatal("migration rewrote a sealed retreat letter")
	}
}
func TestRetreatTwoModesDomainRejectsPercentageEvenWithValidMode(t *testing.T) {
	loss := .5
	hold := true
	if rej := ValidateRetreatMode(&loss, true, false); rej == nil || rej.Status != 400 {
		t.Fatal("retreat_at_loss domain guard missing")
	}
	if _, err := SetStandingOrders(context.Background(), nil, nil, StandingOrdersOrder{RetreatAtLoss: &loss, HoldToLastMan: &hold}); err == nil {
		t.Fatal("per-unit setter accepted fixed retreat percentage")
	}
}
