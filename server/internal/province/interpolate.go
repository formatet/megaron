package province

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

// InterpolatePosition returns the hex a marching unit currently occupies, given
// the outbound path it is following and how much of the journey has elapsed.
// It re-walks the same A* path the unit took at departure — terrain is static,
// so FindPath is deterministic — and steps along it in proportion to elapsed
// time over total travel time. Used by march recall/redirect (temenos_march_recall.md)
// to catch a unit wherever it actually is, not wherever a straight line would place it.
//
// ok is false when the path can no longer be found (origin/target off the map,
// or no traversable route) — callers should fall back to origin in that case.
func InterpolatePosition(
	ctx context.Context, db Queryer, worldID uuid.UUID,
	origin, target MapPosition, category string,
	departsAt, arrivesAt, now time.Time,
) (pos MapPosition, ok bool, err error) {
	path, _, pathOK, err := FindPath(ctx, db, worldID, origin, target, category)
	if err != nil {
		return MapPosition{}, false, err
	}
	if !pathOK || len(path) == 0 {
		return origin, false, nil
	}

	total := arrivesAt.Sub(departsAt)
	if total <= 0 || !now.Before(arrivesAt) {
		return target, true, nil
	}
	if !now.After(departsAt) {
		return origin, true, nil
	}

	frac := float64(now.Sub(departsAt)) / float64(total)
	idx := int(math.Floor(frac * float64(len(path)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(path) {
		idx = len(path) - 1
	}
	return path[idx], true, nil
}
