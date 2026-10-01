package economy

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// ruleBuildingsWithoutWorkplace are the hex-bound buildings that used to have a
// workplace inside the building (BuildingOption placements) and do not any more
// (byggnadsregeln, mig 155): they work their hex instead.
var ruleBuildingsWithoutWorkplace = []string{"mine", "stonequarry", "lumbermill"}

// ReconcilePlacements brings a settlement's placements in line with the
// byggnadsregeln after mig 155 (run by cmd/recompute-all BEFORE
// RecomputeProduction; idempotent, a no-op on a settlement already consistent):
//
//  1. placements INSIDE a mine / stonequarry / lumbermill are invalid and deleted;
//  2. a hex placement count above the hex's places (e.g. grain on a level-2 farm
//     hex, 10 -> 8) is trimmed, highest gubbe ordinal first;
//  3. every freed gubbe is re-placed on the best food slot with room
//     (PlaceNextGubbeOnBestFoodHex, same ordinal); one that finds no room falls
//     to the pool.
//
// Returns how many placements were removed and how many of those found a new place.
func ReconcilePlacements(ctx context.Context, tx Tx, settlementID uuid.UUID) (removed, replaced int, err error) {
	var freed []int

	rows, err := tx.Query(ctx,
		`DELETE FROM settlement_placement
		 WHERE settlement_id = $1 AND target_kind = 'building' AND building_type = ANY($2::text[])
		 RETURNING gubbe_ordinal`,
		settlementID, ruleBuildingsWithoutWorkplace)
	if err != nil {
		return 0, 0, fmt.Errorf("reconcile placements: delete building placements: %w", err)
	}
	for rows.Next() {
		var o int
		if err := rows.Scan(&o); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("reconcile placements: scan: %w", err)
		}
		freed = append(freed, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("reconcile placements: rows: %w", err)
	}

	hexOptions, err := LoadHexProductionOptions(ctx, tx, settlementID, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("reconcile placements: %w", err)
	}
	for _, opt := range hexOptions {
		for good, places := range opt.PlaceCapPerGood {
			prows, err := tx.Query(ctx,
				`SELECT gubbe_ordinal FROM settlement_placement
				 WHERE settlement_id = $1 AND target_kind = 'hex' AND hex_q = $2 AND hex_r = $3 AND good_key = $4
				 ORDER BY gubbe_ordinal`,
				settlementID, opt.Coord.Q, opt.Coord.R, good)
			if err != nil {
				return 0, 0, fmt.Errorf("reconcile placements: read hex placements: %w", err)
			}
			var ords []int
			for prows.Next() {
				var o int
				if err := prows.Scan(&o); err != nil {
					prows.Close()
					return 0, 0, fmt.Errorf("reconcile placements: scan: %w", err)
				}
				ords = append(ords, o)
			}
			prows.Close()
			if len(ords) <= places {
				continue
			}
			for _, o := range ords[places:] { // highest ordinals first to go
				if _, err := tx.Exec(ctx,
					`DELETE FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = $2`,
					settlementID, o); err != nil {
					return 0, 0, fmt.Errorf("reconcile placements: trim: %w", err)
				}
				freed = append(freed, o)
			}
		}
	}

	sort.Ints(freed)
	for _, o := range freed {
		ok, err := PlaceNextGubbeOnBestFoodHex(ctx, tx, settlementID, o)
		if err != nil {
			return len(freed), replaced, fmt.Errorf("reconcile placements: re-place %d: %w", o, err)
		}
		if ok {
			replaced++
		}
	}
	return len(freed), replaced, nil
}
