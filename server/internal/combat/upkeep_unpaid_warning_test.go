package combat

// SLICE A (megaron_todo.md, 2026-07-31): recordUnpaid's silent branch — unpaid_periods
// climbing 1 → 2 with zero player-facing signal until desertion itself fired two
// speldygn later — now fires a forewarning notification, kind UpkeepUnpaid, the
// same tick the shortfall happens. These tests prove:
//  1. period 1 unpaid → unpaid_periods=1, UpkeepUnpaid notified, level 3 (info)
//  2. period 2 unpaid → unpaid_periods=2, UpkeepUnpaid notified, level 2 (urgent)
//  3. period 3 unpaid → desertion fires exactly as before (UnitDeserted), NOT
//     UpkeepUnpaid — nothing in that branch changed
//  4. dedupe is keyed on kind+unit_id+unpaid_periods, not kind+unit_id alone: an
//     unread period-1 warning must not suppress the period-2 escalation (a
//     materially more urgent notice), but an exact repeat of the same period
//     position is suppressed.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// unpaidWarningRecorder is a Broadcaster test double that both records each
// NotifyPlayer call in memory AND persists to the real notifications table via
// the test pool — same pattern as kharis's notifyRecorder
// (internal/kharis/starvation_warning_test.go) — so notifyUpkeepUnpaid's own
// dedupe query reads real unread rows, exactly like *notify.Hub does in
// production. combat may not import internal/notify (G1 — package dependency
// order), so this reimplements just the one INSERT the production Hub does,
// not the whole hub.
type unpaidWarningRecorder struct {
	pool  *pgxpool.Pool
	calls []unpaidWarningCall
}

type unpaidWarningCall struct {
	kind    string
	level   int
	payload map[string]any
}

func (r *unpaidWarningRecorder) BroadcastEvent(worldID uuid.UUID, kind string, payload any) {}

func (r *unpaidWarningRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	m, _ := payload.(map[string]any)
	r.calls = append(r.calls, unpaidWarningCall{kind: kind, level: level, payload: m})
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO notifications (world_id, player_id, kind, level, body_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		worldID, playerID, kind, level, bodyJSON,
	)
	return err
}

// TestUpkeepUnpaidWarning_HandleFlow_AllPeriodsUntilDesertion drives the real
// Handle loop across consecutive daily upkeep ticks for one silver-starved
// unit, mirroring how the recurring tick actually calls this (events.MacroTickInterval,
// upkeep.go:283) once per tick. Covers AK1–AK3.
//
// ⭐ CANON 2026-08-06: upkeepDesertionTicks went 3 → 72 (invariant-preserving
// retune — tick.go comment above the constant), so this now drives
// upkeepDesertionTicks periods, not a hardcoded 3. The loop bound reads the
// constant directly so a future recalibration doesn't silently desync the test
// from the code it's supposed to pin.
func TestUpkeepUnpaidWarning_HandleFlow_AllPeriodsUntilDesertion(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "unpaid-flow")

	seedGoods(t, pool, f.capitalID, f.tick, 100000, 100000)
	// Plenty of grain (grain upkeep always succeeds), zero silver (silver
	// upkeep fails every tick — the exact scenario recordUnpaid's silent
	// counter was built for).
	seedGoods(t, pool, f.townID, f.tick, 100000, 0)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', $3, $3, 0)
		 RETURNING id`,
		f.worldID, f.owner, f.townID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create unit: %v", err)
	}

	rec := &unpaidWarningRecorder{pool: pool}
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	store := events.NewStore(pool)
	h := NewUpkeepHandler(pool, sched, store, rec)

	runOneDay := func(tick int) {
		t.Helper()
		// Distinct event ID per simulated day (tick is unique per call here),
		// mirroring production: EnqueueTickRecurring always inserts a FRESH
		// scheduled_events row for the next day, never reuses the previous
		// day's id. Handle's G2 per-unit claim (event_id, unit_id) — added to
		// close a real double-charge bug — treats two calls with the SAME
		// event ID as a replay of one event, so reusing a zero-value ID across
		// all 72 simulated days would wrongly collapse them into a single
		// claimed day instead of 72 distinct ones.
		if err := h.Handle(ctx, events.ScheduledEvent{ID: int64(tick), WorldID: f.worldID, DueTick: tick}); err != nil {
			t.Fatalf("upkeep Handle at tick %d: %v", tick, err)
		}
	}
	unpaidPeriods := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT unpaid_periods FROM units WHERE id = $1`, unitID).Scan(&n); err != nil {
			t.Fatalf("read unpaid_periods: %v", err)
		}
		return n
	}
	lastCallOfKind := func(kind string) (unpaidWarningCall, bool) {
		for i := len(rec.calls) - 1; i >= 0; i-- {
			if rec.calls[i].kind == kind {
				return rec.calls[i], true
			}
		}
		return unpaidWarningCall{}, false
	}

	// ── Period 1: first unpaid period ────────────────────────────────────────
	runOneDay(f.tick)
	if got := unpaidPeriods(); got != 1 {
		t.Fatalf("AK1: unpaid_periods after period 1 = %d, want 1", got)
	}
	call, ok := lastCallOfKind(upkeepUnpaidWarningKind)
	if !ok {
		t.Fatalf("AK1: no %s notification recorded after period 1; got calls %+v", upkeepUnpaidWarningKind, rec.calls)
	}
	if call.level != 3 {
		t.Errorf("AK1: level = %d, want 3 (info)", call.level)
	}
	if got := call.payload["periods_until_desertion"]; got != upkeepDesertionTicks-1 {
		t.Errorf("AK1: periods_until_desertion = %v, want %d", got, upkeepDesertionTicks-1)
	}
	if got := call.payload["unpaid_periods"]; got != 1 {
		t.Errorf("AK1: payload unpaid_periods = %v, want 1", got)
	}
	if got := call.payload["settlement_id"]; got != f.townID {
		t.Errorf("AK1: settlement_id = %v, want %v", got, f.townID)
	}

	// Verify the actual DB row too (bevisplan step 6) — not just the in-memory
	// recorder's own bookkeeping.
	var dbKind string
	var dbLevel int
	var dbBody []byte
	if err := pool.QueryRow(ctx,
		`SELECT kind, level, body_json FROM notifications
		 WHERE world_id = $1 AND player_id = $2 AND kind = $3`,
		f.worldID, f.owner, upkeepUnpaidWarningKind,
	).Scan(&dbKind, &dbLevel, &dbBody); err != nil {
		t.Fatalf("read notifications row after period 1: %v", err)
	}
	t.Logf("period 1 notifications row: kind=%s level=%d body=%s", dbKind, dbLevel, dbBody)
	if dbLevel != 3 {
		t.Errorf("AK1: DB row level = %d, want 3", dbLevel)
	}

	// ── Periods 2 .. upkeepDesertionTicks-1: keep accruing until one period
	// before desertion. Only level-3 (info) notifications fire in this range —
	// notifyUpkeepUnpaid only escalates to level 2 when periods_until_desertion
	// == 1, i.e. exactly the LAST period before desertion (upkeep.go:411-414).
	// Loop bound reads the constant, not a hardcoded 72, so a future
	// recalibration doesn't silently desync this test from the code.
	for period := 2; period < upkeepDesertionTicks; period++ {
		runOneDay(f.tick + (period - 1))
		if got := unpaidPeriods(); got != period {
			t.Fatalf("unpaid_periods after period %d = %d, want %d", period, got, period)
		}
	}

	// ── AK2: the period immediately before desertion — urgent escalation ────
	call, ok = lastCallOfKind(upkeepUnpaidWarningKind)
	if !ok {
		t.Fatalf("AK2: no %s notification recorded before desertion", upkeepUnpaidWarningKind)
	}
	if call.level != 2 {
		t.Errorf("AK2: level = %d, want 2 (urgent, last period before desertion)", call.level)
	}
	if got := call.payload["periods_until_desertion"]; got != 1 {
		t.Errorf("AK2: periods_until_desertion = %v, want 1", got)
	}
	if got := call.payload["unpaid_periods"]; got != upkeepDesertionTicks-1 {
		t.Errorf("AK2: payload unpaid_periods = %v, want %d", got, upkeepDesertionTicks-1)
	}

	// ── AK3: the desertion period — fires exactly as before ─────────────────
	sizeBefore := 100
	runOneDay(f.tick + (upkeepDesertionTicks - 1))
	if got := unpaidPeriods(); got != upkeepDesertionTicks {
		t.Fatalf("AK3: unpaid_periods after the desertion period = %d, want %d", got, upkeepDesertionTicks)
	}
	if _, ok := lastCallOfKind("UnitDeserted"); !ok {
		t.Errorf("AK3: no UnitDeserted notification recorded on the desertion tick")
	}
	// The desertion tick must NOT also fire a fresh UpkeepUnpaid — recordUnpaid's
	// np >= upkeepDesertionTicks branch is untouched by this slice.
	for _, c := range rec.calls {
		if c.kind == upkeepUnpaidWarningKind && c.payload["unpaid_periods"] == upkeepDesertionTicks {
			t.Errorf("AK3: got an UpkeepUnpaid notification for the desertion period — the desertion branch must not also warn")
		}
	}
	var size int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&size); err != nil {
		t.Fatalf("read unit size: %v", err)
	}
	if size >= sizeBefore {
		t.Errorf("AK3: unit did not desert men on the desertion period: %d → %d", sizeBefore, size)
	}
}

// TestUpkeepUnpaidWarning_EscalationNotSuppressedByUnreadFirstPeriod isolates the
// dedupe decision (AK4): an unread period-1 UpkeepUnpaid must never suppress the
// period-2 escalation, because the two carry different urgency (level 3 vs 2).
// Exercises notifyUpkeepUnpaid directly to pin down the dedupe semantics without
// depending on Handle's full unit-loop machinery.
func TestUpkeepUnpaidWarning_EscalationNotSuppressedByUnreadFirstPeriod(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "unpaid-escalation")

	u := upkeepUnitRow{id: uuid.New(), ownerID: f.owner, unitType: "spearman"}
	rec := &unpaidWarningRecorder{pool: pool}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), rec)

	// Period 1: informational, left unread (the player never opened the feed).
	h.notifyUpkeepUnpaid(ctx, u, f.worldID, f.townID, 1, 2, 4.0)
	// Period 2: urgent escalation, same unit, period-1 warning still unread.
	h.notifyUpkeepUnpaid(ctx, u, f.worldID, f.townID, 2, 1, 4.0)

	if len(rec.calls) != 2 {
		t.Fatalf("NotifyPlayer called %d times, want 2 — an unread period-1 warning must not suppress the period-2 escalation", len(rec.calls))
	}
	if rec.calls[0].level != 3 {
		t.Errorf("period-1 level = %d, want 3 (info)", rec.calls[0].level)
	}
	if rec.calls[1].level != 2 {
		t.Errorf("period-2 level = %d, want 2 (urgent)", rec.calls[1].level)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE world_id = $1 AND player_id = $2 AND kind = $3 AND read_at IS NULL`,
		f.worldID, f.owner, upkeepUnpaidWarningKind,
	).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if n != 2 {
		t.Fatalf("unread notifications rows = %d, want 2 (one per period position)", n)
	}
}

// TestUpkeepUnpaidWarning_DedupeExactRepeat_Suppressed is the other half of AK4:
// an exact repeat of the SAME period position (e.g. an idempotency retry firing
// notifyUpkeepUnpaid twice for period 1) must be suppressed, matching the spirit
// of notifyUnitLoss's existing dedupe — just keyed one level more precisely.
func TestUpkeepUnpaidWarning_DedupeExactRepeat_Suppressed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "unpaid-dedupe-exact")

	u := upkeepUnitRow{id: uuid.New(), ownerID: f.owner, unitType: "spearman"}
	rec := &unpaidWarningRecorder{pool: pool}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), rec)

	h.notifyUpkeepUnpaid(ctx, u, f.worldID, f.townID, 1, 2, 4.0)
	h.notifyUpkeepUnpaid(ctx, u, f.worldID, f.townID, 1, 2, 4.0) // exact repeat, same period

	if len(rec.calls) != 1 {
		t.Fatalf("NotifyPlayer called %d times for an exact-repeat period, want 1 (suppressed)", len(rec.calls))
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE world_id = $1 AND player_id = $2 AND kind = $3`,
		f.worldID, f.owner, upkeepUnpaidWarningKind,
	).Scan(&n); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if n != 1 {
		t.Fatalf("notifications rows = %d, want 1", n)
	}
}
