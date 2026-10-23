package handlers

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/religion"
)

func TestPrayerCanPrepareWithoutTraditionalGoods(t *testing.T) {
	f := newAlphaFixture(t)
	relationValues(t, f)
	ctx := context.Background()
	id := religion.PrayerPotniaVision
	if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET amount=CASE WHEN good_key='incense' THEN 10 ELSE 0 END,rate=0,calc_tick=100 WHERE settlement_id=$1`, f.cities[0]); err != nil {
		t.Fatal(err)
	}
	before := readWaitReasonPrayer(t, f, id)
	// Establish that the existing cast contract already accepts this gift.
	rec := relationCast(f, f.cities[0], map[string]any{"prayer": id, "offering": map[string]float64{"incense": 5}})
	if rec.Code != 200 {
		t.Fatalf("existing composed gift refused: %d %s", rec.Code, rec.Body.String())
	}
	if f.stock(t, f.cities[0], "incense") != 5 || f.stock(t, f.cities[0], "grain") != 0 || f.stock(t, f.cities[0], "silver") != 0 {
		t.Fatal("composed gift changed the wrong stocks")
	}
	if before.Affordable || !before.CanPrepare {
		t.Fatalf("preparation hidden although a composed gift succeeds: %+v", before)
	}
	after := readWaitReasonPrayer(t, f, id)
	if after.CanPrepare || after.Wait <= 0 {
		t.Fatalf("preparation remains enabled during wait: %+v", after)
	}
}

func TestPrayerPreparationRespectsNonMaterialRequirements(t *testing.T) {
	for _, state := range []string{"ready", "temple", "kharis", "waiting", "active", "pending"} {
		t.Run(state, func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			id := religion.PrayerPotniaVision
			switch state {
			case "temple":
				if _, err := f.pool.Exec(ctx, `DELETE FROM buildings WHERE settlement_id=$1 AND god_id='potnia'`, f.cities[0]); err != nil {
					t.Fatal(err)
				}
			case "kharis":
				if _, err := f.pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=1,kharis_rate=0,kharis_calc_tick=100 WHERE player_id=$1 AND world_id=$2`, f.player, f.world); err != nil {
					t.Fatal(err)
				}
			case "active":
				id = religion.PrayerGreenShoots
			case "pending":
				id = religion.PrayerAsterionThread
			}
			if state == "waiting" || state == "active" || state == "pending" {
				if rec := f.cast(f.cities[0], id); rec.Code != 200 {
					t.Fatalf("setup cast=%d %s", rec.Code, rec.Body.String())
				}
			}
			p := readWaitReasonPrayer(t, f, id)
			if p.CanPrepare != (state == "ready") {
				t.Fatalf("preparation gate wrong for%s: %+v", state, p)
			}
		})
	}
}
