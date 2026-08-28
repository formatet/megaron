package economy

import (
	"math"
	"testing"
)

// ⭐ CANON 2026-08-06: a tick IS the day now. This test used to guard the
// per-tick/per-day DISTINCTION — a founder-phase store draining at the daily
// figure instead of the tick-divided one would starve 24× too fast. That
// distinction is gone on purpose: a tick and a game-day are now the same
// unit by definition, so events.TicksPerDay was deleted 2026-08-06 (see
// internal/events.MacroTickInterval). What this now guards is the actual
// BEHAVIOUR — GrainConsumptionPerTick must keep costing exactly pop*0.5 per
// tick — rather than a constant that no longer exists, so a future change to
// the per-citizen figure can't silently drift out of sync with the founder
// phase's own hardcoded expectations.
func TestGrainConsumptionPerTick_EqualsDailyFigureNow(t *testing.T) {
	const pop = 4000
	const wantPerCitizen = 0.005 // 0.5 → 0.005 (mig 136, calibration choice ÷100, not ÷43.2)

	got := GrainConsumptionPerTick(pop)
	want := float64(pop) * wantPerCitizen // pop*0.005 == 20, exactly — a tick IS a day
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("GrainConsumptionPerTick(%d) = %v, want %v (pop*0.005)", pop, got, want)
	}
}

func TestGrainConsumptionPerTick_ClampsNegativePop(t *testing.T) {
	if got := GrainConsumptionPerTick(-500); got != 0 {
		t.Fatalf("GrainConsumptionPerTick(-500) = %v, want 0 (a dead city eats nothing)", got)
	}
}

func TestGrainConsumptionPerTick_ScalesLinearly(t *testing.T) {
	if got, want := GrainConsumptionPerTick(8000), 2*GrainConsumptionPerTick(4000); math.Abs(got-want) > 1e-9 {
		t.Fatalf("twice the people ate %v, want %v", got, want)
	}
}
