package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// CatchmentBasePotential returns the base production potential per good for a
// settlement's catchment (its own hex + the 6 adjacent), gated by the
// settlement's ACTUAL buildings. Mirrors the catchment query
// RecomputeProduction runs internally (recompute.go steps 2+3) — kept as a
// separate exported function rather than folded into RecomputeProduction so
// read-only callers (status endpoint's grain break-even hint — DEL C of
// megaron_ekonomi_legibilitet_plan.md; the allocate-guardrail in DEL D; the
// colony plan) can derive a settlement's production ceiling without running a
// full recompute or duplicating the SQL a third time. If the two queries ever
// drift, RecomputeProduction's is the source of truth (it writes the live
// rates); this one must be kept in sync with it.
//
// Sibling: CatchmentBasePotentialAt (catchment_preview.go) is the hex-scoped,
// pre-settlement variant used by the colonize preview (assumed buildings instead
// of actual ones). Same joins — keep all three in sync.
func CatchmentBasePotential(ctx context.Context, tx Tx, settlementID uuid.UUID) (map[string]float64, error) {
	var worldID uuid.UUID
	var q, r int
	err := tx.QueryRow(ctx,
		`SELECT prov.world_id, prov.map_q, prov.map_r
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&worldID, &q, &r)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: load province coords: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT pr.good_key, SUM(pr.rate_per_tick) AS base_potential
		 FROM map_tiles mt
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR EXISTS (
		             SELECT 1 FROM buildings b
		             WHERE b.settlement_id = $1 AND b.building_type = pr.building_type))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 WHERE mt.world_id = $2
		   AND mt.terrain NOT IN ('deep_sea', 'coastal_sea')
		   AND (
		       (mt.q = $3   AND mt.r = $4  ) OR
		       (mt.q = $3+1 AND mt.r = $4  ) OR (mt.q = $3-1 AND mt.r = $4  ) OR
		       (mt.q = $3   AND mt.r = $4+1) OR (mt.q = $3   AND mt.r = $4-1) OR
		       (mt.q = $3+1 AND mt.r = $4-1) OR (mt.q = $3-1 AND mt.r = $4+1)
		   )
		 GROUP BY pr.good_key`,
		settlementID, worldID, q, r,
	)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: query production rules: %w", err)
	}
	defer rows.Close()

	potentials := make(map[string]float64)
	for rows.Next() {
		var key string
		var bp float64
		if err := rows.Scan(&key, &bp); err != nil {
			return nil, fmt.Errorf("catchment base potential: scan: %w", err)
		}
		potentials[key] = bp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catchment base potential: rows err: %w", err)
	}
	return potentials, nil
}
