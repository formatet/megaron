package combat

// KR2 plan §5's cost proof: "mät kostnaden: logga körtiden för handlern på
// en värld med många marscher." A single measured run, not a b.N
// micro-benchmark loop — Handle() is side-effecting (it halts units and
// writes battles/battle_participants rows), so repeating it would measure a
// mostly-empty replay, not the cold scan the plan asks about. "Mät först,
// optimera inte i förväg" (megaron_arbetssatt.md §6): this only reports a
// number, it makes no pass/fail assertion about it — a fixed latency
// threshold in CI would be exactly the premature optimization the plan
// explicitly says NOT to do here.
//
// Deterministic (fixed PRNG seed) so the number is reproducible run to run —
// committed here, not a scratch script, per megaron_arbetssatt.md §4.

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestMarchEncounterHandler_PerfManyMarches(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	const gridSize = 40 // 1,600 land tiles — enough for FindPath to do real A* work, not a straight line
	const numOwners = 20
	const numUnits = 500

	// Bulk-insert the tile grid — one round trip, not gridSize² individual
	// INSERTs (this fixture only cares about seeding fast, not about
	// realistic terrain generation).
	qs := make([]int, 0, gridSize*gridSize)
	rs := make([]int, 0, gridSize*gridSize)
	for q := 0; q < gridSize; q++ {
		for r := 0; r < gridSize; r++ {
			qs = append(qs, q)
			rs = append(rs, r)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain)
		 SELECT $1, t.q, t.r, 'plains' FROM unnest($2::int[], $3::int[]) AS t(q, r)`,
		worldID, qs, rs,
	); err != nil {
		t.Fatalf("seed tile grid: %v", err)
	}

	owners := make([]uuid.UUID, numOwners)
	usernames := make([]string, numOwners)
	for i := range owners {
		owners[i] = uuid.New()
		usernames[i] = fmt.Sprintf("perf-%d-%s", i, owners[i])
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO players (id, username, password_hash)
		 SELECT t.id, t.u, 'x' FROM unnest($1::uuid[], $2::text[]) AS t(id, u)`,
		owners, usernames,
	); err != nil {
		t.Fatalf("seed owners: %v", err)
	}

	// numUnits marching units, scattered origin→target pairs across the grid
	// (deterministic PRNG, fixed seed) split across numOwners distinct owners
	// — dense enough on a 1,600-tile grid that some genuinely collide, so
	// this run also exercises the battle-creation path, not just the scan.
	rng := rand.New(rand.NewSource(42))
	unitOwners := make([]uuid.UUID, numUnits)
	oq := make([]int, numUnits)
	orr := make([]int, numUnits)
	tq := make([]int, numUnits)
	trr := make([]int, numUnits)
	for i := 0; i < numUnits; i++ {
		unitOwners[i] = owners[i%numOwners]
		oq[i] = rng.Intn(gridSize)
		orr[i] = rng.Intn(gridSize)
		tq[i] = rng.Intn(gridSize)
		trr[i] = rng.Intn(gridSize)
	}

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	departsAt := clk.Now().Add(-1 * time.Hour)
	arrivesAt := clk.Now().Add(1 * time.Hour)
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, departs_at, arrives_at, depart_tick)
		 SELECT $1, t.owner, 'spearman', 'land', 100, 0, 'marching', t.oq, t.orr, t.tq, t.trr, $2, $3, 1
		 FROM unnest($4::uuid[], $5::int[], $6::int[], $7::int[], $8::int[]) AS t(owner, oq, orr, tq, trr)`,
		worldID, departsAt, arrivesAt, unitOwners, oq, orr, tq, trr,
	); err != nil {
		t.Fatalf("seed marching units: %v", err)
	}

	h := NewMarchEncounterHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk, nil)

	start := time.Now()
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 1, WorldID: worldID, DueTick: 0}); err != nil {
		t.Fatalf("march encounter handle: %v", err)
	}
	elapsed := time.Since(start)

	var battleCount int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM battles WHERE world_id = $1`, worldID).Scan(&battleCount)

	t.Logf("MarchEncounterHandler.Handle: %d marching units, %d owners, %dx%d tile grid (%d tiles), %d battles created, elapsed=%s",
		numUnits, numOwners, gridSize, gridSize, gridSize*gridSize, battleCount, elapsed)
}
