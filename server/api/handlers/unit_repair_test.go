package handlers

// megaron_plan_skeppsreparation.md Slice C — Röd-före C: "ett skepp på hull 2
// i en varvstad med tillräckligt virke repareras över N ticks till hull 5;
// utan virke avvisas starten; i en stad utan varv repareras det aldrig."
// Before this slice, POST .../units/{id}/repair did not exist at all (404 —
// this file could not compile against krig/skeppsrep-slice-b, which has no
// UnitHandler.Repair). Real Postgres, gated by DATABASE_URL — same rig as
// recruit_shipyard_gate_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// shipRepairFixture is the common world/player/settlement/ship setup shared
// by the tests below. hasShipyard controls whether a shipyard building is
// seeded (the Slice A gate this slice's own gate sits behind).
type shipRepairFixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	playerID     uuid.UUID
	accessToken  string
	settlementID uuid.UUID
	shipID       uuid.UUID
	uh           *UnitHandler
	router       *chi.Mux
}

func newShipRepairFixture(t *testing.T, hasShipyard bool, goods map[string]float64) shipRepairFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 10) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "shiprepair-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population, state)
		 VALUES ($1, $2, 'Drydock', 'minoan', $3, 'capital', true, 5000, 'active') RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	if hasShipyard {
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'shipyard', 1)`,
			settlementID,
		); err != nil {
			t.Fatalf("create shipyard building: %v", err)
		}
	}

	for good, amount := range goods {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, $3, 0)`,
			settlementID, good, amount,
		); err != nil {
			t.Fatalf("seed %s: %v", good, err)
		}
	}

	var shipID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, hull, status, settlement_id, support_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 2, 'garrison', $3, $3) RETURNING id`,
		worldID, playerID, settlementID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create damaged galley: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	uh := NewUnitHandler(pool, scheduler, eventStore, clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/repair", uh.Repair)

	return shipRepairFixture{
		pool: pool, worldID: worldID, playerID: playerID, accessToken: accessToken,
		settlementID: settlementID, shipID: shipID, uh: uh, router: r,
	}
}

func (f shipRepairFixture) postRepair(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.shipID.String()+"/repair", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// TestRepair_StartsJobDeductsGoodsAndCompletesToFullHull is the Röd-före C
// scenario end to end: start → deduct → tick → complete.
func TestRepair_StartsJobDeductsGoodsAndCompletesToFullHull(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1000, "cedar": 1000, "silver": 1000})
	ctx := context.Background()

	rec := f.postRepair(t)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("Repair() = %d %q, want 202", rec.Code, rec.Body.String())
	}
	var resp struct {
		HullBefore    int     `json:"hull_before"`
		HullTarget    int     `json:"hull_target"`
		Good          string  `json:"good"`
		Amount        float64 `json:"amount"`
		DurationTicks int     `json:"duration_ticks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.HullBefore != 2 || resp.HullTarget != combat.HullMax {
		t.Errorf("hull_before/target = %d/%d, want 2/%d", resp.HullBefore, resp.HullTarget, combat.HullMax)
	}
	// galley: 9 timber/crew × 20 crew = 180 build cost; 3 hull points restored
	// (2→5) × 8%/point = 24% → 43.2 timber (combat.RepairCost pins the exact
	// formula; this just proves the handler actually charges it).
	wantGood, wantAmount, ok := combat.RepairCost("galley", combat.HullMax-2)
	if !ok {
		t.Fatalf("combat.RepairCost(galley, 3) not ok")
	}
	if resp.Good != wantGood || resp.Amount != wantAmount {
		t.Errorf("good/amount = %s/%v, want %s/%v", resp.Good, resp.Amount, wantGood, wantAmount)
	}
	if resp.DurationTicks != combat.RepairDurationTicks(combat.HullMax-2) {
		t.Errorf("duration_ticks = %d, want %d", resp.DurationTicks, combat.RepairDurationTicks(combat.HullMax-2))
	}

	// Unit flipped to repairing with an ETA, goods deducted.
	var status string
	var buildCompleteAt *time.Time
	var hull int
	if err := f.pool.QueryRow(ctx, `SELECT status, hull, build_complete_at FROM units WHERE id = $1`, f.shipID).
		Scan(&status, &hull, &buildCompleteAt); err != nil {
		t.Fatalf("read ship after start: %v", err)
	}
	if status != "repairing" {
		t.Errorf("status = %q, want repairing", status)
	}
	if hull != 2 {
		t.Errorf("hull should be untouched at job START (only set on completion), got %d", hull)
	}
	if buildCompleteAt == nil {
		t.Errorf("build_complete_at not set")
	}
	var timberLeft float64
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'timber'`,
		f.settlementID).Scan(&timberLeft); err != nil {
		t.Fatalf("read timber: %v", err)
	}
	if timberLeft != 1000-wantAmount {
		t.Errorf("timber left = %v, want %v", timberLeft, 1000-wantAmount)
	}

	// A repairing ship cannot start a second repair job (hull check would
	// also reject it once repaired, but right now it's the status gate).
	rec2 := f.postRepair(t)
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Errorf("second Repair() while already repairing = %d, want 422", rec2.Code)
	}

	// ── Completion: drive the scheduled event through the real handler ──
	repairH := combat.NewShipRepairCompleteHandler(f.pool, events.NewStore(f.pool), nil)
	payload, err := json.Marshal(combat.ShipRepairCompletePayload{
		UnitID: f.shipID, SettlementID: f.settlementID, WorldID: f.worldID, UnitType: "galley",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := repairH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("ShipRepairCompleteHandler.Handle: %v", err)
	}

	if err := f.pool.QueryRow(ctx, `SELECT status, hull, build_complete_at FROM units WHERE id = $1`, f.shipID).
		Scan(&status, &hull, &buildCompleteAt); err != nil {
		t.Fatalf("read ship after completion: %v", err)
	}
	if status != "garrison" {
		t.Errorf("status after completion = %q, want garrison", status)
	}
	if hull != combat.HullMax {
		t.Errorf("hull after completion = %d, want %d", hull, combat.HullMax)
	}
	if buildCompleteAt != nil {
		t.Errorf("build_complete_at after completion = %v, want nil", buildCompleteAt)
	}

	// Idempotent: running the SAME completion event again must not error and
	// must leave the ship exactly as it is (already garrison/HullMax) — the
	// WHERE status = 'repairing' guard makes the second run a safe no-op.
	if err := repairH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("ShipRepairCompleteHandler.Handle (retry): %v", err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT status, hull FROM units WHERE id = $1`, f.shipID).
		Scan(&status, &hull); err != nil {
		t.Fatalf("read ship after retry: %v", err)
	}
	if status != "garrison" || hull != combat.HullMax {
		t.Errorf("after retry: status/hull = %s/%d, want garrison/%d", status, hull, combat.HullMax)
	}
}

// TestRepair_RequiresShipyard: a settlement with no shipyard never repairs
// (Röd-före C, third clause).
func TestRepair_RequiresShipyard(t *testing.T) {
	f := newShipRepairFixture(t, false, map[string]float64{"timber": 1000, "silver": 1000})
	rec := f.postRepair(t)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Repair() without shipyard = %d %q, want 422", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "shipyard required" {
		t.Errorf(`error = %v, want "shipyard required"`, resp["error"])
	}
}

// TestRepair_InsufficientGoodsRejectsStart: no timber in stock — start is
// refused outright, no partial charge, no status flip (Röd-före C, second
// clause).
func TestRepair_InsufficientGoodsRejectsStart(t *testing.T) {
	f := newShipRepairFixture(t, true, map[string]float64{"timber": 1, "silver": 1000})
	ctx := context.Background()

	rec := f.postRepair(t)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Repair() with insufficient timber = %d %q, want 422", rec.Code, rec.Body.String())
	}

	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id = $1`, f.shipID).Scan(&status); err != nil {
		t.Fatalf("read ship: %v", err)
	}
	if status != "garrison" {
		t.Errorf("status = %q after a rejected repair start, want garrison (unchanged)", status)
	}
	var timberLeft float64
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'timber'`,
		f.settlementID).Scan(&timberLeft); err != nil {
		t.Fatalf("read timber: %v", err)
	}
	if timberLeft != 1 {
		t.Errorf("timber = %v after a rejected repair start, want 1 (untouched)", timberLeft)
	}
}
