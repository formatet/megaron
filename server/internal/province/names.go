package province

import (
	"context"
	"math/rand"
	"strings"

	"github.com/google/uuid"
)

// CultureSettlementNames maps culture keys to Bronze Age city name pools.
// akhaier/khemetiu/knaani/hatti sourced from bronsaleders_cities_mytologi.csv;
// thrakes/minoan from archaeological and Linear B sources.
var CultureSettlementNames = map[string][]string{
	// Mykensk + Minoisk (Kreta)
	"akhaier": {
		"Ilion", "Mykene", "Tiryns", "Argos", "Pylos", "Sparta",
		"Thebe", "Korint", "Midea", "Dendra", "Orchomenos", "Gla",
		"Asine", "Knossos", "Phaistos", "Mallia", "Zakros", "Kydonia",
		"Gournia", "Palaikastro", "Vasiliki", "Aten", "Eleusis",
		"Iolkos", "Amyklai", "Kalydon", "Dodona", "Nauplia",
		"Epidauros", "Megara",
	},
	// Forntida Egypten
	"khemetiu": {
		"Memfis", "Thebe", "Heliopolis", "Abydos", "Tanis",
		"Dendera", "Edfu", "Kom Ombo", "Aswan", "Luxor",
		"Karnak", "Amarna", "Herakleopolis", "Sais", "Bubastis",
		"Buto", "Mendes", "Giza", "Saqqara", "Busiris",
		"Hermopolis", "Coptos", "Elephantine", "Philae", "Abu Simbel",
		"Avaris",
	},
	// Kanaanitisk/Fenicisk
	"knaani": {
		"Ugarit", "Byblos", "Sidon", "Tyre", "Jeriko",
		"Hazor", "Megiddo", "Gezer", "Lachish", "Beth Shan",
		"Shechem", "Gibeon", "Hebron", "Beersheba", "Jerusalem",
		"Joppe", "Acco", "Dor", "Ashkelon", "Ashdod",
		"Ekron", "Gath", "Gaza", "Heshbon", "Rabbah",
		"Sodom", "Gomorra", "Ai",
	},
	// Hettitisk
	"hatti": {
		"Hattusa", "Wilusa", "Kanesh", "Sapinuwa", "Milawata",
		"Miletos", "Sardis", "Ephesos", "Carchemish", "Zalpa",
		"Kussara", "Purushanda", "Arzawa", "Ahhiyawa", "Zippalanda",
		"Arinna", "Nerik", "Tarhuntassa", "Samuha", "Ankuwa",
		"Harran", "Ebla", "Mari", "Urkesh", "Assur",
	},
	// Thrakisk
	"thrakes": {
		"Seuthopolis", "Kabyle", "Kypsela", "Maroneia", "Abdera",
		"Ainos", "Samothrake", "Doriskos", "Perinthos", "Byzantion",
		"Anchialos", "Odessus", "Apollonia", "Mesambria", "Istros",
		"Tomis", "Kallatis", "Bizone", "Tyras", "Kardia",
		"Lysimacheia", "Odrysai", "Eion", "Amphipolis", "Oisyme",
	},
	// Minoisk (Kretensisk sjöfararkultur)
	"minoan": {
		"Knossos", "Phaistos", "Mallia", "Zakros", "Kydonia",
		"Gournia", "Palaikastro", "Vasiliki", "Hagia Triada", "Archanes",
		"Amnissos", "Tylissos", "Kommos", "Akrotiri", "Mochlos",
		"Pseira", "Petras", "Vathypetro", "Nirou Chani", "Itanos",
		"Praisos", "Lato", "Chersonesos", "Siteia", "Labyrinthos",
	},
}

// SettlementNameForCulture picks a random culture-appropriate city name. It knows
// nothing about the world, so two cities can end up sharing a name — prefer
// UniqueSettlementName wherever a world is at hand.
func SettlementNameForCulture(culture string) string {
	names := CultureSettlementNames[culture]
	if len(names) == 0 {
		return "Unknown Settlement"
	}
	return names[rand.Intn(len(names))]
}

// UniqueSettlementName picks a culture-appropriate name no settlement in the world
// already holds. A name lookup must never block a founding, so a DB error falls
// back to the plain random pick — a duplicate name is a nuisance, a failed
// founding is not.
func UniqueSettlementName(ctx context.Context, db Queryer, worldID uuid.UUID, culture string) string {
	taken, err := TakenSettlementNames(ctx, db, worldID)
	if err != nil {
		return SettlementNameForCulture(culture)
	}
	return settlementNameExcluding(culture, taken)
}

// TakenSettlementNames returns every settlement name in use in the world, keyed
// lower-cased: "Dodona" and "dodona" are the same city to a player reading a map.
// Razed and collapsed rows count — their ruins still carry the name.
func TakenSettlementNames(ctx context.Context, db Queryer, worldID uuid.UUID) (map[string]bool, error) {
	rows, err := db.Query(ctx, `SELECT name FROM settlements WHERE world_id = $1`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		taken[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return taken, rows.Err()
}

// SettlementNameIsTaken reports whether the world already holds a settlement by
// this name — the guard for a Wanax-chosen name, which must be refused rather
// than silently altered.
func SettlementNameIsTaken(ctx context.Context, db Queryer, worldID uuid.UUID, name string) (bool, error) {
	rows, err := db.Query(ctx,
		`SELECT 1 FROM settlements
		 WHERE world_id = $1 AND lower(btrim(name)) = lower(btrim($2)) LIMIT 1`,
		worldID, name)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

// settlementNameExcluding is the pure half of UniqueSettlementName: the culture's
// pool minus the names already spoken for.
func settlementNameExcluding(culture string, taken map[string]bool) string {
	pool := CultureSettlementNames[culture]
	if len(pool) == 0 {
		pool = []string{"Unknown Settlement"}
	}
	free := make([]string, 0, len(pool))
	for _, n := range pool {
		if !taken[strings.ToLower(n)] {
			free = append(free, n)
		}
	}
	if len(free) > 0 {
		return free[rand.Intn(len(free))]
	}
	// The world has outgrown the culture's name list — a settled land names its
	// next city after the old one: Ilion, then Ilion II, Ilion III. Terminates
	// because `taken` is finite and every ordinal adds len(pool) fresh candidates.
	for ord := 2; ; ord++ {
		suffix := " " + roman(ord)
		for _, i := range rand.Perm(len(pool)) {
			if cand := pool[i] + suffix; !taken[strings.ToLower(cand)] {
				return cand
			}
		}
	}
}

// roman renders the epithet ordinal (II, III, IV …).
func roman(n int) string {
	steps := []struct {
		v int
		s string
	}{
		{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"},
		{50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	}
	var b strings.Builder
	for _, st := range steps {
		for n >= st.v {
			b.WriteString(st.s)
			n -= st.v
		}
	}
	return b.String()
}
