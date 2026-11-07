package handlers

// Prayers of permanence (böner steg 4), Stout Hulls for ever: the ship's ceiling is its OWN
// (HullMax + hull_bonus). Repair restores to it, the unit view tells the owner (and only the
// owner), and the storm warning counts against it.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/unit"
)

func (f shipRepairFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}

// 4 of 6 is two points from full: the repair is priced and aimed at 6, and finishes at 6 (not 5) by both completion paths.
func TestPermanentHull_RepairRestoresToTheShipsOwnCeiling(t *testing.T) {
	for _, path := range []string{"legacy ShipRepairComplete", "shipyard work"} {
		t.Run(path, func(t *testing.T) {
			f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "cedar": 1000, "silver": 1000})
			ctx := context.Background()
			f.exec(t, `UPDATE units SET hull=4, hull_bonus=1 WHERE id=$1`, f.shipID)
			f.exec(t, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key) VALUES($1,1,'building','shipyard','shipbuilding')`, f.settlementID)
			rec := f.postRepair(t)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("Repair() = %d %q", rec.Code, rec.Body.String())
			}
			var resp struct {
				HullBefore int     `json:"hull_before"`
				HullTarget int     `json:"hull_target"`
				Amount     float64 `json:"amount"`
				Duration   int     `json:"duration_ticks"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			_, want, _ := combat.RepairCost("galley", 2)
			if resp.HullBefore != 4 || resp.HullTarget != 6 || resp.Amount != want || resp.Duration != combat.RepairDurationTicks(2) {
				t.Fatalf("repair 4->6 should cost two points: %+v (want amount %v)", resp, want)
			}
			if path == "legacy ShipRepairComplete" {
				payload, _ := json.Marshal(combat.ShipRepairCompletePayload{UnitID: f.shipID, SettlementID: f.settlementID, WorldID: f.worldID, UnitType: "galley"})
				if err := combat.NewShipRepairCompleteHandler(f.pool, events.NewStore(f.pool), nil).Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
					t.Fatal(err)
				}
			} else {
				var payload json.RawMessage
				if err := f.pool.QueryRow(ctx, `SELECT payload FROM scheduled_events WHERE event_type='ShipyardWorkComplete' AND payload->>'unit_id'=$1 AND processed_at IS NULL`, f.shipID.String()).Scan(&payload); err != nil {
					t.Fatal(err)
				}
				f.exec(t, `UPDATE worlds SET current_tick=current_tick+20 WHERE id=$1`, f.worldID)
				if err := combat.NewShipyardWorkCompleteHandler(f.pool, events.NewStore(f.pool), nil).Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
					t.Fatal(err)
				}
			}
			var status string
			var hull int
			_ = f.pool.QueryRow(ctx, `SELECT status,hull FROM units WHERE id=$1`, f.shipID).Scan(&status, &hull)
			if status != "garrison" || hull != 6 {
				t.Fatalf("after repair: %s hull %d, want garrison 6 (not 5)", status, hull)
			}
			var body []byte
			if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE stream_id=$1 AND event_type='ShipRepaired'`, f.shipID).Scan(&body); err != nil || !strings.Contains(string(body), `"hull": 6`) && !strings.Contains(string(body), `"hull":6`) {
				t.Fatalf("ShipRepaired should say hull 6: %s %v", body, err)
			}
		})
	}
}

// A full 6/6 ship needs no repair; a 5/6 ship does (one point); an ordinary 5/5 is full as before.
func TestPermanentHull_FullMeansTheShipsOwnCeiling(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "cedar": 1000, "silver": 1000})
	f.exec(t, `UPDATE units SET hull=6, hull_bonus=1 WHERE id=$1`, f.shipID)
	if rec := f.postRepair(t); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "full strength") {
		t.Fatalf("a 6/6 ship: %d %s, want 422 full strength", rec.Code, rec.Body.String())
	}
	f.exec(t, `UPDATE units SET hull=5 WHERE id=$1`, f.shipID)
	if rec := f.postRepair(t); rec.Code != http.StatusAccepted {
		t.Fatalf("a 5/6 ship is one point short: %d %s", rec.Code, rec.Body.String())
	}
}

// The owner's own unit view: hull 6/6 and the blessings, in the server's words.
func TestPermanentBlessings_TheOwnersUnitViewShowsThem(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000})
	f.router.Get("/worlds/{worldID}/units", f.uh.ListUnits)
	f.exec(t, `UPDATE units SET hull=6, hull_bonus=1 WHERE id=$1`, f.shipID)
	f.exec(t, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,from_tick,until_tick,permanent) VALUES($1,$2,$3,'unit',$4,0,$5,true)`,
		f.worldID, f.playerID, religion.EffectBlessedSpeedSea, f.shipID, religion.PermanentUntil)
	var troop string
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
		VALUES ($1,$2,'spearman','land',50,0,'garrison',$3,$3) RETURNING id`, f.worldID, f.playerID, f.settlementID).Scan(&troop); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,unit_id,from_tick,until_tick,permanent) VALUES($1,$2,$3,'unit',$4,0,$5,true)`,
		f.worldID, f.playerID, religion.EffectBlessedStrength, troop, religion.PermanentUntil)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Units []struct {
			ID        string   `json:"id"`
			Hull      int      `json:"hull"`
			HullMax   int      `json:"hull_max"`
			Blessings []string `json:"blessings"`
		} `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, u := range resp.Units {
		switch u.ID {
		case f.shipID.String():
			seen++
			if u.Hull != 6 || u.HullMax != 6 || strings.Join(u.Blessings, "|") != "Stout Hulls for ever|followed by the Sea Daimons, for ever" {
				t.Fatalf("ship view: hull %d/%d blessings %v", u.Hull, u.HullMax, u.Blessings)
			}
		case troop:
			seen++
			if len(u.Blessings) != 1 || u.Blessings[0] != "blessed by Velchanos, for ever" || u.HullMax != 0 {
				t.Fatalf("troop view: %+v", u)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("expected both units in the owner's view, saw %d: %s", seen, rec.Body.String())
	}
}

// What other Wanaxer see of a unit has no hull and no blessing.
func TestPermanentBlessings_ForeignViewCarriesNone(t *testing.T) {
	typ := reflect.TypeOf(foreignUnit{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		for _, banned := range []string{"hull", "blessing", "forever"} {
			if strings.Contains(tag, banned) {
				t.Fatalf("foreignUnit.%s (%s) would show another Wanax a hull or a blessing", typ.Field(i).Name, tag)
			}
		}
	}
}

// The storm warning counts against the ship's own ceiling.
func TestPermanentHull_StormWarningNamesTheShipsCeiling(t *testing.T) {
	w := stormHullWarning(&unit.Unit{Type: unit.TypeGalley, Hull: 2, HullBonus: 1})
	if !strings.Contains(w, "Hull 2 of 6") {
		t.Fatalf("warning %q should read 'Hull 2 of 6'", w)
	}
	if got := stormShipWarning("galley", 2, 1); !strings.Contains(got, "of 6") {
		t.Fatalf("stormShipWarning with a bonus: %q", got)
	}
	if got := stormShipWarning("galley", 2, 0); !strings.Contains(got, "of 5") {
		t.Fatalf("stormShipWarning ordinary: %q", got)
	}
}
