package loyalty

import "testing"

// Unit tests for the L1 welfare formula (Timothy 2026-07-11: stads-loyalty
// välfärdssignaler). welfareDelta is a pure function — no DB — so these cases
// cover the standing-condition combinations without a live world.

func TestWelfareDelta_AllPositive(t *testing.T) {
	// Kharis good + fed + varied diet: +1 +1 +1 = +3 (caller clamps to +2).
	if got := welfareDelta(true, true, false, 3); got != 3 {
		t.Errorf("welfareDelta(good,fed,varied) = %d, want 3 (pre-clamp)", got)
	}
}

func TestWelfareDelta_Starving(t *testing.T) {
	if got := welfareDelta(false, false, true, 0); got != -1 {
		t.Errorf("welfareDelta(starving only) = %d, want -1", got)
	}
}

func TestWelfareDelta_StarvingCancelledByVariety(t *testing.T) {
	// Starving (-1) offset by varied diet (+1) nets to zero — caller must skip
	// the emit in this case (no brus).
	if got := welfareDelta(false, false, true, 4); got != 0 {
		t.Errorf("welfareDelta(starving, varied) = %d, want 0", got)
	}
}

func TestWelfareDelta_NothingStanding(t *testing.T) {
	// Not blessed, not fed, not starving, no variety — no signal.
	if got := welfareDelta(false, false, false, 0); got != 0 {
		t.Errorf("welfareDelta(nothing) = %d, want 0", got)
	}
}

func TestWelfareDelta_KharisAloneIsPositiveOne(t *testing.T) {
	if got := welfareDelta(true, false, false, 0); got != 1 {
		t.Errorf("welfareDelta(kharis good only) = %d, want 1", got)
	}
}

func TestWelfareDelta_VarietyBelowThresholdDoesNotCount(t *testing.T) {
	if got := welfareDelta(false, true, false, varietyThreshold-1); got != 1 {
		t.Errorf("welfareDelta(fed, variety below threshold) = %d, want 1 (fed only)", got)
	}
}

func TestClampNetDelta_BoundsToConfiguredRange(t *testing.T) {
	if got := clampNetDelta(5); got != netDeltaMax {
		t.Errorf("clampNetDelta(5) = %d, want max %d", got, netDeltaMax)
	}
	if got := clampNetDelta(-5); got != netDeltaMin {
		t.Errorf("clampNetDelta(-5) = %d, want min %d", got, netDeltaMin)
	}
	if got := clampNetDelta(1); got != 1 {
		t.Errorf("clampNetDelta(1) = %d, want unchanged 1", got)
	}
}

func TestClassifyWelfare_StarvingTakesPriority(t *testing.T) {
	// Starving should never be true simultaneously with kharisGood/fed in real
	// callers, but the priority order must still surface it first if it were.
	eventType, factors := classifyWelfare(true, false, true, true)
	if eventType != welfareEventStarving {
		t.Errorf("classifyWelfare priority = %q, want %q", eventType, welfareEventStarving)
	}
	if len(factors) != 3 { // well_favoured + starving + varied_diet all passed as active
		t.Errorf("classifyWelfare factors = %v, want 3 entries (favoured + starving + varied)", factors)
	}
}

func TestClassifyWelfare_VariedDietAlone(t *testing.T) {
	eventType, factors := classifyWelfare(false, false, false, true)
	if eventType != welfareEventVariedDiet {
		t.Errorf("classifyWelfare(varied only) type = %q, want %q", eventType, welfareEventVariedDiet)
	}
	if len(factors) != 1 {
		t.Errorf("classifyWelfare(varied only) factors = %v, want 1 entry", factors)
	}
}

func TestWelfareWindowSeconds_MatchesTickSubstrate(t *testing.T) {
	withTickSeconds(t, 3600)
	if got, want := welfareWindowSeconds(), 24*3600; got != want {
		t.Errorf("welfareWindowSeconds() at 60 min/tick = %d, want %d (1 game-day)", got, want)
	}
	withTickSeconds(t, 6)
	if got, want := welfareWindowSeconds(), 24*6; got != want {
		t.Errorf("welfareWindowSeconds() at TICK_SECONDS=6 = %d, want %d (1 game-day)", got, want)
	}
}
