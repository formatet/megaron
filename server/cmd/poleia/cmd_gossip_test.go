package main

import "testing"

func marker(name, id string, own bool) map[string]any {
	return map[string]any{"name": name, "settlement_id": id, "own": own}
}

func TestResolveMessengerDest(t *testing.T) {
	// resolveMessengerDest reads the global cfg (for the capital province id);
	// give it a non-nil value so the capital lookup is a no-op here and the
	// resolver falls back to the first own marker.
	cfg = &Config{ProvinceID: "cap-prov"}
	t.Cleanup(func() { cfg = nil })

	markers := []map[string]any{
		marker("Mykene", "self-1", true),
		marker("Korinth", "k-2", false),
	}
	wanaxes := []map[string]any{
		{"name": "Pylos", "settlement_id": "p-3"},
	}

	t.Run("resolves visible neighbour", func(t *testing.T) {
		dest, name, own, err := resolveMessengerDest(markers, wanaxes, "korinth", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dest != "k-2" || name != "Korinth" || own != "self-1" {
			t.Fatalf("got dest=%q name=%q own=%q", dest, name, own)
		}
	})

	t.Run("falls back to wanaxes list", func(t *testing.T) {
		dest, _, _, err := resolveMessengerDest(markers, wanaxes, "Pylos", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dest != "p-3" {
			t.Fatalf("got dest=%q, want p-3", dest)
		}
	})

	t.Run("rejects own settlement with actionable error", func(t *testing.T) {
		_, _, _, err := resolveMessengerDest(markers, wanaxes, "Mykene", "")
		if err == nil {
			t.Fatal("expected error sending to own settlement")
		}
		if got := err.Error(); !contains(got, "your own settlement") {
			t.Fatalf("error should mention own settlement, got: %q", got)
		}
	})

	t.Run("unknown destination", func(t *testing.T) {
		_, _, _, err := resolveMessengerDest(markers, wanaxes, "Atlantis", "")
		if err == nil {
			t.Fatal("expected error for unknown settlement")
		}
	})

	t.Run("missing own settlement", func(t *testing.T) {
		noOwn := []map[string]any{marker("Korinth", "k-2", false)}
		_, _, _, err := resolveMessengerDest(noOwn, nil, "Korinth", "")
		if err == nil {
			t.Fatal("expected error when own settlement is absent")
		}
	})

	// P9: --to only ever matches a settlement name; a Wanax (ruler) name is an
	// easy, plausible-looking mistake (it's the column right next to the city
	// name in `cities`/`diplomacy`) that used to hard-fail with a generic
	// "no settlement named" error, giving no hint that the mix-up happened.
	t.Run("wanax name instead of settlement name gives actionable error", func(t *testing.T) {
		ruledWanaxes := []map[string]any{
			{"name": "Pylos", "settlement_id": "p-3", "owner": "Nestor"},
			{"name": "Sparte", "settlement_id": "s-4", "owner": "Nestor"},
		}
		_, _, _, err := resolveMessengerDest(markers, ruledWanaxes, "Nestor", "")
		if err == nil {
			t.Fatal("expected error when --to names a Wanax, not a settlement")
		}
		got := err.Error()
		if !contains(got, "Wanax") {
			t.Errorf("error should identify the Wanax/settlement mix-up, got: %q", got)
		}
		if !contains(got, "Pylos") || !contains(got, "Sparte") {
			t.Errorf("error should name the ruler's actual cities, got: %q", got)
		}
	})
}

func TestSettlementsRuledBy(t *testing.T) {
	wanaxes := []map[string]any{
		{"name": "Pylos", "settlement_id": "p-3", "owner": "Nestor"},
		{"name": "Sparte", "settlement_id": "s-4", "owner": "Nestor"},
		{"name": "Mykene", "settlement_id": "m-5", "owner": "Agamemnon"},
	}

	t.Run("finds all cities for a ruler, case-insensitive, sorted", func(t *testing.T) {
		got := settlementsRuledBy(wanaxes, "nestor")
		if len(got) != 2 || got[0] != "Pylos" || got[1] != "Sparte" {
			t.Fatalf("got %v, want [Pylos Sparte]", got)
		}
	})

	t.Run("single city for a ruler", func(t *testing.T) {
		got := settlementsRuledBy(wanaxes, "Agamemnon")
		if len(got) != 1 || got[0] != "Mykene" {
			t.Fatalf("got %v, want [Mykene]", got)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := settlementsRuledBy(wanaxes, "Odysseus")
		if len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
