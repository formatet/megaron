package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
)

// TestProvinces_BurningOneTickLiveOnly: a city sacked and burned this tick
// carries `burning` on the /provinces marker — for that one tick only, and only
// in the viewer's live tier (a fire is activity; memory carries none). A city
// burned on an earlier tick is a plain ruin.
func TestProvinces_BurningOneTickLiveOnly(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'archived', 40) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	viewerID, accessToken := registerViewer(t, ctx, authSvc, "burn-viewer")

	city := func(q int, name string, owner *uuid.UUID, state string, burned *int) {
		t.Helper()
		var provID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&provID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		control := "capital"
		if owner == nil {
			control = "occupied"
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, burned_tick)
			 VALUES ($1, $2, $3, 'achaean', $4, $5, $6, $7, $8)`,
			worldID, provID, name, owner, control, owner != nil, state, burned,
		); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
	}
	now, earlier := 40, 39
	city(0, "Viewerton", &viewerID, "active", nil)
	city(2, "Ashlive", nil, "razed", &now)     // live tier, burned this tick
	city(3, "Coldash", nil, "razed", &earlier) // live tier, burned yesterday
	city(6, "Farsmoke", nil, "razed", &now)    // remembered tier, burned this tick
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, 6, 0)`,
		worldID, viewerID,
	); err != nil {
		t.Fatalf("mark (6,0) remembered: %v", err)
	}

	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Get("/worlds/{worldID}/provinces", wh.Provinces)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /provinces = %d %q, want 200", rec.Code, rec.Body.String())
	}

	var markers []struct {
		Name    string `json:"name"`
		State   string `json:"state"`
		Burning bool   `json:"burning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode /provinces: %v", err)
	}
	got := map[string]bool{}
	seen := map[string]bool{}
	for _, m := range markers {
		got[m.Name] = m.Burning
		seen[m.Name] = true
	}
	for _, name := range []string{"Ashlive", "Coldash", "Farsmoke"} {
		if !seen[name] {
			t.Fatalf("%s missing from /provinces (fixture geometry wrong?)", name)
		}
	}
	if !got["Ashlive"] {
		t.Errorf("Ashlive burning = false, want true — burned this tick, in live sight")
	}
	if got["Coldash"] {
		t.Errorf("Coldash burning = true, want false — the fire lasts one tick")
	}
	if got["Farsmoke"] {
		t.Errorf("Farsmoke burning = true, want false — remembered tier carries no activity")
	}
}
