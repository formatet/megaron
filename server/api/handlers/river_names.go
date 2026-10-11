package handlers

import (
	"context"
	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loadRiverNames reads complete static geography for presentation. Callers
// must gate each returned name with the same visibility as its terrain.
func loadRiverNames(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID) (map[[2]int]string, error) {
	rows, err := pool.Query(ctx, `SELECT q,r,terrain FROM map_tiles WHERE world_id=$1`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tiles []world.MapTile
	for rows.Next() {
		var t world.MapTile
		if err := rows.Scan(&t.Q, &t.R, &t.Terrain); err != nil {
			return nil, err
		}
		tiles = append(tiles, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return world.RiverNames(worldID.String(), tiles), nil
}
