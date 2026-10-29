package handlers

// Cities in memory as they were seen (megaron_plan_vagbeskrivningar §S0,
// Timothy 2026-10-28 20:50/20:53). DB integration tests through a real chi.Mux
// + auth.Middleware, gated by DATABASE_URL like provinces_fow_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type seenMarkerView struct {
	Name       string     `json:"name"`
	Owner      string     `json:"owner"`
	Walls      int        `json:"walls"`
	Q          int        `json:"q"`
	R          int        `json:"r"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

type seenCityView struct {
	Name       string     `json:"name"`
	Owner      string     `json:"owner"`
	Q          *int       `json:"q"`
	R          *int       `json:"r"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

func TestSeenSettlements_MemoryShowsCityAsLastSeen(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'archived', 40) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	token, _, err := authSvc.Register(ctx, "seen-"+uuid.New().String(), "x")
	if err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	viewerID := claims.PlayerID

	newPlayer := func(wanax string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', $2) RETURNING id`,
			"p-"+uuid.New().String(), wanax,
		).Scan(&id); err != nil {
			t.Fatalf("create player: %v", err)
		}
		return id
	}
	oldOwner, newOwner := newPlayer("Wanax Old"), newPlayer("Wanax New")

	city := func(q, r int, name string, owner uuid.UUID, capital bool) uuid.UUID {
		var provinceID, id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			worldID, q, r,
		).Scan(&provinceID); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', $5) RETURNING id`,
			worldID, provinceID, name, owner, capital,
		).Scan(&id); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return id
	}
	remember := func(q, r int) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO player_scouted_tiles (world_id, player_id, q, r) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			worldID, viewerID, q, r,
		); err != nil {
			t.Fatalf("remember (%d,%d): %v", q, r, err)
		}
		if err := province.RecordSeenSettlements(ctx, pool, worldID, viewerID, []province.MapPosition{{Q: q, R: r}}); err != nil {
			t.Fatalf("record seen (%d,%d): %v", q, r, err)
		}
	}

	// Viewer's capital at (0,0); a settlement eye sees 3 hexes over plains,
	// so everything below at distance ≥ 6 is memory only.
	city(0, 0, "Viewerton", viewerID, true)

	// 1. Seen at tick 40, then taken, renamed and walled while nobody looks.
	oldtown := city(8, 0, "Oldtown", oldOwner, false)
	remember(8, 0)
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET owner_id = $2, name = 'Newtown', wall_level = 2 WHERE id = $1`,
		oldtown, newOwner,
	); err != nil {
		t.Fatalf("change Oldtown unseen: %v", err)
	}

	// 2. The hex was remembered empty; a city is founded there afterwards.
	remember(-8, 0)
	city(-8, 0, "Lateburg", oldOwner, false)

	// 3. Seen standing, later seen gone (sunk) — the hex has been seen empty.
	sunk := city(0, 8, "Drowned", oldOwner, false)
	remember(0, 8)
	if _, err := pool.Exec(ctx, `UPDATE settlements SET state = 'sunk' WHERE id = $1`, sunk); err != nil {
		t.Fatalf("sink: %v", err)
	}
	if err := province.RecordSeenSettlements(ctx, pool, worldID, viewerID, []province.MapPosition{{Q: 0, R: 8}}); err != nil {
		t.Fatalf("see the empty hex: %v", err)
	}

	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))
	router := chi.NewRouter()
	router.Use(auth.Middleware(authSvc))
	router.Get("/worlds/{worldID}/provinces", wh.Provinces)
	router.Get("/worlds/{worldID}/cities", wh.Cities)
	get := func(path string, into any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	markerAt := func(q, r int) *seenMarkerView {
		var markers []seenMarkerView
		get("/provinces", &markers)
		for i := range markers {
			if markers[i].Q == q && markers[i].R == r {
				return &markers[i]
			}
		}
		return nil
	}
	cityAt := func(q, r int) *seenCityView {
		var cities []seenCityView
		get("/cities", &cities)
		for i := range cities {
			if cities[i].Q != nil && *cities[i].Q == q && *cities[i].R == r {
				return &cities[i]
			}
		}
		return nil
	}

	m := markerAt(8, 0)
	if m == nil || m.Name != "Oldtown" || m.Owner != "Wanax Old" || m.Walls != 0 || m.LastSeenAt == nil {
		t.Errorf("/provinces memory marker = %+v, want Oldtown / Wanax Old / walls 0 with last_seen_at", m)
	}
	c := cityAt(8, 0)
	if c == nil || c.Name != "Oldtown" || c.Owner != "Wanax Old" || c.LastSeenAt == nil {
		t.Errorf("/cities memory row = %+v, want Oldtown / Wanax Old with last_seen_at", c)
	}

	if m := markerAt(-8, 0); m != nil {
		t.Errorf("Lateburg was founded after the hex was seen; /provinces shows %+v", m)
	}
	if c := cityAt(-8, 0); c != nil {
		t.Errorf("Lateburg was founded after the hex was seen; /cities shows %+v", c)
	}
	if m := markerAt(0, 8); m != nil {
		t.Errorf("Drowned was seen gone; /provinces still shows %+v", m)
	}

	// Seen again: a viewer settlement beside Oldtown puts it in live sight;
	// the /provinces read refreshes memory, which then holds the new truth
	// after the eye is gone.
	outpost := city(9, 0, "Lookout", viewerID, false)
	if m := markerAt(8, 0); m == nil || m.Name != "Newtown" || m.LastSeenAt != nil {
		t.Errorf("in live sight the marker must be live, got %+v", m)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM settlements WHERE id = $1`, outpost); err != nil {
		t.Fatalf("remove lookout: %v", err)
	}
	if m := markerAt(8, 0); m == nil || m.Name != "Newtown" || m.Owner != "Wanax New" || m.Walls != 2 || m.LastSeenAt == nil {
		t.Errorf("after the new sighting memory = %+v, want Newtown / Wanax New / walls 2", m)
	}
}
