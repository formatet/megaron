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

func TestSeedNomadicHost_SilverAmountIs480(t *testing.T) {
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
	if math.Abs(silverAmount-480) > eps {
		t.Errorf("founder reserve=%v, want fixed480 independent of salary", silverAmount)
	}
	// Foot soldiers have no salary drain; the reserve survives indefinitely.
	wantRate := 0.0
	if math.Abs(silverRate-wantRate) > eps {
		t.Errorf("founder_phase.silver_rate = %v, want %v (no salary drain)", silverRate, wantRate)
	}
}

// The unchanged reserve now supports only the professional fleet's pay.
func TestNomadicReserveSupportsProfessionalFleet(t *testing.T) {
	const startSilver = 480.0
	spearmanSilver := combat.UpkeepSpecs["spearman"].Silver
	galleySilver := combat.UpkeepSpecs["galley"].Silver

	perTickDrain := 2*spearmanSilver + 2*galleySilver
	ticks := startSilver / perTickDrain
	if math.Abs(ticks-160) > 1e-9 {
		t.Errorf("480 silver / (2 spearmen + 2 galleys) = %v ticks, want 160 (only the two galleys draw silver)", ticks)
	}
}
