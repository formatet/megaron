package handlers

// Tests for GET /auth/me's wanax_name field (megaron_plan_wanaxnamn_tilltal.md,
// player report 4eb54d52: "no surface addresses me by name"). The invariant is
// COALESCE(wanax_name, username) — never empty, never another player's name:
//
//  1. TestMe_ReturnsWanaxNameWhenSet: a player whose wanax_name has been
//     assigned (as join.go does at join time) sees that name, not their login.
//  2. TestMe_FallsBackToUsernameWhenWanaxNameNull: a player with no wanax_name
//     yet (pre-mig-111 account, or never joined) sees their username instead
//     of an empty string.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func authTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func authTestMeRequest(t *testing.T, authSvc *auth.Service, token string) map[string]any {
	t.Helper()
	h := NewAuthHandler(authSvc)
	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Get("/api/v1/auth/me", h.Me)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me = %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse /auth/me response: %v (body: %s)", err, rec.Body.String())
	}
	return result
}

// TestMe_ReturnsWanaxNameWhenSet is the slice's core acceptance criterion:
// /auth/me carries wanax_name, and it is the assigned name — not the login
// username, and not empty. Mutation-tested manually: deleting the
// "wanax_name" key from AuthHandler.Me's response map makes this test fail
// (the field is simply absent from result), confirming the test actually
// exercises the wired-up field rather than passing vacuously.
func TestMe_ReturnsWanaxNameWhenSet(t *testing.T) {
	pool := authTestPool(t)
	ctx := context.Background()
	authSvc := auth.NewService(pool, "test-secret")

	username := "login-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}

	// Assign a wanax_name the way join.go does — a public display name distinct
	// from the login username (Timothy 2026-08-05: players must stop showing
	// their login to everyone else in the game). players.wanax_name is UNIQUE,
	// so it needs the same per-run uniqueness as username above.
	wanaxName := "Idomeneus-" + uuid.New().String()
	if _, err := pool.Exec(ctx, `UPDATE players SET wanax_name = $1 WHERE id = $2`, wanaxName, claims.PlayerID); err != nil {
		t.Fatalf("seed wanax_name: %v", err)
	}

	result := authTestMeRequest(t, authSvc, accessToken)

	got, ok := result["wanax_name"]
	if !ok {
		t.Fatalf("wanax_name missing from /auth/me response: %v", result)
	}
	if got != wanaxName {
		t.Errorf("wanax_name = %v, want %q", got, wanaxName)
	}
	if result["username"] != username {
		t.Errorf("username = %v, want %q (wanax_name must not replace username)", result["username"], username)
	}
}

// TestMe_FallsBackToUsernameWhenWanaxNameNull covers the other half of the
// invariant: a freshly-registered player (wanax_name still NULL — it is only
// assigned at join, mig 111) must never see an empty string.
func TestMe_FallsBackToUsernameWhenWanaxNameNull(t *testing.T) {
	pool := authTestPool(t)
	ctx := context.Background()
	authSvc := auth.NewService(pool, "test-secret")

	username := "login-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	result := authTestMeRequest(t, authSvc, accessToken)

	if result["wanax_name"] != username {
		t.Errorf("wanax_name = %v, want fallback to username %q (COALESCE(wanax_name, username))", result["wanax_name"], username)
	}
	if result["wanax_name"] == "" {
		t.Error("wanax_name is empty — must never be blank for an authenticated player")
	}
}
