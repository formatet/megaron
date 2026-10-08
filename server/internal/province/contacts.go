package province

import (
	"context"
	"github.com/google/uuid"
)

// VisibleOrigins is the shared letters/foreign-transfer contact gate source.
func VisibleOrigins(ctx context.Context, db Queryer, worldID, playerID uuid.UUID) []MapPosition {
	rows, err := db.Query(ctx,
		`SELECT DISTINCT pos.q, pos.r FROM (
		     -- Own and allied settlements.
		     SELECT p.map_q AS q, p.map_r AS r
		     FROM provinces p
		     JOIN settlements s ON s.province_id = p.id
		     WHERE p.world_id = $1 AND (
		         s.owner_id = $2
		         OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		             SELECT km.kingdom_id FROM kingdom_members km WHERE km.player_id = $2
		         ))
		     )
		     UNION ALL
		     -- Origin and target endpoints of the player's in-flight marches.
		     SELECT op.map_q, op.map_r
		     FROM marching_armies ma
		     JOIN provinces op ON op.id = ma.origin_id
		     JOIN settlements os ON os.province_id = ma.origin_id
		     WHERE ma.world_id = $1 AND ma.resolved = false AND os.owner_id = $2
		     UNION ALL
		     -- Explore marches are excluded: vision is revealed on arrival, not dispatch.
		     SELECT tp.map_q, tp.map_r
		     FROM marching_armies ma
		     JOIN provinces tp ON tp.id = ma.target_id
		     JOIN settlements os ON os.province_id = ma.origin_id
		     WHERE ma.world_id = $1 AND ma.resolved = false AND os.owner_id = $2
		       AND ma.intent != 'explore'
		     UNION ALL
		     -- Settlements this player has contacted by messenger stay visible:
		     -- once a messenger reaches its destination (delivered and onward),
		     -- contact is established, so the destination remains on the map
		     -- afterwards — this is how a Wanax discovers trade partners.
		     SELECT dp.map_q, dp.map_r
		     FROM messengers m
		     JOIN settlements ds ON ds.id = m.destination_id
		     JOIN provinces dp ON dp.id = ds.province_id
		     WHERE m.world_id = $1 AND m.sender_id = $2
		       AND m.status IN ('delivered', 'returning', 'arrived')
		     UNION ALL
		     -- Provinces scouted by explore marches remain visible after the ship returns.
		     SELECT p.map_q, p.map_r
		     FROM player_scouted_provinces sp
		     JOIN provinces p ON p.id = sp.province_id
		     WHERE sp.world_id = $1 AND sp.player_id = $2
		     UNION ALL
		     -- Tiles scouted by unit marches (route-swept FOW).
		     SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2
		 ) pos`,
		worldID, playerID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var origins []MapPosition
	for rows.Next() {
		var pos MapPosition
		if err := rows.Scan(&pos.Q, &pos.R); err == nil {
			origins = append(origins, pos)
		}
	}
	return origins
}
