package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Pure existing endpoint baseline: paid repair at an empty yard must wait.
func TestShipyardUnstaffedPaidRepairWaits(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "silver": 1000})
	rec := f.postRepair(t)
	if rec.Code != http.StatusAccepted {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["shipyard_waiting"] != true || body["complete_at"] != nil {
		t.Fatalf("unstaffed repair claims progress/ETA: %s", rec.Body.String())
	}
}

func TestShipyardPauseResumePreservesPaidProgressAndRejectsStaleTimer(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "silver": 1000})
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	if r := f.postRepair(t); r.Code != http.StatusAccepted {
		t.Fatal(r.Body.String())
	}
	work := func(want int, paused bool) {
		t.Helper()
		w, err := economy.LoadShipyardWork(ctx, f.pool, f.shipID)
		if err != nil {
			t.Fatal(err)
		}
		if w.Remaining == nil || *w.Remaining != want || w.Waiting != paused || (w.CompleteAt == nil) != paused {
			t.Fatalf("work %+v want %d paused=%v", w, want, paused)
		}
	}
	work(3, true)
	exec(`UPDATE worlds SET current_tick=current_tick+8 WHERE id=$1`, f.worldID)
	work(3, true)
	exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID)
	work(3, false)
	var stale json.RawMessage
	if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, f.shipID.String()).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, f.worldID)
	exec(`DELETE FROM settlement_placement WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID)
	work(2, true)
	exec(`UPDATE worlds SET current_tick=current_tick+20 WHERE id=$1`, f.worldID)
	work(2, true)
	h := combat.NewShipyardWorkCompleteHandler(f.pool, events.NewStore(f.pool), nil)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: stale}); err != nil {
		t.Fatal(err)
	}
	work(2, true)
	exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID)
	work(2, false)
	var payload json.RawMessage
	if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, f.shipID.String()).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE worlds SET current_tick=current_tick+2 WHERE id=$1`, f.worldID)
	for i := 0; i < 2; i++ {
		if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var hull, count int
	f.pool.QueryRow(ctx, `SELECT status,hull FROM units WHERE id=$1`, f.shipID).Scan(&status, &hull)
	f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='ShipRepaired'`, f.shipID).Scan(&count)
	if status != "garrison" || hull != combat.HullMax || count != 1 {
		t.Fatalf("completion status=%s hull=%d events=%d", status, hull, count)
	}
	var timber float64
	f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'`, f.settlementID).Scan(&timber)
	_, cost, _ := combat.RepairCost("galley", 3)
	if timber != 1000-cost {
		t.Fatalf("charged more than once: %g", timber)
	}
}

func TestShipyardLifecycleChangesStopWorkAtomically(t *testing.T) {
	for _, change := range []string{"demolition", "population", "owner", "unit-owner", "collapsed"} {
		t.Run(change, func(t *testing.T) {
			f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
			ctx := context.Background()
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := f.pool.Exec(ctx, q, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID)
			if r := f.postRepair(t); r.Code != http.StatusAccepted {
				t.Fatal(r.Body.String())
			}
			exec(`UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, f.worldID)
			switch change {
			case "demolition":
				exec(`DELETE FROM buildings WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID)
			case "population":
				exec(`UPDATE settlements SET population=99 WHERE id=$1`, f.settlementID)
				if _, err := economy.PrunePlacementsToPopulation(ctx, f.pool, f.settlementID); err != nil {
					t.Fatal(err)
				}
			case "owner":
				exec(`UPDATE settlements SET owner_id=NULL WHERE id=$1`, f.settlementID)
			case "unit-owner":
				exec(`UPDATE units SET settlement_id=NULL WHERE id=$1`, f.shipID)
			case "collapsed":
				exec(`UPDATE settlements SET state='collapsed' WHERE id=$1`, f.settlementID)
			}
			exec(`UPDATE worlds SET current_tick=current_tick+10 WHERE id=$1`, f.worldID)
			w, err := economy.LoadShipyardWork(ctx, f.pool, f.shipID)
			if err != nil {
				t.Fatal(err)
			}
			if !w.Waiting || w.CompleteAt != nil || w.Remaining == nil || *w.Remaining != 2 {
				t.Fatalf("%s: %+v", change, w)
			}
		})
	}
}

func TestShipyardDeployTransitionPreservesDeadlineWithoutFreeStaff(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
	ctx := context.Background()
	// Saved pre204 job: two of three normal ticks remain, with a legacy timer.
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='repairing',build_complete_at=(SELECT last_tick_at FROM worlds WHERE id=$2) WHERE id=$1;`, f.shipID, f.worldID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(combat.ShipRepairCompletePayload{UnitID: f.shipID, SettlementID: f.settlementID, WorldID: f.worldID, UnitType: "galley"})
	if _, err := f.pool.Exec(ctx, `INSERT INTO scheduled_events(world_id,event_type,payload,process_after,due_tick) VALUES($1,'ShipRepairComplete',$2,now(),12)`, f.worldID, payload); err != nil {
		t.Fatal(err)
	}
	if err := economy.CompleteShipyardWorkTransition(ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		w, err := economy.LoadShipyardWork(ctx, f.pool, f.shipID)
		if err != nil || w.Remaining == nil || *w.Remaining != 2 || !w.Waiting {
			t.Fatalf("transition %+v err=%v", w, err)
		}
		if err := economy.CompleteShipyardWorkTransition(ctx, f.pool); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE payload->>'unit_id'=$1 AND event_type='ShipRepairComplete' AND processed_at IS NULL`, f.shipID.String()).Scan(&n); err != nil || n != 0 {
		t.Fatalf("legacy timer still armed %d %v", n, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("free staff %d %v", n, err)
	}
}

func TestShipyardPlacementIsServiceWithCapacityAndIdleLifecycle(t *testing.T) {
	f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
	pool := p10TestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level) VALUES($1,'shipyard',1)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	code, body := f.do(t, http.MethodGet, f.placementOptionsPath(), nil)
	if code != 200 {
		t.Fatal(code, body)
	}
	encoded, _ := json.Marshal(body)
	if !strings.Contains(string(encoded), `"shipbuilding"`) || !strings.Contains(string(encoded), `"staffing_only":true`) {
		t.Fatal(string(encoded))
	}
	for i := 0; i < 3; i++ {
		code, body = f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "building", "building_type": "shipyard", "good_key": "shipbuilding"})
		if code != 201 {
			t.Fatal(code, body)
		}
	}
	code, body = f.do(t, http.MethodPost, f.placementsPath(), map[string]any{"target_kind": "building", "building_type": "shipyard", "good_key": "shipbuilding"})
	if code != 409 {
		t.Fatal("overcapacity", code, body)
	}
	weight, shippable, err := economy.IsShippableGood(ctx, pool, "shipbuilding")
	if err != nil || shippable || weight != 0 {
		t.Fatal(weight, shippable, err)
	}
	var bid uuid.UUID
	pool.QueryRow(ctx, `SELECT id FROM buildings WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID).Scan(&bid)
	if _, err := economy.LiftPlacementsAfterDemolish(ctx, pool, f.settlementID, bid, "shipyard", nil, nil); err != nil {
		t.Fatal(err)
	}
	var placed int
	pool.QueryRow(ctx, `SELECT count(*) FROM settlement_placement WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID).Scan(&placed)
	if placed != 0 {
		t.Fatal(placed)
	}
}

func TestShipyardPaidBuildsShareOneWorkerAndPausedOrdersStillCount(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	path := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"
	rec, body := f.post(t, path, map[string]any{"unit_type": "galley", "count": 10})
	if rec.Code != http.StatusCreated || body["shipyard_waiting"] != true || body["complete_at"] != nil {
		t.Fatal(rec.Code, body)
	}
	rec, body = f.post(t, path, map[string]any{"unit_type": "galley"})
	if rec.Code != 422 {
		t.Fatal("paused orders escaped cap", rec.Code, body)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM units WHERE settlement_id=$1 AND shipyard_work_running AND shipyard_work_remaining=3`, f.settlementID).Scan(&n); err != nil || n != 10 {
		t.Fatal("one worker must run all ten jobs at full pace", n, err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlement_placement WHERE settlement_id=$1 AND building_type='shipyard'`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM units WHERE settlement_id=$1 AND NOT shipyard_work_running AND shipyard_work_remaining=3`, f.settlementID).Scan(&n); err != nil || n != 10 {
		t.Fatal("last worker must pause all jobs", n, err)
	}
}

func TestShipyardRepairSchedulingFailureRollsBackPaymentAndStatus(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	sql := fmt.Sprintf(`CREATE FUNCTION fail_shipyard_queue() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.world_id='%s'::uuid AND NEW.event_type='ShipyardWorkComplete' THEN RAISE EXCEPTION 'injected queue failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_shipyard_queue BEFORE INSERT ON scheduled_events FOR EACH ROW EXECUTE FUNCTION fail_shipyard_queue();`, f.worldID)
	if _, err := f.pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer f.pool.Exec(ctx, `DROP TRIGGER fail_shipyard_queue ON scheduled_events; DROP FUNCTION fail_shipyard_queue();`)
	rec := f.postRepair(t)
	if rec.Code != 500 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var status string
	var remaining *int
	var timber float64
	if err := f.pool.QueryRow(ctx, `SELECT status,shipyard_work_remaining FROM units WHERE id=$1`, f.shipID).Scan(&status, &remaining); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='timber'`, f.settlementID).Scan(&timber); err != nil {
		t.Fatal(err)
	}
	if status != "garrison" || remaining != nil || timber != 1000 {
		t.Fatal(status, remaining, timber)
	}
}

func TestShipyardReadSurfacesUseSameWorldAnchorDespiteClockSkew(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	f.uh.clk = clock.NewTestClock(time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC))
	f.router.Get("/worlds/{worldID}/units", f.uh.ListUnits)
	rec := f.postRepair(t)
	if rec.Code != 202 {
		t.Fatal(rec.Body.String())
	}
	var repair map[string]any
	json.Unmarshal(rec.Body.Bytes(), &repair)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	res := httptest.NewRecorder()
	f.router.ServeHTTP(res, req)
	var list struct {
		Units []struct {
			Ready string `json:"build_complete_at"`
		}
	}
	if err := json.Unmarshal(res.Body.Bytes(), &list); err != nil {
		t.Fatal(err, res.Body.String())
	}
	if len(list.Units) != 1 || list.Units[0].Ready != repair["complete_at"] {
		t.Fatal(repair, list, res.Body.String())
	}
}

type shipyardNoticeRecorder struct {
	count, level       int
	name               string
	unitName, cityName string
}

func (r *shipyardNoticeRecorder) BroadcastEvent(uuid.UUID, string, any) {}
func (r *shipyardNoticeRecorder) NotifyPlayer(_ context.Context, _, _ uuid.UUID, _ string, level int, body any) error {
	r.count++
	r.level = level
	r.name, _ = body.(map[string]any)["name"].(string)
	r.unitName, _ = body.(map[string]any)["unit_name"].(string)
	r.cityName, _ = body.(map[string]any)["settlement_name"].(string)
	return nil
}
func TestShipyardCompletionPreservesNotificationIdentityAndSeverity(t *testing.T) {
	for _, status := range []string{"forming", "repairing"} {
		t.Run(status, func(t *testing.T) {
			f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
			ctx := context.Background()
			for _, q := range []string{`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`} {
				if _, err := f.pool.Exec(ctx, q, f.settlementID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.pool.Exec(ctx, `UPDATE units SET status=$2,name='Asterion' WHERE id=$1`, f.shipID, status); err != nil {
				t.Fatal(err)
			}
			if status == "forming" {
				if _, err := f.pool.Exec(ctx, `UPDATE units SET type='merchantman',crew=10 WHERE id=$1`, f.shipID); err != nil {
					t.Fatal(err)
				}
			}
			if err := economy.StartShipyardWork(ctx, f.pool, f.shipID, 1); err != nil {
				t.Fatal(err)
			}
			var payload json.RawMessage
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND processed_at IS NULL AND payload->>'unit_id'=$1`, f.shipID.String()).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=current_tick+1 WHERE id=$1`, f.worldID); err != nil {
				t.Fatal(err)
			}
			recorder := &shipyardNoticeRecorder{}
			h := combat.NewShipyardWorkCompleteHandler(f.pool, events.NewStore(f.pool), recorder)
			for i := 0; i < 2; i++ {
				if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
					t.Fatal(err)
				}
			}
			var cityName string
			if err := f.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id=$1`, f.settlementID).Scan(&cityName); err != nil {
				t.Fatal(err)
			}
			expectedUnit := "Galley"
			if status == "forming" {
				expectedUnit = "Emporos"
			}
			if recorder.cityName != cityName || recorder.unitName != expectedUnit {
				t.Fatalf("shipyard completion lost city/player name: %+v", recorder)
			}
			want := 4
			if status == "repairing" {
				want = 3
				if !strings.Contains(recorder.name, "Asterion") {
					t.Fatal(recorder.name)
				}
			}
			if recorder.count != 1 || recorder.level != want {
				t.Fatal(recorder)
			}
		})
	}
}
