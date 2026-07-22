package province

import (
	"strings"
	"testing"
)

// A generated name must never land on one the world already holds — that is the
// whole point of the guard (two "Dodona" cost a CLI disambiguation fix, 794326a).
func TestSettlementNameExcluding_SkipsTakenNames(t *testing.T) {
	pool := CultureSettlementNames["akhaier"]
	taken := map[string]bool{}
	for _, n := range pool[:len(pool)-1] {
		taken[strings.ToLower(n)] = true
	}
	want := pool[len(pool)-1]
	for i := 0; i < 50; i++ {
		if got := settlementNameExcluding("akhaier", taken); got != want {
			t.Fatalf("expected the one free name %q, got %q", want, got)
		}
	}
}

// Pool exhausted: the next city takes the epithet rather than a duplicate.
func TestSettlementNameExcluding_EpithetWhenPoolExhausted(t *testing.T) {
	pool := CultureSettlementNames["minoan"]
	taken := map[string]bool{}
	for _, n := range pool {
		taken[strings.ToLower(n)] = true
	}

	got := settlementNameExcluding("minoan", taken)
	if !strings.HasSuffix(got, " II") {
		t.Fatalf("expected a %q epithet after the pool ran out, got %q", "II", got)
	}

	// Second ordinal: every " II" is spoken for too.
	for _, n := range pool {
		taken[strings.ToLower(n+" II")] = true
	}
	if got := settlementNameExcluding("minoan", taken); !strings.HasSuffix(got, " III") {
		t.Fatalf("expected a %q epithet, got %q", "III", got)
	}
}

// Case and stray whitespace must not open a loophole: TakenSettlementNames keys
// lower-cased and trimmed, so the generator has to compare the same way.
func TestSettlementNameExcluding_CaseInsensitive(t *testing.T) {
	pool := CultureSettlementNames["hatti"]
	taken := map[string]bool{}
	for _, n := range pool {
		taken[strings.ToLower(n)] = true
	}
	got := settlementNameExcluding("hatti", taken)
	for _, n := range pool {
		if strings.EqualFold(got, n) {
			t.Fatalf("generated %q collides case-insensitively with taken %q", got, n)
		}
	}
}

// An unknown culture still gets a name — a founding never fails over naming.
func TestSettlementNameExcluding_UnknownCulture(t *testing.T) {
	if got := settlementNameExcluding("phrygian", map[string]bool{}); got != "Unknown Settlement" {
		t.Fatalf("unknown culture: got %q", got)
	}
	taken := map[string]bool{"unknown settlement": true}
	if got := settlementNameExcluding("phrygian", taken); got != "Unknown Settlement II" {
		t.Fatalf("unknown culture, name taken: got %q", got)
	}
}

func TestRoman(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{{2, "II"}, {3, "III"}, {4, "IV"}, {9, "IX"}, {14, "XIV"}, {40, "XL"}, {2026, "MMXXVI"}} {
		if got := roman(tc.n); got != tc.want {
			t.Errorf("roman(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
