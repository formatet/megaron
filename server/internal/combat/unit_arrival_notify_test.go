package combat

// Regression test for Fas 2h: a plain peaceful march (no combat, no colonize)
// only ever wrote an audit event (EventUnitArrived) — arriveGarrison never
// called hub.NotifyPlayer, so a Wanax had no notification for "your unit
// safely reached its destination" (ColonyFounded/ArmyArrival/OutpostEstablished
// already notified; this was the one arrival path that didn't).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

// fakeBroadcaster records NotifyPlayer calls for assertion — a minimal
// Broadcaster (internal/combat/notifier.go) test double.
type fakeBroadcaster struct {
	notified []string // kinds, in call order
}

func (f *fakeBroadcaster) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (f *fakeBroadcaster) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	f.notified = append(f.notified, kind)
	return nil
}

func TestArriveGarrison_NotifiesOwner(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"marcher-"+uuid.New().String(), "marcher-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	const unitSize = 100
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', $3, 'marching', 5, 5) RETURNING id`,
		worldID, ownerID, unitSize,
	).Scan(&unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	fb := &fakeBroadcaster{}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        fb,
		scheduler:  nil,
		clk:        clock.NewTestClock(time.Now()),
	}

	u := unitRow{id: unitID, ownerID: ownerID, utype: "spearman", category: "land", size: unitSize, status: "marching", q: 5, r: 5}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := h.arriveGarrison(ctx, tx, u, 10, 10, nil, worldID); err != nil {
		t.Fatalf("arriveGarrison: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if len(fb.notified) != 1 || fb.notified[0] != "UnitArrived" {
		t.Errorf("NotifyPlayer calls = %v, want exactly one \"UnitArrived\"", fb.notified)
	}
}
