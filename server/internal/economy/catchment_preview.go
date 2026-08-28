package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
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
	// 300 → 6,94 (megaron_plan_dagsverkesskalan, mig 136): grain-enhet, ÷43,2.
	// Motsvarar knappt 14 gubbdagsransoner, exakt som före omskalningen.
	ColonyGrainSeed = 6.94
)

// MaxUnitSize is the hard ceiling on a single unit's headcount. Recruitment has
// always enforced it (the remainder spills into a new forming unit), but the
// `divine_recruits` blessing grew its target by 20 % with no ceiling at all —
// and it always picks the LARGEST garrison spearman, so the same unit compounded
// every time the blessing fired. Measured 2026-07-23: five garrison spearmen at
// 1.86–2.13 BILLION men, i.e. saturated against the int32 ceiling. Any writer
// that grows a unit must clamp to this.
const MaxUnitSize = 100

// ReinforceMenPerTick caps how many men a decimated land cohort's home city
// can pour into it in a single tick, via the reinforce trickle
// (kharis/tick.go applyReinforcement, megaron_plan_rekryteringsmodell.md).
// The refill is diverted from the settlement's population GROWTH that same
// tick (never the standing population — the city must never shrink from a
// refill), so this is also throttled by min(ReinforceMenPerTick,
// growth-this-tick, MaxUnitSize-size). Default 4/tick → a full 100-man
// refill takes ~25 ticks (~1 IRL day at the game's usual TicksPerDay=24) for
// a healthy, growing city; a poor one is throttled further by its own
// growth. Tunable — no balance data yet.
const ReinforceMenPerTick = 4

// RecruitCostPerMan returns the per-man resource cost for a unit type
// (good_key → quantity), the same table province.UnitSpecs[type].Costs
// exposes to api/handlers/province.go's Recruit. Thin wrapper so
// kharis/tick.go's reinforce trickle — which may not import province
// directly per G1's dependency list for the kharis package — can reach the
// same cost table through economy, which already imports province (see
// siege.go's LoadTileGraph). Returns nil for an unknown type.
func RecruitCostPerMan(unitType string) map[string]float64 {
	spec, ok := province.UnitSpecs[unitType]
	if !ok {
		return nil
	}
	return spec.Costs
}

// MaxGenesisPopulation bounds the population a genesis silver seed will price
// itself against. The seed is pop-scaled by design (a big capital and a small
// colony get proportionally identical coverage) and it is one of the two
// documented exceptions to "silver never comes from nothing" — which makes it
// the one place where a corrupt population becomes minted silver. It did:
// a colonising unit carrying a blessing-inflated size of 2 976 790 founded
// Phaistos with 31.2 M silver, 99.5 % of the world's supply (2026-07-23).
// 30000 is the settlement population soft cap (see kharis/tick.go growth), so
// this bound never binds a legitimate settlement — it only refuses absurdity.
const MaxGenesisPopulation = 30000

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
// only differences are (1) the caller supplies the hex set explicitly — pass
// hexgrid.Ring(center, hexgrid.CatchmentRadius) to match the other two exactly
// (the settlement's own hex is not a production tile, see catchment.go) — and
// (2) the building gate is `pr.building_type IS NULL OR pr.building_type =
// ANY(assumeBuildings)` instead of an EXISTS against the buildings table
// (empty list = building-free potential). If any of the three drift,
// RecomputeProduction wins — keep this in sync with it.
//
// Passing only known catchment hexes keeps the preview fog-of-war-safe: an
// unknown (fog) hex contributes nothing, so its terrain/deposits never leak into
// the aggregate.
func CatchmentBasePotentialAt(ctx context.Context, tx Tx, worldID uuid.UUID, hexes []hexgrid.Coord, assumeBuildings []string) (map[string]float64, error) {
	if len(hexes) == 0 {
		return map[string]float64{}, nil
	}
	qs, rs := hexgrid.QRArrays(hexes)
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
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 WHERE mt.world_id = $1
		   AND (mt.terrain NOT IN ('deep_sea','coastal_sea','river','river_ford')
		        OR pr.terrain_type = mt.terrain)
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
