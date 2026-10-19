package main

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type agoraExemptTrace struct {
	playerKey int64
	locks     atomic.Int32
}

func (t *agoraExemptTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "SELECT pg_advisory_lock") && len(d.Args) > 0 && d.Args[0] == t.playerKey {
		t.locks.Add(1)
	}
	return ctx
}
func (*agoraExemptTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestAgoraExemptPassSkipsReconcile(t *testing.T) {
	f := newAgoraFixture(t, "Exempt-"+uuid.NewString(), 4)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE players SET agora_exempt=true WHERE id=$1`, f.player); err != nil {
		t.Fatal(err)
	}
	trace := &agoraExemptTrace{playerKey: agoraLockKey("player:" + f.player.String())}
	config := f.pool.Config()
	config.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	remote := newFakeAgoraRemote()
	s := &agoraService{pool: pool, remote: remote, room: "!exempt"}
	s.pass(ctx)
	s.pass(ctx)
	// pass walks every player in the DB; the lock trace and assertState are this player's own.
	if trace.locks.Load() != 0 {
		t.Fatalf("exempt player reconciled: locks=%d", trace.locks.Load())
	}
	f.assertState(t, "", "", 0)
}

func TestAgoraExemptDirectAndAccountLifecycle(t *testing.T) {
	f := newAgoraFixture(t, "Lifecycle-"+uuid.NewString(), 4)
	ctx := context.Background()
	remote := newFakeAgoraRemote()
	s := &agoraService{pool: f.pool, remote: remote, room: "!exempt"}
	for _, state := range []string{"", "creating", "ready"} {
		_, err := f.pool.Exec(ctx, `UPDATE players SET agora_exempt=true,agora_state=NULLIF($2,''),agora_claim_localpart=CASE WHEN $2='' THEN NULL ELSE $3 END,agora_localpart=CASE WHEN $2='ready' THEN $3 ELSE NULL END WHERE id=$1`, f.player, state, "old-"+f.player.String())
		if err != nil {
			t.Fatal(err)
		}
		if err = s.reconcile(ctx, f.player); err != nil {
			t.Fatal(err)
		}
		a, err := s.Account(ctx, f.player)
		if err != nil || a.State != "exempt" || a.UserID != "" || a.Localpart != "" || a.Homeserver != "" {
			t.Fatalf("exempt account leaked ready/pending: %+v %v", a, err)
		}
		if _, err = s.Password(ctx, f.player); err == nil || !strings.Contains(err.Error(), "No community chat account") {
			t.Fatalf("exempt password request: %v", err)
		}
	}
	if remote.createCalls != 0 || len(remote.secrets) != 0 {
		t.Fatalf("remote touched for exempt player: create=%d secrets=%d", remote.createCalls, len(remote.secrets))
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='agora_ready'`, f.player).Scan(&n); err != nil || n != 0 {
		t.Fatalf("exempt notice %d %v", n, err)
	}
	// A renamed flag, not username-prefix guessing, decides ordinary eligibility.
	if _, err := f.pool.Exec(ctx, `UPDATE players SET username=$2,agora_exempt=false,agora_state=NULL,agora_claim_localpart=NULL,agora_localpart=NULL WHERE id=$1`, f.player, "gpt-human-"+f.player.String()); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	a, err := s.Account(ctx, f.player)
	if err != nil || a.State != "ready" || remote.created != 1 {
		t.Fatalf("ordinary account blocked: %+v %v created=%d", a, err, remote.created)
	}
}

func TestAgoraExemptMigrationBackfillAndDefault(t *testing.T) {
	f := newAgoraFixture(t, "Migration-"+uuid.NewString(), 4)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Real PostgreSQL migration against a pre183 players shape, isolated by pg_temp.
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE players(username text PRIMARY KEY); INSERT INTO players VALUES ('luna-test'),('gpt-idomeneus'),('gpt-arete'),('timothy'),('not-gpt-test')`); err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("../../db/migrations/183_agora_exempt.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"luna-test": true, "gpt-idomeneus": true, "gpt-arete": true, "timothy": false, "not-gpt-test": false} {
		var got bool
		if err = tx.QueryRow(ctx, `SELECT agora_exempt FROM players WHERE username=$1`, name).Scan(&got); err != nil || got != want {
			t.Fatalf("backfill %s=%v want%v %v", name, got, want, err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO players(username) VALUES('gpt-future')`); err != nil {
		t.Fatal(err)
	}
	var got bool
	if err = tx.QueryRow(ctx, `SELECT agora_exempt FROM players WHERE username='gpt-future'`).Scan(&got); err != nil || got {
		t.Fatalf("new registration default %v %v", got, err)
	}
	down, err := os.ReadFile("../../db/migrations/183_agora_exempt.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	var again bool
	if err = tx.QueryRow(ctx, `SELECT agora_exempt FROM players WHERE username='gpt-future'`).Scan(&again); err != nil || !again {
		t.Fatalf("re-up backfill %v %v", again, err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO players(username,agora_exempt) VALUES('null-is-invalid',NULL)`); err == nil {
		t.Fatal("NULL exemption accepted")
	}
	// The preceding deliberate constraint violation aborts the fixture transaction.
}
