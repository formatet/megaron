package handlers

import (
	"context"
	"fmt"
	"strings"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stormRouteReach is how near a storm hex must lie to a water hex of the route to count as
// "on" it: the hex itself or a neighbour, since a storm drifts one hex a tick
// (megaron_plan_stormar.md).
const stormRouteReach = 1

// stormOnRoute is one storm the Wanax already knows (sees now or remembers) lying on a sea route.
type stormOnRoute struct {
	Storm    stormView
	Q, R     int // the storm hex nearest the route, as the Wanax knows it
	RouteIdx int // first route hex it touches: the nearest one along the voyage
}

// knownStormsOnRoute lists the storms near the water hexes of route, nearest along the voyage
// first. It reads knownStorms — the same FOW-filtered source as GET /storms — so a storm the
// Wanax has neither in sight nor in memory can never appear here (Timothy 2026-11-01).
// Land hexes of the route are ignored: storms do not touch them.
func knownStormsOnRoute(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, playerID uuid.UUID, route []province.MapPosition) ([]stormOnRoute, int, error) {
	if len(route) == 0 {
		return nil, 0, nil
	}
	now, storms, err := knownStorms(ctx, pool, clk, worldID, playerID)
	if err != nil || len(storms) == 0 {
		return nil, now, err
	}
	qs, rs := make([]int, len(route)), make([]int, len(route))
	for i, hx := range route {
		qs[i], rs[i] = hx.Q, hx.R
	}
	water := map[[2]int]bool{}
	rows, err := pool.Query(ctx,
		`SELECT t.q, t.r, t.terrain FROM map_tiles t
		   JOIN unnest($2::int[], $3::int[]) AS p(q, r) ON t.q = p.q AND t.r = p.r WHERE t.world_id = $1`, worldID, qs, rs)
	if err != nil {
		return nil, now, err
	}
	for rows.Next() {
		var q, r int
		var terrain string
		if rows.Scan(&q, &r, &terrain) == nil && province.IsWaterBarrier(terrain) {
			water[[2]int{q, r}] = true
		}
	}
	rows.Close()

	var out []stormOnRoute
	for _, st := range storms {
		hit := stormOnRoute{Storm: st, RouteIdx: -1}
		best := stormRouteReach + 1
		for i, rh := range route {
			if !water[[2]int{rh.Q, rh.R}] {
				continue
			}
			for _, sh := range st.Hexes {
				d := province.HexDistance(rh, province.MapPosition{Q: sh.Q, R: sh.R})
				if d > stormRouteReach {
					continue
				}
				if hit.RouteIdx < 0 {
					hit.RouteIdx = i
				}
				if d < best {
					best, hit.Q, hit.R = d, sh.Q, sh.R
				}
			}
		}
		if hit.RouteIdx >= 0 {
			out = append(out, hit)
		}
	}
	// nearest along the voyage first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].RouteIdx < out[j-1].RouteIdx; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, now, nil
}

// stormRouteLine is the one line a sea voyage's forecast adds: the storm met first, then "+N more".
func stormRouteLine(found []stormOnRoute, nowTick int) string {
	if len(found) == 0 {
		return ""
	}
	f := found[0]
	name := "Storm"
	if f.Storm.Name != "" {
		name = f.Storm.Name
	}
	var b strings.Builder
	if f.Storm.Tier == "live" {
		fmt.Fprintf(&b, "%s on this route at (%d, %d)", name, f.Q, f.R)
		if f.Storm.Heading != "" {
			fmt.Fprintf(&b, ", drifting %s", f.Storm.Heading)
		}
	} else {
		fmt.Fprintf(&b, "%s seen on this route at (%d, %d), %s ago", name, f.Q, f.R, tick.FormatDays(nowTick-f.Storm.SeenTick))
	}
	if len(found) > 1 {
		fmt.Fprintf(&b, " (+%d more)", len(found)-1)
	}
	fmt.Fprintf(&b, ". A storm takes %d hull.", combat.StormDamage)
	return b.String()
}

// stormForecast is the full storm_warning of a sea voyage: the ship's hull line (if any), then
// the known-storm line for route. Both "" for nothing to say. A failed read degrades to the
// hull line alone — a forecast never blocks an order.
func stormForecast(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, playerID uuid.UUID, hullLine string, route []province.MapPosition) string {
	found, now, err := knownStormsOnRoute(ctx, pool, clk, worldID, playerID, route)
	if err != nil {
		return hullLine
	}
	line := stormRouteLine(found, now)
	if hullLine == "" {
		return line
	}
	if line == "" {
		return hullLine
	}
	return hullLine + " " + line
}

// stormHullWarning tells a Wanax, before and as a ship sails, what any storm
// would do to it at its present hull. It names the rule and the ship's own
// state only — never a storm the Wanax has not seen (Timothy 2026-11-01).
// "" for land units and for ships that would survive a storm.
func stormHullWarning(u *unit.Unit) string {
	if u == nil || unit.CategoryOf(u.Type) != unit.CategoryNaval || u.Hull > combat.StormDamage {
		return ""
	}
	return fmt.Sprintf("Hull %d of %d: any storm this ship meets will sink it (a storm takes %d hull). Unseen storms may lie on the way; repair at a shipyard first.", u.Hull, combat.HullCap(u.HullBonus), combat.StormDamage)
}

// stormShipWarning is stormHullWarning for a ship known only by type and hull
// (a free ship picked for a transfer, a ship named in a passage order); bonus is its units.hull_bonus.
func stormShipWarning(shipType string, hull, bonus int) string {
	return stormHullWarning(&unit.Unit{Type: unit.Type(shipType), Hull: hull, HullBonus: bonus})
}
