package religion

import "math"

// An offering is composed, not prescribed: the Wanax decides what to carry to
// the altar. What it is worth follows from three things multiplied together —
// how much you brought, what the world can spare (DivineValue), and whether
// this particular god wants it at all (affinity).
//
// The god's taste is what makes composition a decision rather than arithmetic.
// Without it, "compose an offering" is just addition with extra steps.

const (
	// affinityDefault applies to any good a god has no stated opinion about.
	// Not zero: an unfavoured gift is a lesser gift, never an insult.
	affinityDefault = 1.0

	// Bounds on how far the offering can move the odds. Unchanged from the
	// fixed-recipe era (riteOfferModFat/Stingy in api/handlers) so every
	// calibration made against the old formula carries over untouched.
	offerModMax = 0.10
	offerModMin = -0.15

	// stingyRatioFloor is the fraction of the baseline at which the penalty
	// bottoms out — the old riteOfferMultiplierMin, kept so the two eras agree
	// on what "a stingy offering" costs.
	stingyRatioFloor = 0.5

	// offerModSlope converts "worth relative to what the god expects" into odds.
	// Saturating rather than linear, and that is the point: an offering can
	// never buy what standing must earn. Kharis is the relationship; the
	// offering is a courtesy.
	offerModSlope = 0.20

	// A gift of many kinds pleases more than a heap of one — and the only way to
	// assemble many kinds is to trade for them. Small on purpose: it should
	// tilt the decision, never dictate it.
	varietyBonusPerGood = 0.015
	varietyBonusMax     = 0.045
)

// OfferingWorth values a composed offering in divine terms.
//
// divineValues is good_key→value from divine_valuations (world scarcity, already
// smoothed). favours is the god's taste; goods absent from it weigh
// affinityDefault. A good absent from divineValues is worth nothing to anyone —
// it is not in the world's economy at all.
func OfferingWorth(offering map[string]float64, divineValues map[string]float64, favours map[string]float64) float64 {
	var worth float64
	for good, amount := range offering {
		if amount <= 0 {
			continue
		}
		value, known := divineValues[good]
		if !known {
			continue
		}
		affinity := affinityDefault
		if a, ok := favours[good]; ok {
			affinity = a
		}
		worth += amount * value * affinity
	}
	return worth
}

// DistinctGoods counts the kinds actually brought (positive amounts only).
func DistinctGoods(offering map[string]float64) int {
	n := 0
	for _, amount := range offering {
		if amount > 0 {
			n++
		}
	}
	return n
}

// OfferMod turns an offering's worth into a nudge on the success chance.
//
// baseline is what this god expects for this prayer, in the same divine units.
// An offering exactly at the baseline is neutral (mod 0) — it neither impresses
// nor offends. Above it saturates toward offerModMax; below it falls toward
// offerModMin, which is the steeper direction: the gods notice stinginess more
// keenly than generosity.
func OfferMod(worth, baseline float64, distinctGoods int) float64 {
	if baseline <= 0 {
		return 0
	}
	ratio := worth / baseline

	var mod float64
	switch {
	case ratio >= 1:
		// Saturating: doubling a generous offering does not double the favour.
		mod = offerModMax * (1 - math.Exp(-(ratio-1)/offerModSlope*0.5))
	default:
		// Below baseline the floor is reached at HALF the baseline, not at an
		// empty altar. That preserves the old scale's anchor points exactly:
		// riteOfferMultiplier 0.5 (the old minimum) was the full −0.15, and
		// 2.0 the full +0.10. Mapping the floor to ratio 0 instead would have
		// made a halved offering cost less than a doubled one gains — measured,
		// and the reason this branch is not a plain (1 − ratio).
		shortfall := (1 - ratio) / (1 - stingyRatioFloor)
		if shortfall > 1 {
			shortfall = 1
		}
		mod = offerModMin * shortfall
	}

	if distinctGoods > 1 {
		bonus := float64(distinctGoods-1) * varietyBonusPerGood
		if bonus > varietyBonusMax {
			bonus = varietyBonusMax
		}
		mod += bonus
	}

	if mod > offerModMax {
		mod = offerModMax
	}
	if mod < offerModMin {
		mod = offerModMin
	}
	return mod
}

// OfferingShortfall reports how far short of the baseline an offering fell, as a
// fraction (0 = at or above baseline, 1 = nothing brought). Feeds the actionable
// error string — "your offering reached 60% of what Apollon expects" beats
// "invalid offering", for a human and an LLM agent alike.
func OfferingShortfall(worth, baseline float64) float64 {
	if baseline <= 0 || worth >= baseline {
		return 0
	}
	return 1 - worth/baseline
}

// TraditionalBaseline is what this god customarily expects, valued at today's
// divine prices: the prayer's inherited fixed recipe.
//
// Deriving the baseline from the old recipe rather than hand-tuning a number per
// prayer means (a) 18 constants never drift apart, (b) the baseline moves with
// scarcity exactly as the offering does, so a shortage raises what the god
// expects AND what your gift is worth — no accidental cheapening, and (c) the
// pre-composition behaviour is reproduced by construction: bring the traditional
// recipe and you land on mod 0, precisely where the fixed-recipe era put you.
func TraditionalBaseline(spec PrayerSpec, divineValues map[string]float64) float64 {
	return OfferingWorth(spec.Offering, divineValues, FavoursFor(spec))
}
