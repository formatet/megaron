package handlers

// Tests for /cities (temenos_gossip.md PASS 2b): rumour-known settlements must
// show up fuzzily (no exact coordinates) and must NOT be contactable — the
// KNOWN-set gate that messenger Send (and the legacy /wanaxes) use is
// loadVisibleOrigins, which deliberately does not consult known_settlements.
//
// These are DB integration tests (real Postgres, gated by DATABASE_URL) since
// loadCities is pure SQL orchestration across settlements/provinces/
// known_settlements that a mock can't meaningfully stand in for.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/province"
)

func citiesTestPool(t *testing.T) *pgxpool.Pool {
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

// TestCitiesRumourKnownIsNotContactable is the load-bearing PASS 2b contract:
// a settlement the player only knows about via gossip (known_settlements,
// level='rumour') appears in /cities fuzzily (no exact q,r) and is excluded
// from the KNOWN set that gates messenger Send — i.e. it stays unreachable
// until the player actually explores there.
func TestCitiesRumourKnownIsNotContactable(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}

	var viewerID, subjectOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"viewer-"+uuid.New().String(), "viewer-"+uuid.New().String()+"@test.invalid",
	).Scan(&viewerID); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"subject-owner-"+uuid.New().String(), "subject-owner-"+uuid.New().String()+"@test.invalid",
	).Scan(&subjectOwnerID); err != nil {
		t.Fatalf("create subject owner: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM known_settlements WHERE world_id = $1`, worldID)
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID) // cascades provinces/settlements
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id IN ($1, $2)`, viewerID, subjectOwnerID)
	})

	// Viewer's own capital — also serves as the landmark for the fuzzy bearing.
	var viewerProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&viewerProvinceID); err != nil {
		t.Fatalf("create viewer province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Viewerton', 'achaean', $3, 'capital', true)`,
		worldID, viewerProvinceID, viewerID,
	); err != nil {
		t.Fatalf("create viewer settlement: %v", err)
	}

	// Subject settlement — far away, never seen/remembered/contacted by viewer.
	var subjectProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 40, 40, 'plains') RETURNING id`,
		worldID,
	).Scan(&subjectProvinceID); err != nil {
		t.Fatalf("create subject province: %v", err)
	}
	var subjectSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Tinhaven', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, subjectProvinceID, subjectOwnerID,
	).Scan(&subjectSettlementID); err != nil {
		t.Fatalf("create subject settlement: %v", err)
	}

	// The viewer has only heard OF Tinhaven via gossip.
	if _, err := pool.Exec(ctx,
		`INSERT INTO known_settlements (world_id, player_id, settlement_id, level, industry_hint)
		 VALUES ($1, $2, $3, 'rumour', 'tin')`,
		worldID, viewerID, subjectSettlementID,
	); err != nil {
		t.Fatalf("seed known_settlements: %v", err)
	}

	h := &WorldHandler{pool: pool}
	cities := h.loadCities(ctx, worldID, viewerID)

	var rumourEntry *cityEntry
	for i := range cities {
		if cities[i].SettlementID == subjectSettlementID.String() {
			rumourEntry = &cities[i]
		}
	}
	if rumourEntry == nil {
		t.Fatalf("expected Tinhaven to appear in /cities as a rumour-known entry")
	}
	if rumourEntry.Knowledge != "rumour" {
		t.Errorf("expected knowledge=rumour, got %q", rumourEntry.Knowledge)
	}
	if rumourEntry.Q != nil || rumourEntry.R != nil {
		t.Errorf("rumour-known entry must not expose exact coordinates, got q=%v r=%v", rumourEntry.Q, rumourEntry.R)
	}
	if rumourEntry.IndustryHint != "tin" {
		t.Errorf("expected industry_hint=tin, got %q", rumourEntry.IndustryHint)
	}
	if rumourEntry.Bearing == "" {
		t.Errorf("expected a fuzzy bearing off the viewer's own capital (the nearest landmark)")
	}
	if rumourEntry.Note == "" {
		t.Errorf("expected a not-contactable note on the rumour entry")
	}

	// Not contactable: the KNOWN-set gate (loadVisibleOrigins) that messenger
	// Send/legacy Wanaxes use must NOT include Tinhaven — a rumour never
	// shortcuts into the known set.
	origins := h.visibleOrigins(ctx, worldID, viewerID)
	if province.VisibleFrom(province.MapPosition{Q: 40, R: 40}, origins, 6) {
		t.Errorf("rumour-known settlement must not be in the KNOWN set — Send would incorrectly allow it")
	}
}
