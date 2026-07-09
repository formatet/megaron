// Package shipnames supplies Bronze-Age-appropriate ship name suggestions for
// the naval recruit flow (ship-build overhaul, Timothy 2026-07-09).
//
// The name pool below is Timothy's real Minoan/Cretan ship-name list
// (minoan_ship_names.csv, 280 names in three styles: English, Greek, Linear B
// romanization). Only Minoan exists today — every other culture falls back to
// it (see Suggest's doc comment).
//
// G1 placement: zero internal deps (like clock/events); any package may
// import this.
package shipnames

import (
	"embed"
	"encoding/csv"
	"math/rand"
)

//go:embed minoan_ship_names.csv
var namesFS embed.FS

// Style is a ship-naming convention within a culture's pool.
type Style string

const (
	StyleEnglish Style = "English"  // evocative readable names, e.g. "Bull of the Deep"
	StyleGreek   Style = "Greek"    // transliterated, e.g. "Tauros Bathys"
	StyleLinearB Style = "Linear B" // Mycenaean romanization
)

// minoanPool holds the parsed CSV, keyed by style.
var minoanPool = mustParsePool("minoan_ship_names.csv")

func mustParsePool(filename string) map[Style][]string {
	f, err := namesFS.Open(filename)
	if err != nil {
		panic("shipnames: open " + filename + ": " + err.Error())
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		panic("shipnames: parse " + filename + ": " + err.Error())
	}

	pool := make(map[Style][]string)
	for i, rec := range records {
		if i == 0 || len(rec) < 2 {
			continue // header row ("name,style") or malformed line
		}
		s := Style(rec[1])
		pool[s] = append(pool[s], rec[0])
	}
	return pool
}

// Suggest picks a culture-appropriate ship name, preferring one not already
// in `taken` (a Wanax's existing fleet names) when that's easy to arrange —
// a repeat isn't a hard error, just tries a few times before giving up.
//
// Only the Minoan pool exists today; any culture falls back to it.
// TODO: give the other five cultures (akhaier, khemetiu, knaani, hatti,
// thrakes) their own pools, and consider letting a culture pick a Style
// (Greek/Linear B) other than English.
func Suggest(culture string, taken map[string]bool) string {
	pool := minoanPool[StyleEnglish]
	if len(pool) == 0 {
		return "Unnamed Vessel"
	}
	name := pool[rand.Intn(len(pool))]
	for i := 0; i < 10 && taken[name]; i++ {
		name = pool[rand.Intn(len(pool))]
	}
	return name
}
