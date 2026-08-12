package handlers

// FOW tests for GET /worlds/{worldID}/marches and /messengers
// (fow/syskonytor-interpolerad-position). Before this slice both surfaces gated a
// moving actor on whether its ORIGIN OR TARGET sat on a live tile — so a march or
// runner strung between two seen hexes was streamed in full, whole route and all,
// even while its body crossed fog. That is a FOW leak in the payload, not the
// picture. The fix mirrors foreign_units.go: gate on the actor's CURRENT
// interpolated position. A player's OWN actor is exempt (information they already
// hold) and is still drawn in full.
//
// Same rig as foreign_units_fow_test.go: real Postgres (DATABASE_URL-gated),
// citiesTestPool, a real auth.Service token, and a chi.Mux with the production
// middleware. Reuses registerViewer from foreign_units_fow_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type marchMarkerView struct {
	ID      string `json:"id"`
	OriginQ int    `json:"origin_q"`
	OriginR int    `json:"origin_r"`
	TargetQ int    `json:"target_q"`
	TargetR int    `json:"target_r"`
}

type messengerMarkerView struct {
	ID      string `json:"id"`
	OriginQ int    `json:"origin_q"`
	OriginR int    `json:"origin_r"`
	DestQ   int    `json:"dest_q"`
	DestR   int    `json:"dest_r"`
	Own     bool   `json:"own"`
}

// TestMarches_ForeignMarchHiddenWhenCurrentPositionInFog is the leak regression:
// an enemy march whose ORIGIN is inside the viewer's vision but whose current
// interpolated position sits in fog must NOT be returned. Under the old
// endpoint-OR gate it was (origin visible ⇒ shown, whole route disclosed).
func TestMarches_ForeignMarchHiddenWhenCurrentPositionInFog(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "marches-viewer")

	var enemyID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"marches-enemy-"+uuid.New().String(), "marches-enemy-"+uuid.New().String()+"@test.invalid",
	).Scan(&enemyID); err != nil {
		t.Fatalf("create enemy: %v", err)
	}

	// Viewer capital at (0,0): settlement eye radius 3 over plains.
	capProv := insertProvince(t, ctx, pool, worldID, 0, 0)
	insertSettlement(t, ctx, pool, worldID, capProv, "Viewerton", viewerID, true)

	// Straight plains corridor q=2..20 so FindPath routes along r=0.
	for q := 2; q <= 20; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("map_tiles(%d,0): %v", q, err)
		}
	}
	// Enemy origin city at (2,0) — hex distance 2, inside the viewer's radius 3.
	originProv := insertProvince(t, ctx, pool, worldID, 2, 0)
	insertSettlement(t, ctx, pool, worldID, originProv, "Enemyburg", enemyID, true)
	// Target province at (20,0) — deep in fog.
	targetProv := insertProvince(t, ctx, pool, worldID, 20, 0)

	// Path (2,0)..(20,0) = 19 hexes (idx 0..18). progress 0.5 ⇒ idx 9 ⇒ q=11,
	// hex distance 11 from the capital: firmly in fog.
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`INSERT INTO marching_armies (world_id, origin_id, target_id, infantry, intent, departs_at, arrives_at)
		 VALUES ($1, $2, $3, 100, 'attack', $4, $5)`,
		worldID, originProv, targetProv, now.Add(-5*time.Hour), now.Add(5*time.Hour),
	); err != nil {
		t.Fatalf("insert marching army: %v", err)
	}

	got := callMarches(t, pool, authSvc, worldID, token, now)
	if len(got) != 0 {
		t.Fatalf("foreign march with a fogged current position must be hidden, got %d: %+v", len(got), got)
	}
}

// TestMarches_OwnMarchShownThroughFog guards the exemption: the viewer's OWN march
// stays visible in full even when its current position crosses fog.
func TestMarches_OwnMarchShownThroughFog(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "marches-own-viewer")

	// Viewer capital at (0,0); the march departs it toward fog.
	capProv := insertProvince(t, ctx, pool, worldID, 0, 0)
	insertSettlement(t, ctx, pool, worldID, capProv, "Viewerton", viewerID, true)
	for q := 0; q <= 20; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("map_tiles(%d,0): %v", q, err)
		}
	}
	targetProv := insertProvince(t, ctx, pool, worldID, 20, 0)

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`INSERT INTO marching_armies (world_id, origin_id, target_id, infantry, intent, departs_at, arrives_at)
		 VALUES ($1, $2, $3, 100, 'attack', $4, $5)`,
		worldID, capProv, targetProv, now.Add(-5*time.Hour), now.Add(5*time.Hour),
	); err != nil {
		t.Fatalf("insert own marching army: %v", err)
	}

	got := callMarches(t, pool, authSvc, worldID, token, now)
	if len(got) != 1 {
		t.Fatalf("the viewer's own march must be shown through fog, got %d: %+v", len(got), got)
	}
}

// TestMapMessengers_ForeignMessengerHiddenWhenCurrentPositionInFog is the leak
// regression for runners: a foreign messenger leaving a seen city but currently in
// fog must NOT be returned, even though its origin is visible.
func TestMapMessengers_ForeignMessengerHiddenWhenCurrentPositionInFog(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, token := registerViewer(t, ctx, authSvc, "msg-viewer")

	var enemyID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"msg-enemy-"+uuid.New().String(), "msg-enemy-"+uuid.New().String()+"@test.invalid",
	).Scan(&enemyID); err != nil {
		t.Fatalf("create enemy: %v", err)
	}

	capProv := insertProvince(t, ctx, pool, worldID, 0, 0)
	insertSettlement(t, ctx, pool, worldID, capProv, "Viewerton", viewerID, true)
	for q := 2; q <= 20; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("map_tiles(%d,0): %v", q, err)
		}
	}
	// Enemy origin city at (2,0) visible; destination city at (20,0) in fog.
	originProv := insertProvince(t, ctx, pool, worldID, 2, 0)
	originSett := insertSettlement(t, ctx, pool, worldID, originProv, "Enemyburg", enemyID, true)
	destProv := insertProvince(t, ctx, pool, worldID, 20, 0)
	destSett := insertSettlement(t, ctx, pool, worldID, destProv, "Faraway", enemyID, false)

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, kind, message_text, status, hex_q, hex_r, sent_at, arrives_at)
		 VALUES ($1, $2, $3, $4, 'message', 'hail', 'outbound', 2, 0, $5, $6)`,
		worldID, enemyID, originSett, destSett, now.Add(-5*time.Hour), now.Add(5*time.Hour),
	); err != nil {
		t.Fatalf("insert messenger: %v", err)
	}

	got := callMessengers(t, pool, authSvc, worldID, token, now)
	if len(got) != 0 {
		t.Fatalf("foreign messenger with a fogged current position must be hidden, got %d: %+v", len(got), got)
	}
}

// --- small fixture + call helpers (kept local to this file) ---

func insertProvince(t *testing.T, ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		worldID, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("insert province (%d,%d): %v", q, r, err)
	}
	return id
}

func insertSettlement(t *testing.T, ctx context.Context, pool *pgxpool.Pool, worldID, provinceID uuid.UUID, name string, ownerID uuid.UUID, capital bool) uuid.UUID {
	t.Helper()
	control := "colony"
	if capital {
		control = "capital"
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, $3, 'achaean', $4, $5, $6) RETURNING id`,
		worldID, provinceID, name, ownerID, control, capital,
	).Scan(&id); err != nil {
		t.Fatalf("insert settlement %q: %v", name, err)
	}
	return id
}

func callMarches(t *testing.T, pool *pgxpool.Pool, authSvc *auth.Service, worldID uuid.UUID, token string, now time.Time) []marchMarkerView {
	t.Helper()
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(now))
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/marches", wh.Marches)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/marches", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /marches = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var out []marchMarkerView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode /marches: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

func callMessengers(t *testing.T, pool *pgxpool.Pool, authSvc *auth.Service, worldID uuid.UUID, token string, now time.Time) []messengerMarkerView {
	t.Helper()
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(now))
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/messengers", wh.MapMessengers)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/messengers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /messengers = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var out []messengerMarkerView
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode /messengers: %v (body: %s)", err, rec.Body.String())
	}
	return out
}
