package province

import "math/rand"

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

// SettlementNameForCulture picks a random culture-appropriate city name.
func SettlementNameForCulture(culture string) string {
	names := CultureSettlementNames[culture]
	if len(names) == 0 {
		return "Unknown Settlement"
	}
	return names[rand.Intn(len(names))]
}
