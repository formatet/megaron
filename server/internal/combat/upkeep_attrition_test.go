package combat

// Besättning ska tunnas, inte utplånas (megaron_plan_upkeep_attrition.md).
//
// Before this slice, applyAttrition drained upkeepAttritionStep (a flat 10)
// off units.size, clipped to size. A ship's size is its hull count — always
// 1, pre-Slice-B — so a single missed grain tick killed it outright: 100% of
// a full-crew galley in one bad day, with no chance for its Wanax to respond.
// A thin land remnant (e.g. 12 men) fared almost as badly, losing 83% of its
// strength in the same single tick a fresh 100-man cohort lost only 10%.
//
// DB integration tests (real Postgres, gated by DATABASE_URL), same rig as
// upkeep_support_settlement_test.go.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mkNavalUnit inserts a naval unit garrisoned at sid, paid by sid.
func mkNavalUnit(t *testing.T, pool *pgxpool.Pool, worldID, owner, sid uuid.UUID, unitType string, crew int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
		 VALUES ($1, $2, $3, 'naval', 1, $4, 'garrison', $5, $5) RETURNING id`,
		worldID, owner, unitType, crew, sid,
	).Scan(&id); err != nil {
		t.Fatalf("create naval unit: %v", err)
	}
	return id
}

// 1. RÖD-FÖRE-target: en galär med full besättning ska överleva EN missad
// grain-tick. Skrovet (size) rörs inte alls — förlusten sker i besättningen.
func TestUpkeepAttrition_NavalUnitSurvivesOneMissedGrainTick(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "attrition-naval-survive")

	// Tomt spannmålslager — varje upkeep-tick misslyckas att föda skeppet.
	seedGoods(t, pool, f.townID, f.tick, 0, 100000)

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3, $3) RETURNING id`,
		f.worldID, f.owner, f.townID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create ship: %v", err)
	}

	broadcaster := &fakeBroadcaster{}
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), broadcaster, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 90001, WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	var status string
	var size, crew int
	if err := pool.QueryRow(ctx, `SELECT status, size, crew FROM units WHERE id = $1`, shipID).Scan(&status, &size, &crew); err != nil {
		t.Fatalf("read ship: %v", err)
	}
	if status == "disbanded" {
		t.Fatalf("ship disbanded after ONE missed grain tick (status=%s, size=%d, crew=%d) — "+
			"a single bad tick must never kill a full-crew ship (invariant, see megaron_plan_upkeep_attrition.md)",
			status, size, crew)
	}
	if size != 1 {
		t.Errorf("ship size = %d after a grain-starved tick, want 1 unchanged — naval attrition must drain crew, not the hull count", size)
	}
	if crew >= 20 {
		t.Errorf("ship crew unchanged at %d after a missed grain tick — attrition never touched the crew column", crew)
	}
	if crew <= 0 {
		t.Errorf("ship crew = %d, want >0 — a single tick must not zero the crew of a 20-man galley", crew)
	}

	found := false
	for _, k := range broadcaster.notified {
		if k == "UnitAttrition" {
			found = true
		}
	}
	if !found {
		t.Errorf("owner was not notified of the crew loss (notifyUnitLoss never fired) — an unseen loss breaks the asynchronicity gate")
	}
}

// 2. Utsträckt svält TAR till slut skeppet — invarianten är "aldrig på en
// tick", inte "aldrig". crew=20 med ett litet steg per tick ska ta flera tick.
func TestUpkeepAttrition_NavalUnitEventuallyDisbandsFromSustainedStarvation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "attrition-naval-eventual")
	seedGoods(t, pool, f.townID, f.tick, 0, 100000)

	shipID := mkNavalUnit(t, pool, f.worldID, f.owner, f.townID, "galley", 20)

	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), &fakeBroadcaster{}, nil)

	var status string
	ticks := 0
	for i := int64(1); i <= 30; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{ID: 91000 + i, WorldID: f.worldID, DueTick: f.tick}); err != nil {
			t.Fatalf("upkeep Handle tick %d: %v", i, err)
		}
		ticks++
		if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&status); err != nil {
			t.Fatalf("read ship status: %v", err)
		}
		if status == "disbanded" {
			break
		}
	}
	if status != "disbanded" {
		t.Fatalf("ship still alive after %d consecutive missed grain ticks — sustained starvation should eventually take it", ticks)
	}
	if ticks < 5 {
		t.Errorf("ship died after only %d ticks of sustained starvation — a 20-crew ship must survive several ticks, not just a handful", ticks)
	}
}

// 3. Land: en fräsch 100-mannakohort ska förlora FLER män i absoluta tal än en
// redan decimerad 12-mannakohort på samma tick, och ingendera utplånas.
func TestUpkeepAttrition_LandCohortLossIsProportionalNotFlat(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "attrition-land-prop")
	seedGoods(t, pool, f.townID, f.tick, 0, 100000)

	mkCohort := func(size int) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
			 VALUES ($1, $2, 'spearman', 'land', $3, 0, 'garrison', $4, $4) RETURNING id`,
			f.worldID, f.owner, size, f.townID,
		).Scan(&id); err != nil {
			t.Fatalf("create cohort size %d: %v", size, err)
		}
		return id
	}
	full := mkCohort(100)
	thin := mkCohort(12)

	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), &fakeBroadcaster{}, nil)
	if err := h.Handle(ctx, events.ScheduledEvent{ID: 92001, WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	var fullSize, thinSize int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, full).Scan(&fullSize); err != nil {
		t.Fatalf("read full cohort: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, thin).Scan(&thinSize); err != nil {
		t.Fatalf("read thin cohort: %v", err)
	}
	if fullSize <= 0 {
		t.Errorf("100-man cohort wiped on a single missed grain tick (size=%d)", fullSize)
	}
	if thinSize <= 0 {
		t.Errorf("12-man cohort wiped on a single missed grain tick (size=%d)", thinSize)
	}
	fullLost := 100 - fullSize
	thinLost := 12 - thinSize
	if fullLost <= thinLost {
		t.Errorf("full cohort lost %d men, thin cohort lost %d — attrition must be proportional, not a flat step "+
			"(a decimated remnant should not bleed as many men in absolute terms as a fresh cohort)", fullLost, thinLost)
	}
}
