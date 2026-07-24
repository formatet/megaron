package kharis

// Single-source-of-truth guard for grain attrition (Timothy's decision,
// 2026-07-25). These began as characterization tests (r6 audit, git 323b3c6)
// locking applyStarvation's own −5%-minimum-1 garrison attrition, written to
// hold the line while the double-count they had just proven awaited a ruling:
// applyStarvation (KharisTick) and combat/upkeep.go applyAttrition (UpkeepTick)
// both fired against the same garrison unit on the same starving day, in the
// same poll batch — a flat −10 plus a further −5%, ~14–15%/day combined, some
// 50% more than upkeep alone and worst for small garrisons. Two mechanics
// written 19 days apart (70199f3 / 203af2c), never cross-checked, with nothing
// documenting the sum as intended.
//
// The ruling was to remove the copy here, keeping upkeep as the ONE mechanic:
// it owns the whole lifecycle (disband, cargo cascade, UnitAttrition event,
// owner notification), whereas this one only decremented size and disbanded
// silently. So these tests are now inverted — they assert applyStarvation does
// NOT touch unit size, and that it still emits StarvationDamage. If a future
// round reintroduces attrition here, these fail and the double-count is caught
// at build time instead of in a soak.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// starvationGarrisonFixture builds on starvationWarningFixture (same file
// group, starvation_warning_test.go) by adding one garrison spearman unit at
// the settlement, so applyStarvation would have something to attrit.
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

// TestApplyStarvation_LeavesGarrisonSizeToUpkeep is the core guard: a starving
// settlement's garrison must come through applyStarvation untouched. Size 100
// is the case the old test pinned at 95 (a clean 5%), so a reintroduced
// attrition of any shape shows up here first.
func TestApplyStarvation_LeavesGarrisonSizeToUpkeep(t *testing.T) {
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
	if size != 100 {
		t.Errorf("size = %d, want 100 — applyStarvation must not attrit; grain attrition "+
			"belongs to combat/upkeep.go applyAttrition alone", size)
	}
	if status != "garrison" {
		t.Errorf("status = %q, want garrison", status)
	}
}

// TestApplyStarvation_DoesNotDisbandSmallGarrison covers the small-unit end,
// where the old minimum-1 floor bit hardest: a 1-man unit was disbanded outright
// by a mechanic that never emitted UnitAttrition or notified its owner.
func TestApplyStarvation_DoesNotDisbandSmallGarrison(t *testing.T) {
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
	if size != 1 || status != "garrison" {
		t.Errorf("size = %d status = %q, want 1/garrison — a starving 1-man unit must be "+
			"disbanded by upkeep (with event + notification), never silently here", size, status)
	}
}

// TestApplyStarvation_StillEmitsStarvationDamage locks what applyStarvation
// KEEPS. It is the audit/flavour signal for "this city went hungry today"; the
// owner-facing warning is applySubsistenceCritical via the notify hub. Removing
// the attrition must not quietly remove the record that starvation happened.
func TestApplyStarvation_StillEmitsStarvationDamage(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, settlementID, _ := starvationGarrisonFixture(t, 100)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.applyStarvation(ctx, worldID)

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events
		 WHERE stream_id = $1 AND event_type = 'StarvationDamage'`,
		settlementID,
	).Scan(&n); err != nil {
		t.Fatalf("count StarvationDamage: %v", err)
	}
	if n != 1 {
		t.Errorf("StarvationDamage events = %d, want 1", n)
	}
}
