package religion

// The gods' hand in battle — R2 battle (Timothy 2026-10-30; raised from 5% to 15% 2026-11-12,
// megaron_religion_talforslag_20261030 §R2). A Favorable realm's men fight
// BattleFavorFactor stronger, a Wrathful realm's BattleWrathFactor weaker. The
// mood is fixed when a unit enters the battle and named in the outcome.
const (
	BattleFavorFactor = 1.15
	BattleWrathFactor = 0.85
)

// BattleDivine returns the gods' hand a unit carries into battle for the
// owner's kharis: "favor", "wrath", or "" for none.
func BattleDivine(kharis float64) string {
	switch {
	case kharis >= MoodFavorable:
		return "favor"
	case kharis < MoodSuspicious:
		return "wrath"
	}
	return ""
}

// BattleFactor is the strength multiplier for a stored BattleDivine value.
func BattleFactor(divine string) float64 {
	switch divine {
	case "favor":
		return BattleFavorFactor
	case "wrath":
		return BattleWrathFactor
	}
	return 1
}
