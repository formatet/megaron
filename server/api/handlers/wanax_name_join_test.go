package handlers

// DB integration tests (real Postgres, gated by DATABASE_URL) for
// mvp/minoisk-identitet Del 2 (Timothy 2026-08-05: "vi måste börja dela ut
// minoiska namn random på spelare så att folk inte använder deras logins").
// players.username was both the login and the only public name any other
// Wanax ever saw. Migration 109 adds players.wanax_name; these tests pin that
// Join is the site that assigns one (a player becomes a Wanax at join) and
// that a read surface downstream (foreign_units.go:93, on the asynchronicity
// grind's path) shows the Wanax name instead of the login once it does.
//
// Red-before (against migration 109 applied, join.go unmodified): a joining
// player's wanax_name stays NULL — join never touches the column.

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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TestJoin_AssignsWanaxName is the red-before pin: before this slice, join
// never sets players.wanax_name, so it stays NULL for every new joiner.
func TestJoin_AssignsWanaxName(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	playerID, token := registerViewer(t, ctx, authSvc, "wanax-solo")

	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /join = %d %q, want 201", rec.Code, rec.Body.String())
	}

	var wanaxName *string
	var username string
	if err := pool.QueryRow(ctx,
		`SELECT wanax_name, username FROM players WHERE id = $1`, playerID,
	).Scan(&wanaxName, &username); err != nil {
		t.Fatalf("read player row: %v", err)
	}
	if wanaxName == nil {
		t.Fatalf("wanax_name after join: got NULL, want a minoan name (not %q, the login)", username)
	}
	if *wanaxName == username {
		t.Errorf("wanax_name after join: got %q, equal to username — the public name must differ from the login", *wanaxName)
	}
}

// TestJoin_TwoPlayers_GetDifferentWanaxNames: the name pool has no reason to
// collide for the first two joiners of a world, and a duplicate would be
// exactly the identity confusion wanax_name exists to prevent.
func TestJoin_TwoPlayers_GetDifferentWanaxNames(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)
	// A second spawn tile far from the first — join's spawn query refuses any
	// tile within 4 hexes of an existing host.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 10, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed second spawn tile: %v", err)
	}

	authSvc := auth.NewService(pool, "test-secret")
	p1ID, tok1 := registerViewer(t, ctx, authSvc, "wanax-a")
	p2ID, tok2 := registerViewer(t, ctx, authSvc, "wanax-b")

	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))
	for _, tok := range []string{tok1, tok2} {
		req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST /join = %d %q, want 201", rec.Code, rec.Body.String())
		}
	}

	var name1, name2, user1, user2 *string
	if err := pool.QueryRow(ctx, `SELECT wanax_name, username FROM players WHERE id = $1`, p1ID).Scan(&name1, &user1); err != nil {
		t.Fatalf("read player 1: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT wanax_name, username FROM players WHERE id = $1`, p2ID).Scan(&name2, &user2); err != nil {
		t.Fatalf("read player 2: %v", err)
	}
	if name1 == nil || name2 == nil {
		t.Fatalf("wanax_name after join: player1=%v player2=%v, want both non-NULL", name1, name2)
	}
	if *name1 == *name2 {
		t.Errorf("two joiners got the SAME wanax_name %q — must be unique", *name1)
	}
	if user1 != nil && *name1 == *user1 {
		t.Errorf("player 1 wanax_name %q equals username — public name must differ from login", *name1)
	}
	if user2 != nil && *name2 == *user2 {
		t.Errorf("player 2 wanax_name %q equals username — public name must differ from login", *name2)
	}
}

// TestForeignUnits_ShowsWanaxNameNotLogin pins the asynchronicity-grind read
// surface foreign_units.go:93 (`COALESCE(pl.wanax_name, pl.username)` after
// this slice): a foreign unit's owner field must be the Wanax's public name,
// never their login.
func TestForeignUnits_ShowsWanaxNameNotLogin(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, viewerToken := registerViewer(t, ctx, authSvc, "fu-viewer-wanax")

	var enemyID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"fu-enemy-wanax-"+uuid.New().String(), "fu-enemy-wanax-"+uuid.New().String()+"@test.invalid",
	).Scan(&enemyID); err != nil {
		t.Fatalf("create enemy owner: %v", err)
	}
	// A synthetic, UUID-suffixed name rather than a real pool entry — this test
	// only cares that the read surface prefers wanax_name over username, not
	// that the name came from the pool, and a pool name could collide with one
	// already assigned to another player row left behind by an earlier test run.
	wantName := "Rhadamas-" + uuid.New().String()
	if _, err := pool.Exec(ctx, `UPDATE players SET wanax_name = $1 WHERE id = $2`, wantName, enemyID); err != nil {
		t.Fatalf("set enemy wanax_name: %v", err)
	}

	// Enemy unit at (0,0), well inside the viewer's own settlement-eye vision
	// once the viewer founds nearby — simplest fixture: place the unit right on
	// the seeded spawn tile and grant the viewer's OWN account visibility via a
	// capital right next to it (foreignUnits doesn't require FOW here; it uses
	// live eyes only, but citiesTestPool fixtures elsewhere show a settlement's
	// own eye is enough — mirror TestForeignUnits_LiveTierRevealsNearbyPositionedUnit).
	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Wanaxview', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, viewerID,
	); err != nil {
		t.Fatalf("create viewer capital: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 2, 0)`,
		worldID, enemyID,
	); err != nil {
		t.Fatalf("create enemy unit: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 2, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed enemy unit tile: %v", err)
	}

	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	rt := chi.NewRouter()
	rt.Use(auth.Middleware(authSvc))
	rt.Get("/worlds/{worldID}/foreign-units", wh.ForeignUnits)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/foreign-units", nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /foreign-units = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var units []foreignUnitView
	if err := json.Unmarshal(rec.Body.Bytes(), &units); err != nil {
		t.Fatalf("decode /foreign-units response: %v (body: %s)", err, rec.Body.String())
	}
	if len(units) != 1 {
		t.Fatalf("expected exactly 1 visible foreign unit, got %d: %+v", len(units), units)
	}
	if units[0].Owner != wantName {
		t.Errorf("foreign unit owner: got %q, want %q (the Wanax name, not the login)", units[0].Owner, wantName)
	}
}
