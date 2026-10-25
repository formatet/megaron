package combat

import (
	"context"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"sort"
)

// timedStormContacts samples saved hex entries and weather changes in temporal
// order. A partial final tick is bounded by through, never rounded into the future.
func timedStormContacts(r StoredRoute, enter []int64, graph province.TileGraph, occupied stormOccupancy, after, through int64, previous uuid.UUID) ([]stormEncounter, uuid.UUID) {
	end := int64(r.EndTick)*1000 + r.OffsetMilli
	if through > end {
		through = end
	}
	times := []int64{}
	for _, at := range enter {
		if at > after && at <= through {
			times = append(times, at)
		}
	}
	for at := (after/1000 + 1) * 1000; at <= through; at += 1000 {
		times = append(times, at)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	var out []stormEncounter
	var last int64 = -1
	for _, at := range times {
		if at == last {
			continue
		}
		last = at
		i := sort.Search(len(enter), func(i int) bool { return enter[i] > at }) - 1
		if i < 0 {
			continue
		}
		id := uuid.Nil
		if isSeaTerrain(graph[r.Hexes[i]]) {
			id = occupied[int(at/1000)][r.Hexes[i]]
		}
		if id != uuid.Nil && id != previous {
			out = append(out, stormEncounter{tick: int(at / 1000), hex: r.Hexes[i], id: id})
		}
		previous = id
	}
	return out, previous
}

// BeforeTimedArrival catches up the existing storm batch to the saved deadline
// before the route can disappear. There are no individual hex jobs.
func (h *SeaStormScanHandler) BeforeTimedArrival(next events.Handler) events.Handler {
	return func(ctx context.Context, e events.ScheduledEvent) error {
		a, err := tick.LoadAnchor(ctx, h.pool, e.WorldID)
		if err != nil {
			return err
		}
		at := a.MilliAt(h.scheduler.Clock().Now())
		graph, err := province.LoadTileGraph(ctx, h.pool, e.WorldID)
		if err != nil {
			return err
		}
		if err = h.stepWeather(ctx, e.WorldID, int(at/1000), graph); err != nil {
			return err
		}
		occupied, err := loadStormOccupancy(ctx, h.pool, e.WorldID, int(at/1000)-stormTrackKeepTicks, int(at/1000))
		if err != nil {
			return err
		}
		voyages, err := h.loadVoyages(ctx, e.WorldID)
		if err != nil {
			return err
		}
		for _, v := range voyages {
			if v.route.Version == 2 {
				if err = h.sail(ctx, e.WorldID, int(at/1000), graph, occupied, v, at); err != nil {
					return err
				}
			}
		}
		return next(ctx, e)
	}
}
