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
// Two player modes: when losing ('{}') or hold to the last man. The engine
// still understands legacy fixed thresholds, but players cannot set them.

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RetreatDefault is one Wanax's realm-wide retreat default. RetreatAtLoss nil
// and HoldToLastMan false together mean "when losing".
type RetreatDefault struct {
	RetreatAtLoss *float64
	HoldToLastMan bool
}

// WhenLosing reports whether the default is the fixed threshold.
func (d RetreatDefault) WhenLosing() bool { return d.RetreatAtLoss == nil && !d.HoldToLastMan }

// RetreatDefaultOrder is a request to change the default. Exactly one of the
// two modes must be selected.
type RetreatDefaultOrder struct {
	RetreatAtLoss *float64
	HoldToLastMan bool
	WhenLosing    bool
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
	if rej := ValidateRetreatMode(o.RetreatAtLoss, o.HoldToLastMan, o.WhenLosing); rej != nil {
		return RetreatDefault{}, rej
	}

	d := RetreatDefault{HoldToLastMan: o.HoldToLastMan}
	raw := []byte(`{}`)
	if d.HoldToLastMan {
		raw = []byte(`{"hold_to_last_man":true}`)
	}

	tag, err := pool.Exec(ctx,
		`UPDATE player_world_records SET retreat_default = (retreat_default - 'retreat_at_loss' - 'hold_to_last_man' - 'by_loyalty') || $3::jsonb WHERE world_id = $1 AND player_id = $2`,
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
// owner id and $7 = world id): the owner's realm-wide default, or '{}' (when
// losing) when the owner has no record in this world — an NPC side, or a
// unit whose Wanax never joined. Seeding on JOIN as well as on start is
// deliberate: a reinforcement entering a fight under way is entering a battle
// too, and silently ignoring the default for it would be a difference the
// player cannot see.
const seedStandingOrdersSQL = `COALESCE((SELECT pwr.retreat_default FROM player_world_records pwr
	   WHERE pwr.world_id = $7 AND pwr.player_id = $3), '{}'::jsonb)`
