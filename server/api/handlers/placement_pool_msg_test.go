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
