package combat

// Unit-vs-unit interception (avsiktslagret §S3/§7, megaron_plan_avsiktslagret.md,
// megaron_plan_kr3_stridssystem.md §7). Mirrors internal/transport/intercept_test.go's
// fixture/scenario style and unit_arrival_field_test.go's world/player/province setup.
//
// §7 cutover (2026-08-07): an "intercept"-policy sentry now HALTS the marching
// unit and initiates/joins a persistent KR3 battle instead of resolving one
// immediate roll — these tests assert the battle is correctly initiated (and,
// where the old suite asserted final losses/notifications, run it to
// completion via BattleTickHandler/runBattleToEnd, same as
// battle_notify_test.go's TestBattleTickHandler_NotifiesBothSidesOnBattleEnd).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type unitInterceptFixture struct {
	worldID  uuid.UUID
	attacker uuid.UUID // marching unit's owner
	defender uuid.UUID // sentry's owner
}

// newUnitInterceptFixture builds a 4-hex land strip (0,0)…(3,0) and an
// attacker with a capital at (0,0) (combat's pop-loss/kharis lookups need one
// to exist even when not asserted on). The defender is created WITHOUT a
// settlement by default — tests add one only when the FOW/eyes scenario needs it.
func newUnitInterceptFixture(t *testing.T, pool *pgxpool.Pool) unitInterceptFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f unitInterceptFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	f.attacker = mkPlayer("attacker")
	f.defender = mkPlayer("defender")

	for q := 0; q <= 3; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("insert map tile (%d,0): %v", q, err)
		}
	}

	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Attacker Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		f.worldID, attCapProv, f.attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}
	for q := 1; q <= 3; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("create province (%d,0): %v", q, err)
		}
	}

	return f
}

// mkMarchingUnit inserts a marching land unit travelling (0,0)→(3,0) over the
// clock's [-1h,+1h] window (halfway now → interpolated position (1,0)).
func mkMarchingUnit(t *testing.T, pool *pgxpool.Pool, f unitInterceptFixture, clk *clock.TestClock, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                     q, r, target_q, target_r, departs_at, arrives_at, depart_tick, capture_mode)
		 VALUES ($1,$2,'spearman','land',$3,0,'marching',0,0,3,0,$4,$5,1,'sack')
		 RETURNING id`,
		f.worldID, f.attacker, size, clk.Now().Add(-1*time.Hour), clk.Now().Add(1*time.Hour),
	).Scan(&id); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}
	return id
}

// mkSentry inserts a positioned+sentry defender unit at (q,r) with an
// explicit reaction_policy.foreign verb ("intercept"/"alert"/"escort").
func mkSentry(t *testing.T, pool *pgxpool.Pool, f unitInterceptFixture, q, r, size int, verb string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r, reaction_policy)
		 VALUES ($1,$2,'spearman','land',$3,0,'positioned','sentry',$4,$5,$4,$5,jsonb_build_object('foreign',$6::text,'own','ignore','ally','ignore'))
		 RETURNING id`,
		f.worldID, f.defender, size, q, r, verb,
	).Scan(&id); err != nil {
		t.Fatalf("create sentry: %v", err)
	}
	return id
}

// loadBattleIDForIntercept reads the single battles row created at (q,r) —
// same shape as battle_test.go's loadBattleID, duplicated locally since these
// two test files intentionally stay independent (different fixtures).
func loadBattleIDForIntercept(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM battles WHERE world_id = $1 AND q = $2 AND r = $3`, worldID, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("load battle at (%d,%d): %v", q, r, err)
	}
	return id
}

// TestUnitInterceptScan_VisibleSentryEngagesMarchingUnit is the substrate's
// happy path (avsiktslagret §S3/§6 point 3, KR3 §7): a foreign march passes
// within reach of a FOW-visible enemy sentry (the sentry itself is its
// owner's own eye, standing right on the march's interpolated hex) → the
// marching unit HALTS there and a persistent battle is initiated with both
// units as participants. RED before this file existed — nothing scanned
// marching units for sentries at all.
func TestUnitInterceptScan_VisibleSentryEngagesMarchingUnit(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 1000)
	sentryUnit := mkSentry(t, pool, f, 1, 0, 10, "intercept")

	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}

	var attStatus string
	var attQ, attR int
	if err := pool.QueryRow(ctx, `SELECT status, q, r FROM units WHERE id=$1`, attackerUnit).Scan(&attStatus, &attQ, &attR); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attStatus != "positioned" {
		t.Errorf("attacker status = %q, want positioned — an intercepted march must halt, not keep going (Timothy 2026-08-07)", attStatus)
	}
	if attQ != 1 || attR != 0 {
		t.Errorf("attacker halted at (%d,%d), want (1,0) — the interception hex", attQ, attR)
	}

	battleID := loadBattleIDForIntercept(t, pool, f.worldID, 1, 0)
	var partCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battle_participants WHERE battle_id = $1 AND unit_id IN ($2, $3)`,
		battleID, attackerUnit, sentryUnit,
	).Scan(&partCount); err != nil {
		t.Fatalf("count participants: %v", err)
	}
	if partCount != 2 {
		t.Errorf("battle_participants count = %d, want 2 (marching unit + sentry)", partCount)
	}

	var eventCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'BattleStarted'`, battleID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count BattleStarted events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("BattleStarted event count = %d, want 1", eventCount)
	}
}

// TestUnitInterceptScan_DoubleAvskarningGuardPreventsRefight pins the
// intercepted_at/unit_interceptions guard (avsiktslagret §S3 design,
// unit_arrival.go's original TODO) for the "alert" verb — the one remaining
// case where a marching unit is NOT halted (it keeps marching untouched), so
// a second scan tick sees it in range of the same sentry again. Without the
// guard the owner would be renotified every tick for the same march instance.
// ("intercept" no longer needs this guard for its own path — halting the unit
// takes it out of the 'marching' query entirely — but the guard is verb-
// agnostic, so this still proves it fires.)
func TestUnitInterceptScan_DoubleAvskarningGuardPreventsRefight(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 200)
	mkSentry(t, pool, f, 1, 0, 10, "alert")

	fb := &fakeBroadcaster{}
	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, fb)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan (1st sweep): %v", err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 2}); err != nil {
		t.Fatalf("unit intercept scan (2nd sweep): %v", err)
	}

	var attStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, attackerUnit).Scan(&attStatus); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attStatus != "marching" {
		t.Errorf("attacker status = %q, want marching — an alert-only sentry must never halt the march", attStatus)
	}
	if len(fb.notified) != 1 {
		t.Errorf("notified %d times across two sweeps, want 1 — the guard must prevent a repeat SentryAlerted for the same march instance", len(fb.notified))
	}
}

// TestUnitInterceptScan_BlindSentryDoesNotEngage is the FOW gate on unit-vs-unit
// interception (avsiktslagret §4, mirrors transport's caravan FOW test): a
// sentry within UnitInterceptRadius whose owner cannot actually see the
// march's live position must never engage it. A naval sentry sees only 1 hex
// over land (province.LiveRadius, EyeShip); posted 2 hexes from the march's
// interpolated position with no other eye nearby, it is blind to it.
func TestUnitInterceptScan_BlindSentryDoesNotEngage(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 1000)

	// Naval sentry at (3,0), watching that hex — distance 2 to the march's
	// interpolated (1,0) = UnitInterceptRadius, so proximity alone would catch
	// it, but a ship's land vision radius is 1.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'galley','naval',1,40,'positioned','sentry',3,0,3,0)`,
		f.worldID, f.defender,
	); err != nil {
		t.Fatalf("create ship sentry: %v", err)
	}

	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}

	var attStatus string
	var attackerSize int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id=$1`, attackerUnit).Scan(&attStatus, &attackerSize); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attStatus != "marching" || attackerSize != 1000 {
		t.Errorf("attacker = (%s, %d), want (marching, 1000) unchanged — blind sentry's owner never saw the march → no interception", attStatus, attackerSize)
	}

	var battleCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battles WHERE world_id = $1`, f.worldID,
	).Scan(&battleCount); err != nil {
		t.Fatalf("count battles: %v", err)
	}
	if battleCount != 0 {
		t.Errorf("battles count = %d, want 0 (blind sentry never fired)", battleCount)
	}
}

// TestUnitInterceptScan_EscortSentryNeverEngages is KR3 §7: a sentry posted
// "escort" must never trigger this scan at all, unlike "alert" (notify-only)
// or "intercept" (fight) — it is excluded from the query itself.
func TestUnitInterceptScan_EscortSentryNeverEngages(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 1000)
	mkSentry(t, pool, f, 1, 0, 10, "escort")

	fb := &fakeBroadcaster{}
	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, fb)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}

	var attStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, attackerUnit).Scan(&attStatus); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attStatus != "marching" {
		t.Errorf("attacker status = %q, want marching — an escort-postured sentry must never trigger this scan", attStatus)
	}
	if len(fb.notified) != 0 {
		t.Errorf("notified %d times, want 0 — escort must be silent, not just non-lethal", len(fb.notified))
	}
	var battleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM battles WHERE world_id = $1`, f.worldID).Scan(&battleCount); err != nil {
		t.Fatalf("count battles: %v", err)
	}
	if battleCount != 0 {
		t.Errorf("battles count = %d, want 0", battleCount)
	}
}

// TestUnitInterceptScan_AlertSentryNotifiesWithoutCombat is KR3 §7's "alert"
// verb: "delta inte, larma bara" — the sentry's owner is notified a foreign
// unit passed within reach, but no battle happens and the march is untouched.
func TestUnitInterceptScan_AlertSentryNotifiesWithoutCombat(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 1000)
	mkSentry(t, pool, f, 1, 0, 10, "alert")

	fb := &fakeBroadcaster{}
	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, fb)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}

	var attStatus string
	var attackerSize int
	if err := pool.QueryRow(ctx, `SELECT status, size FROM units WHERE id=$1`, attackerUnit).Scan(&attStatus, &attackerSize); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attStatus != "marching" || attackerSize != 1000 {
		t.Errorf("attacker = (%s, %d), want (marching, 1000) unchanged — alert must never apply combat", attStatus, attackerSize)
	}

	var battleCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM battles WHERE world_id = $1`, f.worldID).Scan(&battleCount); err != nil {
		t.Fatalf("count battles: %v", err)
	}
	if battleCount != 0 {
		t.Errorf("battles count = %d, want 0 — alert must not fight", battleCount)
	}

	if len(fb.notified) != 1 {
		t.Fatalf("notified %d times, want 1", len(fb.notified))
	}
	if fb.notified[0] != "SentryAlerted" {
		t.Errorf("notification kind = %q, want SentryAlerted", fb.notified[0])
	}
	payload, ok := fb.payloads[0].(map[string]any)
	if !ok {
		t.Fatalf("payload is not map[string]any: %T", fb.payloads[0])
	}
	if payload["foreign_owner"] == "" || payload["foreign_owner"] == nil {
		t.Error("foreign_owner is empty")
	}
}

// TestUnitInterceptScan_InterceptThenBattleNotifiesBothSides is avsiktslagret
// §S4 (megaron_plan_avsiktslagret.md) carried across the §7 cutover: the
// notification contract (both owners, BattleWon/BattleLost, opponent named)
// must hold even though it now fires from BattleTickHandler.notifyBattleEnded
// once the battle concludes, not immediately at interception — same pattern
// as battle_notify_test.go's TestBattleTickHandler_NotifiesBothSidesOnBattleEnd.
func TestUnitInterceptScan_InterceptThenBattleNotifiesBothSides(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	mkMarchingUnit(t, pool, f, clk, 1000)
	mkSentry(t, pool, f, 1, 0, 10, "intercept")

	scanFB := &fakeBroadcaster{}
	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, scanFB)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}
	if len(scanFB.notified) != 0 {
		t.Errorf("notified %d times at interception, want 0 — the report waits for the battle to end (§8 cutover)", len(scanFB.notified))
	}

	battleID := loadBattleIDForIntercept(t, pool, f.worldID, 1, 0)
	battleFB := &fakeBroadcaster{}
	battleH := NewBattleTickHandler(pool, events.NewStore(pool), events.NewScheduler(pool, clk), battleFB, clk)
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 20)

	var attackerNotified, defenderNotified map[string]any
	for i, k := range battleFB.notified {
		if k != "BattleWon" && k != "BattleLost" {
			t.Fatalf("unexpected notification kind %q", k)
		}
		payload, ok := battleFB.payloads[i].(map[string]any)
		if !ok {
			t.Fatalf("payload %d is not map[string]any: %T", i, battleFB.payloads[i])
		}
		switch payload["role"] {
		case "attacker":
			attackerNotified = payload
		case "defender":
			defenderNotified = payload
		}
	}
	if attackerNotified == nil {
		t.Fatal("attacker owner never notified")
	}
	if defenderNotified == nil {
		t.Fatal("defender owner never notified")
	}
	if attackerNotified["opponent_name"] == "" || attackerNotified["opponent_name"] == nil {
		t.Error("attacker's opponent_name is empty")
	}
	if defenderNotified["opponent_name"] == "" || defenderNotified["opponent_name"] == nil {
		t.Error("defender's opponent_name is empty")
	}
}
