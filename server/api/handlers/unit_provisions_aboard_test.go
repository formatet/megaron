package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// Haiku round 6: provision_days floors part of a day to 0, so a galley with its
// last ration aboard read as "out of food". provisions_aboard tells them apart.
func TestListUnits_ProvisionsAboardSeparatesLastRationFromEmpty(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), clk)
	f.router.Get("/worlds/{worldID}/units", uh.ListUnits)

	ration := combat.VoyageRation("galley", 1, "", 0, 10)
	ship := func(provisions float64) uuid.UUID {
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, provisions, provision_goods)
			 VALUES ($1, $2, 'galley', 'naval', 1, 10, 'garrison', $3, $4, jsonb_build_object('grain',$4::float8)) RETURNING id`,
			f.worldID, f.playerID, f.settlementID, provisions).Scan(&id); err != nil {
			t.Fatalf("seed galley: %v", err)
		}
		return id
	}
	lastRation, empty := ship(ration/2), ship(0)

	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /units = %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Units []struct {
			ID               uuid.UUID `json:"id"`
			ProvisionDays    int       `json:"provision_days"`
			ProvisionsAboard bool      `json:"provisions_aboard"`
		} `json:"units"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, u := range resp.Units {
		switch u.ID {
		case lastRation:
			seen++
			if u.ProvisionDays != 0 || !u.ProvisionsAboard {
				t.Errorf("half a ration aboard: days=%d aboard=%v, want 0 and true", u.ProvisionDays, u.ProvisionsAboard)
			}
		case empty:
			seen++
			if u.ProvisionsAboard {
				t.Errorf("empty galley reported provisions aboard")
			}
		}
	}
	if seen != 2 {
		t.Fatalf("listed %d of the 2 galleys", seen)
	}
}

func TestListUnitsUsesActualRowersForFoodAndVoyageDays(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), clk)
	f.router.Get("/worlds/{worldID}/units", uh.ListUnits)
	for _, kind := range []string{"galley", "war_galley", "merchantman"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,provisions,provision_goods)
 VALUES($1,$2,$3,'naval',1,2,'positioned',1,0,0.05,'{"fish":0.05}')`, f.worldID, f.playerID, kind); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("units %d %s", rec.Code, rec.Body.String())
	}
	var data struct {
		Units []struct {
			Crew int     `json:"crew"`
			Food float64 `json:"upkeep_grain_per_tick"`
			Days int     `json:"provision_days"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Units) != 3 {
		t.Fatalf("got %d units", len(data.Units))
	}
	for _, u := range data.Units {
		if u.Crew != 2 || u.Food != 0.02 || u.Days != 2 {
			t.Fatalf("two rowers, 0.05 aboard: %+v, want food0.02/day and2days", u)
		}
	}
}
