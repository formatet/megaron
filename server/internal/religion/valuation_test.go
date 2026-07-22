package religion

import (
	"math"
	"testing"
)

// The two scarcities must stay distinct: a good many Wanaxes hold a little of is
// not the same as a good one Wanax holds mountains of. Collapsing them was the
// failure mode the design explicitly separated them to avoid.
func TestRarity_SpreadAndVolumeAreIndependent(t *testing.T) {
	// One holder out of ten, but an enormous pile: monopolised, not rare.
	monopoly := Rarity(1, 10, 1_000_000, 1_000_000)
	if monopoly.Spread < 0.85 {
		t.Errorf("a good held by 1 of 10 should read as high spread-rarity, got %.2f", monopoly.Spread)
	}
	if monopoly.Volume > 0.05 {
		t.Errorf("the world's largest stock cannot be volume-rare, got %.2f", monopoly.Volume)
	}

	// Everyone holds a trace: common, but genuinely thin on the ground.
	trace := Rarity(10, 10, 50, 1_000_000)
	if trace.Spread > 0.05 {
		t.Errorf("a good everyone holds should read as low spread-rarity, got %.2f", trace.Spread)
	}
	if trace.Volume < 0.5 {
		t.Errorf("50 units against a million should read as volume-rare, got %.2f", trace.Volume)
	}
}

// Grain outweighs every other good by orders of magnitude. On a linear share
// that alone would price everything else as maximally rare — which is why the
// volume measure is logarithmic.
func TestRarity_VolumeIsLogScaled(t *testing.T) {
	// Live-shaped numbers: grain ~500k, tin ~250k, pottery ~1.5k.
	tin := Rarity(2, 10, 250_000, 500_000)
	pottery := Rarity(6, 10, 1_500, 500_000)
	if !(pottery.Volume > tin.Volume) {
		t.Fatalf("the scarcer stock must read rarer: pottery %.2f vs tin %.2f", pottery.Volume, tin.Volume)
	}
	// The point of the log: a 2× difference in stock must NOT be a 2× difference
	// in rarity, or the measure is just the raw share again.
	if tin.Volume > 0.5 {
		t.Errorf("half the largest stock should not read as mostly-rare, got %.2f", tin.Volume)
	}
}

// A good nobody bothers to produce must not become divinely precious through
// neglect — otherwise the cheapest way to please a god is to make nothing.
func TestDivineValue_AnchoredOnBaseValue(t *testing.T) {
	maximallyRare := GoodRarity{Spread: 1, Volume: 1}
	cheap := DivineValue(2, maximallyRare) // stone
	dear := DivineValue(12, maximallyRare) // tin
	if cheap >= dear {
		t.Fatalf("base value must still order the goods: cheap %.1f vs dear %.1f", cheap, dear)
	}
	// And the lift is bounded.
	if got, want := DivineValue(10, maximallyRare), 10*(1+scarcityGain); math.Abs(got-want) > 1e-9 {
		t.Errorf("maximum lift = %.2f, want %.2f", got, want)
	}
	if got := DivineValue(10, GoodRarity{}); math.Abs(got-10) > 1e-9 {
		t.Errorf("no scarcity must leave base value untouched, got %.2f", got)
	}
}

// The gods' taste drifts; it does not jump. A Wanax must be able to plan a
// caravan against a valuation that will still roughly hold when it arrives.
func TestSmoothDivineValue_DriftsNotJumps(t *testing.T) {
	if got := SmoothDivineValue(0, 40); got != 40 {
		t.Errorf("no history should adopt the new value outright, got %.1f", got)
	}
	got := SmoothDivineValue(10, 40)
	if got <= 10 || got >= 40 {
		t.Fatalf("a smoothed step must land strictly between, got %.1f", got)
	}
	// Three days of the same pressure should get most of the way there — the
	// stated design target for how fast taste moves.
	v := 10.0
	for i := 0; i < 3; i++ {
		v = SmoothDivineValue(v, 40)
	}
	if v < 25 {
		t.Errorf("three days of pressure should close most of the gap, got %.1f", v)
	}
}

func TestCountsAsHolder_IgnoresDust(t *testing.T) {
	if CountsAsHolder(0.01) {
		t.Error("rounding dust must not count as holding a good")
	}
	if !CountsAsHolder(50) {
		t.Error("a real stock must count")
	}
}
