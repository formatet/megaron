package economy

// The temple tithe: when a city sells a religiously coded good — wine, oil,
// purple, luxury — the priests take a share of the silver.
//
// Chosen (Timothy 2026-07-22) over a standing silver upkeep for the cult, and
// the difference matters. A flat upkeep drains cities that have no income,
// which is the desertion-spiral shape the design already warns about for bronze;
// worse, the two Wanaxes holding the world's tin were measured at ZERO silver
// the same evening. A tithe on completed trade only bites when silver actually
// moves, which is the standing rule that a sink must have a source.
//
// It is also the cult's hook into the trade leg of the MVP chain: the goods the
// gods want are the goods the temple taxes.

const (
	// titheRate is the share of trade silver the temple takes on a religious
	// good. A tenth — the word means what it says. Strawman; tune against soak.
	titheRate = 0.10

	// titheMinSilver keeps the tithe off trivial exchanges: below this the
	// priests wave the caravan through rather than count out coppers. Without
	// it every 2-silver sale spawns an event row for a rounding artefact.
	titheMinSilver = 10.0
)

// Tithe returns the silver the temple takes from a trade payment, and the
// silver that reaches the seller.
//
// hasTemple gates it: no temple, no priests to collect. That makes the tithe a
// cost of holding a cult rather than a tax on everyone, paid against the kharis
// the temple returns.
func Tithe(silver float64, goodIsReligious, hasTemple bool) (toTemple, toSeller float64) {
	if !goodIsReligious || !hasTemple || silver < titheMinSilver {
		return 0, silver
	}
	toTemple = silver * titheRate
	return toTemple, silver - toTemple
}
