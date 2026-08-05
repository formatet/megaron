package handlers

// DB integration tests (real Postgres, gated by DATABASE_URL) for EK1 = B
// (Timothy 2026-08-05: "MVP är minoiskt"). The other five cultures are
// deactivated, not deleted — data, prayers and name pools stay, they just stop
// being choosable. Two independent entrypoints defaulted to akhaier before this
// slice: Join's round-robin over six cultures (join.go) and Settle's bare
// province.CultureAkhaier fallback (found_metropolis.go) — a player who joined
// as minoan could still found their metropolis as akhaier, since Settle never
// consulted what Join had picked.
//
// These three tests are the red-before evidence for mvp/minoisk-identitet
// Del 1, run once against unmodified master (got akhaier / got hatti, want
// minoan in every case) and kept as the permanent regression pin afterwards.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedJoinableWorld creates a fresh 'forming' world with exactly one eligible
// spawn tile at (0,0) — enough for Join's spawn-tile query to have a single,
// deterministic candidate. Archives any leftover active test world first
// (one_active_world unique index), mirroring escortTestPool's fixture.
func seedJoinableWorld(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name) VALUES ($1) RETURNING id`,
		"test-culture-mvp-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed spawn tile: %v", err)
	}
	return worldID
}

// joinCultureRouter wires a chi router with real auth middleware over Join +
// Settle, same pattern as foreign_units_fow_test.go's registerViewer rig.
func joinCultureRouter(pool *pgxpool.Pool, authSvc *auth.Service, clk clock.Clock) *chi.Mux {
	jh := NewJoinHandler(pool, events.NewStore(pool), economy.LoadSitosConfig(), clk, nil)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/join", jh.Join)
	r.Post("/worlds/{worldID}/founding/settle", jh.Settle)
	return r
}

// TestJoin_NoCultureSpecified_DefaultsToMinoan is the red-before pin for
// join.go's round-robin: the very first joiner in a fresh world (playerCount
// == 0) landed on cultures[0] == akhaier.
func TestJoin_NoCultureSpecified_DefaultsToMinoan(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	_, token := registerViewer(t, ctx, authSvc, "culture-nopref")

	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /join = %d %q, want 201", rec.Code, rec.Body.String())
	}
	var resp struct {
		Culture string `json:"culture"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode /join response: %v (body: %s)", err, rec.Body.String())
	}
	if resp.Culture != "minoan" {
		t.Errorf("join with no culture preference: got %q, want %q", resp.Culture, "minoan")
	}
}

// TestJoin_DeactivatedCultureSpecified_NormalisesToMinoan: join.go never
// validated a client-supplied culture — `keryx found --culture hatti` went
// straight through. Hatti is deactivated for MVP (EK1 = B); a request for it
// must silently normalise to minoan (tyst tvång, not a 400 — an inactive
// culture is not a client error).
func TestJoin_DeactivatedCultureSpecified_NormalisesToMinoan(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	_, token := registerViewer(t, ctx, authSvc, "culture-hatti")

	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join",
		strings.NewReader(`{"culture":"hatti"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /join = %d %q, want 201", rec.Code, rec.Body.String())
	}
	var resp struct {
		Culture string `json:"culture"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode /join response: %v (body: %s)", err, rec.Body.String())
	}
	if resp.Culture != "minoan" {
		t.Errorf("join with culture=hatti: got %q, want %q (deactivated cultures normalise silently)", resp.Culture, "minoan")
	}
}

// TestFoundMetropolis_EmptyCulture_DefaultsToMinoan is the red-before pin for
// found_metropolis.go's OWN, independent akhaier default — the bug found in
// play: a player who joined as minoan could still found as akhaier because
// Settle never looked at what Join had picked.
func TestFoundMetropolis_EmptyCulture_DefaultsToMinoan(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	playerID, token := registerViewer(t, ctx, authSvc, "culture-found")

	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))

	joinReq := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
	joinReq.Header.Set("Authorization", "Bearer "+token)
	joinRec := httptest.NewRecorder()
	r.ServeHTTP(joinRec, joinReq)
	if joinRec.Code != http.StatusCreated {
		t.Fatalf("POST /join = %d %q, want 201", joinRec.Code, joinRec.Body.String())
	}

	settleReq := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/founding/settle",
		strings.NewReader(`{"name":"Testopolis-`+uuid.New().String()+`"}`))
	settleReq.Header.Set("Authorization", "Bearer "+token)
	settleRec := httptest.NewRecorder()
	r.ServeHTTP(settleRec, settleReq)
	if settleRec.Code != http.StatusCreated {
		t.Fatalf("POST /founding/settle = %d %q, want 201", settleRec.Code, settleRec.Body.String())
	}

	var culture string
	if err := pool.QueryRow(ctx,
		`SELECT culture_id FROM settlements WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&culture); err != nil {
		t.Fatalf("read founded settlement culture: %v", err)
	}
	if culture != "minoan" {
		t.Errorf("found_metropolis with empty culture: got %q, want %q", culture, "minoan")
	}
}
