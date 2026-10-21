package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// Runners board any Wanax's ship (Timothy 2026-10-21, temenos_diplomati.md
// §Runners). The live case: a runner waiting in another Wanax's port for a
// ship home, where none of its sender's own ships ever call.

// foreignWanax makes f.destID belong to a second Wanax and returns them.
func (f *passageFixture) foreignWanax(t *testing.T) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var other uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "foreign-carrier-"+uuid.NewString()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, other); err != nil {
		t.Fatal(err)
	}
	return other
}

func TestPassageScan_RunnerBoardsForeignWanaxTransport(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	other := f.foreignWanax(t)
	runner := f.waitingReturnMessenger(t, f.destID, f.currentTick+1)
	ship := f.ship(t, f.destID, "merchantman")
	if _, err := f.pool.Exec(ctx, `UPDATE units SET owner_id=$2 WHERE id=$1`, ship, other); err != nil {
		t.Fatal(err)
	}
	transportID := f.departingTransportTo(t, f.destID, f.originID, 0, 0, ship, f.currentTick+3)
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET owner_id=$2 WHERE id=$1`, transportID, other); err != nil {
		t.Fatal(err)
	}

	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatal(err)
	}
	_, passage, _, _ := f.messengerRow(t, runner)
	if passage == nil || *passage != "aboard" {
		t.Fatalf("runner waiting in Dunios' port must board Dunios' departing ship: passage_status=%v", passage)
	}
	var carrierOwner *uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT carrier_owner_id FROM messengers WHERE id=$1`, runner).Scan(&carrierOwner); err != nil {
		t.Fatal(err)
	}
	if carrierOwner == nil || *carrierOwner != other {
		t.Errorf("carrier_owner_id = %v, want the ship's owner %s", carrierOwner, other)
	}
	if n := f.countScheduled(t, "MessengerReturn", runner); n != 1 {
		t.Errorf("scheduled MessengerReturn = %d, want 1 (the runner reaches home)", n)
	}
	// The carrier's own order is never touched by its passenger.
	var dest uuid.UUID
	var due int
	if err := f.pool.QueryRow(ctx, `SELECT dest_id,due_tick FROM transports WHERE id=$1`, transportID).Scan(&dest, &due); err != nil {
		t.Fatal(err)
	}
	if dest != f.originID || due != f.currentTick+3 {
		t.Errorf("transport order changed by its passenger: dest=%s due=%d", dest, due)
	}
}

// A foreign ship mission must not read as "captured" just because its owner
// is not the runner's sender; only a change of owner after boarding does.
func TestPassageScan_ForeignShipMissionCarriesUntilCaptured(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	other := f.foreignWanax(t)
	runner := f.waitingMessenger(t, f.originID, f.currentTick+1)
	ship := f.marchingShip(t, 1, 0, 4, 0, f.currentTick, f.currentTick+4)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET owner_id=$2 WHERE id=$1`, ship, other); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
			t.Fatal(err)
		}
	}
	row := f.fullRow(t, runner)
	if row.passageStatus == nil || *row.passageStatus != "aboard" || row.carrierUnitID == nil || *row.carrierUnitID != ship {
		t.Fatalf("runner on Dunios' ship after two scans: passage=%v carrier=%v, want aboard %s", row.passageStatus, row.carrierUnitID, ship)
	}

	var third uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "captor-"+uuid.NewString()).Scan(&third); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET owner_id=$2 WHERE id=$1`, ship, third); err != nil {
		t.Fatal(err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
		t.Fatal(err)
	}
	row = f.fullRow(t, runner)
	if row.passageStatus == nil || *row.passageStatus != "returning_sealed" {
		t.Fatalf("captured foreign carrier must seal its runner like an own ship: passage=%v", row.passageStatus)
	}
}

// A storm sinking a foreign carrier loses the runner exactly as an own ship
// would, and only the SENDER hears of it — never the ship's owner.
func TestCarrierFate_StormForeignMissionLosesPassengerToSenderOnly(t *testing.T) {
	f := setupPassageFixture(t)
	ctx := context.Background()
	other := f.foreignWanax(t)
	ship := f.marchingShip(t, 0, 0, 4, 0, 500, 504)
	route := combat.StoredRoute{StartTick: 500, EndTick: 504, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
	raw, _ := json.Marshal(route)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=1, march_route=$2, owner_id=$3 WHERE id=$1`, ship, raw, other); err != nil {
		t.Fatal(err)
	}
	runner := f.waitingMessenger(t, f.originID, 500)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard', carrier_unit_id=$2, carrier_owner_id=$3 WHERE id=$1`, runner, ship, other); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	storm := combat.NewSeaStormScanHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), nil)
	carrierStormOver(t, f.pool, f.worldID)
	f.setTick(t, 501)
	if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	status, passage, _, _ := f.messengerRow(t, runner)
	if status != "lost" || passage != nil {
		t.Fatalf("storm on a foreign carrier must lose the runner: status=%q passage=%v", status, passage)
	}
	var toSender, toOwner int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE player_id=$2), count(*) FILTER (WHERE player_id=$3) FROM notifications WHERE kind='MessengerLostAtSea' AND body_json->>'messenger_id'=$1`, runner.String(), f.ownerID, other).Scan(&toSender, &toOwner); err != nil {
		t.Fatal(err)
	}
	if toSender != 1 || toOwner != 0 {
		t.Errorf("MessengerLostAtSea to sender=%d owner=%d, want 1 and 0 (the sealed envelope is the sender's alone)", toSender, toOwner)
	}
}

// A runner takes a ship only if it lands nearer its goal than the port it
// stands in (Timothy 2026-10-21). Layout: port (0,0), sea (1..4,0), target
// city (5,0), then a land road east (6..11,0). Landing at (11,0) has a land
// road back to the target, but is further from it (6) than the port is (5).
func TestPassageScan_BoardsOnlyShipLandingNearer(t *testing.T) {
	for _, c := range []struct {
		name   string
		landQ  int
		boards bool
	}{{"further away waits", 11, false}, {"nearer boards", 9, true}} {
		t.Run(c.name, func(t *testing.T) {
			f := setupPassageFixture(t)
			ctx := context.Background()
			for q := 6; q <= 11; q++ {
				if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`, f.worldID, q); err != nil {
					t.Fatal(err)
				}
			}
			runner := f.waitingMessenger(t, f.originID, f.currentTick+1)
			ship := f.ship(t, f.originID, "merchantman")
			f.departingTransportTo(t, f.originID, f.destID, c.landQ, 0, ship, f.currentTick+3)
			if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: f.currentTick}); err != nil {
				t.Fatal(err)
			}
			_, passage, _, _ := f.messengerRow(t, runner)
			aboard := passage != nil && *passage == "aboard"
			if aboard != c.boards {
				t.Fatalf("landing at (%d,0): aboard=%v, want %v", c.landQ, aboard, c.boards)
			}
		})
	}
}
