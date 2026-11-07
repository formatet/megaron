package transport

// The Thread (böner steg 5b): a land caravan under a live blessed_thread is only taken by a
// sentry within ONE hex (ordinary: two). The blessing is read at every scan.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

type threadRig struct {
	f       fixture
	ctx     context.Context
	clk     *clock.TestClock
	caravan uuid.UUID
	raider  uuid.UUID
	h       *InterceptScanHandler
}

// newThreadRig: a land caravan halfway along (0,0)->(3,0), at hex (1,0), owned by f.owner.
// A raider's sentry stands at hex (sentryQ,0).
func newThreadRig(t *testing.T, sentryQ int) *threadRig {
	t.Helper()
	pool := testPool(t)
	f := newFixture(t, pool)
	ctx := context.Background()
	r := &threadRig{f: f, ctx: ctx, clk: clock.NewTestClock(time.Unix(1_000_000, 0))}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=100 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	var raiderProv uuid.UUID
	must(t, pool.QueryRow(ctx, `INSERT INTO players (username, password_hash) VALUES ($1,'x') RETURNING id`, "raider-"+uuid.New().String()).Scan(&r.raider))
	must(t, pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 9, 9, 'plains') RETURNING id`, f.worldID).Scan(&raiderProv))
	_, err := pool.Exec(ctx, `INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1,$2,'Raidertown','achaean',$3,'capital',true,'active',5000)`, f.worldID, raiderProv, r.raider)
	must(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, stance, q, r, sentry_q, sentry_r)
		 VALUES ($1,$2,'spearman','land',80,0,'positioned','sentry',$3,0,$3,0)`, f.worldID, r.raider, sentryQ)
	must(t, err)
	must(t, pool.QueryRow(ctx, `INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category, origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, status, interceptable)
		 VALUES ($1,$2,'trade',$3,$4,'land',0,0,3,0,$5,$6,1,'in_transit',true) RETURNING id`,
		f.worldID, f.owner, f.sourceID, f.destID, r.clk.Now().Add(-time.Hour), r.clk.Now().Add(time.Hour)).Scan(&r.caravan))
	r.h = NewInterceptScanHandler(pool, events.NewScheduler(pool, r.clk), events.NewStore(pool), nil, r.clk)
	return r
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (r *threadRig) bless(t *testing.T, owner uuid.UUID, scope string, city, mover *uuid.UUID, from, until int) {
	t.Helper()
	var kind *string
	if mover != nil {
		k := "transport"
		kind = &k
	}
	_, err := testPool(t).Exec(r.ctx, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,settlement_id,mover_id,mover_kind,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.f.worldID, owner, religion.EffectBlessedThread, scope, city, mover, kind, from, until)
	must(t, err)
}

func (r *threadRig) scanAndStatus(t *testing.T) string {
	t.Helper()
	must(t, r.h.Handle(r.ctx, events.ScheduledEvent{WorldID: r.f.worldID, DueTick: 1}))
	var s string
	must(t, testPool(t).QueryRow(r.ctx, `SELECT status FROM transports WHERE id=$1`, r.caravan).Scan(&s))
	return s
}

func TestThread_SentryTwoHexesOffTakesAnOrdinaryCaravan(t *testing.T) {
	r := newThreadRig(t, 3)
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("control: an unblessed caravan with a sentry two hexes off should be taken, status = %q", got)
	}
}

func TestThread_SentryTwoHexesOffCannotTakeABlessedCaravan(t *testing.T) {
	for _, scope := range []string{"unit", "city", "realm"} {
		t.Run(scope, func(t *testing.T) {
			r := newThreadRig(t, 3)
			switch scope {
			case "unit":
				r.bless(t, r.f.owner, "unit", nil, &r.caravan, 0, 200)
			case "city":
				r.bless(t, r.f.owner, "city", &r.f.sourceID, nil, 0, 200)
			default:
				r.bless(t, r.f.owner, "realm", nil, nil, 0, 200)
			}
			if got := r.scanAndStatus(t); got != "in_transit" {
				t.Fatalf("%s thread: sentry at two hexes must not take the caravan, status = %q", scope, got)
			}
		})
	}
}

func TestThread_SentryOneHexOffTakesTheBlessedToo(t *testing.T) {
	r := newThreadRig(t, 2)
	r.bless(t, r.f.owner, "realm", nil, nil, 0, 200)
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("a sentry one hex off takes a blessed caravan too, status = %q", got)
	}
}

// The blessing is read at the scan: a caravan already on the road is protected from the next scan,
// and loses the protection when the blessing has run out (or has not begun).
func TestThread_ReadAtEveryScanNotBakedAtDeparture(t *testing.T) {
	r := newThreadRig(t, 3)
	pool := testPool(t)
	r.bless(t, r.f.owner, "realm", nil, nil, 0, 200)
	if got := r.scanAndStatus(t); got != "in_transit" {
		t.Fatalf("protected while the blessing lasts, status = %q", got)
	}
	if _, err := pool.Exec(r.ctx, `UPDATE worlds SET current_tick=200 WHERE id=$1`, r.f.worldID); err != nil {
		t.Fatal(err)
	}
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("after until_tick the radius is two again, status = %q", got)
	}
}

func TestThread_NotYetBegunProtectsNothing(t *testing.T) {
	r := newThreadRig(t, 3)
	r.bless(t, r.f.owner, "realm", nil, nil, 150, 300)
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("a blessing from a later tick does not cover now, status = %q", got)
	}
}

func TestThread_OnlyTheOwnersCaravans(t *testing.T) {
	r := newThreadRig(t, 3)
	r.bless(t, r.raider, "realm", nil, nil, 0, 200) // the RAIDER's thread covers the raider's caravans, not the victim's
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("someone else's thread must not cover this caravan, status = %q", got)
	}
}

func TestThread_ANamedCaravanCoversOnlyThatCaravan(t *testing.T) {
	r := newThreadRig(t, 3)
	other := uuid.New()
	r.bless(t, r.f.owner, "unit", nil, &other, 0, 200)
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("a thread on another caravan does not cover this one, status = %q", got)
	}
}

func TestThread_ACityThreadCoversOnlyCaravansFromThatCity(t *testing.T) {
	r := newThreadRig(t, 3)
	r.bless(t, r.f.owner, "city", &r.f.destID, nil, 0, 200) // the caravan comes from sourceID
	if got := r.scanAndStatus(t); got != "intercepted" {
		t.Fatalf("a thread on another city does not cover this caravan, status = %q", got)
	}
}
