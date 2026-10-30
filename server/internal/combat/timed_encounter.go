package combat

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"sort"
	"time"
)

// timedEncounters catches contacts between tick samples using saved presence
// intervals. After each stop the candidates are rebuilt: a later traveller can
// meet the stopped army, but cannot meet it on its abandoned future route.
func (h *MarchEncounterHandler) timedEncounters(ctx context.Context, e events.ScheduledEvent, through int64) error {
	a, err := tick.LoadAnchor(ctx, h.pool, e.WorldID)
	if err != nil {
		return err
	}
	from := int64(e.DueTick-1) * 1000
	halted := map[string]int64{}
	for iteration := 0; iteration < 10000; iteration++ {
		rows, err := h.pool.Query(ctx, `SELECT id,owner_id,type,size,status,q,r,march_route,depart_tick,arrive_tick,updated_at FROM units WHERE world_id=$1 AND status IN ('marching','positioned') AND q IS NOT NULL AND r IS NOT NULL AND reaction_policy->>'foreign'='intercept' ORDER BY id`, e.WorldID)
		if err != nil {
			return err
		}
		type actor struct {
			u             crossingUnit
			intervals     []movement.Interval
			moving, timed bool
			raw           []byte
		}
		var actors []actor
		for rows.Next() {
			var x actor
			var status string
			var q, r int
			var raw []byte
			var dep, arr *int
			var updated time.Time
			if err = rows.Scan(&x.u.id, &x.u.owner, &x.u.utype, &x.u.size, &status, &q, &r, &raw, &dep, &arr, &updated); err != nil {
				rows.Close()
				return err
			}
			x.u.pos = province.MapPosition{Q: q, R: r}
			if status == "marching" {
				route, ok := LoadActiveRoute(raw, status, dep, arr)
				if !ok {
					continue
				}
				x.moving = true
				x.timed = route.Version == 2
				x.raw = raw
				start := from
				dispatch := int64(route.StartTick)*1000 + route.OffsetMilli
				if dispatch > start {
					start = dispatch
				}
				end := through + 1
				arrival := int64(route.EndTick)*1000 + route.OffsetMilli + 1
				if arrival < end {
					end = arrival
				}
				x.intervals, err = movement.Presence(route.Move(), movement.Milli(start), movement.Milli(end))
				if err != nil {
					rows.Close()
					return err
				}
			} else {
				start := from
				if at, ok := halted[x.u.id.String()]; ok {
					if at > start {
						start = at
					}
				} else if at := a.ExactMilliAt(updated); at > start {
					start = at
				}
				if start > through {
					continue
				}
				x.intervals = []movement.Interval{{Hex: movement.Hex{Q: q, R: r}, In: movement.Milli(start), Out: movement.Milli(through + 1)}}
			}
			actors = append(actors, x)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		type contact struct {
			i, j int
			hex  movement.Hex
			at   movement.Milli
		}
		var candidates []contact
		for i := range actors {
			for j := i + 1; j < len(actors); j++ {
				x, y := actors[i], actors[j]
				if x.u.owner == y.u.owner || (!x.moving && !y.moving) || (!x.timed && !y.timed) {
					continue
				}
				if hex, at, ok := movement.FirstContact(x.intervals, y.intervals); ok {
					candidates = append(candidates, contact{i, j, hex, at})
				}
			}
		}
		if len(candidates) == 0 {
			return nil
		}
		sort.Slice(candidates, func(i, j int) bool {
			x, y := candidates[i], candidates[j]
			if x.at != y.at {
				return x.at < y.at
			}
			if x.i != y.i {
				return x.i < y.i
			}
			return x.j < y.j
		})
		c := candidates[0]
		group := []crossingUnit{actors[c.i].u, actors[c.j].u}
		for i, index := range []int{c.i, c.j} {
			group[i].pos = province.MapPosition{Q: c.hex.Q, R: c.hex.R}
			group[i].routeRaw = actors[index].raw
		}
		if err = h.resolveHex(ctx, e, [2]int{c.hex.Q, c.hex.R}, group, int64(c.at)); err != nil {
			return err
		}
		// Only routes that actually stopped can acquire a stationary interval.
		changed := false
		for _, u := range group {
			var status string
			if err = h.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, u.id).Scan(&status); err != nil {
				return err
			}
			if status == "positioned" {
				if _, ok := halted[u.id.String()]; !ok {
					halted[u.id.String()] = int64(c.at)
					changed = true
				}
			}
		}
		if !changed {
			return nil
		} // concurrent replacement: next batch reloads it
	}
	return fmt.Errorf("timed encounter candidate bound exceeded")
}
