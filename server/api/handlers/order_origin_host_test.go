package handlers

// Regression (Timothys fynd 2026-07-17): ordering a field unit while the
// founder host is ON THE MOVE dispatched the hemerodromos from where the host
// LAST stood still — a marching unit's stored (q,r) is its origin hex, updated
// only on arrival. The courier must depart from the host's CURRENT
// (interpolated) position instead.
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

func TestMarchCourier_OriginIsMarchingHostsCurrentPos(t *testing.T) {
	pool := unitLoadTestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
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
	username := "hostorigin-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// Plains strip (0,0)..(4,0) for the host's route, plus the spearman's hex
	// and its march target one step away.
	for q := 0; q <= 4; q++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, 0, 'plains')`,
			worldID, q,
		); err != nil {
			t.Fatalf("create tile (%d,0): %v", q, err)
		}
	}
	for _, qr := range [][2]int{{3, 1}, {4, 1}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, qr[0], qr[1],
		); err != nil {
			t.Fatalf("create tile (%d,%d): %v", qr[0], qr[1], err)
		}
	}

	// The founder host, MID-MARCH (0,0)→(4,0): stored q/r still (0,0), the
	// halfway interpolation puts it at (2,0). No settlements exist — the host
	// is the only possible order origin.
	now := time.Now()
	var hostID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status,
		                    q, r, target_q, target_r, departs_at, arrives_at)
		 VALUES ($1, $2, 'nomadic_host', 'land', 1, 'marching', 0, 0, 4, 0, $3, $4) RETURNING id`,
		worldID, playerID, now.Add(-30*time.Minute), now.Add(30*time.Minute),
	).Scan(&hostID); err != nil {
		t.Fatalf("create marching host: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO founder_phase (world_id, owner_id, host_unit_id, population,
		                            grain_amount, grain_rate, silver_amount, silver_rate)
		 VALUES ($1, $2, $3, 4000, 100, 0, 100, 0)`,
		worldID, playerID, hostID,
	); err != nil {
		t.Fatalf("create founder_phase: %v", err)
	}

	// The field spearman being ordered, and a target its own eye can see.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', 3, 1) RETURNING id`,
		worldID, playerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned spearman: %v", err)
	}

	clk := clock.NewTestClock(now)
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	body, _ := json.Marshal(map[string]any{"target_q": 4, "target_r": 1})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/march", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("March(field unit, founder phase) = %d %q, want 202", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status      string    `json:"status"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}
	if resp.Status != "order_dispatched" {
		t.Fatalf("status = %q, want order_dispatched", resp.Status)
	}

	var hexQ, hexR int
	if err := pool.QueryRow(ctx,
		`SELECT hex_q, hex_r FROM messengers WHERE id = $1`, resp.MessengerID,
	).Scan(&hexQ, &hexR); err != nil {
		t.Fatalf("read courier origin: %v", err)
	}
	if hexQ != 2 || hexR != 0 {
		t.Fatalf("courier departs from (%d,%d), want the host's interpolated mid-march position (2,0) — not its stale stored hex (0,0)",
			hexQ, hexR)
	}
}
