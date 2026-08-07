package combat

// SetStandingOrders: KR3 §5's mid-battle retreat order, the validate+execute
// core shared by the HTTP handler (garrison/distance-0 — applies immediately)
// and the order-courier delivery path (a field unit's commander only hears the
// new order when the Runner physically arrives — command is never instant).
// Same extraction pattern as SetStance (stance_set.go).
//
// standing_orders lives on battle_participants (migration 114), not on units —
// there is deliberately no pre-battle "preset" surface here (megaron_todo.md
// KR3 loose end (c) names only the MID-BATTLE change as the remaining gap);
// a unit not currently in an active battle has nothing to update.

import (
	"context"
	"encoding/json"
	"net/http"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StandingOrdersOrder is one retreat-order command against one unit's active
// battle participation. RetreatAtLoss/HoldToLastMan are pointers so the caller
// can change either field alone without clobbering the other.
type StandingOrdersOrder struct {
	WorldID       uuid.UUID
	PlayerID      uuid.UUID
	UnitID        uuid.UUID
	RetreatAtLoss *float64
	HoldToLastMan *bool
}

// StandingOrdersApplied describes the applied order (the handler's 200 body fields).
type StandingOrdersApplied struct {
	UnitID        uuid.UUID
	BattleID      uuid.UUID
	RetreatAtLoss *float64
	HoldToLastMan bool
}

// SetStandingOrders validates and executes one standing-orders change
// atomically. Any *OrderReject return carries the HTTP status + reason
// exactly as the SetStandingOrders handler answered.
func SetStandingOrders(ctx context.Context, pool *pgxpool.Pool, eventStore *events.Store, o StandingOrdersOrder) (*StandingOrdersApplied, error) {
	if o.RetreatAtLoss == nil && o.HoldToLastMan == nil {
		return nil, reject(http.StatusBadRequest, "must set retreat_at_loss and/or hold_to_last_man")
	}
	if o.RetreatAtLoss != nil && (*o.RetreatAtLoss < 0 || *o.RetreatAtLoss > 1) {
		return nil, reject(http.StatusBadRequest, "retreat_at_loss must be between 0 and 1")
	}

	store := unit.NewStore(pool)
	u, err := store.Get(ctx, o.UnitID)
	if err != nil {
		return nil, reject(http.StatusNotFound, "unit not found")
	}
	if u.OwnerID != o.PlayerID {
		return nil, reject(http.StatusForbidden, "not your unit")
	}
	if u.WorldID != o.WorldID {
		return nil, reject(http.StatusForbidden, "unit not in this world")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, reject(http.StatusInternalServerError, "could not begin transaction")
	}
	defer tx.Rollback(ctx)

	var battleID uuid.UUID
	var existingRaw []byte
	if err := tx.QueryRow(ctx,
		`SELECT bp.battle_id, bp.standing_orders
		 FROM battle_participants bp
		 JOIN battles b ON b.id = bp.battle_id
		 WHERE bp.unit_id = $1 AND b.status = 'active' AND bp.left_tick IS NULL
		 FOR UPDATE OF bp`,
		o.UnitID,
	).Scan(&battleID, &existingRaw); err != nil {
		return nil, reject(http.StatusUnprocessableEntity, "unit is not in an active battle")
	}

	fields := parseStandingOrders(existingRaw)
	if o.RetreatAtLoss != nil {
		fields.RetreatAtLoss = o.RetreatAtLoss
	}
	if o.HoldToLastMan != nil {
		fields.HoldToLastMan = *o.HoldToLastMan
	}
	raw, mErr := json.Marshal(fields)
	if mErr != nil {
		return nil, reject(http.StatusInternalServerError, "could not encode standing orders")
	}

	if _, err := tx.Exec(ctx,
		`UPDATE battle_participants SET standing_orders = $3 WHERE battle_id = $1 AND unit_id = $2`,
		battleID, o.UnitID, raw,
	); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not update standing orders")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not commit standing orders change")
	}

	_, _ = eventStore.Append(ctx, battleID, events.StreamCombat, EventStandingOrdersChanged,
		StandingOrdersChangedPayload{
			BattleID:      battleID,
			UnitID:        o.UnitID,
			WorldID:       o.WorldID,
			RetreatAtLoss: fields.RetreatAtLoss,
			HoldToLastMan: fields.HoldToLastMan,
		}, o.WorldID, nil,
	)

	return &StandingOrdersApplied{
		UnitID:        o.UnitID,
		BattleID:      battleID,
		RetreatAtLoss: fields.RetreatAtLoss,
		HoldToLastMan: fields.HoldToLastMan,
	}, nil
}
