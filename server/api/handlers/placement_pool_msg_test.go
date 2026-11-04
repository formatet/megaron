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

// The expedition-budget preview refusal names the public floor (4 game days) but
// never a route-specific minimum.
func TestExpeditionBudgetMsgNamesFloorOnly(t *testing.T) {
	if !strings.Contains(expeditionBudgetMsg, "shortest expedition is 4 game days") || !strings.Contains(expeditionBudgetMsg, "too short") {
		t.Fatal(expeditionBudgetMsg)
	}
}
