package handlers

// FOW march rule (temenos_orderlopare_plan.md Fas 0): a march can only be
// ordered onto land the Wanax has seen — tier-1 live vision or tier-2
// remembered. Never-seen fog is rejected with 422 BEFORE any terrain or
// settlement response can leak what stands there. Explore-intent is exempt
// (it is the sanctioned order into the unknown), and so is a redirect target
// the player has seen. Redirects of marching units obey the same rule.
//
// DB integration tests (real Postgres, gated by DATABASE_URL) through a real
// chi.Mux with the prod route patterns.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

func TestMarch_FOWRule(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "fow-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// Capital at (0,0): settlement eye, live radius 3 over plains.
	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, capitalProvinceID, playerID,
	).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A contiguous strip of plains (0,0)..(10,0): (2,0) is inside the capital's
	// live radius (3); (10,0) is far outside it and never scouted → unseen.
	for q := 0; q <= 10; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create map tile (%d,0): %v", q, err)
		}
	}

	// Two garrisoned full-strength land units at the capital.
	newUnit := func() uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, settlement_id, type, category, size, status)
			 VALUES ($1, $2, $3, 'spearman', 'land', 100, 'garrison') RETURNING id`,
			worldID, playerID, capitalID,
		).Scan(&id); err != nil {
			t.Fatalf("create garrisoned unit: %v", err)
		}
		return id
	}
	unitA, unitB := newUnit(), newUnit()

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	march := func(unitID uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost,
			"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/march", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// 1. Plain march to a never-seen hex → 422 with the FOW explanation.
	rec := march(unitA, map[string]any{"target_q": 10, "target_r": 0})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("march to never-seen hex = %d %q, want 422", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "into unknown land") {
		t.Errorf("FOW rejection should explain the rule, got %q", rec.Body.String())
	}

	// 2. Explore-intent to the same unseen hex → allowed (202): explore is the
	// sanctioned order into the unknown.
	rec = march(unitB, map[string]any{"target_q": 10, "target_r": 0, "intent": "explore"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("explore to never-seen hex = %d %q, want 202 (explore is exempt from the FOW rule)",
			rec.Code, rec.Body.String())
	}

	// 3. Plain march to a hex inside the capital's live radius → allowed (202).
	rec = march(unitA, map[string]any{"target_q": 2, "target_r": 0})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("march to live-seen hex = %d %q, want 202", rec.Code, rec.Body.String())
	}
}

// TestRecall_FOW_RedirectNeverSeenRejected: redirecting a marching unit is a
// march order like any other — the new destination must be seen or remembered.
func TestRecall_FOW_RedirectNeverSeenRejected(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "fow-redir-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// The redirect target far away, never seen. It must exist in map_tiles
	// (the 404 fires before the FOW check otherwise).
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 10, 5, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("create redirect target tile: %v", err)
	}

	// A unit mid-march from the capital hex toward (5,0).
	now := time.Now()
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status,
		                    q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 0, 0, 5, 0, $3, $4) RETURNING id`,
		worldID, playerID, now, now.Add(time.Hour),
	).Scan(&unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	clk := clock.NewTestClock(now)
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)

	body, _ := json.Marshal(map[string]any{"target_q": 10, "target_r": 5})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/recall", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("redirect to never-seen hex = %d %q, want 422", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "into unknown land") {
		t.Errorf("FOW rejection should explain the rule, got %q", rec.Body.String())
	}
}
