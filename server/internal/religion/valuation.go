package religion

import "math"

// What a good is worth TO A GOD, and what an offering composed of several goods
// is worth. Deliberately pure: no DB, no clock, no world — the caller supplies
// the numbers. That keeps the whole valuation testable without a rig, and lets
// the same function serve both consumers (a one-off prayer's odds, and the
// standing cult's kharis flow).
//
// A settlement's own local scarcity (its stock against its own production)
// is not what matters here. The gods judge differently. Mortals would price
// by their own need; gods price by the world's.

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
	// a good for the spread measure, EXPRESSED IN THE PRE-mig-136 SCALE: one raw
	// unit of the good. Without a floor, a rounding dust of 0.01 grain would
	// count as a holder and flatten spread to zero everywhere.
	//
	// Migration 136 (db/migrations/136_dagsverkesskalan.up.sql) divided every
	// good's stored amount by a different per-good divisor, so "1 raw unit" is
	// no longer the same number of stored units for every good. A flat 1.0
	// silently miscounted small-but-real stocks as "no holding" for every good
	// mig 136 rescaled (timber's old 1 unit is now 1/216 stored; the OLD 1.0
	// floor put it under the floor even though nothing about the world changed).
	// The equivalent floor per good is holderMinStock/goodDivisor(key) — see
	// CountsAsHolder, which is what actually applies this floor.
	holderMinStock = 1.0
)

// goodDivisors mirrors migration 136 (db/migrations/136_dagsverkesskalan.up.sql
// §5/§6): the per-good factor that migration divided settlement_goods amounts
// (and rate_per_tick, base_value) by, so "one gubbe on standard terrain"
// produces 1/tick. The migration itself is a one-time historical transform, not
// a live table — this map is the ONE place these numbers live going forward.
// Do not hand-copy them elsewhere; call goodDivisor or CountsAsHolder instead.
//
// A good absent here (silver, bronze) has an implicit divisor of 1 — mig 136
// deliberately left both at the pre-136 scale ("SILVER OCH BRONS RÖRS INTE":
// bronze was already 1.0 per-gubbe via the foundry, and silver is a currency
// calibrated against itself, not a production good — see the migration's
// header comment for the full reasoning).
var goodDivisors = map[string]float64{
	"timber":    216,
	"fish":      86.4,
	"cedar":     72,
	"grain":     43.2,
	"livestock": 36,
	"copper":    28.8,
	"oil":       21.6,
	"tin":       14.4,
	"wine":      14.4,
	"stone":     7.2,
	"pottery":   43.2,
	"horses":    28.8,
	"purple":    21.6,
}

// goodDivisor returns migration 136's rescale factor for a good, defaulting to
// 1 (unscaled) for goods mig 136 did not touch — silver and bronze.
func goodDivisor(goodKey string) float64 {
	if d, ok := goodDivisors[goodKey]; ok {
		return d
	}
	return 1.0
}

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
// goodKey for spread purposes. The floor is holderMinStock (one pre-mig-136 raw
// unit) expressed in the good's OWN post-136 scale — see holderMinStock and
// goodDivisors for why a flat threshold cannot be reused across every good.
func CountsAsHolder(goodKey string, stock float64) bool {
	return stock >= holderMinStock/goodDivisor(goodKey)
}
