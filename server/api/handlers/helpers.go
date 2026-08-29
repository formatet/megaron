package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// writeGoodsError emits a structured 422 with error_code "insufficient_goods"
// and a machine-parseable missing list so agents can act on the shortfall
// instead of retrying against an opaque prose string.
func writeGoodsError(w http.ResponseWriter, e *insufficientGoodsError) {
	type missingEntry struct {
		Good string  `json:"good"`
		Need float64 `json:"need"`
		Have float64 `json:"have"`
	}
	missing := make([]missingEntry, len(e.Short))
	for i, s := range e.Short {
		missing[i] = missingEntry{Good: s.Good, Need: s.Need, Have: s.Have}
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":   "insufficient_goods",
		"missing": missing,
	})
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

// shortfall renders a "need X, have Y" pair without hiding its own difference.
//
// Same lie as cli-sanning row D (megaron_plan_cli_sanning.md §D, fixed
// 2026-08-28 in insufficientKharisMessage): with %.0f on BOTH numbers a Wanax
// holding 11,7 of a required 12 reads "need 12, have 12" and cannot see why the
// gate refused him. The requirement keeps %.0f — build and recruit costs are
// whole numbers by construction — while the stock, which is a lazily evaluated
// float, gets one decimal plus the shortfall named outright, so the reader never
// has to subtract two rounded numbers to learn what is missing.
//
// Decimals appear only where they carry information: a whole 50 prints as "50",
// not "50.0". Sub-0,1 shortfalls fall back to two decimals rather than the
// self-contradictory "have 12.0, 0.0 short" — at that distance the exact figure
// IS the information.
// ⚠️ The precision is driven by the GAP, never by the stock figure on its own.
// The first version of this helper picked one decimal per value and promptly
// reintroduced the very lie it exists to remove: 4,97 against a required 5
// rendered as "have 5.0" and read as equal again. Its own test caught it.
func shortfall(need, have float64) string {
	diff := need - have
	prec := 1
	if diff < 0.1 {
		prec = 2 // the gap itself would round away to "0.0"
	}
	if have == math.Trunc(have) && diff == math.Trunc(diff) {
		prec = 0 // nothing fractional to show — "50" beats "50.0"
	}
	return fmt.Sprintf("need %s, have %.*f, %.*f short", trimNum(need), prec, have, prec, diff)
}

// trimNum prints a whole float bare and anything else at one decimal. Used for
// the requirement, which is a whole number by construction in every current
// caller; the stock and the gap get their precision from shortfall instead.
func trimNum(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func (e *insufficientGoodsError) Error() string {
	parts := make([]string, len(e.Short))
	for i, s := range e.Short {
		parts[i] = fmt.Sprintf("%s (%s)", s.Good, shortfall(s.Need, s.Have))
	}
	return "insufficient resources: " + strings.Join(parts, ", ")
}

// insufficientTradeMsg renders the shortfall when a messenger trade cannot
// settle, naming the party (buyer/seller), the good, and how much it holds —
// so the agent learns whether to decline, restock, or counter instead of
// retrying the same blind 422 forever (633 trade offers fired in playtest, most
// dying on a bare "seller has insufficient goods").
func insufficientTradeMsg(party, good string, need, have float64) string {
	return fmt.Sprintf("%s has insufficient %s (%s)", party, good, shortfall(need, have))
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
		{"spearman", want.Spearman, have.Spearman},
		{"war_chariot", want.WarChariot, have.WarChariot},
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

// deductGoods checks and deducts each good in costs from settlement_goods, using
// the caller's transaction tx. All goods are checked first: if ANY good lacks
// stock, nothing is deducted and an *insufficientGoodsError (listing every
// shortfall) is returned. The caller owns commit/rollback, so goods can be drained
// atomically together with silver/kharis/population — closing the partial-drain
// where goods were committed before a later currency deduction failed.
func deductGoods(ctx context.Context, tx pgx.Tx, settlementID uuid.UUID, costs map[string]float64) error {
	// Pass 1: lock the rows and check effective (lazy-evaluated) stock.
	var short []goodShortfall
	for key, qty := range costs {
		if qty <= 0 {
			continue
		}
		var have float64
		err := tx.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick)
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

	// Pass 2: every good is affordable — deduct them all (caller commits).
	for key, qty := range costs {
		if qty <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET
			     amount  = settled(amount, rate, calc_tick) - $1,
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = $3`,
			qty, settlementID, key,
		); err != nil {
			return err
		}
	}
	return nil
}
