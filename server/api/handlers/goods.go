package handlers

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TradeableGood is one entry in the handelsbara varukatalogen — a good a
// trade offer's want_good/offer_good may legally name. Key + Name are enough
// for a client <select>; Tier/Category ride along because the `goods` table
// already carries them and a dropdown can use them to group/label options
// without any new SQL (megaron_plan_offertens_varulista.md Steg 1).
type TradeableGood struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Tier     string `json:"tier"`
	Category string `json:"category"`
}

// tradeableGoodsCatalog is THE single source for "which goods may a trade
// offer name" — read by both GoodsHandler.TradeableCatalogue (the
// client-facing list this file exposes) and MessengerHandler.tradeableGood
// (messenger.go — the server-side validator that rejects an offer's
// want_good/offer_good before any silver or goods are escrowed). Two
// readers, one query, so the dropdown and the validation can never drift
// apart. Excludes silver (payment currency) and cult (temple labor, not a
// tradeable good — migration 094_cult_is_not_a_good) — see
// MessengerHandler.tradeableGood's doc comment for why those two are
// carved out there instead of here; this helper only owns the query.
func tradeableGoodsCatalog(ctx context.Context, pool *pgxpool.Pool) ([]TradeableGood, error) {
	rows, err := pool.Query(ctx,
		`SELECT key, name, tier, category FROM goods WHERE key NOT IN ('silver', 'cult') ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	goods := []TradeableGood{}
	for rows.Next() {
		var g TradeableGood
		if err := rows.Scan(&g.Key, &g.Name, &g.Tier, &g.Category); err == nil {
			goods = append(goods, g)
		}
	}
	return goods, nil
}

// GoodsHandler serves the static goods reference catalogue — same family as
// ProvinceHandler.BuildingCatalogue / UnitCatalogue / RecipeCatalogue
// (province.go): no auth, static data, world-agnostic (the `goods` table
// carries no world_id, so unlike settlement-scoped reads there is nothing
// per-world to key this on).
type GoodsHandler struct {
	pool *pgxpool.Pool
}

// NewGoodsHandler creates a GoodsHandler.
func NewGoodsHandler(pool *pgxpool.Pool) *GoodsHandler {
	return &GoodsHandler{pool: pool}
}

// TradeableCatalogue handles GET /api/v1/goods — the goods a trade offer may
// legally name. Deliberately NOT the same list as /provinces/{id}/goods
// (internal/economy's settlement stock — "what I have") or the settlement
// transfer dropdown (also stock-filtered): this is "what exists to be
// traded", the complement of MessengerHandler.tradeableGood's own query
// (messenger.go), read from the same helper so the two can never disagree.
func (h *GoodsHandler) TradeableCatalogue(w http.ResponseWriter, r *http.Request) {
	goods, err := tradeableGoodsCatalog(r.Context(), h.pool)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load goods catalogue")
		return
	}
	writeJSON(w, http.StatusOK, goods)
}
