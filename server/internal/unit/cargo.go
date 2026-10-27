package unit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CargoCapacity counts whole land units, independent of hull crew or goods.
func CargoCapacity(kind Type) int {
	switch kind {
	case TypeMerchantman:
		return 3
	case TypeGalley, TypeWarGalley:
		return 1
	}
	return 0
}

// FirstCargoSQL is a compatibility projection, never an alternative owner.
// Consumers needing a troop outcome must use CargoIDs rather than this scalar.
func FirstCargoSQL(alias string) string {
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(alias) {
		panic("invalid trusted unit alias")
	}
	return `(SELECT passenger.id FROM units passenger WHERE passenger.carried_by_ship_id=` + alias + `.id ORDER BY passenger.created_at,passenger.id LIMIT 1)`
}

type CargoReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// CargoIDs orders by creation then id: the oldest passenger founds a colony.
func CargoIDs(ctx context.Context, db CargoReader, ship uuid.UUID) ([]uuid.UUID, error) {
	rows, err := db.Query(ctx, `SELECT id FROM units WHERE carried_by_ship_id=$1 ORDER BY created_at,id`, ship)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

var ErrCargoFull = errors.New("ship cargo full")

// BindCargoTx serializes boarding by locking the ship before its passenger.
// Callers own mission-specific status/location validation and status updates.
func BindCargoTx(ctx context.Context, tx pgx.Tx, ship, cargo uuid.UUID) error {
	var world, owner uuid.UUID
	var kind Type
	var category Category
	if err := tx.QueryRow(ctx, `SELECT world_id,owner_id,type,category FROM units WHERE id=$1 FOR UPDATE`, ship).Scan(&world, &owner, &kind, &category); err != nil {
		return err
	}
	if category != CategoryNaval || CargoCapacity(kind) == 0 {
		return fmt.Errorf("unit is not a cargo vessel")
	}
	ids, err := CargoIDs(ctx, tx, ship)
	if err != nil {
		return err
	}
	if len(ids) >= CargoCapacity(kind) {
		return fmt.Errorf("%s carries at most %d land forces: %w", DisplayName(string(kind)), CargoCapacity(kind), ErrCargoFull)
	}
	var cargoWorld, cargoOwner uuid.UUID
	var cargoType Type
	var cargoCategory Category
	var carriedBy *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT world_id,owner_id,type,category,carried_by_ship_id FROM units WHERE id=$1 FOR UPDATE`, cargo).Scan(&cargoWorld, &cargoOwner, &cargoType, &cargoCategory, &carriedBy); err != nil {
		return err
	}
	if cargoWorld != world || cargoOwner != owner {
		return fmt.Errorf("ship and land unit must have the same owner and world")
	}
	if cargoCategory != CategoryLand || !CanEmbark(cargoType) {
		return fmt.Errorf("only land forces can be loaded onto ships")
	}
	if carriedBy != nil {
		return fmt.Errorf("land unit is already aboard a ship")
	}
	_, err = tx.Exec(ctx, `UPDATE units SET carried_by_ship_id=$2,updated_at=now() WHERE id=$1`, cargo, ship)
	return err
}

// NoCargoSQL is true when nothing rides the ship row named alias.
func NoCargoSQL(alias string) string {
	return `NOT EXISTS(SELECT 1 FROM units passenger WHERE passenger.carried_by_ship_id=` + trustedAlias(alias) + `.id)`
}

// CargoRoomSQL is true when the ship row named alias can take one more land
// unit; the per-type numbers come from CargoCapacity, never a second copy.
func CargoRoomSQL(alias string) string {
	a := trustedAlias(alias)
	return `(SELECT count(*) FROM units passenger WHERE passenger.carried_by_ship_id=` + a + `.id) < CASE ` + a + `.type WHEN '` +
		string(TypeMerchantman) + `' THEN ` + strconv.Itoa(CargoCapacity(TypeMerchantman)) + ` ELSE ` + strconv.Itoa(CargoCapacity(TypeGalley)) + ` END`
}

func trustedAlias(alias string) string {
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(alias) {
		panic("invalid trusted unit alias")
	}
	return alias
}
