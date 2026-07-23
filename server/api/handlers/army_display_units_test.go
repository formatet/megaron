package handlers

// SB7 regression: a settlement's displayed army must come from the units-table
// garrison — the single source of truth the combat resolver also reads — not the
// retired settlements.* integer columns. This is the Amarna bug (status showed a
// phantom 2530 while only 141 men in the units table actually fought): the display
// read a stale column, combat read units, and the two had drifted apart. After the
// column drop the display can only tell the truth.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"formatet/megaron/server/internal/auth"
)

func armyDisplayTestPool(t *testing.T) *pgxpool.Pool {
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

func TestLoadSettlement_ArmyReflectsUnitsGarrison(t *testing.T) {
	pool := armyDisplayTestPool(t)
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
	username := "armyview-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

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
		 VALUES ($1, $2, 'Amarna', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Garrison: 100 + 41 = 141 spearmen across two units, 10 chariots, 1 galley.
	// A separate FORMING spearman (60 men) must NOT count — it can't fight yet.
	seed := []struct {
		utype    string
		category string
		size     int
		status   string
	}{
		{"spearman", "land", 100, "garrison"},
		{"spearman", "land", 41, "garrison"},
		{"war_chariot", "land", 10, "garrison"},
		{"galley", "naval", 1, "garrison"},
		{"spearman", "land", 60, "forming"}, // excluded
	}
	for _, u := range seed {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			worldID, playerID, u.utype, u.category, u.size, u.status, settlementID,
		); err != nil {
			t.Fatalf("seed unit %s/%s: %v", u.utype, u.status, err)
		}
	}

	sett, err := loadSettlement(ctx, pool, settlementID, worldID)
	if err != nil {
		t.Fatalf("loadSettlement: %v", err)
	}

	if sett.Army.Spearman != 141 {
		t.Errorf("Spearman = %d, want 141 (garrison only, forming excluded)", sett.Army.Spearman)
	}
	if sett.Army.WarChariot != 10 {
		t.Errorf("WarChariot = %d, want 10", sett.Army.WarChariot)
	}
	if sett.Army.Ship != 1 {
		t.Errorf("Ship = %d, want 1", sett.Army.Ship)
	}
	if sett.Army.EliteInfantry != 0 {
		t.Errorf("EliteInfantry = %d, want 0", sett.Army.EliteInfantry)
	}
	if sett.Army.Priest != 0 {
		t.Errorf("Priest = %d, want 0 (priest is no longer a unit)", sett.Army.Priest)
	}
}
