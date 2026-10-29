package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/world"
)

// tellPlaceRequest names the place a letter tells of
// (megaron_plan_vagbeskrivningar v1): a named terrain place by its anchor, or
// kind "city" with the city's own hex as anchor.
type tellPlaceRequest struct {
	Kind    string `json:"kind"`
	AnchorQ int    `json:"anchor_q"`
	AnchorR int    `json:"anchor_r"`
}

// storyError is a refusal the player can act on (422 + text).
type storyError struct{ msg string }

func (e storyError) Error() string { return e.msg }

// buildStory freezes what teller can tell of the place at this moment: the
// place's hexes the teller knows first-hand (live sight ∪ own memory — never
// what was only heard), and every city standing on them as the teller last
// saw it. A told story is never false: nothing outside the teller's own
// knowledge enters it.
func (h *MessengerHandler) buildStory(ctx context.Context, worldID, teller uuid.UUID, req tellPlaceRequest) (*messenger.Story, error) {
	eyes := loadLiveEyes(ctx, h.pool, worldID, teller, h.clk.Now())
	remembered := loadRememberedTiles(ctx, h.pool, worldID, teller)
	tiles, err := loadGeography(ctx, h.pool, worldID)
	if err != nil {
		return nil, err
	}
	terrain := make(map[[2]int]string, len(tiles))
	for _, t := range tiles {
		terrain[[2]int{t.Q, t.R}] = string(t.Terrain)
	}
	knows := func(xy [2]int) bool {
		return remembered[xy] || province.AnyEyeSees(eyes, province.MapPosition{Q: xy[0], R: xy[1]}, terrain[xy])
	}
	anchor := [2]int{req.AnchorQ, req.AnchorR}
	story := &messenger.Story{Kind: req.Kind, AnchorQ: anchor[0], AnchorR: anchor[1]}

	if req.Kind == "city" {
		if !knows(anchor) {
			return nil, storyError{"you can only tell of a city your own people have seen"}
		}
		story.Hexes = [][2]int{anchor}
	} else {
		states, err := loadPlaceStates(ctx, h.pool, worldID)
		if err != nil {
			return nil, err
		}
		var place *world.Place
		for _, p := range world.Places(tiles) {
			if p.Kind == req.Kind && p.Anchor == anchor {
				place = &p
				break
			}
		}
		if place == nil {
			return nil, storyError{"there is no such place to tell of"}
		}
		name := states[placeKey{place.Kind, place.Anchor}].Name
		if name == "" {
			return nil, storyError{"only a named place can be told of — whoever reaches it first gives it its name"}
		}
		story.Name = name
		for _, xy := range place.Hexes {
			if knows(xy) {
				story.Hexes = append(story.Hexes, xy)
			}
		}
		if len(story.Hexes) == 0 {
			return nil, storyError{fmt.Sprintf("your people have not seen %s — you can only tell of what they saw", name)}
		}
	}

	// Cities on the told hexes: live ones are snapshotted now, so the story
	// carries each city as the teller last saw it (§S0).
	var live []province.MapPosition
	qs, rs := make([]int, len(story.Hexes)), make([]int, len(story.Hexes))
	for i, xy := range story.Hexes {
		p := province.MapPosition{Q: xy[0], R: xy[1]}
		if province.AnyEyeSees(eyes, p, terrain[xy]) {
			live = append(live, p)
		}
		qs[i], rs[i] = xy[0], xy[1]
	}
	if err := province.RecordSeenSettlements(ctx, h.pool, worldID, teller, live); err != nil {
		return nil, err
	}
	rows, err := h.pool.Query(ctx,
		`SELECT pss.settlement_id, pss.q, pss.r, pss.name, pss.owner_id, pss.owner_name,
		        pss.is_capital, pss.state, pss.wall_level, pss.population, pss.seen_tick
		   FROM player_seen_settlements pss
		   JOIN unnest($3::int[], $4::int[]) AS h(q, r) ON pss.q = h.q AND pss.r = h.r
		  WHERE pss.world_id = $1 AND pss.player_id = $2`,
		worldID, teller, qs, rs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c messenger.StoryCity
		if err := rows.Scan(&c.SettlementID, &c.Q, &c.R, &c.Name, &c.OwnerID, &c.OwnerName,
			&c.IsCapital, &c.State, &c.WallLevel, &c.Population, &c.SeenTick); err != nil {
			return nil, err
		}
		story.Cities = append(story.Cities, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if req.Kind == "city" {
		if len(story.Cities) == 0 {
			return nil, storyError{"you can only tell of a city your own people have seen"}
		}
		story.Name = story.Cities[0].Name
	}
	return story, nil
}

// writeStoryError answers a buildStory failure.
func writeStoryError(w http.ResponseWriter, err error) {
	if se, ok := err.(storyError); ok {
		writeError(w, http.StatusUnprocessableEntity, se.msg)
		return
	}
	writeError(w, http.StatusInternalServerError, "could not prepare the story")
}

// loadToldBy maps each hex playerID knows only from a told story to the
// teller's name, for "Told by Wanax X" on the map.
func loadToldBy(ctx context.Context, db province.Queryer, worldID, playerID uuid.UUID) map[[2]int]string {
	out := map[[2]int]string{}
	rows, err := db.Query(ctx,
		`SELECT h.q, h.r, COALESCE(p.wanax_name, p.username, '')
		   FROM player_heard_tiles h JOIN players p ON p.id = h.told_by
		  WHERE h.world_id = $1 AND h.player_id = $2 AND h.source = 'told'`,
		worldID, playerID,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var q, r int
		var name string
		if rows.Scan(&q, &r, &name) == nil {
			out[[2]int{q, r}] = name
		}
	}
	return out
}
