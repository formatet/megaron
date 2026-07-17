package handlers

// Fas 2f regression test: a field-positioned land unit may colonize the empty
// hex it already occupies (target == origin), without marching one hex out and
// back. Before the fix the March handler rejected target == origin outright
// ("cannot march to own hex") for every intent, so a positioned colonist could
// never settle where it stood. The fix special-cases intent=colonize on a
// positioned unit's own hex, bypassing FindPath (a zero-distance query) and
// scheduling an immediate arrival.
//
// DB integration test (real Postgres, gated by DATABASE_URL) through a real
// chi.Mux with the prod route pattern ({unitID}).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
)

func TestMarch_ColonizeInPlace_PositionedUnitFoundsOnOwnHex(t *testing.T) {
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
	username := "settler-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, username+"@test.invalid", "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}
	playerID := claims.PlayerID

	// A capital, so the player is under (not at) the settlement cap.
	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital', 'achaean', $3, 'capital', true)`,
		worldID, capitalProvinceID, playerID,
	); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// The empty hex the colonist stands on. March queries map_tiles for the
	// target's terrain, so the tile must exist and be passable land.
	const hexQ, hexR = 3, -1
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
		worldID, hexQ, hexR,
	); err != nil {
		t.Fatalf("create target map tile: %v", err)
	}

	// A field-positioned, full-strength land unit standing on that hex.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, q, r)
		 VALUES ($1, $2, 'spearman', 'land', 100, 'positioned', $3, $4) RETURNING id`,
		worldID, playerID, hexQ, hexR,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned colonist: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	body, _ := json.Marshal(map[string]any{
		"target_q": hexQ,
		"target_r": hexR,
		"intent":   "colonize",
		"name":     "Insitu",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/worlds/"+worldID.String()+"/units/"+unitID.String()+"/march", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("March(colonize own hex) = %d %q, want 202 (a positioned unit must be able to colonize the hex it stands on)",
			rec.Code, rec.Body.String())
	}

	// Order latency (temenos_orderlopare_plan.md Fas 2): the colonist stands in
	// the field, so the order travels by hemerodromos from the nearest own city
	// and executes only on delivery. The 202 is a dispatch receipt; deliver the
	// courier to get the march started.
	var dispatchResp struct {
		Status      string    `json:"status"`
		MessengerID uuid.UUID `json:"messenger_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dispatchResp); err != nil {
		t.Fatalf("decode dispatch response: %v", err)
	}
	if dispatchResp.Status != "order_dispatched" {
		t.Fatalf("dispatch status = %q, want order_dispatched (field unit orders travel by runner)", dispatchResp.Status)
	}
	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM scheduled_events
		 WHERE event_type = 'OrderDelivery' AND (payload->>'messenger_id')::uuid = $1
		   AND processed_at IS NULL AND failed_at IS NULL`,
		dispatchResp.MessengerID,
	).Scan(&rawPayload); err != nil {
		t.Fatalf("load scheduled OrderDelivery: %v", err)
	}
	odh := messenger.NewOrderDeliveryHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := odh.Handle(ctx, events.ScheduledEvent{Payload: rawPayload}); err != nil {
		t.Fatalf("deliver order: %v", err)
	}

	var intent, status string
	var colonyName *string
	if err := pool.QueryRow(ctx,
		`SELECT march_intent, status, colony_name FROM units WHERE id = $1`, unitID,
	).Scan(&intent, &status, &colonyName); err != nil {
		t.Fatalf("read unit after march: %v", err)
	}
	if intent != "colonize" {
		t.Errorf("unit.march_intent = %q, want colonize", intent)
	}
	if status != "marching" {
		t.Errorf("unit.status = %q, want marching (settles into a colony on the next tick)", status)
	}
	if colonyName == nil || *colonyName != "Insitu" {
		t.Errorf("unit.colony_name = %v, want Insitu", colonyName)
	}
}
