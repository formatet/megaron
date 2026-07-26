// Package shipnames supplies Bronze-Age-appropriate ship name suggestions for
// the naval recruit flow (ship-build overhaul, Timothy 2026-07-09).
//
// Den minoiska poolen är Timothys egen lista (minoan_ship_names.csv, 280 namn i
// tre stilar). De fem andra kulturerna fick egna pooler 2026-07-26 (Fas 3 i
// megaron_aktorer_plan.md) — mindre, men skrivna ur namnstandardens sex
// temafamiljer (§7.6) så att varje kultur täcker dem alla:
//
//	hemstad · gudomligt · hav/natur · last/handel · bedrift/minne · oikos
//
// Undvik i alla pooler: HMS, serienummer, moderna nationsbeteckningar,
// "Destroyer", "Victory-class" och annan senare marin konvention.
//
// G1 placement: zero internal deps (like clock/events); any package may
// import this.
package shipnames

import (
	"embed"
	"encoding/csv"
	"math/rand"
)

//go:embed *.csv
var namesFS embed.FS

// Style is a ship-naming convention within a culture's pool.
type Style string

const (
	StyleEnglish Style = "English"  // evocative readable names, e.g. "Bull of the Deep"
	StyleGreek   Style = "Greek"    // transliterated, e.g. "Tauros Bathys"
	StyleLinearB Style = "Linear B" // Mycenaean romanization
)

// Kulturnyckeln är den som världen använder (se render/map.js CULTURE_ACCENT och
// worldbuilding-doken). En okänd kultur faller tillbaka på den minoiska poolen —
// ett saknat namn får aldrig blockera ett skeppsbygge.
var pools = map[string]map[Style][]string{
	"minoan":   mustParsePool("minoan_ship_names.csv"),
	"akhaier":  mustParsePool("akhaier_ship_names.csv"),
	"khemetiu": mustParsePool("khemetiu_ship_names.csv"),
	"knaani":   mustParsePool("knaani_ship_names.csv"),
	"hatti":    mustParsePool("hatti_ship_names.csv"),
	"thrakes":  mustParsePool("thrakes_ship_names.csv"),
}

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
func Suggest(culture string, taken map[string]bool) string {
	return SuggestWith(rand.Intn, culture, taken)
}

// SuggestWith är samma sak med en injicerad slumpkälla, så tester kan vara
// reproducerbara. Den globala math/rand-källan gjorde varje namntest till ett
// lotteri, vilket är precis den sorts icke-determinism riggarnas arbetsregler
// förbjuder på bildsidan — samma krav gäller här.
//
// intn har math/rand.Intn:s kontrakt: returnerar [0, n).
func SuggestWith(intn func(int) int, culture string, taken map[string]bool) string {
	pool := pools[culture][StyleEnglish]
	if len(pool) == 0 {
		pool = pools["minoan"][StyleEnglish]
	}
	if len(pool) == 0 {
		return "Unnamed Vessel"
	}
	name := pool[intn(len(pool))]
	for i := 0; i < 10 && taken[name]; i++ {
		name = pool[intn(len(pool))]
	}
	return name
}
