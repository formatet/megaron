package economy

import (
	"math"
	"testing"

	"github.com/poleia/server/internal/events"
)

// The founder-phase store drains at a per-TICK rate, while upkeep and consumption
// are authored per DAY. Getting that conversion wrong is silent: the host would
// starve 24× too fast or last 24× too long, and only a playtest would notice.
// These pin the unit.
func TestGrainConsumptionPerTick_IsPerTickNotPerDay(t *testing.T) {
	const pop = 4000

	got := GrainConsumptionPerTick(pop)
	wantPerDay := float64(pop) * GrainConsumptionPerCitizenPerDay // 2000/day
	want := wantPerDay / float64(events.TicksPerDay)              // ≈83.33/tick

	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("GrainConsumptionPerTick(%d) = %v, want %v", pop, got, want)
	}
	if math.Abs(got-wantPerDay) < 1e-9 {
		t.Fatalf("GrainConsumptionPerTick(%d) returned the DAILY figure (%v) — "+
			"the TicksPerDay division is missing", pop, got)
	}
	// A day's worth of ticks must add back up to a day's consumption.
	if roundTrip := got * float64(events.TicksPerDay); math.Abs(roundTrip-wantPerDay) > 1e-9 {
		t.Fatalf("%d ticks of consumption = %v, want one day = %v", events.TicksPerDay, roundTrip, wantPerDay)
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
