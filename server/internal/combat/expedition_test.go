package combat

// Upptäckarexpeditionen (megaron_plan_upptackarexpeditionen.md). These tests
// drive the real StartMarch and the real arrival handler leg by leg, moving
// the world clock to each scheduled arrival, until the unit is home.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

type expeditionFixture struct {
	worldID, ownerID, capitalID, unitID uuid.UUID
	h                                   *UnitArrivalHandler
	scheduler                           *events.Scheduler
	eventStore                          *events.Store
	clk                                 *clock.TestClock
}

// newExpeditionFixture lays a plains map q∈[minQ,maxQ], r∈[-4,4] with the
// capital at (0,0) and one garrisoned infantry cohort in it. copperAt, if
// set, gets a copper deposit.
func newExpeditionFixture(t *testing.T, minQ, maxQ int, copperAt *[2]int) expeditionFixture {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active worlds: %v", err)
	}
	var f expeditionFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"expedition-"+uuid.New().String(),
	).Scan(&f.ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}
	for q := minQ; q <= maxQ; q++ {
		for r := -4; r <= 4; r++ {
			copper := copperAt != nil && copperAt[0] == q && copperAt[1] == r
			if _, err := pool.Exec(ctx,
				`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit) VALUES ($1, $2, $3, 'plains', $4)`,
				f.worldID, q, r, copper,
			); err != nil {
				t.Fatalf("insert tile (%d,%d): %v", q, r, err)
			}
		}
	}
	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Home', 'achaean', $3, 'capital', true) RETURNING id`,
		f.worldID, provinceID, f.ownerID,
	).Scan(&f.capitalID); err != nil {
		t.Fatalf("create capital: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id)
		 VALUES ($1, $2, 'infantry', 'land', 100, 'garrison', $3) RETURNING id`,
		f.worldID, f.ownerID, f.capitalID,
	).Scan(&f.unitID); err != nil {
		t.Fatalf("create unit: %v", err)
	}
	f.clk = clock.NewTestClock(time.Now())
	f.scheduler = events.NewScheduler(pool, f.clk)
	f.eventStore = events.NewStore(pool)
	f.h = &UnitArrivalHandler{pool: pool, eventStore: f.eventStore, scheduler: f.scheduler, clk: f.clk}
	return f
}

// legLog is one resolved arrival: the tick it resolved on and the intent the
// unit carried into it.
type legLog struct {
	tick   int
	intent string
}

// runUntilHome resolves arrivals one at a time — the world clock set to each
// arrival's own tick — until the unit is back in garrison.
func (f expeditionFixture) runUntilHome(t *testing.T) []legLog {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	var log []legLog
	for i := 0; i < 60; i++ {
		var status, intent string
		var arriveTick *int
		if err := pool.QueryRow(ctx,
			`SELECT status, COALESCE(march_intent, ''), arrive_tick FROM units WHERE id = $1`, f.unitID,
		).Scan(&status, &intent, &arriveTick); err != nil {
			t.Fatalf("load unit: %v", err)
		}
		if status == "garrison" {
			return log
		}
		if status != "marching" || arriveTick == nil {
			t.Fatalf("unit status %q arrive_tick %v — expected a march in progress", status, arriveTick)
		}
		if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, *arriveTick); err != nil {
			t.Fatalf("advance clock: %v", err)
		}
		raw, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.worldID, ArriveTick: arriveTick})
		if err := f.h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: raw}); err != nil {
			t.Fatalf("arrival at tick %d: %v", *arriveTick, err)
		}
		log = append(log, legLog{tick: *arriveTick, intent: intent})
	}
	t.Fatal("unit never came home")
	return nil
}

func (f expeditionFixture) start(t *testing.T, q, r, ticks int) *MarchStarted {
	t.Helper()
	res, err := StartMarch(context.Background(), testPool(t), f.scheduler, f.eventStore, f.clk, MarchOrder{
		WorldID: f.worldID, PlayerID: f.ownerID, UnitID: f.unitID,
		TargetQ: q, TargetR: r, Intent: "explore", ExpeditionTicks: ticks,
	}, nil)
	if err != nil {
		t.Fatalf("StartMarch(explore): %v", err)
	}
	return res
}

func (f expeditionFixture) eventPayloads(t *testing.T, eventType string) []json.RawMessage {
	t.Helper()
	rows, err := testPool(t).Query(context.Background(),
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = $2 ORDER BY id`, f.unitID, eventType)
	if err != nil {
		t.Fatalf("load %s events: %v", eventType, err)
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var p json.RawMessage
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		out = append(out, p)
	}
	return out
}

// Invariant 1: no leg ends after the turn tick, and the unit is home no later
// than start + length — here the area is far larger than 12 ticks can cover,
// so the expedition must turn on time, not when it runs out of ground.
func TestExpedition_TurnsByHalfTimeAndIsHomeByLength(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	res := f.start(t, 9, 0, 12)
	if res.Expedition == nil {
		t.Fatal("MarchStarted.Expedition = nil for an explore order")
	}
	if res.Expedition.TurnTick != 6 || res.Expedition.HomeByTick != 12 {
		t.Fatalf("plan turn/home = %d/%d, want 6/12", res.Expedition.TurnTick, res.Expedition.HomeByTick)
	}
	if res.TargetQ != 4 || res.TargetR != 0 {
		t.Errorf("first leg = (%d,%d), want (4,0): the nearest unseen hex of the area from home", res.TargetQ, res.TargetR)
	}

	legs := f.runUntilHome(t)
	last := legs[len(legs)-1]
	if last.tick > 12 {
		t.Errorf("home at tick %d, after start+length 12 — the way home did not fit", last.tick)
	}
	for _, l := range legs[:len(legs)-1] {
		if l.intent == "explore" && l.tick > 6 {
			t.Errorf("an outbound leg ended at tick %d, after the turn tick 6", l.tick)
		}
	}

	turned := f.eventPayloads(t, unit.EventExpeditionTurnedHome)
	if len(turned) != 1 {
		t.Fatalf("ExpeditionTurnedHome events = %d, want 1", len(turned))
	}
	var tp unit.ExpeditionTurnedHomePayload
	_ = json.Unmarshal(turned[0], &tp)
	if tp.Reason != ExpeditionTurnHalfTime {
		t.Errorf("turn reason = %q, want %q", tp.Reason, ExpeditionTurnHalfTime)
	}
	if n := len(f.eventPayloads(t, unit.EventUnitExploreReturned)); n != 0 {
		t.Errorf("UnitExploreReturned events = %d, want 0 — an expedition names its own turn", n)
	}
	var settlementID *uuid.UUID
	if err := testPool(t).QueryRow(context.Background(),
		`SELECT settlement_id FROM units WHERE id = $1`, f.unitID).Scan(&settlementID); err != nil {
		t.Fatal(err)
	}
	if settlementID == nil || *settlementID != f.capitalID {
		t.Errorf("unit settlement = %v, want home %v", settlementID, f.capitalID)
	}
}

// Invariant 2: a leg records what the unit saw along the whole way, not only
// where it stopped — (1,2) is in sight from (1,0) on the first leg's road to
// (4,0) but not from (4,0) itself, and nobody read the map in between.
func TestExpedition_RecordsSightAlongTheWholeLeg(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	f.start(t, 9, 0, 12)

	pool := testPool(t)
	ctx := context.Background()
	var arriveTick int
	if err := pool.QueryRow(ctx, `SELECT arrive_tick FROM units WHERE id = $1`, f.unitID).Scan(&arriveTick); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, f.worldID, arriveTick); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.worldID, ArriveTick: &arriveTick})
	if err := f.h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: raw}); err != nil {
		t.Fatalf("first leg arrival: %v", err)
	}

	var scouted bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2 AND q = 1 AND r = 2)`,
		f.worldID, f.ownerID,
	).Scan(&scouted); err != nil {
		t.Fatal(err)
	}
	if !scouted {
		t.Error("(1,2) not recorded — the leg only swept where it stopped, so the next target depends on the player reading the map")
	}
}

// A small area is used up before the time is: the expedition turns because
// there is nothing more to see, and the report names what it found.
func TestExpedition_AreaKnownTurnsEarlyAndReportsFinds(t *testing.T) {
	copper := [2]int{6, -1}
	f := newExpeditionFixture(t, -2, 8, &copper)
	f.start(t, 5, 0, 30)
	legs := f.runUntilHome(t)
	if last := legs[len(legs)-1]; last.tick > 30 {
		t.Errorf("home at tick %d, after start+length 30", last.tick)
	}

	turned := f.eventPayloads(t, unit.EventExpeditionTurnedHome)
	if len(turned) != 1 {
		t.Fatalf("ExpeditionTurnedHome events = %d, want 1", len(turned))
	}
	var tp unit.ExpeditionTurnedHomePayload
	_ = json.Unmarshal(turned[0], &tp)
	if tp.Reason != ExpeditionTurnAreaKnown {
		t.Errorf("turn reason = %q, want %q", tp.Reason, ExpeditionTurnAreaKnown)
	}

	reports := f.eventPayloads(t, unit.EventExpeditionReport)
	if len(reports) != 1 {
		t.Fatalf("ExpeditionReport events = %d, want 1", len(reports))
	}
	var rp unit.ExpeditionReportPayload
	_ = json.Unmarshal(reports[0], &rp)
	if rp.HexesSeen == 0 || rp.Furthest == 0 || rp.TicksOut == 0 {
		t.Errorf("report = %+v, want hexes seen, a furthest distance and ticks out", rp)
	}
	found := false
	for _, fd := range rp.Finds {
		if fd.Kind == "copper" && fd.Q == copper[0] && fd.R == copper[1] {
			found = true
		}
	}
	if !found {
		t.Errorf("report finds = %+v, want copper at %v", rp.Finds, copper)
	}

	var rows int
	if err := testPool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM unit_expeditions WHERE unit_id = $1`, f.unitID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("unit_expeditions rows after the report = %d, want 0", rows)
	}
}

// A recall or redirect gives the unit a new arrive_tick; the expedition must
// let go rather than steer a unit that has been given another order.
func TestExpedition_NewCourseEndsTheExpedition(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	f.start(t, 9, 0, 12)

	pool := testPool(t)
	ctx := context.Background()
	// What a redirect does to the row that matters here: a new arrival tick.
	var arriveTick int
	if err := pool.QueryRow(ctx,
		`UPDATE units SET arrive_tick = arrive_tick + 1 WHERE id = $1 RETURNING arrive_tick`, f.unitID,
	).Scan(&arriveTick); err != nil {
		t.Fatal(err)
	}
	legs := f.runUntilHome(t)
	if len(legs) != 2 {
		t.Errorf("arrivals = %d, want 2 (point explore: out, home)", len(legs))
	}
	if n := len(f.eventPayloads(t, unit.EventExpeditionReport)); n != 0 {
		t.Errorf("ExpeditionReport events = %d, want 0 after a new course", n)
	}
	if n := len(f.eventPayloads(t, unit.EventUnitExploreReturned)); n != 1 {
		t.Errorf("UnitExploreReturned events = %d, want 1 (the plain explore turn)", n)
	}
}

func TestExpedition_DispatchRefusals(t *testing.T) {
	f := newExpeditionFixture(t, -2, 16, nil)
	ctx := context.Background()
	order := func(q, r, ticks int) error {
		_, err := StartMarch(ctx, testPool(t), f.scheduler, f.eventStore, f.clk, MarchOrder{
			WorldID: f.worldID, PlayerID: f.ownerID, UnitID: f.unitID,
			TargetQ: q, TargetR: r, Intent: "explore", ExpeditionTicks: ticks,
		}, nil)
		return err
	}
	if err := order(9, 0, ExpeditionMaxTicks+1); err == nil {
		t.Error("length above the maximum accepted")
	}
	// Nearest unseen ground around (14,0) from home is (9,0): 9 ticks away,
	// more than half of 10.
	if err := order(14, 0, 10); err == nil {
		t.Error("an expedition whose first leg ends after its turn tick was accepted")
	}
	// Everything around (1,0) known.
	if _, err := testPool(t).Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
		 SELECT $1, $2, q, r FROM map_tiles WHERE world_id = $1`, f.worldID, f.ownerID); err != nil {
		t.Fatal(err)
	}
	if err := order(1, 0, 10); err == nil {
		t.Error("an expedition into fully known ground was accepted")
	}
}
