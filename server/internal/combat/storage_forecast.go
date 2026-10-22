package combat

import (
	"context"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"slices"
	"sort"
)

type StorageForecaster struct{}

// StorageForecast composes the shared food split and upkeep owner. The caller's
// transaction gives the observer and read surface the same snapshot.
func (StorageForecaster) StorageForecast(ctx context.Context, tx pgx.Tx, sid uuid.UUID) ([]economy.StorageForecast, error) {
	var population, tick int
	if err := tx.QueryRow(ctx, `SELECT s.population,w.current_tick FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.id=$1`, sid).Scan(&population, &tick); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT good_key,GREATEST(0,LEAST(cap,settled(amount,rate,calc_tick))),rate,cap FROM settlement_goods WHERE settlement_id=$1 ORDER BY good_key`, sid)
	if err != nil {
		return nil, err
	}
	var result []economy.StorageForecast
	stock := map[string]float64{}
	for rows.Next() {
		var f economy.StorageForecast
		if err := rows.Scan(&f.GoodKey, &f.Amount, &f.NetPerTick, &f.Cap); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, f)
		stock[f.GoodKey] = f.Amount
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Use the same billing set, payer and field ration as UpkeepHandler. Units
	// consuming ship provisions do not draw food from a distant home store.
	units, err := tx.Query(ctx, `SELECT u.id,u.type,u.category,u.size,u.status,u.crew,u.settlement_id,
 (SELECT c.id FROM units c WHERE c.cargo_unit_id=u.id AND c.world_id=u.world_id)
 FROM units u JOIN settlements s ON s.id=u.support_settlement_id AND s.owner_id=u.owner_id
 WHERE s.id=$1 AND u.status IN('garrison','marching','positioned','embarked','repairing','freighting','fitting_out')
 AND NOT EXISTS(SELECT 1 FROM founder_phase f WHERE f.world_id=u.world_id AND f.owner_id=u.owner_id AND f.active)`, sid)
	if err != nil {
		return nil, err
	}
	foodNeed := float64(population) * economy.GrainConsumptionPerCitizenPerTick
	silverNeed := 0.
	for units.Next() {
		var u upkeepUnitRow
		if err := units.Scan(&u.id, &u.unitType, &u.category, &u.size, &u.status, &u.crew, &u.settlementID, &u.carrierID); err != nil {
			units.Close()
			return nil, err
		}
		need := UnitUpkeep(u.unitType, u.category, u.size, u.status, u.crew)
		if provisionSourceFor(u) == nil || atHomePort(u) {
			foodNeed += need.Grain
		}
		silverNeed += need.Silver
	}
	units.Close()
	if err := units.Err(); err != nil {
		return nil, err
	}
	// Production available before the next meal joins stock in the same split.
	available := map[string]float64{}
	for _, f := range result {
		available[f.GoodKey] = f.Amount + max(0, f.NetPerTick)
	}
	meal := economy.PlanFood(foodNeed, available, false)
	receipts := map[string][]economy.StorageReceipt{}
	incoming, err := tx.Query(ctx, `SELECT tg.good_key,GREATEST(0,t.due_tick-$2),tg.quantity
 FROM transports t JOIN transport_goods tg ON tg.transport_id=t.id JOIN settlements s ON s.id=t.dest_id
 WHERE t.dest_id=$1 AND t.status='in_transit' AND (t.owner_id=s.owner_id OR t.kind IN('trade','trade_return'))
 ORDER BY t.due_tick,t.id,tg.good_key`, sid, tick)
	if err != nil {
		return nil, err
	}
	for incoming.Next() {
		var good string
		var r economy.StorageReceipt
		if err := incoming.Scan(&good, &r.InTicks, &r.Quantity); err != nil {
			incoming.Close()
			return nil, err
		}
		receipts[good] = append(receipts[good], r)
	}
	incoming.Close()
	if err := incoming.Err(); err != nil {
		return nil, err
	}
	for i, f := range result {
		net := f.NetPerTick - meal.Used[f.GoodKey]
		if f.GoodKey == economy.GoodSilver {
			net -= silverNeed
		}
		result[i] = economy.ProjectStorage(f.GoodKey, f.Amount, f.Cap, net, receipts[f.GoodKey])
		delete(receipts, f.GoodKey)
	}
	// A commodity not stored yet can still arrive and fill a default-cap row.
	for good, rs := range receipts {
		storeCap := province.DefaultGoodStorageCap
		if slices.Contains(economy.FoodGoods, good) {
			storeCap = province.FoodStorageCap(economy.SiloLevel(ctx, tx, sid))
		}
		result = append(result, economy.ProjectStorage(good, 0, storeCap, 0, rs))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].GoodKey < result[j].GoodKey })
	return result, nil
}
