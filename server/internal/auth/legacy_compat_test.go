package auth

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Proves the documented consequence for pre-existing accounts: their
// password_hash was bcrypt(username) (the old placeholder), so once real
// checking ships their working password is their own username, and nothing
// else, until they set a real one. Not designed UX — a mathematical fact
// worth having a test pin down.
//
// The legacy row is SEEDED here rather than assumed to exist. The first
// version of this test logged in as a bare "legacyuser" that nothing ever
// created: it passed only on a database that happened to carry such a row, and
// failed on every fresh, freshly-migrated clone (found 2026-09-04 when a
// sub-agent ran the full suite against its own empty container). A green
// ./... that never ran against an empty DB is unproven.
func TestLegacyAccountPasswordIsItsOwnUsername(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	username := "legacy-" + uuid8()
	seedLegacyPlayer(t, ctx, username)

	if _, _, err := svc.Login(ctx, username, username); err != nil {
		t.Errorf("legacy account login with username-as-password: got %v, want nil", err)
	}
	if _, _, err := svc.Login(ctx, username, "wrong"); err != ErrInvalidPassword {
		t.Errorf("legacy account login with wrong password: got %v, want ErrInvalidPassword", err)
	}
}

// seedLegacyPlayer inserts a player exactly the way the pre-3aa10e0 code did:
// password_hash = bcrypt(username). It writes the row directly instead of
// calling Register, because Register hashes whatever password it is given —
// reproducing the legacy shape is the whole point of the test.
func seedLegacyPlayer(t *testing.T, ctx context.Context, username string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(username), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, $2)`,
		username, string(hash),
	); err != nil {
		t.Fatalf("seed legacy player: %v", err)
	}
}
