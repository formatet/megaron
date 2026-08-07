package province

import (
	"context"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// catchmentHexes returns the hexgrid.CatchmentRadius+1-hex catchment (P1,
// megaron_plan_fysisk_gubbemodell.md) centred at (q, r), INCLUDING the centre
// itself — this is a spatial overlap check (does a candidate site's ground
// already belong to a neighbour?), not a production query, so unlike
// economy's Ring-based catchment queries the centre hex belongs in the set.
func catchmentHexes(q, r int) []hexgrid.Coord {
	return hexgrid.Disk(hexgrid.Coord{Q: q, R: r}, hexgrid.CatchmentRadius)
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
// (any owner) in worldID whose catchment overlaps the catchment a NEW
// settlement founded at (q, r) would claim (hexgrid.CatchmentRadius, P1) —
// the delad-catchment-grind invariant (Timothy 2026-07-27 "JA", precised
// 2026-07-28): "finns delat catchment kan staden inte grundas." The rule
// holds for every owner alike — a Wanax cannot reach a neighbour's ore by
// planting a second city on top of it; the settlement cap and conquest are
// the only sanctioned ways in.
//
// Checked as an actual SET overlap between the two catchments, never as a
// hardcoded hex-distance number, so the rule keeps holding automatically if
// the catchment radius ever changes again — only hexgrid.CatchmentRadius
// would need to move.
//
// Returns (nil, nil) when the site is clear. An existing overlapping PAIR of
// settlements is never re-examined here — this is only ever called at
// founding time against a NEW candidate site, so an already-standing pair
// can never be retroactively flagged (grandfather clause, Timothy 2026-07-28).
func SettlementCatchmentOverlap(ctx context.Context, db Queryer, worldID uuid.UUID, q, r int) (*CatchmentConflict, error) {
	candidate := map[hexgrid.Coord]bool{}
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
// hexgrid.CatchmentRadius via the formula below: this is explanatory
// arithmetic for a human/agent-readable message, not the gate itself (the
// gate is radius-proof; this convenience number is not).
func CatchmentClearanceHexes(dist int) int {
	// Two radius-R catchments (disks) stop touching once their centres are
	// more than 2R hexes apart — safeCentreDistance is the first distance at
	// which they are guaranteed disjoint.
	safeCentreDistance := 2*hexgrid.CatchmentRadius + 1
	if dist >= safeCentreDistance {
		return 0
	}
	return safeCentreDistance - dist
}
