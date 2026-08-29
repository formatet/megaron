package combat

// KR2 — fientliga härar i samma hex möts (megaron_plan_kr2_motet.md).
//
// RED BEFORE this file's production counterpart (march_encounter.go) existed:
// nothing in the codebase ever compared two marching units' interpolated
// positions to each other. Combat only triggered at ARRIVAL (unit_arrival.go,
// status flips to 'positioned' first) or against a STATIONARY sentry
// (unit_intercept_scan.go, avsiktslagret §S3) — two columns that are BOTH
// still marching when their paths cross mid-route walked straight through
// each other. TestMarchEncounter_CrossingHostileMarchesCollide is the proof:
// before march_encounter.go, MarchEncounterHandler didn't exist at all, so
// this file failed to compile — the sharpest possible "red".
//
// Fixture note: the crossing scenario deliberately uses an ODD-length path
// (3 hexes, (0,0)-(1,0)-(2,0)) so InterpolateAlongPath's
// round(progress*(len(path)-1)) lands on a whole index (progress=0.5 → idx=1)
// with no rounding-mode ambiguity, instead of reusing unit_intercept_scan_test.go's
// 4-hex mkMarchingUnit fixture (progress=0.5 → idx=round(1.5), rounding-mode
// dependent). Both units traverse the SAME window (departs -1h, arrives +1h)
// in opposite directions, so by symmetry both resolve to the midpoint hex (1,0)
// at "now" — no guessing, no debug prints, just an odd path length.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// meFixture is a 3-hex land strip (0,0)-(1,0)-(2,0) plus two players, each
// with a capital (combat's pop-loss/kharis lookups need one to exist even
// when not asserted on — same reasoning as newUnitInterceptFixture).
type meFixture struct {
	worldID uuid.UUID
	ownerA  uuid.UUID
	ownerB  uuid.UUID
}

func newMarchEncounterFixture(t *testing.T, pool *pgxpool.Pool) meFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f meFixture
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
			`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(),
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	f.ownerA = mkPlayer("owner-a")
	f.ownerB = mkPlayer("owner-b")

	for q := 0; q <= 2; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("insert map tile (%d,0): %v", q, err)
		}
	}

	var provA uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&provA); err != nil {
		t.Fatalf("create owner-a capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Owner A Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		f.worldID, provA, f.ownerA,
	); err != nil {
		t.Fatalf("create owner-a capital: %v", err)
	}
	for q := 1; q <= 2; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("create province (%d,0): %v", q, err)
		}
	}

	return f
}

// mkCrossingUnit inserts a marching land unit travelling originQ→targetQ (both
// on row r=0) over the clock's [-1h,+1h] window — halfway "now" puts it at
// the path's midpoint hex. arriveTick is nullable (pass nil for "not due this
// tick", the common case every test but the arrival-wins trap uses).
func mkCrossingUnit(t *testing.T, pool *pgxpool.Pool, worldID, owner uuid.UUID, clk *clock.TestClock, originQ, targetQ, size int, arriveTick *int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                     q, r, target_q, target_r, departs_at, arrives_at, depart_tick, arrive_tick)
		 VALUES ($1,$2,'spearman','land',$3,0,'marching',$4,0,$5,0,$6,$7,1,$8)
		 RETURNING id`,
		worldID, owner, size, originQ, targetQ,
		clk.Now().Add(-1*time.Hour), clk.Now().Add(1*time.Hour), arriveTick,
	).Scan(&id); err != nil {
		t.Fatalf("create crossing unit: %v", err)
	}
	return id
}

// setReactionForeign overrides a unit's reaction_policy.foreign verb —
// mirrors mkSentry's inline jsonb_build_object, but as a post-insert UPDATE
// since mkCrossingUnit doesn't take a verb parameter (most tests want the
// column DEFAULT, "intercept", unchanged).
func setReactionForeign(t *testing.T, pool *pgxpool.Pool, unitID uuid.UUID, verb string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE units SET reaction_policy = jsonb_set(reaction_policy, '{foreign}', to_jsonb($2::text)) WHERE id = $1`,
		unitID, verb,
	); err != nil {
		t.Fatalf("set reaction_policy.foreign: %v", err)
	}
}

func loadBattleIDAt(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) (uuid.UUID, bool) {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM battles WHERE world_id = $1 AND q = $2 AND r = $3`, worldID, q, r,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func unitStatusPos(t *testing.T, pool *pgxpool.Pool, unitID uuid.UUID) (status string, q, r *int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, q, r FROM units WHERE id = $1`, unitID,
	).Scan(&status, &q, &r); err != nil {
		t.Fatalf("read unit %s: %v", unitID, err)
	}
	return
}

// TestMarchEncounter_CrossingHostileMarchesCollide is the huvudfall: two
// fientliga marching armies whose paths cross end up on the SAME hex at the
// same tick — must halt and fight, not pass through.
func TestMarchEncounter_CrossingHostileMarchesCollide(t *testing.T) {
	pool := testPool(t)
	f := newMarchEncounterFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	unitA := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 0, 2, 1000, nil)
	unitB := mkCrossingUnit(t, pool, f.worldID, f.ownerB, clk, 2, 0, 500, nil)

	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.worldID, DueTick: 0}); err != nil {
		t.Fatalf("march encounter handle: %v", err)
	}

	statusA, qA, rA := unitStatusPos(t, pool, unitA)
	statusB, qB, rB := unitStatusPos(t, pool, unitB)
	if statusA != "positioned" || qA == nil || *qA != 1 || rA == nil || *rA != 0 {
		t.Errorf("unit A = (%s, %v, %v), want (positioned, 1, 0) — a crossing march must halt at the shared hex, not pass through", statusA, qA, rA)
	}
	if statusB != "positioned" || qB == nil || *qB != 1 || rB == nil || *rB != 0 {
		t.Errorf("unit B = (%s, %v, %v), want (positioned, 1, 0)", statusB, qB, rB)
	}

	battleID, ok := loadBattleIDAt(t, pool, f.worldID, 1, 0)
	if !ok {
		t.Fatalf("no battle at (1,0) — the crossing was never detected")
	}
	var partCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battle_participants WHERE battle_id = $1 AND unit_id IN ($2, $3)`,
		battleID, unitA, unitB,
	).Scan(&partCount); err != nil {
		t.Fatalf("count participants: %v", err)
	}
	if partCount != 2 {
		t.Errorf("battle_participants count = %d, want 2 (both crossing units)", partCount)
	}
}

// TestMarchEncounter_OwnUnitsPassThrough: two units under the SAME owner
// sharing a hex must never fight each other — "own" is always excluded, no
// reaction_policy involved at all.
func TestMarchEncounter_OwnUnitsPassThrough(t *testing.T) {
	pool := testPool(t)
	f := newMarchEncounterFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	unitA := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 0, 2, 1000, nil)
	unitA2 := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 2, 0, 500, nil)

	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.worldID, DueTick: 0}); err != nil {
		t.Fatalf("march encounter handle: %v", err)
	}

	statusA, _, _ := unitStatusPos(t, pool, unitA)
	statusA2, _, _ := unitStatusPos(t, pool, unitA2)
	if statusA != "marching" || statusA2 != "marching" {
		t.Errorf("own units = (%s, %s), want (marching, marching) unchanged — own units never fight", statusA, statusA2)
	}
	if _, ok := loadBattleIDAt(t, pool, f.worldID, 1, 0); ok {
		t.Errorf("a battle was created for two own units sharing a hex")
	}
}

// TestMarchEncounter_ArrivalWinsOverEncounter pins trap #1 (plan §4): a unit
// due to ARRIVE this exact tick (arrive_tick <= currentTick) must be resolved
// by the arrival handler, never by this scan — even though it is still
// technically status='marching' (the arrival event hasn't run yet) and shares
// a hex with a hostile marching unit right now. current_world_tick() is 0 for
// a freshly created test world (worlds.current_tick DEFAULT 0), so
// arrive_tick=0 is "due this tick".
func TestMarchEncounter_ArrivalWinsOverEncounter(t *testing.T) {
	pool := testPool(t)
	f := newMarchEncounterFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	dueNow := 0
	unitA := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 0, 2, 1000, &dueNow)
	unitB := mkCrossingUnit(t, pool, f.worldID, f.ownerB, clk, 2, 0, 500, nil)

	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.worldID, DueTick: 0}); err != nil {
		t.Fatalf("march encounter handle: %v", err)
	}

	statusA, _, _ := unitStatusPos(t, pool, unitA)
	statusB, _, _ := unitStatusPos(t, pool, unitB)
	if statusA != "marching" {
		t.Errorf("unit A (arriving this tick) status = %q, want marching — the scan must leave it for the arrival handler", statusA)
	}
	if statusB != "marching" {
		t.Errorf("unit B status = %q, want marching — its only counterpart was excluded, so the hex never had ≥2 owners in scope", statusB)
	}
	if _, ok := loadBattleIDAt(t, pool, f.worldID, 1, 0); ok {
		t.Errorf("a battle was created even though the only foreign counterpart is due to arrive this tick")
	}
}

// TestMarchEncounter_ReactionPolicyRespected: detection is unconditional but
// combat is not — a unit whose reaction_policy.foreign is "ignore" must not
// be dragged into a fight it opted out of, and with only one intercept-minded
// owner left in the group, no battle happens at all (plan §5's own wording:
// "en enhet vars policy är att undvika slåss inte").
func TestMarchEncounter_ReactionPolicyRespected(t *testing.T) {
	pool := testPool(t)
	f := newMarchEncounterFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	unitA := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 0, 2, 1000, nil)
	unitB := mkCrossingUnit(t, pool, f.worldID, f.ownerB, clk, 2, 0, 500, nil)
	setReactionForeign(t, pool, unitB, "ignore")

	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: f.worldID, DueTick: 0}); err != nil {
		t.Fatalf("march encounter handle: %v", err)
	}

	statusA, _, _ := unitStatusPos(t, pool, unitA)
	statusB, _, _ := unitStatusPos(t, pool, unitB)
	if statusA != "marching" || statusB != "marching" {
		t.Errorf("units = (%s, %s), want (marching, marching) — B's ignore policy means no battle for either side", statusA, statusB)
	}
	if _, ok := loadBattleIDAt(t, pool, f.worldID, 1, 0); ok {
		t.Errorf("a battle was created despite B's reaction_policy.foreign=ignore")
	}
}

// TestMarchEncounter_IdempotentOnReplay pins G2: the SAME scheduled event
// (same event.ID) processed twice — a crash/timeout replay, not a new tick —
// must not start a second battle at the same hex.
func TestMarchEncounter_IdempotentOnReplay(t *testing.T) {
	pool := testPool(t)
	f := newMarchEncounterFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	unitA := mkCrossingUnit(t, pool, f.worldID, f.ownerA, clk, 0, 2, 1000, nil)
	unitB := mkCrossingUnit(t, pool, f.worldID, f.ownerB, clk, 2, 0, 500, nil)

	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)
	ev := events.ScheduledEvent{ID: 42, WorldID: f.worldID, DueTick: 0}
	if err := h.Handle(ctx, ev); err != nil {
		t.Fatalf("march encounter handle (1st): %v", err)
	}
	if err := h.Handle(ctx, ev); err != nil {
		t.Fatalf("march encounter handle (2nd, replay): %v", err)
	}

	var battleCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM battles WHERE world_id = $1 AND q = 1 AND r = 0`, f.worldID,
	).Scan(&battleCount); err != nil {
		t.Fatalf("count battles: %v", err)
	}
	if battleCount != 1 {
		t.Errorf("battles at (1,0) = %d, want 1 — replaying the same event must not start a second battle", battleCount)
	}

	_ = unitA
	_ = unitB
}
