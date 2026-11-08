package handlers

// Varvet delar sin takt (Timothy 2026-11-07, mig 255): the yard's placed workers (max 3) are shared
// EQUALLY by every ship the yard is working on (building or repairing), in 60ths of a worker-tick.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// addShip puts a new ship of the fixture's owner in the fixture's city, with `work` worker-ticks paid.
func (f shipRepairFixture) addShip(t *testing.T, status string, work int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, hull, status, settlement_id, support_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 2, $3, $4, $4) RETURNING id`,
		f.worldID, f.playerID, status, f.settlementID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if work > 0 {
		if err := economy.StartShipyardWork(context.Background(), f.pool, id, work); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (f shipRepairFixture) tick(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT current_tick FROM worlds WHERE id=$1`, f.worldID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// daysToFinish = due_tick of the ONE pending ShipyardWorkComplete minus now.
func (f shipRepairFixture) daysToFinish(t *testing.T, ship uuid.UUID) int {
	t.Helper()
	var n, due int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, ship.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ship %s: %d pending ShipyardWorkComplete, want exactly 1", ship, n)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT due_tick FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, ship.String()).Scan(&due); err != nil {
		t.Fatal(err)
	}
	return due - f.tick(t)
}

func (f shipRepairFixture) finish(t *testing.T, ship uuid.UUID) {
	t.Helper()
	var payload json.RawMessage
	if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, ship.String()).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if err := combat.NewShipyardWorkCompleteHandler(f.pool, events.NewStore(f.pool), nil).Handle(context.Background(), events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatal(err)
	}
}

func yardFixture(t *testing.T, level, workers int) shipRepairFixture {
	t.Helper()
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "silver": 1000, "cedar": 1000})
	f.exec(t, `UPDATE buildings SET level=$2 WHERE settlement_id=$1`, f.settlementID, level)
	placeShipyardWorkers(t, f.pool, f.settlementID, workers)
	return f
}

// One ship, one worker, 12 worker-ticks: done after 12 days, exactly as before the change.
func TestVarvet_SingleShipKeepsItsFinishTick(t *testing.T) {
	f := yardFixture(t, 3, 1)
	ship := f.addShip(t, "forming", 12)
	if d := f.daysToFinish(t, ship); d != 12 {
		t.Fatalf("one ship, one worker: %d days, want 12", d)
	}
	var rem, rate int
	f.pool.QueryRow(context.Background(), `SELECT shipyard_work_remaining,shipyard_work_rate FROM units WHERE id=$1`, ship).Scan(&rem, &rate)
	if rem != 12*economy.ShipyardWorkParts || rate != economy.ShipyardWorkParts {
		t.Fatalf("stored %d at rate %d, want %d at %d", rem, rate, 12*economy.ShipyardWorkParts, economy.ShipyardWorkParts)
	}
}

func TestVarvet_TheYardSharesItsPaceEqually(t *testing.T) {
	for _, c := range []struct{ ships, workers, days int }{{3, 3, 12}, {2, 3, 8}, {1, 3, 4}, {3, 1, 36}, {2, 1, 24}} {
		f := yardFixture(t, 3, c.workers)
		var ships []uuid.UUID
		for i := 0; i < c.ships; i++ {
			ships = append(ships, f.addShip(t, "forming", 12))
		}
		for _, s := range ships {
			if d := f.daysToFinish(t, s); d != c.days {
				t.Fatalf("%d ships %d workers: %d days, want %d", c.ships, c.workers, d, c.days)
			}
		}
		w, err := economy.LoadShipyardWork(context.Background(), f.pool, ships[0])
		if err != nil || w.Workers != c.workers || w.Sharing != c.ships || w.Remaining == nil || *w.Remaining != 12 {
			t.Fatalf("%d ships %d workers: %+v %v", c.ships, c.workers, w, err)
		}
	}
}

// Repairs and builds share the same pace.
func TestVarvet_RepairsShareWithBuilds(t *testing.T) {
	f := yardFixture(t, 3, 2)
	build := f.addShip(t, "forming", 12)
	repair := f.addShip(t, "repairing", 12)
	if f.daysToFinish(t, build) != 12 || f.daysToFinish(t, repair) != 12 {
		t.Fatal("2 workers, 2 ships: one worker each, 12 days")
	}
}

// A ship that finishes gives its share back the same tick: the others get a new timer, no double event.
func TestVarvet_FinishedShipReturnsItsShare(t *testing.T) {
	f := yardFixture(t, 3, 3)
	a := f.addShip(t, "forming", 6)
	b := f.addShip(t, "forming", 12)
	c := f.addShip(t, "forming", 12)
	f.exec(t, `UPDATE worlds SET current_tick=current_tick+6 WHERE id=$1`, f.worldID)
	f.finish(t, a)
	// b and c have 6 worker-ticks left; two ships share 3 workers = 1.5 each -> 4 days
	for _, s := range []uuid.UUID{b, c} {
		if d := f.daysToFinish(t, s); d != 4 {
			t.Fatalf("after a finished: %d days, want 4", d)
		}
	}
	var open int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, a.String()).Scan(&open)
	if open != 0 {
		t.Fatal("the finished ship still has a timer")
	}
}

// A ship that starts slows the others; what they already did is kept.
func TestVarvet_NewShipSlowsTheOthersButKeepsDoneWork(t *testing.T) {
	f := yardFixture(t, 3, 3)
	a := f.addShip(t, "forming", 12)
	if d := f.daysToFinish(t, a); d != 4 {
		t.Fatalf("alone with 3 workers: %d, want 4", d)
	}
	f.exec(t, `UPDATE worlds SET current_tick=current_tick+2 WHERE id=$1`, f.worldID) // 6 worker-ticks done
	b := f.addShip(t, "forming", 12)
	w, _ := economy.LoadShipyardWork(context.Background(), f.pool, a)
	if w.Remaining == nil || *w.Remaining != 6 || w.Sharing != 2 {
		t.Fatalf("a: %+v", w)
	}
	if d := f.daysToFinish(t, a); d != 4 { // 6 left at 1.5/day
		t.Fatalf("a: %d days, want 4", d)
	}
	if d := f.daysToFinish(t, b); d != 8 { // 12 at 1.5/day
		t.Fatalf("b: %d days, want 8", d)
	}
}

// A ship that leaves the yard (status change) also returns its share, and the other trigger columns do too.
func TestVarvet_UnitTriggerSyncsTheWholeCity(t *testing.T) {
	f := yardFixture(t, 3, 3)
	a := f.addShip(t, "forming", 12)
	b := f.addShip(t, "forming", 12)
	if f.daysToFinish(t, b) != 8 {
		t.Fatal("setup")
	}
	f.exec(t, `UPDATE units SET status='garrison' WHERE id=$1`, a)
	if d := f.daysToFinish(t, b); d != 4 {
		t.Fatalf("b after a left: %d days, want 4", d)
	}
	f.exec(t, `UPDATE units SET settlement_id=NULL WHERE id=$1`, b)
}

// Staffing changes still reach every ship (existing city trigger) with the shared rate.
func TestVarvet_FewerWorkersSlowsEveryShip(t *testing.T) {
	f := yardFixture(t, 3, 3)
	a := f.addShip(t, "forming", 12)
	b := f.addShip(t, "forming", 12)
	f.exec(t, `DELETE FROM settlement_placement WHERE settlement_id=$1 AND gubbe_ordinal>1`, f.settlementID)
	if f.daysToFinish(t, a) != 24 || f.daysToFinish(t, b) != 24 {
		t.Fatal("1 worker, 2 ships: 24 days")
	}
}

// Migration 255: a job in flight keeps its finish tick and its done work, and is stored in 60ths.
func TestVarvet_MigrationKeepsFinishTickAndWork(t *testing.T) {
	raw, err := os.ReadFile("../../db/migrations/255_shipyard_shared_pace.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	i, j := strings.Index(s, "-- BEGIN CONVERT"), strings.Index(s, "-- END CONVERT")
	if i < 0 || j < i {
		t.Fatal("convert markers missing")
	}
	convert := s[i:j]
	for _, c := range []struct {
		name             string
		elapsed, wantRem int
	}{{"untouched", 0, 12}, {"four days in", 4, 8}} {
		f := yardFixture(t, 3, 1)
		ship := f.addShip(t, "forming", 12)
		t0 := f.tick(t)
		due0 := t0 + 12
		// put the row back as the OLD code stored it: worker-ticks, rate = workers
		f.exec(t, `UPDATE units SET shipyard_work_remaining=12,shipyard_work_rate=1 WHERE id=$1`, ship)
		f.exec(t, `UPDATE worlds SET current_tick=current_tick+$2 WHERE id=$1`, f.worldID, c.elapsed)
		// the suite leaves new-format jobs in the shared DB; production holds only old-format ones
		f.exec(t, `UPDATE units SET shipyard_work_remaining=NULL WHERE shipyard_work_remaining IS NOT NULL AND id<>$1`, ship)
		f.exec(t, convert)
		var rem, rate, calc int
		f.pool.QueryRow(context.Background(), `SELECT shipyard_work_remaining,shipyard_work_rate,shipyard_work_calc_tick FROM units WHERE id=$1`, ship).Scan(&rem, &rate, &calc)
		if rem != c.wantRem*60 || rate != 60 || calc != t0+c.elapsed {
			t.Fatalf("%s: rem=%d rate=%d calc=%d", c.name, rem, rate, calc)
		}
		if got := f.daysToFinish(t, ship) + f.tick(t); got != due0 {
			t.Fatalf("%s: finish tick %d, was %d", c.name, got, due0)
		}
	}
}

// The Builder's Blessing cuts a shared ship's remaining time to 3/4, once.
func TestVarvet_BlessingOnASharedShip(t *testing.T) {
	f := yardFixture(t, 3, 3)
	a := f.addShip(t, "forming", 12)
	b := f.addShip(t, "forming", 12)
	f.exec(t, `INSERT INTO prayer_blessings(world_id,owner_id,scope,settlement_id,effect,from_tick,until_tick) VALUES($1,$2,'city',$3,'blessed_building',$4,$5)`,
		f.worldID, f.playerID, f.settlementID, f.tick(t), f.tick(t)+50)
	if _, err := economy.BlessBuilds(context.Background(), f.pool, []uuid.UUID{f.settlementID}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []uuid.UUID{a, b} {
		if d := f.daysToFinish(t, s); d != 6 { // 8 days -> ceil(3*8/4)=6
			t.Fatalf("blessed shared ship: %d days, want 6", d)
		}
	}
	f.exec(t, `SELECT sync_shipyard_work($1)`, a)
	if d := f.daysToFinish(t, a); d != 6 {
		t.Fatalf("re-sync cut again: %d", d)
	}
}

func (f shipRepairFixture) repair(t *testing.T, ship uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/"+ship.String()+"/repair", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// Repair berths = the yard's workplaces: 1/2/3 by level. Repairs already running above a lowered cap go on.
func TestVarvet_RepairBerthsAreWorkplaces(t *testing.T) {
	for _, c := range []struct{ level, allowed int }{{1, 1}, {2, 2}, {3, 3}} {
		f := yardFixture(t, c.level, 1)
		for i := 0; i < c.allowed; i++ {
			ship := f.shipID
			if i > 0 {
				ship = f.addShip(t, "garrison", 0)
			}
			if r := f.repair(t, ship); r.Code != http.StatusAccepted {
				t.Fatalf("level %d repair %d: %d %s", c.level, i+1, r.Code, r.Body.String())
			}
		}
		r := f.repair(t, f.addShip(t, "garrison", 0))
		if r.Code != http.StatusUnprocessableEntity || !strings.Contains(r.Body.String(), "shipyard full") {
			t.Fatalf("level %d: one repair too many: %d %s", c.level, r.Code, r.Body.String())
		}
	}
}
