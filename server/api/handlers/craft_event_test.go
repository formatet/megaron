package handlers

// DB integration test for the GoodsCrafted event (2026-07-22). Craft used to
// mutate settlement_goods and return JSON with no trace at all: no event, no
// notification. That made casting bronze — the act the #1 gate hangs on —
// silent to the player, and unanswerable after the fact, since a stock of 0
// cannot distinguish "never cast" from "cast and spent" (the trap that broke
// the "craft-events >0" metric in rapport_bronsmatning_20260722.md).
//
// Real Postgres, gated by DATABASE_URL — same harness and leftover-world
// handling as recruit_ship_test.go (one_active_world partial unique index).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type craftFixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	provinceID   uuid.UUID
	settlementID uuid.UUID
	playerID     uuid.UUID
	accessToken  string
	router       *chi.Mux
}

// setupCraftFixture builds a world/player/settlement holding a foundry and
// enough copper+tin to cast bronze — the co-location that does not exist
// anywhere in the live world, which is why this path can only be exercised here.
func setupCraftFixture(t *testing.T) *craftFixture {
	t.Helper()
	pool := recruitShipTestPool(t)
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
	username := "founder-" + uuid.New().String()
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
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', false) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Foundrytown', 'minoan', $3, 'capital', true, 5000) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'foundry', 1)`,
		settlementID,
	); err != nil {
		t.Fatalf("create foundry: %v", err)
	}

	// Recipe 1 is bronze: 2 copper + 1 tin → 1 bronze.
	for good, amount := range map[string]float64{"copper": 100, "tin": 100} {
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
	hub := notify.New()
	hub.SetPool(pool) // without a pool the hub only broadcasts and persists nothing
	ph := NewProvinceHandler(pool, scheduler, clk, economy.SitosConfig{}, eventStore, hub)

	r := chi.NewRouter()
	r.Use(auth.Middleware(authSvc))
	r.Post("/worlds/{worldID}/provinces/{provinceID}/craft", ph.Craft)

	return &craftFixture{
		pool: pool, worldID: worldID, provinceID: provinceID, settlementID: settlementID,
		playerID: playerID, accessToken: accessToken, router: r,
	}
}

func (f *craftFixture) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	rf := &recruitShipFixture{accessToken: f.accessToken, router: f.router}
	rec, resp := rf.post(t, path, body)
	return rec.Code, resp
}

// TestCraft_EmitsGoodsCraftedEventAndNotification is the whole point: casting
// bronze must leave both an audit row in events and a notification the player
// can actually see, carrying what went IN as well as what came out.
func TestCraft_EmitsGoodsCraftedEventAndNotification(t *testing.T) {
	f := setupCraftFixture(t)
	ctx := context.Background()

	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/craft",
		map[string]any{"recipe_id": 1, "quantity": 3.0})
	if code != 200 {
		t.Fatalf("craft returned %d: %v", code, resp)
	}
	if resp["output_key"] != "bronze" {
		t.Fatalf("output_key = %v, want bronze", resp["output_key"])
	}
	if produced, _ := resp["produced"].(float64); produced != 3 {
		t.Fatalf("produced = %v, want 3", resp["produced"])
	}

	// The audit row, on the settlement's own stream.
	var payload []byte
	if err := f.pool.QueryRow(ctx,
		`SELECT payload FROM events
		  WHERE world_id = $1 AND stream_id = $2 AND event_type = 'GoodsCrafted'`,
		f.worldID, f.settlementID,
	).Scan(&payload); err != nil {
		t.Fatalf("no GoodsCrafted event appended: %v", err)
	}
	var ev struct {
		OutputKey string             `json:"output_key"`
		Produced  float64            `json:"produced"`
		Consumed  map[string]float64 `json:"consumed"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if ev.OutputKey != "bronze" || ev.Produced != 3 {
		t.Fatalf("event says %v %s, want 3 bronze", ev.Produced, ev.OutputKey)
	}
	// Outcome, not intention: the ingredients are what was actually deducted
	// for THIS quantity (2 copper + 1 tin per unit, ×3), not the recipe's
	// per-unit cost.
	if ev.Consumed["copper"] != 6 || ev.Consumed["tin"] != 3 {
		t.Fatalf("consumed = %v, want copper:6 tin:3", ev.Consumed)
	}

	// The notification the player sees.
	var kind string
	var body []byte
	if err := f.pool.QueryRow(ctx,
		`SELECT kind, body_json FROM notifications
		  WHERE world_id = $1 AND player_id = $2 AND kind = 'GoodsCrafted'`,
		f.worldID, f.playerID,
	).Scan(&kind, &body); err != nil {
		t.Fatalf("no GoodsCrafted notification persisted: %v", err)
	}
	var nb struct {
		OutputKey string             `json:"output_key"`
		Consumed  map[string]float64 `json:"consumed"`
	}
	if err := json.Unmarshal(body, &nb); err != nil {
		t.Fatalf("unmarshal notification body: %v", err)
	}
	if nb.OutputKey != "bronze" || nb.Consumed["copper"] != 6 {
		t.Fatalf("notification body = %s, want bronze from 6 copper", string(body))
	}

	// And the goods actually moved: 100−6 copper, 100−3 tin, 3 bronze.
	stock := map[string]float64{}
	rows, err := f.pool.Query(ctx,
		`SELECT good_key, amount FROM settlement_goods WHERE settlement_id = $1`, f.settlementID)
	if err != nil {
		t.Fatalf("read goods: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v float64
		if rows.Scan(&k, &v) == nil {
			stock[k] = v
		}
	}
	if stock["copper"] != 94 || stock["tin"] != 97 || stock["bronze"] != 3 {
		t.Fatalf("stock = %v, want copper:94 tin:97 bronze:3", stock)
	}
}

// A failed craft must leave no trace — the event says "X happened" or does not
// exist (CLAUDE.md, Fas 2.3). Insufficient tin means nothing was cast.
func TestCraft_InsufficientIngredientEmitsNoEvent(t *testing.T) {
	f := setupCraftFixture(t)
	ctx := context.Background()

	// 100 tin only covers 100 units; ask for 500.
	code, _ := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/craft",
		map[string]any{"recipe_id": 1, "quantity": 500.0})
	if code != 422 {
		t.Fatalf("craft returned %d, want 422", code)
	}

	var n int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE world_id = $1 AND event_type = 'GoodsCrafted'`,
		f.worldID,
	).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 0 {
		t.Fatalf("%d GoodsCrafted events after a failed craft, want 0", n)
	}
}
