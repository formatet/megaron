package province

import (
	"container/heap"
	"context"

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
// cost is the sum of TerrainMoveHours for each tile entered (path[1:]).
// ok is false when the origin or target tile is absent or impassable, or when no
// traversable route exists. err is non-nil only for DB or scan failures.
func FindPath(ctx context.Context, db Queryer, worldID uuid.UUID, origin, target MapPosition, category string) (path []MapPosition, cost float64, ok bool, err error) {
	rows, err := db.Query(ctx,
		`SELECT q, r, terrain FROM map_tiles WHERE world_id = $1`,
		worldID,
	)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	tiles := make(map[[2]int]string)
	for rows.Next() {
		var q, r int
		var terrain string
		if scanErr := rows.Scan(&q, &r, &terrain); scanErr != nil {
			return nil, 0, false, scanErr
		}
		tiles[[2]int{q, r}] = terrain
	}
	if err = rows.Err(); err != nil {
		return nil, 0, false, err
	}

	path, cost, ok = findPath(tiles, origin, target, category)
	return path, cost, ok, nil
}

// axialDirs lists the 6 axial hex neighbours.
var axialDirs = [6][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1}}

// isPassable reports whether terrain is traversable for the given unit category.
//   - "naval": only coastal_sea and deep_sea are passable.
//   - "land" (and any other value): coastal_sea, deep_sea, mountain_limestone,
//     mountain_red are impassable; semi_desert costs 2.0 but is passable.
func isPassable(terrain, category string) bool {
	if category == "naval" {
		return terrain == "coastal_sea" || terrain == "deep_sea"
	}
	switch terrain {
	case "coastal_sea", "deep_sea", "mountain_limestone", "mountain_red":
		return false
	}
	return true
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

	// A* state: gScore[node] = best known cost from origin to node.
	gScore := map[MapPosition]float64{origin: 0}
	prev := map[MapPosition]MapPosition{}

	pq := &aStarQueue{}
	heap.Init(pq)
	heap.Push(pq, &aStarItem{
		pos: origin,
		g:   0,
		f:   float64(HexDistance(origin, target)),
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
			ng := gScore[pos] + TerrainMoveHours(nterrain) // cost to ENTER npos
			prev_g, seen := gScore[npos]
			if !seen || ng < prev_g-1e-9 {
				gScore[npos] = ng
				prev[npos] = pos
				h := float64(HexDistance(npos, target))
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

func (q aStarQueue) Len() int            { return len(q) }
func (q aStarQueue) Less(i, j int) bool  { return q[i].f < q[j].f }
func (q aStarQueue) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q *aStarQueue) Push(x any)         { *q = append(*q, x.(*aStarItem)) }
func (q *aStarQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return item
}
