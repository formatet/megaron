package religion

import "math"

// What a good is worth TO A GOD, and what an offering composed of several goods
// is worth. Deliberately pure: no DB, no clock, no world — the caller supplies
// the numbers. That keeps the whole valuation testable without a rig, and lets
// the same function serve both consumers (a one-off prayer's odds, and the
// standing cult's kharis flow).
//
// The economy already has a scarcity measure — economy.LocalPrice — but it is
// LOCAL: a settlement's stock against its own production reference. The gods
// judge differently. Mortals price by their own need; gods price by the world's.

const (
	// Weights over the two scarcities. Spread leads: it is the one that forces
	// trade (a good only two Wanaxes hold must be bargained for), while volume
	// alone would make luxuries divine merely because they are slow to make.
	raritySpreadWeight = 0.65
	rarityVolumeWeight = 0.35

	// scarcityGain caps how far scarcity can lift a good above its base value:
	// at maximum rarity a good is worth (1 + scarcityGain)× its base. Bounded on
	// purpose — a runaway multiplier turns one good into the only thing worth
	// offering, and the offering burns it out of the world.
	scarcityGain = 2.0

	// smoothing is the daily blend toward the newly computed value. ~0.34 moves
	// the gods' taste over roughly three game-days: fast enough that a real
	// shortage registers, slow enough that a Wanax can plan a caravan against it.
	smoothing = 0.34

	// holderMinStock is the stock above which a settlement counts as "holding"
	// a good for the spread measure. Without a floor, a rounding dust of 0.01
	// grain would count as a holder and flatten spread to zero everywhere.
	holderMinStock = 1.0
)

// GoodRarity is the world-wide scarcity of a single good, both halves separate
// so the two can be inspected (and tuned) independently.
type GoodRarity struct {
	Spread float64 // 0..1 — high when FEW Wanaxes hold it
	Volume float64 // 0..1 — high when LITTLE of it exists
}

// Rarity derives both scarcities for one good.
//
// holders/totalOwners gives spread. worldStock is measured against the largest
// stock of any good in the world (maxStock) on a LOG scale — raw shares are
// useless here because grain outweighs every other good by orders of magnitude,
// which would price all of them as equally, maximally rare.
func Rarity(holders, totalOwners int, worldStock, maxStock float64) GoodRarity {
	var spread float64
	if totalOwners > 0 {
		held := float64(holders) / float64(totalOwners)
		if held > 1 {
			held = 1
		}
		spread = 1 - held
	}

	volume := 1.0
	if maxStock > 0 && worldStock > 0 {
		volume = 1 - math.Log1p(worldStock)/math.Log1p(maxStock)
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	return GoodRarity{Spread: spread, Volume: volume}
}

// DivineValue is what one unit of a good is worth to a god: its base value
// lifted by world scarcity. Anchored on baseValue on purpose — without the
// anchor a good nobody bothers to produce would become divinely precious purely
// through neglect, and the cheapest way to please a god would be to make nothing.
func DivineValue(baseValue float64, r GoodRarity) float64 {
	rarity := raritySpreadWeight*r.Spread + rarityVolumeWeight*r.Volume
	return baseValue * (1 + scarcityGain*rarity)
}

// SmoothDivineValue blends a newly computed valuation with the previous day's,
// so the gods' taste drifts rather than jumps. previous == 0 means "no history"
// (first computation, or a new world) and adopts the new value outright.
func SmoothDivineValue(previous, computed float64) float64 {
	if previous <= 0 {
		return computed
	}
	return previous*(1-smoothing) + computed*smoothing
}

// CountsAsHolder reports whether a stock level makes a settlement a holder of
// the good for spread purposes.
func CountsAsHolder(stock float64) bool { return stock >= holderMinStock }
