package handlers

// Regression test for Fas 1c: `status` (ProvinceHandler.Get) and `goods`
// (ProvinceHandler.Goods) showing different amounts for the same good at the
// same tick. Root cause: settled() extrapolates linearly with no ceiling
// (db/migrations/067_tick_substrate.up.sql — "amount + rate × ticks elapsed",
// no LEAST(..., cap)). Goods already clamped to cap in Go after scanning;
// Get's resSnap loop only floored at 0 (GREATEST(0, ...)), so a good whose
// calc_tick hadn't been refreshed in a while showed an ever-growing uncapped
// number in status while goods correctly reported it flat at cap.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func provinceParityTestPool(t *testing.T) *pgxpool.Pool {
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

func TestStatusGoodsParity_AmountClampedToCapInBoth(t *testing.T) {
	pool := provinceParityTestPool(t)
	ctx := context.Background()

	// settled() reads current_world_tick(), which requires a single active
	// world (one_active_world partial unique index) — see
	// unit_arrival_colonize_test.go for why leftovers must be archived first.
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-world-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 1000) RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"parity-"+uuid.New().String(), "parity-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Parityton', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// Timber sitting far below cap at calc_tick=0, with a rate that would blow
	// way past its cap (500) by the world's current tick (1000) if left
	// unclamped: 0 + 10*(1000-0) = 10000 >> 500.
	const cap = 500.0
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'timber', 0, 10, $2, 0)`,
		settlementID, cap,
	); err != nil {
		t.Fatalf("seed timber settlement_goods row: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	ph := NewProvinceHandler(pool, nil, clk, economy.SitosConfig{}, nil, nil)

	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	r.Get("/worlds/{worldID}/provinces/{provinceID}/goods", ph.Goods)

	getReq := httptest.NewRequest(http.MethodGet,
		"/worlds/"+worldID.String()+"/provinces/"+provinceID.String(), nil)
	getRec := httptest.NewRecorder()
	r.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("Get = %d: %s", getRec.Code, getRec.Body.String())
	}
	var getResp struct {
		Settlement struct {
			Resources map[string]struct {
				Amount float64 `json:"amount"`
			} `json:"resources"`
		} `json:"settlement"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("parse Get response: %v", err)
	}
	statusAmount := getResp.Settlement.Resources["timber"].Amount

	goodsReq := httptest.NewRequest(http.MethodGet,
		"/worlds/"+worldID.String()+"/provinces/"+provinceID.String()+"/goods", nil)
	goodsRec := httptest.NewRecorder()
	r.ServeHTTP(goodsRec, goodsReq)
	if goodsRec.Code != http.StatusOK {
		t.Fatalf("Goods = %d: %s", goodsRec.Code, goodsRec.Body.String())
	}
	var goodsResp []struct {
		Key    string  `json:"key"`
		Amount float64 `json:"amount"`
	}
	if err := json.Unmarshal(goodsRec.Body.Bytes(), &goodsResp); err != nil {
		t.Fatalf("parse Goods response: %v", err)
	}
	var goodsAmount float64
	found := false
	for _, g := range goodsResp {
		if g.Key == "timber" {
			goodsAmount = g.Amount
			found = true
		}
	}
	if !found {
		t.Fatalf("goods response has no timber row: %+v", goodsResp)
	}

	if statusAmount != cap {
		t.Errorf("status timber amount = %.1f, want %.1f (capped) — status is not clamping to cap", statusAmount, cap)
	}
	if goodsAmount != cap {
		t.Errorf("goods timber amount = %.1f, want %.1f (capped)", goodsAmount, cap)
	}
	if statusAmount != goodsAmount {
		t.Errorf("status/goods parity broken: status=%.1f goods=%.1f", statusAmount, goodsAmount)
	}
}
