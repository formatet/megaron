package combat

// Embarkerad ranson (Timothy 2026-08-05, follow-up to soldatens föda/silver-
// halveringen): a land unit embarked on a ship (status 'embarked') is a
// soldier away from the city's stores — maximally away, since the ship IS
// the away-ness. It must pay the same field ration as marching/positioned
// (double grain), and it must be billed at all: before this slice, 'embarked'
// was absent from Handle's status filter entirely, so an embarked cohort paid
// NOTHING — neither grain nor silver — for as long as it stood on a ship.
// Before slice A that was a rounding error (5 grain/day). After slice A's
// ×10 land-grain recalibration it is 100 grain/day quietly waived — "put the
// army on a boat" had become a way to stop feeding it.
//
// DB integration test (real Postgres, gated by DATABASE_URL, same rig as
// silver_audit_test.go / upkeep_circulation_test.go).

import (
	"context"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestUpkeepHandle_EmbarkedCohortIsBilledFieldRation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 4000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-embark-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-embark-"+uuid.New().String(), tick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"embark-"+uuid.New().String(), "embark-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}
	var prov, sid uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`, worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Embarktown', 'achaean', $3, 'capital', true, 'active', 1000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&sid); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	for _, g := range []struct {
		key         string
		amount, cap float64
	}{{"silver", 10000, 100000}, {"grain", 10000, 100000}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1, $2, $3, 0, $4, $5)`,
			sid, g.key, g.amount, g.cap, tick,
		); err != nil {
			t.Fatalf("seed good %s: %v", g.key, err)
		}
	}

	// A 100-man spearman cohort, embarked (no q/r, no settlement_id — it moves
	// with its ship), still paid by the town that raised it.
	var cohortID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, support_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'embarked', $3) RETURNING id`,
		worldID, owner, sid,
	).Scan(&cohortID); err != nil {
		t.Fatalf("create embarked cohort: %v", err)
	}

	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), nil)
	h.soldShare = 0 // isolate the gross debit; circulation is not this test's concern
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	var grain, silver float64
	if err := pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, sid,
	).Scan(&grain); err != nil {
		t.Fatalf("read grain: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, sid,
	).Scan(&silver); err != nil {
		t.Fatalf("read silver: %v", err)
	}

	// spearman size 100, embarked = field ration: grain 0.5 base ×2 = 1.0/day
	// (100 → 1.0, mig 136, UpkeepSpecs grain ÷100 — see upkeep.go's comment).
	// Silver never doubles with status: 1/day (SLICE B halved table, untouched).
	wantGrain, wantSilver := 10000.0-1.0, 10000.0-1
	if math.Abs(grain-wantGrain) > 1e-9 {
		t.Errorf("settlement grain after upkeep tick = %v, want %v (10000 − 100 field-ration grain for the embarked cohort)", grain, wantGrain)
	}
	if math.Abs(silver-wantSilver) > 1e-9 {
		t.Errorf("settlement silver after upkeep tick = %v, want %v (10000 − 1 silver for the embarked cohort)", silver, wantSilver)
	}
}

// TestUnitUpkeep_EmbarkedDoublesGrainLikeMarching pins the pure-function
// contract UnitUpkeep must uphold once Handle's status filter includes
// 'embarked': the field-ration factor applies to it exactly like
// marching/positioned, and silver never changes with status.
func TestUnitUpkeep_EmbarkedDoublesGrainLikeMarching(t *testing.T) {
	up := UnitUpkeep("spearman", "land", 100, "embarked")
	// 100 → 1.0 (mig 136, UpkeepSpecs grain ÷100).
	if up.Grain != 1.0 {
		t.Errorf(`UnitUpkeep("spearman","land",100,"embarked").Grain = %v, want 1.0 (field ration, same as marching/positioned)`, up.Grain)
	}
	if up.Silver != 1 {
		t.Errorf(`UnitUpkeep("spearman","land",100,"embarked").Silver = %v, want 1 (status never changes silver)`, up.Silver)
	}
	// Naval never doubles regardless of status, including embarked (the ship
	// itself, not its cargo — included for completeness of the status contract).
	// 4 → 0.04 (mig 136, UpkeepSpecs grain ÷100).
	upShip := UnitUpkeep("galley", "naval", 1, "embarked")
	if upShip.Grain != 0.04 {
		t.Errorf(`UnitUpkeep("galley","naval",1,"embarked").Grain = %v, want 0.04 (naval never doubles)`, upShip.Grain)
	}
}
