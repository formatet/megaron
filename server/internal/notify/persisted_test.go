package notify

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeliverPersistedTargetsOnlyRecipient(t *testing.T) {
	h := New()
	world, owner := uuid.New(), uuid.New()
	target := &client{worldID: world, playerID: owner, send: make(chan []byte, 1)}
	other := &client{worldID: world, playerID: uuid.New(), send: make(chan []byte, 1)}
	otherWorld := &client{worldID: uuid.New(), playerID: owner, send: make(chan []byte, 1)}
	anonymous := &client{worldID: world, playerID: uuid.Nil, send: make(chan []byte, 1)}
	for _, c := range []*client{target, other, otherWorld, anonymous} {
		h.clients[c] = struct{}{}
	}
	msg := Msg{Kind: "agora_ready", ID: uuid.NewString(), Level: 4, Payload: map[string]string{"user_id": "@agamemnon:agora.test"}}
	h.DeliverPersisted(context.Background(), world, owner, msg)
	if len(target.send) != 1 || len(other.send) != 0 || len(otherWorld.send) != 0 || len(anonymous.send) != 0 {
		t.Fatal("personal persisted notification leaked or missing")
	}
	var got Msg
	if json.Unmarshal(<-target.send, &got) != nil || got.ID != msg.ID || got.Level != 4 {
		t.Fatal("committed archive identity lost")
	}
	h.DeliverPersisted(context.Background(), world, uuid.Nil, msg)
	msg.ID = ""
	h.DeliverPersisted(context.Background(), world, owner, msg)
	if len(target.send) != 0 || len(anonymous.send) != 0 {
		t.Fatal("unpersisted or anonymous personal notification delivered")
	}
}

func TestDeliverPersistedHonorsMuteWithoutReinserting(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var world, owner, id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES($1,'archived') RETURNING id`, "agora-notify-"+uuid.NewString()).Scan(&world); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "agora-notify-"+uuid.NewString()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, world)
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id=$1`, owner)
	}()
	if err := pool.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level) VALUES($1,$2,'agora_ready',4) RETURNING id`, world, owner).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_mutes(player_id,kind) VALUES($1,'agora_ready')`, owner); err != nil {
		t.Fatal(err)
	}
	h := New()
	h.SetPool(pool)
	target := &client{worldID: world, playerID: owner, send: make(chan []byte, 2)}
	h.clients[target] = struct{}{}
	msg := Msg{Kind: "agora_ready", ID: id.String(), Level: 4}
	h.DeliverPersisted(ctx, world, owner, msg)
	if len(target.send) != 0 {
		t.Fatal("muted notification pushed")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM dispatch_mutes WHERE player_id=$1`, owner); err != nil {
		t.Fatal(err)
	}
	h.DeliverPersisted(ctx, world, owner, msg)
	if len(target.send) != 1 {
		t.Fatal("unmuted persisted notification not pushed")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1`, owner).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("delivery reinserted archive row: %d", count)
	}
}
