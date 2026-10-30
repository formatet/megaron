package religion

// The gods' hand at sea — R2 storm, mild (Timothy 2026-10-30,
// megaron_religion_talforslag_20261030 §R2). A Favorable realm's ship may be
// spared a storm's blow; a Wrathful realm's ship may lose one more hull. The
// chance is shown before sailing (kharis standing) and named in the outcome.
const (
	StormSpareChance    = 0.15
	StormWrathChance    = 0.15
	StormWrathExtraHull = 1
)

// StormTilt returns the chance a storm spares the ship and the chance it takes
// StormWrathExtraHull more, for the owner's kharis. At most one is non-zero.
func StormTilt(kharis float64) (spare, wrath float64) {
	switch {
	case kharis >= MoodFavorable:
		return StormSpareChance, 0
	case kharis < MoodSuspicious:
		return 0, StormWrathChance
	}
	return 0, 0
}
