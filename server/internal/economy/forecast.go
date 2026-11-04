package economy

import (
	"sort"

	"formatet/megaron/server/internal/ledger"
)

// FoodLowDays: food that lasts fewer game days than this, while the net is negative,
// is "running low" (Timothy 2026-11-04).
const FoodLowDays = 24

// Flow is one expected post of a good's next game day. Categories are the ledger's:
// the forecast looks forward at the same posts the books record backward.
type Flow struct {
	Category string  `json:"category"`
	Label    string  `json:"label"`
	Amount   float64 `json:"amount"` // per game day, signed
}

// GoodForecast is the expected net of one good per game day, post by post.
type GoodForecast struct {
	GoodKey string  `json:"good_key"`
	Net     float64 `json:"net"`
	Flows   []Flow  `json:"flows"`
}

// FoodForecast is the whole food basket: stock, the net the growth gate also
// uses (production − rations − tithes + the herd's surplus), and how long it lasts.
type FoodForecast struct {
	Stock       float64  `json:"stock"`
	Net         float64  `json:"net"`
	HerdSurplus float64  `json:"herd_surplus"`
	Days        *float64 `json:"days"` // nil when the net is not negative
	Flows       []Flow   `json:"flows"`
}

// CityForecast is the single forward owner for one city: tick, notices and surfaces read it.
type CityForecast struct {
	Goods []GoodForecast `json:"goods"`
	Food  FoodForecast   `json:"food"`
}

// FoodRunway is how many game days the food lasts at a negative net; nil when the net is not negative.
func FoodRunway(stock, net float64) *float64 {
	if net >= 0 {
		return nil
	}
	d := 0.0
	if stock > 0 {
		d = stock / -net
	}
	return &d
}

// FoodLow reports whether the food runs out inside FoodLowDays at a negative net.
func FoodLow(f FoodForecast) bool {
	return f.Net < 0 && f.Days != nil && *f.Days < FoodLowDays
}

// AddFlow appends a non-zero flow, labelled.
func AddFlow(flows []Flow, category string, amount float64) []Flow {
	if amount > -1e-9 && amount < 1e-9 {
		return flows
	}
	return append(flows, Flow{Category: category, Label: ledger.Label(category), Amount: amount})
}

// SortFlows orders flows by category for stable output.
func SortFlows(flows []Flow) {
	sort.Slice(flows, func(i, j int) bool { return flows[i].Category < flows[j].Category })
}
