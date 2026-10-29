package messenger

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Story is a place told of in a letter (megaron_plan_vagbeskrivningar v1),
// frozen when the Runner leaves: the hexes the teller had seen and the cities
// on them as the teller last saw them. Stored on messengers.told_place.
type Story struct {
	Kind    string      `json:"kind"`
	AnchorQ int         `json:"anchor_q"`
	AnchorR int         `json:"anchor_r"`
	Name    string      `json:"name"`
	Hexes   [][2]int    `json:"hexes"`
	Cities  []StoryCity `json:"cities,omitempty"`
}

// StoryCity is one player_seen_settlements row carried in a story.
type StoryCity struct {
	SettlementID uuid.UUID  `json:"settlement_id"`
	Q            int        `json:"q"`
	R            int        `json:"r"`
	Name         string     `json:"name"`
	OwnerID      *uuid.UUID `json:"owner_id,omitempty"`
	OwnerName    string     `json:"owner_name"`
	IsCapital    bool       `json:"is_capital"`
	State        string     `json:"state"`
	WallLevel    int        `json:"wall_level"`
	Population   int        `json:"population"`
	SeenTick     int        `json:"seen_tick"`
}

// deliverStoryTx hands a letter's story to the recipient inside the
// delivery transaction (guarded by the status flip, so it runs once): the
// hexes become known by word (player_heard_tiles, source 'told' — drawn as
// memory, never a trade or founding gate), and each city's snapshot replaces
// the recipient's only if it is newer. Returns nil when the letter told of
// nothing.
func deliverStoryTx(ctx context.Context, tx pgx.Tx, worldID, messengerID, tellerID, recipientID uuid.UUID) (*Story, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT told_place FROM messengers WHERE id = $1`, messengerID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load told place: %w", err)
	}
	if len(raw) == 0 || recipientID == uuid.Nil || recipientID == tellerID {
		return nil, nil
	}
	var s Story
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("decode told place: %w", err)
	}
	qs, rs := make([]int, len(s.Hexes)), make([]int, len(s.Hexes))
	for i, xy := range s.Hexes {
		qs[i], rs[i] = xy[0], xy[1]
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO player_heard_tiles (world_id, player_id, q, r, source, heard_tick, told_by)
		 SELECT $1, $2, h.q, h.r, 'told', w.current_tick, $5
		   FROM unnest($3::int[], $4::int[]) AS h(q, r), worlds w
		  WHERE w.id = $1
		 ON CONFLICT (world_id, player_id, q, r) DO NOTHING`,
		worldID, recipientID, qs, rs, tellerID,
	); err != nil {
		return nil, fmt.Errorf("record told hexes: %w", err)
	}
	for _, c := range s.Cities {
		if _, err := tx.Exec(ctx,
			`INSERT INTO player_seen_settlements
			        (world_id, player_id, settlement_id, q, r, name, owner_id, owner_name,
			         is_capital, state, wall_level, population, seen_tick)
			 SELECT $1, $2, s.id, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
			   FROM settlements s WHERE s.id = $3
			 ON CONFLICT (player_id, settlement_id) DO UPDATE SET
			        q = EXCLUDED.q, r = EXCLUDED.r, name = EXCLUDED.name,
			        owner_id = EXCLUDED.owner_id, owner_name = EXCLUDED.owner_name,
			        is_capital = EXCLUDED.is_capital, state = EXCLUDED.state,
			        wall_level = EXCLUDED.wall_level, population = EXCLUDED.population,
			        seen_tick = EXCLUDED.seen_tick
			  WHERE player_seen_settlements.seen_tick < EXCLUDED.seen_tick`,
			worldID, recipientID, c.SettlementID, c.Q, c.R, c.Name, c.OwnerID, c.OwnerName,
			c.IsCapital, c.State, c.WallLevel, c.Population, c.SeenTick,
		); err != nil {
			return nil, fmt.Errorf("record told city: %w", err)
		}
	}
	return &s, nil
}
