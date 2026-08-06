package handlers

// SLICE B (silverupkeep halveras, Timothy 2026-08-05): halving the upkeep
// table's silver column, on its own, halves the founder phase's opening
// silver too — silverAmount is derived from -silverRate*nomadicHostRationTicks,
// and silverRate follows the table (perTick.Silver). That would quietly halve
// startsilvret 480→240 the moment the table changed, which is why
// nomadicHostRationTicks must double in the SAME slice.
//
// Reconciled with tick=day canon (merge 2026-08-06): mig 109 made UpkeepSpecs
// per TICK (a tick is a day) — there is no ticks-per-day divisor any more, so
// silverRate is the full per-tick drain, not table/24. Master had already
// retuned the ration base 2880→120 for that; halving the silver column then
// doubles it 120→240. Net at the merge: silverRate=-2, ration=240 ticks,
// silverAmount=480 — the calibrated opening, held unconditionally.
//
// DB integration test (real Postgres, gated by DATABASE_URL), same rig as
// escort_adoption_test.go / nomadic_host_dowry_test.go.

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
		t.Errorf("founder_phase.silver_amount = %v, want 480 — halving UpkeepSpecs' silver column "+
			"without doubling nomadicHostRationTicks would have halved this to 240 and made the "+
			"halving pointless", silverAmount)
	}
	// silverRate = -(spearmen × spearman silver upkeep per tick) = -(2×1). Under
	// tick=day canon UpkeepSpecs is per tick — no ticks-per-day divisor (mig 109).
	wantRate := -float64(2 * 1)
	if math.Abs(silverRate-wantRate) > eps {
		t.Errorf("founder_phase.silver_rate = %v, want %v (-(2 spearmen × 1 silver/tick), per-tick, no ÷24)", silverRate, wantRate)
	}
}

// TestSilverTrapGrind directly codes the grind Timothy set for this slice
// (plan §SLICE B): 480 starting silver, two spearmen cohorts plus two
// galleys, should now last 96 ticks — double the pre-slice runway. This is
// the regression threshold: if either the silver table or
// nomadicHostRationTicks drifts again without the other, this test is the
// tripwire.
func TestSilverTrapGrind(t *testing.T) {
	const startSilver = 480.0
	spearmanSilver := combat.UpkeepSpecs["spearman"].Silver
	galleySilver := combat.UpkeepSpecs["galley"].Silver

	perTickDrain := 2*spearmanSilver + 2*galleySilver
	ticks := startSilver / perTickDrain
	if math.Abs(ticks-96) > 1e-9 {
		t.Errorf("480 silver / (2 spearmen + 2 galleys) = %v ticks, want 96 (double the pre-slice-B runway)", ticks)
	}
}
