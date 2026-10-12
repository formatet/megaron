package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"math"
	"reflect"
	"testing"
)

func TestSilverCalibrationFirstFoundingEndowment(t *testing.T) {
	for _, tc := range []struct {
		name               string
		coastal            bool
		carried, want, cap float64
	}{
		{"inland_spent_purse", false, 7, 730, 1460},
		{"coast_spent_purse", true, 7, 1278, 2556},
		{"inland_full_purse", false, 480, 730, 1460},
		{"coast_full_purse", true, 480, 1278, 2556},
		{"existing_wealth_is_not_destroyed", false, 5000, 5000, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := escortTestPool(t)
			ctx := context.Background()
			wid, pid := seedEscortWorld(t, pool)
			if _, err := pool.Exec(ctx, `UPDATE map_tiles SET coastal=$2 WHERE world_id=$1 AND q=0 AND r=0`, wid, tc.coastal); err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			store := events.NewStore(pool)
			if _, err := seedNomadicHost(ctx, tx, store, wid, pid, 0, 0); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE founder_phase SET silver_amount=$3,silver_rate=0 WHERE world_id=$1 AND owner_id=$2`, wid, pid, tc.carried); err != nil {
				t.Fatal(err)
			}
			found, err := foundMetropolisFromNomadicHost(ctx, tx, store, economy.LoadSitosConfig(), wid, pid, "Silver-first", "minoan")
			if err != nil {
				t.Fatal(err)
			}
			var amount, cap float64
			if err := tx.QueryRow(ctx, `SELECT settled(amount,rate,calc_tick),cap FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, found.SettlementID).Scan(&amount, &cap); err != nil {
				t.Fatal(err)
			}
			if amount != tc.want || cap != tc.cap {
				t.Fatalf("first founding silver %g/cap%g; want %g/%g; carried%g must be included once", amount, cap, tc.want, tc.cap, tc.carried)
			}
			var active bool
			if err := tx.QueryRow(ctx, `SELECT active FROM founder_phase WHERE world_id=$1 AND owner_id=$2`, wid, pid).Scan(&active); err != nil {
				t.Fatal(err)
			}
			if active {
				t.Fatal("founder phase not consumed")
			}
		})
	}
}

func TestSilverCalibrationDirectMetropolisPopulationIndependent(t *testing.T) {
	for _, coastal := range []bool{false, true} {
		pool, sid := foundMetropolisFixtureCoastal(t, [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"}, coastal)
		var amount, cap float64
		if err := pool.QueryRow(context.Background(), `SELECT amount,cap FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, sid).Scan(&amount, &cap); err != nil {
			t.Fatal(err)
		}
		want, wcap := 730.0, 1460.0
		if coastal {
			want, wcap = 1278, 2556
		}
		if amount != want || cap != wcap {
			t.Fatalf("direct caller with pop4000 gave %g/%g want %g/%g", amount, cap, want, wcap)
		}
	}
}

func TestSilverCalibrationRefugeeFoundingNeverMints(t *testing.T) {
	for _, coastal := range []bool{false, true} {
		pool := escortTestPool(t)
		ctx := context.Background()
		wid, pid := seedEscortWorld(t, pool)
		if _, err := pool.Exec(ctx, `UPDATE map_tiles SET coastal=$2 WHERE world_id=$1`, wid, coastal); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		store := events.NewStore(pool)
		if _, err := seedHost(ctx, tx, store, wid, pid, 0, 0, hostSpec{population: 400, spearmen: 1, rationTicks: 60, refugee: true}); err != nil {
			t.Fatal(err)
		}
		var initial float64
		var refugee bool
		if err := tx.QueryRow(ctx, `SELECT silver_amount,is_refugee FROM founder_phase WHERE world_id=$1 AND owner_id=$2`, wid, pid).Scan(&initial, &refugee); err != nil {
			t.Fatal(err)
		}
		if initial != 60 || !refugee {
			t.Fatalf("Rise purse=%g refugee=%v", initial, refugee)
		}
		if _, err := tx.Exec(ctx, `UPDATE founder_phase SET silver_amount=17,silver_rate=0 WHERE world_id=$1 AND owner_id=$2`, wid, pid); err != nil {
			t.Fatal(err)
		}
		found, err := foundMetropolisFromNomadicHost(ctx, tx, store, economy.LoadSitosConfig(), wid, pid, "Silver-refugee", "minoan")
		if err != nil {
			t.Fatal(err)
		}
		var amount float64
		if err := tx.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`, found.SettlementID).Scan(&amount); err != nil {
			t.Fatal(err)
		}
		if amount != 17 {
			t.Fatalf("refugee founding minted silver: %g want carried17", amount)
		}
	}
}

func TestSilverCalibrationOfferWorkAndLegacyPayloadRead(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	store := events.NewStore(f.pool)
	legacyRite := map[string]any{"offering_worth": 200.0, "offering_baseline": 200.0, "success": false, "prayer": "legacy"}
	legacyCult := map[string]any{"scarcity_factor": 2.0, "kharis_gain": 2.4, "temples_fed": 1.0}
	a, err := store.Append(ctx, f.cities[0], events.StreamReligion, "RiteCast", legacyRite, f.world, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Append(ctx, f.cities[0], events.StreamProvince, "KharisOffering", legacyCult, f.world, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := religion.InitializeLaborValuations(ctx, f.pool, f.world); err != nil {
		t.Fatal(err)
	}
	values, err := religion.LoadDivineValues(ctx, f.pool, f.world)
	if err != nil {
		t.Fatal(err)
	}
	want := 15*values["grain"]*1.3 + 2*values["silver"]
	response := f.cast(f.cities[0], religion.PrayerGreenShoots)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var raw []byte
	if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE stream_id=$1 AND event_type='RiteCast' ORDER BY id DESC LIMIT 1`, f.cities[0]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var newPayload map[string]any
	if err := json.Unmarshal(raw, &newPayload); err != nil {
		t.Fatal(err)
	}
	if work, ok := newPayload["offering_work"].(float64); !ok || math.Abs(work-want) > 1e-8 {
		t.Fatalf("offer work=%v want%g from grain/silver labor values", newPayload["offering_work"], want)
	}
	if newPayload["offer_mod"] != float64(0) {
		t.Fatalf("default offering changed its chance modifier: %v", newPayload["offer_mod"])
	}
	if newPayload["offering_value_unit"] != "worker_ticks" || newPayload["valuation_version"] != float64(2) {
		t.Fatalf("new units absent: %s", raw)
	}
	read, err := store.LoadStream(ctx, f.cities[0])
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, ev := range read {
		var expected map[string]any
		if ev.ID == a.ID {
			expected = legacyRite
		}
		if ev.ID == b.ID {
			expected = legacyCult
		}
		if expected == nil {
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(ev.Payload, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("legacy %s was reinterpreted: %v want%v", ev.EventType, got, expected)
		}
		seen++
	}
	if seen != 2 {
		t.Fatalf("legacy reader returned%d historical payloads want2", seen)
	}
}
