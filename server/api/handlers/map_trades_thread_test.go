package handlers

// The owner sees the Thread on their own caravan (böner steg 5b) — "the Thread, N game days" —
// and nobody else sees it, not even on a caravan that is theirs to receive.

import (
	"testing"

	"formatet/megaron/server/internal/religion"
)

func TestMapTrades_TheOwnerSeesTheThreadOnTheirCaravanOnly(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	for _, owner := range []any{f.playerA, f.playerB} {
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,from_tick,until_tick) VALUES($1,$2,$3,'realm',0,12)`,
			f.worldID, owner, religion.EffectBlessedThread); err != nil {
			t.Fatal(err)
		}
	}
	markers := f.get(t, f.tokenA)
	own := findByGood(t, markers, "grain")
	lines, _ := own["blessings"].([]any)
	if len(lines) != 1 || lines[0] != "the Thread, 12 game days" {
		t.Fatalf("own caravan blessings = %v", own["blessings"])
	}
	incoming := findByGood(t, markers, "copper") // B's caravan, B has a Thread too
	if _, has := incoming["blessings"]; has {
		t.Fatalf("someone else's caravan shows a blessing: %v", incoming)
	}
}
