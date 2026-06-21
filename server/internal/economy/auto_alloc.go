package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// oreSlice is the labor weight automatically granted to a newly unlocked ore good.
const oreSlice = 0.15

// AutoAllocateUnlocked assigns a starting labor slice (oreSlice) to each good in
// unlockedGoods that currently has weight=0 in settlement_labor.
// If there is idle capacity (sumW < 1.0) it is used first; the remainder is
// skimmed from grain only. If not enough capacity exists the good is skipped.
//
// Returns the slice of goods that were actually allocated (for notification text).
//
// Must be called inside an existing transaction. Passes ctx to every DB call.
func AutoAllocateUnlocked(ctx context.Context, tx Tx, settlementID uuid.UUID, unlockedGoods []string) ([]string, error) {
	if len(unlockedGoods) == 0 {
		return nil, nil
	}

	// Load current weights.
	rows, err := tx.Query(ctx,
		`SELECT good_key, weight FROM settlement_labor WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("auto_alloc: load weights: %w", err)
	}
	weights := make(map[string]float64)
	for rows.Next() {
		var k string
		var w float64
		if err := rows.Scan(&k, &w); err != nil {
			rows.Close()
			return nil, fmt.Errorf("auto_alloc: scan weight: %w", err)
		}
		weights[k] = w
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auto_alloc: weight rows err: %w", err)
	}

	// Compute sumW and grainW.
	var sumW float64
	for _, w := range weights {
		sumW += w
	}
	grainW := weights[GoodGrain]

	var allocated []string

	for _, g := range unlockedGoods {
		if g == GoodGrain {
			// Never skim grain to finance grain.
			continue
		}

		idle := 1.0 - sumW
		if idle < 0 {
			idle = 0
		}

		var actualSlice float64
		var skimFromGrain float64

		if idle >= oreSlice {
			// Enough idle capacity — use it directly.
			actualSlice = oreSlice
			skimFromGrain = 0
		} else {
			need := oreSlice - idle
			skim := grainW
			if skim > need {
				skim = need
			}
			actualSlice = idle + skim
			skimFromGrain = skim
		}

		if actualSlice <= 0 {
			// No room at all — leave this good unallocated (old idle-hint stays).
			continue
		}

		// Reduce grain if we skimmed from it.
		if skimFromGrain > 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE settlement_labor SET weight = weight - $1
				 WHERE settlement_id = $2 AND good_key = 'grain'`,
				skimFromGrain, settlementID,
			); err != nil {
				return nil, fmt.Errorf("auto_alloc: skim grain for %s: %w", g, err)
			}
			grainW -= skimFromGrain
			weights[GoodGrain] = grainW
		}

		// UPSERT the new good's weight.
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_labor (settlement_id, good_key, weight)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET weight = EXCLUDED.weight`,
			settlementID, g, actualSlice,
		); err != nil {
			return nil, fmt.Errorf("auto_alloc: upsert %s: %w", g, err)
		}

		sumW += actualSlice - skimFromGrain // net change to sumW
		weights[g] = actualSlice
		allocated = append(allocated, g)
	}

	return allocated, nil
}
