package religion

// An offering is composed, not prescribed: the Wanax decides what to carry to
// the altar. What it is worth is amount x the good's fixed divine value
// (FixedDivineValues) x the god's taste for it (affinity). That worth sets the
// gift step of the prayer (how many times the baseline the gift reaches).
//
// The god's taste is what makes composition a decision rather than arithmetic:
// incense, for one, weighs more in the favours tables of prayers.go.

// affinityDefault applies to any good a god has no stated opinion about.
// Not zero: an unfavoured gift is a lesser gift, never an insult.
const affinityDefault = 1.0

// OfferingWorth values a composed offering in divine terms.
//
// divineValues is good_key→value from LoadDivineValues (the fixed list).
// favours is the god's taste; goods absent from it weigh
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
