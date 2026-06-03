package main

import "testing"

func marker(name, id string, own bool) map[string]any {
	return map[string]any{"name": name, "settlement_id": id, "own": own}
}

func TestResolveMessengerDest(t *testing.T) {
	markers := []map[string]any{
		marker("Mykene", "self-1", true),
		marker("Korinth", "k-2", false),
	}
	wanaxes := []map[string]any{
		{"name": "Pylos", "settlement_id": "p-3"},
	}

	t.Run("resolves visible neighbour", func(t *testing.T) {
		dest, name, own, err := resolveMessengerDest(markers, wanaxes, "korinth")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dest != "k-2" || name != "Korinth" || own != "self-1" {
			t.Fatalf("got dest=%q name=%q own=%q", dest, name, own)
		}
	})

	t.Run("falls back to wanaxes list", func(t *testing.T) {
		dest, _, _, err := resolveMessengerDest(markers, wanaxes, "Pylos")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dest != "p-3" {
			t.Fatalf("got dest=%q, want p-3", dest)
		}
	})

	t.Run("rejects own settlement with actionable error", func(t *testing.T) {
		_, _, _, err := resolveMessengerDest(markers, wanaxes, "Mykene")
		if err == nil {
			t.Fatal("expected error sending to own settlement")
		}
		if got := err.Error(); !contains(got, "your own settlement") {
			t.Fatalf("error should mention own settlement, got: %q", got)
		}
	})

	t.Run("unknown destination", func(t *testing.T) {
		_, _, _, err := resolveMessengerDest(markers, wanaxes, "Atlantis")
		if err == nil {
			t.Fatal("expected error for unknown settlement")
		}
	})

	t.Run("missing own settlement", func(t *testing.T) {
		noOwn := []map[string]any{marker("Korinth", "k-2", false)}
		_, _, _, err := resolveMessengerDest(noOwn, nil, "Korinth")
		if err == nil {
			t.Fatal("expected error when own settlement is absent")
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
