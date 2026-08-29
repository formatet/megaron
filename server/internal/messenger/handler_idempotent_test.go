package messenger

// G2 idempotency regression for ArrivalHandler (ScheduledMessengerArrival) and
// ReturnHandler (ScheduledMessengerReturn).
//
// Both handlers read the messenger's current status BEFORE mutating and
// no-op if it has already moved past the expected starting state:
// ArrivalHandler exits early unless status=='outbound' (handler.go:79);
// ReturnHandler's own doc comment: "Idempotent: if the messenger is already
// 'arrived', does nothing" (handler.go:132/150). These are DB integration
// tests (real Postgres, gated by DATABASE_URL) proving both guards: each
// drives the SAME scheduled event through Handle twice and asserts the
// messenger's status only flips once (and, for Arrival, that only one
// ScheduledMessengerReturn auto-return timer is armed).

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

func handlerIdemTestPool(t *testing.T) *pgxpool.Pool {
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

// handlerIdemFixture is a minimal world + sender + origin/destination
// settlement pair, shared by both tests below.
type handlerIdemFixture struct {
	worldID  uuid.UUID
	sender   uuid.UUID
	originID uuid.UUID
	destID   uuid.UUID
}

func newHandlerIdemFixture(t *testing.T, pool *pgxpool.Pool, tag string) handlerIdemFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	f := handlerIdemFixture{}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 500) RETURNING id`,
		"handler-idem-"+tag+"-"+uuid.NewString(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"handler-idem-"+tag+"-"+uuid.NewString(),
	).Scan(&f.sender); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mkSettlement := func(q int, name string) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province: %v", err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active') RETURNING id`,
			f.worldID, prov, name, f.sender,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.originID = mkSettlement(0, "HandlerIdem-Origin-"+tag)
	f.destID = mkSettlement(3, "HandlerIdem-Dest-"+tag)
	return f
}

func TestArrivalHandler_ReplayIsIdempotent(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "arrival")

	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'hello','outbound',0,0,now()) RETURNING id`,
		f.worldID, f.sender, f.originID, f.destID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create outbound messenger: %v", err)
	}

	payload, _ := json.Marshal(ArrivalPayload{MessengerID: messengerID})
	evt := events.ScheduledEvent{ID: 1, WorldID: f.worldID, Payload: payload, DueTick: 500}

	clk := clock.NewTestClock(time.Now())
	h := NewArrivalHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool))

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var statusAfterFirst string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&statusAfterFirst); err != nil {
		t.Fatalf("read status after first run: %v", err)
	}
	if statusAfterFirst != "delivered" {
		t.Fatalf("status after first run = %q, want delivered — fixture does not exercise the handler", statusAfterFirst)
	}

	var returnTimersAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='MessengerReturn' AND (payload->>'messenger_id')::uuid=$2`,
		f.worldID, messengerID,
	).Scan(&returnTimersAfterFirst); err != nil {
		t.Fatalf("count return timers after first run: %v", err)
	}
	if returnTimersAfterFirst != 1 {
		t.Fatalf("return timers armed after first run = %d, want 1", returnTimersAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var statusAfterReplay string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&statusAfterReplay); err != nil {
		t.Fatalf("read status after replay: %v", err)
	}
	if statusAfterReplay != "delivered" {
		t.Errorf("status after replay = %q, want still delivered", statusAfterReplay)
	}

	var returnTimersAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='MessengerReturn' AND (payload->>'messenger_id')::uuid=$2`,
		f.worldID, messengerID,
	).Scan(&returnTimersAfterReplay); err != nil {
		t.Fatalf("count return timers after replay: %v", err)
	}
	if returnTimersAfterReplay != 1 {
		t.Errorf("return timers armed after replay = %d, want still 1 (a non-idempotent handler would arm a second auto-return timer)", returnTimersAfterReplay)
	}
}

func TestReturnHandler_ReplayIsIdempotent(t *testing.T) {
	pool := handlerIdemTestPool(t)
	ctx := context.Background()
	f := newHandlerIdemFixture(t, pool, "return")

	var messengerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'hello','delivered',0,0,now()) RETURNING id`,
		f.worldID, f.sender, f.originID, f.destID,
	).Scan(&messengerID); err != nil {
		t.Fatalf("create delivered messenger: %v", err)
	}

	payload, _ := json.Marshal(ReturnPayload{MessengerID: messengerID})
	evt := events.ScheduledEvent{ID: 1, WorldID: f.worldID, Payload: payload}

	h := NewReturnHandler(pool, events.NewStore(pool))

	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (first run): %v", err)
	}

	var statusAfterFirst string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&statusAfterFirst); err != nil {
		t.Fatalf("read status after first run: %v", err)
	}
	if statusAfterFirst != "arrived" {
		t.Fatalf("status after first run = %q, want arrived — fixture does not exercise the handler", statusAfterFirst)
	}

	var eventCountAfterFirst int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE world_id=$1 AND event_type='MessengerReturned' AND payload->>'messenger_id'=$2`,
		f.worldID, messengerID.String(),
	).Scan(&eventCountAfterFirst); err != nil {
		t.Fatalf("count MessengerReturned audit events after first run: %v", err)
	}
	if eventCountAfterFirst != 1 {
		t.Fatalf("MessengerReturned audit events after first run = %d, want 1", eventCountAfterFirst)
	}

	// Replay the SAME event.
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle (replay): %v", err)
	}

	var statusAfterReplay string
	if err := pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id = $1`, messengerID).Scan(&statusAfterReplay); err != nil {
		t.Fatalf("read status after replay: %v", err)
	}
	if statusAfterReplay != "arrived" {
		t.Errorf("status after replay = %q, want still arrived", statusAfterReplay)
	}

	var eventCountAfterReplay int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE world_id=$1 AND event_type='MessengerReturned' AND payload->>'messenger_id'=$2`,
		f.worldID, messengerID.String(),
	).Scan(&eventCountAfterReplay); err != nil {
		t.Fatalf("count MessengerReturned audit events after replay: %v", err)
	}
	if eventCountAfterReplay != 1 {
		t.Errorf("MessengerReturned audit events after replay = %d, want still 1 (a non-idempotent handler would append a second one)", eventCountAfterReplay)
	}
}
