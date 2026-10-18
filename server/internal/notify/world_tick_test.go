package notify

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNotificationWorldTickSurvivesDelayedDelivery(t *testing.T) {
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
	var world, owner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,current_tick) VALUES('notification-tick','archived',31) RETURNING id`).Scan(&world); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "tick-"+uuid.NewString()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, world)
		pool.Exec(ctx, `DELETE FROM players WHERE id=$1`, owner)
	}()
	h := New()
	h.SetPool(pool)
	conn, closeConn := dialHub(t, h, world, owner)
	defer closeConn()
	if !waitFor(t, 2*time.Second, func() bool { return clientCount(h) == 1 }) {
		t.Fatal("client never registered")
	}
	if err := h.NotifyPlayer(ctx, world, owner, "BuildComplete", 3, map[string]string{"name": "Phaistos"}); err != nil {
		t.Fatal(err)
	}
	msg := readMsg(t, conn)
	if msg.WorldTick == nil || *msg.WorldTick != 31 {
		t.Fatalf("live notification lost recorded tick31: %+v", msg)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=99 WHERE id=$1`, world); err != nil {
		t.Fatal(err)
	}
	h.DeliverCommittedNotification(ctx, world, owner, msg.ID, msg.Kind, msg.Level, msg.Payload)
	replay := readMsg(t, conn)
	if replay.WorldTick == nil || *replay.WorldTick != 31 || replay.ID != msg.ID {
		t.Fatalf("delayed delivery borrowed current tick99: %+v", replay)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1`, world).Scan(&count); err != nil || count != 1 {
		t.Fatalf("delivery reinserted row count%d err%v", count, err)
	}
	// Day zero is a known day, not missing information.
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=0 WHERE id=$1`, world); err != nil {
		t.Fatal(err)
	}
	if err := h.NotifyPlayer(ctx, world, owner, "BuildComplete", 3, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	zero := readMsg(t, conn)
	if zero.WorldTick == nil || *zero.WorldTick != 0 {
		t.Fatalf("day zero dropped: %+v", zero)
	}
}
