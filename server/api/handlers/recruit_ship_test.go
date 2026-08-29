package handlers

// DB integration tests for the ship-build overhaul (Timothy 2026-07-09,
// temenos_enheter.md "Flottdesign"): naval recruits build exactly one vessel
// with a name and a build ETA (not deployable until TrainComplete fires),
// land recruit keeps its existing 10-men-batch behaviour unchanged, and
// naval units reject a stance request. Real Postgres, gated by DATABASE_URL —
// see internal/combat/unit_arrival_colonize_test.go for why leftover active
// test worlds must be archived first (one_active_world partial unique index).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func recruitShipTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// recruitShipFixture sets up a world/player/settlement with the buildings and
// goods needed to recruit both a "ship" (naval) and a "spearman" (land) unit,
// and returns the handlers wired to a chi router with the routes this test
// exercises.
type recruitShipFixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	provinceID   uuid.UUID
	settlementID uuid.UUID
	playerID     uuid.UUID
	accessToken  string
	router       *chi.Mux
	trainH       *combat.TrainCompleteHandler
}

func setupRecruitShipFixture(t *testing.T) *recruitShipFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "shipwright-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
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
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Shipyard', 'minoan', $3, 'capital', true, 5000) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Buildings: shipyard (ship, per Slice A's harbour/shipyard split) +
	// harbour (kept — this fixture's ship tests only care about shipyard, but
	// harbour stays present so nothing here silently starts depending on its
	// absence) + barracks (spearman).
	for _, bt := range []string{"harbour", "shipyard", "barracks"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, $2, 1)`,
			settlementID, bt,
		); err != nil {
			t.Fatalf("create %s building: %v", bt, err)
		}
	}

	// Goods: enough for a "ship" (crew 20 × {timber:9, silver:0.3}) and a
	// "spearman" batch of 30 men (30 × {grain:3, silver:0.2}).
	goods := map[string]float64{
		"timber": 1000,
		"silver": 1000,
		"grain":  1000,
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

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, nil)
	uh := NewUnitHandler(pool, scheduler, eventStore, clk)
	trainH := combat.NewTrainCompleteHandler(pool, eventStore, nil)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/recruit", ph.Recruit)
	r.Get("/worlds/{worldID}/units", uh.ListUnits)
	r.Post("/worlds/{worldID}/units/{unitID}/stance", uh.SetStance)
	r.Post("/worlds/{worldID}/units/{unitID}/reinforce", uh.Reinforce)

	return &recruitShipFixture{
		pool: pool, worldID: worldID, provinceID: provinceID, settlementID: settlementID,
		playerID: playerID, accessToken: accessToken, router: r, trainH: trainH,
	}
}

func (f *recruitShipFixture) post(t *testing.T, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec, resp
}

// TestRecruitShip_NavalBuildsOneFormingVesselWithNameAndETA verifies (a) from
// the plan's success criteria: a naval recruit creates exactly one vessel in
// 'forming' status with a suggested name and a build ETA, it is not
// deployable until TrainComplete fires, and after that it flips to garrison
// and becomes deployable.
func TestRecruitShip_NavalBuildsOneFormingVesselWithNameAndETA(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()

	recruitPath := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"
	rec, resp := f.post(t, recruitPath, map[string]any{"unit_type": "galley"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("Recruit(galley) = %d %q, want 201", rec.Code, rec.Body.String())
	}

	unitIDs, _ := resp["unit_ids"].([]any)
	if len(unitIDs) != 1 {
		t.Fatalf("unit_ids = %v, want exactly 1 vessel", unitIDs)
	}
	unitIDStr, _ := unitIDs[0].(string)
	unitID, err := uuid.Parse(unitIDStr)
	if err != nil {
		t.Fatalf("parse unit id %q: %v", unitIDStr, err)
	}

	names, _ := resp["names"].([]any)
	if len(names) != 1 || names[0].(string) == "" {
		t.Errorf("response names = %v, want exactly one non-empty suggested name", resp["names"])
	}
	if resp["complete_at"] == nil || resp["complete_at"] == "" {
		t.Errorf("response complete_at missing — no build ETA returned")
	}

	// Row-level assertions: size always 1, name set, build_complete_at set,
	// status forming.
	var status, dbName string
	var size, crew int
	var buildCompleteAt *time.Time
	if err := f.pool.QueryRow(ctx,
		`SELECT status, size, crew, name, build_complete_at FROM units WHERE id = $1`, unitID,
	).Scan(&status, &size, &crew, &dbName, &buildCompleteAt); err != nil {
		t.Fatalf("load created ship: %v", err)
	}
	if status != "forming" {
		t.Errorf("status = %q, want forming (not instantly garrisoned)", status)
	}
	if size != 1 {
		t.Errorf("size = %d, want 1 (naval size is always 1 vessel)", size)
	}
	if crew != 20 {
		t.Errorf("crew = %d, want 20 (ship crew)", crew)
	}
	if dbName == "" {
		t.Error("name column empty — a suggested name should have been stored")
	}
	if buildCompleteAt == nil {
		t.Fatal("build_complete_at is nil — a still-forming ship must carry a build ETA")
	}

	// Deployable=false via the JSON summary while forming.
	getReq := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	getReq.Header.Set("Authorization", "Bearer "+f.accessToken)
	getRec := httptest.NewRecorder()
	f.router.ServeHTTP(getRec, getReq)
	var listBody struct {
		Units []struct {
			ID              string     `json:"id"`
			Deployable      bool       `json:"deployable"`
			Name            *string    `json:"name"`
			BuildCompleteAt *time.Time `json:"build_complete_at"`
			Status          string     `json:"status"`
		} `json:"units"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("parse unit list: %v", err)
	}
	var found bool
	for _, u := range listBody.Units {
		if u.ID != unitID.String() {
			continue
		}
		found = true
		if u.Deployable {
			t.Error("deployable = true while still forming, want false")
		}
		if u.Name == nil || *u.Name == "" {
			t.Error("unit summary name missing")
		}
		if u.BuildCompleteAt == nil {
			t.Error("unit summary build_complete_at missing")
		}
	}
	if !found {
		t.Fatalf("recruited ship %s not found in unit list", unitID)
	}

	// Fire TrainComplete directly (same pattern as calling a handler method
	// directly in unit_arrival_colonize_test.go) — flips forming→garrison and
	// clears the ETA.
	payload, _ := json.Marshal(combat.TrainCompletePayload{
		SettlementID: f.settlementID,
		UnitType:     "galley",
		Count:        1,
		UnitID:       unitID,
	})
	if err := f.trainH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("TrainComplete handle: %v", err)
	}

	var statusAfter string
	var buildCompleteAtAfter *time.Time
	if err := f.pool.QueryRow(ctx,
		`SELECT status, build_complete_at FROM units WHERE id = $1`, unitID,
	).Scan(&statusAfter, &buildCompleteAtAfter); err != nil {
		t.Fatalf("load ship after TrainComplete: %v", err)
	}
	if statusAfter != "garrison" {
		t.Errorf("status after TrainComplete = %q, want garrison", statusAfter)
	}
	if buildCompleteAtAfter != nil {
		t.Errorf("build_complete_at after TrainComplete = %v, want nil (cleared)", buildCompleteAtAfter)
	}

	// Idempotency (Fas 2.2): running the same event again must be a safe no-op.
	if err := f.trainH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("TrainComplete re-handle (idempotency): %v", err)
	}
}

// TestRecruit_LandAlwaysDraftsFullCohort verifies kohort-rekrytering
// (megaron_plan_rekryteringsmodell.md, Timothy 2026-08-19): a land recruit
// call ignores whatever `men` the caller sends and always drafts the full
// 100-man cohort in one shot, entering `training` immediately (not staying
// `forming` — there is no partial-batch gathering left on the happy path).
// origin_settlement_id is set to the recruiting settlement and support_settlement_id
// still matches it too. Was named TestRecruitShip_LandRecruitUnchanged before
// this plan; land recruiting is very much changed now, hence the rename.
func TestRecruit_LandAlwaysDraftsFullCohort(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()

	recruitPath := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"
	// men:30 is sent but must be ignored — one call always drafts 100.
	rec, resp := f.post(t, recruitPath, map[string]any{"unit_type": "spearman", "men": 30})
	if rec.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman, men=30) = %d %q, want 201", rec.Code, rec.Body.String())
	}
	unitIDs, _ := resp["unit_ids"].([]any)
	if len(unitIDs) != 1 {
		t.Fatalf("unit_ids = %v, want exactly 1 unit", unitIDs)
	}
	unitIDStr, _ := unitIDs[0].(string)
	unitID, err := uuid.Parse(unitIDStr)
	if err != nil {
		t.Fatalf("parse unit id %q: %v", unitIDStr, err)
	}

	var status, dbName *string
	var size int
	var buildCompleteAt *time.Time
	var originID *uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`SELECT status, size, name, build_complete_at, origin_settlement_id FROM units WHERE id = $1`, unitID,
	).Scan(&status, &size, &dbName, &buildCompleteAt, &originID); err != nil {
		t.Fatalf("load recruited spearman: %v", err)
	}
	if size != 100 {
		t.Errorf("size = %d, want 100 (men=30 must be ignored — every call drafts a full cohort)", size)
	}
	if status == nil || *status != "training" {
		t.Errorf("status = %v, want training (100/100 enters training immediately, no gathering phase left)", status)
	}
	if dbName != nil {
		t.Errorf("name = %q, want NULL for land units", *dbName)
	}
	if buildCompleteAt == nil {
		t.Error("build_complete_at = nil, want a ready ETA — training started at recruit time")
	}
	if originID == nil || *originID != f.settlementID {
		t.Errorf("origin_settlement_id = %v, want %v (set once at recruit)", originID, f.settlementID)
	}

	// A bogus --count on a land recruit must not multiply anything (still
	// ignored, unchanged from before) — and a second call is a fresh,
	// independent 100-man cohort, not a top-up of the first (which is
	// already in training, not forming).
	rec2, resp2 := f.post(t, recruitPath, map[string]any{"unit_type": "spearman", "men": 10, "count": 5})
	if rec2.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman, men=10, count=5) = %d %q, want 201", rec2.Code, rec2.Body.String())
	}
	if got := resp2["forming_size"]; got != float64(100) {
		t.Errorf("forming_size on second cohort = %v, want 100 (count ignored for land; a fresh full cohort)", got)
	}
}

// TestRecruit_LandLifecycle verifies the forming→training→garrison lifecycle
// (Timothy 2026-07-15, temenos_recruit_lifecycle_plan.md): a land unit gathers
// men as `forming` (< 100), enters `training` with exactly ONE TrainComplete
// when it reaches 100, is NOT deployable until that fires, then flips to
// `garrison`. An over-100 recruit caps the unit at 100 and spills the remainder
// into a fresh forming unit. No per-10-men batches.
func TestRecruit_LandLifecycle(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	recruitPath := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"

	// (1) A full 100-man recruit enters `training` (not `garrison`) with exactly
	// one scheduled TrainComplete and a ready ETA.
	rec, resp := f.post(t, recruitPath, map[string]any{"unit_type": "spearman", "men": 100})
	if rec.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman, men=100) = %d %q, want 201", rec.Code, rec.Body.String())
	}
	unitIDs, _ := resp["unit_ids"].([]any)
	trainingID, err := uuid.Parse(unitIDs[0].(string))
	if err != nil {
		t.Fatalf("parse unit id: %v", err)
	}
	var status string
	var size int
	var bca *time.Time
	if err := f.pool.QueryRow(ctx,
		`SELECT status, size, build_complete_at FROM units WHERE id=$1`, trainingID,
	).Scan(&status, &size, &bca); err != nil {
		t.Fatalf("load unit: %v", err)
	}
	if status != "training" {
		t.Errorf("status = %q, want training (full at 100, not yet deployable)", status)
	}
	if size != 100 {
		t.Errorf("size = %d, want 100", size)
	}
	if bca == nil {
		t.Error("build_complete_at nil — a training unit must carry a ready ETA")
	}
	var pending int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM scheduled_events WHERE event_type='TrainComplete'
		   AND processed_at IS NULL AND (payload->>'unit_id')::uuid=$1`, trainingID,
	).Scan(&pending); err != nil {
		t.Fatalf("count scheduled: %v", err)
	}
	if pending != 1 {
		t.Errorf("pending TrainComplete = %d, want exactly 1 (no per-10 batches)", pending)
	}

	// Not deployable while training.
	getReq := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/units", nil)
	getReq.Header.Set("Authorization", "Bearer "+f.accessToken)
	getRec := httptest.NewRecorder()
	f.router.ServeHTTP(getRec, getReq)
	var listBody struct {
		Units []struct {
			ID         string `json:"id"`
			Deployable bool   `json:"deployable"`
		} `json:"units"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("parse unit list: %v", err)
	}
	for _, u := range listBody.Units {
		if u.ID == trainingID.String() && u.Deployable {
			t.Error("deployable = true while training, want false")
		}
	}

	// Fire TrainComplete → garrison, ETA cleared.
	payload, _ := json.Marshal(combat.TrainCompletePayload{
		SettlementID: f.settlementID, UnitType: "spearman", Count: 100, UnitID: trainingID,
	})
	if err := f.trainH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatalf("TrainComplete handle: %v", err)
	}
	if err := f.pool.QueryRow(ctx,
		`SELECT status, build_complete_at FROM units WHERE id=$1`, trainingID,
	).Scan(&status, &bca); err != nil {
		t.Fatalf("reload after flip: %v", err)
	}
	if status != "garrison" {
		t.Errorf("status after TrainComplete = %q, want garrison", status)
	}
	if bca != nil {
		t.Errorf("build_complete_at after flip = %v, want nil (cleared)", bca)
	}

	// (2) Kohort-rekrytering (megaron_plan_rekryteringsmodell.md): a second
	// recruit call of the same type at the same settlement is an INDEPENDENT
	// fresh 100-man cohort, not accumulation into the first — the first
	// cohort is already in `training` (not `forming`), so there is nothing
	// left to top up on the normal happy path. (The old "recruit 80, then +40
	// spills 20 into a new forming unit" scenario is gone: no call can ever
	// leave a *fresh* cohort under 100 anymore. The top-up/spill CODE PATH
	// itself is still live for a legacy pre-mig-126 forming straggler — see
	// TestRecruit_LegacyFormingStragglerToppedUpAndSpills below.)
	if r, resp2 := f.post(t, recruitPath, map[string]any{"unit_type": "spearman"}); r.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman) second cohort = %d, want 201", r.Code)
	} else {
		unitIDs2, _ := resp2["unit_ids"].([]any)
		secondID, err := uuid.Parse(unitIDs2[0].(string))
		if err != nil {
			t.Fatalf("parse second unit id: %v", err)
		}
		if secondID == trainingID {
			t.Fatal("second recruit reused the first cohort's id — expected an independent cohort")
		}
		var status2 string
		var size2 int
		if err := f.pool.QueryRow(ctx,
			`SELECT status, size FROM units WHERE id=$1`, secondID,
		).Scan(&status2, &size2); err != nil {
			t.Fatalf("load second cohort: %v", err)
		}
		if status2 != "training" || size2 != 100 {
			t.Errorf("second cohort = (%s, %d), want (training, 100)", status2, size2)
		}
	}
}

// TestRecruit_LegacyFormingStragglerToppedUpAndSpills locks in that the
// pre-kohort top-up + spill branch in province.go Recruit (the existingUnitID
// path) still works correctly for a straggler 'forming' unit under 100 men —
// the kind mig 126's backfill would have left behind from before
// kohort-rekrytering. No live code path can create such a straggler anymore
// (every fresh recruit already delivers exactly 100), so this seeds one
// directly and proves the still-present branch doesn't silently rot: an
// 80-man straggler + a 100-man cohort's worth of new men caps at 100
// (training) and spills the remaining 80 into a fresh forming unit — the
// exact arithmetic the old batch-model test used to cover at the API surface.
func TestRecruit_LegacyFormingStragglerToppedUpAndSpills(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	recruitPath := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"

	var strayID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, origin_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 80, 0, 'forming', $3, $3, $3)
		 RETURNING id`,
		f.worldID, f.playerID, f.settlementID,
	).Scan(&strayID); err != nil {
		t.Fatalf("seed legacy straggler forming unit: %v", err)
	}

	if r, _ := f.post(t, recruitPath, map[string]any{"unit_type": "spearman"}); r.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman) on top of straggler = %d, want 201", r.Code)
	}

	var toppedStatus string
	var toppedSize int
	if err := f.pool.QueryRow(ctx,
		`SELECT status, size FROM units WHERE id=$1`, strayID,
	).Scan(&toppedStatus, &toppedSize); err != nil {
		t.Fatalf("load topped-up straggler: %v", err)
	}
	if toppedStatus != "training" || toppedSize != 100 {
		t.Errorf("topped-up straggler = (%s, %d), want (training, 100) — 80+100 caps at 100", toppedStatus, toppedSize)
	}

	var spillSize int
	if err := f.pool.QueryRow(ctx,
		`SELECT size FROM units WHERE settlement_id=$1 AND type='spearman' AND status='forming' AND id != $2`,
		f.settlementID, strayID,
	).Scan(&spillSize); err != nil {
		t.Fatalf("load spill forming unit: %v", err)
	}
	if spillSize != 80 {
		t.Errorf("spill forming size = %d, want 80 (80+100−100)", spillSize)
	}
}

// TestRecruit_TrainingStartedAlwaysTrueForLand verifies the forming-
// legibility response fields' truth AFTER kohort-rekrytering
// (megaron_plan_rekryteringsmodell.md): training_started/men_needed
// (2026-07-30) still exist on the response struct (province.go keeps the
// field-population code — it is exercised by the still-live legacy top-up
// branch, see TestRecruit_LegacyFormingStragglerToppedUpAndSpills), but on
// every reachable LAND recruit call today — fresh or on a legacy straggler —
// req.Men is forced to economy.MaxUnitSize, so newSize is always >= 100 and
// training_started is always true with men_needed always omitted. There is
// no more API-reachable path that returns training_started:false; this test
// documents that as the current truth rather than asserting the impossible
// old "still gathering" case. Naval still carries neither field.
func TestRecruit_TrainingStartedAlwaysTrueForLand(t *testing.T) {
	f := setupRecruitShipFixture(t)
	recruitPath := "/worlds/" + f.worldID.String() + "/provinces/" + f.provinceID.String() + "/recruit"

	rec, resp := f.post(t, recruitPath, map[string]any{"unit_type": "spearman"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("Recruit(spearman) = %d %q, want 201", rec.Code, rec.Body.String())
	}
	if got := resp["training_started"]; got != true {
		t.Errorf("training_started = %v, want true (one call always drafts the full 100)", got)
	}
	if _, present := resp["men_needed"]; present {
		t.Errorf("men_needed present = %v, want omitted (nothing left to report)", resp["men_needed"])
	}

	// Naval carries neither field — no size-based forming gate to report on.
	rec3, resp3 := f.post(t, recruitPath, map[string]any{"unit_type": "galley"})
	if rec3.Code != http.StatusCreated {
		t.Fatalf("Recruit(galley) = %d %q, want 201", rec3.Code, rec3.Body.String())
	}
	if _, present := resp3["training_started"]; present {
		t.Errorf("training_started present on naval response = %v, want omitted", resp3["training_started"])
	}
	if _, present := resp3["men_needed"]; present {
		t.Errorf("men_needed present on naval response = %v, want omitted", resp3["men_needed"])
	}
}

// TestRecruitShip_StanceRejectedOnNaval verifies SetStance 422s for a naval
// unit (ships carry no stance — Skepp-taxonomi, temenos_enheter.md).
func TestRecruitShip_StanceRejectedOnNaval(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()

	var shipID uuid.UUID
	if err := f.pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, name)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'garrison', $3, 'Test Vessel') RETURNING id`,
		f.worldID, f.playerID, f.settlementID,
	).Scan(&shipID); err != nil {
		t.Fatalf("create garrisoned ship: %v", err)
	}

	rec, resp := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+shipID.String()+"/stance",
		map[string]any{"stance": "fortify"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("SetStance(naval unit) = %d %q, want 422", rec.Code, rec.Body.String())
	}
	if resp["error"] == nil || resp["error"] == "" {
		t.Error("422 response missing an actionable error message")
	}
}
