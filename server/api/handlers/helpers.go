package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/province"
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

// goodShortfall reports one good the settlement cannot afford.
type goodShortfall struct {
	Good string  `json:"good"`
	Need float64 `json:"need"`
	Have float64 `json:"have"`
}

// insufficientGoodsError lists every good that fell short, so the API can tell
// the caller exactly what to acquire (or trade for) instead of a blind 422.
type insufficientGoodsError struct {
	Short []goodShortfall
}

func (e *insufficientGoodsError) Error() string {
	parts := make([]string, len(e.Short))
	for i, s := range e.Short {
		parts[i] = fmt.Sprintf("%s (need %.0f, have %.0f)", s.Good, s.Need, s.Have)
	}
	return "insufficient resources: " + strings.Join(parts, ", ")
}

// insufficientTradeMsg renders the shortfall when a messenger trade cannot
// settle, naming the party (buyer/seller), the good, and how much it holds —
// so the agent learns whether to decline, restock, or counter instead of
// retrying the same blind 422 forever (633 trade offers fired in playtest, most
// dying on a bare "seller has insufficient goods").
func insufficientTradeMsg(party, good string, need, have float64) string {
	return fmt.Sprintf("%s has insufficient %s (need %.0f, have %.0f)", party, good, need, have)
}

// insufficientUnitsMsg compares the army a caller tried to send (want) against
// what the settlement actually holds (have) and lists every unit type that fell
// short, so a blind "insufficient units" 422 becomes actionable — the caller
// sees exactly which units it lacks and by how much (e.g. when an agent tries to
// outpost with more troops than its fresh garrison holds). Unit keys are the
// wire names the caller sends, so the message is machine-parseable.
func insufficientUnitsMsg(want, have province.ArmyComposition) string {
	units := []struct {
		name string
		w, h int
	}{
		{"infantry", want.Infantry, have.Infantry},
		{"chariot", want.Chariot, have.Chariot},
		{"priest", want.Priest, have.Priest},
		{"ship", want.Ship, have.Ship},
		{"elite_infantry", want.EliteInfantry, have.EliteInfantry},
		{"war_galley", want.WarGalley, have.WarGalley},
		{"merchantman", want.Merchantman, have.Merchantman},
	}
	var parts []string
	for _, u := range units {
		if u.w > u.h {
			parts = append(parts, fmt.Sprintf("%s (need %d, have %d)", u.name, u.w, u.h))
		}
	}
	if len(parts) == 0 {
		return "insufficient units"
	}
	return "insufficient units: " + strings.Join(parts, ", ")
}

// deductGoods atomically deducts each good in costs from settlement_goods.
// All goods are checked and deducted inside one transaction: if ANY good lacks
// stock, nothing is deducted and an *insufficientGoodsError (listing every
// shortfall) is returned. This prevents the silent partial-drain that happened
// when goods were deducted one-by-one on the pool and a later good failed.
func deductGoods(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID, costs map[string]float64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Pass 1: lock the rows and check effective (lazy-evaluated) stock.
	var short []goodShortfall
	for key, qty := range costs {
		if qty <= 0 {
			continue
		}
		var have float64
		err := tx.QueryRow(ctx,
			`SELECT amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate
			   FROM settlement_goods
			  WHERE settlement_id = $1 AND good_key = $2
			  FOR UPDATE`,
			settlementID, key,
		).Scan(&have)
		if err == pgx.ErrNoRows {
			have = 0 // settlement has never held this good
		} else if err != nil {
			return err
		}
		if have < qty {
			short = append(short, goodShortfall{Good: key, Need: qty, Have: have})
		}
	}
	if len(short) > 0 {
		sort.Slice(short, func(i, j int) bool { return short[i].Good < short[j].Good })
		return &insufficientGoodsError{Short: short}
	}

	// Pass 2: every good is affordable — deduct them all and commit.
	for key, qty := range costs {
		if qty <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET
			     amount  = amount + EXTRACT(EPOCH FROM (now() - calc_at))/60 * rate - $1,
			     calc_at = now()
			 WHERE settlement_id = $2 AND good_key = $3`,
			qty, settlementID, key,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
