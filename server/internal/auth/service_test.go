package auth

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uuid8 gives each test run a unique username so repeated runs never collide
// on the players.username UNIQUE constraint.
func uuid8() string {
	return uuid.NewString()[:8]
}

// DB integration test (real Postgres, gated by DATABASE_URL) — see
// api/handlers/unit_load_test.go for the same convention.
func newTestService(t *testing.T) *Service {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewService(pool, "test-secret")
}

func TestLoginRequiresMatchingPassword(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	username := "password-slice-" + uuid8()
	if _, _, err := svc.Register(ctx, username, "correct-horse"); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, _, err := svc.Login(ctx, username, "correct-horse"); err != nil {
		t.Errorf("login with correct password: got %v, want nil", err)
	}

	if _, _, err := svc.Login(ctx, username, "wrong-password"); err != ErrInvalidPassword {
		t.Errorf("login with wrong password: got %v, want ErrInvalidPassword", err)
	}
}

func TestRegisterAllowsEmptyPassword(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	username := "password-slice-empty-" + uuid8()
	if _, _, err := svc.Register(ctx, username, ""); err != nil {
		t.Fatalf("register with empty password: %v", err)
	}

	if _, _, err := svc.Login(ctx, username, ""); err != nil {
		t.Errorf("login with matching empty password: got %v, want nil", err)
	}

	if _, _, err := svc.Login(ctx, username, "not-empty"); err != ErrInvalidPassword {
		t.Errorf("login with wrong password: got %v, want ErrInvalidPassword", err)
	}
}
