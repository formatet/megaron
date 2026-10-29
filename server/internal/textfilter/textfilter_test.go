package textfilter

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

func seeded() func(int) int { return rand.New(rand.NewPCG(1, 2)).IntN }

func TestCoarseWordsAreCaught(t *testing.T) {
	for _, in := range []string{
		"fuck", "FUCK", "Fucking", "motherfucker", "fuuuuck", "f u c k", "f.u.c.k", "f-u-c-k",
		"fuсk", // Cyrillic с
		"cunt", "nigger", "n1gg3r", "faggot", "slut", "whore",
		"fitta", "fiiitta", "kuk", "knulla", "rövhål", "hora", "neger", "subba",
	} {
		if _, hits := Letter(in, seeded()); hits == 0 {
			t.Errorf("%q: no hit", in)
		}
	}
	if _, hits := Letter("sh1thead cunt", seeded()); hits != 1 {
		t.Errorf("mild shit passes, cunt is caught: got %d hits", hits)
	}
}

func TestInnocentWordsPass(t *testing.T) {
	for _, in := range []string{
		"Scunthorpe", "Essex", "Sussex", "assassin", "class", "profitable", "fitting", "cocktail",
		"Dickens", "spice", "shitake", "hell", "damn", "fan", "jävla", "helvete", "skit",
		"Pakistan", "Mongolia", "cum laude", "a chink in the wall", "hour", "Knossos", "Mykenai",
		"1000 silver for 3 bronze", "arrive at 18:34", "horisont", "negerande",
	} {
		if out, hits := Letter(in, seeded()); hits != 0 {
			t.Errorf("%q flagged as %q", in, out)
		}
	}
}

func TestLetterKeepsTextAroundTheWord(t *testing.T) {
	out, hits := Letter("Send grain, you fucking cunt. Now!", seeded())
	if hits != 2 {
		t.Fatalf("hits = %d, want 2 (%q)", hits, out)
	}
	if !strings.HasPrefix(out, "Send grain, you ") || !strings.HasSuffix(out, ". Now!") {
		t.Fatalf("surrounding text changed: %q", out)
	}
	if utf8.RuneCountInString(out) != utf8.RuneCountInString("Send grain, you fucking cunt. Now!") {
		t.Fatalf("length changed: %q", out)
	}
	if strings.ContainsAny(out, "fkcnu") && strings.Contains(out, "fuck") {
		t.Fatalf("word survived: %q", out)
	}
	if again, n := Letter(out, seeded()); n != 0 {
		t.Fatalf("grawlix is itself flagged: %q", again)
	}
}

func TestNameReplacementIsAPlausibleName(t *testing.T) {
	intn := seeded()
	for _, in := range []string{"Fuck Bay", "Kukhamn", "Mount Cunt", "Nigger Rock"} {
		out, hits := Name(in, intn)
		if hits != 1 {
			t.Fatalf("%q: hits %d", in, hits)
		}
		if utf8.RuneCountInString(out) != utf8.RuneCountInString(in) {
			t.Errorf("%q → %q: length changed", in, out)
		}
		for _, r := range out {
			if !(r == ' ' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				t.Errorf("%q → %q: %q breaks the name rule", in, out, r)
			}
		}
		if _, n := Name(out, seeded()); n != 0 {
			t.Errorf("%q → %q is itself flagged", in, out)
		}
	}
	a, _ := Name("Fuck", seeded())
	b, _ := Name("Fuck", rand.New(rand.NewPCG(9, 9)).IntN)
	if a == b {
		t.Errorf("replacement is not random: %q twice", a)
	}
	if a[0] < 'A' || a[0] > 'Z' {
		t.Errorf("capital lost: %q", a)
	}
}
