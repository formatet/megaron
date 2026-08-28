package handlers

// FOW subset invariant for /provinces (fow/provinces-samma-kunskap, 2026-07-30).
//
// Before this slice, /provinces (and /wanaxes, /cities) gated on
// province.VisibleFrom(pos, origins, 6) — a flat radius-6 disc around every
// visibility origin. /map gates on a different, correct model: tier-1 live eyes
// (loadLiveEyes + province.AnyEyeSees, per-eye-kind × per-target-terrain radius)
// union tier-2 remembered tiles (loadRememberedTiles). A hex could therefore get
// a full /provinces marker (name, owner, wall level, size_tier) while /map still
// reported it as fog — Timothy 2026-07-28: "en stad ska aldrig kunna stå på
// svart – har man inte kunskap om hexen kan man inte se staden."
//
// These are DB integration tests (real Postgres, gated by DATABASE_URL) driven
// through a real chi.Mux + auth.Middleware, exactly like production requests.

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

// mapKnows reproduces /map's exact knowledge decision (tier-1 live eyes ∪ tier-2
// remembered tiles) without depending on world.go's internal knownToPlayer helper —
// same pattern as fow_test.go's tierOf, so the contract is pinned even if the
// handler is refactored later.
func mapKnows(eyes []province.Eye, remembered map[[2]int]bool, pos province.MapPosition, terrain string) bool {
	return province.AnyEyeSees(eyes, pos, terrain) || remembered[[2]int{pos.Q, pos.R}]
}

type provinceMarkerView struct {
	SettlementID string `json:"settlement_id"`
	Name         string `json:"name"`
	Q            int    `json:"q"`
	R            int    `json:"r"`
	Visible      bool   `json:"visible"`
}

// TestProvincesFOW_HidesCityWithinOldFlatRadiusButOutsideRealKnowledge is the
// concrete AK3 case: a city 5 hexes from the viewer's capital sits INSIDE the old
// flat radius (6) but OUTSIDE the viewer's real knowledge — a settlement eye only
// sees 3 hexes over plain land (province.LiveRadius), and the hex was never
// scouted/remembered/contacted. On unmodified code (province.VisibleFrom(pos,
// origins, 6)) this city surfaces as a full marker; after the fix it must not
// appear in the response at all.
func TestProvincesFOW_HidesCityWithinOldFlatRadiusButOutsideRealKnowledge(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) // cascades provinces/settlements
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "provfow-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	viewerID := claims.PlayerID

	var subjectOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"subject-"+uuid.New().String(),
	).Scan(&subjectOwnerID); err != nil {
		t.Fatalf("create subject owner: %v", err)
	}

	// Viewer's capital at (0,0), plains — settlement eye sees 3 hexes over land.
	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Viewerton', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, viewerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// Subject city at hex distance 5 — inside the OLD flat radius 6, outside the
	// settlement eye's real radius 3, never scouted/contacted.
	var hiddenProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 5, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&hiddenProvinceID); err != nil {
		t.Fatalf("create hidden-city province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Shadowhaven', 'achaean', $3, 'capital', true)`,
		worldID, hiddenProvinceID, subjectOwnerID,
	); err != nil {
		t.Fatalf("create hidden-city settlement: %v", err)
	}

	// Control: a second subject city at hex distance 2 — inside the real
	// live-vision radius, must remain visible both before and after the fix.
	var seenProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 2, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&seenProvinceID); err != nil {
		t.Fatalf("create seen-city province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Nearshore', 'achaean', $3, 'capital', true)`,
		worldID, seenProvinceID, subjectOwnerID,
	); err != nil {
		t.Fatalf("create seen-city settlement: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)

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
	var markers []provinceMarkerView
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode /provinces response: %v (body: %s)", err, rec.Body.String())
	}

	var sawHidden, sawSeen bool
	for _, m := range markers {
		if m.Name == "Shadowhaven" {
			sawHidden = true
		}
		if m.Name == "Nearshore" {
			sawSeen = true
		}
	}
	if sawHidden {
		t.Error("Shadowhaven (distance 5, outside real knowledge) must NOT appear in /provinces — " +
			"this is the exact over-leak the flat VisibleFrom(...,6) gate produced on unmodified code")
	}
	if !sawSeen {
		t.Error("Nearshore (distance 2, inside settlement live-vision radius) should still appear in /provinces")
	}
}

// TestProvincesFOW_SubsetOfMapKnowledge is the AK2 general contract: for a viewer
// with one settlement and one field unit, every hex /provinces marks Visible must
// also be known under the exact tier-1∪tier-2 model /map uses
// (loadLiveEyes/AnyEyeSees ∪ loadRememberedTiles) — never a superset.
func TestProvincesFOW_SubsetOfMapKnowledge(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "provsub-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	viewerID := claims.PlayerID

	var otherOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"other-"+uuid.New().String(),
	).Scan(&otherOwnerID); err != nil {
		t.Fatalf("create other owner: %v", err)
	}

	// Viewer's capital at (0,0), plains.
	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Viewerton', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, viewerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A field unit, positioned well away from the capital (own eye, radius 2 land).
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 0, 10)`,
		worldID, viewerID,
	); err != nil {
		t.Fatalf("create positioned field unit: %v", err)
	}

	// A scatter of other-owned settlements at varying hex distances, spanning
	// inside/outside both the old flat radius (6) and the real live-vision radii.
	scatter := []struct {
		name string
		q, r int
	}{
		{"D2", 2, 0},   // inside settlement eye (3)
		{"D3", 0, 3},   // exactly at settlement eye edge (3)
		{"D4", 4, 0},   // outside settlement eye, inside old flat radius 6
		{"D5", 5, 0},   // outside settlement eye, inside old flat radius 6
		{"D6", 0, 6},   // outside settlement eye, at old flat radius edge
		{"D8unit", 0, 8}, // inside the field unit's own eye (radius 2 from (0,10))
		{"D20", 20, 0}, // outside everything
	}
	for _, s := range scatter {
		var pid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			worldID, s.q, s.r,
		).Scan(&pid); err != nil {
			t.Fatalf("create province %s: %v", s.name, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true)`,
			worldID, pid, s.name, otherOwnerID,
		); err != nil {
			t.Fatalf("create settlement %s: %v", s.name, err)
		}
	}

	clk := clock.NewTestClock(time.Now())
	wh := NewWorldHandler(pool, authSvc, clk)

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
	var markers []provinceMarkerView
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode /provinces response: %v (body: %s)", err, rec.Body.String())
	}

	// Independently recompute /map's own knowledge model for the same fixture and
	// assert /provinces' output is a SUBSET of it — the load-bearing invariant.
	eyes := loadLiveEyes(ctx, pool, worldID, viewerID, clk.Now())
	remembered := loadRememberedTiles(ctx, pool, worldID, viewerID)

	nameByPos := map[string][2]int{}
	for _, s := range scatter {
		nameByPos[s.name] = [2]int{s.q, s.r}
	}

	revealed := map[string]bool{}
	for _, m := range markers {
		revealed[m.Name] = true
		if m.Name == "Viewerton" {
			continue // own capital — trivially known
		}
		pos, ok := nameByPos[m.Name]
		if !ok {
			continue
		}
		q, r := pos[0], pos[1]
		if !mapKnows(eyes, remembered, province.MapPosition{Q: q, R: r}, "plains") {
			t.Errorf("/provinces revealed %s at (%d,%d) but /map's own knowledge model says it is unknown — "+
				"subset invariant violated", m.Name, q, r)
		}
	}

	// Sanity: the scatter actually spans both sides of the old flat radius 6, so
	// this test is not vacuously true. D4/D5/D6 are exactly the over-leak shape —
	// on unmodified code all three would appear (VisibleFrom radius 6 from (0,0));
	// under the fix only D2/D3 (settlement eye) and D8unit (field unit's own eye)
	// should appear.
	for _, want := range []struct {
		name    string
		visible bool
	}{
		{"D2", true}, {"D3", true},
		{"D4", false}, {"D5", false}, {"D6", false},
		{"D8unit", true}, {"D20", false},
	} {
		if got := revealed[want.name]; got != want.visible {
			t.Errorf("%s: revealed=%v, want %v", want.name, got, want.visible)
		}
	}
}

// TestProvincesFOW_UnauthenticatedSeesEverything is the AK4 regression guard: an
// unauthenticated request (no FOW context at all — guest view, offline rigs) must
// keep getting every settlement marked Visible, exactly as before this slice.
// knownToPlayer must never even be consulted in that path.
func TestProvincesFOW_UnauthenticatedSeesEverything(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"guestowner-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create owner: %v", err)
	}

	// A settlement far from everything (q=50) — would be fog under any FOW model,
	// yet an unauthenticated caller must still see it.
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 50, 50, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Farholm', 'achaean', $3, 'capital', true)`,
		worldID, provinceID, ownerID,
	); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	authSvc := auth.NewService(pool, "test-secret")
	wh := NewWorldHandler(pool, authSvc, clock.NewTestClock(time.Now()))

	r := chi.NewRouter()
	r.Use(auth.OptionalMiddleware(authSvc))
	r.Get("/worlds/{worldID}/provinces", wh.Provinces)

	// No Authorization header at all.
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /provinces (unauthenticated) = %d %q, want 200", rec.Code, rec.Body.String())
	}
	var markers []provinceMarkerView
	if err := json.Unmarshal(rec.Body.Bytes(), &markers); err != nil {
		t.Fatalf("decode /provinces response: %v (body: %s)", err, rec.Body.String())
	}
	var found *provinceMarkerView
	for i := range markers {
		if markers[i].Name == "Farholm" {
			found = &markers[i]
		}
	}
	if found == nil {
		t.Fatalf("expected Farholm to appear even to an unauthenticated caller")
	}
	if !found.Visible {
		t.Errorf("unauthenticated caller must see every settlement as Visible, got Visible=false for Farholm")
	}
}
