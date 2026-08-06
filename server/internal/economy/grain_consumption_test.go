package economy

import (
	"math"
	"testing"

	"formatet/megaron/server/internal/events"
)

// ⭐ CANON 2026-08-06: a tick IS the day now (events.TicksPerDay = 1). This
// test used to guard the per-tick/per-day DISTINCTION — a founder-phase
// store draining at the daily figure instead of the tick-divided one would
// starve 24× too fast. That distinction is gone on purpose: with
// TicksPerDay=1, "per tick" and "per day" are the same number by definition.
// What still needs guarding is that GrainConsumptionPerTick keeps deriving
// from pop*GrainConsumptionPerCitizenPerDay/events.TicksPerDay (not a
// separately hardcoded pop*0.5) — so if TicksPerDay is ever recalibrated
// away from 1 again, this test starts failing immediately instead of
// silently reading the wrong figure forever, the way it did before.
func TestGrainConsumptionPerTick_EqualsDailyFigureNow(t *testing.T) {
	const pop = 4000

	if events.TicksPerDay != 1 {
		t.Fatalf("test premise broke: expected TicksPerDay=1, got %d", events.TicksPerDay)
	}

	got := GrainConsumptionPerTick(pop)
	want := float64(pop) * GrainConsumptionPerCitizenPerDay // pop*0.5 == 2000, exactly — a tick IS a day
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("GrainConsumptionPerTick(%d) = %v, want %v (pop*0.5, since TicksPerDay=1)", pop, got, want)
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
