package province

import (
	"context"

	"github.com/google/uuid"
)

// catchmentNeighborOffsets are the 6 axial neighbour offsets a settlement's
// catchment reaches beyond its own hex — the same fixed offsets duplicated
// across economy.CatchmentBasePotential, economy.CatchmentBasePotentialAt,
// api/handlers/create_metropolis.go's Demeter check and api/handlers/world.go's
// ColonizePreview. Catchment radius is 1 hex (own hex + 6 neighbours = 7
// total) and is NOT touched by this slice (server/delad-catchment-grind,
// 2026-07-30) — if it ever changes, every one of these sibling lists,
// including this one, must move together.
var catchmentNeighborOffsets = [6][2]int{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1},
}

// catchmentHexes returns the 7 hexes of a catchment centred at (q, r).
func catchmentHexes(q, r int) [7][2]int {
	var out [7][2]int
	out[0] = [2]int{q, r}
	for i, o := range catchmentNeighborOffsets {
		out[i+1] = [2]int{q + o[0], r + o[1]}
	}
	return out
}

// CatchmentConflict identifies an existing settlement whose catchment
// overlaps a candidate founding site.
type CatchmentConflict struct {
	SettlementID uuid.UUID
	OwnerID      *uuid.UUID // nil is not expected among the "alive" states this query returns, but scanned defensively
	Name         string
	Terrain      string // "" if the tile row is somehow missing; callers must not assume it is always populated
	Q, R         int
}

// settlementDeadStates excludes settlement rows whose ground is no longer
// held/farmed — the same "still a going concern" filter already used
// throughout the codebase (internal/kharis/project.go, internal/kharis/tick.go,
// api/handlers/db.go: `state NOT IN ('sunk', 'collapsed', 'razed')`). Collapse
// and razing free the province (internal/combat/collapse.go,
// internal/combat/sack.go set territory_state/controller_id back to free/NULL);
// abandonment frees it too (api/handlers/settlement.go Abandon). A settlement
// in one of these states is a ruin, not a farm — nothing is harvesting its
// catchment any more, so it cannot be "shared" with a new founding.
const settlementDeadStatesSQL = `'collapsed', 'razed', 'sunk', 'abandoned'`

// SettlementCatchmentOverlap reports the nearest existing, alive settlement
// (any owner) in worldID whose 7-hex catchment overlaps the 7-hex catchment a
// NEW settlement founded at (q, r) would claim — the delad-catchment-grind
// invariant (Timothy 2026-07-27 "JA", precised 2026-07-28): "finns delat
// catchment kan staden inte grundas." The rule holds for every owner alike —
// a Wanax cannot reach a neighbour's ore by planting a second city on top of
// it; the settlement cap and conquest are the only sanctioned ways in.
//
// Checked as an actual SET overlap between the two 7-hex catchments, never as
// a hardcoded hex-distance number, so the rule keeps holding automatically if
// the catchment radius (currently 1, untouched by this slice) ever changes —
// only catchmentNeighborOffsets above would need to move.
//
// Returns (nil, nil) when the site is clear. An existing overlapping PAIR of
// settlements is never re-examined here — this is only ever called at
// founding time against a NEW candidate site, so an already-standing pair
// can never be retroactively flagged (grandfather clause, Timothy 2026-07-28).
func SettlementCatchmentOverlap(ctx context.Context, db Queryer, worldID uuid.UUID, q, r int) (*CatchmentConflict, error) {
	candidate := map[[2]int]bool{}
	for _, hex := range catchmentHexes(q, r) {
		candidate[hex] = true
	}

	rows, err := db.Query(ctx,
		`SELECT s.id, s.owner_id, s.name, p.map_q, p.map_r, mt.terrain
		 FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 LEFT JOIN map_tiles mt ON mt.world_id = s.world_id AND mt.q = p.map_q AND mt.r = p.map_r
		 WHERE s.world_id = $1
		   AND s.state NOT IN (`+settlementDeadStatesSQL+`)`,
		worldID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var best *CatchmentConflict
	bestDist := 0
	for rows.Next() {
		var c CatchmentConflict
		var terrain *string
		if err := rows.Scan(&c.SettlementID, &c.OwnerID, &c.Name, &c.Q, &c.R, &terrain); err != nil {
			return nil, err
		}
		if terrain != nil {
			c.Terrain = *terrain
		}

		overlaps := false
		for _, hex := range catchmentHexes(c.Q, c.R) {
			if candidate[hex] {
				overlaps = true
				break
			}
		}
		if !overlaps {
			continue
		}

		dist := HexDistance(MapPosition{Q: q, R: r}, MapPosition{Q: c.Q, R: c.R})
		if best == nil || dist < bestDist {
			cc := c
			best = &cc
			bestDist = dist
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return best, nil
}

// CatchmentClearanceHexes returns how many more hexes a candidate site would
// need to move away from a settlement at hex-distance dist to clear its
// catchment — the number the founding error/preview reports to the player.
//
// Unlike SettlementCatchmentOverlap's actual set-overlap test, this IS tied to
// the current catchment radius via the literal 3 below: this is explanatory
// arithmetic for a human/agent-readable message, not the gate itself (the
// gate is radius-proof; this convenience number is not). If the catchment
// radius ever changes, update the 3 here alongside catchmentNeighborOffsets.
func CatchmentClearanceHexes(dist int) int {
	const safeCentreDistance = 3 // two radius-1 catchments stop touching at centre-distance 3
	if dist >= safeCentreDistance {
		return 0
	}
	return safeCentreDistance - dist
}
