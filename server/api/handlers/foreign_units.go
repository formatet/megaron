package handlers

import (
	"context"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// foreignUnit is the JSON shape returned by ForeignUnits for one enemy/neutral
// unit the caller currently has eyes on.
//
// Q/R here are the unit's INTERPOLATED CURRENT position — not the stored origin
// hex. This is the opposite convention from unitSummary (unit.go), where q/r is
// the stored origin and the client interpolates itself; a foreign unit's stored
// row is never exposed, only what the viewer's eyes actually see right now, so
// the server does the interpolation once instead of teaching the client to
// re-derive a stranger's march the way it derives its own.
type foreignUnit struct {
	ID uuid.UUID `json:"id"`
	// Name is the server-formatted unit name ("2nd Spearmen of Knossos") —
	// the map tooltip names every unit it shows, foreign ones included
	// (Timothy 2026-09-26: "vems den är och vad den heter").
	Name     string    `json:"name"`
	Owner    string    `json:"owner"`
	OwnerID  uuid.UUID `json:"owner_id"`
	Type     string    `json:"type"`
	Category string    `json:"category"`
	Size     int       `json:"size"`
	Crew     int       `json:"crew,omitempty"`
	Status   string    `json:"status"`
	Stance   string    `json:"stance,omitempty"`
	Q        int       `json:"q"`
	R        int       `json:"r"`
	// Cargo surfaces a naval unit's embarked land cohort (units.cargo_unit_id,
	// mig 047) as a market signal — a wanax who already sees this ship also
	// sees what it carries, exactly like seeing a laden cart in port. It adds
	// no new visibility: the carrier row above already passed the FOW gate,
	// this is only an extra column on a row that would be returned anyway.
	// Nil for an empty or non-naval unit. Goods-cargo (trade caravans, which
	// carry quantities in transport_goods, not units.cargo_unit_id) is a
	// separate system and out of scope for this field — flagged as a
	// follow-up, not built here.
	Cargo *foreignCargo `json:"cargo,omitempty"`
	// Heading is which way a marching unit goes NOW (province.ReadMarch, the
	// map's own compass) — never where it is going. Timothy 2026-09-26: "likrikta
	// det" — the target, route and arrival that kanonbeslut 3 (2026-08-03) used
	// to disclose here no longer leave the server (megaron_plan_karavanbeslag §7).
	Heading string `json:"heading,omitempty"`
	// Toward is set when the march SEEMS bound for the catchment of one of the
	// caller's own cities: which one, and when it would get there if that is
	// where it is going. An appearance, not a fact — a road round a mountain
	// fools it by design.
	Toward *foreignToward `json:"toward,omitempty"`
}

// foreignToward is the caller's city a foreign march seems bound for.
type foreignToward struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	Name         string    `json:"name"`
	EtaAt        time.Time `json:"eta_at"`
	EtaTick      int       `json:"eta_tick"`
}

// foreignCargo is the embarked cohort a foreign naval unit carries (type +
// size only — same minimal shape unitSummary already exposes to the OWNING
// wanax via CargoUnitID, but here resolved server-side since the watcher has
// no other way to look the cargo unit's id up).
type foreignCargo struct {
	Type string `json:"type"`
	Size int    `json:"size"`
}

// ForeignUnits handles GET /worlds/{worldID}/foreign-units — every unit NOT owned
// by the caller whose current position sits in the caller's tier-1 (live) vision.
//
// This is the informational surface temenos_synlighet.md/megaron_kartaktorer §Relation
// always specified but that was never built: enemy/neutral units had no visibility
// path at all (garrison/forming units are reported via a city's army_total instead,
// see world.go's provinceMarker.ArmyTotal gate). Kanonbeslut Timothy 2026-08-03:
//  1. Live tier reveals a foreign unit in full — type, size AND stance. No blur.
//  2. Remembered (tier 2) reveals nothing — memory carries no activity.
//  3. A march is revealed in full (target/timing/route) as knowledge about the
//     UNIT, but never turns the target/route hexes into map knowledge — the gate
//     that decides whether the unit itself is listed is its CURRENT interpolated
//     position, exactly the same two calls (FindPath + interpolatedEyePos) used to
//     place the viewer's own eyes in loadLiveEyes, so the watcher and the watched
//     never disagree about where anything is.
//
// Unauthenticated callers always get an empty list — unlike /map, which shows the
// whole world to a logged-out caller, no unit data leaves the server for someone
// who isn't a player.
func (h *WorldHandler) ForeignUnits(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	now := h.clk.Now()
	eyes := loadLiveEyes(r.Context(), h.pool, worldID, playerID, now)

	g, err := province.LoadTileGraph(r.Context(), h.pool, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load terrain")
		return
	}

	ownCities, err := loadOwnCityCatchments(r.Context(), h.pool, worldID, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settlements")
		return
	}

	// garrison/forming units live inside a settlement and have no q/r of their
	// own — they surface via the city's army_total (world.go §2b), not here.
	// embarked units carry no position; they move with their ship. Kingdoms are
	// post-MVP and disabled, so every non-owner row here is treated as foreign.
	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.owner_id, COALESCE(pl.wanax_name, pl.username, ''), u.type, u.category, u.size, u.crew,
		        u.status, u.stance, u.q, u.r,
		        u.target_q, u.target_r, u.departs_at, u.arrives_at, u.depart_tick, u.arrive_tick,
		        cu.type, cu.size
		 FROM units u
		 LEFT JOIN players pl ON pl.id = u.owner_id
		 LEFT JOIN units cu ON cu.id = u.cargo_unit_id
		 WHERE u.world_id = $1 AND u.owner_id IS DISTINCT FROM $2
		   AND u.status IN ('positioned','marching')
		   AND u.q IS NOT NULL AND u.r IS NOT NULL`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load foreign units")
		return
	}
	defer rows.Close()

	var out []foreignUnit
	for rows.Next() {
		var fu foreignUnit
		var stance *string
		var storedQ, storedR int
		var targetQ, targetR, departTick, arriveTick *int
		var departsAt, arrivesAt *time.Time
		var cargoType *string
		var cargoSize *int
		if err := rows.Scan(&fu.ID, &fu.OwnerID, &fu.Owner, &fu.Type, &fu.Category, &fu.Size, &fu.Crew,
			&fu.Status, &stance, &storedQ, &storedR,
			&targetQ, &targetR, &departsAt, &arrivesAt, &departTick, &arriveTick,
			&cargoType, &cargoSize); err != nil {
			continue
		}
		if stance != nil {
			fu.Stance = *stance
		}
		if cargoType != nil && cargoSize != nil {
			fu.Cargo = &foreignCargo{Type: *cargoType, Size: *cargoSize}
		}

		// Current position: interpolate a marching unit exactly like loadLiveEyes
		// interpolates the caller's own eyes, so watcher and watched share one
		// position model. Fall back to the stored (origin) hex when pathfinding
		// fails or the unit is merely 'positioned'.
		pos := province.MapPosition{Q: storedQ, R: storedR}
		var march *province.ApparentMarch
		var pathSteps int
		if fu.Status == "marching" && targetQ != nil && targetR != nil && departsAt != nil && arrivesAt != nil {
			p, _, ok := g.FindPath(pos, province.MapPosition{Q: *targetQ, R: *targetR}, fu.Category)
			if ok && len(p) > 0 {
				pos = province.InterpolateAlongPath(now, *departsAt, *arrivesAt, p)
				if m, ok := province.ReadMarch(p, *departsAt, *arrivesAt, now); ok {
					march, pathSteps = &m, len(p)-1
				}
			}
		}

		// Fail closed: no map_tiles row for the unit's current hex means we cannot
		// determine visibility for it at all — omit it rather than guess.
		terrain, ok := g[[2]int{pos.Q, pos.R}]
		if !ok {
			continue
		}
		if !province.AnyEyeSees(eyes, pos, terrain) {
			continue
		}

		fu.Q = pos.Q
		fu.R = pos.R
		if march != nil {
			fu.Heading = march.Heading
			fu.Toward = seemsBoundForOwn(*march, ownCities, departTick, arriveTick, pathSteps)
		}
		out = append(out, fu)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load foreign units")
		return
	}
	// Names are loaded after the rows are drained — only for units that passed
	// the FOW gate, and never while the query above holds its connection.
	for i := range out {
		out[i].Name = unit.LoadDisplayName(r.Context(), h.pool, out[i].ID)
	}
	if out == nil {
		out = []foreignUnit{}
	}
	writeJSON(w, http.StatusOK, out)
}

// ownCity is one of the caller's settlements with its catchment, for testing
// which of them a foreign march seems bound for.
type ownCity struct {
	id        uuid.UUID
	name      string
	catchment []province.MapPosition
}

func loadOwnCityCatchments(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID) ([]ownCity, error) {
	rows, err := pool.Query(ctx,
		`SELECT s.id, s.name, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id = $2`, worldID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ownCity
	for rows.Next() {
		var c ownCity
		var q, r int
		if err := rows.Scan(&c.id, &c.name, &q, &r); err != nil {
			return nil, err
		}
		for _, h := range hexgrid.Disk(hexgrid.Coord{Q: q, R: r}, hexgrid.CatchmentRadius) {
			c.catchment = append(c.catchment, province.MapPosition{Q: h.Q, R: h.R})
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// seemsBoundForOwn picks the nearest of the caller's cities whose catchment the
// march seems headed into, with the arrival "if that is where it is going".
func seemsBoundForOwn(m province.ApparentMarch, cities []ownCity, departTick, arriveTick *int, pathSteps int) *foreignToward {
	var best *foreignToward
	bestDist := -1
	for _, c := range cities {
		d, ok := m.SeemsBoundFor(c.catchment)
		if !ok || (bestDist >= 0 && d >= bestDist) {
			continue
		}
		bestDist = d
		t := &foreignToward{SettlementID: c.id, Name: c.name, EtaAt: m.ArrivalIf(d)}
		if departTick != nil && arriveTick != nil {
			t.EtaTick = m.ArrivalTickIf(d, *departTick, *arriveTick, pathSteps)
		}
		best = t
	}
	return best
}
