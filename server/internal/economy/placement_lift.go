package economy

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// LiftPlacementsAfterDemolish takes the gubbar off the places a demolished
// building provided. Call it INSIDE the demolishing transaction, after the
// buildings row is deleted. Lifted gubbar simply lose their settlement_placement
// row — the same thing UnplaceGubbe does — so they fall back to the pool and
// are never lost from the population. Nothing is re-placed.
//
//   - city workplace placements (target_kind='building') of that type go;
//   - for a rule building (farm/mine/lumbermill/stonequarry on hexQ/hexR, or
//     harbour city-wide) the hex placements of the goods the building enabled
//     are trimmed to what the hex now holds without it, highest gubbe ordinal
//     first (a farm's extra places go, the hex's base places stay; a mine's
//     silver workers all go because silver needs its mine).
//
// Returns the lifted ordinals, ascending. Caller runs RecomputeProduction.
func LiftPlacementsAfterDemolish(ctx context.Context, tx Tx, settlementID, buildingID uuid.UUID, buildingType string, hexQ, hexR *int) ([]int, error) {
	var lifted []int

	// A temple's workers stand at that one temple (building_id, mig 202);
	// demolishing one god's temple never lifts another god's workers.
	rows, err := tx.Query(ctx,
		`DELETE FROM settlement_placement
		 WHERE settlement_id = $1 AND target_kind = 'building' AND building_type = $2
		   AND (building_id IS NULL OR building_id = $3)
		 RETURNING gubbe_ordinal`,
		settlementID, buildingType, buildingID)
	if err != nil {
		return nil, fmt.Errorf("lift placements: building slots: %w", err)
	}
	for rows.Next() {
		var o int
		if err := rows.Scan(&o); err != nil {
			rows.Close()
			return nil, fmt.Errorf("lift placements: scan: %w", err)
		}
		lifted = append(lifted, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lift placements: rows: %w", err)
	}

	goods, err := RuleBuildingGoods(ctx, tx, settlementID, buildingType, hexQ, hexR)
	if err != nil {
		return nil, fmt.Errorf("lift placements: %w", err)
	}
	hexLifted, err := liftHexPlacementsToCapacity(ctx, tx, settlementID, goods, hexQ, hexR)
	if err != nil {
		return nil, err
	}
	lifted = append(lifted, hexLifted...)

	sort.Ints(lifted)
	return lifted, nil
}

// liftHexPlacementsToCapacity retains the lowest ordinals on each hex and
// frees only surplus workers. Demolition and fishing migration share this path;
// it never auto-places people in a different job.
func liftHexPlacementsToCapacity(ctx context.Context, tx Tx, settlementID uuid.UUID, goods []string, hexQ, hexR *int) ([]int, error) {
	var lifted []int
	if len(goods) > 0 {
		opts, err := LoadHexProductionOptions(ctx, tx, settlementID, nil)
		if err != nil {
			return nil, fmt.Errorf("lift placements: %w", err)
		}
		capAt := map[[2]int]map[string]int{}
		for _, o := range opts {
			capAt[[2]int{o.Coord.Q, o.Coord.R}] = o.PlaceCapPerGood
		}
		prows, err := tx.Query(ctx,
			`SELECT gubbe_ordinal, hex_q, hex_r, good_key FROM settlement_placement
			 WHERE settlement_id = $1 AND target_kind = 'hex' AND good_key = ANY($2::text[])
			   AND ($3::int IS NULL OR (hex_q = $3 AND hex_r = $4))
			 ORDER BY gubbe_ordinal`,
			settlementID, goods, hexQ, hexR)
		if err != nil {
			return nil, fmt.Errorf("lift placements: read hex placements: %w", err)
		}
		type key struct {
			q, r int
			good string
		}
		seen := map[key]int{}
		var drop []int
		for prows.Next() {
			var o, q, r int
			var good string
			if err := prows.Scan(&o, &q, &r, &good); err != nil {
				prows.Close()
				return nil, fmt.Errorf("lift placements: scan: %w", err)
			}
			k := key{q, r, good}
			seen[k]++
			if seen[k] > capAt[[2]int{q, r}][good] {
				drop = append(drop, o)
			}
		}
		prows.Close()
		if err := prows.Err(); err != nil {
			return nil, fmt.Errorf("lift placements: rows: %w", err)
		}
		for _, o := range drop {
			if _, err := tx.Exec(ctx,
				`DELETE FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = $2`,
				settlementID, o); err != nil {
				return nil, fmt.Errorf("lift placements: delete: %w", err)
			}
		}
		lifted = append(lifted, drop...)
	}
	return lifted, nil
}
