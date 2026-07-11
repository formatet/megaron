// Package economy implements goods, pricing, and production for Poleia settlements.
package economy

import (
	"math"
	"time"
)

// Good key constants — match the goods table.
const (
	GoodGrain   = "grain"
	GoodFish    = "fish"
	GoodTimber  = "timber"
	GoodCedar   = "cedar"
	GoodCopper  = "copper"
	GoodTin     = "tin"
	GoodSilver  = "silver"
	GoodWine    = "wine"
	GoodOil     = "oil"
	GoodHorses  = "horses"
	GoodBronze  = "bronze"
	GoodPurple  = "purple"
	GoodPottery = "pottery"
	GoodLuxury  = "luxury"
	GoodCult    = "cult" // internal sacred good produced by temple labor → converted to kharis daily
)

// FoodGoods är de varor som räknas som mat för kost-variation (Timothy 2026-07-11: bred palett).
var FoodGoods = []string{GoodGrain, GoodFish, GoodWine, GoodOil}

// Good is the catalog entry for a tradeable good.
type Good struct {
	Key       string
	Name      string
	Tier      string // 'commodity' | 'manufactured'
	Category  string // 'staple' | 'strategic' | 'prestige' | 'bulk'
	BaseValue float64
	Weight    float64 // transport cost multiplier
}

// GoodState is a lazy-eval record for a settlement's stock of one good.
type GoodState struct {
	GoodKey string
	Amount  float64
	Rate    float64 // production rate per minute
	Cap     float64
	CalcAt  time.Time
}

// Current returns the stock amount at time at, capped at Cap and floored at 0.
func (g GoodState) Current(at time.Time) float64 {
	elapsed := at.Sub(g.CalcAt).Minutes()
	v := g.Amount + elapsed*g.Rate
	return math.Min(math.Max(v, 0), g.Cap)
}
