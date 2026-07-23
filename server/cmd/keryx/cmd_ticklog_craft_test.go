package main

// The keryx side of the GoodsCrafted event (2026-07-22). The payloads below are
// copied verbatim from a real craft against the test clone, so this test breaks
// if the server's field names ever drift from what the CLI reads.

import (
	"encoding/json"
	"testing"
)

func TestRenderTickEvent_GoodsCrafted(t *testing.T) {
	// Verbatim from events.payload after POST /craft {recipe_id:1, quantity:3}.
	payload := json.RawMessage(`{"consumed": {"tin": 3, "copper": 6}, "produced": 3, ` +
		`"output_key": "bronze", "settlement_id": "e7f190ec-7ea1-44e5-a628-84cf9642556e"}`)

	got := renderTickEvent("GoodsCrafted", payload)
	want := "Gjutning: 3 bronze ur 6 copper + 3 tin"
	if got != want {
		t.Fatalf("renderTickEvent = %q, want %q", got, want)
	}
}

// Go map iteration order is random; the ingredient list is sorted so the same
// craft never renders two different audit lines. Repeat enough to catch it.
func TestRenderTickEvent_GoodsCraftedIngredientOrderIsStable(t *testing.T) {
	payload := json.RawMessage(`{"consumed": {"tin": 3, "copper": 6, "charcoal": 1}, ` +
		`"produced": 3, "output_key": "bronze"}`)

	first := renderTickEvent("GoodsCrafted", payload)
	for i := 0; i < 50; i++ {
		if got := renderTickEvent("GoodsCrafted", payload); got != first {
			t.Fatalf("unstable render: %q then %q", first, got)
		}
	}
}

// An older or malformed payload must not produce a broken line.
func TestRenderTickEvent_GoodsCraftedWithoutIngredients(t *testing.T) {
	got := renderTickEvent("GoodsCrafted", json.RawMessage(`{"produced": 1, "output_key": "bronze"}`))
	if want := "Gjutning: 1 bronze"; got != want {
		t.Fatalf("renderTickEvent = %q, want %q", got, want)
	}
}
