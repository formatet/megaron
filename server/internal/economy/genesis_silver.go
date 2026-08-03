package economy

// Genesis silver — the sanctioned faucets.
//
// B3 (Timothy 2026-08-02): silver enters the game ONLY via starting silver and
// mines. Everything else moves silver that already exists. This file is the
// starting-silver half, kept apart from the granary (sitos.go, sitos_tick.go)
// so that "the granary never touches silver" is a property you can check by
// reading a file rather than by tracing a call graph.
//
// Until migration 106 there was a third faucet: every founding also minted a
// Sitos fund seed, pop x 10.5, so a ~2000-pop colony printed ~21 000 silver
// into a world holding 106 678 liquid. Expansion was a printing press. It is
// gone.

// dailyGrainNeedInSilver prices a settlement's daily food need at grain's
// base_value. It anchors the genesis silver figures so they stay pop-invariant:
// a pop=100 colony and a pop=20000 capital start with the same number of days
// of coverage. The granary is measured in food and never in silver, so it does
// not use this — see DailyFoodNeed.
func dailyGrainNeedInSilver(population int, grainBaseValue float64) float64 {
	return DailyFoodNeed(population) * grainBaseValue
}

// GenesisSilverLiquid returns a new settlement's starting LIQUID silver (goods
// amount) and its silver-good cap, both pop-anchored to grain-need so the ratio
// stays pop-invariant. This is one of the TWO sanctioned silver faucets in the
// game (B3: starting silver and mines, full stop) — the other being the mines.
// Bounded via genesisNeed.
func GenesisSilverLiquid(population int, grainBaseValue float64, cfg SitosConfig) (seed, cap float64) {
	need := genesisNeed(population, grainBaseValue)
	return need * cfg.SilverStartDays, need * cfg.SilverLiquidCapDays
}

// genesisNeed is dailyGrainNeedInSilver with the population bounded to
// MaxGenesisPopulation. Genesis is a place silver is created rather than moved,
// so it is a place where a corrupt population turns into minted silver — and on
// 2026-07-23 it did: a colonising unit whose size had been inflated to 2 976 790
// by the unbounded `divine_recruits` blessing founded a colony holding 99.5 % of
// the world's silver. The seed must never price itself against a caller's
// population figure without a ceiling.
func genesisNeed(population int, grainBaseValue float64) float64 {
	if population > MaxGenesisPopulation {
		population = MaxGenesisPopulation
	}
	return dailyGrainNeedInSilver(population, grainBaseValue)
}
