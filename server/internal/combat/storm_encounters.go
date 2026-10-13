package combat

import (
	"sort"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

type stormEncounter struct {
	tick int
	hex  [2]int
	id   uuid.UUID
}

// stormEncounters follows the saved entry clock, including every sea hex entered
// during (t-1,t]. It also checks the hex held at the beginning of that interval:
// a storm can drift onto a slow ship before it leaves. Weather is fixed at t
// throughout the interval; only a continuous stay in the same storm is one hit.
// Used by both the real scan and the paired weather simulation.
func stormEncounters(route StoredRoute, enter []int64, graph province.TileGraph, occupied stormOccupancy, lastTick, dueTick int) []stormEncounter {
	holding := func(t int) int {
		if t < route.StartTick || t > route.EndTick {
			return -1
		}
		return sort.Search(len(enter), func(i int) bool { return enter[i] > int64(t)*1000 }) - 1
	}
	stormAt := func(t int, index int) uuid.UUID {
		if index < 1 || !isSeaTerrain(graph[route.Hexes[index]]) {
			return uuid.Nil
		}
		return occupied[t][route.Hexes[index]]
	}
	previous := stormAt(lastTick, holding(lastTick))
	encounters := []stormEncounter{}
	for t := lastTick + 1; t <= dueTick && t <= route.EndTick; t++ {
		if t < route.StartTick {
			continue
		}
		visit := func(index int) {
			id := stormAt(t, index)
			if id != uuid.Nil && id != previous {
				encounters = append(encounters, stormEncounter{tick: t, hex: route.Hexes[index], id: id})
			}
			previous = id
		}
		start, end := int64(t-1)*1000, int64(t)*1000
		first := sort.Search(len(enter), func(i int) bool { return enter[i] > start })
		if first < 1 {
			first = 1
		}
		// Weather can move a storm off the previous hex while it remains over
		// the next one. Its ID still marks the same encounter across ticks;
		// an entered clear hex, or a whole clear stationary tick, ends it.
		if index := holding(t - 1); index >= 1 {
			if id := stormAt(t, index); id != uuid.Nil {
				visit(index)
			} else if first >= len(enter) || enter[first] > end {
				previous = uuid.Nil
			}
		}

		for i := first; i < len(enter) && enter[i] <= end; i++ {
			visit(i)
		}
	}
	return encounters
}
