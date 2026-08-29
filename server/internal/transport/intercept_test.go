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
		`INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`,
		"raider-"+uuid.New().String()).Scan(&raider)
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

// TestInterceptScan_SentryOwnerBlindToTargetDoesNotSeize is the FOW gate on
// interception (avsiktslagret §S2/§4, megaron_plan_avsiktslagret.md): a sentry
// may only seize a caravan its OWNER can actually see at the interception
// moment. A naval sentry watches a hex, but a ship's crew reads only 1 hex over
// land (province.LiveRadius, EyeShip kind) — so a land caravan passing at
// distance 2 sits inside the ship's interceptRadius (2) yet outside its
// owner's actual vision. The all-seeing tripwire ("allvetande snubbeltråd")
// must NOT fire: the caravan continues untouched.
//
// RED before the fix: today's query seizes on proximity alone, with no
// AnyEyeSees check at all — the caravan gets seized even though the raider
// never laid eyes on it.
func TestInterceptScan_SentryOwnerBlindToTargetDoesNotSeize(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool) // land strip (0,0)…(3,0); caravan halfway → (1,0)
	ctx := context.Background()

	// A raider whose ONLY eye near the route is the ship itself — no
	// settlements anywhere close.
	var raider uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`,
		"raider-"+uuid.New().String()).Scan(&raider); err != nil {
		t.Fatalf("create raider: %v", err)
	}

	// A naval sentry posted at (3,0), watching that hex. Distance to the
	// caravan's halfway hex (1,0) is 2 = interceptRadius, so the proximity
	// query alone catches it — but a ship sees only 1 hex over land, so its
	// owner cannot actually see (1,0).
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'galley','naval',1,40,'positioned','sentry',3,0,3,0)`,
		f.worldID, raider); err != nil {
		t.Fatalf("create ship sentry: %v", err)
	}

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	var caravan uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,3,0,$5,$6,1,'in_transit',true)
		 RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID,
		clk.Now().Add(-1*time.Hour), clk.Now().Add(1*time.Hour), // halfway → (1,0)
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
		t.Errorf("caravan status = %q, want in_transit (sentry owner cannot see the caravan → no interception)", status)
	}
}

// TestInterceptScan_OwnCaravanNeverIntercepted pins avsiktslagret §6 point 2:
// a Wanax's own sentry never seizes their own caravan, even sitting right on
// top of it — today this is already true via owner_id<>$2 in the sentry
// query, but the reaction_policy encoding (S1/S2) must not accidentally break
// it (own is never read from the policy column; it stays enforced by the
// owner comparison). Not a red-before case — this must stay green throughout.
func TestInterceptScan_OwnCaravanNeverIntercepted(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()

	// f.owner posts their OWN sentry right on the caravan's halfway hex.
	if _, err := pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'spearman','land',80,0,'positioned','sentry',1,0,1,0)`,
		f.worldID, f.owner); err != nil {
		t.Fatalf("create own sentry: %v", err)
	}

	clk := clock.NewTestClock(time.Unix(1_000_000, 0))
	var caravan uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,3,0,$5,$6,1,'in_transit',true)
		 RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID,
		clk.Now().Add(-1*time.Hour), clk.Now().Add(1*time.Hour), // halfway → (1,0), same hex as the sentry
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
		t.Errorf("caravan status = %q, want in_transit (own sentry never intercepts own caravan)", status)
	}
}
