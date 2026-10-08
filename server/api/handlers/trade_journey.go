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

// Foreign gifts preserve both endpoint owners at dispatch. Ordered locks retain
// the existing own→own deadlock avoidance without weakening origin ownership.
func lockGiftSettlements(ctx context.Context, tx pgx.Tx, worldID, sender, recipient, origin, destination uuid.UUID) bool {
	rows, err := tx.Query(ctx, `SELECT id,owner_id,state FROM settlements WHERE world_id=$1 AND (id=$2 OR id=$3) ORDER BY id FOR UPDATE`, worldID, origin, destination)
	if err != nil {
		return false
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id uuid.UUID
		var owner *uuid.UUID
		var state string
		if rows.Scan(&id, &owner, &state) != nil || owner == nil || state != "active" {
			return false
		}
		expected := sender
		if id == destination {
			expected = recipient
		}
		if *owner != expected {
			return false
		}
		n++
	}
	return rows.Err() == nil && n == 2
}
