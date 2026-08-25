package religion

// The single canonical kharis→mood tier table (0-100 scale). Every reader —
// kharis.deriveMood, api/handlers.kharisToMood, and PrayerSpec.MinKharis —
// must read these constants instead of restating the numbers, or the
// tables drift apart again (megaron_plan_kultbrunnen.md §6, 2026-08-25:
// mood's lowest tier was 10 while the lowest prayer gate was 5 — a Wanax
// could satisfy the gods' lowest prayer while still being called Wrathful).
const (
	MoodFavorable   = 60.0 // Favorable / overdadig
	MoodIndifferent = 30.0 // Indifferent / vardig
	MoodSuspicious  = 5.0  // Suspicious / tveksam — also the lowest prayer tier (MinKharis)
)
