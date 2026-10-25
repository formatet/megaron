package combat

import (
	"context"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"github.com/jackc/pgx/v5"
	"sort"
	"time"
)

// BeforeTimedAdvance processes a tick's reached boundaries in order, without
// creating one durable job per hex. Existing world scans and precise arrivals
// call it before consuming/changing route state. Retries use the existing
// sight/claim, storm-progress and battle guards.
func (h *PlaceReachScanHandler) BeforeTimedAdvance(next events.Handler) events.Handler {
	return func(ctx context.Context, e events.ScheduledEvent) error {
		if err := h.advanceTimed(ctx, e); err != nil {
			return err
		}
		return next(ctx, e)
	}
}
func (h *PlaceReachScanHandler) advanceTimed(ctx context.Context, e events.ScheduledEvent) error {
	if err := province.SweepRunnerRoutes(ctx, h.pool, e.WorldID, h.scheduler.Clock().Now()); err != nil {
		return err
	}
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,618))`, e.WorldID.String()); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,618))`, e.WorldID.String())

	a, err := tick.LoadAnchor(ctx, h.pool, e.WorldID)
	if err != nil {
		return err
	}
	through := a.ExactMilliAt(h.scheduler.Clock().Now())
	from := int64(e.DueTick-1) * 1000
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	voyages, err := loadPlaceVoyages(ctx, tx, e.WorldID)
	_ = tx.Rollback(ctx)
	if err != nil {
		return err
	}
	var last int64
	err = h.pool.QueryRow(ctx, `SELECT last_milli FROM timed_route_progress WHERE world_id=$1`, e.WorldID).Scan(&last)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	fresh := err == pgx.ErrNoRows
	if err == nil {
		from = last
	} else {
		last = from - 1
	}
	if through <= last {
		return nil
	}
	times := map[int64]bool{}
	hasTimed := false
	earliest := through
	for _, v := range voyages {
		if v.route.Version == 2 {
			hasTimed = true
			dispatch := int64(v.route.StartTick)*1000 + v.route.OffsetMilli
			if dispatch < earliest {
				earliest = dispatch
			}
		}
		entered, err := RouteEnterMilli(v.route)
		if err != nil {
			return err
		}
		for _, at := range entered {
			if at >= from && at > last && at <= through {
				times[at] = true
			}
		}
	}
	if !hasTimed {
		return nil
	}
	// A seed/catch-up job can predate the first v2 journey by many ticks.
	// There was no timed actor then, so no timed boundary needs replaying.
	if fresh && from < earliest {
		from = earliest
		last = from - 1
		for at := range times {
			if at < from {
				delete(times, at)
			}
		}
	}
	for at := (from/1000 + 1) * 1000; at <= through; at += 1000 {
		if at > last {
			times[at] = true
		}
	}
	times[through] = true
	points := make([]int64, 0, len(times))
	for at := range times {
		points = append(points, at)
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	for _, at := range points {
		clk := routeInstant{a.WallAt(at)}
		scheduler := events.NewScheduler(h.pool, clk)
		// Sight belongs to a reached hex even if a storm or enemy stops the unit there.
		tx, err = h.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if err = placeReachLock(ctx, tx, e.WorldID); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		reached, err := recordPlaceEntriesTx(ctx, tx, h.store, e.WorldID, at)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		deliverPlaceReach(ctx, h.store, h.hub, e.WorldID, reached)
		storm := NewSeaStormScanHandler(h.pool, scheduler, h.store, h.hub)
		// Storms retain their existing priority at an exact tie.
		if err = storm.advanceTimedAt(ctx, e, at); err != nil {
			return err
		}
		encounter := NewMarchEncounterHandler(h.pool, scheduler, h.store, clk, h.hub)
		if err = encounter.timedEncounters(ctx, e, at); err != nil {
			return err
		}
		intercept := NewUnitInterceptScanHandler(h.pool, scheduler, h.store, clk, h.hub)
		if err = intercept.scan(ctx, e, at); err != nil {
			return err
		}
		freight := transport.NewInterceptScanHandler(h.pool, scheduler, h.store, h.hub, clk)
		if err = freight.ScanAt(ctx, e); err != nil {
			return err
		}
		if _, err = h.pool.Exec(ctx, `INSERT INTO timed_route_progress(world_id,last_milli) VALUES($1,$2) ON CONFLICT(world_id) DO UPDATE SET last_milli=GREATEST(timed_route_progress.last_milli,EXCLUDED.last_milli)`, e.WorldID, at); err != nil {
			return err
		}
	}
	return nil
}

type routeInstant struct{ at time.Time }

func (c routeInstant) Now() time.Time { return c.at }
