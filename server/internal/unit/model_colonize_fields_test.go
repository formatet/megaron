package unit

// Regression test for Fas 2i: a colonize march has no settlement row until it
// arrives, so ListByOwner/Get were the only server-side reads that could ever
// surface the pending colony's chosen name — but selectCols/scanUnit didn't
// load march_intent/colony_name at all, so the Unit struct always saw them as
// zero-valued regardless of what the March handler had written. This left the
// colony's name invisible in `unit list` (and everywhere else) until the
// settlement actually existed.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func colonizeFieldsTestPool(t *testing.T) *pgxpool.Pool {
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

func TestListByOwner_SurfacesColonizeMarchIntentAndName(t *testing.T) {
	pool := colonizeFieldsTestPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"colonizer-"+uuid.New().String(), "colonizer-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	const wantColonyName = "Newhaven"
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r,
		                    target_q, target_r, march_intent, colony_name)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 0, 0, 10, 10, 'colonize', $3)
		 RETURNING id`,
		worldID, ownerID, wantColonyName,
	).Scan(&unitID); err != nil {
		t.Fatalf("create colonizing unit: %v", err)
	}

	store := NewStore(pool)
	units, err := store.ListByOwner(ctx, ownerID, worldID)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("ListByOwner returned %d units, want 1", len(units))
	}
	u := units[0]
	if u.MarchIntent == nil || *u.MarchIntent != "colonize" {
		t.Errorf("MarchIntent = %v, want \"colonize\"", u.MarchIntent)
	}
	if u.ColonyName == nil || *u.ColonyName != wantColonyName {
		t.Errorf("ColonyName = %v, want %q", u.ColonyName, wantColonyName)
	}
}
