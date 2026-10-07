package handlers

import (
	"context"
	"errors"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func writeTradeJourneyError(w http.ResponseWriter, err error) {
	if errors.Is(err, province.ErrNoTradePath) {
		writeError(w, http.StatusUnprocessableEntity, "no passable route for this caravan")
		return
	}
	writeError(w, http.StatusInternalServerError, "could not plan caravan journey")
}

// Ordered locks prevent two opposite transfers from deadlocking on their endpoints.
func lockOwnedTradeSettlements(ctx context.Context, tx pgx.Tx, worldID, ownerID, originID, destID uuid.UUID) bool {
	rows, err := tx.Query(ctx, `SELECT id, owner_id FROM settlements WHERE world_id=$1 AND (id=$2 OR id=$3) ORDER BY id FOR UPDATE`, worldID, originID, destID)
	if err != nil {
		return false
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id uuid.UUID
		var owner *uuid.UUID
		if rows.Scan(&id, &owner) != nil || owner == nil || *owner != ownerID {
			return false
		}
		count++
	}
	wanted := 2
	if originID == destID {
		wanted = 1
	}
	return rows.Err() == nil && count == wanted
}
