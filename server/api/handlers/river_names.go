package handlers

import (
	"context"
	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type placeState struct {
	Name      string
	ClaimedBy *uuid.UUID
	Named     bool
}
type placeKey struct {
	Kind   string
	Anchor [2]int
}

func loadPlaceStates(ctx context.Context, pool *pgxpool.Pool, w uuid.UUID) (map[placeKey]placeState, error) {
	rows, err := pool.Query(ctx, `SELECT kind,anchor_q,anchor_r,COALESCE(name,''),claimed_by,named_tick IS NOT NULL FROM place_names WHERE world_id=$1`, w)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[placeKey]placeState{}
	for rows.Next() {
		var k placeKey
		var s placeState
		if err := rows.Scan(&k.Kind, &k.Anchor[0], &k.Anchor[1], &s.Name, &s.ClaimedBy, &s.Named); err != nil {
			return nil, err
		}
		out[k] = s
	}
	return out, rows.Err()
}
func loadGeography(ctx context.Context, pool *pgxpool.Pool, w uuid.UUID) ([]world.MapTile, error) {
	rows, err := pool.Query(ctx, `SELECT q,r,terrain FROM map_tiles WHERE world_id=$1`, w)
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
	return tiles, rows.Err()
}

// Catchment callers already authorize the settlement's known ring. Geography
// only identifies components; player-chosen names come from the projection.
func loadRiverNames(ctx context.Context, pool *pgxpool.Pool, w uuid.UUID) (map[[2]int]string, error) {
	tiles, err := loadGeography(ctx, pool, w)
	if err != nil {
		return nil, err
	}
	states, err := loadPlaceStates(ctx, pool, w)
	if err != nil {
		return nil, err
	}
	out := map[[2]int]string{}
	for _, p := range world.Places(tiles) {
		if p.Kind != "river" {
			continue
		}
		s := states[placeKey{p.Kind, p.Anchor}]
		for _, h := range p.Hexes {
			out[h] = s.Name
		}
	}
	return out, nil
}
