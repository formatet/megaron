package handlers

// Tests for PlacementRoster (report 19ed51f1, 2026-09-04): GET
// /worlds/{worldID}/settlements/placement-roster — a realm-wide roll of every
// placed gubbe grouped by settlement, so a Wanax can find idle citizens and
// see where each one works without opening every settlement one at a time.
//
//  1. TestPlacementRoster_OnlyOwnSettlements_AndCounts: two owned + one rival
//     settlement, each with placements. Only the caller's own come back
//     (FOW/ownership), and the placed/idle/count arithmetic is correct.
//  2. TestPlacementRosterParity_OrdinalsMatchPlacements: for a settlement's
//     hex placements, the roster's hex_ordinal matches what the established
//     /provinces/{id}/placements endpoint reports for the same hex — both call
//     hexgrid.RingOrdinal against the same centre, so a client never sees two
//     different numbers for one gubbe.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rosterSeedPlacement inserts one settlement_placement row directly (bypassing
// PlaceGubbe's FOW/capacity gates — this test exercises the read path, not the
// write path, so it needs no map_tiles or scouting rows).
func rosterSeedPlacement(t *testing.T, pool *pgxpool.Pool, ctx context.Context,
	settlementID uuid.UUID, ordinal int, kind, goodKey string, hexQ, hexR *int, buildingType *string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, building_type, good_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		settlementID, ordinal, kind, hexQ, hexR, buildingType, goodKey,
	); err != nil {
		t.Fatalf("seed placement #%d (%s/%s): %v", ordinal, kind, goodKey, err)
	}
}

func intPtr(n int) *int       { return &n }
func strPtr(s string) *string { return &s }

type rosterAssignJSON struct {
	GoodKey      string `json:"good_key"`
	TargetKind   string `json:"target_kind"`
	HexOrdinal   *int   `json:"hex_ordinal"`
	HexQ         *int   `json:"hex_q"`
	HexR         *int   `json:"hex_r"`
	BuildingType string `json:"building_type"`
	Count        int    `json:"count"`
}
type rosterSettJSON struct {
	ID          uuid.UUID          `json:"id"`
	ProvinceID  uuid.UUID          `json:"province_id"`
	Name        string             `json:"name"`
	IsCapital   bool               `json:"is_capital"`
	TotalGubbar int                `json:"total_gubbar"`
	Placed      int                `json:"placed"`
	Idle        int                `json:"idle"`
	Assignments []rosterAssignJSON `json:"assignments"`
}

func fetchRoster(t *testing.T, r chi.Router, worldID uuid.UUID, token string) []rosterSettJSON {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/settlements/placement-roster", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PlacementRoster = %d: %s", rec.Code, rec.Body.String())
	}
	var out []rosterSettJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse roster response: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

func TestPlacementRoster_OnlyOwnSettlements_AndCounts(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	worldID := settlementsOverviewTestWorld(t, pool, ctx)

	authSvc := auth.NewService(pool, "test-secret")
	ownerAID, tokenA := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-ra-"+uuid.New().String())
	ownerBID, _ := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-rb-"+uuid.New().String())

	// Capital at (0,0), colony at (10,0), rival at (20,0) — well apart so their
	// catchments never collide. Each seed settlement is population 800 → 8 gubbar.
	capID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerAID, "Knossos", true, 0, 0)
	colID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerAID, "Kommos", false, 10, 0)
	rivalID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerBID, "RivalCity", true, 20, 0)

	// Knossos: 3 grain on hex (-2,0), 1 fish on hex (-2,1), 1 stone in quarry → 5 placed, 3 idle.
	rosterSeedPlacement(t, pool, ctx, capID, 1, "hex", "grain", intPtr(-2), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, capID, 2, "hex", "grain", intPtr(-2), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, capID, 3, "hex", "grain", intPtr(-2), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, capID, 4, "hex", "fish", intPtr(-2), intPtr(1), nil)
	rosterSeedPlacement(t, pool, ctx, capID, 5, "building", "stone", nil, nil, strPtr("stonequarry"))
	// Kommos: 2 grain on hex (8,0) → 2 placed, 6 idle.
	rosterSeedPlacement(t, pool, ctx, colID, 1, "hex", "grain", intPtr(8), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, colID, 2, "hex", "grain", intPtr(8), intPtr(0), nil)
	// Rival: 4 grain — must never appear in ownerA's roster or its counts.
	rosterSeedPlacement(t, pool, ctx, rivalID, 1, "hex", "grain", intPtr(18), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, rivalID, 2, "hex", "grain", intPtr(18), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, rivalID, 3, "hex", "grain", intPtr(18), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, rivalID, 4, "hex", "grain", intPtr(18), intPtr(0), nil)

	sh := NewSettlementHandler(pool, nil, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{})
	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/settlements/placement-roster", sh.PlacementRoster)

	roster := fetchRoster(t, r, worldID, tokenA)

	if len(roster) != 2 {
		t.Fatalf("got %d settlements, want 2 (only ownerA's)", len(roster))
	}
	byID := map[uuid.UUID]rosterSettJSON{}
	for _, s := range roster {
		byID[s.ID] = s
		if s.Name == "RivalCity" {
			t.Fatalf("another Wanax's settlement leaked into the roster — ownership broken")
		}
	}

	cap := byID[capID]
	if cap.TotalGubbar != 8 || cap.Placed != 5 || cap.Idle != 3 {
		t.Errorf("Knossos totals = %d/%d placed, %d idle; want 8 total, 5 placed, 3 idle", cap.Placed, cap.TotalGubbar, cap.Idle)
	}
	// Assignments deterministic: building before hex, then by ordinal, then good.
	if len(cap.Assignments) != 3 {
		t.Fatalf("Knossos: got %d assignments, want 3: %+v", len(cap.Assignments), cap.Assignments)
	}
	if a := cap.Assignments[0]; a.TargetKind != "building" || a.BuildingType != "stonequarry" || a.GoodKey != "stone" || a.Count != 1 {
		t.Errorf("Knossos assignment[0] = %+v; want 1× stone in stonequarry (building)", a)
	}
	if a := cap.Assignments[1]; a.TargetKind != "hex" || a.GoodKey != "grain" || a.Count != 3 {
		t.Errorf("Knossos assignment[1] = %+v; want 3× grain on a hex", a)
	}
	if a := cap.Assignments[2]; a.TargetKind != "hex" || a.GoodKey != "fish" || a.Count != 1 {
		t.Errorf("Knossos assignment[2] = %+v; want 1× fish on a hex", a)
	}

	col := byID[colID]
	if col.TotalGubbar != 8 || col.Placed != 2 || col.Idle != 6 {
		t.Errorf("Kommos totals = %d/%d placed, %d idle; want 8 total, 2 placed, 6 idle", col.Placed, col.TotalGubbar, col.Idle)
	}
	if len(col.Assignments) != 1 || col.Assignments[0].Count != 2 {
		t.Errorf("Kommos assignments = %+v; want one row of 2× grain", col.Assignments)
	}
}

func TestPlacementRosterParity_OrdinalsMatchPlacements(t *testing.T) {
	pool := settlementsOverviewTestPool(t)
	ctx := context.Background()
	worldID := settlementsOverviewTestWorld(t, pool, ctx)

	authSvc := auth.NewService(pool, "test-secret")
	ownerID, token := settlementsOverviewRegisterPlayer(t, authSvc, ctx, "wanax-rp-"+uuid.New().String())
	settID := settlementsOverviewSeedSettlement(t, pool, ctx, worldID, ownerID, "Parityton", true, 0, 0)

	// Two hexes in the catchment ring, distinct ordinals.
	rosterSeedPlacement(t, pool, ctx, settID, 1, "hex", "grain", intPtr(-2), intPtr(0), nil)
	rosterSeedPlacement(t, pool, ctx, settID, 2, "hex", "fish", intPtr(-2), intPtr(2), nil)

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT province_id FROM settlements WHERE id = $1`, settID).Scan(&provinceID); err != nil {
		t.Fatalf("resolve province ID: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	sh := NewSettlementHandler(pool, nil, nil, clk, economy.SitosConfig{})
	ph := NewProvinceHandler(pool, nil, clk, economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/settlements/placement-roster", sh.PlacementRoster)
	r.With(auth.Middleware(authSvc)).Get("/worlds/{worldID}/provinces/{provinceID}/placements", ph.Placements)

	// Ordinals as the established Placements endpoint reports them, keyed by coord.
	plReq := httptest.NewRequest(http.MethodGet, "/worlds/"+worldID.String()+"/provinces/"+provinceID.String()+"/placements", nil)
	plReq.Header.Set("Authorization", "Bearer "+token)
	plRec := httptest.NewRecorder()
	r.ServeHTTP(plRec, plReq)
	if plRec.Code != http.StatusOK {
		t.Fatalf("Placements = %d: %s", plRec.Code, plRec.Body.String())
	}
	var pl struct {
		Placements []struct {
			HexQ       *int `json:"hex_q"`
			HexR       *int `json:"hex_r"`
			HexOrdinal *int `json:"hex_ordinal"`
		} `json:"placements"`
	}
	if err := json.Unmarshal(plRec.Body.Bytes(), &pl); err != nil {
		t.Fatalf("parse placements: %v", err)
	}
	type coord struct{ q, r int }
	placementsOrdinal := map[coord]int{}
	for _, p := range pl.Placements {
		if p.HexQ != nil && p.HexR != nil && p.HexOrdinal != nil {
			placementsOrdinal[coord{*p.HexQ, *p.HexR}] = *p.HexOrdinal
		}
	}
	if len(placementsOrdinal) != 2 {
		t.Fatalf("Placements resolved %d hex ordinals, want 2: %s", len(placementsOrdinal), plRec.Body.String())
	}

	roster := fetchRoster(t, r, worldID, token)
	var parityton *rosterSettJSON
	for i := range roster {
		if roster[i].ID == settID {
			parityton = &roster[i]
		}
	}
	if parityton == nil {
		t.Fatalf("Parityton missing from roster")
	}
	checked := 0
	for _, a := range parityton.Assignments {
		if a.TargetKind != "hex" || a.HexQ == nil || a.HexR == nil {
			continue
		}
		want, ok := placementsOrdinal[coord{*a.HexQ, *a.HexR}]
		if !ok {
			t.Errorf("roster hex (%d,%d) not present in Placements output", *a.HexQ, *a.HexR)
			continue
		}
		if a.HexOrdinal == nil || *a.HexOrdinal != want {
			t.Errorf("ordinal mismatch for hex (%d,%d): roster=%v placements=%d", *a.HexQ, *a.HexR, a.HexOrdinal, want)
		}
		checked++
	}
	if checked != 2 {
		t.Errorf("checked %d hex ordinals for parity, want 2", checked)
	}
}
