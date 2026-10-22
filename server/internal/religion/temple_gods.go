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
}

// TempleGods are the gods a temple can be built for now: the four Minoan gods
// with working prayers (decision 1). Other cultures' gods come later.
var TempleGods = []TempleGod{
	{ID: "potnia", Name: "Potnia", Cultures: []string{"minoan"}},
	{ID: "velchanos", Name: "Velchanos", Cultures: []string{"minoan"}},
	{ID: "asterion", Name: "Asterion", Cultures: []string{"minoan"}},
	{ID: "sea_daimons_stillness", Name: "the Sea Daimons of Stillness", Cultures: []string{"minoan"}},
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
