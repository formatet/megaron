package combat

// R2/R3 (megaron_plan_ordna_passage.md, slice 3b-3): the "passage" march
// intent's own dispatch/arrival mechanics, proven directly against StartMarch
// and the real arrival handler (runFieldArrival — the exact code path a
// ScheduledUnitArrival firing would take), independent of the HTTP surface
// and the messenger sea-lift boarding mechanic (both proven end-to-end in
// api/handlers/messenger_passage_arrange_test.go). Fixture: a coastal home
// port at (0,0), a sea lane out to (5,0), and a coastal foreign settlement at
// (6,0) — close enough that the disembark hex is the settlement's own hex
// (R2's common case).

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupPassageMission builds the fixture above. Returns the ship (galley,
// garrisoned at home) and the foreign settlement's own hex.
func setupPassageMission(t *testing.T) (pool *pgxpool.Pool, worldID, ownerID, shipID uuid.UUID, foreignQ, foreignR int, homeID uuid.UUID) {
	t.Helper()
	pool = testPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var wID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-passage-"+uuid.New().String(),
	).Scan(&wID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, wID) })

	var oID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"passage-captain-"+uuid.New().String(),
	).Scan(&oID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	var foreignOwnerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"passage-foreign-"+uuid.New().String(),
	).Scan(&foreignOwnerID); err != nil {
		t.Fatalf("create foreign player: %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`, wID); err != nil {
		t.Fatalf("insert home tile: %v", err)
	}
	for q := 1; q <= 5; q++ {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'coastal_sea')`, wID, q); err != nil {
			t.Fatalf("insert sea tile (%d,0): %v", q, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 6, 0, 'plains')`, wID); err != nil {
		t.Fatalf("insert foreign tile: %v", err)
	}

	var homeProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		wID,
	).Scan(&homeProvinceID); err != nil {
		t.Fatalf("create home province: %v", err)
	}
	var hID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true) RETURNING id`,
		wID, homeProvinceID, oID,
	).Scan(&hID); err != nil {
		t.Fatalf("create home settlement: %v", err)
	}

	var foreignProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 6, 0, 'plains', true) RETURNING id`,
		wID,
	).Scan(&foreignProvinceID); err != nil {
		t.Fatalf("create foreign province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Foreign', 'achaean', $3, 'capital', true)`,
		wID, foreignProvinceID, foreignOwnerID,
	); err != nil {
		t.Fatalf("create foreign settlement: %v", err)
	}

	var sID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3) RETURNING id`,
		wID, oID, hID,
	).Scan(&sID); err != nil {
		t.Fatalf("create garrisoned ship: %v", err)
	}
	return pool, wID, oID, sID, 6, 0, hID
}

// insertWaitingRunner is a bare messenger row this file's tests point
// passage_messenger_id at — shaped just enough for passageArrived's own
// kind/status read; none of the sea-lift boarding columns are exercised here
// (that mechanic is proven in internal/messenger/passage_test.go and the
// api/handlers end-to-end tests).
func insertWaitingRunner(t *testing.T, pool *pgxpool.Pool, worldID, senderID, originID, destID uuid.UUID, kind, status string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, kind, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'hello',$5,$6,6,0,now()) RETURNING id`,
		worldID, senderID, originID, destID, status, kind,
	).Scan(&id); err != nil {
		t.Fatalf("insert waiting runner: %v", err)
	}
	return id
}

func TestStartMarch_PassageIntent_DispatchesShipToTargetsOwnHex(t *testing.T) {
	pool, worldID, ownerID, shipID, fq, fr, homeID := setupPassageMission(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	var foreignID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id = $1 AND owner_id != $2`, worldID, ownerID).Scan(&foreignID); err != nil {
		t.Fatalf("load foreign settlement: %v", err)
	}
	msgID := insertWaitingRunner(t, pool, worldID, ownerID, homeID, foreignID, "diplomatic", "outbound")
	res, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: fq, TargetR: fr, Intent: "passage", PassageMessengerID: &msgID,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(passage) failed: %v", err)
	}
	// The ship's real sailing target is the sea hex next to (6,0), i.e. (5,0) —
	// never the settled hex itself.
	if res.TargetQ != 5 || res.TargetR != 0 {
		t.Errorf("ship sailing target = (%d,%d), want (5,0) — the sea hex next to the disembark hex", res.TargetQ, res.TargetR)
	}

	var landTargetQ, landTargetR *int
	var passageMessengerID *uuid.UUID
	var marchIntent *string
	if err := pool.QueryRow(ctx,
		`SELECT land_target_q, land_target_r, passage_messenger_id, march_intent FROM units WHERE id = $1`, shipID,
	).Scan(&landTargetQ, &landTargetR, &passageMessengerID, &marchIntent); err != nil {
		t.Fatalf("load ship: %v", err)
	}
	if landTargetQ == nil || *landTargetQ != fq || landTargetR == nil || *landTargetR != fr {
		t.Errorf("land_target = (%v,%v), want (%d,%d) — reused for boardShipMissions", landTargetQ, landTargetR, fq, fr)
	}
	if passageMessengerID == nil || *passageMessengerID != msgID {
		t.Errorf("passage_messenger_id = %v, want %v", passageMessengerID, msgID)
	}
	if marchIntent == nil || *marchIntent != "passage" {
		t.Errorf("march_intent = %v, want \"passage\"", marchIntent)
	}
}

func TestStartMarch_PassageIntent_RejectsWarGalley(t *testing.T) {
	pool, worldID, ownerID, shipID, fq, fr, _ := setupPassageMission(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE units SET type = 'war_galley' WHERE id = $1`, shipID); err != nil {
		t.Fatalf("retype ship: %v", err)
	}
	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	msgID := uuid.New()

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: fq, TargetR: fr, Intent: "passage", PassageMessengerID: &msgID,
	}, nil)
	if err == nil {
		t.Fatal("StartMarch(passage) on a war galley succeeded, want rejected")
	}
	rej, ok := err.(*OrderReject)
	if !ok || rej.Status != 422 {
		t.Errorf("err = %v, want a 422 OrderReject", err)
	}
}

// TestPassageArrived_WaitingRunner_ShipParksPassageWait is R3's first branch:
// the runner this ship was arranged for still needs a ride home (kind !=
// order, status != arrived) — the ship holds rather than sailing straight
// back.
func TestPassageArrived_WaitingRunner_ShipParksPassageWait(t *testing.T) {
	pool, worldID, ownerID, shipID, fq, fr, homeID := setupPassageMission(t)
	ctx := context.Background()

	var foreignID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id = $1 AND owner_id != $2`, worldID, ownerID).Scan(&foreignID); err != nil {
		t.Fatalf("load foreign settlement: %v", err)
	}
	msgID := insertWaitingRunner(t, pool, worldID, ownerID, homeID, foreignID, "diplomatic", "outbound")

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: fq, TargetR: fr, Intent: "passage", PassageMessengerID: &msgID,
	}, nil); err != nil {
		t.Fatalf("StartMarch(passage): %v", err)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID)

	var status string
	var marchIntent *string
	if err := pool.QueryRow(ctx, `SELECT status, march_intent FROM units WHERE id = $1`, shipID).Scan(&status, &marchIntent); err != nil {
		t.Fatalf("load ship after arrival: %v", err)
	}
	if status != "positioned" {
		t.Errorf("status = %q, want \"positioned\" (waiting)", status)
	}
	if marchIntent == nil || *marchIntent != "passage_wait" {
		t.Errorf("march_intent = %v, want \"passage_wait\"", marchIntent)
	}
}

// TestPassageArrived_OrderRunner_ShipGoesHomeImmediately is R3's second
// branch: an order never needs a ride home, so the ship turns around the
// instant it lands, no waiting.
func TestPassageArrived_OrderRunner_ShipGoesHomeImmediately(t *testing.T) {
	pool, worldID, ownerID, shipID, fq, fr, homeID := setupPassageMission(t)
	ctx := context.Background()

	var foreignID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id = $1 AND owner_id != $2`, worldID, ownerID).Scan(&foreignID); err != nil {
		t.Fatalf("load foreign settlement: %v", err)
	}
	msgID := insertWaitingRunner(t, pool, worldID, ownerID, homeID, foreignID, "order", "outbound")

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: fq, TargetR: fr, Intent: "passage", PassageMessengerID: &msgID,
	}, nil); err != nil {
		t.Fatalf("StartMarch(passage): %v", err)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID)

	var status string
	var marchIntent *string
	var landTargetQ *int
	var passageMessengerID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, march_intent, land_target_q, passage_messenger_id FROM units WHERE id = $1`, shipID,
	).Scan(&status, &marchIntent, &landTargetQ, &passageMessengerID); err != nil {
		t.Fatalf("load ship after arrival: %v", err)
	}
	if status != "marching" {
		t.Errorf("status = %q, want \"marching\" (turning for home right away)", status)
	}
	if marchIntent == nil || *marchIntent != "explore_return" {
		t.Errorf("march_intent = %v, want \"explore_return\"", marchIntent)
	}
	if landTargetQ != nil {
		t.Errorf("land_target_q = %v, want nil — cleared on the return leg (megaron_plan_ordna_passage.md 3b-3 R4)", *landTargetQ)
	}
	if passageMessengerID != nil {
		t.Errorf("passage_messenger_id = %v, want nil — cleared on the return leg", *passageMessengerID)
	}
}

// TestPassageArrived_RunnerAlreadyArrived_ShipGoesHomeImmediately is R3's
// "or gone" case: the runner finished (or vanished) before the ship even
// landed — nothing left to wait for.
func TestPassageArrived_RunnerAlreadyArrived_ShipGoesHomeImmediately(t *testing.T) {
	pool, worldID, ownerID, shipID, fq, fr, homeID := setupPassageMission(t)
	ctx := context.Background()

	var foreignID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id = $1 AND owner_id != $2`, worldID, ownerID).Scan(&foreignID); err != nil {
		t.Fatalf("load foreign settlement: %v", err)
	}
	msgID := insertWaitingRunner(t, pool, worldID, ownerID, homeID, foreignID, "diplomatic", "arrived")

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: fq, TargetR: fr, Intent: "passage", PassageMessengerID: &msgID,
	}, nil); err != nil {
		t.Fatalf("StartMarch(passage): %v", err)
	}

	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID)

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, shipID).Scan(&status); err != nil {
		t.Fatalf("load ship after arrival: %v", err)
	}
	if status != "marching" {
		t.Errorf("status = %q, want \"marching\" (nothing left to wait for)", status)
	}
}

// TestReleasePassageWaitShip_ClearsStaleMissionColumns proves the combat-side
// half of R4: releasing a passage_wait ship starts its return leg and clears
// land_target_q/r + passage_messenger_id, which would otherwise misdirect
// boardShipMissions on the return leg (see dispatchReturnHome's own comment).
func TestReleasePassageWaitShip_ClearsStaleMissionColumns(t *testing.T) {
	pool, worldID, ownerID, shipID, fq, fr, homeID := setupPassageMission(t)
	ctx := context.Background()

	var foreignID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id = $1 AND owner_id != $2`, worldID, ownerID).Scan(&foreignID); err != nil {
		t.Fatalf("load foreign settlement: %v", err)
	}
	msgID := insertWaitingRunner(t, pool, worldID, ownerID, homeID, foreignID, "diplomatic", "delivered")

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
		TargetQ: fq, TargetR: fr, Intent: "passage", PassageMessengerID: &msgID,
	}, nil); err != nil {
		t.Fatalf("StartMarch(passage): %v", err)
	}
	markUnitArrivalProcessed(t, pool, worldID, shipID)
	h := newArrivalHandler(pool, nil)
	runFieldArrival(t, pool, h, worldID, shipID) // parks passage_wait (status='delivered' still needs a ride)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.ReleasePassageWaitShip(ctx, tx, shipID, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("ReleasePassageWaitShip: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var status string
	var marchIntent *string
	var landTargetQ, landTargetR *int
	var passageMessengerID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, march_intent, land_target_q, land_target_r, passage_messenger_id FROM units WHERE id = $1`, shipID,
	).Scan(&status, &marchIntent, &landTargetQ, &landTargetR, &passageMessengerID); err != nil {
		t.Fatalf("load ship after release: %v", err)
	}
	if status != "marching" {
		t.Errorf("status = %q, want \"marching\"", status)
	}
	if marchIntent == nil || *marchIntent != "explore_return" {
		t.Errorf("march_intent = %v, want \"explore_return\"", marchIntent)
	}
	if landTargetQ != nil || landTargetR != nil {
		t.Errorf("land_target = (%v,%v), want (nil,nil) — cleared so the return leg's own disembark resolves to home", landTargetQ, landTargetR)
	}
	if passageMessengerID != nil {
		t.Errorf("passage_messenger_id = %v, want nil", *passageMessengerID)
	}

	// Idempotent: a second call on the now-'marching' ship is a no-op.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.ReleasePassageWaitShip(ctx, tx2, shipID, worldID); err != nil {
		tx2.Rollback(ctx)
		t.Fatalf("2nd ReleasePassageWaitShip: %v", err)
	}
	tx2.Commit(ctx)
}
