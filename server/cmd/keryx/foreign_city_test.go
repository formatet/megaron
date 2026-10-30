package main

import (
	"strings"
	"testing"
	"time"
)

func TestForeignCityLines_SayWhenAndFromWhom(t *testing.T) {
	now := time.Date(2026, 10, 30, 15, 0, 0, 0, keryxTZ)
	sett := map[string]any{
		"name": "Tylissos", "owner_name": "Wanax Far", "size": "town", "walls": 1.0,
		"intel": map[string]any{
			"level": "contact", "seen_at": "2026-10-30T13:20:00Z", "contact_source": "messenger", "contact_at": "2026-10-27T08:10:00Z",
			"contact": map[string]any{"size": "town", "food": "hungry",
				"goods":     []any{map[string]any{"good": "oil", "word": "plenty of"}},
				"buildings": []any{map[string]any{"name": "Granary", "level": 2.0}}},
		},
	}
	got := strings.Join(foreignCityLines(sett, now), "\n")
	for _, want := range []string{"Tylissos — Wanax Far's city", "Seen today", "town, walls ▮", "Your messenger was there", "town, hungry, plenty of oil.", "Buildings then: Granary L2."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	delete(sett["intel"].(map[string]any), "contact")
	if got := strings.Join(foreignCityLines(sett, now), "\n"); !strings.Contains(got, "Send a messenger") {
		t.Errorf("a city never entered does not say how to learn more:\n%s", got)
	}
}
