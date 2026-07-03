package kharis

// Regression test for Fas 2d: the starvation gossip warning only ever fired
// AFTER grain hit zero (applyStarvation) — a Wanax got no notice before the
// damage started. applyStarvationWarning is the proactive counterpart: it
// fires while grain is still positive but on a trend that empties it within
// the next game-day (TicksPerDay ticks).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/events"
)

// starvationWarningFixture builds a minimal active world + settlement with a
// single grain settlement_goods row — lighter than newGrowthFixture since
// this test doesn't need catchment/production, just a grain amount+rate.
func starvationWarningFixture(t *testing.T, grainAmount, grainRate float64) (worldID, settlementID, ownerID uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-starvewarn-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-starvewarn-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"starvewarn-"+uuid.New().String(), "starvewarn-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Starveton', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', $2, $3, 1000, 0)`,
		settlementID, grainAmount, grainRate,
	); err != nil {
		t.Fatalf("seed grain: %v", err)
	}
	return worldID, settlementID, ownerID
}

func gossipCount(t *testing.T, worldID, recipientID uuid.UUID) int {
	t.Helper()
	pool := testPool(t)
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND text LIKE '%running out%'`,
		worldID, recipientID,
	).Scan(&n); err != nil {
		t.Fatalf("count gossip: %v", err)
	}
	return n
}

func TestApplyStarvationWarning_FiresWhenGrainWillEmptyWithinADay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// amount=100, rate=-10/tick → empty in 10 ticks, well within TicksPerDay (24).
	worldID, _, ownerID := starvationWarningFixture(t, 100, -10)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool))
	h.applyStarvationWarning(ctx, worldID)

	if n := gossipCount(t, worldID, ownerID); n != 1 {
		t.Errorf("gossip warning count = %d, want 1 (grain trending to empty within a day)", n)
	}
}

func TestApplyStarvationWarning_SilentWhenGrainAlreadyEmpty(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Already at zero — this is applyStarvation's (reactive) territory, not
	// the proactive warning's; the two must not double up.
	worldID, _, ownerID := starvationWarningFixture(t, 0, -10)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool))
	h.applyStarvationWarning(ctx, worldID)

	if n := gossipCount(t, worldID, ownerID); n != 0 {
		t.Errorf("gossip warning count = %d, want 0 (already-empty grain is applyStarvation's case, not the warning's)", n)
	}
}

func TestApplyStarvationWarning_SilentWhenTrendIsSafe(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// amount=10000, rate=-1/tick → empty in 10000 ticks, far beyond the
	// one-day (24-tick) horizon — no warning warranted yet.
	worldID, _, ownerID := starvationWarningFixture(t, 10000, -1)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool))
	h.applyStarvationWarning(ctx, worldID)

	if n := gossipCount(t, worldID, ownerID); n != 0 {
		t.Errorf("gossip warning count = %d, want 0 (trend is not near-term)", n)
	}
}

func TestApplyStarvationWarning_SilentWhenGrainGrowing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, ownerID := starvationWarningFixture(t, 100, 5)

	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool))
	h.applyStarvationWarning(ctx, worldID)

	if n := gossipCount(t, worldID, ownerID); n != 0 {
		t.Errorf("gossip warning count = %d, want 0 (positive rate — not trending toward empty)", n)
	}
}
