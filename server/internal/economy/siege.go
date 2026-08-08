package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// siegeInterceptRadius mirrors transport.interceptRadius (unexported there,
// and economy may not import transport — a sibling tier in G1, not a
// downward dependency) — the same tuning value: an enemy unit within this
// many hexes of a catchment chokepoint denies it. megaron_plan_belagring.md
// §Rattar: "hur nära en enhet måste vara för att neka en hex (rek: återanvänd
// interceptRadius)."
const siegeInterceptRadius = 2

// siegeCheckRadius is how far from the settlement's own hex an enemy
// positioned unit must stand before the reachability walk below runs at all
// (§S1.2, "billig förkoll") — the catchment ring's own radius plus how far a
// held hex denies beyond itself. The overwhelming majority of settlements
// have no enemy this close on any given tick and skip straight to full
// production with a single cheap EXISTS query.
const siegeCheckRadius = hexgrid.CatchmentRadius + siegeInterceptRadius

// ReachableCatchmentHexes returns, for every hex in ring, whether
// settlementOwner still has physical access to it (§S1+§S2,
// megaron_plan_belagring.md), and whether the settlement counts as besieged
// (at least one ring hex denied). Denial is NOT FOW-gated (§S1.6): a
// blockade starves the city whether the defender has SEEN the blockader or
// not — the notice comes from the besieged flag + falling stock, not sight.
//
// Land ring hexes are reachable when a land-passable path from the
// settlement's own hex (center) reaches them without entering an enemy-held
// hex — BFS over the world's tile graph, blocked nodes = enemy-positioned
// hexes. Sea ring hexes (coastal_sea/deep_sea/river/river_ford — a
// settlement's own adjacent water, worked without any path needed) are not
// walked at all: an enemy unit holding any sea hex NEXT TO the settlement's
// own hex denies every sea ring hex at once (Timothy 2026-08-07: "en galär
// vid hamnen nekar hela sjö-catchmenten... INTE en per havshex" — the
// harbour is the sea's own chokepoint, not a per-hex blockade).
func ReachableCatchmentHexes(ctx context.Context, tx Tx, worldID uuid.UUID, settlementOwner uuid.UUID, center hexgrid.Coord, ring []hexgrid.Coord) (reachable map[hexgrid.Coord]bool, besieged bool, err error) {
	full := make(map[hexgrid.Coord]bool, len(ring))
	for _, c := range ring {
		full[c] = true
	}

	// §S1.2 billig förkoll: no enemy positioned unit within reach of the ring
	// at all → skip the graph walk entirely, full production. This is the
	// condition that lets RecomputeProduction (a hot path, run for every
	// settlement many times a day) pay for the BFS below only for the rare
	// besieged city.
	var enemyNearby bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM units
		   WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned'
		     AND q IS NOT NULL AND r IS NOT NULL
		     AND (ABS(q - $3) + ABS(r - $4) + ABS((q + r) - ($3 + $4))) / 2 <= $5
		 )`,
		worldID, settlementOwner, center.Q, center.R, siegeCheckRadius,
	).Scan(&enemyNearby); err != nil {
		return nil, false, fmt.Errorf("reachable catchment hexes: enemy check: %w", err)
	}
	if !enemyNearby {
		return full, false, nil
	}

	// §S1.3: every enemy-positioned hex within reach — STÅ räcker (Beslutade
	// delbeslut 1), no stance/reaction_policy gate, no FOW gate — becomes a
	// blocked node in the graph walk below.
	rows, err := tx.Query(ctx,
		`SELECT q, r FROM units
		 WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned'
		   AND q IS NOT NULL AND r IS NOT NULL
		   AND (ABS(q - $3) + ABS(r - $4) + ABS((q + r) - ($3 + $4))) / 2 <= $5`,
		worldID, settlementOwner, center.Q, center.R, siegeCheckRadius,
	)
	if err != nil {
		return nil, false, fmt.Errorf("reachable catchment hexes: enemy positions: %w", err)
	}
	enemyHeld := make(map[hexgrid.Coord]bool)
	for rows.Next() {
		var q, r int
		if scanErr := rows.Scan(&q, &r); scanErr != nil {
			rows.Close()
			return nil, false, fmt.Errorf("reachable catchment hexes: scan enemy position: %w", scanErr)
		}
		enemyHeld[hexgrid.Coord{Q: q, R: r}] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("reachable catchment hexes: enemy position rows: %w", err)
	}

	tiles, err := province.LoadTileGraph(ctx, tx, worldID)
	if err != nil {
		return nil, false, fmt.Errorf("reachable catchment hexes: load tile graph: %w", err)
	}

	// Sea denial: an enemy on any sea hex adjacent to the settlement's own
	// hex blocks the harbour chokepoint, denying every sea ring hex at once.
	seaBlocked := false
	for _, n := range hexgrid.Neighbors(center) {
		if !enemyHeld[n] {
			continue
		}
		if terrain, ok := tiles[[2]int{n.Q, n.R}]; ok && isSeaCatchmentTerrain(terrain) {
			seaBlocked = true
			break
		}
	}

	// BFS over land-passable terrain from the settlement's own hex, blocked
	// by enemy-held hexes.
	visited := map[hexgrid.Coord]bool{center: true}
	queue := []hexgrid.Coord{center}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range hexgrid.Neighbors(cur) {
			if visited[n] || enemyHeld[n] {
				continue
			}
			terrain, ok := tiles[[2]int{n.Q, n.R}]
			if !ok || !isLandPassableTerrain(terrain) {
				continue
			}
			visited[n] = true
			queue = append(queue, n)
		}
	}

	reachable = make(map[hexgrid.Coord]bool, len(ring))
	for _, c := range ring {
		terrain, ok := tiles[[2]int{c.Q, c.R}]
		if !ok {
			continue // unknown tile — never grant access to something we can't verify
		}
		if isSeaCatchmentTerrain(terrain) {
			if !seaBlocked {
				reachable[c] = true
			}
			continue
		}
		if visited[c] {
			reachable[c] = true
		}
	}

	besieged = len(reachable) < len(ring)
	return reachable, besieged, nil
}

// isSeaCatchmentTerrain matches the water terrains LoadHexProductionOptions'
// catchment query treats as fish-only tiles (recompute.go's NOT IN list) —
// worked directly from the settlement's own shore, never via a land path.
func isSeaCatchmentTerrain(terrain string) bool {
	switch terrain {
	case "coastal_sea", "deep_sea", "river", "river_ford":
		return true
	}
	return false
}

// isLandPassableTerrain mirrors province.isPassable's "land" branch (that
// function is unexported; economy cannot call it directly, and duplicating
// just this predicate is cheaper than exporting a province internal used
// nowhere else outside pathfinding) — river is a wall for land units
// (megaron_floden_plan.md), river_ford is the one deliberate gap in it, and
// coastal_sea/deep_sea/the two mountain terrains are impassable.
func isLandPassableTerrain(terrain string) bool {
	switch terrain {
	case "coastal_sea", "deep_sea", "river", "mountain_limestone", "mountain_red":
		return false
	}
	return true
}
