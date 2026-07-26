package combat

// De tre bevisen för §3.1 i megaron_aktorer_plan.md — den försörjande staden.
//
// Före mig 100 fanns ingen stabil sanning om vilken stad som betalar ett
// förband: payingSettlement() returnerade units.settlement_id om den var satt
// och annars ÄGARENS HUVUDSTAD som tyst fallback. Ett skepp till sjöss och varje
// marscherande enhet debiterades alltså huvudstaden av misstag, inte av design.
// Den betalande staden var ingen designad sanning; den var en tystnad.
//
// Testerna nedan bevisar att tystnaden är borta:
//  1. ett skepp till sjöss debiteras sin support_settlement_id, INTE huvudstaden
//  2. en enhet vars försörjande stad fallit får unpaid_periods uppräknad och
//     deserterar via befintlig applyAttrition — ingen tyst omdirigering
//  3. ordinalen återanvänds inte: 3 rekryter, upplös nr 2, rekrytera → 4th

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// supportFixture reser en värld med en spelare, en huvudstad och en andra stad.
// Båda har gott om spannmål; silvret sätts per test.
type supportFixture struct {
	worldID, owner    uuid.UUID
	capitalID, townID uuid.UUID
	tick              int
}

func newSupportFixture(t *testing.T, pool *pgxpool.Pool, tag string) supportFixture {
	t.Helper()
	ctx := context.Background()
	const tick = 3000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE $1`,
		"test-"+tag+"-%",
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	f := supportFixture{tick: tick}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-"+tag+"-"+uuid.New().String(), tick,
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		tag+"-"+uuid.New().String(), tag+"-"+uuid.New().String()+"@test.invalid",
	).Scan(&f.owner); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mk := func(q int, name string, capital bool) uuid.UUID {
		var prov, sid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			f.worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province: %v", err)
		}
		ctype := "colony"
		if capital {
			ctype = "capital"
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, $5, $6, 'active', 1000) RETURNING id`,
			f.worldID, prov, name, f.owner, ctype, capital,
		).Scan(&sid); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return sid
	}
	f.capitalID = mk(0, "Knossos", true)
	f.townID = mk(4, "Kydonia", false)
	return f
}

func seedGoods(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, tick, grain, silver int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', $2, 0, 1000000, $4), ($1, 'silver', $3, 0, 1000000, $4)`,
		sid, grain, silver, tick,
	); err != nil {
		t.Fatalf("seed goods: %v", err)
	}
}

func settledAmount(t *testing.T, pool *pgxpool.Pool, sid uuid.UUID, good string) float64 {
	t.Helper()
	var v float64
	if err := pool.QueryRow(context.Background(),
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods
		  WHERE settlement_id = $1 AND good_key = $2`, sid, good,
	).Scan(&v); err != nil {
		t.Fatalf("read %s: %v", good, err)
	}
	return v
}

func runUpkeep(t *testing.T, pool *pgxpool.Pool, f supportFixture) {
	t.Helper()
	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		events.NewStore(pool), &fakeBroadcaster{})
	if err := h.Handle(context.Background(),
		events.ScheduledEvent{WorldID: f.worldID, DueTick: f.tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}
}

// 1. Ett skepp till sjöss (settlement_id NULL) ska debiteras sin FÖRSÖRJANDE
// stad, inte huvudstaden. Det var precis den här tysta omdirigeringen som gjorde
// den betalande staden till en gissning i stället för ett faktum.
func TestUpkeep_ShipAtSeaBilledToSupportSettlement_NotCapital(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "support-ship")

	seedGoods(t, pool, f.capitalID, f.tick, 100000, 100000)
	seedGoods(t, pool, f.townID, f.tick, 100000, 100000)

	// Skeppet står inte i någon stad (positioned = till sjöss) men försörjs av
	// Kydonia. Före mig 100 hade huvudstaden Knossos betalat det.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, support_settlement_id)
		 VALUES ($1, $2, 'war_galley', 'naval', 1, 50, 'positioned', 9, 0, $3)`,
		f.worldID, f.owner, f.townID,
	); err != nil {
		t.Fatalf("create ship: %v", err)
	}

	capBefore := settledAmount(t, pool, f.capitalID, "silver")
	townBefore := settledAmount(t, pool, f.townID, "silver")

	runUpkeep(t, pool, f)

	capAfter := settledAmount(t, pool, f.capitalID, "silver")
	townAfter := settledAmount(t, pool, f.townID, "silver")

	if capAfter != capBefore {
		t.Errorf("huvudstaden debiterades %.1f silver för ett skepp den inte försörjer — "+
			"huvudstadsfallbacken lever kvar", capBefore-capAfter)
	}
	if townAfter >= townBefore {
		t.Errorf("den försörjande staden debiterades inte: %.1f → %.1f", townBefore, townAfter)
	}
}

// 2. Faller den försörjande staden betalas ingen sold. Ingen tyst omdirigering
// till huvudstaden — enheten går genom unpaid_periods och applyAttrition, samma
// maskineri som all annan uteblivning. Det finns alltså ingen väg att rädda ett
// förband vars stad fallit, och det är avsiktligt (§3.1 punkt 1 och 3).
func TestUpkeep_FallenSupportSettlement_UnitDesertsInsteadOfBillingCapital(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "support-fallen")

	seedGoods(t, pool, f.capitalID, f.tick, 100000, 100000)
	seedGoods(t, pool, f.townID, f.tick, 100000, 100000)

	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    q, r, support_settlement_id, unpaid_periods)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'positioned', 9, 0, $3, 0)
		 RETURNING id`,
		f.worldID, f.owner, f.townID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create unit: %v", err)
	}

	// Staden faller: erövrad av någon annan. Raden finns kvar, ägaren är en annan
	// — vilket är precis vad queryns korrelerade subselect ska fånga.
	var enemy uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"conqueror-"+uuid.New().String(), "conqueror-"+uuid.New().String()+"@test.invalid",
	).Scan(&enemy); err != nil {
		t.Fatalf("create conqueror: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE settlements SET owner_id = $1 WHERE id = $2`, enemy, f.townID); err != nil {
		t.Fatalf("capture settlement: %v", err)
	}

	capBefore := settledAmount(t, pool, f.capitalID, "grain")
	sizeBefore := 100

	runUpkeep(t, pool, f)

	if after := settledAmount(t, pool, f.capitalID, "grain"); after != capBefore {
		t.Errorf("huvudstaden debiterades %.1f spannmål för ett förband vars stad fallit — "+
			"den tysta omdirigeringen lever kvar", capBefore-after)
	}
	var size int
	if err := pool.QueryRow(ctx, `SELECT size FROM units WHERE id = $1`, unitID).Scan(&size); err != nil {
		t.Fatalf("read unit size: %v", err)
	}
	if size >= sizeBefore {
		t.Errorf("förbandet blödde inte män trots utebliven försörjning: %d → %d", sizeBefore, size)
	}
}

// 3. Ordinalen återanvänds ALDRIG. Rekrytera tre, upplös nr 2, rekrytera igen →
// 4th, inte 2nd. Timothy 2026-07-26: "historiska regementen får isf vara
// historiska" — numret är organisatorisk historia, inte en ledig plats.
func TestAllocateOrdinal_NeverReusesANumber(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newSupportFixture(t, pool, "support-ordinal")

	var got []int
	for i := 0; i < 3; i++ {
		n, err := unit.AllocateOrdinal(ctx, pool, f.townID, "spearman")
		if err != nil {
			t.Fatalf("allocate %d: %v", i, err)
		}
		got = append(got, n)
	}
	if got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("första tre ordinalerna = %v, vill ha [1 2 3]", got)
	}

	// "Upplös nr 2": enheten försvinner, men numret gör det inte.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, ordinal)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'disbanded', $3, $3, 2)`,
		f.worldID, f.owner, f.townID,
	); err != nil {
		t.Fatalf("insert disbanded unit: %v", err)
	}

	next, err := unit.AllocateOrdinal(ctx, pool, f.townID, "spearman")
	if err != nil {
		t.Fatalf("allocate after disband: %v", err)
	}
	if next != 4 {
		t.Errorf("nästa ordinal = %d, vill ha 4 — numret 2 återanvändes", next)
	}

	// Räknaren är per (stad, typ): en annan typ i samma stad börjar om på 1.
	other, err := unit.AllocateOrdinal(ctx, pool, f.townID, "war_chariot")
	if err != nil {
		t.Fatalf("allocate other type: %v", err)
	}
	if other != 1 {
		t.Errorf("ny typ i samma stad = %d, vill ha 1", other)
	}
}
