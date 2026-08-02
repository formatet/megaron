package handlers

// P6 (soak 2026-07-18): a galley disbanded the instant it garrisoned
// ("grain_shortage") and garrisoned spearmen starved to death in a city whose
// grain rate LOOKED healthy — because army upkeep is a separate once-daily
// discrete debit (combat/upkeep.go), never folded into settlement_goods'
// continuous rate. upkeepNetPerDay is the pure function that nets the two
// together so `status`/`recruit --list`/the Recruit response can warn BEFORE
// a build/recruit pushes a settlement into upkeep deficit. Pure — no DB.

import (
	"math"
	"testing"
)

func TestUpkeepNetPerDay(t *testing.T) {
	cases := []struct {
		name          string
		grainRate     float64 // settlement_goods rate, per tick (citizens already netted)
		silverRate    float64
		up            upkeepAmount // upkeep this city already pays, gross
		circulated    float64      // Del C: sold spent back into the town, same tick
		wantGrainNet  float64
		wantSilverNet float64
	}{
		{
			name:          "healthy grain rate still net negative once army upkeep is counted",
			grainRate:     0.2, // 4.8/day at TicksPerDay=24 — looks fine in isolation
			silverRate:    0.1,
			up:            upkeepAmount{Grain: 11.05, Silver: 5.82}, // matches TestArmyUpkeep_SumsGarrisonViaUnitUpkeep
			wantGrainNet:  0.2*24 - 11.05,
			wantSilverNet: 0.1*24 - 5.82,
		},
		{
			name:          "no garrison — net equals raw production",
			grainRate:     1.0,
			silverRate:    0.5,
			up:            upkeepAmount{},
			wantGrainNet:  1.0 * 24,
			wantSilverNet: 0.5 * 24,
		},
		{
			name:          "negative production compounds with upkeep, doesn't cancel it",
			grainRate:     -0.5,
			silverRate:    0,
			up:            upkeepAmount{Grain: 5, Silver: 2},
			wantGrainNet:  -0.5*24 - 5,
			wantSilverNet: 0 - 2,
		},
		{
			// Del C: a city whose whole garrison stands at home loses only
			// (1−share) of the silver. Before this, the same city read as −2/day
			// and the recruit surface called it unsustainable while it was in
			// fact in surplus. Grain is untouched — soldiers eat their rations.
			name:          "sold circulating back is not drain",
			grainRate:     0,
			silverRate:    0.05, // 1.2/day
			up:            upkeepAmount{Grain: 5, Silver: 2},
			circulated:    1.4, // share 0.7 of the whole 2
			wantGrainNet:  0 - 5,
			wantSilverNet: 1.2 - 0.6,
		},
		{
			// A garrison that marches out keeps billing its home town, but stops
			// spending there: circulated falls to 0 and the drain is the gross.
			// Same city, same units — the difference is only where they stand.
			name:          "field army: nothing circulates, full gross drains",
			grainRate:     0,
			silverRate:    0.05,
			up:            upkeepAmount{Grain: 5, Silver: 2},
			circulated:    0,
			wantGrainNet:  0 - 5,
			wantSilverNet: 1.2 - 2,
		},
	}
	const eps = 1e-9
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotGrain, gotSilver := upkeepNetPerDay(tc.grainRate, tc.silverRate, tc.up, tc.circulated)
			if math.Abs(gotGrain-tc.wantGrainNet) > eps {
				t.Errorf("grainNetPerDay = %v, want %v", gotGrain, tc.wantGrainNet)
			}
			if math.Abs(gotSilver-tc.wantSilverNet) > eps {
				t.Errorf("silverNetPerDay = %v, want %v", gotSilver, tc.wantSilverNet)
			}
		})
	}
}
