package province

import (
	"strings"
	"testing"
)

// A generated Wanax name must never land on one already assigned — two
// players answering to the same public name is exactly the confusion
// wanax_name's partial unique index (mig 109) exists to prevent.
func TestWanaxNameExcluding_SkipsTakenNames(t *testing.T) {
	taken := map[string]bool{}
	for _, n := range WanaxNamePool[:len(WanaxNamePool)-1] {
		taken[strings.ToLower(n)] = true
	}
	want := WanaxNamePool[len(WanaxNamePool)-1]
	for i := 0; i < 50; i++ {
		if got := wanaxNameExcluding(taken); got != want {
			t.Fatalf("expected the one free name %q, got %q", want, got)
		}
	}
}

// Pool exhausted (< 100 players, per the plan): the next Wanax takes a roman
// ordinal suffix rather than a duplicate — same fallback as settlement names.
func TestWanaxNameExcluding_OrdinalWhenPoolExhausted(t *testing.T) {
	taken := map[string]bool{}
	for _, n := range WanaxNamePool {
		taken[strings.ToLower(n)] = true
	}

	got := wanaxNameExcluding(taken)
	if !strings.HasSuffix(got, " II") {
		t.Fatalf("expected a %q suffix after the pool ran out, got %q", "II", got)
	}

	// Second ordinal: every " II" is spoken for too.
	for _, n := range WanaxNamePool {
		taken[strings.ToLower(n+" II")] = true
	}
	if got := wanaxNameExcluding(taken); !strings.HasSuffix(got, " III") {
		t.Fatalf("expected a %q suffix, got %q", "III", got)
	}
}

// The name must never come back empty, however exhausted the pool — an empty
// wanax_name would either violate the partial unique index the moment a
// second player hit it, or silently show as a blank Wanax to everyone else.
func TestWanaxNameExcluding_NeverEmpty(t *testing.T) {
	taken := map[string]bool{}
	for i := 0; i < 5; i++ {
		got := wanaxNameExcluding(taken)
		if got == "" {
			t.Fatalf("round %d: got empty name", i)
		}
		taken[strings.ToLower(got)] = true
	}
	// Exhaust the whole base pool, then two ordinal rounds, and check still
	// never empty.
	for _, n := range WanaxNamePool {
		taken[strings.ToLower(n)] = true
	}
	for ord := 2; ord <= 3; ord++ {
		for _, n := range WanaxNamePool {
			taken[strings.ToLower(n+" "+roman(ord))] = true
		}
	}
	if got := wanaxNameExcluding(taken); got == "" {
		t.Fatal("got empty name after exhausting base pool + two ordinal rounds")
	}
}

// Case and stray whitespace must not open a loophole — same guard as
// TestSettlementNameExcluding_CaseInsensitive.
func TestWanaxNameExcluding_CaseInsensitive(t *testing.T) {
	taken := map[string]bool{}
	for _, n := range WanaxNamePool {
		taken[strings.ToLower(n)] = true
	}
	got := wanaxNameExcluding(taken)
	for _, n := range WanaxNamePool {
		if strings.EqualFold(got, n) {
			t.Fatalf("generated %q collides case-insensitively with taken %q", got, n)
		}
	}
}
