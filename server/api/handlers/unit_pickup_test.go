package handlers

// End-to-end DB integration tests for megaron_plan_hamta_hem.md, slice 2b
// ("hämta hem en ensam enhet" — mission "pickup"). Every acceptance scenario
// is driven through the REAL flow: the HTTP endpoint (Pickup) →
// combat.StartMarch → the scheduled arrival events, fired through the SAME
// handlers the worker would call → internal/combat.UnitArrivalHandler →
// messenger.PassageScanHandler/OrderDeliveryHandler — never a fixture that
// hand-sets an intermediate state. Geography: navalPlayerFixture's own
// coastal home settlement (q=0) plus a sea lane (q=1..4) and a far,
// UNSETTLED landmass (q=5..7) — deliberately bare shore, not a foreign city,
// since pickup fetches a field unit, never a settlement.
//
// §6 acceptance covered here:
//  1. (R0 covered separately in messenger_passage_arrange_test.go.)
//  2. The fetched unit already stands on the shore: no runner, straight
//     board-and-sail-home.
//  3. The fetched unit stands inland: a runner is dispatched, rides the same
//     ship, delivers the march order, and the unit boards on arrival — both
//     orderings (unit reaches the shore before the ship, R3; the ship
//     before the unit, R4).
//  4. The unit never reaches the shore in time: the timeout sends the ship
//     home empty.
//  5. Rejections, and R7's lifecycle sweep: SweepShipsAtSeaOnDeploy leaves a
//     pickup_wait ship alone; the fetched unit disbanding while the ship
//     waits still sends it home empty; the ship being lost while it waits
//     leaves the timeout a no-op.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// listUnits GETs /worlds/{worldID}/units and returns the decoded response.
func (f *navalPlayerFixture) listUnits(t *testing.T, token string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListUnits status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode ListUnits response: %v", err)
	}
	return resp
}

// findUnitSummary locates one unit's summary map in a ListUnits response.
func findUnitSummary(t *testing.T, resp map[string]any, id uuid.UUID) map[string]any {
	t.Helper()
	units, _ := resp["units"].([]any)
	for _, raw := range units {
		u, _ := raw.(map[string]any)
		if u["id"] == id.String() {
			return u
		}
	}
	t.Fatalf("unit %s not found in ListUnits response", id)
	return nil
}

// pickupUnitRow is the subset of units columns these tests read back.
type pickupUnitRow struct {
	status       string
	marchIntent  *string
	settlementID *uuid.UUID
	q, r         *int
	cargoUnitID  *uuid.UUID
}

func (f *navalPlayerFixture) unitRow(t *testing.T, id uuid.UUID) pickupUnitRow {
	t.Helper()
	var row pickupUnitRow
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, march_intent, settlement_id, q, r, cargo_unit_id FROM units WHERE id = $1`, id,
	).Scan(&row.status, &row.marchIntent, &row.settlementID, &row.q, &row.r, &row.cargoUnitID); err != nil {
		t.Fatalf("load unit %s: %v", id, err)
	}
	return row
}

// landUnit creates a field-positioned (no settlement) land unit — exactly
// the state a pickup mission targets.
func (f *navalPlayerFixture) landUnit(t *testing.T, q, r int, ownerID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'infantry', 'land', 100, 'positioned', $3, $4) RETURNING id`,
		f.worldID, ownerID, q, r,
	).Scan(&id); err != nil {
		t.Fatalf("create land unit at (%d,%d): %v", q, r, err)
	}
	return id
}

// pickup POSTs the Pickup endpoint.
func (f *navalPlayerFixture) pickup(t *testing.T, token string, fetchUnitID, shipID uuid.UUID, waitTicks *int) (int, map[string]any) {
	t.Helper()
	body := map[string]any{"ship_id": shipID.String()}
	if waitTicks != nil {
		body["wait_ticks"] = *waitTicks
	}
	return f.post(t, token,
		"/worlds/"+f.worldID.String()+"/units/"+fetchUnitID.String()+"/pickup", body)
}

// setupPickupGeography seeds a home port (q=0, the initiator's, coastal), a
// sea lane (q=1..4), and a bare, UNSETTLED landmass (q=5..7) — never a
// foreign city, since pickup's target is always a field unit. Returns the
// home settlement and a merchantman docked there.
func setupPickupGeography(t *testing.T, f *passageArrangeFixture) (homeID, shipID uuid.UUID) {
	homeID = f.settlement(t, "Pickup-Home-"+uuid.New().String(), 0, f.initiatorID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	f.mapTile(t, 5, 0, "plains")
	f.mapTile(t, 6, 0, "plains")
	f.mapTile(t, 7, 0, "plains")
	shipID = f.ship(t, homeID, f.initiatorID, "merchantman")
	return homeID, shipID
}

func (f *passageArrangeFixture) runPickupTimeout(t *testing.T, atTick int, shipID uuid.UUID) {
	t.Helper()
	f.setTick(t, atTick)
	ev := f.loadPendingEventBy(t, string(events.ScheduledPickupTimeout), "unit_id", shipID)
	if err := f.unitArrivalH.HandlePickupTimeout(context.Background(), ev); err != nil {
		t.Fatalf("pickup timeout: %v", err)
	}
	f.markProcessed(t, ev.ID)
}

// ---------------------------------------------------------------------------
// Acceptance 2: the fetched unit already stands on the shore — no runner.
// ---------------------------------------------------------------------------

func TestPickup_UnitOnShore_NoRunnerNeeded(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 5, 0, f.initiatorID)

	code, resp := f.pickup(t, f.initiatorToken, fetchedID, shipID, nil)
	if code != 202 {
		t.Fatalf("Pickup status = %d, want 202 (%v)", code, resp)
	}
	if resp["messenger_id"] != nil {
		t.Errorf("messenger_id = %v, want nil (no runner needed)", resp["messenger_id"])
	}
	if v, ok := resp["estimated_ticks_to_shore"].(float64); !ok || v != 0 {
		t.Errorf("estimated_ticks_to_shore = %v, want 0", resp["estimated_ticks_to_shore"])
	}
	shoreQ, shoreR := int(resp["shore_q"].(float64)), int(resp["shore_r"].(float64))
	if shoreQ != 5 || shoreR != 0 {
		t.Errorf("shore = (%d,%d), want (5,0) — the unit's own hex", shoreQ, shoreR)
	}
	arriveTick := int(resp["arrival_tick"].(float64))

	// Ship reaches the shore: the unit boards immediately (R3's own branch).
	f.runUnitArrival(t, arriveTick, shipID)

	fetched := f.unitRow(t, fetchedID)
	if fetched.status != "embarked" {
		t.Fatalf("fetched unit status = %q, want \"embarked\"", fetched.status)
	}
	ship := f.unitRow(t, shipID)
	if ship.cargoUnitID == nil || *ship.cargoUnitID != fetchedID {
		t.Fatalf("ship cargo_unit_id = %v, want %v", ship.cargoUnitID, fetchedID)
	}
	if ship.marchIntent == nil || *ship.marchIntent != "pickup_return" {
		t.Fatalf("ship march_intent = %v, want \"pickup_return\"", ship.marchIntent)
	}

	var homeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&homeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, homeTick, shipID)

	finalShip := f.unitRow(t, shipID)
	if finalShip.status != "garrison" || finalShip.cargoUnitID != nil {
		t.Errorf("final ship = (%s, cargo=%v), want (\"garrison\", nil)", finalShip.status, finalShip.cargoUnitID)
	}
	finalFetched := f.unitRow(t, fetchedID)
	if finalFetched.status != "garrison" || finalFetched.settlementID == nil {
		t.Errorf("final fetched unit = (%s, %v), want (\"garrison\", a settlement)", finalFetched.status, finalFetched.settlementID)
	}
}

// ---------------------------------------------------------------------------
// Acceptance 3: the fetched unit stands inland — a runner is dispatched.
// Both orderings of who reaches the shore first.
// ---------------------------------------------------------------------------

// dispatchPickupWithRunner POSTs Pickup for a unit standing inland (q=7,r=0)
// and returns the ship's own arrival tick and the runner's messenger id.
func dispatchPickupWithRunner(t *testing.T, f *passageArrangeFixture, fetchedID, shipID uuid.UUID) (shipArriveTick int, messengerID uuid.UUID) {
	t.Helper()
	code, resp := f.pickup(t, f.initiatorToken, fetchedID, shipID, nil)
	if code != 202 {
		t.Fatalf("Pickup status = %d, want 202 (%v)", code, resp)
	}
	msgIDStr, ok := resp["messenger_id"].(string)
	if !ok || msgIDStr == "" {
		t.Fatalf("messenger_id missing, want a runner (unit stands inland): %v", resp)
	}
	messengerID, err := uuid.Parse(msgIDStr)
	if err != nil {
		t.Fatalf("parse messenger_id: %v", err)
	}
	if est, ok := resp["estimated_ticks_to_shore"].(float64); !ok || est <= 0 {
		t.Errorf("estimated_ticks_to_shore = %v, want > 0 (a runner is needed)", resp["estimated_ticks_to_shore"])
	}
	shoreQ, shoreR := int(resp["shore_q"].(float64)), int(resp["shore_r"].(float64))
	if shoreQ != 5 || shoreR != 0 {
		t.Fatalf("shore = (%d,%d), want (5,0) — the nearest open shore to (7,0)", shoreQ, shoreR)
	}
	shipArriveTick = int(resp["arrival_tick"].(float64))

	// R0/R2.3: the runner boards the ship AT DISPATCH — no scan needed.
	msg := f.messengerRow(t, messengerID)
	if msg.passageStatus == nil || *msg.passageStatus != "aboard" {
		t.Fatalf("runner passage_status = %v immediately after Pickup, want \"aboard\"", msg.passageStatus)
	}
	return shipArriveTick, messengerID
}

func TestPickup_UnitInland_ShipArrivesFirst_BoardsViaR4(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	shipArriveTick, messengerID := dispatchPickupWithRunner(t, f, fetchedID, shipID)

	// The ship reaches the shore first: no one there yet — it waits.
	f.runUnitArrival(t, shipArriveTick, shipID)
	waiting := f.unitRow(t, shipID)
	if waiting.status != "positioned" || waiting.marchIntent == nil || *waiting.marchIntent != "pickup_wait" {
		t.Fatalf("ship after arrival = (%s, %v), want (\"positioned\", \"pickup_wait\")", waiting.status, waiting.marchIntent)
	}

	// The runner disembarks and delivers the march order to the unit.
	var deliveryTick int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT due_tick FROM scheduled_events WHERE event_type = $1 AND (payload->>'messenger_id') = $2 AND processed_at IS NULL`,
		string(events.ScheduledOrderDelivery), messengerID,
	).Scan(&deliveryTick); err != nil {
		t.Fatalf("load order delivery due tick: %v", err)
	}
	f.runOrderDelivery(t, deliveryTick, messengerID)

	marching := f.unitRow(t, fetchedID)
	if marching.status != "marching" {
		t.Fatalf("fetched unit status after order delivery = %q, want \"marching\"", marching.status)
	}

	var unitArriveTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, fetchedID).Scan(&unitArriveTick); err != nil {
		t.Fatalf("load fetched unit's own arrival tick: %v", err)
	}
	// The unit reaches the shore AFTER the ship: R4's own hook boards it.
	f.runUnitArrival(t, unitArriveTick, fetchedID)

	fetched := f.unitRow(t, fetchedID)
	if fetched.status != "embarked" {
		t.Fatalf("fetched unit status = %q, want \"embarked\" (R4 boarding)", fetched.status)
	}
	ship := f.unitRow(t, shipID)
	if ship.cargoUnitID == nil || *ship.cargoUnitID != fetchedID {
		t.Fatalf("ship cargo_unit_id = %v, want %v", ship.cargoUnitID, fetchedID)
	}
	if ship.status != "marching" || ship.marchIntent == nil || *ship.marchIntent != "pickup_return" {
		t.Fatalf("ship after R4 boarding = (%s, %v), want (\"marching\", \"pickup_return\")", ship.status, ship.marchIntent)
	}

	var homeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&homeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, homeTick, shipID)

	finalShip := f.unitRow(t, shipID)
	finalFetched := f.unitRow(t, fetchedID)
	if finalShip.status != "garrison" || finalShip.cargoUnitID != nil {
		t.Errorf("final ship = (%s, cargo=%v), want (\"garrison\", nil)", finalShip.status, finalShip.cargoUnitID)
	}
	if finalFetched.status != "garrison" {
		t.Errorf("final fetched unit status = %q, want \"garrison\"", finalFetched.status)
	}
}

func TestPickup_UnitInland_UnitArrivesFirst_BoardsViaR3(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	shipArriveTick, messengerID := dispatchPickupWithRunner(t, f, fetchedID, shipID)

	// Drive the runner's delivery and the unit's own march to completion
	// BEFORE the ship's own arrival is processed — the real handlers, in the
	// order this test chooses, to exercise R3's immediate-board branch
	// (as opposed to R4's arriveGarrison hook, covered above).
	var deliveryTick int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT due_tick FROM scheduled_events WHERE event_type = $1 AND (payload->>'messenger_id') = $2 AND processed_at IS NULL`,
		string(events.ScheduledOrderDelivery), messengerID,
	).Scan(&deliveryTick); err != nil {
		t.Fatalf("load order delivery due tick: %v", err)
	}
	f.runOrderDelivery(t, deliveryTick, messengerID)

	var unitArriveTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, fetchedID).Scan(&unitArriveTick); err != nil {
		t.Fatalf("load fetched unit's own arrival tick: %v", err)
	}
	f.runUnitArrival(t, unitArriveTick, fetchedID)

	onShore := f.unitRow(t, fetchedID)
	if onShore.status != "positioned" || onShore.q == nil || *onShore.q != 5 || onShore.r == nil || *onShore.r != 0 {
		t.Fatalf("fetched unit after its own march = (%s, %v,%v), want (\"positioned\", 5,0)", onShore.status, onShore.q, onShore.r)
	}

	// Now the ship arrives: the unit is already there — R3's own branch boards
	// it immediately, no pickup_wait detour.
	f.runUnitArrival(t, shipArriveTick, shipID)

	fetched := f.unitRow(t, fetchedID)
	if fetched.status != "embarked" {
		t.Fatalf("fetched unit status = %q, want \"embarked\" (R3 boarding)", fetched.status)
	}
	ship := f.unitRow(t, shipID)
	if ship.cargoUnitID == nil || *ship.cargoUnitID != fetchedID {
		t.Fatalf("ship cargo_unit_id = %v, want %v", ship.cargoUnitID, fetchedID)
	}
	if ship.status != "marching" || ship.marchIntent == nil || *ship.marchIntent != "pickup_return" {
		t.Fatalf("ship after R3 boarding = (%s, %v), want (\"marching\", \"pickup_return\") — never parked pickup_wait", ship.status, ship.marchIntent)
	}

	var homeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&homeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, homeTick, shipID)

	finalShip := f.unitRow(t, shipID)
	finalFetched := f.unitRow(t, fetchedID)
	if finalShip.status != "garrison" || finalShip.cargoUnitID != nil {
		t.Errorf("final ship = (%s, cargo=%v), want (\"garrison\", nil)", finalShip.status, finalShip.cargoUnitID)
	}
	if finalFetched.status != "garrison" {
		t.Errorf("final fetched unit status = %q, want \"garrison\"", finalFetched.status)
	}
}

// ---------------------------------------------------------------------------
// Acceptance 4: the unit never reaches the shore — the timeout sails home
// empty.
// ---------------------------------------------------------------------------

func TestPickup_Timeout_ShipSailsHomeEmpty(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	waitTicks := 2
	shipArriveTick, _ := dispatchPickupWithRunner(t, f, fetchedID, shipID)
	f.runUnitArrival(t, shipArriveTick, shipID)

	waiting := f.unitRow(t, shipID)
	if waiting.status != "positioned" || waiting.marchIntent == nil || *waiting.marchIntent != "pickup_wait" {
		t.Fatalf("ship after arrival = (%s, %v), want (\"positioned\", \"pickup_wait\")", waiting.status, waiting.marchIntent)
	}

	// Never deliver the order — the unit simply never gets there in time.
	f.runPickupTimeout(t, shipArriveTick+waitTicks, shipID)

	timedOut := f.unitRow(t, shipID)
	if timedOut.status != "marching" || timedOut.marchIntent == nil || *timedOut.marchIntent != "explore_return" {
		t.Fatalf("ship after timeout = (%s, %v), want (\"marching\", \"explore_return\") — empty return", timedOut.status, timedOut.marchIntent)
	}
	if timedOut.cargoUnitID != nil {
		t.Errorf("ship cargo_unit_id = %v, want nil (nothing was ever boarded)", timedOut.cargoUnitID)
	}
	stillInland := f.unitRow(t, fetchedID)
	if stillInland.status != "positioned" || stillInland.q == nil || *stillInland.q != 7 {
		t.Errorf("fetched unit = (%s, q=%v), want still (\"positioned\", 7) — never moved", stillInland.status, stillInland.q)
	}

	var homeTick int
	if err := f.pool.QueryRow(context.Background(), `SELECT arrive_tick FROM units WHERE id = $1`, shipID).Scan(&homeTick); err != nil {
		t.Fatalf("load ship home arrival tick: %v", err)
	}
	f.runUnitArrival(t, homeTick, shipID)
	finalShip := f.unitRow(t, shipID)
	if finalShip.status != "garrison" {
		t.Errorf("final ship status = %q, want \"garrison\"", finalShip.status)
	}
}

// ---------------------------------------------------------------------------
// Acceptance 5: rejections.
// ---------------------------------------------------------------------------

func TestPickup_Rejections(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID, shipID := setupPickupGeography(t, f)

	t.Run("fetched unit is marching", func(t *testing.T) {
		marchingID := f.landUnit(t, 6, 0, f.initiatorID)
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE units SET status = 'marching' WHERE id = $1`, marchingID,
		); err != nil {
			t.Fatalf("force marching: %v", err)
		}
		code, resp := f.pickup(t, f.initiatorToken, marchingID, shipID, nil)
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("fetched unit is in a city", func(t *testing.T) {
		garrisonID := f.landUnit(t, 6, 0, f.initiatorID)
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE units SET status = 'garrison', settlement_id = $2, q = NULL, r = NULL WHERE id = $1`,
			garrisonID, homeID,
		); err != nil {
			t.Fatalf("force garrison: %v", err)
		}
		code, resp := f.pickup(t, f.initiatorToken, garrisonID, shipID, nil)
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("ship already carries cargo", func(t *testing.T) {
		fetchedID := f.landUnit(t, 5, 0, f.initiatorID)
		cargoShip := f.ship(t, homeID, f.initiatorID, "merchantman")
		otherLand := f.landUnit(t, 6, 0, f.initiatorID)
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE units SET cargo_unit_id = $2 WHERE id = $1`, cargoShip, otherLand,
		); err != nil {
			t.Fatalf("force cargo: %v", err)
		}
		code, resp := f.pickup(t, f.initiatorToken, fetchedID, cargoShip, nil)
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("ship not in its own port", func(t *testing.T) {
		fetchedID := f.landUnit(t, 5, 0, f.initiatorID)
		awayShip := f.ship(t, homeID, f.initiatorID, "merchantman")
		if _, err := f.pool.Exec(context.Background(),
			`UPDATE units SET status = 'positioned', settlement_id = NULL, q = 3, r = 0 WHERE id = $1`, awayShip,
		); err != nil {
			t.Fatalf("force at sea: %v", err)
		}
		code, resp := f.pickup(t, f.initiatorToken, fetchedID, awayShip, nil)
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("war galley refused when a runner is needed", func(t *testing.T) {
		inlandID := f.landUnit(t, 7, 0, f.initiatorID)
		galleyID := f.ship(t, homeID, f.initiatorID, "war_galley")
		code, resp := f.pickup(t, f.initiatorToken, inlandID, galleyID, nil)
		if code != 422 {
			t.Errorf("status = %d, want 422 (%v)", code, resp)
		}
	})

	t.Run("war galley allowed when the unit already stands on the shore", func(t *testing.T) {
		onShoreID := f.landUnit(t, 5, 0, f.initiatorID)
		galleyID := f.ship(t, homeID, f.initiatorID, "war_galley")
		code, resp := f.pickup(t, f.initiatorToken, onShoreID, galleyID, nil)
		if code != 202 {
			t.Errorf("status = %d, want 202 (%v)", code, resp)
		}
	})

	t.Run("wait_ticks too low", func(t *testing.T) {
		fetchedID := f.landUnit(t, 5, 0, f.initiatorID)
		wt := 0
		shipID := f.ship(t, homeID, f.initiatorID, "merchantman")
		code, resp := f.pickup(t, f.initiatorToken, fetchedID, shipID, &wt)
		if code != 400 {
			t.Errorf("status = %d, want 400 (%v)", code, resp)
		}
	})

	t.Run("wait_ticks too high", func(t *testing.T) {
		fetchedID := f.landUnit(t, 5, 0, f.initiatorID)
		wt := combat.PickupWaitMaxTicks + 1
		shipID := f.ship(t, homeID, f.initiatorID, "merchantman")
		code, resp := f.pickup(t, f.initiatorToken, fetchedID, shipID, &wt)
		if code != 400 {
			t.Errorf("status = %d, want 400 (%v)", code, resp)
		}
	})

	t.Run("not your unit", func(t *testing.T) {
		theirID := f.landUnit(t, 5, 0, f.counterpartyID)
		shipID := f.ship(t, homeID, f.initiatorID, "merchantman")
		code, resp := f.pickup(t, f.initiatorToken, theirID, shipID, nil)
		if code != 403 {
			t.Errorf("status = %d, want 403 (%v)", code, resp)
		}
	})

	t.Run("not your ship", func(t *testing.T) {
		fetchedID := f.landUnit(t, 5, 0, f.initiatorID)
		theirShip := f.ship(t, f.settlement(t, "Foreign-"+uuid.New().String(), 20, f.counterpartyID, true), f.counterpartyID, "merchantman")
		code, resp := f.pickup(t, f.initiatorToken, fetchedID, theirShip, nil)
		if code != 403 {
			t.Errorf("status = %d, want 403 (%v)", code, resp)
		}
	})
}

// ---------------------------------------------------------------------------
// R7: livscykel — sweep exemption, and the two loss-during-wait cases.
// ---------------------------------------------------------------------------

func TestPickup_SweepShipsAtSeaOnDeploy_LeavesPickupWaitAlone(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	shipArriveTick, _ := dispatchPickupWithRunner(t, f, fetchedID, shipID)
	f.runUnitArrival(t, shipArriveTick, shipID)

	before := f.unitRow(t, shipID)
	if before.status != "positioned" || before.marchIntent == nil || *before.marchIntent != "pickup_wait" {
		t.Fatalf("precondition: ship = (%s, %v), want (\"positioned\", \"pickup_wait\")", before.status, before.marchIntent)
	}

	if _, err := combat.SweepShipsAtSeaOnDeploy(context.Background(), f.pool, f.unitArrivalH); err != nil {
		t.Fatalf("sweep ships at sea: %v", err)
	}

	after := f.unitRow(t, shipID)
	if after.status != "positioned" || after.marchIntent == nil || *after.marchIntent != "pickup_wait" {
		t.Fatalf("ship after sweep = (%s, %v), want UNCHANGED (\"positioned\", \"pickup_wait\") — the timer owns this ship's way home", after.status, after.marchIntent)
	}
}

func TestPickup_UnitDisbandedWhileWaiting_TimeoutSailsHomeEmpty(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	waitTicks := 2
	shipArriveTick, _ := dispatchPickupWithRunner(t, f, fetchedID, shipID)
	f.runUnitArrival(t, shipArriveTick, shipID)

	// The fetched unit is lost (starved, destroyed) while the ship waits.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE units SET status = 'disbanded', q = NULL, r = NULL WHERE id = $1`, fetchedID,
	); err != nil {
		t.Fatalf("disband fetched unit: %v", err)
	}

	f.runPickupTimeout(t, shipArriveTick+waitTicks, shipID)

	timedOut := f.unitRow(t, shipID)
	if timedOut.status != "marching" || timedOut.cargoUnitID != nil {
		t.Fatalf("ship after timeout = (%s, cargo=%v), want (\"marching\", nil) — sailing home empty", timedOut.status, timedOut.cargoUnitID)
	}
}

func TestPickup_ShipLostWhileWaiting_TimeoutIsNoOp(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	_, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	waitTicks := 2
	shipArriveTick, _ := dispatchPickupWithRunner(t, f, fetchedID, shipID)
	f.runUnitArrival(t, shipArriveTick, shipID)

	// The ship is sunk (or captured) while it waits — it leaves 'pickup_wait'
	// by a path other than the timeout.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE units SET status = 'disbanded', q = NULL, r = NULL WHERE id = $1`, shipID,
	); err != nil {
		t.Fatalf("disband ship: %v", err)
	}

	f.runPickupTimeout(t, shipArriveTick+waitTicks, shipID)

	afterShip := f.unitRow(t, shipID)
	if afterShip.status != "disbanded" {
		t.Errorf("ship status = %q, want UNCHANGED \"disbanded\" — the timeout must be a no-op", afterShip.status)
	}
	// The fetched unit is unaffected — still standing where it was.
	fetched := f.unitRow(t, fetchedID)
	if fetched.status != "positioned" || fetched.q == nil || *fetched.q != 7 {
		t.Errorf("fetched unit = (%s, q=%v), want still (\"positioned\", 7)", fetched.status, fetched.q)
	}
}

// ---------------------------------------------------------------------------
// temenos JSON surface: can_fetch_by_ship/pickup_ships on the fetchable unit,
// pickup_for/shore_q,shore_r/waiting_until_tick on the ship.
// ---------------------------------------------------------------------------

func TestPickup_ListUnitsSurface(t *testing.T) {
	f := setupPassageArrangeFixture(t)
	homeID, shipID := setupPickupGeography(t, f)
	fetchedID := f.landUnit(t, 7, 0, f.initiatorID)

	// Before any pickup is arranged: the fetchable unit already advertises
	// the idle ship as a choice.
	before := findUnitSummary(t, f.listUnits(t, f.initiatorToken), fetchedID)
	if canFetch, _ := before["can_fetch_by_ship"].(bool); !canFetch {
		t.Fatalf("can_fetch_by_ship = %v, want true (an idle ship exists)", before["can_fetch_by_ship"])
	}
	ships, _ := before["pickup_ships"].([]any)
	if len(ships) == 0 {
		t.Fatalf("pickup_ships is empty, want the idle ship listed")
	}
	firstShip, _ := ships[0].(map[string]any)
	if firstShip["id"] != shipID.String() {
		t.Errorf("pickup_ships[0].id = %v, want %v", firstShip["id"], shipID)
	}
	if canCarry, _ := firstShip["can_carry_runner"].(bool); !canCarry {
		t.Errorf("can_carry_runner = %v, want true (a merchantman)", firstShip["can_carry_runner"])
	}

	// A garrisoned (non-fetchable) unit must NOT advertise the surface.
	garrisonID := f.landUnit(t, 6, 0, f.initiatorID)
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE units SET status = 'garrison', settlement_id = $2, q = NULL, r = NULL WHERE id = $1`,
		garrisonID, homeID,
	); err != nil {
		t.Fatalf("force garrison: %v", err)
	}
	garrisonSummary := findUnitSummary(t, f.listUnits(t, f.initiatorToken), garrisonID)
	if v, ok := garrisonSummary["can_fetch_by_ship"]; ok && v != nil && v != false {
		t.Errorf("garrisoned unit can_fetch_by_ship = %v, want absent/false", v)
	}

	// After dispatch: the ship names the fetched unit and the shore.
	shipArriveTick, _ := dispatchPickupWithRunner(t, f, fetchedID, shipID)
	dispatched := findUnitSummary(t, f.listUnits(t, f.initiatorToken), shipID)
	if dispatched["pickup_for"] == nil {
		t.Errorf("pickup_for missing on a ship mid-pickup mission")
	}
	if shoreQ, _ := dispatched["shore_q"].(float64); int(shoreQ) != 5 {
		t.Errorf("shore_q = %v, want 5", dispatched["shore_q"])
	}

	// Once waiting: waiting_until_tick appears too.
	f.runUnitArrival(t, shipArriveTick, shipID)
	waiting := findUnitSummary(t, f.listUnits(t, f.initiatorToken), shipID)
	if waiting["waiting_until_tick"] == nil {
		t.Errorf("waiting_until_tick missing on a pickup_wait ship")
	}
}
