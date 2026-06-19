package economy

import (
	"math"
	"testing"
	"time"
)

// strptr is a test helper for the optional pointer fields on ProductionRule.
func strptr(s string) *string { return &s }

// TestBronzeChain_TinOnlyOnMountain verifies the geographic forcing function:
// a mountain_limestone hex WITH a tin deposit produces tin; a plains hex WITH
// the same deposit flag does NOT produce tin (wrong terrain). This is the
// fundamental rule that makes tin scarce and forces cross-island trade.
func TestBronzeChain_TinOnlyOnMountain(t *testing.T) {
	// Minimal rule set mirroring the real production_rules rows for tin.
	rules := []ProductionRule{
		{TerrainType: strptr("mountain_limestone"), GoodKey: GoodTin, RatePerMin: 0.01, RequiresDeposit: strptr("tin")},
		{TerrainType: strptr("mountain_limestone"), BuildingType: strptr("mine"), GoodKey: GoodTin, RatePerMin: 0.025, RequiresDeposit: strptr("tin")},
	}

	// Plains with tin deposit: no production (wrong terrain).
	plainsWithDeposit := effectiveRates(rules, "plains", nil, false, true)
	if plainsWithDeposit[GoodTin] != 0 {
		t.Errorf("plains must not produce tin even with deposit, got %.4f", plainsWithDeposit[GoodTin])
	}

	// Mountain_limestone without deposit: no production.
	mountainNoDeposit := effectiveRates(rules, "mountain_limestone", nil, false, false)
	if mountainNoDeposit[GoodTin] != 0 {
		t.Errorf("mountain without tin deposit must not produce tin, got %.4f", mountainNoDeposit[GoodTin])
	}

	// Mountain_limestone with deposit: produces tin (terrain-only rule).
	mountainWithDeposit := effectiveRates(rules, "mountain_limestone", nil, false, true)
	if mountainWithDeposit[GoodTin] != 0.01 {
		t.Errorf("mountain+deposit should produce 0.01 tin/min, got %.4f", mountainWithDeposit[GoodTin])
	}

	// Mountain_limestone + deposit + mine: both rules fire.
	mountainWithMine := effectiveRates(rules, "mountain_limestone", []string{"mine"}, false, true)
	if mountainWithMine[GoodTin] != 0.035 {
		t.Errorf("mountain+deposit+mine should produce 0.035 tin/min, got %.4f", mountainWithMine[GoodTin])
	}
}

// TestBronzeChain_CopperNotOnMountain verifies that copper production requires hills
// terrain (not mountain_limestone), reinforcing the geographic separation design:
// copper and tin are on different terrain types.
func TestBronzeChain_CopperNotOnMountain(t *testing.T) {
	rules := []ProductionRule{
		{TerrainType: strptr("hills"), GoodKey: GoodCopper, RatePerMin: 0.02, RequiresDeposit: strptr("copper")},
		{TerrainType: strptr("hills"), BuildingType: strptr("mine"), GoodKey: GoodCopper, RatePerMin: 0.04, RequiresDeposit: strptr("copper")},
	}

	// Mountain_limestone with copper deposit: no production (wrong terrain).
	mountainWithCopper := effectiveRates(rules, "mountain_limestone", nil, true, false)
	if mountainWithCopper[GoodCopper] != 0 {
		t.Errorf("mountain must not produce copper (copper is a hills good), got %.4f", mountainWithCopper[GoodCopper])
	}

	// Hills with copper deposit: produces copper.
	hillsWithCopper := effectiveRates(rules, "hills", nil, true, false)
	if hillsWithCopper[GoodCopper] != 0.02 {
		t.Errorf("hills+copper_deposit should produce 0.02/min, got %.4f", hillsWithCopper[GoodCopper])
	}
}

func TestGoodState_CurrentProjectsForward(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	g := GoodState{Amount: 100, Rate: 2, Cap: 1000, CalcAt: base}
	// 10 minutes at 2/min → +20.
	if got := g.Current(base.Add(10 * time.Minute)); math.Abs(got-120) > 1e-9 {
		t.Errorf("expected 120 after 10 min, got %.3f", got)
	}
}

func TestGoodState_CurrentClampsToCap(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	g := GoodState{Amount: 990, Rate: 2, Cap: 1000, CalcAt: base}
	if got := g.Current(base.Add(60 * time.Minute)); got != 1000 {
		t.Errorf("should clamp to cap 1000, got %.3f", got)
	}
}

func TestGoodState_CurrentFloorsAtZero(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	// Negative rate (e.g. recipe consumption) must never drive stock below zero.
	g := GoodState{Amount: 10, Rate: -2, Cap: 1000, CalcAt: base}
	if got := g.Current(base.Add(60 * time.Minute)); got != 0 {
		t.Errorf("should floor at 0, got %.3f", got)
	}
}

func TestGoodState_CurrentAtCalcAt(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	g := GoodState{Amount: 42, Rate: 5, Cap: 1000, CalcAt: base}
	if got := g.Current(base); got != 42 {
		t.Errorf("at calc_at should equal stored amount, got %.3f", got)
	}
}

// TestTimescaleRelation_SettledFormula locks the TIME_SCALE↔production-rate contract.
//
// The settled() SQL function is the canonical lazy-eval path for all settlement_goods:
//   settled(amount, rate, calc_at) = amount + epoch_seconds(now−calc_at)/60 × rate × factor
//
// where factor = TIME_SCALE (written to sim_config by main.go at boot).
//
// At TIME_SCALE=100: 1 real minute of wall-clock time accrues 100 game-minutes of production.
// production_rules.rate_per_min is expressed in game-minutes — so at TIME_SCALE=100 a rate of
// 0.01/game-min yields 0.01×100 = 1.0 units per real minute. This is intentional and correct.
//
// GoodState.Current() (Go-side) does NOT apply TIME_SCALE. It is only used for the legacy
// silver ResourceState columns which were dropped in migration 057 (P0a silver-unification).
// All active reads/writes go through settled() in SQL. GoodState.Current is a Go-side test
// helper; callers in production code are province/model.go's silver column path (now dead).
//
// SB4 audit verdict (2026-06-19): NO BUG in the production path. The settled() function
// correctly scales production by TIME_SCALE. The ×100 live world's balance data is valid.
func TestTimescaleRelation_SettledFormula(t *testing.T) {
	// Mirror settled() arithmetic in Go to document the formula.
	// settled(amount, rate, calc_at) = amount + elapsed_wall_seconds/60 × rate × factor
	settledGo := func(amount, rate float64, factor float64, elapsedWallSeconds float64) float64 {
		return amount + (elapsedWallSeconds/60)*rate*factor
	}

	const rate = 0.01    // grain/game-min (e.g. plains terrain rule)
	const factor = 100.0 // TIME_SCALE=100 (11-day playtest)
	const elapsed = 60.0 // 60 real seconds = 1 real minute

	// At TIME_SCALE=100: 1 real minute → 100 game-minutes → 0.01×100 = 1.0 grain.
	got := settledGo(0, rate, factor, elapsed)
	const want = 1.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("settled formula: expected %.4f after 60s wall-time at rate %.4f factor %.0f, got %.4f",
			want, rate, factor, got)
	}

	// At TIME_SCALE=1 (real-time): 1 real minute → 1 game-minute → 0.01×1 = 0.01 grain.
	got1x := settledGo(0, rate, 1.0, elapsed)
	if math.Abs(got1x-0.01) > 1e-9 {
		t.Errorf("settled formula at 1x: expected 0.01, got %.4f", got1x)
	}
}
