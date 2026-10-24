package economy

import (
	"context"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// Storm stops the fishing (Timothy 2026-10-24): while a storm stands on a sea
// hex, that hex's fishing places give nothing. The storm track
// (sea_storm_track, written by combat's weather step) is read as data here;
// economy does not import combat. Every recompute reads the current tick, and
// the weather step recomputes the cities whose fishing it covers or leaves
// (combat.recomputeStormFishing), so the stored rate follows the storm.

// stormHexesNow is the set of hexes a storm in the world stands on this tick.
func stormHexesNow(ctx context.Context, tx Tx, worldID uuid.UUID) (map[hexgrid.Coord]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT t.q, t.r FROM sea_storm_track t JOIN sea_storms s ON s.id = t.storm_id
		  WHERE s.world_id = $1 AND t.tick = current_world_tick()`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[hexgrid.Coord]bool{}
	for rows.Next() {
		var c hexgrid.Coord
		if err := rows.Scan(&c.Q, &c.R); err != nil {
			return nil, err
		}
		out[c] = true
	}
	return out, rows.Err()
}

// StormNoFishingHexes lists the settlement's staffed fishing hexes that a
// storm stands on now — what the city view names as "storm — no fishing".
func StormNoFishingHexes(ctx context.Context, tx Tx, settlementID uuid.UUID) []hexgrid.Coord {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT p.hex_q, p.hex_r FROM settlement_placement p
		   JOIN settlements s ON s.id = p.settlement_id
		   JOIN sea_storm_track t ON t.q = p.hex_q AND t.r = p.hex_r AND t.tick = current_world_tick()
		   JOIN sea_storms ss ON ss.id = t.storm_id AND ss.world_id = s.world_id
		  WHERE p.settlement_id = $1 AND p.target_kind = 'hex' AND p.good_key = $2
		  ORDER BY 1, 2`, settlementID, GoodFish)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []hexgrid.Coord
	for rows.Next() {
		var c hexgrid.Coord
		if rows.Scan(&c.Q, &c.R) == nil {
			out = append(out, c)
		}
	}
	return out
}
