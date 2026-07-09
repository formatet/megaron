package combat

import "math/rand"

// rollFortune returns a combat fortune modifier ∈ [-0.2, +0.2].
// Positive favours the attacker; negative favours the defender.
//
// The base roll is uniform in [-0.2, +0.2]. Kharis delta shifts the centre:
// high attacker kharis → skews positive; high defender kharis → skews negative.
// Kharis effect is capped at ±0.1 (half the fortune range), so it biases but never
// determines the outcome by itself. normalisation cap: 75 (kharis 0-100 rescale,
// Timothy 2026-07-09 — temenos_kharis.md §"KANONISK OMDESIGN"; was 1500 on the old
// 0-2000 scale). Strawman — temenos_balans_spakar.md §9.
//
// The caller rolls fortune ONCE per battle and passes it to ResolveStrengths/Resolve.
// Storing it in the event payload satisfies Fas 2.3 (result, not intent, in events).
func rollFortune(attackerKharis, defenderKharis float64) float64 {
	const (
		fortuneRange = 0.20
		kharisCap    = 75
		kharisWeight = 0.10
	)
	base := rand.Float64()*fortuneRange*2 - fortuneRange
	norm := (attackerKharis - defenderKharis) / kharisCap
	if norm > 1 {
		norm = 1
	} else if norm < -1 {
		norm = -1
	}
	f := base + norm*kharisWeight
	if f > fortuneRange {
		return fortuneRange
	}
	if f < -fortuneRange {
		return -fortuneRange
	}
	return f
}
