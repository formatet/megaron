package religion

import (
	"reflect"
	"testing"
)

func TestAlphaTraditionalRecipesOctoberDecision(t *testing.T) {
	expected := map[string]map[string]float64{
		PrayerPotniaVision:     {"grain": 10, "silver": 100},
		PrayerPotniaWalls:      {"grain": 8, "timber": 4},
		PrayerGreenShoots:      {"grain": 15, "silver": 50},
		PrayerAsterionThread:   {"timber": 4, "silver": 100},
		PrayerStillnessPassage: {"fish": 6, "silver": 100},
	}
	for id, recipe := range expected {
		t.Run(id, func(t *testing.T) {
			if !reflect.DeepEqual(PrayerSpecs[id].Offering, recipe) {
				t.Fatalf("catalogue recipe %v, want %v", PrayerSpecs[id].Offering, recipe)
			}
		})
	}
}
