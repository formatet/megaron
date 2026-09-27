package messenger

// TestCourierTravelOnGraph_NoLongerShortcutsOverSea is 3b-4's acceptance
// criterion 3 (megaron_plan_ordna_passage.md): a land route that used to
// shortcut across a narrow strait — crossing ONE sea hex at the old flat
// abstract boat rate — now pays the full, longer, pure-land detour instead.
// Pure in-memory TileGraph test, no DB needed (CourierTravelOnGraph never
// touches the database).

import (
	"testing"

	"formatet/megaron/server/internal/province"
)

// Map layout (axial q,r) — a two-hex-wide strait with a land bypass one row
// south:
//
//	(0,0) plains ─ (1,0) sea ─ (2,0) sea ─ (3,0) plains     old shortcut: 4 hexes, 2 of them sea
//	  |                                       |
//	(0,1) plains ─ (1,1) plains ─ (2,1) plains ─ (3,1) plains
//
// Values measured directly (not hand-derived — a hex grid's diagonal-ish
// neighbours make hand arithmetic unreliable, confirmed the hard way while
// building this fixture: a narrower strait gave the SAME tick count both
// ways by coincidence):
//   - Old (pre-3b-4, sea passable at province.CourierSeaTicks=0.5 per hex):
//     path (0,0)→(1,0)→(2,0)→(3,0), cost 0.5+0.5+0.375=1.375h → 1 tick.
//   - New (this test, sea now a wall — R3): path (0,0)→(0,1)→(1,1)→(2,1)→
//     (3,0), cost 4×0.375=1.5h → 2 ticks.
func TestCourierTravelOnGraph_NoLongerShortcutsOverSea(t *testing.T) {
	g := province.TileGraph{
		{0, 0}: "plains", {1, 0}: "coastal_sea", {2, 0}: "coastal_sea", {3, 0}: "plains",
		{0, 1}: "plains", {1, 1}: "plains", {2, 1}: "plains", {3, 1}: "plains",
	}
	ticks, _, ok := CourierTravelOnGraph(g, province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 3, R: 0})
	if !ok {
		t.Fatal("expected a land-only route to still exist, around the strait")
	}
	const oldValue = 1 // pre-3b-4: the sea shortcut
	const newValue = 2 // 3b-4: the pure land detour
	if ticks != newValue {
		t.Errorf("CourierTravelOnGraph ticks = %d, want %d (the pure land detour) — "+
			"the old (pre-3b-4) sea-shortcut value was %d", ticks, newValue, oldValue)
	}
}

// TestCourierTravelOnGraph_UnreachableIsNeverARaklinje is the rest of
// acceptance criterion 3: a target genuinely unreachable by any courier route
// (here, on the far side of open sea with no land detour at all) reports
// ok=false — never a guessed straight-line travel time. The old fallback
// (province.HexDistance + MessengerTravelTicks) is gone from
// CourierTravelOnGraph entirely; this pins that it stays gone.
func TestCourierTravelOnGraph_UnreachableIsNeverARaklinje(t *testing.T) {
	g := province.TileGraph{
		{0, 0}: "plains", {1, 0}: "coastal_sea", {2, 0}: "plains",
	}
	ticks, dur, ok := CourierTravelOnGraph(g, province.MapPosition{Q: 0, R: 0}, province.MapPosition{Q: 2, R: 0})
	if ok {
		t.Fatalf("CourierTravelOnGraph = ok=true (ticks=%d, dur=%v), want ok=false — no land route exists around the sea in this fixture",
			ticks, dur)
	}
	if ticks != 0 || dur != 0 {
		t.Errorf("CourierTravelOnGraph on ok=false = (%d, %v), want the zero value, not a guessed straight-line time", ticks, dur)
	}
}
