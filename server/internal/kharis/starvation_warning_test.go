package kharis

// Regression tests for the Sparta-forensiken 2026-07-12 rework: the starvation
// warning used to land in gossip_events (a LIMIT-30 minor channel) and went
// SILENT the moment grain hit zero — exactly when the collapse began. It now
// emits SubsistenceWarning notifications through the notify hub, in escalating
// tiers (yellow while grain still positive; red when it empties within a day;
// critical from applySubsistenceCritical once grain is empty and pop drops).

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/events"
)

// notifyRecorder is a Broadcaster test double: it records each NotifyPlayer
// call AND persists to the real notifications table via the test pool, so the
// emitSubsistenceWarning dedupe query (unread same kind+settlement+tier) sees
// prior inserts exactly like *notify.Hub does in production.
type notifyRecorder struct {
	pool  *pgxpool.Pool
	mu    sync.Mutex
	calls []recordedNotify
}

type recordedNotify struct {
	playerID uuid.UUID
	kind     string
	level    int
	tier     string
}

func newNotifyRecorder(pool *pgxpool.Pool) *notifyRecorder {
	return &notifyRecorder{pool: pool}
}

func (r *notifyRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	tier := ""
	var settlementID uuid.UUID
	if m, ok := payload.(map[string]any); ok {
		if t, ok := m["tier"].(string); ok {
			tier = t
		}
		if sid, ok := m["settlement_id"].(uuid.UUID); ok {
			settlementID = sid
		}
	}
	r.mu.Lock()
	r.calls = append(r.calls, recordedNotify{playerID: playerID, kind: kind, level: level, tier: tier})
	r.mu.Unlock()

	bodyJSON := []byte(`{"settlement_id":"` + settlementID.String() + `","tier":"` + tier + `"}`)
	_, _ = r.pool.Exec(ctx,
		`INSERT INTO notifications (world_id, player_id, kind, level, body_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		worldID, playerID, kind, level, bodyJSON)
	return nil
}

func (r *notifyRecorder) countTier(playerID uuid.UUID, tier string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if c.playerID == playerID && c.kind == subsistenceKind && c.tier == tier {
			n++
		}
	}
	return n
}

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

func TestApplyStarvationWarning_RedWhenGrainWillEmptyWithinADay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// amount=100, rate=-10/tick → empty in 10 ticks, well within TicksPerDay (24).
	worldID, _, ownerID := starvationWarningFixture(t, 100, -10)

	rec := newNotifyRecorder(pool)
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyStarvationWarning(ctx, worldID)

	if n := rec.countTier(ownerID, tierRed); n != 1 {
		t.Errorf("red warning count = %d, want 1 (grain empties within a day)", n)
	}
}

func TestApplyStarvationWarning_YellowWhenTrendIsFarOff(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// amount=10000, rate=-1/tick → empty in 10000 ticks: net negative but far
	// beyond the one-day horizon → yellow, not red.
	worldID, _, ownerID := starvationWarningFixture(t, 10000, -1)

	rec := newNotifyRecorder(pool)
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyStarvationWarning(ctx, worldID)

	if n := rec.countTier(ownerID, tierYellow); n != 1 {
		t.Errorf("yellow warning count = %d, want 1 (net negative, not near-term)", n)
	}
	if n := rec.countTier(ownerID, tierRed); n != 0 {
		t.Errorf("red warning count = %d, want 0 (not near-term)", n)
	}
}

func TestApplyStarvationWarning_SilentFromWarningWhenGrainAlreadyEmpty(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Already at zero — this is applySubsistenceCritical's territory, not the
	// proactive (grain>0) warning's; the two must not double up.
	worldID, _, ownerID := starvationWarningFixture(t, 0, -10)

	rec := newNotifyRecorder(pool)
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyStarvationWarning(ctx, worldID)

	if n := rec.countTier(ownerID, tierYellow) + rec.countTier(ownerID, tierRed); n != 0 {
		t.Errorf("proactive warning count = %d, want 0 (already-empty is the critical pass's case)", n)
	}
}

func TestApplySubsistenceCritical_FiresWhenGrainEmpty(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, ownerID := starvationWarningFixture(t, 0, -10)

	rec := newNotifyRecorder(pool)
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applySubsistenceCritical(ctx, worldID)

	if n := rec.countTier(ownerID, tierCritical); n != 1 {
		t.Errorf("critical warning count = %d, want 1 (grain empty + negative net)", n)
	}
}

func TestApplyStarvationWarning_SilentWhenGrainGrowing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, ownerID := starvationWarningFixture(t, 100, 5)

	rec := newNotifyRecorder(pool)
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	h.applyStarvationWarning(ctx, worldID)

	if n := rec.countTier(ownerID, tierYellow) + rec.countTier(ownerID, tierRed); n != 0 {
		t.Errorf("warning count = %d, want 0 (positive rate — not trending toward empty)", n)
	}
}

func TestEmitSubsistenceWarning_DedupesUnreadSameTier(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, _, ownerID := starvationWarningFixture(t, 100, -10)

	rec := newNotifyRecorder(pool)
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
	// Two consecutive passes with the same still-holding trend: the second must
	// be deduped (unread red already exists in notifications).
	h.applyStarvationWarning(ctx, worldID)
	h.applyStarvationWarning(ctx, worldID)

	if n := rec.countTier(ownerID, tierRed); n != 1 {
		t.Errorf("red warning count after two passes = %d, want 1 (unread dedupe)", n)
	}
}
