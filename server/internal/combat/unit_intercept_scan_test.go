package combat

// Unit-vs-unit interception (avsiktslagret §S3, megaron_plan_avsiktslagret.md).
// Mirrors internal/transport/intercept_test.go's fixture/scenario style and
// unit_arrival_field_test.go's world/player/province setup.

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

// mkSentry inserts a positioned+sentry defender unit at (q,r) with the
// migration default reaction_policy (foreign→intercept).
func mkSentry(t *testing.T, pool *pgxpool.Pool, f unitInterceptFixture, q, r, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'spearman','land',$3,0,'positioned','sentry',$4,$5,$4,$5)
		 RETURNING id`,
		f.worldID, f.defender, size, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("create sentry: %v", err)
	}
	return id
}

// TestUnitInterceptScan_VisibleSentryEngagesMarchingUnit is the substrate's
// happy path (avsiktslagret §S3/§6 point 3): a foreign march passes within
// reach of a FOW-visible enemy sentry (the sentry itself is its owner's own
// eye, standing right on the march's interpolated hex) → combat resolves,
// UnitIntercepted is recorded, and losses land on both sides. RED before this
// file existed — nothing scanned marching units for sentries at all.
func TestUnitInterceptScan_VisibleSentryEngagesMarchingUnit(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	// Overwhelming attacker vs a weak sentry, right on the march's path (1,0) —
	// the sentry is its own eye, so no separate visibility fixture is needed.
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 1000)
	sentryUnit := mkSentry(t, pool, f, 1, 0, 10)

	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}

	var sentryStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, sentryUnit).Scan(&sentryStatus); err != nil {
		t.Fatalf("read sentry: %v", err)
	}
	if sentryStatus != "disbanded" {
		t.Errorf("sentry status = %q, want disbanded (overwhelmed by a 100x attacker) — no interception occurred", sentryStatus)
	}

	var attackerSize int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id=$1`, attackerUnit).Scan(&attackerSize); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attackerSize == 1000 {
		t.Errorf("attacker size = %d, want < 1000 (combat must apply SOME loss even on a win)", attackerSize)
	}

	var eventCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitIntercepted'`, attackerUnit,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count UnitIntercepted events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("UnitIntercepted event count = %d, want 1", eventCount)
	}
}

// TestUnitInterceptScan_DoubleAvskarningGuardPreventsRefight pins the
// intercepted_at/unit_interceptions guard (avsiktslagret §S3 design,
// unit_arrival.go's original TODO): a march that SURVIVES an interception
// keeps marching untouched, so a second scan tick sees it in range of the same
// sentry again — without the guard it would fight (and take losses) twice for
// the same march instance.
func TestUnitInterceptScan_DoubleAvskarningGuardPreventsRefight(t *testing.T) {
	pool := testPool(t)
	f := newUnitInterceptFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	// Neither side overwhelming — the attacker survives (loyalty 2 baseline
	// rout is 25% of strength; 200 vs 10 strength-equivalent leaves the
	// attacker comfortably alive after one round of losses).
	attackerUnit := mkMarchingUnit(t, pool, f, clk, 200)
	mkSentry(t, pool, f, 1, 0, 10)

	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan (1st sweep): %v", err)
	}
	var sizeAfterFirst int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id=$1`, attackerUnit).Scan(&sizeAfterFirst); err != nil {
		t.Fatalf("read attacker after 1st sweep: %v", err)
	}
	if sizeAfterFirst >= 200 {
		t.Fatalf("attacker size after 1st sweep = %d, want < 200 (interception must have applied a loss for this test to be meaningful)", sizeAfterFirst)
	}

	// Second sweep: same tick's re-enqueue scheduled itself for DueTick+interval,
	// so simulate the next sweep directly — the march is still 'marching' at the
	// same interpolated hex, still in range of the same sentry.
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 2}); err != nil {
		t.Fatalf("unit intercept scan (2nd sweep): %v", err)
	}
	var sizeAfterSecond int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id=$1`, attackerUnit).Scan(&sizeAfterSecond); err != nil {
		t.Fatalf("read attacker after 2nd sweep: %v", err)
	}
	if sizeAfterSecond != sizeAfterFirst {
		t.Errorf("attacker size after 2nd sweep = %d, want unchanged from %d (same sentry must not re-fight the same march instance)", sizeAfterSecond, sizeAfterFirst)
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
	var sentryUnit uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'galley','naval',1,40,'positioned','sentry',3,0,3,0)
		 RETURNING id`,
		f.worldID, f.defender,
	).Scan(&sentryUnit); err != nil {
		t.Fatalf("create ship sentry: %v", err)
	}

	h := NewUnitInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("unit intercept scan: %v", err)
	}

	var attackerSize int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id=$1`, attackerUnit).Scan(&attackerSize); err != nil {
		t.Fatalf("read attacker: %v", err)
	}
	if attackerSize != 1000 {
		t.Errorf("attacker size = %d, want unchanged 1000 (blind sentry's owner never saw the march → no interception)", attackerSize)
	}

	var eventCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE stream_id = $1 AND event_type = 'UnitIntercepted'`, attackerUnit,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count UnitIntercepted events: %v", err)
	}
	if eventCount != 0 {
		t.Errorf("UnitIntercepted event count = %d, want 0 (blind sentry never fired)", eventCount)
	}
}
