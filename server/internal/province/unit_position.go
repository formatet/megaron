package province

import (
	"context"
	"time"

	"formatet/megaron/server/internal/movement"
	"github.com/google/uuid"
)

// UnitPosition is the stored movement projection. Q/R identify the departure
// while marching; Physical resolves the one position used by eyes and Hosts.
// Callers supply the clock instant and tick.Anchor.MilliAt(now) (G1).
type UnitPosition struct {
	Status, Category       string
	Q, R                   int
	TargetQ, TargetR       *int
	DepartsAt, ArrivesAt   *time.Time
	DepartTick, ArriveTick *int
	Route                  []byte
}

func (u UnitPosition) Physical(ctx context.Context, db Queryer, worldID uuid.UUID, now time.Time, at int64) MapPosition {
	origin := MapPosition{Q: u.Q, R: u.R}
	if saved, ok := movement.LoadActiveRoute(u.Route, u.Status, u.DepartTick, u.ArriveTick); ok {
		if physical, err := movement.PositionAt(saved.Move(), movement.Milli(at)); err == nil {
			return MapPosition{Q: physical.Q, R: physical.R}
		}
	}
	// Preserve the existing legacy eye/Host behavior only for missing/stale
	// routes: interpolate a traversable path, keeping the origin on failure.
	if u.Status == "marching" && u.TargetQ != nil && u.TargetR != nil && u.DepartsAt != nil && u.ArrivesAt != nil {
		path, _, ok, err := FindPath(ctx, db, worldID, origin, MapPosition{Q: *u.TargetQ, R: *u.TargetR}, u.Category)
		if err == nil && ok && len(path) > 0 {
			return InterpolateAlongPath(now, *u.DepartsAt, *u.ArrivesAt, path)
		}
	}
	return origin
}
