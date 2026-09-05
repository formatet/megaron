package province

import (
	"context"

	"github.com/google/uuid"
)

// minSettlementCentreDistance is the minimum hex-distance allowed between two
// settlements' centres — an OWN design number (§3,
// megaron_plan_hexagarskap_och_stadsavstand.md), not a derivation of
// hexgrid.CatchmentRadius.
//
// Before §2/§2b, two overlapping catchments meant double-harvest: the same
// hex, worked by two settlements, paid out twice. That made
// 2*CatchmentRadius+1 (the first centre distance at which two radius-R
// catchment disks are guaranteed disjoint) a LOAD-BEARING gate — overlap
// itself was the exploit, so founding had to forbid it outright, and
// SettlementCatchmentOverlap enforced that by testing actual catchment SET
// overlap. §2/§2b made hex capacity global and gave every hex a single owner
// (first settlement to place a gubbe there takes it, no exceptions) —
// overlapping catchments are ordinary now, not an exploit, so forbidding
// overlap stopped being the job of this gate. §3 replaces the set-overlap
// test with a plain minimum centre distance, set to 4: the same minimum Civ
// VI uses between city centres, and the number that makes founding next to a
// resource 4 hexes from an existing city legal (Timothy 2026-09-04). Tune
// this directly; it no longer moves in lockstep with hexgrid.CatchmentRadius,
// and two settlements' catchments CAN now genuinely overlap (hex ownership,
// not distance, is what keeps that safe).
const minSettlementCentreDistance = 4

// CatchmentConflict identifies an existing settlement within
// minSettlementCentreDistance of a candidate founding site.
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
// (any owner) in worldID within minSettlementCentreDistance hexes of a NEW
// settlement founded at (q, r) — the delad-catchment-grind invariant
// (Timothy 2026-07-27 "JA", precised 2026-07-28): "finns delat catchment kan
// staden inte grundas," narrowed by §3 to a plain minimum centre distance
// now that hex ownership (§2/§2b) makes catchment overlap itself safe. The
// rule holds for every owner alike — a Wanax cannot found on top of a
// neighbour's doorstep; the settlement cap and conquest are the only
// sanctioned ways in.
//
// Returns (nil, nil) when the site is clear. An existing too-close PAIR of
// settlements is never re-examined here — this is only ever called at
// founding time against a NEW candidate site, so an already-standing pair
// can never be retroactively flagged (grandfather clause, Timothy 2026-07-28).
func SettlementCatchmentOverlap(ctx context.Context, db Queryer, worldID uuid.UUID, q, r int) (*CatchmentConflict, error) {
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

		dist := HexDistance(MapPosition{Q: q, R: r}, MapPosition{Q: c.Q, R: c.R})
		if dist >= minSettlementCentreDistance {
			continue
		}
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
// need to move away from a settlement at hex-distance dist to clear
// minSettlementCentreDistance — the number the founding error/preview
// reports to the player. Mirrors the same threshold SettlementCatchmentOverlap
// enforces, so the message always matches the gate.
func CatchmentClearanceHexes(dist int) int {
	if dist >= minSettlementCentreDistance {
		return 0
	}
	return minSettlementCentreDistance - dist
}
