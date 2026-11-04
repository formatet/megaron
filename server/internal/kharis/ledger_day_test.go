package kharis

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/ledger"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ledgerClose(t *testing.T, pool *pgxpool.Pool, wid uuid.UUID) {
	t.Helper()
	if err := ledger.CloseDay(context.Background(), pool, wid, atomic.AddInt64(&advanceOneDayEventID, 1)); err != nil {
		t.Fatalf("ledger close: %v", err)
	}
}

func dayPosts(d ledger.Day, good string) map[string]float64 {
	out := map[string]float64{}
	for _, g := range d.Goods {
		if g.Good == good {
			for _, p := range g.Posts {
				out[p.Category] = p.Amount
			}
		}
	}
	return out
}

// ledgerCity: an army, three temples (three gods), a herd and a gift, played over
// several real days through the real food tick, tithe, ration and credit functions.
func ledgerCity(t *testing.T) (*pgxpool.Pool, uuid.UUID, uuid.UUID, *TickHandler) {
	pool, wid, sid := newGrowthFixture(t, [6]string{"plains", "plains", "plains", "plains", "plains", "plains"}, 1000)
	ctx := context.Background()
	h := newTestTickHandler(pool)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES ($1,'temple',1,'poseidon'),($1,'temple',1,'demeter')`, sid)
	exec(`DELETE FROM settlement_placement WHERE settlement_id=$1`, sid)
	exec(`INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key,building_id)
	      SELECT $1, row_number() OVER (), 'building','temple','cult', b.id
	        FROM buildings b, generate_series(1,2) WHERE b.settlement_id=$1 AND b.building_type='temple'`, sid)
	exec(`INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick)
	      SELECT $1,key,0,0,1,current_world_tick() FROM goods WHERE key IN ('grain','fish','timber','silver','incense')`, wid)
	for g, a := range map[string]float64{"grain": 900, "silver": 400, "incense": 50, "livestock": 20} {
		exec(`INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,$3,0,5000,current_world_tick())
		      ON CONFLICT(settlement_id,good_key) DO UPDATE SET amount=$3,rate=CASE WHEN $2='grain' THEN 6 ELSE 0 END,cap=5000,calc_tick=current_world_tick()`, sid, g, a)
	}
	exec(`UPDATE settlement_goods SET rate=6 WHERE settlement_id=$1 AND good_key='grain'`, sid)
	return pool, wid, sid, h
}

func playLedgerDay(t *testing.T, pool *pgxpool.Pool, wid, sid uuid.UUID, h *TickHandler, gift bool) {
	t.Helper()
	ctx := context.Background()
	advanceOneDay(t, h, pool, wid)
	values := map[string]float64{"grain": 1, "fish": 1, "timber": 1, "silver": 1, "incense": 1}
	if _, err := h.chargeTempleOffering(ctx, sid, values); err != nil {
		t.Fatalf("tithe: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = ledger.Reason(ctx, tx, "army_ration")
	if _, err := economy.TakeFood(ctx, tx, sid, 3, false, true); err != nil {
		t.Fatalf("army ration: %v", err)
	}
	if gift {
		if _, err := tx.Exec(ctx, `SELECT credit_settlement_good($1,'silver',25,'gift_delivery')`, sid); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	ledgerClose(t, pool, wid)
}

// TestLedgerDay_OtherIsZeroWithArmyThreeTemplesHerdAndGift: every change of the stock
// is explained by a post, so Other vanishes — and the posts are the real ones.
func TestLedgerDay_OtherIsZeroWithArmyThreeTemplesHerdAndGift(t *testing.T) {
	pool, wid, sid, h := ledgerCity(t)
	ctx := context.Background()
	ledgerClose(t, pool, wid) // opening
	for day := 0; day < 12; day++ {
		playLedgerDay(t, pool, wid, sid, h, day == 11)
	}
	d, err := ledger.LastDay(ctx, pool, sid)
	if err != nil || !d.Known {
		t.Fatalf("day known=%v err=%v", d.Known, err)
	}
	for _, g := range d.Goods {
		if g.Other != 0 {
			t.Errorf("%s: Other = %.4f, want 0 (delta %.4f, posts %+v)", g.Good, g.Other, g.Delta, g.Posts)
		}
	}
	grain := dayPosts(d, "grain")
	for _, want := range []string{"production", "meal", "army_ration"} {
		if grain[want] == 0 {
			t.Errorf("grain posts %v lack %q", grain, want)
		}
	}
	tithes := 0
	for _, g := range d.Goods {
		for _, p := range g.Posts {
			if len(p.Category) > 12 && p.Category[:12] == "temple_tithe" {
				tithes++
			}
		}
	}
	if tithes < 3 {
		t.Errorf("temple_tithe posts = %d, want at least 3 (one per god, across goods); day %+v", tithes, d.Goods)
	}
	if dayPosts(d, "silver")["gift_delivery"] != 25 {
		t.Errorf("silver posts %v lack gift_delivery 25", dayPosts(d, "silver"))
	}
}

// TestLedgerDay_UnknownDebitShowsAsOther: a debit that names no category cannot hide.
func TestLedgerDay_UnknownDebitShowsAsOther(t *testing.T) {
	pool, wid, sid, h := ledgerCity(t)
	ctx := context.Background()
	ledgerClose(t, pool, wid)
	playLedgerDay(t, pool, wid, sid, h, false)
	advanceOneDay(t, h, pool, wid)
	if _, err := pool.Exec(ctx, `UPDATE settlement_goods SET amount=settled(amount,rate,calc_tick)-7, calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key='grain'`, sid); err != nil {
		t.Fatal(err)
	}
	ledgerClose(t, pool, wid)
	d, err := ledger.LastDay(ctx, pool, sid)
	if err != nil || !d.Known {
		t.Fatalf("known=%v err=%v", d.Known, err)
	}
	var other float64
	for _, g := range d.Goods {
		if g.Good == "grain" {
			other = g.Other
		}
	}
	// The posted categories of an ordinary day sum to the rest; the hidden -7 is what remains.
	if math.Abs(other+7) > 0.05 {
		t.Errorf("grain Other = %.4f, want -7 (the unlabelled debit)", other)
	}
}
