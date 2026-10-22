package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func setupLayUp(t *testing.T) shipRepairFixture {
	t.Helper()
	f := newShipRepairFixture(t, true, nil)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO buildings (settlement_id,building_type,level) VALUES ($1,'harbour',1)`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=5 WHERE id=$1`, f.shipID); err != nil {
		t.Fatal(err)
	}
	f.router.Post("/worlds/{worldID}/units/{unitID}/lay-up", f.uh.LayUp)
	f.router.Post("/worlds/{worldID}/units/{unitID}/fit-out", f.uh.FitOut)
	f.router.Get("/worlds/{worldID}/units", f.uh.ListUnits)
	return f
}

func (f shipRepairFixture) portFlags(t *testing.T) (bool, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list units: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Units []struct {
			ID        uuid.UUID `json:"id"`
			CanLayUp  bool      `json:"can_lay_up"`
			CanFitOut bool      `json:"can_fit_out"`
		} `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, u := range body.Units {
		if u.ID == f.shipID {
			return u.CanLayUp, u.CanFitOut
		}
	}
	t.Fatal("ship absent from roster")
	return false, false
}

func TestDisbandLaidUpHullDoesNotReturnCrewTwice(t *testing.T) {
	f := setupLayUp(t)
	ctx := context.Background()
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusOK {
		t.Fatalf("lay up: %d %s", got.Code, got.Body.String())
	}
	var provinceID uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id=$1`, f.settlementID).Scan(&provinceID); err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(f.pool, nil, clock.NewTestClock(time.Now()), nil, nil)
	f.router.Post("/worlds/{worldID}/provinces/{provinceID}/disband", ph.Disband)
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/provinces/"+provinceID.String()+"/disband", strings.NewReader(`{"unit_ids":["`+f.shipID.String()+`"]}`))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disband: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		PopRestored int `json:"pop_restored"`
		Population  int `json:"population"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.PopRestored != 0 || result.Population != 5020 {
		t.Fatalf("double crew refund: %+v", result)
	}
}

func (f shipRepairFixture) shipAction(t *testing.T, action string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/"+f.shipID.String()+"/"+action, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// NamedRED: before Lay up, an idle ship still charged silver and its crew
// stayed outside population; before Fit out, there was no 3-tick readiness.
func TestLayUpReturnsCrewAndZeroesShipUpkeep_FitOutDraftsAndCompletesAfterThreeTicks(t *testing.T) {
	f := setupLayUp(t)
	ctx := context.Background()
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusOK {
		t.Fatalf("lay up: %d %s", got.Code, got.Body.String())
	}
	var status string
	var pop int
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.shipID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, f.settlementID).Scan(&pop); err != nil {
		t.Fatal(err)
	}
	if status != "laid_up" || pop != 5020 {
		t.Fatalf("after lay up: status=%s pop=%d; want laid_up/5020", status, pop)
	}
	if lay, fit := f.portFlags(t); lay || !fit {
		t.Fatalf("laid-up port flags lay=%v fit=%v", lay, fit)
	}
	up := combat.UnitUpkeep("galley", "naval", 1, status, 20)
	if up.Grain != 0 || up.Silver != 0 {
		t.Fatalf("laid-up ship upkeep=%+v, want zero", up)
	}
	drain, _, err := settlementUpkeepDrain(ctx, f.pool, f.settlementID)
	if err != nil || drain.Grain != 0 || drain.Silver != 0 {
		t.Fatalf("city drain=%+v err=%v, want zero", drain, err)
	}
	composition, _, err := armyUpkeep(ctx, f.pool, f.settlementID)
	if err != nil || composition.Grain != 0 || composition.Silver != 0 {
		t.Fatalf("laid-up city army upkeep=%+v err=%v", composition, err)
	}
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate lay up=%d", got.Code)
	}
	if got := f.shipAction(t, "fit-out"); got.Code != http.StatusAccepted {
		t.Fatalf("fit out: %d %s", got.Code, got.Body.String())
	} else {
		var response struct {
			ReadyTick int `json:"ready_tick"`
			Duration  int `json:"duration_ticks"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.ReadyTick != 13 || response.Duration != 3 {
			t.Fatalf("fit out ETA=%+v, want tick 13 after 3", response)
		}
	}
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.shipID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, f.settlementID).Scan(&pop); err != nil {
		t.Fatal(err)
	}
	if status != "fitting_out" || pop != 5000 {
		t.Fatalf("during fit out: status=%s pop=%d", status, pop)
	}
	var scheduled int
	if err := f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE world_id=$1 AND event_type='ShipFitOutComplete' AND payload->>'unit_id'=$2`, f.worldID, f.shipID.String()).Scan(&scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled != 13 {
		t.Fatalf("scheduled completion at tick %d, want 13", scheduled)
	}
	drain, _, err = settlementUpkeepDrain(ctx, f.pool, f.settlementID)
	if err != nil || drain.Silver <= 0 || drain.Grain <= 0 {
		t.Fatalf("fit-out crew upkeep=%+v err=%v, want positive", drain, err)
	}
	composition, _, err = armyUpkeep(ctx, f.pool, f.settlementID)
	if err != nil || composition.Silver != drain.Silver || composition.Grain != drain.Grain {
		t.Fatalf("fit-out city army upkeep=%+v drain=%+v err=%v", composition, drain, err)
	}
	if got := f.shipAction(t, "fit-out"); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate fit out=%d", got.Code)
	}
	h := combat.NewShipFitOutCompleteHandler(f.pool)
	payload, _ := json.Marshal(combat.ShipFitOutCompletePayload{UnitID: f.shipID, WorldID: f.worldID})
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload, DueTick: 13}); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload, DueTick: 13}); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.shipID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "garrison" {
		t.Fatalf("completed status=%s", status)
	}
}

func TestLayUpRejectsCargoPassengersAndUnharbouredShip(t *testing.T) {
	f := setupLayUp(t)
	ctx := context.Background()
	var cargo uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO units (world_id,owner_id,type,category,size,status,settlement_id) VALUES ($1,$2,'spearman','land',100,'embarked',$3) RETURNING id`, f.worldID, f.playerID, f.settlementID).Scan(&cargo); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET cargo_unit_id=$2 WHERE id=$1`, f.shipID, cargo); err != nil {
		t.Fatal(err)
	}
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cargo accepted: %d", got.Code)
	}
	_, _ = f.pool.Exec(ctx, `UPDATE units SET cargo_unit_id=NULL WHERE id=$1`, f.shipID)
	var messengerID uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers (world_id,sender_id,origin_id,destination_id,message_text,status,hex_q,hex_r,arrives_at,passage_status,carrier_unit_id)
		VALUES ($1,$2,$3,$3,'passenger','outbound',0,0,now(),'aboard',$4) RETURNING id`, f.worldID, f.playerID, f.settlementID, f.shipID).Scan(&messengerID); err != nil {
		t.Fatal(err)
	}
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "passengers") {
		t.Fatalf("passenger accepted: %d %s", got.Code, got.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM messengers WHERE id=$1`, messengerID); err != nil {
		t.Fatal(err)
	}
	_, _ = f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND building_type='harbour'`, f.settlementID)
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unharboured ship accepted: %d", got.Code)
	}
}

func TestFitOutRequiresFullCrewInOwnedPort(t *testing.T) {
	f := setupLayUp(t)
	ctx := context.Background()
	if lay, fit := f.portFlags(t); !lay || fit {
		t.Fatalf("active port flags lay=%v fit=%v", lay, fit)
	}
	if got := f.shipAction(t, "lay-up"); got.Code != http.StatusOK {
		t.Fatalf("lay up: %d %s", got.Code, got.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET population=19 WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if lay, fit := f.portFlags(t); lay || fit {
		t.Fatalf("undercrewed port flags lay=%v fit=%v", lay, fit)
	}
	if got := f.shipAction(t, "fit-out"); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "need 20 people") {
		t.Fatalf("undercrewed fit out: %d %s", got.Code, got.Body.String())
	}
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.shipID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "laid_up" {
		t.Fatalf("undercrewed ship status=%s", status)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET population=100, state='occupied' WHERE id=$1`, f.settlementID); err != nil {
		t.Fatal(err)
	}
	if got := f.shipAction(t, "fit-out"); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("occupied port fit out=%d", got.Code)
	}
}
