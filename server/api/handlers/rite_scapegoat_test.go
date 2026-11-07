package handlers

// Potnia's Scapegoat (bönesystemet v3 steg 5a) as a cast. What the goat does at the
// punishment is tested in kharis/scapegoat_test.go.

import (
	"context"
	"strings"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func TestScapegoatCast_RealmOnlyFixedPriceOneAtATime(t *testing.T) {
	f := newAlphaFixture(t)
	livestock := f.stock(t, f.cities[0], "livestock")
	for name, body := range map[string]map[string]any{
		"the city step": {"target_scope": "city"},
		"a unit":        {"target_scope": "unit", "target": uuid.NewString()},
		"for ever":      {"target_scope": "permanent", "target": uuid.NewString()},
		"no step":       {},
	} {
		body["prayer"] = religion.PrayerPotniaScapegoat
		if r := f.castBody(f.cities[0], body); r.Code != 400 {
			t.Fatalf("%s: %d %s", name, r.Code, r.Body.String())
		}
	}
	if f.stock(t, f.cities[0], "livestock") != livestock || f.rows(t, religion.EffectScapegoat) != 0 {
		t.Fatal("a refused cast cost goods or left a goat")
	}
	realm := map[string]any{"prayer": religion.PrayerPotniaScapegoat, "target_scope": "realm"}
	if r := f.castBody(f.cities[0], realm); r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	if got := livestock - f.stock(t, f.cities[0], "livestock"); got != 12 { // 3 x4 (the realm)
		t.Fatalf("livestock paid %v want 12", got)
	}
	var scope string
	var city *uuid.UUID
	var from, until int
	if err := f.pool.QueryRow(context.Background(), `SELECT scope,settlement_id,from_tick,until_tick FROM prayer_blessings WHERE world_id=$1 AND effect=$2`, f.world, religion.EffectScapegoat).Scan(&scope, &city, &from, &until); err != nil {
		t.Fatal(err)
	}
	if scope != "realm" || city != nil || from != 100 || until != 130 {
		t.Fatalf("row %s %v %d..%d, want realm 100..130", scope, city, from, until)
	}
	// a second goat while one stands: 409 with the words, also from the other city's temple
	f.clearRest(t)
	if r := f.castBody(f.cities[1], realm); r.Code != 409 || !strings.Contains(r.Body.String(), "a scapegoat already stands for you") {
		t.Fatalf("second goat: %d %s", r.Code, r.Body.String())
	}
	// once the 30 game days are over without a punishment the goat is gone and a new one can be sent
	f.setTick(t, 130)
	if r := f.castBody(f.cities[1], realm); r.Code != 200 {
		t.Fatalf("after expiry: %d %s", r.Code, r.Body.String())
	}
}
