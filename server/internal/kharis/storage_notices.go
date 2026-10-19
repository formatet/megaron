package kharis

import (
	"context"
	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StorageForecaster is a consuming read interface: kharis must not import combat.
type StorageForecaster interface {
	StorageForecast(context.Context, pgx.Tx, uuid.UUID) ([]economy.StorageForecast, error)
}

func (h *TickHandler) SetStorageForecaster(f StorageForecaster) { h.storageForecaster = f }

func (h *TickHandler) observeStorage(ctx context.Context, world uuid.UUID) error {
	if h.storageForecaster == nil {
		return nil
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tick int
	if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1 FOR SHARE`, world).Scan(&tick); err != nil {
		return err
	}
	// An ownership gap closes the old owner's period without exposing new stock.
	if _, err := tx.Exec(ctx, `UPDATE kharis_notice_state ns SET state='false',observed_tick=$2
 WHERE ns.world_id=$1 AND ns.condition LIKE 'storage_cap:%' AND ns.state='true'
 AND NOT EXISTS(SELECT 1 FROM settlements s WHERE s.id=ns.scope_id AND s.owner_id=ns.player_id AND s.state NOT IN('sunk','collapsed','razed'))`, world, tick); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT s.id,s.owner_id,s.province_id,s.name FROM settlements s
 WHERE s.world_id=$1 AND s.owner_id IS NOT NULL AND s.state NOT IN('sunk','collapsed','razed') ORDER BY s.id FOR SHARE`, world)
	if err != nil {
		return err
	}
	type city struct {
		id, owner, province uuid.UUID
		name                string
	}
	var cities []city
	for rows.Next() {
		var c city
		if err := rows.Scan(&c.id, &c.owner, &c.province, &c.name); err != nil {
			rows.Close()
			return err
		}
		cities = append(cities, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var notices []transitionNotice
	for _, c := range cities {
		forecasts, err := h.storageForecaster.StorageForecast(ctx, tx, c.id)
		if err != nil {
			return err
		}
		for _, f := range forecasts {
			next, kind := "false", ""
			if f.Warning {
				next, kind = "true", "StorageCapacityWarning"
			}
			body := map[string]any{"settlement_id": c.id, "province_id": c.province, "name": c.name, "good_key": f.GoodKey, "amount": f.Amount, "cap": f.Cap, "net_per_tick": f.NetPerTick, "incoming": f.Incoming, "full_in_ticks": f.FullInTicks}
			n, err := h.recordTransition(ctx, tx, world, c.owner, c.id, "storage_cap:"+f.GoodKey, "false", next, tick, kind, 3, body)
			if err != nil {
				return err
			}
			if n != nil {
				notices = append(notices, *n)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.deliverTransitions(ctx, notices)
	return nil
}
