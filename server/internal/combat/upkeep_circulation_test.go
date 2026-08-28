package combat

// Sold circulation (Del C) DB integration: a garrisoned unit's soldiers spend
// soldShare of their pay back into the town they hold (recorded in circulated_to),
// while a field unit (paid by the metropolis fallback) is a full sink. The
// affordability gate stays on the FULL upkeep; a town that can't cover it pays
// nothing (silver_unpaid, no partial). share=0 must be bit-identical to the
// pre-Del-C behaviour. Skips without DATABASE_URL.

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

func TestUpkeepSoldCirculation(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 2000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-sold-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-sold-"+uuid.New().String(), tick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	mkPlayer := func() uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			"sold-"+uuid.New().String(), "sold-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player: %v", err)
		}
		return id
	}
	mkSettlementAs := func(owner uuid.UUID, name string, q int, silver, silverCap float64, ctype string, isCapital bool) uuid.UUID {
		var prov uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
			 VALUES ($1, $2, $3, 'achaean', $4, $5, $6, 'active', 1000) RETURNING id`,
			worldID, prov, name, owner, ctype, isCapital,
		).Scan(&id); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		for _, g := range []struct {
			key         string
			amount, cap float64
		}{{"silver", silver, silverCap}, {"grain", 100000, 100000}} {
			if _, err := pool.Exec(ctx,
				`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
				 VALUES ($1, $2, $3, 0, $4, $5)`,
				id, g.key, g.amount, g.cap, tick,
			); err != nil {
				t.Fatalf("seed good %s: %v", g.key, err)
			}
		}
		return id
	}
	mkSettlement := func(owner uuid.UUID, name string, q int, silver, silverCap float64) uuid.UUID {
		return mkSettlementAs(owner, name, q, silver, silverCap, "capital", true)
	}
	// Since mig 100 the recruitment path sets BOTH settlement_id and
	// support_settlement_id, and support_settlement_id is the sole payer (the
	// silent capital fallback is gone). A home garrison therefore has the two
	// columns equal — that equality, not "settlement_id is set", is what makes
	// the sold circulate.
	mkGarrison := func(owner, sid uuid.UUID, utype string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
			 VALUES ($1, $2, $3, 'land', 100, 0, 'garrison', $4, $4)`,
			worldID, owner, utype, sid,
		); err != nil {
			t.Fatalf("create garrison %s: %v", utype, err)
		}
	}
	// Garrison stationed in a town that does NOT pay it (its metropolis does).
	// This is the fixture that separates the two discriminators — see the case
	// comment at pA below.
	mkGarrisonAway := func(owner, stands, payer uuid.UUID, utype string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
			 VALUES ($1, $2, $3, 'land', 100, 0, 'garrison', $4, $5)`,
			worldID, owner, utype, stands, payer,
		); err != nil {
			t.Fatalf("create away garrison %s: %v", utype, err)
		}
	}
	// Field unit: away from home, so settlement_id is NULL while its raising town
	// keeps paying it. Full sink.
	mkField := func(owner, support uuid.UUID, utype string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, support_settlement_id)
			 VALUES ($1, $2, $3, 'land', 100, 0, 'positioned', 5, 5, $4)`,
			worldID, owner, utype, support,
		); err != nil {
			t.Fatalf("create field %s: %v", utype, err)
		}
	}

	// pG: rich garrison town (spearman N=1, SLICE B halved table). pF: metropolis
	// whose field chariot (N=3) is a full sink. pP: poor garrison town, silver
	// 0 < N=1 → gate fails. (Before SLICE B, N=2 and this fixture seeded 1 silver
	// — insufficient under the old table but NOT under the halved one, since the
	// gate is >=; 0, not 1, is the seed that stays insufficient after halving.)
	pG, pF, pP := mkPlayer(), mkPlayer(), mkPlayer()
	sG := mkSettlement(pG, "Garrisontown", 0, 1000, 100000)
	sF := mkSettlement(pF, "Metropolis", 4, 1000, 100000)
	sP := mkSettlement(pP, "Poortown", 8, 0, 100000)
	mkGarrison(pG, sG, "spearman") // N=1, garrison at home → credit
	mkField(pF, sF, "war_chariot") // N=3, field (sF raised and pays it) → full sink
	mkGarrison(pP, sP, "spearman") // N=1 but 0 silver → unpaid

	// pA — the discriminating case. This unit garrisons a colony that does NOT
	// pay it; its metropolis does. Both other garrison fixtures have
	// settlement_id == support_settlement_id, so they pass under EITHER rule and
	// prove nothing about which one is in force. Here the two rules disagree:
	// the plan's original "settlement_id is set" would credit sM — a town the
	// soldiers are not standing in — while the mig-100 rule (stands in its own
	// paying town) makes it a full sink. Without this case the discriminator is
	// untested.
	pA := mkPlayer()
	sM := mkSettlement(pA, "Metropolis-A", 12, 1000, 100000)
	sC := mkSettlementAs(pA, "Colony-A", 16, 500, 100000, "colony", false)
	mkGarrisonAway(pA, sC, sM, "spearman") // N=1, stands in sC, paid by sM

	silverOf := func(sid uuid.UUID) float64 {
		var v float64
		if err := pool.QueryRow(ctx,
			`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, sid,
		).Scan(&v); err != nil {
			t.Fatalf("read silver: %v", err)
		}
		return v
	}
	type settled struct {
		paid, unpaid                         int
		gross, circ, destroyed, unpaidSilver float64
		circulatedTo                         string
	}
	readSettled := func(sid uuid.UUID) settled {
		var s settled
		if err := pool.QueryRow(ctx,
			`SELECT (payload->>'units_paid')::int, (payload->>'units_unpaid')::int,
			        (payload->>'silver_gross')::float, (payload->>'silver_circulated')::float,
			        (payload->>'silver_destroyed')::float, (payload->>'silver_unpaid')::float,
			        (payload->'circulated_to')::text
			 FROM events WHERE world_id = $1 AND event_type = 'UpkeepSettled' AND stream_id = $2`,
			worldID, sid,
		).Scan(&s.paid, &s.unpaid, &s.gross, &s.circ, &s.destroyed, &s.unpaidSilver, &s.circulatedTo); err != nil {
			t.Fatalf("read UpkeepSettled %s: %v", sid, err)
		}
		return s
	}
	approx := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

	store := events.NewStore(pool)
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	h := NewUpkeepHandler(pool, sched, store, nil, nil)
	h.soldShare = 0.7

	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	// Garrison: net debit (1−0.7)·1 = 0.3 → 1000 − 0.3 = 999.7. circ 0.7, destr 0.3.
	// circulated_to records the recipient (= the paying town, MVP payer).
	if got := silverOf(sG); !approx(got, 999.7) {
		t.Errorf("garrison town silver = %.4f, want 999.7 (net (1−share)·N)", got)
	}
	if s := readSettled(sG); s.paid != 1 || !approx(s.gross, 1) || !approx(s.circ, 0.7) || !approx(s.destroyed, 0.3) {
		t.Errorf("garrison UpkeepSettled = %+v, want {paid1 gross1 circ0.7 destr0.3}", s)
	}
	var circMap float64
	if err := pool.QueryRow(ctx,
		`SELECT (payload->'circulated_to'->>$3::text)::float FROM events
		 WHERE world_id = $1 AND event_type = 'UpkeepSettled' AND stream_id = $2`,
		worldID, sG, sG.String(),
	).Scan(&circMap); err != nil || !approx(circMap, 0.7) {
		t.Errorf("garrison circulated_to[%s] = %.4f (err %v), want 0.7", sG, circMap, err)
	}
	// Field: full sink. sF (capital) pays 3 → 997. circ 0, destroyed 3, empty map.
	if got := silverOf(sF); !approx(got, 997) {
		t.Errorf("metropolis silver = %.4f, want 997 (field full sink)", got)
	}
	if s := readSettled(sF); s.paid != 1 || !approx(s.circ, 0) || !approx(s.destroyed, 3) || s.circulatedTo != "{}" {
		t.Errorf("field UpkeepSettled = %+v, want {paid1 circ0 destr3 circulatedTo {}}", s)
	}
	// Gate on full N: poor town can't cover N=1 with 0 silver → unpaid, no partial.
	// silver_unpaid records the stopped amount; silver stays untouched.
	if got := silverOf(sP); !approx(got, 0) {
		t.Errorf("poor town silver = %.4f, want 0 (untouched — no partial pay)", got)
	}
	if s := readSettled(sP); s.unpaid != 1 || s.paid != 0 || !approx(s.gross, 0) || !approx(s.unpaidSilver, 1) {
		t.Errorf("poor town UpkeepSettled = %+v, want {unpaid1 paid0 gross0 unpaidSilver1}", s)
	}
	// Garrison away from its payer: full sink. The metropolis pays all of N=1
	// (1000 → 999) and the colony it stands in receives nothing (500 untouched).
	// Under the plan's original discriminator sM would instead have been credited
	// and read a smaller debit — that difference is what this case exists to catch.
	if got := silverOf(sM); !approx(got, 999) {
		t.Errorf("metropolis-A silver = %.4f, want 999 (full sink — unit not standing in its payer)", got)
	}
	if got := silverOf(sC); !approx(got, 500) {
		t.Errorf("colony-A silver = %.4f, want 500 (untouched — it does not pay the garrison)", got)
	}
	if s := readSettled(sM); s.paid != 1 || !approx(s.gross, 1) || !approx(s.circ, 0) || !approx(s.destroyed, 1) || s.circulatedTo != "{}" {
		t.Errorf("away-garrison UpkeepSettled = %+v, want {paid1 gross1 circ0 destr1 circulatedTo {}}", s)
	}
}

// TestUpkeepSoldShareZeroIdentity: with soldShare=0 a garrisoned unit's whole
// upkeep leaves the world (no credit) — bit-identical to the pre-Del-C behaviour.
func TestUpkeepSoldShareZeroIdentity(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const tick = 3000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-sold0-%'`,
	); err != nil {
		t.Fatalf("archive leftover worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-sold0-"+uuid.New().String(), tick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var owner uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"sold0-"+uuid.New().String(), "sold0-"+uuid.New().String()+"@test.invalid",
	).Scan(&owner); err != nil {
		t.Fatalf("create player: %v", err)
	}
	var prov, sid uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`, worldID,
	).Scan(&prov); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Zero', 'achaean', $3, 'capital', true, 'active', 1000) RETURNING id`,
		worldID, prov, owner,
	).Scan(&sid); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	for _, g := range []struct {
		key         string
		amount, cap float64
	}{{"silver", 1000, 100000}, {"grain", 100000, 100000}} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1, $2, $3, 0, $4, $5)`,
			sid, g.key, g.amount, g.cap, tick,
		); err != nil {
			t.Fatalf("seed good: %v", g.key)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id, support_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', $3, $3)`,
		worldID, owner, sid,
	); err != nil {
		t.Fatalf("create garrison: %v", err)
	}

	h := NewUpkeepHandler(pool, events.NewScheduler(pool, clock.NewTestClock(time.Now())), events.NewStore(pool), nil, nil)
	h.soldShare = 0
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	var silver, circ, destroyed float64
	if err := pool.QueryRow(ctx,
		`SELECT settled(amount, rate, calc_tick) FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`, sid,
	).Scan(&silver); err != nil {
		t.Fatalf("read silver: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT (payload->>'silver_circulated')::float, (payload->>'silver_destroyed')::float
		 FROM events WHERE world_id = $1 AND event_type = 'UpkeepSettled' AND stream_id = $2`, worldID, sid,
	).Scan(&circ, &destroyed); err != nil {
		t.Fatalf("read UpkeepSettled: %v", err)
	}
	if math.Abs(silver-999) > 1e-6 { // 1000 − full N=1 (SLICE B: spearman silver halved), no credit
		t.Errorf("share=0 silver = %.4f, want 999 (whole upkeep destroyed)", silver)
	}
	if math.Abs(circ) > 1e-6 || math.Abs(destroyed-1) > 1e-6 {
		t.Errorf("share=0 circ/destroyed = %.4f/%.4f, want 0/1", circ, destroyed)
	}
}
