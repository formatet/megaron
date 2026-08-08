package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TestGoods_WineProducibleOnPlainsOnlyCatchment is the HTTP-level half of
// AK2 (server/druvor-utanfor-hills, mig 103): GET .../goods must list wine
// as producible for a settlement whose entire 7-hex catchment is plains —
// no hills tile anywhere. This is exactly what `keryx goods` and
// `keryx build --list`'s underlying data reads (cmd_goods.go,
// cmd_allocate.go both call this same endpoint). Reuses p10TestPool
// (p10_gate_visibility_test.go, same package) for the DB connection.
func TestGoods_WineProducibleOnPlainsOnlyCatchment(t *testing.T) {
	pool := p10TestPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-wine-goods-%'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 100) RETURNING id`,
		"test-wine-goods-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"wine-goods-"+uuid.New().String(), "wine-goods-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	// Own hex + 6 neighbours, ALL plains — the "inland city, no hills in
	// reach" scenario the contract's player truth names.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, 0, 'plains')`,
		worldID,
	); err != nil {
		t.Fatalf("seed own-hex tile: %v", err)
	}
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, d[0], d[1],
		); err != nil {
			t.Fatalf("seed catchment tile: %v", err)
		}
	}

	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1, $2, 'Ampeloxoron', 'achaean', $3, 'capital', true, 100) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// P4: production follows PLACEMENT, not an auto-seeded weight — place a
	// gubbe on one of the seeded plains neighbour hexes before recomputing.
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
		 VALUES ($1, 1, 'hex', 1, 0, 'wine')`,
		settlementID,
	); err != nil {
		t.Fatalf("place wine gubbe: %v", err)
	}

	// RecomputeProduction is what actually writes the settlement_goods row
	// that the Goods handler reads — exactly what every real call site
	// (build, train, allocate, tick, ...) triggers before a Wanax ever looks
	// at GET .../goods.
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("RecomputeProduction: %v", err)
	}

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)
	r := chi.NewRouter()
	r.Get("/worlds/{worldID}/provinces/{provinceID}/goods", ph.Goods)

	req := httptest.NewRequest(http.MethodGet,
		"/worlds/"+worldID.String()+"/provinces/"+provinceID.String()+"/goods", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Goods = %d: %s", rec.Code, rec.Body.String())
	}

	var goods []struct {
		Key        string  `json:"key"`
		Rate       float64 `json:"rate_per_tick"`
		Producible bool    `json:"producible"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &goods); err != nil {
		t.Fatalf("parse Goods response: %v", err)
	}

	var wine *struct {
		Key        string  `json:"key"`
		Rate       float64 `json:"rate_per_tick"`
		Producible bool    `json:"producible"`
	}
	for i := range goods {
		if goods[i].Key == "wine" {
			wine = &goods[i]
			break
		}
	}
	if wine == nil {
		t.Fatal("no wine entry in GET .../goods for a plains-only-catchment settlement " +
			"— on unmigrated code (pre-103) wine never gets a settlement_goods row here at all")
	}
	if !wine.Producible {
		t.Errorf("wine.producible = false for a plains-only-catchment settlement, want true")
	}
	if wine.Rate <= 0 {
		t.Errorf("wine.rate_per_tick = %.6f for a plains-only-catchment settlement, want > 0", wine.Rate)
	}
}
