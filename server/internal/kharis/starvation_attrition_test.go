package kharis

// Characterization tests for applyStarvation's garrison attrition (r6 audit,
// git 323b3c6): a dedicated round flagged that this mechanic MAY double-count
// with combat/upkeep.go's applyAttrition — both can fire against the same
// garrison spearman/war_chariot unit on the same starving day (settlement
// grain balance <= 0), one taking a flat upkeepAttritionStep (10, capped to
// unit size) via the daily UpkeepTick, the other taking 5% (minimum 1) via
// the daily KharisTick. These tests lock the CURRENT applyStarvation behavior
// only — they assert nothing about upkeep.go and change no production code.
// Resolution (keep both / gate one / merge) is a design decision for Timothy,
// not this test.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// starvationGarrisonFixture builds on starvationWarningFixture (same file
// group, starvation_warning_test.go) by adding one garrison spearman unit at
// the settlement, so applyStarvation has something to attrit.
func starvationGarrisonFixture(t *testing.T, unitSize int) (worldID, settlementID, unitID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	worldID, settlementID, ownerID := starvationWarningFixture(t, 0, -10)

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', $3, 0, 'garrison', $4)
		 RETURNING id`,
		worldID, ownerID, unitSize, settlementID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrison unit: %v", err)
	}
	return worldID, settlementID, unitID
}

// TestApplyStarvation_FivePercentOfGarrisonSpearmen locks the documented rate
// (tick.go:979-980, "infantry and chariots each lose 5% (minimum 1) per day")
// for a size where 5% is an exact, unambiguous integer — no rounding edge.
func TestApplyStarvation_FivePercentOfGarrisonSpearmen(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, unitID := starvationGarrisonFixture(t, 100)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.applyStarvation(ctx, worldID)

	var size int
	var status string
	if err := pool.QueryRow(ctx, `SELECT size, status FROM units WHERE id = $1`, unitID).Scan(&size, &status); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if size != 95 {
		t.Errorf("size = %d, want 95 (5%% of 100 lost to one starving day)", size)
	}
	if status != "garrison" {
		t.Errorf("status = %q, want garrison (95 men left, not disbanded)", status)
	}
}

// TestApplyStarvation_MinimumOneLostBelowFivePercentFloor locks the "minimum
// 1" floor: 5% of 5 is 0.25, which truncates to 0 — GREATEST(1, ...) forces a
// loss of 1 man even though the raw percentage rounds to nothing.
func TestApplyStarvation_MinimumOneLostBelowFivePercentFloor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, unitID := starvationGarrisonFixture(t, 5)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.applyStarvation(ctx, worldID)

	var size int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&size); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if size != 4 {
		t.Errorf("size = %d, want 4 (minimum-1 floor applied to a 0.25-man 5%%)", size)
	}
}

// TestApplyStarvation_DisbandsUnitStarvedToZero locks the disband path: a
// unit reduced to size <= 0 by the day's starvation loss is disbanded, not
// left at zero.
func TestApplyStarvation_DisbandsUnitStarvedToZero(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, unitID := starvationGarrisonFixture(t, 1)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.applyStarvation(ctx, worldID)

	var size int
	var status string
	if err := pool.QueryRow(ctx, `SELECT size, status FROM units WHERE id = $1`, unitID).Scan(&size, &status); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}
	if status != "disbanded" {
		t.Errorf("status = %q, want disbanded (a 1-man unit loses its last man to starvation)", status)
	}
}
