package combat

// megaron_plan_skeppsreparation.md Slice C — pins the two calibration details
// the plan explicitly delegates to the implementer (strawman, not canon):
// repairCostFractionPerHullPoint (~8%/point, off the LIVING training.go
// build cost) and repairTicksPerHullPoint. Pure, no DB — ship_repair_db_test.go
// covers the DB-backed end-to-end wiring (start → tick → completion → notify).

import (
	"math"
	"testing"
)

func TestRepairCost(t *testing.T) {
	cases := []struct {
		name       string
		unitType   string
		hullPoints int
		wantGood   string
		wantAmount float64
	}{
		// galley: training.go Costs{timber:0.0417} per crew member (9→0.0417, mig
		// 136, timber ÷216), CrewFor=20 → full-ship build cost = 0.834 timber.
		// 8%/point × 1 point = 0.0667.
		{"galley, one hull point", "galley", 1, "timber", 0.834 * repairCostFractionPerHullPoint},
		// war_galley: Costs{cedar:0.0694} per crew (5→0.0694, mig 136, cedar ÷72),
		// CrewFor=50 → 3.47 cedar full build. A full 5-point repair costs 5×8% =
		// 40% of that.
		{"war_galley, full repair", "war_galley", hullMax, "cedar", 3.47 * repairCostFractionPerHullPoint * float64(hullMax)},
		// merchantman: Costs{timber:0.0405} per crew (8.75→0.0405, mig 136, timber
		// ÷216), CrewFor=10 → 0.405 timber build.
		{"merchantman, three hull points", "merchantman", 3, "timber", 0.405 * repairCostFractionPerHullPoint * 3},
		{"zero hull points costs nothing", "galley", 0, "timber", 0},
		{"unknown unit type", "spearman", 2, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			good, amount, ok := RepairCost(c.unitType, c.hullPoints)
			if c.wantGood == "" {
				if ok {
					t.Fatalf("RepairCost(%q, %d) ok=true, want false (unsupported type)", c.unitType, c.hullPoints)
				}
				return
			}
			if !ok {
				t.Fatalf("RepairCost(%q, %d) ok=false, want true", c.unitType, c.hullPoints)
			}
			if good != c.wantGood {
				t.Errorf("RepairCost(%q, %d) good = %q, want %q", c.unitType, c.hullPoints, good, c.wantGood)
			}
			if math.Abs(amount-c.wantAmount) > 1e-9 {
				t.Errorf("RepairCost(%q, %d) amount = %v, want %v", c.unitType, c.hullPoints, amount, c.wantAmount)
			}
		})
	}
}

// TestRepairCost_FullRepairIsFractionOfBuildCost pins the plan's own
// calibration framing (§Slice C point 2): a full HullMax→0 repair costs
// repairCostFractionPerHullPoint × hullMax (40% at the 8%/point strawman) of
// a fresh build in the same material, for every naval type.
func TestRepairCost_FullRepairIsFractionOfBuildCost(t *testing.T) {
	wantFraction := repairCostFractionPerHullPoint * float64(hullMax)
	for _, ut := range []string{"galley", "war_galley", "merchantman"} {
		good, shipCost, ok := repairMaterial(ut)
		if !ok {
			t.Fatalf("repairMaterial(%q) not ok", ut)
		}
		_, amount, ok := RepairCost(ut, hullMax)
		if !ok {
			t.Fatalf("RepairCost(%q, hullMax) not ok", ut)
		}
		gotFraction := amount / shipCost
		if math.Abs(gotFraction-wantFraction) > 1e-9 {
			t.Errorf("%s: full repair = %v of build cost (%s), want %v", ut, gotFraction, good, wantFraction)
		}
	}
}

func TestRepairDurationTicks(t *testing.T) {
	cases := []struct {
		hullPoints int
		want       int
	}{
		{0, 1},  // defensive floor — never a zero-tick job
		{-3, 1}, // defensive floor
		{1, 1},
		{3, 3},
		{hullMax, hullMax},
	}
	for _, c := range cases {
		if got := RepairDurationTicks(c.hullPoints); got != c.want {
			t.Errorf("RepairDurationTicks(%d) = %d, want %d", c.hullPoints, got, c.want)
		}
	}
}
