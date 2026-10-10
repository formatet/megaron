package main

// Erasure anonymises the login, clears the player's private rows, releases
// their cities as abandon does, keeps the Wanax name and hands Agora to the
// server loop. A dry run changes nothing. DB test (DATABASE_URL).

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestErasePlayer(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	var world, player, province, city, garrison uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES($1,'archived') RETURNING id`, "erase-"+uuid.NewString()).Scan(&world))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, world) })
	username := "erase-" + uuid.NewString()
	wanaxName := "Nestor-" + uuid.NewString()[:8]
	localpart := "nestor" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	must(pool.QueryRow(ctx,
		`INSERT INTO players(username,wanax_name,password_hash,agora_state,agora_localpart,agora_claim_localpart)
		 VALUES($1,$3,'x','ready',$2,$2) RETURNING id`, username, localpart, wanaxName).Scan(&player))
	must(pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type,territory_state) VALUES($1,0,0,'plains','owned') RETURNING id`, world).Scan(&province))
	must(pool.QueryRow(ctx,
		`INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,is_capital,state)
		 VALUES($1,$2,'Pylos','minoan',$3,true,'active') RETURNING id`, world, province, player).Scan(&city))
	must(pool.QueryRow(ctx,
		`INSERT INTO units(world_id,owner_id,type,category,size,crew,status,settlement_id)
		 VALUES($1,$2,'spearman','land',100,0,'garrison',$3) RETURNING id`, world, player, city).Scan(&garrison))
	_, err = pool.Exec(ctx, `INSERT INTO notifications (world_id, player_id, kind, level, body_json) VALUES ($1,$2,'BuildComplete',4,'{}')`, world, player)
	must(err)
	_, err = pool.Exec(ctx, `INSERT INTO player_world_records (player_id, world_id, settlement_id, status) VALUES ($1,$2,$3,'active')`, player, world, city)
	must(err)

	// Dry run: reports, changes nothing.
	rep, err := erasePlayer(ctx, pool, username, false)
	must(err)
	if rep.cities != 1 || rep.notifications != 1 || !rep.agora || rep.wanax != wanaxName {
		t.Fatalf("dry run report = %+v, want 1 city, 1 notification, chat account, Wanax Nestor", rep)
	}
	var still string
	must(pool.QueryRow(ctx, `SELECT username FROM players WHERE id=$1`, player).Scan(&still))
	if still != username {
		t.Fatalf("dry run changed the username to %q", still)
	}

	rep, err = erasePlayer(ctx, pool, username, true)
	must(err)

	var newName, hash, wanax string
	var erased, pending bool
	must(pool.QueryRow(ctx,
		`SELECT username, password_hash, COALESCE(wanax_name,''), erased_at IS NOT NULL, agora_deactivate_pending FROM players WHERE id=$1`,
		player).Scan(&newName, &hash, &wanax, &erased, &pending))
	if !strings.HasPrefix(newName, "erased-") || newName != rep.newUsername || hash != "!erased" {
		t.Errorf("login after erase = %q / %q, want erased-… and an unusable hash", newName, hash)
	}
	if wanax != wanaxName {
		t.Errorf("wanax_name = %q, want %q kept", wanax, wanaxName)
	}
	if !erased || !pending {
		t.Errorf("erased=%v pending=%v, want both true (the server loop deactivates the chat account)", erased, pending)
	}
	var owner *uuid.UUID
	var state, territory, unitStatus, record string
	must(pool.QueryRow(ctx, `SELECT owner_id, state FROM settlements WHERE id=$1`, city).Scan(&owner, &state))
	must(pool.QueryRow(ctx, `SELECT territory_state FROM provinces WHERE id=$1`, province).Scan(&territory))
	must(pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, garrison).Scan(&unitStatus))
	must(pool.QueryRow(ctx, `SELECT status FROM player_world_records WHERE player_id=$1`, player).Scan(&record))
	if owner != nil || state != "abandoned" || territory != "free" || unitStatus != "disbanded" || record != "departed" {
		t.Errorf("after erase: owner=%v state=%q territory=%q garrison=%q record=%q; want ownerless abandoned city, free hex, disbanded garrison, departed",
			owner, state, territory, unitStatus, record)
	}
	var notes int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1`, player).Scan(&notes))
	if notes != 0 {
		t.Errorf("notifications left: %d, want 0", notes)
	}

	if _, err := erasePlayer(ctx, pool, newName, true); !errors.Is(err, errAlreadyErased) {
		t.Errorf("second erase = %v, want errAlreadyErased", err)
	}
}
