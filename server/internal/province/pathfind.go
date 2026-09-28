package province

import (
	"container/heap"
	"context"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Queryer abstracts *pgxpool.Pool and pgx.Tx for tile loading in FindPath.
// Both concrete types satisfy this interface via their Query method.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// FindPath runs A* over the world's hex terrain graph. It loads all map_tiles for
// worldID into memory once, then delegates to the pure findPath logic.
//
// path includes both origin (first element) and target (last element).
// cost is the sum of TerrainMoveTicks for each tile entered (path[1:]).
// ok is false when the origin or target tile is absent or impassable, or when no
// traversable route exists. err is non-nil only for DB or scan failures.
func FindPath(ctx context.Context, db Queryer, worldID uuid.UUID, origin, target MapPosition, category string) (path []MapPosition, cost float64, ok bool, err error) {
	g, err := LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return nil, 0, false, err
	}
	path, cost, ok = g.FindPath(origin, target, category)
	return path, cost, ok, nil
}

// TileGraph is an in-memory snapshot of a world's terrain. Load it once with
// LoadTileGraph, then call FindPath many times without re-querying map_tiles —
// used when a single request must path several units (e.g. the /marches and
// /units map endpoints), where per-unit FindPath would reload all tiles each time.
type TileGraph map[[2]int]string

// LoadTileGraph loads every tile of a world into memory for repeated pathfinding.
func LoadTileGraph(ctx context.Context, db Queryer, worldID uuid.UUID) (TileGraph, error) {
	rows, err := db.Query(ctx,
		`SELECT q, r, terrain FROM map_tiles WHERE world_id = $1`,
		worldID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiles := make(TileGraph)
	for rows.Next() {
		var q, r int
		var terrain string
		if scanErr := rows.Scan(&q, &r, &terrain); scanErr != nil {
			return nil, scanErr
		}
		tiles[[2]int{q, r}] = terrain
	}
	return tiles, rows.Err()
}

// FindPath runs A* over an already-loaded graph (no DB access). Semantics match
// the package-level FindPath: path includes origin (first) and target (last);
// ok is false when origin/target is absent/impassable or no route exists.
func (g TileGraph) FindPath(origin, target MapPosition, category string) (path []MapPosition, cost float64, ok bool) {
	return findPath(g, origin, target, category)
}

// StepHours returns the cost of entering each hex of an ALREADY-FOUND path
// (path[1:]), using the same moveHoursFor A* itself uses. It does not search
// a path — it only prices one a caller already has (movement slice 2a, R2:
// "en redan funnen väg"). len(result) == len(path)-1, matching path[1:].
// A path shorter than two hexes (nothing to enter) returns nil.
func (g TileGraph) StepHours(path []MapPosition, category string) []float64 {
	if len(path) < 2 {
		return nil
	}
	out := make([]float64, len(path)-1)
	for i := 1; i < len(path); i++ {
		terrain := g[[2]int{path[i].Q, path[i].R}]
		out[i-1] = moveHoursFor(terrain, category)
	}
	return out
}

// StepHoursDB is StepHours for a caller that only holds a path from the
// package-level FindPath, which loads its own TileGraph internally and does
// not expose it. It loads the world's tiles again to price the path — a
// second DB query, but not a second path SEARCH, which is the thing R1/R5
// forbid re-doing at read time (movement slice 2a).
func StepHoursDB(ctx context.Context, db Queryer, worldID uuid.UUID, path []MapPosition, category string) ([]float64, error) {
	g, err := LoadTileGraph(ctx, db, worldID)
	if err != nil {
		return nil, err
	}
	return g.StepHours(path, category), nil
}

// axialDirs lists the 6 axial hex neighbours, sourced from hexgrid (the
// single source of truth — megaron_todo.md "7-hex-catchmentlistan är
// duplicerad") rather than a second local literal duplicating hex.go's.
var axialDirs = func() [6][2]int {
	var out [6][2]int
	for i, c := range hexgrid.Neighbors(hexgrid.Coord{}) {
		out[i] = [2]int{c.Q, c.R}
	}
	return out
}()

// CategoryCourier routes Runners — order/message couriers
// (temenos_orderlopare_plan.md Fas 4, beslut Timothy 2026-07-16): every land
// hex except mountains at HALF a land unit's terrain ticks (2× spearman
// speed). Mountains are routed around like for land.
//
// SEA is a wall (megaron_plan_ordna_passage.md, slice 3b-4, R3): the old
// "abstract boat" — a courier silently commandeering a boat to cross
// coastal_sea/deep_sea at the flat CourierSeaTicks rate — is gone. A runner
// may only cross open sea aboard a real carrier now (internal/messenger's
// sea-lift mechanic); a route needing the sea reports ok=false here, same as
// for a land unit. RIVER is untouched and stays a boat crossing (below) —
// that is a separate, pre-existing design (megaron_floden_plan.md, Timothy
// 2026-07-29: "a runner commandeers a boat over a river the same as over the
// sea") this slice does not touch; only rivers have their own ford substrate
// (river_ford) as the deliberate land crossing.
const CategoryCourier = "courier"

// CourierSeaTicks is a courier's ticks per RIVER hex — the abstracted boat
// passage over a plain river (megaron_floden_plan.md), unaffected by 3b-4.
// The name is a pre-existing misnomer now that sea itself is impassable for
// CategoryCourier — kept as-is rather than renamed, to avoid an unrelated
// identifier churn across the tree for a slice that did not touch rivers.
const CourierSeaTicks = 0.5

// CategoryCourierLand is CategoryCourier's land-only twin (megaron_plan_
// budet_liftar.md R1, "en ren landväg — courier-graf utan havshex"): same
// runner speed (half a land unit's terrain ticks), but sea and river are
// walls, exactly like the "land" category — a runner may only cross open
// water by boarding a real carrier now, never by silently wading/rowing it
// himself. Used to decide whether a messenger's route needs the sea-lift
// mechanic at all before falling back to CategoryCourier's own abstract boat.
const CategoryCourierLand = "courier_land"

// isPassable reports whether terrain is traversable for the given unit category.
//   - "naval": coastal_sea, deep_sea, river and river_ford are passable.
//   - "courier": everything except mountains AND sea (river = boat passage,
//     unaffected by 3b-4 — see CategoryCourier's own doc comment).
//   - "courier_land": everything "courier" allows, minus river too — a pure
//     land route with no water crossing of any kind.
//   - "land" (and any other value): coastal_sea, deep_sea, river, mountain_limestone,
//     mountain_red are impassable; semi_desert costs 2.0 but is passable.
//     River is a wall for land units (megaron_floden_plan.md — Timothy 2026-07-29).
//     river_ford is the one deliberate gap in that wall (megaron_plan_
//     flodbudget_och_vadstalle.md, Timothy 2026-08-02) — passable for BOTH land
//     and naval, at a steep TerrainMoveTicks cost (movement.go) rather than
//     being excluded here.
func isPassable(terrain, category string) bool {
	if category == "naval" {
		return terrain == "coastal_sea" || terrain == "deep_sea" || terrain == "river" || terrain == "river_ford"
	}
	if category == CategoryCourier {
		switch terrain {
		case "coastal_sea", "deep_sea", "mountain_limestone", "mountain_red":
			return false
		}
		return true
	}
	if category == CategoryCourierLand {
		switch terrain {
		case "coastal_sea", "deep_sea", "river", "mountain_limestone", "mountain_red":
			return false
		}
		return true
	}
	switch terrain {
	case "coastal_sea", "deep_sea", "river", "mountain_limestone", "mountain_red":
		return false
	}
	return true
}

// moveHoursFor returns the cost to enter a hex of terrain for the category.
// Couriers run land at half a land unit's terrain hours (2× spearman speed —
// temenos_synlighet.md §Nivå 1) and cross a plain RIVER (a runner commandeers a
// boat over a river the same as over the sea used to be, megaron_floden_plan.md
// — untouched by 3b-4) at the flat boat rate; every other category pays the
// plain TerrainMoveTicks. Sea never reaches this function for CategoryCourier —
// isPassable above walls it out before moveHoursFor is ever asked its cost.
// river_ford is deliberately ABSENT from the courier-boat-rate branch below: a
// runner does not commandeer a boat to cross a ford, he wades (megaron_plan_
// flodbudget_och_vadstalle.md) — it falls through to TerrainMoveTicks/2 like
// any other land terrain, and TerrainMoveTicks("river_ford") is itself steep
// (movement.go), so the runner still pays for the crossing, just not at the
// flat boat rate.
func moveHoursFor(terrain, category string) float64 {
	if category == CategoryCourier {
		if terrain == "river" {
			return CourierSeaTicks
		}
		return TerrainMoveTicks(terrain) / 2
	}
	if category == CategoryCourierLand {
		return TerrainMoveTicks(terrain) / 2
	}
	return TerrainMoveTicks(terrain)
}

// NearestSeaNeighbor returns the coordinates of a hex adjacent to (q,r) that is
// sea or river terrain (coastal_sea, deep_sea, river or river_ford — a ship in
// a river town must be able to put out into the river, and a ford is just as
// much the river's own water as any other river hex, megaron_plan_
// flodbudget_och_vadstalle.md). Naval units garrisoned at a settlement
// have no position of their own — their origin resolves to the settlement's own
// (land) province hex, which a naval unit can never legally occupy. Callers use
// this to resolve the real departure hex (the harbour dock) before pathfinding,
// instead of letting FindPath reject the unit at its own settlement.
// found=false when no neighbouring hex is sea (e.g. an inland settlement).
func NearestSeaNeighbor(ctx context.Context, db Queryer, worldID uuid.UUID, q, r int) (sq, sr int, found bool, err error) {
	for _, d := range axialDirs {
		nq, nr := q+d[0], r+d[1]
		rows, qerr := db.Query(ctx,
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			worldID, nq, nr,
		)
		if qerr != nil {
			return 0, 0, false, qerr
		}
		var terrain string
		hasRow := rows.Next()
		if hasRow {
			if scanErr := rows.Scan(&terrain); scanErr != nil {
				rows.Close()
				return 0, 0, false, scanErr
			}
		}
		rows.Close()
		if hasRow && (terrain == "coastal_sea" || terrain == "deep_sea" || terrain == "river" || terrain == "river_ford") {
			return nq, nr, true, nil
		}
	}
	return 0, 0, false, nil
}

// NearestUnclaimedLandNeighbor returns the coordinates of a hex adjacent to
// (q,r) that is land (not sea, not impassable mountain) and has no settlement
// of its own — i.e. open ground a land unit could step onto or found a colony
// on. P7 soak fix (2026-07-19, "unit unload kräver hamn/garrison — embark kan
// aldrig etablera fotfäste på ny mark"): a ship carrying cargo that sails to a
// sea hex next to unclaimed shore (rather than into one of its own harbours)
// used to have no way at all to put that cargo ashore — Unload required the
// ship to already be garrisoned at a friendly settlement, so a ship-borne
// landing on genuinely new coastline was structurally impossible. This lets
// the Unload handler find where the cargo can step off onto dry, unclaimed
// land. found=false when every neighbour is sea/mountain or already settled
// (by anyone) — the caller reports that as a clear, actionable rejection
// rather than silently doing nothing.
func NearestUnclaimedLandNeighbor(ctx context.Context, db Queryer, worldID uuid.UUID, q, r int) (lq, lr int, found bool, err error) {
	for _, d := range axialDirs {
		nq, nr := q+d[0], r+d[1]
		rows, qerr := db.Query(ctx,
			`SELECT mt.terrain,
			        EXISTS(
			          SELECT 1 FROM provinces p JOIN settlements s ON s.province_id = p.id
			          WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3
			        ) AS settled
			 FROM map_tiles mt
			 WHERE mt.world_id = $1 AND mt.q = $2 AND mt.r = $3`,
			worldID, nq, nr,
		)
		if qerr != nil {
			return 0, 0, false, qerr
		}
		var terrain string
		var settled bool
		hasRow := rows.Next()
		if hasRow {
			if scanErr := rows.Scan(&terrain, &settled); scanErr != nil {
				rows.Close()
				return 0, 0, false, scanErr
			}
		}
		rows.Close()
		// river_ford counts as sea here too: it is water (shallow and narrow,
		// but water), not the dry unclaimed land this helper is looking for —
		// same reasoning as spawnBuildable's exclusion (megaron_plan_
		// flodbudget_och_vadstalle.md).
		isSea := terrain == "coastal_sea" || terrain == "deep_sea" || terrain == "river" || terrain == "river_ford"
		isMountain := terrain == "mountain_limestone" || terrain == "mountain_red"
		if hasRow && !isSea && !isMountain && !settled {
			return nq, nr, true, nil
		}
	}
	return 0, 0, false, nil
}

// NearestSettlementNeighbor returns the ACTIVE settlement (any owner) adjacent
// to (q,r), with its own province hex coordinates — the reverse of
// NearestSeaNeighbor: given the sea hex a ship occupies (its own position, or
// its march target), which land settlement's harbour is this? Used by
// megaron_plan_budet_liftar.md to find (a) the port a naval march departed
// from — a ship's stored q/r is its harbour SEA hex, not the settlement's own
// land hex, so no direct join is possible — and (b) the land settlement a
// plain ship march's sea-hex target neighbours, i.e. where a lifted messenger
// can step ashore. found=false when no neighbour hex holds an active
// settlement (patrol/explore targets on open water, most often).
func NearestSettlementNeighbor(ctx context.Context, db Queryer, worldID uuid.UUID, q, r int) (settlementID uuid.UUID, sq, sr int, found bool, err error) {
	for _, d := range axialDirs {
		nq, nr := q+d[0], r+d[1]
		rows, qerr := db.Query(ctx,
			`SELECT s.id, p.map_q, p.map_r FROM provinces p
			 JOIN settlements s ON s.province_id = p.id
			 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active'`,
			worldID, nq, nr,
		)
		if qerr != nil {
			return uuid.Nil, 0, 0, false, qerr
		}
		var id uuid.UUID
		var mq, mr int
		hasRow := rows.Next()
		if hasRow {
			if scanErr := rows.Scan(&id, &mq, &mr); scanErr != nil {
				rows.Close()
				return uuid.Nil, 0, 0, false, scanErr
			}
		}
		rows.Close()
		if hasRow {
			return id, mq, mr, true, nil
		}
	}
	return uuid.Nil, 0, 0, false, nil
}

// minPassableCost returns the cheapest TerrainMoveTicks among terrains passable
// for the given category. It is the admissible A* heuristic multiplier: the
// heuristic (HexDistance × minPassableCost) must never overestimate the true
// remaining cost, and the true cost per hex is never lower than this floor.
//   - land: plains (0.75) is the cheapest passable terrain.
//   - naval: coastal_sea (0.4) is the cheapest passable terrain — river (0.5) and
//     river_ford (2.5, movement.go) are both more expensive so the floor stands;
//     a ford is deliberately the priciest naval hex there is (deliberate design,
//     megaron_plan_flodbudget_och_vadstalle.md: "a ford IS shallow and narrow" —
//     it can only ever push this floor down further from admissible, never up).
//     If river's rate is ever tuned below 0.4, this floor must be recomputed or
//     the A* heuristic becomes inadmissible.
//   - courier: plains at half hours (0.375) is the cheapest passable terrain
//     (cheaper than the 0.5 sea boat rate).
func minPassableCost(category string) float64 {
	if category == "naval" {
		return TerrainMoveTicks("coastal_sea") // 0.4
	}
	if category == CategoryCourier || category == CategoryCourierLand {
		return TerrainMoveTicks("plains") / 2 // 0.375
	}
	return TerrainMoveTicks("plains") // 0.75
}

// findPath is the pure A* implementation over an in-memory tile map.
// It returns the shortest passable path from origin to target (ok=true),
// or ok=false if the route is absent or unreachable.
// Exported via FindPath; also callable directly in tests.
func findPath(tiles map[[2]int]string, origin, target MapPosition, category string) (path []MapPosition, cost float64, ok bool) {
	// Validate that both endpoints exist and are passable.
	originTerrain, hasOrigin := tiles[[2]int{origin.Q, origin.R}]
	targetTerrain, hasTarget := tiles[[2]int{target.Q, target.R}]
	if !hasOrigin || !hasTarget {
		return nil, 0, false
	}
	if !isPassable(originTerrain, category) || !isPassable(targetTerrain, category) {
		return nil, 0, false
	}

	_ = originTerrain // validated; cost to enter origin is not added (path[0] is free)

	// Admissible heuristic multiplier: the true cost of any passable hex is never
	// below this floor, so HexDistance × minCost never overestimates (R2 fix).
	minCost := minPassableCost(category)

	// A* state: gScore[node] = best known cost from origin to node.
	gScore := map[MapPosition]float64{origin: 0}
	prev := map[MapPosition]MapPosition{}

	pq := &aStarQueue{}
	heap.Init(pq)
	heap.Push(pq, &aStarItem{
		pos: origin,
		g:   0,
		f:   float64(HexDistance(origin, target)) * minCost,
	})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(*aStarItem)
		pos := cur.pos

		// Stale heap entry: a shorter path to this node was already discovered.
		if cur.g > gScore[pos]+1e-9 {
			continue
		}

		if pos == target {
			// Reconstruct path from target back to origin, then reverse.
			var rev []MapPosition
			for p := pos; ; p = prev[p] {
				rev = append(rev, p)
				if p == origin {
					break
				}
			}
			for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
				rev[i], rev[j] = rev[j], rev[i]
			}
			return rev, gScore[target], true
		}

		// Expand neighbours.
		for _, d := range axialDirs {
			npos := MapPosition{Q: pos.Q + d[0], R: pos.R + d[1]}
			nterrain, exists := tiles[[2]int{npos.Q, npos.R}]
			if !exists || !isPassable(nterrain, category) {
				continue
			}
			ng := gScore[pos] + moveHoursFor(nterrain, category) // cost to ENTER npos
			prev_g, seen := gScore[npos]
			if !seen || ng < prev_g-1e-9 {
				gScore[npos] = ng
				prev[npos] = pos
				h := float64(HexDistance(npos, target)) * minCost
				heap.Push(pq, &aStarItem{pos: npos, g: ng, f: ng + h})
			}
		}
	}

	// No path found.
	return nil, 0, false
}

// aStarItem is one entry in the A* priority queue.
type aStarItem struct {
	pos MapPosition
	g   float64 // actual cost from origin to this node (at time of insertion)
	f   float64 // f-score = g + heuristic
}

// aStarQueue is a min-heap of aStarItems ordered by f-score.
type aStarQueue []*aStarItem

func (q aStarQueue) Len() int           { return len(q) }
func (q aStarQueue) Less(i, j int) bool { return q[i].f < q[j].f }
func (q aStarQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *aStarQueue) Push(x any)        { *q = append(*q, x.(*aStarItem)) }
func (q *aStarQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return item
}
