package handlers

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// Bönegrunden: the gift sets the length, the temple rests (not the god), the
// rest shrinks with the temple's level and only counts down while the temple is
// fully staffed.

func restOf(t *testing.T, f alphaFixture, city uuid.UUID, god string) (remaining, last int, found bool) {
	t.Helper()
	err := f.pool.QueryRow(context.Background(), `SELECT tr.remaining_ticks, tr.last_tick FROM temple_rest tr JOIN buildings b ON b.id=tr.building_id WHERE b.settlement_id=$1 AND b.god_id=$2`, city, god).Scan(&remaining, &last)
	return remaining, last, err == nil
}

func TestGiftSetsPrayerLengthWithCap(t *testing.T) {
	for _, tc := range []struct {
		prayer string
		column string
		factor float64
		want   int
	}{
		{religion.PrayerPotniaWalls, "garrison_defense_until_tick", 1, 6},
		{religion.PrayerPotniaWalls, "garrison_defense_until_tick", 2, 12},
		{religion.PrayerPotniaWalls, "garrison_defense_until_tick", 10, 30}, // capped at 5x
		{religion.PrayerGreenShoots, "harvest_blessing_until_tick", 2, 50},
		{religion.PrayerGreenShoots, "harvest_blessing_until_tick", 1, 25},
	} {
		t.Run(fmt.Sprintf("%s x%v", tc.prayer, tc.factor), func(t *testing.T) {
			f := newAlphaFixture(t)
			relationValues(t, f)
			spec := religion.PrayerSpecs[tc.prayer]
			offering := map[string]float64{}
			for g, n := range spec.Offering {
				offering[g] = n * tc.factor
			}
			r := relationCast(f, f.cities[0], map[string]any{"prayer": tc.prayer, "offering": offering})
			if r.Code != 200 {
				t.Fatalf("cast %d %s", r.Code, r.Body.String())
			}
			var until int
			if err := f.pool.QueryRow(context.Background(), `SELECT `+tc.column+` FROM settlements WHERE id=$1`, f.cities[0]).Scan(&until); err != nil {
				t.Fatal(err)
			}
			if until != 100+tc.want {
				t.Fatalf("until %d want %d", until, 100+tc.want)
			}
			if !strings.Contains(r.Body.String(), fmt.Sprintf("for %d game day", tc.want)) || !strings.Contains(r.Body.String(), fmt.Sprintf(`"duration_ticks":%d`, tc.want)) {
				t.Fatalf("length missing from answer: %s", r.Body.String())
			}
			var logged int
			if err := f.pool.QueryRow(context.Background(), `SELECT (payload->>'duration_ticks')::int FROM events WHERE world_id=$1 AND event_type='RiteCast'`, f.world).Scan(&logged); err != nil || logged != tc.want {
				t.Fatalf("event duration_ticks %d (%v) want %d", logged, err, tc.want)
			}
		})
	}
}

func TestTempleRestShrinksWithLevelAndGrowsWithGift(t *testing.T) {
	for _, tc := range []struct {
		level  int
		factor float64
		want   int
	}{{1, 1, 24}, {2, 1, 12}, {3, 1, 8}, {2, 2, 24}} {
		t.Run(fmt.Sprintf("L%d x%v", tc.level, tc.factor), func(t *testing.T) {
			f := newAlphaFixture(t)
			relationValues(t, f)
			if _, err := f.pool.Exec(context.Background(), `UPDATE buildings SET level=$2 WHERE settlement_id=$1 AND god_id='potnia'`, f.cities[0], tc.level); err != nil {
				t.Fatal(err)
			}
			r := relationCast(f, f.cities[0], map[string]any{"prayer": religion.PrayerPotniaWalls, "offering": map[string]float64{"grain": 8 * tc.factor, "timber": 4 * tc.factor}})
			if r.Code != 200 {
				t.Fatalf("cast %d %s", r.Code, r.Body.String())
			}
			rem, last, ok := restOf(t, f, f.cities[0], "potnia")
			if !ok || rem != tc.want || last != 100 {
				t.Fatalf("rest %d last %d found %v want %d", rem, last, ok, tc.want)
			}
		})
	}
}

func TestTempleRestRefusalNamesPauseUntilStaffed(t *testing.T) {
	f := newAlphaFixture(t)
	if r := f.cast(f.cities[0], religion.PrayerPotniaWalls); r.Code != 200 {
		t.Fatalf("first %d %s", r.Code, r.Body.String())
	}
	r := f.cast(f.cities[0], religion.PrayerPotniaVision)
	if r.Code != 409 || !strings.Contains(r.Body.String(), "stands still") || !strings.Contains(r.Body.String(), "0 of 2 servants") {
		t.Fatalf("understaffed refusal %d %s", r.Code, r.Body.String())
	}
	staffTemple(t, f, f.cities[0], "potnia", 2)
	r = f.cast(f.cities[0], religion.PrayerPotniaVision)
	if r.Code != 409 || strings.Contains(r.Body.String(), "stands still") || !strings.Contains(r.Body.String(), "rests for another") {
		t.Fatalf("fully staffed refusal %d %s", r.Code, r.Body.String())
	}
}

func staffTemple(t *testing.T, f alphaFixture, city uuid.UUID, god string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key,building_id) SELECT $1,$3,'building','temple','cult',id FROM buildings WHERE settlement_id=$1 AND building_type='temple' AND god_id=$2`, city, god, i); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdvanceTempleRestOnlyWhenFullyStaffedAndIdempotent(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	if r := f.cast(f.cities[0], religion.PrayerPotniaWalls); r.Code != 200 { // rest 24, last_tick 100
		t.Fatalf("cast %d %s", r.Code, r.Body.String())
	}
	staffTemple(t, f, f.cities[0], "potnia", 1) // 1 of 2: short
	if err := economy.AdvanceTempleRest(ctx, f.pool, f.world, 101); err != nil {
		t.Fatal(err)
	}
	if rem, last, _ := restOf(t, f, f.cities[0], "potnia"); rem != 24 || last != 101 {
		t.Fatalf("understaffed temple counted down: rest %d last %d", rem, last)
	}
	staffTemple2 := func() {
		if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key,building_id) SELECT $1,2,'building','temple','cult',id FROM buildings WHERE settlement_id=$1 AND building_type='temple' AND god_id='potnia'`, f.cities[0]); err != nil {
			t.Fatal(err)
		}
	}
	staffTemple2()           // now 2 of 2
	for i := 0; i < 2; i++ { // the same tick twice counts once
		if err := economy.AdvanceTempleRest(ctx, f.pool, f.world, 102); err != nil {
			t.Fatal(err)
		}
	}
	if rem, last, _ := restOf(t, f, f.cities[0], "potnia"); rem != 23 || last != 102 {
		t.Fatalf("fully staffed temple rest %d last %d want 23/102", rem, last)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE temple_rest SET remaining_ticks=1`); err != nil {
		t.Fatal(err)
	}
	if err := economy.AdvanceTempleRest(ctx, f.pool, f.world, 103); err != nil {
		t.Fatal(err)
	}
	if _, _, found := restOf(t, f, f.cities[0], "potnia"); found {
		t.Fatal("rest at 0 was not deleted")
	}
	if r := f.cast(f.cities[0], religion.PrayerPotniaVision); r.Code != 200 {
		t.Fatalf("rested temple still refuses %d %s", r.Code, r.Body.String())
	}
}
