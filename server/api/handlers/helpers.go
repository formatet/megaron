package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write JSON response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// errInsufficientGoods is returned by deductGoods when stock is too low.
var errInsufficientGoods = errors.New("insufficient goods")

// deductGoods atomically deducts each good in costs from settlement_goods.
// Goods are deducted sequentially within the caller's DB pool (non-transactional).
// Call inside a transaction if you need full atomicity across goods.
// Returns errInsufficientGoods if any good lacks stock; returns a wrapped DB error otherwise.
func deductGoods(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID, costs map[string]float64) error {
	for key, qty := range costs {
		if qty <= 0 {
			continue
		}
		tag, err := pool.Exec(ctx,
			`UPDATE settlement_goods SET
			     amount  = amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate - $1,
			     calc_at = now()
			 WHERE settlement_id = $2 AND good_key = $3
			   AND amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate >= $1`,
			qty, settlementID, key,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errInsufficientGoods
		}
	}
	return nil
}
