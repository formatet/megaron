package combat

import (
	"context"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/ledger"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"slices"
	"sort"
)

// StorageForecaster is the forward owner of a city's goods. Tithe is wired by
// main (it lives in kharis, which combat may not import): tomorrow's tithe per
// god and good. A nil Tithe forecasts none.
type StorageForecaster struct {
	Tithe func(ctx context.Context, tx pgx.Tx, sid uuid.UUID) (map[string]map[string]float64, error)
}

// CityForecast is the expected next game day of every good, post by post, in the
// ledger's categories — the same posts the books record backward. It composes the
// shared food split, the upkeep owner and the temple plan; nothing is re-derived
// by a surface. The caller's transaction gives every reader one snapshot.
func (f StorageForecaster) CityForecast(ctx context.Context, tx pgx.Tx, sid uuid.UUID) (economy.CityForecast, error) {
	var out economy.CityForecast
	var population int
	if err := tx.QueryRow(ctx, `SELECT s.population FROM settlements s WHERE s.id=$1`, sid).Scan(&population); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT good_key,GREATEST(0,LEAST(cap,settled(amount,rate,calc_tick))),rate FROM settlement_goods WHERE settlement_id=$1 ORDER BY good_key`, sid)
	if err != nil {
		return out, err
	}
	stock, rate := map[string]float64{}, map[string]float64{}
	var keys []string
	for rows.Next() {
		var k string
		var a, r float64
		if err := rows.Scan(&k, &a, &r); err != nil {
			rows.Close()
			return out, err
		}
		stock[k], rate[k] = a, r
		keys = append(keys, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	// Same billing set, payer and field ration as UpkeepHandler. Units consuming
	// ship provisions do not draw food from a distant home store.
	units, err := tx.Query(ctx, `SELECT u.id,u.type,u.category,u.size,u.status,u.crew,u.settlement_id,
 u.carried_by_ship_id
 FROM units u JOIN settlements s ON s.id=u.support_settlement_id AND s.owner_id=u.owner_id
 WHERE s.id=$1 AND u.status IN('garrison','marching','positioned','embarked','repairing','freighting','fitting_out')
 AND NOT EXISTS(SELECT 1 FROM founder_phase f WHERE f.world_id=u.world_id AND f.owner_id=u.owner_id AND f.active)`, sid)
	if err != nil {
		return out, err
	}
	armyFood, armyWages := 0., 0.
	for units.Next() {
		var u upkeepUnitRow
		if err := units.Scan(&u.id, &u.unitType, &u.category, &u.size, &u.status, &u.crew, &u.settlementID, &u.carrierID); err != nil {
			units.Close()
			return out, err
		}
		need := UnitUpkeep(u.unitType, u.category, u.size, u.status, u.crew)
		if provisionSourceFor(u) == nil || atHomePort(u) {
			armyFood += need.Grain
		}
		armyWages += need.Silver
	}
	units.Close()
	if err := units.Err(); err != nil {
		return out, err
	}
	tithe := map[string]map[string]float64{}
	if f.Tithe != nil {
		if tithe, err = f.Tithe(ctx, tx, sid); err != nil {
			return out, err
		}
	}
	// Production available before the next meal joins stock in the same split. The
	// tick order is army first (upkeep), then the citizens' meal (food tick).
	available := map[string]float64{}
	for _, k := range keys {
		available[k] = stock[k] + max(0, rate[k])
	}
	army := economy.PlanFood(armyFood, available, false)
	for k, used := range army.Used {
		available[k] = max(0, available[k]-used)
	}
	civil := economy.PlanFood(float64(population)*economy.GrainConsumptionPerCitizenPerTick, available, false)

	flows := map[string][]economy.Flow{}
	add := func(good, cat string, amount float64) { flows[good] = economy.AddFlow(flows[good], cat, amount) }
	for _, k := range keys {
		add(k, ledger.Production, rate[k])
		add(k, "army_ration", -army.Used[k])
		add(k, "meal", -civil.Used[k])
	}
	add(economy.GoodSilver, "army_wages", -armyWages)
	gods := make([]string, 0, len(tithe))
	for god := range tithe {
		gods = append(gods, god)
	}
	sort.Strings(gods)
	for _, god := range gods {
		for good, amount := range tithe[god] {
			add(good, ledger.TempleTithe(god), -amount)
		}
	}
	food := economy.FoodForecast{}
	foodFlows := map[string]float64{}
	for _, k := range keys {
		fl := flows[k]
		economy.SortFlows(fl)
		net := 0.
		for _, x := range fl {
			net += x.Amount
		}
		out.Goods = append(out.Goods, economy.GoodForecast{GoodKey: k, Net: net, Flows: fl})
		if slices.Contains(economy.FoodGoods, k) {
			food.Stock += stock[k]
			food.Net += net
			for _, x := range fl {
				foodFlows[x.Category] += x.Amount
			}
		}
	}
	// The herd's overflow at the pen cap is food too (economy.HerdSurplusRate, Timothy
	// 2026-10-31) — the same term the growth gate adds.
	food.HerdSurplus = economy.HerdSurplusRate(stock[economy.GoodLivestock], economy.PenLevel(ctx, tx, sid))
	food.Net += food.HerdSurplus
	foodFlows["herd_surplus"] += food.HerdSurplus
	for cat, amount := range foodFlows {
		food.Flows = economy.AddFlow(food.Flows, cat, amount)
	}
	economy.SortFlows(food.Flows)
	food.Days = economy.FoodRunway(food.Stock, food.Net)
	out.Food = food
	return out, nil
}

// StorageForecast projects each good's store from the same forecast owner.
func (f StorageForecaster) StorageForecast(ctx context.Context, tx pgx.Tx, sid uuid.UUID) ([]economy.StorageForecast, error) {
	var tick int
	if err := tx.QueryRow(ctx, `SELECT w.current_tick FROM settlements s JOIN worlds w ON w.id=s.world_id WHERE s.id=$1`, sid).Scan(&tick); err != nil {
		return nil, err
	}
	city, err := f.CityForecast(ctx, tx, sid)
	if err != nil {
		return nil, err
	}
	net := map[string]float64{}
	for _, g := range city.Goods {
		net[g.GoodKey] = g.Net
	}
	rows, err := tx.Query(ctx, `SELECT good_key,GREATEST(0,LEAST(cap,settled(amount,rate,calc_tick))),cap FROM settlement_goods WHERE settlement_id=$1 ORDER BY good_key`, sid)
	if err != nil {
		return nil, err
	}
	var result []economy.StorageForecast
	for rows.Next() {
		var sf economy.StorageForecast
		if err := rows.Scan(&sf.GoodKey, &sf.Amount, &sf.Cap); err != nil {
			rows.Close()
			return nil, err
		}
		sf.NetPerTick = net[sf.GoodKey]
		result = append(result, sf)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
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
		net := f.NetPerTick
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
