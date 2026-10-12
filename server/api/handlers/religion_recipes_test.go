package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"formatet/megaron/server/internal/religion"
)

// The literal expected recipes come from Timothy's decision, independently of
// the production catalogue. A web-style default Offer must debit exactly them,
// and freeze that same sacrifice in RiteCast, even when the answer is silent.
func TestAlphaOfferDebitsDecisionRecipeExactly(t *testing.T) {
	expected := map[string]map[string]float64{
		religion.PrayerPotniaVision:     {"grain": 10, "silver": 100},
		religion.PrayerPotniaWalls:      {"grain": 8, "timber": 4},
		religion.PrayerGreenShoots:      {"grain": 15, "silver": 50},
		religion.PrayerAsterionThread:   {"timber": 4, "silver": 100},
		religion.PrayerStillnessPassage: {"fish": 6, "silver": 100},
	}
	for id, recipe := range expected {
		for _, silent := range []bool{false, true} {
			name := id + "/answered"
			if silent {
				name = id + "/silent"
			}
			t.Run(name, func(t *testing.T) {
				f := newAlphaFixture(t)
				ctx := context.Background()
				if silent {
					f.h.RiteDice = alphaDice{99}
				}
				rows, err := f.pool.Query(ctx, `SELECT good_key,amount FROM settlement_goods WHERE settlement_id=$1`, f.cities[0])
				if err != nil {
					t.Fatal(err)
				}
				before := map[string]float64{}
				for rows.Next() {
					var good string
					var n float64
					if err := rows.Scan(&good, &n); err != nil {
						t.Fatal(err)
					}
					before[good] = n
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				response := f.cast(f.cities[0], id)
				if response.Code != 200 {
					t.Fatalf("Offer status%d %s", response.Code, response.Body.String())
				}
				for good, amount := range before {
					if after := f.stock(t, f.cities[0], good); after != amount-recipe[good] {
						t.Fatalf("Offer debited %s: before%g after%g, expected debit%g", good, amount, after, recipe[good])
					}
				}
				var raw []byte
				if err := f.pool.QueryRow(ctx, `SELECT payload FROM events WHERE stream_id=$1 AND event_type='RiteCast' ORDER BY created_at DESC LIMIT 1`, f.cities[0]).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var payload struct {
					Offering map[string]float64 `json:"offering"`
					Success  bool               `json:"success"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(payload.Offering, recipe) || payload.Success == silent {
					t.Fatalf("frozen outcome %s", raw)
				}
			})
		}
	}
}
