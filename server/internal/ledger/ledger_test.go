package ledger

import "testing"

func TestHerdSurplusIsLabelledForPlayers(t *testing.T) {
	if got := Label("herd_surplus"); got != "Herd" {
		t.Fatalf("herd_surplus must not reach a player as a raw key, got %q", got)
	}
}
