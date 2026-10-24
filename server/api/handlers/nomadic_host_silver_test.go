package handlers

// Starting reserves remain fixed when ordinary foot soldiers stop drawing
// silver (Timothy 2026-10-13). This exercises the real founder INSERT.

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
)

func TestSeedNomadicHost_SilverAmountIs120(t *testing.T) {
	pool := escortTestPool(t)
	ctx := context.Background()
	worldID, playerID := seedEscortWorld(t, pool)
	eventStore := events.NewStore(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := seedNomadicHost(ctx, tx, eventStore, worldID, playerID, 0, 0); err != nil {
		t.Fatalf("seedNomadicHost: %v", err)
	}

	var silverAmount, silverRate float64
	if err := tx.QueryRow(ctx,
		`SELECT silver_amount, silver_rate FROM founder_phase WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&silverAmount, &silverRate); err != nil {
		t.Fatalf("read founder_phase silver columns: %v", err)
	}

	const eps = 1e-9
	if math.Abs(silverAmount-120) > eps {
		t.Errorf("founder reserve=%v, want fixed120 independent of salary", silverAmount)
	}
	// Foot soldiers have no salary drain; the reserve survives indefinitely.
	wantRate := 0.0
	if math.Abs(silverRate-wantRate) > eps {
		t.Errorf("founder_phase.silver_rate = %v, want %v (no salary drain)", silverRate, wantRate)
	}
}

// Basic ships and escort do not drain the new host's reserve.
func TestNomadicReserveSupportsBasicFleet(t *testing.T) {
	for _, kind := range []string{"spearman", "galley", "merchantman"} {
		category := "naval"
		if kind == "spearman" {
			category = "land"
		}
		if got := combat.UnitUpkeep(kind, category, 100, "marching", 20).Silver; got != 0 {
			t.Errorf("%s pay=%g want0", kind, got)
		}
	}
	if risenHostStartingSilver != 60 {
		t.Fatalf("rest host silver=%g want60", risenHostStartingSilver)
	}
}
