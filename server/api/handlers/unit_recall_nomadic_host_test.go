package handlers

// Regression for buggrapport 70c1bfb3 (tick 184, formatet): "no Runner can
// catch this unit before it completes its march" against the nomadic host —
// but the order to the host IS Wanax's own order to their own body, since
// Wanax travels WITH it. unit.CommandedInPerson gates exactly this one unit
// type; the messenger pillar (command is never instant) stands unchanged for
// every other unit, including a field unit in the identical situation.
//
// Both units below march the SAME route with the SAME timing (5 minutes left
// of a 60-minute march, default tick = 60 minutes) — deliberately too little
// remaining time for any physically real courier to catch either unit (a
// courier needs at least one tick's wall time, 60 minutes, to travel even the
// shortest leg). That proves the nomadic host's redirect succeeds not because
// the fixture happens to leave enough slack for a courier, but because no
// courier is dispatched for it at all.
//
// DB integration test (real Postgres, gated by DATABASE_URL).

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
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type nomadicHostRecallFixture struct {
	worldID     uuid.UUID
	playerID    uuid.UUID
	accessToken string
	hostID      uuid.UUID
	spearmanID  uuid.UUID
	now         time.Time
}

// setupNomadicHostRecallWorld builds a world with NO settlements — the only
// possible order origin is the founder-phase host itself — and two units
// marching the identical (0,0)->(4,0) plains route with 5 minutes left of a
// 60-minute march: the nomadic host (also founder_phase.host_unit_id) and an
// ordinary spearman.
func setupNomadicHostRecallWorld(t *testing.T) (nomadicHostRecallFixture, *chi.Mux) {
	t.Helper()
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}

	var f nomadicHostRecallFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "nomadhost-recall-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	f.playerID = claims.PlayerID
	f.accessToken = accessToken

	// Route tiles (0,0)..(4,0), plus the redirect target (3,1) — one hex off
	// the route, within a marching land unit's own live-vision radius (2).
	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			f.worldID, q,
		); err != nil {
			t.Fatalf("create tile (%d,0): %v", q, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 3, 1, 'plains')`,
		f.worldID,
	); err != nil {
		t.Fatalf("create redirect target tile: %v", err)
	}

	f.now = time.Now()
	departsAt := f.now.Add(-55 * time.Minute)
	arrivesAt := f.now.Add(5 * time.Minute) // 60-minute march, 5 minutes left

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'nomadic_host', 'land', 1, 'marching', 0, 0, 4, 0, $3, $4) RETURNING id`,
		f.worldID, f.playerID, departsAt, arrivesAt,
	).Scan(&f.hostID); err != nil {
		t.Fatalf("create marching host: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO founder_phase (world_id, owner_id, host_unit_id, population,
		                            grain_amount, grain_rate, silver_amount, silver_rate)
		 VALUES ($1, $2, $3, 1000, 100, 0, 100, 0)`,
		f.worldID, f.playerID, f.hostID,
	); err != nil {
		t.Fatalf("create founder_phase: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'marching', 0, 0, 4, 0, $3, $4) RETURNING id`,
		f.worldID, f.playerID, departsAt, arrivesAt,
	).Scan(&f.spearmanID); err != nil {
		t.Fatalf("create marching spearman: %v", err)
	}

	clk := clock.NewTestClock(f.now)
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/recall", uh.Recall)
	return f, r
}

func TestRecall_NomadicHostAppliesWithoutRunner(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	f, router := setupNomadicHostRecallWorld(t)

	body, _ := json.Marshal(map[string]any{"target_q": 3, "target_r": 1})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.hostID.String()+"/recall", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Recall(nomadic_host, redirect) = %d %q, want 200 (applied directly, no Runner) — "+
			"the exact 5-minutes-left setup that must 422 an ordinary unit", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status  string `json:"status"`
		Verb    string `json:"verb"`
		TargetQ int    `json:"target_q"`
		TargetR int    `json:"target_r"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "order_applied" || resp.Verb != "redirect" {
		t.Fatalf("response = %+v, want status=order_applied verb=redirect", resp)
	}
	if resp.TargetQ != 3 || resp.TargetR != 1 {
		t.Fatalf("response target = (%d,%d), want (3,1)", resp.TargetQ, resp.TargetR)
	}

	// The unit itself must show the new course immediately — no waiting for a
	// courier to deliver it.
	var status string
	var targetQ, targetR int
	if err := pool.QueryRow(ctx,
		`SELECT status, target_q, target_r FROM units WHERE id = $1`, f.hostID,
	).Scan(&status, &targetQ, &targetR); err != nil {
		t.Fatalf("read host after redirect: %v", err)
	}
	if status != "marching" || targetQ != 3 || targetR != 1 {
		t.Fatalf("host after redirect: status=%s target=(%d,%d), want marching/(3,1)", status, targetQ, targetR)
	}

	// No courier was ever dispatched for this order — command really was
	// instant, not just fast.
	var courierCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM messengers WHERE world_id = $1 AND kind = 'order' AND origin_unit_id = $2`,
		f.worldID, f.hostID,
	).Scan(&courierCount); err != nil {
		t.Fatalf("count couriers: %v", err)
	}
	if courierCount != 0 {
		t.Fatalf("courier count = %d, want 0 — the host must not go through the Runner path at all", courierCount)
	}
}

func TestRecall_FieldUnitStillNeedsRunner_SameSetupAsHost(t *testing.T) {
	f, router := setupNomadicHostRecallWorld(t)

	body, _ := json.Marshal(map[string]any{"target_q": 3, "target_r": 1})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+f.worldID.String()+"/units/"+f.spearmanID.String()+"/recall", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Recall(spearman, redirect, 5 min left) = %d %q, want 422 — the messenger pillar "+
			"must still stand for every unit that is not the nomadic host", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == "" {
		t.Fatalf("empty error message on 422")
	}
}
