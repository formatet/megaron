package messenger

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

// testPool connects to a real Postgres instance — march recall is pure SQL
// orchestration (FOR UPDATE claims, FindPath over map_tiles, scheduled_events)
// that a mock can't meaningfully stand in for. Skips (not fails) when
// DATABASE_URL isn't set, so `go test ./...` stays green without a database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// marchRecallFixture builds a world with a straight 5-hex plains line
// (0,0)..(4,0), a capital settlement at the origin, and a unit marching from
// (0,0) to (4,0). departsAt/arrivesAt span 3h (4 hexes × 0.75h/hex, plains).
// Using status='archived' for the world sidesteps the one_active_world unique
// index (this shared dev DB always has a real active world running).
type marchRecallFixture struct {
	worldID    uuid.UUID
	ownerID    uuid.UUID
	unitID     uuid.UUID
	departsAt  time.Time
	arrivesAt  time.Time
}

func newMarchRecallFixture(t *testing.T, pool *pgxpool.Pool) marchRecallFixture {
	t.Helper()
	ctx := context.Background()

	var f marchRecallFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"recall-owner-"+uuid.New().String(), "recall-owner-"+uuid.New().String()+"@test.invalid",
	).Scan(&f.ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("create map_tiles(%d,0): %v", q, err)
		}
	}

	var originProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&originProvinceID); err != nil {
		t.Fatalf("create origin province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true)`,
		f.worldID, originProvinceID, f.ownerID,
	); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}

	f.departsAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f.arrivesAt = f.departsAt.Add(3 * time.Hour) // 4 hexes × 0.75h/hex plains

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 0, 0, 4, 0, $3, $4) RETURNING id`,
		f.worldID, f.ownerID, f.departsAt, f.arrivesAt,
	).Scan(&f.unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	return f
}

// insertRecallMessenger inserts a minimal recall-kind messenger row (kind, hex_q/r,
// dest_q/r) — mirrors what UnitHandler.Recall would create, without exercising
// the HTTP handler.
func insertRecallMessenger(t *testing.T, pool *pgxpool.Pool, f marchRecallFixture) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var homeSettlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		f.worldID, f.ownerID,
	).Scan(&homeSettlementID); err != nil {
		t.Fatalf("load home settlement: %v", err)
	}
	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers
		     (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, dest_q, dest_r, arrives_at)
		 VALUES ($1,$2,$3,NULL,'Recall order','outbound','recall',0,0,2,0,$4)
		 RETURNING id`,
		f.worldID, f.ownerID, homeSettlementID, f.arrivesAt,
	).Scan(&messengerID); err != nil {
		t.Fatalf("insert recall messenger: %v", err)
	}
	return messengerID
}

func TestMarchRecall_TurnsUnitTowardOrigin(t *testing.T) {
	pool := testPool(t)
	f := newMarchRecallFixture(t, pool)
	messengerID := insertRecallMessenger(t, pool, f)

	// Fire the recall halfway through the outbound march: the unit should be
	// caught at its interpolated position (2,0), not teleported, and its new
	// arrival must lie in the future — turning around is not instant either.
	now := f.departsAt.Add(90 * time.Minute) // 1.5h of a 3h journey = halfway
	testClk := clock.NewTestClock(now)
	h := NewMarchRecallHandler(pool, events.NewScheduler(pool, testClk), events.NewStore(pool), nil, testClk)

	payload := MarchRecallPayload{WorldID: f.worldID, UnitID: f.unitID, MessengerID: messengerID, Mode: "recall"}
	raw, _ := json.Marshal(payload)
	if err := h.Handle(context.Background(), events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var q, r, targetQ, targetR int
	var status string
	var arrivesAt time.Time
	var marchIntent *string
	if err := pool.QueryRow(context.Background(),
		`SELECT q, r, target_q, target_r, status, arrives_at, march_intent FROM units WHERE id = $1`,
		f.unitID,
	).Scan(&q, &r, &targetQ, &targetR, &status, &arrivesAt, &marchIntent); err != nil {
		t.Fatalf("load unit: %v", err)
	}

	if q != 2 || r != 0 {
		t.Errorf("expected unit caught at interpolated (2,0), got (%d,%d)", q, r)
	}
	if targetQ != 0 || targetR != 0 {
		t.Errorf("expected new target = origin (0,0), got (%d,%d)", targetQ, targetR)
	}
	if status != "marching" {
		t.Errorf("expected unit still marching (turning takes time), got %q", status)
	}
	if !arrivesAt.After(now) {
		t.Errorf("expected new arrives_at (%v) after now (%v) — turning around is not instant", arrivesAt, now)
	}
	if marchIntent != nil {
		t.Errorf("expected march_intent cleared on recall, got %v", *marchIntent)
	}

	var messengerStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&messengerStatus); err != nil {
		t.Fatalf("load messenger: %v", err)
	}
	if messengerStatus != "arrived" {
		t.Errorf("expected messenger marked arrived, got %q", messengerStatus)
	}
}

func TestMarchRecall_RedirectSetsNewTarget(t *testing.T) {
	pool := testPool(t)
	f := newMarchRecallFixture(t, pool)
	messengerID := insertRecallMessenger(t, pool, f)

	now := f.departsAt.Add(90 * time.Minute)
	testClk := clock.NewTestClock(now)
	h := NewMarchRecallHandler(pool, events.NewScheduler(pool, testClk), events.NewStore(pool), nil, testClk)

	newQ, newR := 2, 1 // adjacent to the interpolated catch-point (2,0)
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains') ON CONFLICT DO NOTHING`,
		f.worldID, newQ, newR,
	); err != nil {
		t.Fatalf("create redirect target tile: %v", err)
	}

	payload := MarchRecallPayload{
		WorldID: f.worldID, UnitID: f.unitID, MessengerID: messengerID, Mode: "redirect",
		NewTargetQ: &newQ, NewTargetR: &newR,
	}
	raw, _ := json.Marshal(payload)
	if err := h.Handle(context.Background(), events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var targetQ, targetR int
	if err := pool.QueryRow(context.Background(),
		`SELECT target_q, target_r FROM units WHERE id = $1`, f.unitID,
	).Scan(&targetQ, &targetR); err != nil {
		t.Fatalf("load unit: %v", err)
	}
	if targetQ != newQ || targetR != newR {
		t.Errorf("expected redirected target (%d,%d), got (%d,%d)", newQ, newR, targetQ, targetR)
	}
}

func TestMarchRecall_IdempotentReplay(t *testing.T) {
	pool := testPool(t)
	f := newMarchRecallFixture(t, pool)
	messengerID := insertRecallMessenger(t, pool, f)

	now := f.departsAt.Add(90 * time.Minute)
	testClk := clock.NewTestClock(now)
	h := NewMarchRecallHandler(pool, events.NewScheduler(pool, testClk), events.NewStore(pool), nil, testClk)

	payload := MarchRecallPayload{WorldID: f.worldID, UnitID: f.unitID, MessengerID: messengerID, Mode: "recall"}
	raw, _ := json.Marshal(payload)

	if err := h.Handle(context.Background(), events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	var arrivesAt1 time.Time
	if err := pool.QueryRow(context.Background(), `SELECT arrives_at FROM units WHERE id = $1`, f.unitID).Scan(&arrivesAt1); err != nil {
		t.Fatalf("load unit after first Handle: %v", err)
	}

	// Replay the exact same scheduled event (simulating a crash between commit
	// and markDone) at a later clock reading — a non-idempotent handler would
	// re-interpolate from the now-updated departs_at/arrives_at and turn the
	// unit again, moving arrives_at further out.
	testClk.Advance(30 * time.Minute)
	if err := h.Handle(context.Background(), events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	var arrivesAt2 time.Time
	if err := pool.QueryRow(context.Background(), `SELECT arrives_at FROM units WHERE id = $1`, f.unitID).Scan(&arrivesAt2); err != nil {
		t.Fatalf("load unit after second Handle: %v", err)
	}

	if !arrivesAt1.Equal(arrivesAt2) {
		t.Errorf("replay changed arrives_at: first=%v second=%v — handler is not idempotent", arrivesAt1, arrivesAt2)
	}
}

func TestMarchRecall_TooLateNoOp(t *testing.T) {
	pool := testPool(t)
	f := newMarchRecallFixture(t, pool)
	messengerID := insertRecallMessenger(t, pool, f)

	// The unit already completed its original march before the recall order
	// caught up — simulate the arrival handler having already flipped it to
	// garrison.
	if _, err := pool.Exec(context.Background(),
		`UPDATE units SET status = 'garrison', settlement_id = NULL, target_q = NULL, target_r = NULL,
		                  departs_at = NULL, arrives_at = NULL WHERE id = $1`,
		f.unitID,
	); err != nil {
		t.Fatalf("simulate prior arrival: %v", err)
	}

	testClk := clock.NewTestClock(f.arrivesAt.Add(time.Hour))
	h := NewMarchRecallHandler(pool, events.NewScheduler(pool, testClk), events.NewStore(pool), nil, testClk)

	payload := MarchRecallPayload{WorldID: f.worldID, UnitID: f.unitID, MessengerID: messengerID, Mode: "recall"}
	raw, _ := json.Marshal(payload)
	if err := h.Handle(context.Background(), events.ScheduledEvent{Payload: raw}); err != nil {
		t.Fatalf("Handle should no-op without error, got: %v", err)
	}

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM units WHERE id = $1`, f.unitID).Scan(&status); err != nil {
		t.Fatalf("load unit: %v", err)
	}
	if status != "garrison" {
		t.Errorf("expected unit status untouched ('garrison'), got %q", status)
	}
}
