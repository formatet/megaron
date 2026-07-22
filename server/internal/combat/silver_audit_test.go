package combat

// Silver flow bookkeeping (Del A) DB integration: one daily upkeep tick over two
// settlements (one with a silver mine + garrison, one plain garrison) plus a
// pending buy offer must produce (a) one UpkeepSettled per paying settlement whose
// aggregate matches the per-unit facit, and (b) one world SilverAudit whose stocks
// (liquid/fund/escrow) and mined-since-last window are exact. Skips without
// DATABASE_URL, like the other combat integration tests.

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// auditRow mirrors the SilverAudit payload fields the test reads back.
type auditRow struct{ liquid, fund, escrow, mined, prev, delta float64 }

func (a auditRow) total() float64 { return a.liquid + a.fund + a.escrow }

func TestUpkeepSilverBookkeeping(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	const tick = 1000

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active' AND name LIKE 'test-silver-%'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-silver-"+uuid.New().String(), tick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	mkPlayer := func() uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			"silver-"+uuid.New().String(), "silver-"+uuid.New().String()+"@test.invalid",
		).Scan(&id); err != nil {
			t.Fatalf("create player: %v", err)
		}
		return id
	}

	mkSettlement := func(owner uuid.UUID, name string, q int, fund float64) uuid.UUID {
		var prov uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, 0, 'plains') RETURNING id`,
			worldID, q,
		).Scan(&prov); err != nil {
			t.Fatalf("create province %s: %v", name, err)
		}
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, sitos_fund_silver)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true, 'active', 1000, $5) RETURNING id`,
			worldID, prov, name, owner, fund,
		).Scan(&id); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return id
	}

	mkGood := func(sid uuid.UUID, key string, amount, rate, cap float64) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			sid, key, amount, rate, cap, tick,
		); err != nil {
			t.Fatalf("seed good %s: %v", key, err)
		}
	}

	mkUnit := func(owner, sid uuid.UUID, utype string, size int) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, $3, 'land', $4, 0, 'garrison', $5)`,
			worldID, owner, utype, size, sid,
		); err != nil {
			t.Fatalf("create unit %s: %v", utype, err)
		}
	}

	p1, p2 := mkPlayer(), mkPlayer()
	// A mines silver (rate 5); B does not. Both hold plenty of grain + silver so
	// the units all pay in full — the audit measures a solvent world.
	sA := mkSettlement(p1, "Argyros", 0, 500) // fund 500
	sB := mkSettlement(p2, "Bare", 4, 300)    // fund 300
	mkGood(sA, "silver", 10000, 5, 100000)
	mkGood(sA, "grain", 10000, 0, 100000)
	mkGood(sB, "silver", 10000, 0, 100000)
	mkGood(sB, "grain", 10000, 0, 100000)
	mkUnit(p1, sA, "spearman", 100)    // grain 5, silver 2
	mkUnit(p2, sB, "war_chariot", 100) // grain 8, silver 6

	// Escrow: one pending BUY offer holds 250 silver. A SELL offer (escrows goods)
	// and a non-pending offer must both be ignored by the audit.
	mkOffer := func(sender, origin, dest uuid.UUID, offerJSON string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, hex_q, hex_r, arrives_at, trade_offer)
			 VALUES ($1, $2, $3, $4, 'offer', 0, 0, now(), $5::jsonb)`,
			worldID, sender, origin, dest, offerJSON,
		); err != nil {
			t.Fatalf("create offer: %v", err)
		}
	}
	mkOffer(p1, sA, sB, `{"status":"pending","kind":"buy","offer_silver":250}`)
	mkOffer(p1, sA, sB, `{"status":"pending","kind":"sell","offer_silver":999,"offer_good":"grain","offer_qty":10}`)
	mkOffer(p2, sB, sA, `{"status":"expired","kind":"buy","offer_silver":777}`)

	store := events.NewStore(pool)
	sched := events.NewScheduler(pool, clock.NewTestClock(time.Now()))
	h := NewUpkeepHandler(pool, sched, store, nil)

	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick}); err != nil {
		t.Fatalf("upkeep Handle: %v", err)
	}

	// ── UpkeepSettled per settlement matches the per-unit facit. ──
	type settled struct {
		paid, unpaid                  int
		grain, gross, circ, destroyed float64
		unpaidSilver                  float64
		circulatedTo                  string
	}
	readSettled := func(sid uuid.UUID) settled {
		var s settled
		if err := pool.QueryRow(ctx,
			`SELECT (payload->>'units_paid')::int, (payload->>'units_unpaid')::int,
			        (payload->>'grain_total')::float, (payload->>'silver_gross')::float,
			        (payload->>'silver_circulated')::float, (payload->>'silver_destroyed')::float,
			        (payload->>'silver_unpaid')::float, (payload->'circulated_to')::text
			 FROM events WHERE world_id = $1 AND event_type = 'UpkeepSettled' AND stream_id = $2`,
			worldID, sid,
		).Scan(&s.paid, &s.unpaid, &s.grain, &s.gross, &s.circ, &s.destroyed, &s.unpaidSilver, &s.circulatedTo); err != nil {
			t.Fatalf("read UpkeepSettled for %s: %v", sid, err)
		}
		return s
	}
	wantSettled := func(name string, got, want settled) {
		if got != want {
			t.Errorf("%s UpkeepSettled = %+v, want %+v", name, got, want)
		}
	}
	// Both units pay in full → no circulation (no soldShare), no unpaid, empty map.
	wantSettled("Argyros", readSettled(sA), settled{paid: 1, unpaid: 0, grain: 5, gross: 2, circ: 0, destroyed: 2, unpaidSilver: 0, circulatedTo: "{}"})
	wantSettled("Bare", readSettled(sB), settled{paid: 1, unpaid: 0, grain: 8, gross: 6, circ: 0, destroyed: 6, unpaidSilver: 0, circulatedTo: "{}"})

	// ── SilverAudit stocks (first audit ⇒ no prev, mined 0, delta 0). ──
	// A: 10000 − 2 = 9998; B: 10000 − 6 = 9994 → liquid 19992. fund 500+300=800.
	// escrow = the single pending BUY offer = 250.
	readAudit := func() auditRow {
		var a auditRow
		if err := pool.QueryRow(ctx,
			`SELECT (payload->>'liquid_total')::float, (payload->>'fund_total')::float,
			        (payload->>'escrow_total')::float, (payload->>'mined_since_last')::float,
			        (payload->>'audit_prev_total')::float, (payload->>'net_delta')::float
			 FROM events WHERE world_id = $1 AND event_type = 'SilverAudit' ORDER BY id DESC LIMIT 1`,
			worldID,
		).Scan(&a.liquid, &a.fund, &a.escrow, &a.mined, &a.prev, &a.delta); err != nil {
			t.Fatalf("read SilverAudit: %v", err)
		}
		return a
	}
	approx := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }
	a1 := readAudit()
	if !approx(a1.liquid, 19992) || !approx(a1.fund, 800) || !approx(a1.escrow, 250) {
		t.Errorf("audit1 stocks = liquid %.2f fund %.2f escrow %.2f, want 19992/800/250", a1.liquid, a1.fund, a1.escrow)
	}
	if !approx(a1.mined, 0) || !approx(a1.delta, 0) {
		t.Errorf("audit1 mined/delta = %.4f/%.4f, want 0/0 (first audit)", a1.mined, a1.delta)
	}

	// ── mined_since_last window: advance 10 ticks, re-audit directly. Only A mines
	// (rate 5) → mined = 5 × 10 = 50; liquid grows by the same 50; delta = 50. ──
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`, worldID, tick+10); err != nil {
		t.Fatalf("advance tick: %v", err)
	}
	h.emitSilverAudit(ctx, worldID)
	a2 := readAudit()
	if !approx(a2.mined, 50) {
		t.Errorf("audit2 mined = %.4f, want 50 (rate 5 × 10 ticks)", a2.mined)
	}
	if !approx(a2.prev, a1.total()) {
		t.Errorf("audit2 prev = %.4f, want %.4f (= audit1 total)", a2.prev, a1.total())
	}
	if !approx(a2.delta, 50) {
		t.Errorf("audit2 delta = %.4f, want 50 (only mined inflow)", a2.delta)
	}
}
