package unit

import "testing"

// Ordinalen är den enda platsen i namnstandarden där en naiv implementation
// nästan alltid har fel: 11, 12 och 13 slutar på 1/2/3 men heter 11th, 12th,
// 13th. 111 och 112 likaså. Testet finns för att den regeln inte ska tappas
// vid en refaktor.
func TestOrdinal(t *testing.T) {
	cases := map[int]string{
		1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 5: "5th",
		11: "11th", 12: "12th", 13: "13th",
		21: "21st", 22: "22nd", 23: "23rd", 24: "24th",
		101: "101st", 111: "111th", 112: "112th", 113: "113th",
		121: "121st",
	}
	for n, want := range cases {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q, vill ha %q", n, got, want)
		}
	}
}

func TestLandUnitName(t *testing.T) {
	tests := []struct {
		name     string
		unitType string
		ordinal  int
		town     string
		want     string
	}{
		{"kanoniskt", "spearman", 2, "Knossos", "2nd Spearmen of Knossos"},
		{"elit", "elite_infantry", 1, "Knossos", "1st Elite Infantry of Knossos"},
		{"vagn", "war_chariot", 3, "Miletos", "3rd War Chariot of Miletos"},
		// En warband efter en kollaps har ingen försörjande stad. Ledet ska
		// falla bort helt — inte lämna ett "of " eller ett "of <tomt>".
		{"utan stad", "spearman", 2, "", "2nd Spearmen"},
		// Backfill kan lämna ordinal = 0 på rader som aldrig fick ett nummer.
		{"utan ordinal", "spearman", 0, "Knossos", "Spearmen of Knossos"},
		// Apostrof i stadsnamnet får inte behandlas specialt — namnet är data.
		{"apostrof", "spearman", 1, "K'ana", "1st Spearmen of K'ana"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LandUnitName(tt.unitType, tt.ordinal, tt.town).DisplayName; got != tt.want {
				t.Errorf("= %q, vill ha %q", got, tt.want)
			}
		})
	}
}

func TestShipDisplayName(t *testing.T) {
	tests := []struct {
		name           string
		unitType, ship string
		town, want     string
	}{
		{"kanoniskt", "galley", "White Dolphin", "Kydonia",
			"White Dolphin, Galley of Kydonia"},
		{"emporos aldrig merchantman", "merchantman", "Bearer of Cedar", "Byblos",
			"Bearer of Cedar, Emporos of Byblos"},
		{"krigsgalär", "war_galley", "Avenger of Knossos", "Knossos",
			"Avenger of Knossos, War Galley of Knossos"},
		// Ett skepp utan namn är inte ett fel: bygget kan ha lagts utan
		// namnförslag. Då bär typen namnet.
		{"namnlöst skepp", "galley", "", "Kydonia", "Galley of Kydonia"},
		{"utan stad", "galley", "White Dolphin", "", "White Dolphin, Galley"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShipDisplayName(tt.unitType, tt.ship, tt.town).DisplayName; got != tt.want {
				t.Errorf("= %q, vill ha %q", got, tt.want)
			}
		})
	}
}

func TestJourneyNames(t *testing.T) {
	if got := HostName("Ariadne").DisplayName; got != "Nomadic Host of Ariadne" {
		t.Errorf("HostName = %q", got)
	}
	if got := RunnerName("Ariadne", "Knossos", "Minos").DisplayName; got != "Ariadne's Runner from Knossos to Minos" {
		t.Errorf("RunnerName = %q", got)
	}
	if got := CaravanName("Ariadne", "silver", "Minos", "Knossos").DisplayName; got != "Ariadne's Silver Caravan from Minos to Knossos" {
		t.Errorf("CaravanName = %q", got)
	}
	// Grekiska namn slutar ofta på s. "Sarpedones's" läser fel.
	if got := RunnerName("Sarpedones", "Pylos", "Miletos").DisplayName; got != "Sarpedones' Runner from Pylos to Miletos" {
		t.Errorf("possessiv på s = %q", got)
	}
	// En karavan utan känd vara är fortfarande en karavan.
	if got := CaravanName("Ariadne", "", "Minos", "Knossos").DisplayName; got != "Ariadne's Caravan from Minos to Knossos" {
		t.Errorf("CaravanName utan vara = %q", got)
	}
}

// Ingen spelarvänd yta får säga merchantman, hoplites, agema eller hiereus.
func TestRetiredNamesNeverSurface(t *testing.T) {
	for _, typ := range []string{"spearman", "elite_infantry", "war_chariot",
		"galley", "war_galley", "merchantman", "nomadic_host"} {
		label := DisplayName(typ)
		for _, forbidden := range []string{"merchantman", "Merchantman",
			"hoplites", "Hoplites", "agema", "Agema", "hiereus", "Hiereus"} {
			if label == forbidden {
				t.Errorf("DisplayName(%q) = %q — pensionerat namn nådde spelaren", typ, label)
			}
		}
	}
	if DisplayName("merchantman") != "Emporos" {
		t.Errorf("merchantman ska visas som Emporos, fick %q", DisplayName("merchantman"))
	}
}

// TestRetiredIdentifiersStayRetired låser den delade listan (RetiredIdentifiers)
// mot den hårdkodade ordlistan ovan — svepet 2026-08-23 (megaron_plan_cli_sanning
// §E) missade `keryx disband`s FLAGGNAMN helt eftersom DisplayName()-testet
// bara kollar visningstext, aldrig identifierare på andra ytor. Ordlistan
// exporteras nu (internal/unit.RetiredIdentifiers) just så att cmd/keryx —
// som ligger nedströms om denna paketet i G1-ordningen och alltså inte kan
// testas härifrån — har EN gemensam källa att greppa mot i stället för en
// egen kopia som kan glida isär från den här (se cmd/keryx/cmd_disband_test.go).
func TestRetiredIdentifiersStayRetired(t *testing.T) {
	want := map[string]bool{"hoplites": true, "agema": true, "trireme": true, "hiereus": true}
	got := map[string]bool{}
	for _, id := range RetiredIdentifiers {
		got[id] = true
	}
	if len(got) != len(RetiredIdentifiers) {
		t.Errorf("RetiredIdentifiers har dubbletter: %v", RetiredIdentifiers)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("RetiredIdentifiers saknar %q", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("RetiredIdentifiers innehåller oväntat %q — uppdatera testet om detta är avsiktligt", id)
		}
	}
}
