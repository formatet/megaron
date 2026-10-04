package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/api/handlers"
	"formatet/megaron/server/internal/agora"
	"formatet/megaron/server/internal/notify"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type agoraFixture struct {
	pool                *pgxpool.Pool
	world, player, city uuid.UUID
	name                string
}

func newAgoraFixture(t *testing.T, name string, maxConns int32) agoraFixture {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := agoraFixture{pool: pool, name: name}
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status) VALUES($1,'archived') RETURNING id`, "agora-"+uuid.NewString()).Scan(&f.world); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,wanax_name,password_hash) VALUES($1,$2,'x') RETURNING id`, "agora-"+uuid.NewString(), name).Scan(&f.player); err != nil {
		t.Fatal(err)
	}
	var province uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,0,0,'plains') RETURNING id`, f.world).Scan(&province); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id) VALUES($1,$2,'Agora fixture','achaean',$3) RETURNING id`, f.world, province, f.player).Scan(&f.city); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, f.world)
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id=$1`, f.player)
	})
	return f
}

type fakeAgoraRemote struct {
	mu                     sync.Mutex
	accounts               map[string]string
	transactions           map[string]error
	created, createCalls   int
	down, crashAfterCreate bool
	secrets                []string
}

func newFakeAgoraRemote() *fakeAgoraRemote {
	return &fakeAgoraRemote{accounts: map[string]string{}, transactions: map[string]error{}}
}
func (*fakeAgoraRemote) Initialize(context.Context) error { return nil }
func (*fakeAgoraRemote) Homeserver() string               { return "https://agora.test" }
func (*fakeAgoraRemote) UserID(localpart string) string   { return "@" + localpart + ":agora.test" }
func (f *fakeAgoraRemote) Create(_ context.Context, txn, localpart, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.down {
		return agora.ErrUnavailable
	}
	if outcome, ok := f.transactions[txn]; ok {
		return outcome
	}
	if _, ok := f.accounts[localpart]; ok {
		f.transactions[txn] = agora.ErrOccupied
		return agora.ErrOccupied
	}
	f.accounts[localpart] = ""
	f.transactions[txn] = nil
	f.created++
	f.secrets = append(f.secrets, password)
	if f.crashAfterCreate {
		f.crashAfterCreate = false
		return agora.ErrUnavailable
	}
	return nil
}
func (f *fakeAgoraRemote) ResetPassword(_ context.Context, _, localpart, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return agora.ErrUnavailable
	}
	if _, ok := f.accounts[localpart]; !ok {
		return agora.ErrUnavailable
	}
	f.secrets = append(f.secrets, password)
	return nil
}
func (f *fakeAgoraRemote) SetDisplayName(_ context.Context, localpart, _, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[localpart] = name
	return nil
}

func (f agoraFixture) assertState(t *testing.T, state, localpart string, notices int) {
	t.Helper()
	var gotState, gotLocalpart string
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT COALESCE(agora_state,''),COALESCE(agora_localpart,'') FROM players WHERE id=$1`, f.player).Scan(&gotState, &gotLocalpart); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='agora_ready'`, f.player).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if gotState != state || gotLocalpart != localpart || count != notices {
		t.Fatalf("state=%q localpart=%q notices=%d; want %q %q %d", gotState, gotLocalpart, count, state, localpart, notices)
	}
}

func TestAgoraReconcileRetriesOutageAndCrashWithOneAccount(t *testing.T) {
	f := newAgoraFixture(t, "Ágamemnon-"+uuid.NewString(), 4)
	r := newFakeAgoraRemote()
	s := &agoraService{pool: f.pool, remote: r, room: "!test"}
	ctx := context.Background()
	r.down = true
	if s.reconcile(ctx, f.player) == nil {
		t.Fatal("outage unexpectedly succeeded")
	}
	f.assertState(t, "creating", "", 0)
	r.down = false
	r.crashAfterCreate = true
	if s.reconcile(ctx, f.player) == nil {
		t.Fatal("lost create response unexpectedly succeeded")
	}
	f.assertState(t, "creating", "", 0)
	var stableClaim string
	if err := f.pool.QueryRow(ctx, `SELECT agora_claim_localpart FROM players WHERE id=$1`, f.player).Scan(&stableClaim); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	f.assertState(t, "ready", stableClaim, 1)
	if r.created != 1 || r.accounts[stableClaim] != f.name {
		t.Fatalf("created=%d; displayname preserved=%v", r.created, r.accounts[stableClaim] == f.name)
	}
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	f.assertState(t, "ready", stableClaim, 1)
	// Removing the archived notice must never trigger reprovisioning/reinsert.
	if _, err := f.pool.Exec(ctx, `DELETE FROM notifications WHERE player_id=$1`, f.player); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	f.assertState(t, "ready", stableClaim, 0)
	if r.created != 1 {
		t.Fatal("repeat reconciliation created another account")
	}
}

func TestAgoraReconcileConcurrentAndDeliverAfterPoolRelease(t *testing.T) {
	f := newAgoraFixture(t, "Concurrent-"+uuid.NewString(), 1)
	r := newFakeAgoraRemote()
	h := notify.New()
	h.SetPool(f.pool)
	s := &agoraService{pool: f.pool, remote: r, room: "!test", hub: h}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errorsCh := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errorsCh <- s.reconcile(ctx, f.player) }()
	}
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal("concurrent reconciliation or post-release delivery failed", err)
		}
	}
	localpart, _ := agora.Localpart(f.name)
	f.assertState(t, "ready", localpart, 1)
	if r.created != 1 || r.createCalls != 1 {
		t.Fatalf("created=%d createCalls=%d", r.created, r.createCalls)
	}
}

func TestAgoraNormalizedAndRemoteOccupiedCollisions(t *testing.T) {
	tag := strings.ReplaceAll(uuid.NewString(), "-", "")
	f := newAgoraFixture(t, "Ágamemnon"+tag, 4)
	second := newAgoraFixture(t, "Agamemnon"+tag, 4)
	r := newFakeAgoraRemote()
	base, _ := agora.Localpart(f.name)
	r.accounts[base] = "deactivated or independently owned"
	s := &agoraService{pool: f.pool, remote: r, room: "!test"}
	ctx := context.Background()
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	f.assertState(t, "ready", base+"2", 1)
	if err := s.reconcile(ctx, second.player); err != nil {
		t.Fatal(err)
	}
	second.assertState(t, "ready", base+"3", 1)
	if r.accounts[base] != "deactivated or independently owned" || r.accounts[base+"2"] != f.name || r.accounts[base+"3"] != second.name {
		t.Fatal("collision overwrote another identity or displayname")
	}
}

func TestAgoraReadyNotificationFailureRollsBackAndRecovers(t *testing.T) {
	f := newAgoraFixture(t, "Rollback-"+uuid.NewString(), 4)
	r := newFakeAgoraRemote()
	s := &agoraService{pool: f.pool, remote: r, room: "!test"}
	ctx := context.Background()
	// Restrict the injected database failure to this fixture's recipient.
	name := "agora_fail_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	function := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.player_id='%s'::uuid AND NEW.kind='agora_ready' THEN RAISE EXCEPTION 'fixture notification failure'; END IF; RETURN NEW; END $$`, name, f.player)
	if _, err := f.pool.Exec(ctx, function); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION %s()`, name, name)); err != nil {
		t.Fatal(err)
	}
	remove := func() {
		_, _ = f.pool.Exec(ctx, "DROP TRIGGER IF EXISTS "+name+" ON notifications")
		_, _ = f.pool.Exec(ctx, "DROP FUNCTION IF EXISTS "+name+"()")
	}
	t.Cleanup(remove)
	if s.reconcile(ctx, f.player) == nil {
		t.Fatal("notification insertion failure ignored")
	}
	f.assertState(t, "creating", "", 0)
	remove()
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	localpart, _ := agora.Localpart(f.name)
	f.assertState(t, "ready", localpart, 1)
	if r.created != 1 {
		t.Fatal("atomic recovery created another account")
	}
	var body string
	if err := f.pool.QueryRow(ctx, `SELECT body_json::text FROM notifications WHERE player_id=$1`, f.player).Scan(&body); err != nil {
		t.Fatal(err)
	}
	for _, secret := range r.secrets {
		if strings.Contains(body, secret) {
			t.Fatal("password persisted in notification")
		}
	}
	var eventCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1`, f.player).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatal("Agora credentials flow created a game event")
	}
}

func TestAgoraEligibilityAndPlayerPassword(t *testing.T) {
	f := newAgoraFixture(t, "Eligibility-"+uuid.NewString(), 4)
	r := newFakeAgoraRemote()
	s := &agoraService{pool: f.pool, remote: r, room: "!test"}
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=NULL WHERE id=$1`, f.city); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	f.assertState(t, "", "", 0)
	if r.created != 0 {
		t.Fatal("account provisioned before owning a city")
	}
	if _, err := s.Password(ctx, f.player); !errors.Is(err, handlers.ErrAgoraPending) {
		t.Fatal("password allowed before readiness")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$1 WHERE id=$2`, f.player, f.city); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcile(ctx, f.player); err != nil {
		t.Fatal(err)
	}
	account, err := s.Account(ctx, f.player)
	if err != nil || account.State != "ready" || account.UserID != r.UserID(account.Localpart) {
		t.Fatal("account read did not show ready identity")
	}
	a, err := s.Password(ctx, f.player)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Password(ctx, f.player)
	if err != nil {
		t.Fatal(err)
	}
	if a.Password == b.Password || a.UserID != account.UserID || a.Homeserver != account.Homeserver {
		t.Fatal("password rotation or identity mismatch")
	}
}

func TestAgoraMigrationRejectsPartialStates(t *testing.T) {
	f := newAgoraFixture(t, "Schema-"+uuid.NewString(), 4)
	for _, sql := range []string{
		`UPDATE players SET agora_state='ready',agora_localpart='invalid' WHERE id=$1`,
		`UPDATE players SET agora_state='creating' WHERE id=$1`,
		`UPDATE players SET agora_claim_localpart='invalid' WHERE id=$1`,
		`UPDATE players SET agora_state='unknown' WHERE id=$1`,
	} {
		_, err := f.pool.Exec(context.Background(), sql, f.player)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("partial state not rejected by CHECK: %v", err)
		}
	}
}
