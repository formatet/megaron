package handlers

// Prayers of permanence (bönesystemet v3 steg 4): the fourth step of a blessing prayer lays it on
// one NAMED unit for ever, at 20 times the traditional gift. Strength and sea speed are one
// prayer_blessings row without an end; hull is +1 max hull (units.hull_bonus), +1 hull now.

import (
	"context"
	"strings"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func (f alphaFixture) rich(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE settlement_goods SET amount=2000, cap=5000 WHERE settlement_id = ANY($1)`, f.cities); err != nil {
		t.Fatal(err)
	}
}

func (f alphaFixture) permanent(prayer string, target uuid.UUID) map[string]any {
	return map[string]any{"prayer": prayer, "target_scope": "permanent", "target": target.String()}
}

// Price x20 (the traditional gift 20 times over), the temple rests exactly as after an ordinary prayer,
// and the row has no end.
func TestPermanentCast_CostsTwentyFoldRestsLikeOneUnitAndNeverEnds(t *testing.T) {
	f := newAlphaFixture(t)
	f.rich(t)
	ctx := context.Background()
	troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	troop2 := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[1])
	// The ordinary prayer in the other city is the yardstick for the temple's rest.
	if r := f.castBody(f.cities[1], map[string]any{"prayer": religion.PrayerVelchanosStrength, "target_scope": "unit", "target": troop2.String()}); r.Code != 200 {
		t.Fatalf("ordinary cast %d %s", r.Code, r.Body.String())
	}
	ordinary := f.lastRite(t)
	livestock, wine := f.stock(t, f.cities[0], "livestock"), f.stock(t, f.cities[0], "wine")
	r := f.castBody(f.cities[0], f.permanent(religion.PrayerVelchanosStrength, troop))
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"success":true`) {
		t.Fatalf("permanent cast %d %s", r.Code, r.Body.String())
	}
	if got := livestock - f.stock(t, f.cities[0], "livestock"); got != 6*20 {
		t.Fatalf("livestock paid %v, want 120", got)
	}
	if got := wine - f.stock(t, f.cities[0], "wine"); got != 4*20 {
		t.Fatalf("wine paid %v, want 80", got)
	}
	var scope string
	var until int
	var permanent bool
	var unitID *uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT scope,until_tick,permanent,unit_id FROM prayer_blessings WHERE world_id=$1 AND unit_id=$2`, f.world, troop).Scan(&scope, &until, &permanent, &unitID); err != nil {
		t.Fatal(err)
	}
	if scope != "unit" || until != religion.PermanentUntil || !permanent {
		t.Fatalf("row scope=%s until=%d permanent=%v", scope, until, permanent)
	}
	ev := f.lastRite(t)
	if ev["rest_ticks"] != ordinary["rest_ticks"] || ev["gift_factor"] != 1.0 || ev["rest_ticks"] == nil {
		t.Fatalf("rest after the x20 prayer %v (gift_factor %v), after the ordinary one %v", ev["rest_ticks"], ev["gift_factor"], ordinary["rest_ticks"])
	}
	eff, _ := ev["effect"].(map[string]any)
	if eff["target_scope"] != "permanent" || eff["target_unit_id"] != troop.String() {
		t.Fatalf("RiteCast effect %v", eff)
	}
}

// A city or the realm cannot be blessed for ever, nor a Runner or caravan; a wrong kind or a foreign unit is refused. Nothing is paid.
func TestPermanentCast_RefusesWhatIsNotANamedUnitOfTheRightKind(t *testing.T) {
	f := newAlphaFixture(t)
	f.rich(t)
	foreign := f.foreigner(t)
	troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	ship := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	theirs := f.unit(t, foreign, "infantry", "land", "garrison", f.cities[0])
	dead := f.unit(t, f.player, "infantry", "land", "disbanded", f.cities[0])
	before := f.stock(t, f.cities[0], "livestock")
	for _, c := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"no target at all (a city or the realm)", map[string]any{"prayer": religion.PrayerVelchanosStrength, "target_scope": "permanent"}, 400},
		{"a city id as the target", f.permanent(religion.PrayerVelchanosStrength, f.cities[0]), 404},
		{"someone else's unit", f.permanent(religion.PrayerVelchanosStrength, theirs), 404},
		{"a ship for the strength prayer", f.permanent(religion.PrayerVelchanosStrength, ship), 400},
		{"a disbanded unit", f.permanent(religion.PrayerVelchanosStrength, dead), 400},
		{"a Runner or caravan (Swift Feet has no permanent step)", f.permanent(religion.PrayerAsterionSwift, troop), 400},
		{"a troop for the hull prayer", f.permanent(religion.PrayerStillnessHull, troop), 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			if r := f.castBody(f.cities[0], c.body); r.Code != c.want {
				t.Fatalf("got %d want %d: %s", r.Code, c.want, r.Body.String())
			}
		})
	}
	if f.stock(t, f.cities[0], "livestock") != before || f.rows(t, religion.EffectBlessedStrength) != 0 || f.rows(t, religion.EffectBlessedHull) != 0 {
		t.Fatal("a refused cast cost goods or left a blessing")
	}
}

// No stacking: the same permanent twice is 409, and an ordinary prayer of the same kind on a permanent unit is 409.
func TestPermanentCast_NoStackingTwiceOrWithAnOrdinaryPrayer(t *testing.T) {
	f := newAlphaFixture(t)
	f.rich(t)
	troop := f.unit(t, f.player, "infantry", "land", "garrison", f.cities[0])
	ship := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	for _, c := range []struct {
		prayer string
		unit   uuid.UUID
	}{{religion.PrayerVelchanosStrength, troop}, {religion.PrayerStillnessSwift, ship}, {religion.PrayerStillnessHull, ship}} {
		f.clearRest(t)
		if r := f.castBody(f.cities[0], f.permanent(c.prayer, c.unit)); r.Code != 200 {
			t.Fatalf("%s first: %d %s", c.prayer, r.Code, r.Body.String())
		}
		f.clearRest(t)
		before := f.stock(t, f.cities[0], "fish") + f.stock(t, f.cities[0], "livestock")
		r := f.castBody(f.cities[0], f.permanent(c.prayer, c.unit))
		if r.Code != 409 || !strings.Contains(r.Body.String(), "for ever") {
			t.Fatalf("%s twice: %d %s, want 409 'already blessed for ever'", c.prayer, r.Code, r.Body.String())
		}
		if f.stock(t, f.cities[0], "fish")+f.stock(t, f.cities[0], "livestock") != before {
			t.Fatalf("%s: the refused cast cost goods", c.prayer)
		}
		if c.prayer == religion.PrayerStillnessHull {
			continue // an ordinary ward is still welcome on a ship with the permanent plank
		}
		f.clearRest(t)
		if r := f.castBody(f.cities[0], map[string]any{"prayer": c.prayer, "target_scope": "unit", "target": c.unit.String()}); r.Code != 409 {
			t.Fatalf("%s ordinary on the permanent unit: %d %s, want 409", c.prayer, r.Code, r.Body.String())
		}
		// A city or realm prayer of the same kind is still allowed; it changes nothing for the permanent unit.
		f.clearRest(t)
		if r := f.castBody(f.cities[0], map[string]any{"prayer": c.prayer, "target_scope": "city"}); r.Code != 200 {
			t.Fatalf("%s city blessing beside a permanent unit: %d %s", c.prayer, r.Code, r.Body.String())
		}
	}
}

// Following Seas for ever on a ship under way speeds what is left, like the ordinary prayer on one ship.
func TestPermanentCast_SeaSpeedSpeedsTheVoyageUnderWay(t *testing.T) {
	f := newAlphaFixture(t)
	f.rich(t)
	s1 := f.ship(t, f.player, f.cities[0])
	s2 := f.ship(t, f.player, f.cities[0])
	if r := f.castBody(f.cities[0], f.permanent(religion.PrayerStillnessSwift, s1)); r.Code != 200 {
		t.Fatalf("cast %d %s", r.Code, r.Body.String())
	}
	if f.arriveTick(t, "units", s1) != 106 || f.arriveTick(t, "units", s2) != 108 {
		t.Fatalf("s1=%d (want 106) s2=%d (want 108, not named)", f.arriveTick(t, "units", s1), f.arriveTick(t, "units", s2))
	}
}

// Stout Hulls for ever: +1 max hull, +1 hull now; the plank is a column, not a ward, so no blessing row.
func TestPermanentCast_HullGivesAPlankToThisShipOnly(t *testing.T) {
	f := newAlphaFixture(t)
	f.rich(t)
	ctx := context.Background()
	ship := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	other := f.unit(t, f.player, "galley", "naval", "garrison", f.cities[0])
	if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=3 WHERE id=$1`, ship); err != nil {
		t.Fatal(err)
	}
	fish, oil := f.stock(t, f.cities[0], "fish"), f.stock(t, f.cities[0], "oil")
	if r := f.castBody(f.cities[0], f.permanent(religion.PrayerStillnessHull, ship)); r.Code != 200 {
		t.Fatalf("cast %d %s", r.Code, r.Body.String())
	}
	if got := fish - f.stock(t, f.cities[0], "fish"); got != 8*20 {
		t.Fatalf("fish paid %v, want 160", got)
	}
	if got := oil - f.stock(t, f.cities[0], "oil"); got != 4*20 {
		t.Fatalf("oil paid %v, want 80", got)
	}
	read := func(id uuid.UUID) (hull, bonus int) {
		if err := f.pool.QueryRow(ctx, `SELECT hull,hull_bonus FROM units WHERE id=$1`, id).Scan(&hull, &bonus); err != nil {
			t.Fatal(err)
		}
		return
	}
	if h, b := read(ship); h != 4 || b != 1 {
		t.Fatalf("named ship hull %d bonus %d, want 4 (3+1) and 1", h, b)
	}
	if h, b := read(other); h != 5 || b != 0 {
		t.Fatalf("another ship hull %d bonus %d, want untouched 5/0", h, b)
	}
	if f.rows(t, religion.EffectBlessedHull) != 0 {
		t.Fatal("a permanent hull wrote a ward row")
	}
	eff, _ := f.lastRite(t)["effect"].(map[string]any)
	if eff["target_scope"] != "permanent" || eff["target_unit_id"] != ship.String() || eff["hull_bonus"] != 1.0 || eff["hull_before"] != 3.0 || eff["hull_after"] != 4.0 {
		t.Fatalf("RiteCast effect %v", eff)
	}
	// A full ship stands at 6 of 6 afterwards.
	f.clearRest(t)
	if r := f.castBody(f.cities[0], f.permanent(religion.PrayerStillnessHull, other)); r.Code != 200 {
		t.Fatalf("second ship %d %s", r.Code, r.Body.String())
	}
	if h, b := read(other); h != 6 || b != 1 {
		t.Fatalf("a full ship should stand at 6/6: hull %d bonus %d", h, b)
	}
}
