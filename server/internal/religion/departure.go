package religion

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const DepartureTimeFactor = 0.75

func DeparturePending(ctx context.Context, db PrayerDB, city, owner uuid.UUID, effect string) (bool, error) {
	var pending bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM departure_prayers d JOIN settlements s ON s.id=d.settlement_id WHERE d.settlement_id=$1 AND d.owner_id=$2 AND s.owner_id=$2 AND d.effect=$3)`, city, owner, effect).Scan(&pending)
	return pending, err
}

// ConsumeDeparture locks the city: casting and departing serialize on this same
// row. A changed owner cannot inherit the gift. Consumption rolls back on failure.
func ConsumeDeparture(ctx context.Context, tx pgx.Tx, world, owner, city uuid.UUID, effect string) (bool, error) {
	var actual *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1 AND world_id=$2 FOR UPDATE`, city, world).Scan(&actual)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if actual == nil || *actual != owner {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `DELETE FROM departure_prayers WHERE settlement_id=$1 AND owner_id=$2 AND effect=$3`, city, owner, effect)
	return tag.RowsAffected() == 1, err
}
