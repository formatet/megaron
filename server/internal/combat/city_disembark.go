package combat

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Physical city changes only. Timothy 2026-10-25: the ship and passengers retain
// support_settlement_id, including the ship's next provisioning source.
func (h *UnitArrivalHandler) disembarkAtOwnCity(ctx context.Context, tx pgx.Tx, u unitRow, city *uuid.UUID, q, r int, world uuid.UUID) error {
	if u.category != "naval" || city == nil {
		return nil
	}
	var owned bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM settlements WHERE id=$1 AND world_id=$2 AND owner_id=$3 AND state='active')`, *city, world, u.ownerID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return nil
	}
	ids, err := unit.CargoIDs(ctx, tx, u.id)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `UPDATE units SET status='garrison',settlement_id=$2,q=$3,r=$4,carried_by_ship_id=NULL,updated_at=now() WHERE id=$1`, id, *city, q, r); err != nil {
			return fmt.Errorf("disembark at city: %w", err)
		}
		if _, err := tx.Exec(ctx, `WITH p AS (SELECT carried_silver FROM units WHERE id=$1 AND carried_silver>0), credit AS (SELECT credit_settlement_good($2,'silver',carried_silver,'returned_purse') FROM p) UPDATE units SET carried_silver=0 WHERE id=$1 AND carried_silver>0 AND (SELECT count(*) FROM credit)>0`, id, *city); err != nil {
			return err
		}
		if _, err := h.eventStore.Append(ctx, u.id, events.StreamType(unit.StreamUnit), unit.EventShipUnloaded, unit.ShipUnloadedPayload{ShipUnitID: u.id, CargoUnitID: id, Q: q, R: r}, world, nil); err != nil {
			return err
		}
		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, world, u.ownerID, "UnitArrived", 4, map[string]any{"unit_id": id, "name": unit.LoadDisplayName(ctx, tx, id), "q": q, "r": r, "status": "garrison", "settlement_id": city})
		}
	}
	return nil
}
