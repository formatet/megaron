package unit

// DB integration test (real Postgres, gated by DATABASE_URL) — see
// api/handlers/unit_load_test.go for the same convention.
//
// LoadDisplayName re-derives the namnstandarden name a notification payload
// needs (megaron_plan_dispatches.md §4/§6:2) — this proves it reads the same
// columns unitSummaries (api/handlers/unit.go) formats from, for every unit
// shape naming.go itself covers: a land unit, a named and an unnamed ship,
// and the nomadic host (whose name comes from the OWNER, not a support
// settlement).

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func lookupTestPool(t *testing.T) *pgxpool.Pool {
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

func TestLoadDisplayName(t *testing.T) {
	pool := lookupTestPool(t)
	ctx := context.Background()

	// See internal/combat/unit_arrival_colonize_test.go for why leftover
	// active test worlds must be archived first (one_active_world partial
	// unique index).
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
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

	// wanax_name carries a UNIQUE constraint (mig 111) — a fixed literal would
	// collide with itself on the next run against a reused DB, exactly the
	// "két färska DB per arm" trap megaron_arbetssatt.md §3 warns about.
	wanaxName := "Ariadne-" + uuid.New().String()[:8]
	var playerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', $2) RETURNING id`,
		"lookup-"+uuid.New().String(), wanaxName,
	).Scan(&playerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Knossos', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	var landID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, support_settlement_id, ordinal)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'garrison', $3, 2) RETURNING id`,
		worldID, playerID, settlementID,
	).Scan(&landID); err != nil {
		t.Fatalf("create land unit: %v", err)
	}
	if got := LoadDisplayName(ctx, pool, landID); got != "2nd Spearmen of Knossos" {
		t.Errorf("land unit: LoadDisplayName = %q, want %q", got, "2nd Spearmen of Knossos")
	}

	var landNoOrdinalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'garrison') RETURNING id`,
		worldID, playerID,
	).Scan(&landNoOrdinalID); err != nil {
		t.Fatalf("create land unit without ordinal/support: %v", err)
	}
	if got := LoadDisplayName(ctx, pool, landNoOrdinalID); got != "Spearmen" {
		t.Errorf("land unit without ordinal/support: LoadDisplayName = %q, want %q", got, "Spearmen")
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, support_settlement_id, name)
		 VALUES ($1, $2, 'galley', 'naval', 1, 'garrison', $3, 'White Dolphin') RETURNING id`,
		worldID, playerID, settlementID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create named ship: %v", err)
	}
	if got := LoadDisplayName(ctx, pool, shipID); got != "White Dolphin, Galley of Knossos" {
		t.Errorf("named ship: LoadDisplayName = %q, want %q", got, "White Dolphin, Galley of Knossos")
	}

	var unnamedShipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, support_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 'garrison', $3) RETURNING id`,
		worldID, playerID, settlementID,
	).Scan(&unnamedShipID); err != nil {
		t.Fatalf("create unnamed ship: %v", err)
	}
	if got := LoadDisplayName(ctx, pool, unnamedShipID); got != "Galley of Knossos" {
		t.Errorf("unnamed ship: LoadDisplayName = %q, want %q", got, "Galley of Knossos")
	}

	var hostID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status)
		 VALUES ($1, $2, 'nomadic_host', 'land', 100, 'garrison') RETURNING id`,
		worldID, playerID,
	).Scan(&hostID); err != nil {
		t.Fatalf("create nomadic host: %v", err)
	}
	wantHost := "Nomadic Host of " + wanaxName
	if got := LoadDisplayName(ctx, pool, hostID); got != wantHost {
		t.Errorf("nomadic host: LoadDisplayName = %q, want %q", got, wantHost)
	}

	if got := LoadDisplayName(ctx, pool, uuid.New()); got != "" {
		t.Errorf("unknown unit id: LoadDisplayName = %q, want \"\" (best-effort, never fabricated)", got)
	}
}
