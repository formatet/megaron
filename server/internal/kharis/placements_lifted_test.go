package kharis

import (
	"context"
	"sync"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type liftNoticeRecorder struct {
	mu      sync.Mutex
	kinds   []string
	payload []any
}

func (r *liftNoticeRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kinds = append(r.kinds, kind)
	r.payload = append(r.payload, payload)
	return nil
}

func (r *liftNoticeRecorder) count(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, k := range r.kinds {
		if k == kind {
			n++
		}
	}
	return n
}

// seedPlacements replaces the settlement's placements with n grain placements
// (ordinals 1..n) and sets population.
func seedPlacements(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, pop, n int) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,hex_q,hex_r,good_key) VALUES($1,$2,'hex',$3,$4,'grain')`,
			sid, i, 100+i, 100); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population=$2 WHERE id=$1`, sid, pop); err != nil {
		t.Fatal(err)
	}
}

func placementCount(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1`, sid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Reinforcement takes men: the post of the man who left is lifted in the same
// step, and the owner gets a PlacementsLifted notice.
func TestApplyReinforcement_LiftsPlacementAndNotifies(t *testing.T) {
	pool, worldID, sid := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, sid, "grain", 50000)
	setGood(t, pool, sid, "silver", 500)
	seedReinforcingUnit(t, pool, worldID, sid, "spearman", 62)
	seedPlacements(t, pool, sid, 500, 5)
	h := newTestTickHandler(pool)
	rec := &liftNoticeRecorder{}
	h.hub = rec

	h.applyReinforcement(context.Background(), worldID, 910001, []popCrossing{{id: sid, oldPop: 496, newPop: 500}})

	var pop int
	if err := pool.QueryRow(context.Background(), `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&pop); err != nil {
		t.Fatal(err)
	}
	if pop != 496 {
		t.Fatalf("population = %d, want 496 (4 men reinforced)", pop)
	}
	if got := placementCount(t, pool, sid); got != 4 {
		t.Errorf("placements = %d, want 4 (cap floor(496/100))", got)
	}
	if rec.count("PlacementsLifted") != 1 {
		t.Errorf("PlacementsLifted notices = %d, want 1", rec.count("PlacementsLifted"))
	}
}

// A reinforcement that leaves every post manned says nothing.
func TestApplyReinforcement_NoLiftNoNotice(t *testing.T) {
	pool, worldID, sid := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, sid, "grain", 50000)
	setGood(t, pool, sid, "silver", 500)
	seedReinforcingUnit(t, pool, worldID, sid, "spearman", 62)
	seedPlacements(t, pool, sid, 520, 4)
	h := newTestTickHandler(pool)
	rec := &liftNoticeRecorder{}
	h.hub = rec

	h.applyReinforcement(context.Background(), worldID, 910002, []popCrossing{{id: sid, oldPop: 516, newPop: 520}})

	if got := placementCount(t, pool, sid); got != 4 {
		t.Errorf("placements = %d, want 4 untouched", got)
	}
	if rec.count("PlacementsLifted") != 0 {
		t.Errorf("notices = %d, want 0", rec.count("PlacementsLifted"))
	}
}

// The daily tick catches a population that fell between ticks (starvation,
// battle, occupation) and says so with cause "decline".
func TestApplyDecay_DeclineLiftNotifies(t *testing.T) {
	pool, worldID, sid := newGrowthFixture(t, richTerrain, 5000)
	setGood(t, pool, sid, "grain", 50000)
	seedPlacements(t, pool, sid, 300, 5)
	h := newTestTickHandler(pool)
	rec := &liftNoticeRecorder{}
	h.hub = rec

	advanceOneDay(t, h, pool, worldID)

	var pop int
	_ = pool.QueryRow(context.Background(), `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&pop)
	if got := placementCount(t, pool, sid); got > pop/100 {
		t.Errorf("placements = %d > floor(pop %d/100)", got, pop)
	}
	if rec.count("PlacementsLifted") != 1 {
		t.Fatalf("PlacementsLifted notices = %d, want 1", rec.count("PlacementsLifted"))
	}
	var cause string
	for i, k := range rec.kinds {
		if k == "PlacementsLifted" {
			cause = causeOf(rec.payload[i])
		}
	}
	if cause != "decline" {
		t.Errorf("cause = %q, want decline", cause)
	}
}

func causeOf(p any) string {
	if pl, ok := p.(economy.PlacementsLiftedPayload); ok {
		return pl.Cause
	}
	return ""
}
