package kharis

import "testing"

// Nådefristen (Timothy 2026-08-25, alternativ A). Straffet är ett tärningskast,
// så BESLUTET att kasta måste gå att bevisa utan att kasta — därför testas
// shouldRollPunishment och inte processMaintenance.
func TestShouldRollPunishment_FoundingGrace(t *testing.T) {
	cases := []struct {
		name               string
		kharis             float64
		ticksSinceFounding int
		want               bool
	}{
		// Själva buggen. En nygrundad Wanax står på startkharis 25, under
		// punishThreshold 30, utan tempel — alltså på missed-grenen. Före fixen
		// rullade gudarna mot henne från dygn ett.
		{"nygrundad på startkharis rullas inte", 25, 0, false},

		// Fristens sista dygn skyddar fortfarande, även när hon hunnit tyna hela
		// vägen till golvet. Decay stängs inte av av fristen — bara straffet.
		{"sista dygnet i fristen skyddar även på golvet", kharisFloor, foundingGraceTicks - 1, false},

		// Och den släpper på exakt rätt dygn, inte ett senare.
		{"fristen slutar precis vid gränsen", kharisFloor, foundingGraceTicks, true},

		// Fristen är inte en generell amnesti: över tröskeln rullas ingenting
		// ändå, och det ska gälla oförändrat efter att fristen gått ut.
		{"på tröskeln rullas inget, fristen ovidkommande", punishThreshold, foundingGraceTicks, false},
		{"strax under tröskeln rullas det", punishThreshold - 0.01, foundingGraceTicks, true},

		// Dokumenterar ÄRLIGT att alternativ A inte stänger brunnen. En Wanax som
		// långt senare står tempellös på taket 25 är straffbar igen — hon har haft
		// gott om tid att resa ett tempel. Faller det här fallet har någon råkat
		// bygga alternativ B i stället.
		{"tempellös långt efter fristen är straffbar igen", TempleKharisCeiling(0), 1000, true},

		// Trasig data ska luta åt att inte straffa: en bakåtfylld rad eller en skev
		// klocka får inte bli en pestvåg.
		{"negativt tick läses som inom fristen", kharisFloor, -5, false},
	}

	for _, c := range cases {
		if got := shouldRollPunishment(c.kharis, c.ticksSinceFounding); got != c.want {
			t.Errorf("%s: shouldRollPunishment(%.2f, %d) = %v, vill ha %v",
				c.name, c.kharis, c.ticksSinceFounding, got, c.want)
		}
	}
}

// Roten under hela raden, fastspikad så att den inte tyst repareras åt fel håll:
// taket för en tempellös Wanax ligger UNDER straffgränsen. Så länge det gäller är
// straffet omöjligt att spela sig ur utan att bygga ett tempel, och nådefristen är
// det som gör starten rimlig. Höjer någon taket över tröskeln har premissen för
// fristen ändrats och den bör omprövas, inte behållas av slentrian.
func TestTempleCeiling_LevelZeroSitsBelowThePunishThreshold(t *testing.T) {
	if TempleKharisCeiling(0) >= punishThreshold {
		t.Fatalf("premissen för nådefristen är borta: tempellöst tak %.0f når nu "+
			"straffgränsen %.0f — ompröva foundingGraceTicks i stället för att "+
			"lappa det här testet", TempleKharisCeiling(0), punishThreshold)
	}
}

// Literal approved boundary: do not derive these ticks from the constant,
// or reverting that constant would silently move the test's expectations.
func TestShouldRollPunishment_Grace144(t *testing.T) {
	for _, tc := range []struct {
		kharis float64
		age    int
		want   bool
	}{
		{1, 60, false}, {1, 143, false}, {29.99, 143, false}, {1, 144, true},
		{29.99, 144, true}, {30, 144, false}, {60, 144, false}, {1, -1, false},
	} {
		if got := shouldRollPunishment(tc.kharis, tc.age); got != tc.want {
			t.Errorf("kharis=%g age=%d: roll=%v want=%v", tc.kharis, tc.age, got, tc.want)
		}
	}
	if FoundingGraceTicks() != 144 {
		t.Errorf("published grace=%d want144", FoundingGraceTicks())
	}
	if punishThreshold != 30 || punishProbability != 0.30 {
		t.Fatal("punishment threshold or probability changed")
	}
}
