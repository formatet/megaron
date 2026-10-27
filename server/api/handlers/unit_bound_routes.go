package handlers

import (
	"context"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// attachBoundRoutes describes durable owner-private bindings independently of
// a ship's status or its current cargo leg. freighting_note remains leg-specific.
func attachBoundRoutes(ctx context.Context, db province.Queryer, world, owner uuid.UUID, summaries []unitSummary) error {
	if len(summaries) == 0 {
		return nil
	}
	rows, err := db.Query(ctx, `SELECT so.ship_unit_id,so.id,sf.name||' → '||st.name
 FROM standing_orders so JOIN settlements sf ON sf.id=so.from_settlement_id JOIN settlements st ON st.id=so.to_settlement_id
 WHERE so.world_id=$1 AND so.owner_id=$2 AND so.ship_unit_id IS NOT NULL`, world, owner)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[uuid.UUID]int{}
	for i, s := range summaries {
		byID[s.ID] = i
	}
	for rows.Next() {
		var ship, id uuid.UUID
		var name string
		if err := rows.Scan(&ship, &id, &name); err != nil {
			return err
		}
		if i, ok := byID[ship]; ok {
			summaries[i].BoundRouteID = &id
			summaries[i].BoundRouteName = &name
		}
	}
	return rows.Err()
}
