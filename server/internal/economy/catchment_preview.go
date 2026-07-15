package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Colony founding assumptions — the single source of truth, shared with
// combat.foundColony (unit_arrival.go), which references these directly when it
// writes the real colony. They also let the colonize preview (DEL A of
// megaron_koloni_legibilitet_plan.md) estimate the founding grain balance BEFORE
// any settlement row exists to read. Change a number here and both the live
// colony and its preview move together.
const (
	// ColonyBaseFoundingPopulation is a new colony's baseline population before
	// the colonising unit's own size is added (see foundColony).
	ColonyBaseFoundingPopulation = 1500
	// ColonyGrainSeed is the starting grain stockpile a colony is seeded with.
	ColonyGrainSeed = 300
)

// CatchmentBasePotentialAt returns the base production potential per good summed
// over an EXPLICIT set of catchment hexes, gated by an assumed (rather than
// actual) building set. It exists for the colonize preview, which must estimate a
// hex's production BEFORE a settlement is founded there — so it takes raw
// coordinates and a hypothetical building list instead of a settlement id.
//
// DRIFT-GUARD: this mirrors the catchment production query in two sibling
// functions — RecomputeProduction (recompute.go steps 2+3, the source of truth
// that writes live rates) and CatchmentBasePotential (catchment.go, settlement-
// scoped, gated by ACTUAL buildings). Same joins / deposit / coastal logic; the
// only differences are (1) the caller supplies the hex set explicitly (the fixed
// 7-hex axial offsets live in the caller — see the colonize-preview handler,
// which reuses RecomputeProduction's offsets), and (2) the building gate is
// `pr.building_type IS NULL OR pr.building_type = ANY(assumeBuildings)` instead of
// an EXISTS against the buildings table (empty list = building-free potential).
// If any of the three drift, RecomputeProduction wins — keep this in sync with it.
//
// Passing only known catchment hexes keeps the preview fog-of-war-safe: an
// unknown (fog) hex contributes nothing, so its terrain/deposits never leak into
// the aggregate.
func CatchmentBasePotentialAt(ctx context.Context, tx Tx, worldID uuid.UUID, hexes [][2]int, assumeBuildings []string) (map[string]float64, error) {
	if len(hexes) == 0 {
		return map[string]float64{}, nil
	}
	qs := make([]int32, len(hexes))
	rs := make([]int32, len(hexes))
	for i, h := range hexes {
		qs[i], rs[i] = int32(h[0]), int32(h[1])
	}
	if assumeBuildings == nil {
		assumeBuildings = []string{}
	}

	rows, err := tx.Query(ctx,
		`SELECT pr.good_key, SUM(pr.rate_per_tick) AS base_potential
		 FROM map_tiles mt
		 JOIN unnest($2::int[], $3::int[]) AS hx(q, r) ON hx.q = mt.q AND hx.r = mt.r
		 JOIN production_rules pr ON
		     (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.building_type IS NULL OR pr.building_type = ANY($4::text[]))
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 WHERE mt.world_id = $1
		   AND mt.terrain NOT IN ('deep_sea', 'coastal_sea')
		 GROUP BY pr.good_key`,
		worldID, qs, rs, assumeBuildings,
	)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential at: query production rules: %w", err)
	}
	defer rows.Close()

	potentials := make(map[string]float64)
	for rows.Next() {
		var key string
		var bp float64
		if err := rows.Scan(&key, &bp); err != nil {
			return nil, fmt.Errorf("catchment base potential at: scan: %w", err)
		}
		potentials[key] = bp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catchment base potential at: rows err: %w", err)
	}
	return potentials, nil
}
