package transport

// Caravan interception (Del 3-fas-4 / movement-motor Slice C). A sentry posted on a
// trade route seizes a passing enemy caravan; the loot lands in the raider's capital.
// Crucially, a MESSENGER at the very same hex is untouched — messengers are sacred
// and the scan never even reads their table.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestInterceptScan_SeizesCaravanButNeverMessenger(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool) // world, owner, source@(0,0), dest@(3,0), land strip
	ctx := context.Background()

	// Raider (interceptor) with a capital to receive the loot.
	var raider, raiderProv, raiderCapital uuid.UUID
	_ = pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1,$2,'x') RETURNING id`,
		"raider-"+uuid.New().String(), "raider-"+uuid.New().String()+"@test.invalid").Scan(&raider)
	_ = pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 9, 9, 'plains') RETURNING id`,
		f.worldID).Scan(&raiderProv)
	_ = pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Raidertown','achaean',$3,'capital',true,'active',5000) RETURNING id`,
		f.worldID, raiderProv, raider).Scan(&raiderCapital)

	// A sentry the raider posts on the route, watching hex (1,0) — the caravan's
	// halfway position.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'spearman','land',80,0,'positioned','sentry',1,0,1,0)`,
		f.worldID, raider); err != nil {
		t.Fatalf("create sentry: %v", err)
	}

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	departs := clk.Now().Add(-1 * time.Hour)
	arrives := clk.Now().Add(1 * time.Hour) // halfway now → hex (1,0)

	// The victim's caravan, in transit (0,0)→(3,0), carrying silver.
	var caravan uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,3,0,$5,$6,1,'in_transit',true)
		 RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID, departs, arrives,
	).Scan(&caravan); err != nil {
		t.Fatalf("create caravan: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,'silver',100)`, caravan); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}

	// A messenger at the SAME hex (1,0) — must be untouched by the scan.
	var messenger uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, status, hex_q, hex_r, arrives_at)
		 VALUES ($1,$2,$3,$4,'greetings','outbound',1,0, now() + interval '1 hour')
		 RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID).Scan(&messenger); err != nil {
		t.Fatalf("create messenger: %v", err)
	}

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	// Caravan seized.
	var cStatus string
	_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, caravan).Scan(&cStatus)
	if cStatus != "intercepted" {
		t.Errorf("caravan status = %q, want intercepted", cStatus)
	}

	// Loot in the raider's capital.
	var loot float64
	_ = pool.QueryRow(ctx,
		`SELECT COALESCE(settled(amount, rate, calc_tick),0) FROM settlement_goods
		 WHERE settlement_id=$1 AND good_key='silver'`, raiderCapital).Scan(&loot)
	if loot != 100 {
		t.Errorf("raider capital silver = %v, want 100 (seized loot)", loot)
	}

	// Messenger at the same hex is sacred — never touched.
	var mStatus string
	_ = pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, messenger).Scan(&mStatus)
	if mStatus != "outbound" {
		t.Errorf("messenger status = %q, want outbound (messengers are uninterceptable)", mStatus)
	}
}

func TestInterceptScan_NoSentryLeavesCaravanAlone(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	var caravan uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,3,0,$5,$6,1,'in_transit',true)
		 RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID,
		clk.Now().Add(-1*time.Hour), clk.Now().Add(1*time.Hour),
	).Scan(&caravan); err != nil {
		t.Fatalf("create caravan: %v", err)
	}

	h := NewInterceptScanHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), nil, clk)
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
		t.Fatalf("intercept scan: %v", err)
	}

	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, caravan).Scan(&status)
	if status != "in_transit" {
		t.Errorf("caravan status = %q, want in_transit (no sentry → no interception)", status)
	}
}
