package religion

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ProfanityKharisLoss is what each coarse word a Wanax writes costs the realm
// (Timothy 2026-10-29: the word is replaced and the gods are angered).
const ProfanityKharisLoss = 5.0

// TakeProfanityKharisTx takes words × ProfanityKharisLoss from the writer's
// realm, never below the floor, and returns kharis before and after.
func TakeProfanityKharisTx(ctx context.Context, tx pgx.Tx, worldID, writer uuid.UUID, words int) (before, after float64, err error) {
	err = tx.QueryRow(ctx,
		`WITH old AS (
		   SELECT settled(kharis_amount, kharis_rate, kharis_calc_tick) AS k FROM player_world_records
		    WHERE player_id = $1 AND world_id = $2 FOR UPDATE)
		 UPDATE player_world_records p
		    SET kharis_amount = GREATEST($3, old.k - $4), kharis_calc_tick = (SELECT current_tick FROM worlds WHERE id = $2)
		   FROM old WHERE p.player_id = $1 AND p.world_id = $2
		 RETURNING old.k, p.kharis_amount`,
		writer, worldID, RealmKharisFloor, ProfanityKharisLoss*float64(words),
	).Scan(&before, &after)
	if err == pgx.ErrNoRows {
		return 0, 0, nil
	}
	return before, after, err
}
