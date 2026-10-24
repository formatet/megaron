package religion

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Hiketeia (Timothy 2026-10-24, megaron_normer_xenia): a fallen Wanax's
// risen Nomadic Host stands under the gods' protection until it founds a
// city. Attacking it is allowed; the attacker's realm loses this much
// kharis when the battle starts.
const HiketeiaKharisLoss = 70.0

// RealmKharisFloor is the realm Kharis floor (kharis.Floor aliases it).
const RealmKharisFloor = 1.0

// HiketeiaWarning is shown before an order against a risen host goes out.
const HiketeiaWarning = "These are the survivors of a fallen Wanax, under the gods' protection until they found a city. Attacking them breaks hiketeia: your realm loses 70 kharis."

// TakeHiketeiaKharisTx takes the price from the attacker's realm, never below
// the floor, and returns kharis before and after.
func TakeHiketeiaKharisTx(ctx context.Context, tx pgx.Tx, worldID, attacker uuid.UUID) (before, after float64, err error) {
	err = tx.QueryRow(ctx,
		`WITH old AS (
		   SELECT settled(kharis_amount, kharis_rate, kharis_calc_tick) AS k FROM player_world_records
		    WHERE player_id = $1 AND world_id = $2 FOR UPDATE)
		 UPDATE player_world_records p
		    SET kharis_amount = GREATEST($3, old.k - $4), kharis_calc_tick = current_world_tick()
		   FROM old WHERE p.player_id = $1 AND p.world_id = $2
		 RETURNING old.k, p.kharis_amount`,
		attacker, worldID, RealmKharisFloor, HiketeiaKharisLoss,
	).Scan(&before, &after)
	if err == pgx.ErrNoRows {
		return 0, 0, nil
	}
	return before, after, err
}
