package handlers

import (
	"strings"
	"testing"
)

// The "pool empty" 409 must tell the player how to free a citizen.
func TestNoCitizensLeftMsgNamesTheFreeingVerbs(t *testing.T) {
	for _, want := range []string{"no citizens left in the pool", "keryx place <good> -1", "keryx staff <building> -1"} {
		if !strings.Contains(noCitizensLeftMsg, want) {
			t.Fatalf("missing %q in %q", want, noCitizensLeftMsg)
		}
	}
}

// The expedition-budget preview refusal names the duration the player gave and the public range, never
// a route-specific minimum, and never says "too short" about a duration longer than the floor.
func TestExpeditionBudgetMsgNamesAskedDuration(t *testing.T) {
	got := expeditionBudgetMsg(10)
	if !strings.Contains(got, "an expedition of 10 game days is not enough") || !strings.Contains(got, "4 to 365 game days") || strings.Contains(got, "too short") {
		t.Fatal(got)
	}
	if def := expeditionBudgetMsg(0); !strings.Contains(def, "of 10 game days") {
		t.Fatal(def)
	}
}

func TestStormShipWarningByTypeAndHull(t *testing.T) {
	if got := stormShipWarning("galley", 2, 0); !strings.Contains(got, "will sink it") {
		t.Fatalf("damaged galley: %q", got)
	}
	if got := stormShipWarning("galley", 5, 0); got != "" {
		t.Fatalf("sound galley: %q", got)
	}
	if got := stormShipWarning("spearman", 1, 0); got != "" {
		t.Fatalf("land unit: %q", got)
	}
}
