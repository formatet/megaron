package kharis

import (
	"maps"
	"math"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

func richPlan(level int, rich string, stock map[string]float64) TempleOffer {
	t := []templeRef{{id: uuid.New(), god: "potnia", level: level, staffed: 2, rich: rich}}
	return planCityOffers(uuid.New(), "c", t, stock, religion.FixedDivineValues)[0]
}

func TestRichOfferingOnOffAndMissing(t *testing.T) {
	stock := map[string]float64{"grain": 10, "fish": 10}
	off := richPlan(1, "", stock)
	on := richPlan(1, "fish", stock)
	if off.RichPaid || off.RichGood != "" {
		t.Fatalf("off: %+v", off)
	}
	// 0.5 * 0.25 * 1 * grain(1) / fish(2) = 0.0625
	if !on.RichPaid || math.Abs(on.Offering["fish"]-0.0625) > 1e-9 || on.Offering["grain"] != 0.25 {
		t.Fatalf("on: %+v", on)
	}
	if math.Abs(on.EffectiveWeight-off.EffectiveWeight*1.5) > 1e-9 {
		t.Fatalf("weight %g vs %g", on.EffectiveWeight, off.EffectiveWeight)
	}
	// Good missing: no partial bonus, nothing drawn.
	miss := richPlan(1, "incense", stock)
	if miss.RichPaid || miss.Offering["incense"] != 0 || miss.EffectiveWeight != off.EffectiveWeight || miss.RichGood != "incense" {
		t.Fatalf("missing: %+v", miss)
	}
	// Short of the full amount: still no partial bonus.
	short := richPlan(1, "fish", map[string]float64{"grain": 10, "fish": 0.06})
	if short.RichPaid || short.Offering["fish"] != 0 || short.EffectiveWeight != off.EffectiveWeight {
		t.Fatalf("short: %+v", short)
	}
	// Unpriced good is ignored.
	if o := richPlan(1, "unobtainium", stock); o.RichPaid || o.RichGood != "" {
		t.Fatalf("unpriced: %+v", o)
	}
}

func TestRichOfferingDrawnAfterBaseTithe(t *testing.T) {
	// Only 0.25 grain exists. Base tithe takes it all; rich grain cannot be paid from what is left.
	o := richPlan(1, "grain", map[string]float64{"grain": 0.25})
	if !o.Fed || o.RichPaid || o.Offering["grain"] != 0.25 {
		t.Fatalf("base must come first: %+v", o)
	}
	// With enough for both, both are drawn: 0.25 + 0.125.
	o = richPlan(1, "grain", map[string]float64{"grain": 0.375})
	if !o.RichPaid || math.Abs(o.Offering["grain"]-0.375) > 1e-9 {
		t.Fatalf("both: %+v", o)
	}
	// A rich offering never starves a second temple's base tithe.
	ts := []templeRef{{id: uuid.New(), god: "asterion", level: 1, staffed: 2, rich: "grain"}, {id: uuid.New(), god: "potnia", level: 1, staffed: 2}}
	out := planCityOffers(uuid.New(), "c", ts, map[string]float64{"grain": 0.5}, religion.FixedDivineValues)
	if !out[0].Fed || !out[1].Fed || out[0].RichPaid {
		t.Fatalf("second base starved: %+v", out)
	}
}

func TestRichOfferingLivestockInBaseTithe(t *testing.T) {
	// 0.25 grain due; livestock is worth 2, so 0.125.
	o := richPlan(1, "", map[string]float64{"livestock": 1})
	if !o.Fed || math.Abs(o.Offering["livestock"]-0.125) > 1e-9 {
		t.Fatalf("livestock base: %+v", o)
	}
}

func TestRichOfferingTasteLowersAmountAndLevelScales(t *testing.T) {
	if got := richAmount(2, "potnia", "silver", religion.FixedDivineValues); math.Abs(got-0.5*0.25*4/2) > 1e-9 {
		t.Fatalf("L2 silver %g", got)
	}
	// Taste lowers the amount: a god who prizes fish (taste 2) asks half as much.
	was := templeTaste
	defer func() { templeTaste = was }()
	templeTaste = func(god, good string) float64 {
		if good == "fish" {
			return 2
		}
		return 1
	}
	if got := richAmount(1, "potnia", "fish", religion.FixedDivineValues); math.Abs(got-0.5*0.25/(2*2)) > 1e-9 {
		t.Fatalf("taste ignored: %g", got)
	}
	templeTaste = was
	base := richAmount(1, "potnia", "fish", map[string]float64{"grain": 1, "fish": 4})
	if math.Abs(base-0.5*0.25/4) > 1e-9 {
		t.Fatalf("value divides: %g", base)
	}
}

func TestRichOfferingLeavesCapAndBaseUntouched(t *testing.T) {
	stock := map[string]float64{"grain": 10, "silver": 10, "incense": 10, "fish": 10}
	off := richPlan(3, "", maps.Clone(stock))
	on := richPlan(3, "fish", maps.Clone(stock))
	if on.PaidFraction != off.PaidFraction || on.Weight != off.Weight || on.Required["grain"] != off.Required["grain"] {
		t.Fatalf("base changed: %+v vs %+v", on, off)
	}
	// Rich bonus rides on the weight only; the kharis ceiling reads Level, which is unchanged.
	if on.Level != off.Level {
		t.Fatal("level changed")
	}
}
