package kharis

// Potnia's scapegoat (bönesystemet v3 steg 5a): the next divine punishment that would strike is
// borne by the goat. The dice are the gods' as before; only the outcome changes. The goat is spent,
// the gods' mercy (the shield) is set as after any punishment, and the owner is told.

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func layGoat(t *testing.T, pool *pgxpool.Pool, world, owner uuid.UUID, from, until int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,from_tick,until_tick) VALUES($1,$2,$3,'realm',$4,$5) RETURNING id`,
		world, owner, religion.EffectScapegoat, from, until).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func setWorldTick(t *testing.T, pool *pgxpool.Pool, world uuid.UUID, tick int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE worlds SET current_tick=$2 WHERE id=$1`, world, tick); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// belongings is everything a punishment can take, summed: one number that must not move.
func belongings(t *testing.T, pool *pgxpool.Pool, city uuid.UUID) (units, grain float64) {
	t.Helper()
	ctx := context.Background()
	_ = pool.QueryRow(ctx, `SELECT COALESCE(sum(size),0)::float8 + count(*) FILTER (WHERE status='disbanded')*1000 FROM units WHERE settlement_id=$1`, city).Scan(&units)
	_ = pool.QueryRow(ctx, `SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key='grain'`, city).Scan(&grain)
	return
}

func TestScapegoat_TheGoatBearsThePunishmentAndTheMercyHolds(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, city, owner := starvationWarningFixture(t, 400, 0)
	for i := 0; i < 8; i++ { // every branch of the roll: the goat bears them all
		setWorldTick(t, pool, worldID, 1000*(i+1))
		seedDivineGarrison(t, pool, ctx, worldID, city, owner)
		if _, err := pool.Exec(ctx, `DELETE FROM prayer_blessings WHERE owner_id=$1`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM kharis_notice_state WHERE player_id=$1 AND condition='divine_punishment'`, owner); err != nil {
			t.Fatal(err)
		}
		now := 1000 * (i + 1)
		goat := layGoat(t, pool, worldID, owner, now-5, now+25)
		u0, g0 := belongings(t, pool, city)
		rec := &divineNotifyRecorder{}
		h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), rec)
		before := count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='DivinePunishment'`, city)
		either := count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type IN ('DivinePunishment','DivinePunishmentAverted')`, city)
		if err := h.applyDivinePunishment(ctx, city, worldID, owner); err != nil {
			t.Fatal(err)
		}
		if u1, g1 := belongings(t, pool, city); u1 != u0 || g1 != g0 {
			t.Fatalf("iter %d: the goat bore it but units %v->%v grain %v->%v", i, u0, u1, g0, g1)
		}
		if got := count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='DivinePunishment'`, city); got != before {
			t.Fatalf("iter %d: a DivinePunishment was recorded though the goat bore it", i)
		}
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT payload FROM events WHERE stream_id=$1 AND event_type='DivinePunishmentAverted' ORDER BY id DESC LIMIT 1`, city).Scan(&raw); err != nil {
			t.Fatalf("iter %d: no DivinePunishmentAverted: %v", i, err)
		}
		if count(t, pool, `SELECT count(*) FROM events WHERE id=(SELECT max(id) FROM events WHERE stream_id=$1 AND event_type='DivinePunishmentAverted') AND payload->>'ward_id'=$2 AND payload->>'settlement_id'=$1::text AND payload->>'would_have' IN ('chariot_loss','ship_loss','harvest_failure','garrison_plague')`, city, goat.String()) != 1 {
			t.Fatalf("iter %d: Averted payload %s lacks settlement_id/would_have/ward_id", i, raw)
		}
		// the shield is set exactly as after a punishment
		var until int
		if err := pool.QueryRow(ctx, `SELECT (context->>'until_tick')::int FROM kharis_notice_state WHERE player_id=$1 AND condition='divine_punishment'`, owner).Scan(&until); err != nil || until != now+WrathProtectionTicks {
			t.Fatalf("iter %d: shield until %d (%v), want %d", i, until, err, now+WrathProtectionTicks)
		}
		// the goat is spent (the row stays as the record)
		var goatUntil int
		_ = pool.QueryRow(ctx, `SELECT until_tick FROM prayer_blessings WHERE id=$1`, goat).Scan(&goatUntil)
		if goatUntil != now {
			t.Fatalf("iter %d: goat until %d, want spent at %d", i, goatUntil, now)
		}
		if len(rec.calls) != 1 || rec.calls[0].kind != "DivinePunishmentAverted" || rec.calls[0].playerID != owner {
			t.Fatalf("iter %d: notifications %+v", i, rec.calls)
		}
		if rec.calls[0].payload["would_have"] == nil || rec.calls[0].payload["name"] == nil {
			t.Fatalf("iter %d: notification lacks would_have/name: %v", i, rec.calls[0].payload)
		}
		// a second roll inside the mercy does nothing at all; after it, with no goat, the next one strikes
		if err := h.applyDivinePunishment(ctx, city, worldID, owner); err != nil {
			t.Fatal(err)
		}
		if got := count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type IN ('DivinePunishment','DivinePunishmentAverted')`, city); got != either+1 {
			t.Fatalf("iter %d: punishment inside the mercy", i)
		}
		setWorldTick(t, pool, worldID, now+WrathProtectionTicks)
		seedDivineGarrison(t, pool, ctx, worldID, city, owner)
		if err := h.applyDivinePunishment(ctx, city, worldID, owner); err != nil {
			t.Fatal(err)
		}
		if got := count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='DivinePunishment'`, city); got != before+1 {
			t.Fatalf("iter %d: the next punishment after the mercy did not strike normally (goat reused?)", i)
		}
	}
}

func TestScapegoat_AnExpiredOrForeignOrFutureGoatBearsNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, city, owner := starvationWarningFixture(t, 400, 0)
	var stranger uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "goat-"+uuid.NewString()).Scan(&stranger); err != nil {
		t.Fatal(err)
	}
	setWorldTick(t, pool, worldID, 500)
	seedDivineGarrison(t, pool, ctx, worldID, city, owner)
	layGoat(t, pool, worldID, owner, 400, 500)    // ended on this very tick
	layGoat(t, pool, worldID, owner, 501, 540)    // not yet
	layGoat(t, pool, worldID, stranger, 400, 540) // someone else's goat
	h := NewTickHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), &divineNotifyRecorder{})
	if err := h.applyDivinePunishment(ctx, city, worldID, owner); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='DivinePunishment'`, city) != 1 ||
		count(t, pool, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='DivinePunishmentAverted'`, city) != 0 {
		t.Fatal("a goat that is over, not yet here or not yours bore the punishment")
	}
	if count(t, pool, `SELECT count(*) FROM prayer_blessings WHERE owner_id=$1 AND until_tick=500 AND from_tick=400`, stranger) != 0 ||
		count(t, pool, `SELECT count(*) FROM prayer_blessings WHERE owner_id=$1 AND effect=$2 AND until_tick=540`, stranger, religion.EffectScapegoat) != 1 {
		t.Fatal("somebody else's goat was spent")
	}
}

// The wrath status every surface reads (keryx status, dossier, Kult) names a standing goat.
func TestScapegoat_WrathStatusNamesTheStandingGoat(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, _, owner := starvationWarningFixture(t, 400, 0)
	setWorldTick(t, pool, worldID, 700)
	if w, err := LoadWrathStatus(ctx, pool, owner, worldID); err != nil || w.ScapegoatRemainingDays != 0 || w.ScapegoatUntilTick != 0 {
		t.Fatalf("no goat: %+v %v", w, err)
	}
	layGoat(t, pool, worldID, owner, 700, 730)
	w, err := LoadWrathStatus(ctx, pool, owner, worldID)
	if err != nil || w.ScapegoatUntilTick != 730 || w.ScapegoatRemainingDays != 30 {
		t.Fatalf("a goat stands to 730: %+v %v", w, err)
	}
	setWorldTick(t, pool, worldID, 730)
	if w, _ = LoadWrathStatus(ctx, pool, owner, worldID); w.ScapegoatRemainingDays != 0 {
		t.Fatalf("an over goat is still named: %+v", w)
	}
}
