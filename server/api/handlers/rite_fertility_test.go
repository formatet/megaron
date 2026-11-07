package handlers

// Potnia's Fruitful Season (bönesystemet v3 steg 5a) as a cast: what it costs, what row it
// leaves, what it refuses. The growth itself is tested in kharis/economy.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func TestFertility_EconomyNamesTheEffectTheCastStores(t *testing.T) {
	if economy.FertilityEffect != religion.EffectBlessedFertility {
		t.Fatalf("economy reads %q, the prayer stores %q", economy.FertilityEffect, religion.EffectBlessedFertility)
	}
	if economy.FertilityGrowthFactor != 1.5 || religion.FertilityTicks != 20 || religion.FertilityTicks < economy.HerdGrazeBirthEvery {
		t.Fatal("fertility numbers moved: 1.5 for 20 game days, never shorter than a grazing cycle")
	}
}

func TestFertilityCast_CostRowAndPayload(t *testing.T) {
	for _, c := range []struct {
		scope  string
		factor float64
	}{{"city", 2}, {"realm", 4}} {
		f := newAlphaFixture(t)
		livestock, grain := f.stock(t, f.cities[0], "livestock"), f.stock(t, f.cities[0], "grain")
		r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerPotniaFertility, "target_scope": c.scope})
		if r.Code != 200 {
			t.Fatalf("%s: %d %s", c.scope, r.Code, r.Body.String())
		}
		if got := livestock - f.stock(t, f.cities[0], "livestock"); got != 4*c.factor {
			t.Fatalf("%s livestock paid %v want %v", c.scope, got, 4*c.factor)
		}
		if got := grain - f.stock(t, f.cities[0], "grain"); got != 4*c.factor {
			t.Fatalf("%s grain paid %v want %v", c.scope, got, 4*c.factor)
		}
		var scope string
		var city *uuid.UUID
		var owner uuid.UUID
		var from, until int
		if err := f.pool.QueryRow(context.Background(), `SELECT scope,settlement_id,owner_id,from_tick,until_tick FROM prayer_blessings WHERE world_id=$1 AND effect=$2`, f.world, religion.EffectBlessedFertility).Scan(&scope, &city, &owner, &from, &until); err != nil {
			t.Fatal(err)
		}
		if scope != c.scope || owner != f.player || from != 100 || until != 120 || (c.scope == "city") != (city != nil && *city == f.cities[0]) {
			t.Fatalf("row %s city=%v owner=%v %d..%d", scope, city, owner, from, until)
		}
		var raw []byte
		_ = f.pool.QueryRow(context.Background(), `SELECT payload FROM events WHERE world_id=$1 AND event_type='RiteCast'`, f.world).Scan(&raw)
		var p map[string]any
		_ = json.Unmarshal(raw, &p)
		eff, _ := p["effect"].(map[string]any)
		if eff["target_scope"] != c.scope || eff["until_tick"] != float64(120) || eff["growth_factor"] != 1.5 || eff["blessing_id"] == nil {
			t.Fatalf("RiteCast effect %v", eff)
		}
	}
}

func TestFertilityCast_RefusesWhatItCannotCover(t *testing.T) {
	f := newAlphaFixture(t)
	stock := f.stock(t, f.cities[0], "livestock")
	for name, body := range map[string]map[string]any{
		"a unit":       {"target_scope": "unit", "target": uuid.NewString()},
		"for ever":     {"target_scope": "permanent", "target": uuid.NewString()},
		"no step":      {},
		"unknown step": {"target_scope": "world"},
	} {
		body["prayer"] = religion.PrayerPotniaFertility
		if r := f.castBody(f.cities[0], body); r.Code != 400 {
			t.Fatalf("%s: %d %s", name, r.Code, r.Body.String())
		}
	}
	if f.stock(t, f.cities[0], "livestock") != stock || f.rows(t, religion.EffectBlessedFertility) != 0 {
		t.Fatal("a refused cast cost goods or left a blessing")
	}
	// the same city twice is 409; the realm over a city blessing is allowed (a wider one)
	good := map[string]any{"prayer": religion.PrayerPotniaFertility, "target_scope": "city"}
	if r := f.castBody(f.cities[0], good); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	f.clearRest(t)
	if r := f.castBody(f.cities[0], good); r.Code != 409 {
		t.Fatalf("same city twice: %d", r.Code)
	}
	if r := f.castBody(f.cities[0], map[string]any{"prayer": religion.PrayerPotniaFertility, "target_scope": "realm"}); r.Code != 200 {
		t.Fatalf("the realm over a city blessing: %d %s", r.Code, r.Body.String())
	}
}
