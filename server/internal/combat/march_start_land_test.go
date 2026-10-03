package combat

// R1 (megaron_plan_skeppsuppdrag_landsatt.md): mission "land" — a laden ship
// dispatched from port sails to the sea hex next to a chosen land target,
// lands its cargo there, optionally founds a colony on arrival, and always
// turns for home on its own (reusing the explore_return machinery). Two
// acceptance scenarios (plan §5 items 1-2): without a grounding order, and
// with cargo_intent=colonize.

import (
	"context"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupLandMission builds a small geography: a coastal home settlement at
// (0,0), a long sea lane out to (9,0), and an unclaimed land target at
// (10,0) — reachable by sea from the home port and far enough away that its
// catchment (radius hexgrid.CatchmentRadius) does not overlap the home
// settlement's own. Returns the world, owner, ship (garrisoned, cargo
// loaded) and cargo unit ids.
func setupLandMission(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID, shipID, cargoID uuid.UUID) {
	t.Helper()
	pool = testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"landing-captain-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	tiles := []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"},
	}
	for q := 1; q <= 9; q++ {
		tiles = append(tiles, struct {
			q, r    int
			terrain string
		}{q, 0, "coastal_sea"})
	}
	tiles = append(tiles, struct {
		q, r    int
		terrain string
	}{10, 0, "plains"})
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, $4)`,
			worldID, tl.q, tl.r, tl.terrain,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}
	var homeProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		worldID,
	).Scan(&homeProvinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	var homeID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, homeProvinceID, ownerID,
	).Scan(&homeID); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3) RETURNING id`,
		worldID, ownerID, homeID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create garrisoned ship: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'embarked', NULL) RETURNING id`,
		worldID, ownerID,
	).Scan(&cargoID); err != nil {
		t.Fatalf("create embarked cargo unit: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE units SET cargo_unit_id = $1 WHERE id = $2`, cargoID, shipID); err != nil {
		t.Fatalf("load cargo onto ship: %v", err)
	}
	return pool, worldID, ownerID, shipID, cargoID
}

// markUnitArrivalProcessed marks any pending ScheduledUnitArrival row for
// unitID as processed. runFieldArrival calls resolve() directly, bypassing
// the event worker's own claim step, so a still-unprocessed row can collide
// with the NEXT leg's own schedule attempt on idx_scheduled_recurring_dedup
// (mig 127, world_id+event_type+due_tick+payload) — a fixed TestClock never
// advances the tick between two legs of equal length, so outbound and return
// land on the exact same due_tick. A real tick worker marks a row processed
// before calling resolve(); this mirrors that, once per leg.
func markUnitArrivalProcessed(t *testing.T, pool *pgxpool.Pool, worldID, unitID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE scheduled_events SET processed_at = now()
		 WHERE world_id = $1 AND (payload->>'unit_id')::uuid = $2 AND processed_at IS NULL`,
		worldID, unitID,
	); err != nil {
		t.Fatalf("mark unit arrival processed: %v", err)
	}
}

func TestLandMission_WithoutGroundingOrder_CargoLandsShipReturnsHome(t *testing.T) {
	pool, worldID, ownerID, shipID, cargoID := setupLandMission(t)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 10, TargetR: 0, Intent: "land",
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(land, no grounding order) failed: %v", err)
	}
	// The ship's real sailing target is the sea hex next to (3,0), i.e. (2,0) —
	// not the land hex itself (naval units cannot enter land).
	if res.TargetQ != 9 || res.TargetR != 0 {
		t.Errorf("ship sailing target = (%d,%d), want (9,0) — the sea hex next to the land target", res.TargetQ, res.TargetR)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID) // ship reaches (9,0): lands cargo, turns for home

	var cargoStatus string
	var cargoQ, cargoR int
	var cargoSettlement *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, q, r, settlement_id FROM units WHERE id = $1`, cargoID,
	).Scan(&cargoStatus, &cargoQ, &cargoR, &cargoSettlement); err != nil {
		t.Fatalf("load cargo after landing: %v", err)
	}
	if cargoStatus != "positioned" {
		t.Errorf("cargo status = %q, want \"positioned\"", cargoStatus)
	}
	if cargoQ != 10 || cargoR != 0 {
		t.Errorf("cargo position = (%d,%d), want (10,0) — the chosen land target", cargoQ, cargoR)
	}
	if cargoSettlement != nil {
		t.Errorf("cargo settlement_id = %v, want nil", *cargoSettlement)
	}

	var shipStatus string
	var shipCargo *uuid.UUID
	var shipMarchIntent *string
	if err := pool.QueryRow(ctx,
		`SELECT status, cargo_unit_id, march_intent FROM units WHERE id = $1`, shipID,
	).Scan(&shipStatus, &shipCargo, &shipMarchIntent); err != nil {
		t.Fatalf("load ship after landing: %v", err)
	}
	if shipStatus != "marching" {
		t.Errorf("ship status = %q, want \"marching\" (turning for home)", shipStatus)
	}
	if shipCargo != nil {
		t.Errorf("ship cargo_unit_id = %v, want nil (cargo disembarked)", *shipCargo)
	}
	if shipMarchIntent == nil || *shipMarchIntent != "explore_return" {
		t.Errorf("ship march_intent = %v, want \"explore_return\" (reusing the built-in return leg)", shipMarchIntent)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)

	// Second arrival: the ship completes its return leg and re-garrisons at home.
	runFieldArrival(t, pool, h, worldID, shipID)
	var shipStatusAfterHome string
	var shipSettlementID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id FROM units WHERE id = $1`, shipID,
	).Scan(&shipStatusAfterHome, &shipSettlementID); err != nil {
		t.Fatalf("load ship after return: %v", err)
	}
	if shipStatusAfterHome != "garrison" {
		t.Errorf("ship status after return leg = %q, want \"garrison\"", shipStatusAfterHome)
	}
	if shipSettlementID == nil {
		t.Errorf("ship settlement_id after return leg = nil, want the home settlement")
	}
}

func TestLandMission_WithColonizeCargoIntent_ColonyFoundsWithoutFurtherOrder(t *testing.T) {
	pool, worldID, ownerID, shipID, cargoID := setupLandMission(t)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 10, TargetR: 0, Intent: "land", CargoIntent: "colonize", Name: "Newbeach",
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(land, cargo_intent=colonize) failed: %v", err)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID)

	var colonyID uuid.UUID
	var colonyOwner uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id, owner_id FROM settlements WHERE world_id = $1 AND name = 'Newbeach'`, worldID,
	).Scan(&colonyID, &colonyOwner); err != nil {
		t.Fatalf("load founded colony: %v — a land mission with cargo_intent=colonize must found a colony on arrival with no further order", err)
	}
	if colonyOwner != ownerID {
		t.Errorf("colony owner = %v, want %v", colonyOwner, ownerID)
	}

	var cargoStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, cargoID).Scan(&cargoStatus); err != nil {
		t.Fatalf("load cargo after colonize: %v", err)
	}
	if cargoStatus != "disbanded" {
		t.Errorf("cargo status = %q, want \"disbanded\" (folded into the new colony's populace)", cargoStatus)
	}

	var shipStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&shipStatus); err != nil {
		t.Fatalf("load ship after colonize: %v", err)
	}
	if shipStatus != "marching" {
		t.Errorf("ship status = %q, want \"marching\" (turning for home)", shipStatus)
	}
}

// A colony founded from a landing carries its purse exactly like one that
// walked there (B3, mig 107): the port it sails from pays the colonist purse
// at dispatch, the cargo carries it across the sea, and the colony starts
// with it. Until 2026-10-03 the land path never withdrew a purse and the
// cargo row passed to foundColony had no carriedSilver — every colony
// founded from the sea started at 0 silver.
func TestLandMission_WithColonizeCargoIntent_ColonyGetsThePurse(t *testing.T) {
	pool, worldID, ownerID, shipID, cargoID := setupLandMission(t)
	ctx := context.Background()

	var homeID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT settlement_id FROM units WHERE id = $1`, shipID).Scan(&homeID); err != nil {
		t.Fatalf("load home port: %v", err)
	}
	const homeSilver = 1_000_000.0
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'silver', $2, 0, $2, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET amount = $2, rate = 0, cap = $2`,
		homeID, homeSilver,
	); err != nil {
		t.Fatalf("seed home silver: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: 10, TargetR: 0, Intent: "land", CargoIntent: "colonize", Name: "Pursebeach",
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(land, cargo_intent=colonize) failed: %v", err)
	}
	if res.CarriedSilver <= 0 {
		t.Fatalf("CarriedSilver = %v at dispatch, want > 0 — the port must pay the colonist purse when the ship sails", res.CarriedSilver)
	}
	var homeAfter, cargoPurse float64
	_ = pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, homeID).Scan(&homeAfter)
	_ = pool.QueryRow(ctx, `SELECT carried_silver FROM units WHERE id = $1`, cargoID).Scan(&cargoPurse)
	if math.Abs(homeSilver-homeAfter-res.CarriedSilver) > 0.01 || math.Abs(cargoPurse-res.CarriedSilver) > 0.01 {
		t.Fatalf("port debited %v, cargo carries %v, response says %v — all three must agree", homeSilver-homeAfter, cargoPurse, res.CarriedSilver)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID)

	var colonySilver float64
	if err := pool.QueryRow(ctx,
		`SELECT g.amount FROM settlements s JOIN settlement_goods g ON g.settlement_id = s.id AND g.good_key = 'silver'
		 WHERE s.world_id = $1 AND s.name = 'Pursebeach'`, worldID,
	).Scan(&colonySilver); err != nil {
		t.Fatalf("load colony silver: %v", err)
	}
	if math.Abs(colonySilver-res.CarriedSilver) > 0.01 {
		t.Errorf("colony silver = %v, want the carried purse %v", colonySilver, res.CarriedSilver)
	}
	if err := pool.QueryRow(ctx, `SELECT carried_silver FROM units WHERE id = $1`, cargoID).Scan(&cargoPurse); err == nil && cargoPurse != 0 {
		t.Errorf("cargo still carries %v after founding — the purse would exist twice", cargoPurse)
	}
}
