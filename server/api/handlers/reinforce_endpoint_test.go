package handlers

// DB integration tests for POST .../units/{id}/reinforce (mig 126,
// megaron_plan_rekryteringsmodell.md): the endpoint's own eligibility gate —
// status='garrison' AND settlement_id=origin_settlement_id AND size<100 —
// mirrored exactly by unitSummary.CanReinforce in unit.go so the web client's
// button never shows for a cohort the server would reject anyway. Reuses
// setupRecruitShipFixture (recruit_ship_test.go), which now also wires the
// reinforce route.

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// insertUnit is a minimal direct-SQL land unit seed for these eligibility
// tests — deliberately bypassing Recruit so each case can set exactly the
// (status, settlement_id, origin_settlement_id, size) combination it needs,
// including combinations Recruit itself would never produce (e.g. garrisoned
// away from origin, simulating a unit some future kingdom-loan feature moved).
func insertUnit(t *testing.T, f *recruitShipFixture, status string, settlementID, originID *uuid.UUID, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var unitID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, origin_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', $3, 0, $4, $5, $6)
		 RETURNING id`,
		f.worldID, f.playerID, size, status, settlementID, originID,
	).Scan(&unitID); err != nil {
		t.Fatalf("seed unit (status=%s size=%d): %v", status, size, err)
	}
	return unitID
}

func TestReinforce_AcceptedWhenGarrisonedAtOriginUnderFullStrength(t *testing.T) {
	f := setupRecruitShipFixture(t)
	unitID := insertUnit(t, f, "garrison", &f.settlementID, &f.settlementID, 62)

	rec, resp := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+unitID.String()+"/reinforce", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("Reinforce(eligible) = %d %q, want 200", rec.Code, rec.Body.String())
	}
	if resp["reinforcing"] != true {
		t.Errorf("response reinforcing = %v, want true", resp["reinforcing"])
	}

	var reinforcing bool
	if err := f.pool.QueryRow(context.Background(),
		`SELECT reinforcing FROM units WHERE id = $1`, unitID,
	).Scan(&reinforcing); err != nil {
		t.Fatalf("reload unit: %v", err)
	}
	if !reinforcing {
		t.Error("units.reinforcing = false after a successful Reinforce call, want true")
	}
}

func TestReinforce_RejectedWhenNotGarrison(t *testing.T) {
	f := setupRecruitShipFixture(t)
	// 'training' — mid-pipeline, not yet in garrison at all.
	unitID := insertUnit(t, f, "training", &f.settlementID, &f.settlementID, 100)

	rec, resp := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+unitID.String()+"/reinforce", map[string]any{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Reinforce(training) = %d %q, want 422", rec.Code, rec.Body.String())
	}
	if resp["error"] == nil {
		t.Error("expected an actionable error message, got none")
	}
}

func TestReinforce_RejectedWhenGarrisonedAwayFromOrigin(t *testing.T) {
	f := setupRecruitShipFixture(t)

	// A second settlement (simulates a cohort garrisoned somewhere other than
	// the city that recruited it — origin_settlement_id stays the FIRST city
	// forever, per mig 126, so this is exactly the shape a future kingdom-loan
	// would produce; reinforce must still refuse it today).
	var otherProvinceID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 1, 0, 'plains', false) RETURNING id`,
		f.worldID,
	).Scan(&otherProvinceID); err != nil {
		t.Fatalf("create second province: %v", err)
	}
	var otherSettlementID uuid.UUID
	if err := f.pool.QueryRow(context.Background(),
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Away City', 'minoan', $3, 'colony', false, 5000) RETURNING id`,
		f.worldID, otherProvinceID, f.playerID,
	).Scan(&otherSettlementID); err != nil {
		t.Fatalf("create second settlement: %v", err)
	}

	// Garrisoned at otherSettlementID, but origin is still the FIRST settlement.
	unitID := insertUnit(t, f, "garrison", &otherSettlementID, &f.settlementID, 62)

	rec, _ := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+unitID.String()+"/reinforce", map[string]any{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Reinforce(garrisoned away from origin) = %d %q, want 422", rec.Code, rec.Body.String())
	}
}

func TestReinforce_RejectedWhenAlreadyFullStrength(t *testing.T) {
	f := setupRecruitShipFixture(t)
	unitID := insertUnit(t, f, "garrison", &f.settlementID, &f.settlementID, 100)

	rec, _ := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+unitID.String()+"/reinforce", map[string]any{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Reinforce(size=100) = %d, want 422", rec.Code)
	}
}

func TestReinforce_RejectedOnNaval(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	var shipID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, origin_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3, $3) RETURNING id`,
		f.worldID, f.playerID, f.settlementID,
	).Scan(&shipID); err != nil {
		t.Fatalf("seed galley: %v", err)
	}

	rec, _ := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+shipID.String()+"/reinforce", map[string]any{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Reinforce(naval) = %d, want 422 (only land cohorts can be reinforced)", rec.Code)
	}
}
