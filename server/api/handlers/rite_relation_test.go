package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

type relationDice struct {
	calls int
	roll  int
}

func (d *relationDice) Intn(n int) int   { d.calls++; return d.roll % n }
func (d *relationDice) Float64() float64 { return float64(d.roll) / 100 }

func relationCast(f alphaFixture, city uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/worlds/"+f.world.String()+"/settlements/"+city.String()+"/rite", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}
func relationTuple(t *testing.T, f alphaFixture) string {
	t.Helper()
	var tuple string
	if err := f.pool.QueryRow(context.Background(), `SELECT jsonb_build_array(kharis_amount,kharis_rate,kharis_calc_tick)::text FROM player_world_records WHERE player_id=$1 AND world_id=$2`, f.player, f.world).Scan(&tuple); err != nil {
		t.Fatal(err)
	}
	return tuple
}
func relationValues(t *testing.T, f alphaFixture) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(), `INSERT INTO divine_labor_valuations(world_id,good_key,rarity_spread,rarity_volume,divine_value,calc_tick) SELECT $1,key,1,1,1,100 FROM goods WHERE status='active' ON CONFLICT(world_id,good_key) DO UPDATE SET divine_value=1`, f.world)
	if err != nil {
		t.Fatal(err)
	}
}
func TestRelationEverydayExactThresholdNoDiceOrStandingDebit(t *testing.T) {
	for _, prayer := range []string{religion.PrayerPotniaVision, religion.PrayerPotniaWalls, religion.PrayerGreenShoots, religion.PrayerAsterionThread, religion.PrayerStillnessPassage} {
		t.Run(prayer, func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			spec := religion.PrayerSpecs[prayer]
			if _, err := f.pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=$3,kharis_rate=0,kharis_calc_tick=100 WHERE player_id=$1 AND world_id=$2`, f.player, f.world, spec.MinKharis); err != nil {
				t.Fatal(err)
			}
			dice := &relationDice{roll: 99}
			f.h.RiteDice = dice
			before := relationTuple(t, f)
			r := relationCast(f, f.cities[0], map[string]any{"prayer": prayer, "offer_multiplier": 2})
			if r.Code != 200 || !strings.Contains(r.Body.String(), `"success":true`) {
				t.Fatalf("exact threshold response %d %s", r.Code, r.Body.String())
			}
			if dice.calls != 0 {
				t.Errorf("everyday prayer rolled dice %d times", dice.calls)
			}
			if after := relationTuple(t, f); after != before {
				t.Errorf("standing tuple changed: %s -> %s", before, after)
			}
			for good, need := range spec.Offering {
				start := 100.0
				if good == "silver" {
					start = 500
				}
				if got := f.stock(t, f.cities[0], good); got != start-need {
					t.Errorf("multiplier must be ignored: %s stock %g want %g", good, got, start-need)
				}
			}
			var success bool
			var spent, mult float64
			if err := f.pool.QueryRow(ctx, `SELECT (payload->>'success')::boolean,(payload->>'kharis_spent')::float8,(payload->>'offer_multiplier')::float8 FROM events WHERE world_id=$1 AND event_type='RiteCast'`, f.world).Scan(&success, &spent, &mult); err != nil {
				t.Fatal(err)
			}
			if !success || spent != 0 || mult != 1 {
				t.Errorf("wrong event outcome: %v spent %g multiplier %g", success, spent, mult)
			}
			if !strings.Contains(r.Body.String(), `"kharis_spent":0`) {
				t.Error("response must retain zero standing spend")
			}
		})
	}
}
func TestRelationComposedOfferingThresholdAndRollback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		factor float64
		status int
	}{{"short", .5, 400}, {"equal", 1, 200}, {"extra", 2, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAlphaFixture(t)
			relationValues(t, f)
			before := relationTuple(t, f)
			r := relationCast(f, f.cities[0], map[string]any{"prayer": religion.PrayerPotniaWalls, "offering": map[string]float64{"grain": 8 * tc.factor, "timber": 4 * tc.factor}})
			if r.Code != tc.status {
				t.Fatalf("response %d %s", r.Code, r.Body.String())
			}
			if relationTuple(t, f) != before {
				t.Fatal("composed prayer changed standing")
			}
			if tc.status == 400 {
				if !strings.Contains(r.Body.String(), "8 grain + 4 timber") {
					t.Error("refusal must name traditional recipe")
				}
				if f.stock(t, f.cities[0], "grain") != 100 {
					t.Error("rejected offering debited goods")
				}
				var n int
				if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM events WHERE world_id=$1 AND event_type='RiteCast')+(SELECT count(*) FROM temple_rest tr JOIN buildings b ON b.id=tr.building_id JOIN settlements s ON s.id=b.settlement_id WHERE s.world_id=$1)`, f.world).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 0 {
					t.Error("rejected offering left event/cooldown")
				}
			} else {
				if f.stock(t, f.cities[0], "grain") != 100-8*tc.factor {
					t.Error("composed offering not charged")
				}
				if !strings.Contains(r.Body.String(), "a larger gift lasts longer") {
					t.Errorf("verdict implies more helps: %s", r.Body.String())
				}
			}
		})
	}
}
func TestRelationLegacyRollAndMultiplierWithoutStandingDebit(t *testing.T) {
	for _, roll := range []int{0, 99} {
		t.Run(fmt.Sprintf("roll_%d", roll), func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			if _, err := f.pool.Exec(ctx, `UPDATE settlements SET culture_id='akhaier' WHERE id=$1`, f.cities[0]); err != nil {
				t.Fatal(err)
			}
			dice := &relationDice{roll: roll}
			f.h.RiteDice = dice
			before := relationTuple(t, f)
			r := relationCast(f, f.cities[0], map[string]any{"prayer": "akhaier_oracle_deposits", "offer_multiplier": .5})
			want := `"success":true`
			if roll == 99 {
				want = `"success":false`
			}
			if r.Code != 200 || !strings.Contains(r.Body.String(), want) {
				t.Fatalf("legacy response %d %s", r.Code, r.Body.String())
			}
			if dice.calls != 1 {
				t.Errorf("legacy dice calls %d", dice.calls)
			}
			if relationTuple(t, f) != before {
				t.Error("legacy prayer debited standing")
			}
			if f.stock(t, f.cities[0], "oil") != 90 || f.stock(t, f.cities[0], "wine") != 95 {
				t.Error("legacy multiplier not applied")
			}
		})
	}
}

func TestRelationBelowThresholdPreservesAllState(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE player_world_records SET kharis_amount=4.999,kharis_rate=0 WHERE player_id=$1 AND world_id=$2`, f.player, f.world); err != nil {
		t.Fatal(err)
	}
	before := relationTuple(t, f)
	r := f.cast(f.cities[0], religion.PrayerPotniaWalls)
	if r.Code != 400 {
		t.Fatalf("below threshold %d %s", r.Code, r.Body.String())
	}
	if relationTuple(t, f) != before || f.stock(t, f.cities[0], "grain") != 100 {
		t.Fatal("refused prayer changed state")
	}
}
func TestRelationComposedSubstitutionAndInvalidAmounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		gift   map[string]float64
		status int
	}{
		{"substitute", map[string]float64{"fish": 30}, 200},
		{"negative", map[string]float64{"grain": 20, "timber": -1}, 400},
		{"unknown", map[string]float64{"unknown": 20}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAlphaFixture(t)
			relationValues(t, f)
			r := relationCast(f, f.cities[0], map[string]any{"prayer": religion.PrayerPotniaWalls, "offering": tc.gift})
			if r.Code != tc.status {
				t.Fatalf("%d %s", r.Code, r.Body.String())
			}
			if tc.status == 200 && (f.stock(t, f.cities[0], "fish") != 70 || f.stock(t, f.cities[0], "grain") != 100) {
				t.Fatal("substitution was not actually used")
			}
			if tc.status == 400 && f.stock(t, f.cities[0], "grain") != 100 {
				t.Fatal("invalid composition changed stores")
			}
		})
	}
}

func TestRelationProvincePublishesRequirementsWithoutCost(t *testing.T) {
	f := newAlphaFixture(t)
	var prov uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT province_id FROM settlements WHERE id=$1`, f.cities[0]).Scan(&prov); err != nil {
		t.Fatal(err)
	}
	ph := NewProvinceHandler(f.pool, nil, f.h.clk, nil, nil)
	router := chi.NewRouter()
	router.With(auth.Middleware(auth.NewService(f.pool, "alpha-secret"))).Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.world.String()+"/provinces/"+prov.String(), nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("province %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Settlement map[string]json.RawMessage `json:"settlement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload.Settlement["rite_kharis_cost"]; exists {
		t.Fatal("obsolete cost advertised")
	}
	var prayers []struct {
		GodID string `json:"god_id"`
	}
	if err := json.Unmarshal(payload.Settlement["available_prayers"], &prayers); err != nil {
		t.Fatal(err)
	}
	if len(prayers) != 5 {
		t.Fatalf("catalogue length %d", len(prayers))
	}
	for _, p := range prayers {
		if p.GodID == "" {
			t.Fatal("missing server-owned god key")
		}
	}
}
