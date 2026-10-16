package kharis

import (
	"context"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"math"
	"sync/atomic"
	"testing"
)

// Named change proof: the old price-scale grain value clips the early
// standing reference offering at 2x. Both read projection and actual maintenance
// now use the catalogue work reference, so measured plenty/shortage is respected.
func TestSilverCalibrationKharisFlowLeavesStalePriceClamp(t *testing.T) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	var pid uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, sid).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO player_world_records(player_id,world_id,settlement_id,kharis_amount,kharis_rate,kharis_calc_tick,founded_tick)
 VALUES($1,$2,$3,20,0,current_world_tick(),current_world_tick()-144)`, pid, wid, sid); err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"grain"} {
		if _, err := pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick)
  VALUES($1,$2,100,0,1000,current_world_tick()) ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=100,rate=0`, sid, g); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick)
  VALUES($1,$2,0,0,1.5,current_world_tick())`, wid, g); err != nil {
			t.Fatal(err)
		}
	}
	h := newTestTickHandler(pool)
	if factor := h.templeOfferScarcity(ctx, wid); factor != 1.5 {
		t.Fatalf("actual maintenance factor=%g want1.5, stale price denominator must be gone", factor)
	}
	net, has, dev, _, err := ProjectDailyNet(ctx, pool, pid, wid)
	if err != nil {
		t.Fatal(err)
	}
	want := .15*kharisPerTempleDay*1.5 - decayBas
	if !has || dev != .15 || math.Abs(net-want) > 1e-8 {
		t.Fatalf("projection %g devotion%g has%v want%g", net, dev, has, want)
	}
	// Simulate the old valuation scale's stale denominator to display the change.
	oldFactor := scarcityFromValues(map[string]float64{"grain": 129.6}, map[string]float64{"grain": 1}, nil)
	oldNet := .15*kharisPerTempleDay*oldFactor - decayBas
	if oldFactor != 2 || oldNet <= net {
		t.Fatalf("old price clamp %g should overstate catalogue-based net%g", oldNet, net)
	}
	// Charge the actual maintenance path and compare the persisted relationship.
	before := 20.0
	eid := atomic.AddInt64(&advanceOneDayEventID, 1)
	if err := h.processMaintenance(ctx, wanaxSnap{ticksSinceFounding: foundingGraceTicks, playerID: pid, settlementID: sid, kharis: before,
		kharisCap: 100, devotionSum: .15, templeCities: 1, maxTempleLevel: 1}, wid, eid); err != nil {
		t.Fatal(err)
	}
	var after float64
	if err := pool.QueryRow(ctx, `SELECT kharis_amount FROM player_world_records WHERE player_id=$1 AND world_id=$2`, pid, wid).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if math.Abs(after-(before+net)) > 1e-8 {
		t.Fatalf("persisted kharis %g want projected%g", after, before+net)
	}
	var unit string
	var version int
	if err := pool.QueryRow(ctx, `SELECT payload->>'offering_value_unit',(payload->>'valuation_version')::int
 FROM events WHERE world_id=$1 AND event_type='KharisOffering' ORDER BY id DESC LIMIT 1`, wid).Scan(&unit, &version); err != nil {
		t.Fatal(err)
	}
	if unit != religion.LaborValueUnit || version != 2 {
		t.Fatalf("standing cult event unit%q/version%d", unit, version)
	}
	// Change the shared catalogue reference: both consumers must observe it.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE goods SET labor_value=2 WHERE key='grain'`); err != nil {
		t.Fatal(err)
	}
	refs, err := religion.LoadLaborValues(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if got := scarcityFromValues(map[string]float64{"grain": 1.5}, refs, nil); got != 1 {
		t.Fatalf("catalogue reference not used: %g", got)
	}
}
