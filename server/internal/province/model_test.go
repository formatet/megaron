package province

import (
	"math"
	"testing"
	"time"
)

func TestResourceState_CurrentProjectsForward(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rs := ResourceState{Amount: 100, RatePerMinute: 2, Cap: 1000, LastCalcAt: base}
	if got := rs.Current(base.Add(30 * time.Minute)); math.Abs(got-160) > 1e-9 {
		t.Errorf("expected 160 after 30 min at 2/min, got %.3f", got)
	}
}

func TestResourceState_CurrentClampsToCap(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rs := ResourceState{Amount: 980, RatePerMinute: 2, Cap: 1000, LastCalcAt: base}
	if got := rs.Current(base.Add(60 * time.Minute)); got != 1000 {
		t.Errorf("should clamp to cap 1000, got %.3f", got)
	}
}

func TestResourceState_CurrentFloorsAtZero(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rs := ResourceState{Amount: 5, RatePerMinute: -2, Cap: 1000, LastCalcAt: base}
	if got := rs.Current(base.Add(60 * time.Minute)); got != 0 {
		t.Errorf("negative rate must floor at 0, got %.3f", got)
	}
}

func TestResourceLedger_SnapshotGold(t *testing.T) {
	base := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	rl := ResourceLedger{Gold: ResourceState{Amount: 50, RatePerMinute: 1, Cap: 500, LastCalcAt: base}}
	snap := rl.Snapshot(base.Add(10 * time.Minute))
	if math.Abs(snap["gold"]-60) > 1e-9 {
		t.Errorf("gold snapshot should be 60, got %.3f", snap["gold"])
	}
	full := rl.SnapshotFull(base.Add(10 * time.Minute))
	if math.Abs(full["gold"].Amount-60) > 1e-9 || full["gold"].Rate != 1 || full["gold"].Cap != 500 {
		t.Errorf("full gold snapshot mismatch: %+v", full["gold"])
	}
}
