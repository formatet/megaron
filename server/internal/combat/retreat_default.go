package combat

// The realm-wide DEFAULT retreat order (Timothy 2026-09-25: "there must be
// some kind of general setting under War"). Stored on player_world_records.
// retreat_default (migration 145) in the same JSONB shape as
// battle_participants.standing_orders, and copied onto every participant row
// a Wanax's unit gets when it enters a battle (startBattle/joinBattle,
// battle.go). The mid-battle per-unit override (SetStandingOrders) edits that
// copy afterwards, so changing the default never touches a battle already
// under way.
//
// It is a standing doctrine, not an order to a unit: nothing moves, so it
// applies at once (no Runner) — command is still never instant, because it
// only reaches men who enter a battle after the change.
//
// Three states, exactly one at a time:
//   - by loyalty ('{}', the column default): break at the loyalty-derived
//     threshold (routFractionForLoyalty) — today's behaviour.
//   - retreat_at_loss = x: break when the side is down to x of its starting
//     strength (same meaning as SetStandingOrders' field of the same name).
//   - hold_to_last_man: never break.

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RetreatDefault is one Wanax's realm-wide retreat default. RetreatAtLoss nil
// and HoldToLastMan false together mean "by loyalty".
type RetreatDefault struct {
	RetreatAtLoss *float64
	HoldToLastMan bool
}

// ByLoyalty reports whether the default is the loyalty-derived threshold.
func (d RetreatDefault) ByLoyalty() bool { return d.RetreatAtLoss == nil && !d.HoldToLastMan }

// RetreatDefaultOrder is a request to change the default. Exactly one of the
// three must be set.
type RetreatDefaultOrder struct {
	RetreatAtLoss *float64
	HoldToLastMan bool
	ByLoyalty     bool
}

// LoadRetreatDefault reads one Wanax's default in one world. A Wanax who has
// not joined the world is refused (404) rather than handed a guessed default.
func LoadRetreatDefault(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID) (RetreatDefault, error) {
	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT retreat_default FROM player_world_records WHERE world_id = $1 AND player_id = $2`,
		worldID, playerID,
	).Scan(&raw); err != nil {
		return RetreatDefault{}, reject(http.StatusNotFound, "you have not joined this world")
	}
	so := parseStandingOrders(raw)
	return RetreatDefault{RetreatAtLoss: so.RetreatAtLoss, HoldToLastMan: so.HoldToLastMan}, nil
}

// SetRetreatDefault validates and stores a new default. Any *OrderReject
// return carries the HTTP status + reason for the handler to answer with.
func SetRetreatDefault(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID, o RetreatDefaultOrder) (RetreatDefault, error) {
	set := 0
	if o.RetreatAtLoss != nil {
		set++
	}
	if o.HoldToLastMan {
		set++
	}
	if o.ByLoyalty {
		set++
	}
	if set != 1 {
		return RetreatDefault{}, reject(http.StatusBadRequest,
			"set exactly one of retreat_at_loss, hold_to_last_man: true, or by_loyalty: true")
	}
	// Same range SetStandingOrders accepts for the same field — one rule, not two.
	if o.RetreatAtLoss != nil && (*o.RetreatAtLoss < 0 || *o.RetreatAtLoss > 1) {
		return RetreatDefault{}, reject(http.StatusBadRequest, "retreat_at_loss must be between 0 and 1")
	}

	d := RetreatDefault{RetreatAtLoss: o.RetreatAtLoss, HoldToLastMan: o.HoldToLastMan}
	raw := []byte(`{}`)
	if !d.ByLoyalty() {
		b, err := json.Marshal(standingOrdersFields{RetreatAtLoss: d.RetreatAtLoss, HoldToLastMan: d.HoldToLastMan})
		if err != nil {
			return RetreatDefault{}, reject(http.StatusInternalServerError, "could not encode retreat default")
		}
		raw = b
	}
	tag, err := pool.Exec(ctx,
		`UPDATE player_world_records SET retreat_default = $3 WHERE world_id = $1 AND player_id = $2`,
		worldID, playerID, raw,
	)
	if err != nil {
		return RetreatDefault{}, reject(http.StatusInternalServerError, "could not save retreat default")
	}
	if tag.RowsAffected() == 0 {
		return RetreatDefault{}, reject(http.StatusNotFound, "you have not joined this world")
	}
	return d, nil
}

// seedStandingOrdersSQL is the standing_orders value a new battle_participants
// row is born with (startBattle and joinBattle, battle.go — both bind $3 =
// owner id and $7 = world id): the owner's realm-wide default, or '{}' (by
// loyalty) when the owner has no record in this world — an NPC side, or a
// unit whose Wanax never joined. Seeding on JOIN as well as on start is
// deliberate: a reinforcement entering a fight under way is entering a battle
// too, and silently ignoring the default for it would be a difference the
// player cannot see.
const seedStandingOrdersSQL = `COALESCE((SELECT pwr.retreat_default FROM player_world_records pwr
	   WHERE pwr.world_id = $7 AND pwr.player_id = $3),'{}'::jsonb)`
