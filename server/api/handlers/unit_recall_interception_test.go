package handlers

// E2E proof for the moving-target interception fix (temenos_orderlopare_plan.md,
// 2026-07-30), through the real HTTP Recall handler + DB — companion to the
// pure-math tests in internal/messenger/intercept_test.go, which pin the same
// hand-worked numbers without a database.
//
// Fixture: a spearman 75% through an 8-hex (0,0)→(8,0), 6-hour plains march
// (departed 4.5h ago, arrives in 1.5h). Two capitals placed off the march line
// give two outcomes with the SAME unit/march geometry, differing only in where
// the Runner sets out from:
//   - capital at (8,3): 3 hexes (≈1 tick, 1h) from the march's destination
//     (8,0) — comfortably catchable there, though NOT at the dispatch-time
//     snapshot (6,0), which is 5 hexes away (≈2 ticks, 2h > the 1.5h
//     remaining). The OLD code aimed at that snapshot and would have missed.
//   - capital at (6,5): uniformly 5 hexes from every remaining hex on the
//     path, including the destination — no Runner can catch this unit at all;
//     the fix must reject at dispatch (422), not queue a doomed courier.
//
// DB integration tests (real Postgres, gated by DATABASE_URL).

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
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type interceptFixture struct {
	worldID     uuid.UUID
	playerID    uuid.UUID
	accessToken string
	unitID      uuid.UUID
	departsAt   time.Time
	arrivesAt   time.Time
}

// setupInterceptWorld builds the 8-hex-march fixture with the capital at
// capitalQ,capitalR (off the march line) — the only knob that distinguishes
// the catchable-ahead-of-snapshot case from the undeliverable case.
func setupInterceptWorld(t *testing.T, capitalQ, capitalR int) (interceptFixture, *chi.Mux) {
	t.Helper()
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var f interceptFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID) })

	authSvc := auth.NewService(pool, "test-secret")
	username := "intercept-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	f.playerID = claims.PlayerID
	f.accessToken = accessToken

	// Full connectivity grid: the march line (r=0, q 0..8) plus enough rows
	// above it to reach a capital placed off to the side.
	for q := 0; q <= 8; q++ {
		for r := 0; r <= 5; r++ {
			if _, err := pool.Exec(ctx,
				`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains') ON CONFLICT DO NOTHING`,
				f.worldID, q, r,
			); err != nil {
				t.Fatalf("create map_tiles(%d,%d): %v", q, r, err)
			}
		}
	}

	var capProvID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		f.worldID, capitalQ, capitalR,
	).Scan(&capProvID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		f.worldID, capProvID, f.playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// 75% through an 8-hex, 6h plains march ((0,0)→(8,0), 8×0.75h/hex).
	f.departsAt = time.Now().Add(-4*time.Hour - 30*time.Minute)
	f.arrivesAt = f.departsAt.Add(6 * time.Hour)
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 0, 0, 8, 0, $3, $4) RETURNING id`,
		f.worldID, f.playerID, f.departsAt, f.arrivesAt,
	).Scan(&f.unitID); err != nil {
		t.Fatalf("create marching unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)
	return f, r
}

// TestRecall_InterceptsAheadOfStaleSnapshot proves the fixed dispatch aims the
// Runner PAST the naive dispatch-time snapshot (6,0) — at (8,0), the only hex
// on the remaining path a courier from (8,3) can reach before the unit does —
// and that delivering the order there actually turns the still-marching unit.
func TestRecall_InterceptsAheadOfStaleSnapshot(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupInterceptWorld(t, 8, 3)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("Recall = %d %q, want 202 order_dispatched (a Runner from (8,3) can catch this unit at its destination)",
			rec.Code, rec.Body.String())
	}
	var resp struct {
		MessengerID     uuid.UUID `json:"messenger_id"`
		CourierArrivesAt time.Time `json:"courier_arrives_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}

	var destQ, destR int
	if err := pool.QueryRow(ctx, `SELECT dest_q, dest_r FROM messengers WHERE id = $1`, resp.MessengerID).
		Scan(&destQ, &destR); err != nil {
		t.Fatalf("read messenger dest: %v", err)
	}
	if destQ == 6 && destR == 0 {
		t.Fatalf("Runner aimed at the stale dispatch-time snapshot (6,0) — the old bug: a courier from "+
			"(8,3) cannot reach (6,0) (5 hexes, ≈2h) before the unit's 1.5h-remaining march completes")
	}
	if destQ != 8 || destR != 0 {
		t.Errorf("Runner aimed at (%d,%d), want (8,0) — the only hex on the remaining path a courier "+
			"from (8,3) can reach before the unit does", destQ, destR)
	}

	// The computed courier ETA must land BEFORE the unit's own arrival — this
	// is what actually guarantees delivery finds the unit still marching.
	if !resp.CourierArrivesAt.Before(f.arrivesAt) {
		t.Errorf("courier_arrives_at %v is not before the unit's arrival %v — delivery is not honestly guaranteed",
			resp.CourierArrivesAt, f.arrivesAt)
	}

	// Deliver: the unit — still marching — must actually be turned.
	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		resp.MessengerID,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	clk := clock.NewTestClock(time.Now())
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload}); err != nil {
		t.Fatalf("deliver recall order: %v", err)
	}

	var status string
	var targetQ, targetR int
	if err := pool.QueryRow(ctx, `SELECT status, target_q, target_r FROM units WHERE id = $1`, f.unitID).
		Scan(&status, &targetQ, &targetR); err != nil {
		t.Fatalf("read unit post-delivery: %v", err)
	}
	if status != "marching" {
		t.Errorf("status = %q, want marching (turning takes time too)", status)
	}
	if targetQ != 0 || targetR != 0 {
		t.Errorf("target = (%d,%d), want (0,0) — recall turns the unit home", targetQ, targetR)
	}
}

// TestRecall_UndeliverableFromFarCapital_FailsVisibly proves the invariant's
// clause (b): when no Runner can honestly catch the unit — courierOrigin
// (6,5) is 5 hexes (≈2h) from EVERY remaining hex on the path, including the
// destination, always more than the 1.5h left on the march — dispatch must
// reject visibly (422) rather than queue a courier already certain to arrive
// too late and vanish into a silent log line.
func TestRecall_UndeliverableFromFarCapital_FailsVisibly(t *testing.T) {
	f, router := setupInterceptWorld(t, 6, 5)

	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.unitID.String()+"/recall", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Recall = %d %q, want 422 (no Runner from (6,5) can catch this unit before it arrives) — "+
			"a doomed dispatch must fail now, visibly, not silently later", rec.Code, rec.Body.String())
	}
}
