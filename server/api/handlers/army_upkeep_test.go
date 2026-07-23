package handlers

// SB7 follow-up: the army read surface reports the SAME upkeep the daily tick
// debits — summed from the units garrison via combat.UnitUpkeep, not the aggregate
// display counts (which can't represent naval flat-upkeep). Forming units, which
// can't fight, are excluded. DB integration test (real Postgres, gated by
// DATABASE_URL via the shared armyDisplayTestPool helper).

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"

	"formatet/megaron/server/internal/auth"
)

func TestArmyUpkeep_SumsGarrisonViaUnitUpkeep(t *testing.T) {
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
	username := "upkeepview-" + uuid.New().String()
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

	// 100+41 garrison spearmen (land — upkeep scales with size/100), 1 galley (naval —
	// flat per vessel). A FORMING spearman (60) must NOT count toward garrison upkeep.
	seed := []struct {
		utype    string
		category string
		size     int
		status   string
	}{
		{"spearman", "land", 100, "garrison"},
		{"spearman", "land", 41, "garrison"},
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

	total, perType, err := armyUpkeep(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("armyUpkeep: %v", err)
	}

	const eps = 1e-9
	// 141 spearmen: grain 141/100*5 = 7.05, silver 141/100*2 = 2.82; galley flat 4/3.
	if math.Abs(total.Grain-11.05) > eps {
		t.Errorf("total grain = %v, want 11.05 (7.05 spearman + 4 galley; forming excluded)", total.Grain)
	}
	if math.Abs(total.Silver-5.82) > eps {
		t.Errorf("total silver = %v, want 5.82 (2.82 spearman + 3 galley)", total.Silver)
	}
	if math.Abs(perType["spearman"].Grain-7.05) > eps {
		t.Errorf("spearman grain = %v, want 7.05", perType["spearman"].Grain)
	}
	if math.Abs(perType["galley"].Grain-4) > eps {
		t.Errorf("galley grain = %v, want 4 (naval flat, independent of size)", perType["galley"].Grain)
	}
}
