package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// CatchmentBasePotential returns the base production potential per good for a
// settlement's catchment (hexgrid.CatchmentRadius around its own hex), gated
// by the settlement's ACTUAL buildings. Mirrors the catchment query
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
//
// Belägring S1 (megaron_plan_belagring.md §Implementeringskontrakt step 4):
// mirrors LoadHexProductionOptions' siege-denial filtering, so a besieged
// settlement's break-even hint/status endpoint doesn't show potential the
// blockade has actually cut off. Computed independently here (its own
// ReachableCatchmentHexes call) rather than threaded in from
// RecomputeProduction — this is a read-only display path, not the hot
// production-write path S1's "billig förkoll" is written to protect.
// The per-hex blockade (RecomputeProduction step 1c) is mirrored the same way.
func CatchmentBasePotential(ctx context.Context, tx Tx, settlementID uuid.UUID) (map[string]float64, error) {
	var worldID uuid.UUID
	var ownerID uuid.UUID
	var q, r int
	err := tx.QueryRow(ctx,
		`SELECT prov.world_id, s.owner_id, prov.map_q, prov.map_r
		 FROM settlements s
		 JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&worldID, &ownerID, &q, &r)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: load province coords: %w", err)
	}

	// Ring, not Disk: the settlement's own hex is not a normal production tile
	// (megaron_plan_fysisk_gubbemodell.md §3.2) — it gets NearjordGrainPerTick
	// separately (RecomputeProduction adds it; this base-potential mirror
	// intentionally does not, matching its existing "potential from the
	// worked land" contract — see the doc comment above).
	center := hexgrid.Coord{Q: q, R: r}
	ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
	reachable, _, err := ReachableCatchmentHexes(ctx, tx, worldID, ownerID, center, ring)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: %w", err)
	}
	// Blockad med enhet — mirrors RecomputeProduction step 1c: a fientlig unit
	// in fortify/sentry ON a ring hex silences it whether or not the city is
	// besieged. Without this the break-even hint promised a blockaded city the
	// production it had just lost.
	blockedHexes, err := LoadEnemyPositionedHexes(ctx, tx, worldID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: %w", err)
	}
	for _, c := range ring {
		if blockedHexes[c] {
			reachable[c] = false
		}
	}
	filteredRing := ring[:0:0]
	for _, c := range ring {
		if reachable[c] {
			filteredRing = append(filteredRing, c)
		}
	}
	bs, err := loadBuildingSet(ctx, tx, settlementID)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: %w", err)
	}
	opts, err := loadHexOptionsForHexes(ctx, tx, worldID, filteredRing, bs)
	if err != nil {
		return nil, fmt.Errorf("catchment base potential: %w", err)
	}
	return FullCrewPotential(opts), nil
}
