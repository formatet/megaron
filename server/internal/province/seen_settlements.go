package province

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// RecordSeenSettlements is the one owner of player_seen_settlements
// (megaron_plan_vagbeskrivningar §S0): a city on a remembered hex is shown as
// the player last saw it. Call it wherever hexes enter the player's memory
// (every INSERT INTO player_scouted_tiles) and wherever a city is in live
// sight, with exactly those hexes. Each city standing on them is snapshotted
// as it is now; a snapshot on one of them whose city no longer stands there
// (sunk, or gone) is struck, because the hex has been seen empty.
// Idempotent: a re-run writes the same row.
func RecordSeenSettlements(ctx context.Context, db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}, worldID, playerID uuid.UUID, hexes []MapPosition) error {
	if len(hexes) == 0 {
		return nil
	}
	// Deduplicated: ON CONFLICT DO UPDATE may not touch a row twice.
	seen := make(map[MapPosition]bool, len(hexes))
	var qs, rs []int
	for _, h := range hexes {
		if !seen[h] {
			seen[h] = true
			qs, rs = append(qs, h.Q), append(rs, h.R)
		}
	}
	if _, err := db.Exec(ctx,
		`DELETE FROM player_seen_settlements pss
		  USING unnest($3::int[], $4::int[]) AS h(q, r)
		  WHERE pss.world_id = $1 AND pss.player_id = $2 AND pss.q = h.q AND pss.r = h.r
		    AND NOT EXISTS (SELECT 1 FROM settlements s JOIN provinces p ON p.id = s.province_id
		                     WHERE s.id = pss.settlement_id AND s.state != 'sunk'
		                       AND p.map_q = pss.q AND p.map_r = pss.r)`,
		worldID, playerID, qs, rs,
	); err != nil {
		return err
	}
	_, err := db.Exec(ctx,
		`INSERT INTO player_seen_settlements
		        (world_id, player_id, settlement_id, q, r, name, owner_id, owner_name,
		         is_capital, state, wall_level, population, seen_tick)
		 SELECT s.world_id, $2, s.id, p.map_q, p.map_r, s.name, s.owner_id,
		        COALESCE(pl.wanax_name, pl.username, ''), s.is_capital, s.state,
		        s.wall_level, s.population, w.current_tick
		   FROM unnest($3::int[], $4::int[]) AS h(q, r)
		   JOIN provinces p ON p.world_id = $1 AND p.map_q = h.q AND p.map_r = h.r
		   JOIN settlements s ON s.province_id = p.id
		   JOIN worlds w ON w.id = s.world_id
		   LEFT JOIN players pl ON pl.id = s.owner_id
		  WHERE s.state != 'sunk'
		 ON CONFLICT (player_id, settlement_id) DO UPDATE SET
		        q = EXCLUDED.q, r = EXCLUDED.r, name = EXCLUDED.name,
		        owner_id = EXCLUDED.owner_id, owner_name = EXCLUDED.owner_name,
		        is_capital = EXCLUDED.is_capital, state = EXCLUDED.state,
		        wall_level = EXCLUDED.wall_level, population = EXCLUDED.population,
		        seen_tick = EXCLUDED.seen_tick`,
		worldID, playerID, qs, rs,
	)
	return err
}
