package province

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RowQueryer is the single-row read the waterway predicate needs (pool or tx).
type RowQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// HasThalassaWaterway reports whether navigable water beside the land hex
// (q, r) — sea, lake, river or ford — leads on to the Thalassa. It is the one
// definition behind both the shipyard gate and Poseidon's founding galley: a
// ship must be able to leave. A closed lake has no such way out.
func HasThalassaWaterway(ctx context.Context, db RowQueryer, worldID uuid.UUID, q, r int) (bool, error) {
	var reaches bool
	err := db.QueryRow(ctx, `
		WITH RECURSIVE water AS (
		  SELECT q, r, terrain FROM map_tiles
		   WHERE world_id = $1 AND terrain_is_navigable(terrain)
		     AND (q, r) IN (($2+1,$3), ($2-1,$3), ($2,$3+1), ($2,$3-1), ($2+1,$3-1), ($2-1,$3+1))
		  UNION
		  SELECT n.q, n.r, n.terrain FROM water w
		    JOIN map_tiles n ON n.world_id = $1 AND terrain_is_navigable(n.terrain)
		     AND (n.q, n.r) IN ((w.q+1,w.r), (w.q-1,w.r), (w.q,w.r+1), (w.q,w.r-1), (w.q+1,w.r-1), (w.q-1,w.r+1))
		)
		SELECT EXISTS (SELECT 1 FROM water WHERE terrain_is_thalassa(terrain))`,
		worldID, q, r,
	).Scan(&reaches)
	return reaches, err
}
