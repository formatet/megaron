package religion

import "strings"

// TempleGod is a god a temple can be dedicated to (ORDER 15, Timothy
// 2026-10-22 — temenos_pantheon_tempel_riter.md §Beslut ORDER 15). Each city
// holds at most one temple per god; the temple's god decides which prayers it
// opens. ID is the same stable key as PrayerSpec.GodID.
type TempleGod struct {
	ID       string
	Name     string
	Cultures []string
	// Tastes multiply a good's divine value in this god's rich offering (Timothy 2026-11-04, alternative A):
	// above 1 the god asks less of it. Goods not listed count at 1. Codex temples.md prints the table.
	Tastes map[string]float64
}

// TempleGods are the gods a temple can be built for now: the four Minoan gods
// with working prayers (decision 1). Other cultures' gods come later.
var TempleGods = []TempleGod{
	{ID: "potnia", Name: "Potnia", Cultures: []string{"minoan"}, Tastes: map[string]float64{"livestock": 2, "fish": 1.5}},
	{ID: "velchanos", Name: "Velchanos", Cultures: []string{"minoan"}, Tastes: map[string]float64{"wine": 2, "grain": 1.5, "livestock": 1.5}},
	{ID: "asterion", Name: "Asterion", Cultures: []string{"minoan"}, Tastes: map[string]float64{"silver": 2, "stone": 1.5}},
	{ID: "sea_daimons_stillness", Name: "the Sea Daimons of Stillness", Cultures: []string{"minoan"}, Tastes: map[string]float64{"fish": 2, "oil": 1.5}},
}

// StartTempleGod is the god every temple standing before ORDER 15 was
// dedicated to (decision 2: a fixed start god per culture, Potnia for the
// Minoans; every MVP culture normalises to Minoan).
const StartTempleGod = "potnia"

// TempleGodByID looks a god up by ID.
func TempleGodByID(id string) (TempleGod, bool) {
	for _, g := range TempleGods {
		if g.ID == id {
			return g, true
		}
	}
	return TempleGod{}, false
}

// TempleName is the temple's name on every surface: "Temple of Potnia".
func TempleName(godID string) string {
	if g, ok := TempleGodByID(godID); ok {
		return "Temple of " + g.Name
	}
	return "Temple"
}

// TempleGodIDs lists the IDs, for error messages and help text.
func TempleGodIDs() string {
	ids := make([]string, len(TempleGods))
	for i, g := range TempleGods {
		ids[i] = g.ID
	}
	return strings.Join(ids, ", ")
}

// TempleTaste is the god's taste for a good in a rich offering; 1 when the god has no stated taste.
func TempleTaste(godID, good string) float64 {
	if g, ok := TempleGodByID(godID); ok {
		if t, ok := g.Tastes[good]; ok && t > 0 {
			return t
		}
	}
	return 1
}
