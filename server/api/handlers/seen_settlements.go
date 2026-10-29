package handlers

import (
	"context"
	"time"

	"github.com/google/uuid"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
)

// seenCity is a city as playerID last saw it (player_seen_settlements,
// written by province.RecordSeenSettlements). A city on a memory-only hex is
// shown from this row, never from the live one (megaron_plan_vagbeskrivningar
// §S0); without a row the hex is remembered from before the city stood there.
type seenCity struct {
	Name       string
	OwnerID    *uuid.UUID
	OwnerName  string
	IsCapital  bool
	State      string
	Walls      int
	Population int
	SeenTick   int
}

func loadSeenSettlements(ctx context.Context, db province.Queryer, worldID, playerID uuid.UUID) map[uuid.UUID]seenCity {
	out := map[uuid.UUID]seenCity{}
	rows, err := db.Query(ctx,
		`SELECT settlement_id, name, owner_id, owner_name, is_capital, state, wall_level, population, seen_tick
		   FROM player_seen_settlements WHERE world_id = $1 AND player_id = $2`,
		worldID, playerID,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var c seenCity
		if rows.Scan(&id, &c.Name, &c.OwnerID, &c.OwnerName, &c.IsCapital, &c.State, &c.Walls, &c.Population, &c.SeenTick) == nil {
			out[id] = c
		}
	}
	return out
}

// lastSeen is the player-facing age of a memory snapshot: wall clock when it
// was seen and how many game days ago.
func lastSeen(seenTick int, anchor tick.Anchor) (time.Time, int) {
	age := anchor.Tick - seenTick
	if age < 0 {
		age = 0
	}
	return anchor.WallAt(int64(seenTick) * 1000), age
}
