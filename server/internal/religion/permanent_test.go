package religion

// Prayers of permanence (bönesystemet v3 steg 4): a blessing on one named unit with
// until_tick = PermanentUntil never runs out, and covers that unit alone.

import (
	"context"
	"testing"
)

func TestPermanentBlessing_HoldsLongAfterAnOrdinaryOneEndedAndCoversOnlyTheNamedUnit(t *testing.T) {
	f := newBlessFix(t)
	ctx := context.Background()
	f.bless(t, f.me, EffectBlessedStrength, ScopeUnit, &f.home, nil, 100, PermanentUntil)
	f.bless(t, f.me, EffectBlessedStrength, ScopeUnit, &f.marching, nil, 100, 106) // an ordinary prayer on the other unit
	for _, tick := range []int{100, 106, 1100, 2_000_000} {
		got, err := LoadBlessed(ctx, f.pool, f.me, f.home, EffectBlessedStrength, tick)
		if err != nil || !got {
			t.Fatalf("named unit at tick %d: %v %v, want blessed", tick, got, err)
		}
	}
	if got, _ := LoadBlessed(ctx, f.pool, f.me, f.marching, EffectBlessedStrength, 1100); got {
		t.Fatal("the ordinary prayer should have ended long ago")
	}
	if got, _ := LoadBlessed(ctx, f.pool, f.me, f.elsewhere, EffectBlessedStrength, 1100); got {
		t.Fatal("a permanent blessing leaked to another unit of the owner")
	}
	if got, _ := LoadBlessed(ctx, f.pool, f.other, f.home, EffectBlessedStrength, 1100); got {
		t.Fatal("someone else's reading of my unit saw the blessing")
	}
	if active, err := BlessingActive(ctx, f.pool, f.me, EffectBlessedStrength, ScopeUnit, &f.home, nil, 50_000); err != nil || !active {
		t.Fatalf("BlessingActive after 50000 ticks: %v %v", active, err)
	}
}

func TestPermanentBlessing_SeaSpeedReachesEveryVoyageOfTheNamedShipOnly(t *testing.T) {
	f := newBlessFix(t)
	ctx := context.Background()
	f.bless(t, f.me, EffectBlessedSpeedSea, ScopeUnit, &f.ship, nil, 100, PermanentUntil)
	for _, tick := range []int{101, 700, 1500} { // three voyages, far apart
		if _, ok, err := SpeedBlessingFor(ctx, f.pool, f.me, EffectBlessedSpeedSea, f.ship, &f.cityA, tick); err != nil || !ok {
			t.Fatalf("voyage at tick %d: %v %v", tick, ok, err)
		}
	}
	if _, ok, _ := SpeedBlessingFor(ctx, f.pool, f.me, EffectBlessedSpeedSea, f.ship2, &f.cityA, 700); ok {
		t.Fatal("another ship got the named ship's speed")
	}
}

func TestScopeCostFactorsFor_PermanentOnlyWhereItExists(t *testing.T) {
	for _, c := range []struct {
		effect string
		want   bool
	}{{EffectBlessedStrength, true}, {EffectBlessedHull, true}, {EffectBlessedSpeedSea, true}, {EffectBlessedSpeedLand, false}} {
		got := ScopeCostFactorsFor(c.effect)
		if _, has := got[ScopePermanent]; has != c.want {
			t.Fatalf("%s: permanent step present=%v, want %v", c.effect, has, c.want)
		}
		if c.want && got[ScopePermanent] != 20 {
			t.Fatalf("%s: permanent costs x%d, want x20", c.effect, got[ScopePermanent])
		}
		if got[ScopeUnit] != 1 || got[ScopeCity] != 2 || got[ScopeRealm] != 4 {
			t.Fatalf("%s: the three old steps changed: %v", c.effect, got)
		}
	}
}
