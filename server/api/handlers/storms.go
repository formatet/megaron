package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// stormHexView / stormView are the wire shape of GET /worlds/{id}/storms.
type stormHexView struct {
	Q int `json:"q"`
	R int `json:"r"`
}

type stormView struct {
	ID        uuid.UUID      `json:"id"`
	Tier      string         `json:"tier"` // "live" | "remembered"
	Hexes     []stormHexView `json:"hexes"`
	Heading   string         `json:"heading,omitempty"` // live only: where it is drifting
	SeenTick  int            `json:"seen_tick"`         // live: now; remembered: when you last saw it
	LastKnown bool           `json:"last_known,omitempty"`
	Name      string         `json:"name,omitempty"` // empty until the first Wanax to meet it names it
	CanName   bool           `json:"can_name,omitempty"` // you were first to meet it and have not named it yet
}

// Storms handles GET /worlds/:worldID/storms (megaron_plan_stormar.md).
//
// FOW: a storm is shown live, with its heading, only while one of its hexes is inside the
// Wanax's current sight (the same eyes and the same Eye.Sees test as /map). Seeing it writes
// player_storm_sightings, so afterwards it is shown where it was LAST SEEN, marked
// remembered with that tick — never its live position. A remembered storm whose every
// remembered hex is in sight again, and which is not there, has moved on and is forgotten.
// The full track is never exposed.
func (h *WorldHandler) Storms(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ctx := r.Context()

	var tick int
	if err := h.pool.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id = $1`, worldID).Scan(&tick); err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	eyes := loadLiveEyes(ctx, h.pool, worldID, playerID, h.clk.Now())

	type current struct {
		heading int
		hexes   []stormHexView
	}
	order := []uuid.UUID{}
	now := map[uuid.UUID]*current{}
	rows, err := h.pool.Query(ctx,
		`SELECT s.id, s.heading, t.q, t.r
		   FROM sea_storms s
		   JOIN sea_storm_track t ON t.storm_id = s.id
		    AND t.tick = (SELECT max(tick) FROM sea_storm_track WHERE storm_id = s.id)
		  WHERE s.world_id = $1 ORDER BY s.id, t.slot`, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load storms")
		return
	}
	for rows.Next() {
		var id uuid.UUID
		var heading int
		var hx stormHexView
		if err := rows.Scan(&id, &heading, &hx.Q, &hx.R); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "could not read storms")
			return
		}
		c := now[id]
		if c == nil {
			c = &current{heading: heading}
			now[id] = c
			order = append(order, id)
		}
		c.hexes = append(c.hexes, hx)
	}
	rows.Close()

	remembered := map[uuid.UUID]struct {
		seen  int
		hexes []stormHexView
	}{}
	srows, err := h.pool.Query(ctx,
		`SELECT p.storm_id, p.seen_tick, p.hexes FROM player_storm_sightings p
		   JOIN sea_storms s ON s.id = p.storm_id WHERE p.player_id = $1 AND s.world_id = $2`, playerID, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load storm memory")
		return
	}
	for srows.Next() {
		var id uuid.UUID
		var seen int
		var raw []byte
		if err := srows.Scan(&id, &seen, &raw); err != nil {
			continue
		}
		var hexes []stormHexView
		if json.Unmarshal(raw, &hexes) == nil {
			remembered[id] = struct {
				seen  int
				hexes []stormHexView
			}{seen, hexes}
		}
	}
	srows.Close()

	terrain := map[[2]int]string{}
	var qs, rs []int
	for _, c := range now {
		for _, hx := range c.hexes {
			qs, rs = append(qs, hx.Q), append(rs, hx.R)
		}
	}
	for _, m := range remembered {
		for _, hx := range m.hexes {
			qs, rs = append(qs, hx.Q), append(rs, hx.R)
		}
	}
	if len(qs) > 0 {
		trows, err := h.pool.Query(ctx,
			`SELECT t.q, t.r, t.terrain FROM map_tiles t
			   JOIN unnest($2::int[], $3::int[]) AS p(q, r) ON t.q = p.q AND t.r = p.r WHERE t.world_id = $1`, worldID, qs, rs)
		if err == nil {
			for trows.Next() {
				var q, r int
				var t string
				if trows.Scan(&q, &r, &t) == nil {
					terrain[[2]int{q, r}] = t
				}
			}
			trows.Close()
		}
	}
	sees := func(hx stormHexView) bool {
		return province.AnyEyeSees(eyes, province.MapPosition{Q: hx.Q, R: hx.R}, terrain[[2]int{hx.Q, hx.R}])
	}

	out := []stormView{}
	for _, id := range order {
		c := now[id]
		live := false
		for _, hx := range c.hexes {
			live = live || sees(hx)
		}
		if live {
			out = append(out, stormView{ID: id, Tier: "live", Hexes: c.hexes, Heading: combat.StormHeadingName(c.heading), SeenTick: tick})
			h.rememberStorm(ctx, playerID, id, tick, c.hexes)
			continue
		}
		m, ok := remembered[id]
		if !ok {
			continue
		}
		allInSight := len(m.hexes) > 0
		for _, hx := range m.hexes {
			allInSight = allInSight && sees(hx)
		}
		if allInSight {
			// You are looking at the place it was and it is not there: it has moved on.
			_, _ = h.pool.Exec(ctx, `DELETE FROM player_storm_sightings WHERE player_id = $1 AND storm_id = $2`, playerID, id)
			continue
		}
		out = append(out, stormView{ID: id, Tier: "remembered", Hexes: m.hexes, SeenTick: m.seen, LastKnown: true})
	}
	h.nameStorms(ctx, worldID, playerID, out)
	writeJSON(w, http.StatusOK, map[string]any{"tick": tick, "storms": out})
}

func (h *WorldHandler) rememberStorm(ctx context.Context, playerID, stormID uuid.UUID, tick int, hexes []stormHexView) {
	raw, err := json.Marshal(hexes)
	if err != nil {
		return
	}
	_, _ = h.pool.Exec(ctx,
		`INSERT INTO player_storm_sightings (player_id, storm_id, seen_tick, hexes) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (player_id, storm_id) DO UPDATE SET seen_tick = EXCLUDED.seen_tick, hexes = EXCLUDED.hexes`,
		playerID, stormID, tick, raw)
}

// nameStorms adds each listed storm's name and whether this Wanax may still name it. Only
// storms already in the FOW-filtered list are touched, so a name never reveals a storm.
func (h *WorldHandler) nameStorms(ctx context.Context, worldID, playerID uuid.UUID, out []stormView) {
	if len(out) == 0 {
		return
	}
	type nm struct {
		name      string
		claimedBy *uuid.UUID
		named     bool
	}
	byID := map[uuid.UUID]nm{}
	rows, err := h.pool.Query(ctx, `SELECT id, COALESCE(name, ''), claimed_by, named_tick IS NOT NULL FROM sea_storms WHERE world_id = $1`, worldID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var n nm
		if rows.Scan(&id, &n.name, &n.claimedBy, &n.named) == nil {
			byID[id] = n
		}
	}
	for i := range out {
		n := byID[out[i].ID]
		out[i].Name = n.name
		out[i].CanName = !n.named && n.claimedBy != nil && *n.claimedBy == playerID
	}
}
