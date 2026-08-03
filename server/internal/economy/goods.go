// Package economy implements goods, pricing, and production for Megaron settlements.
package economy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
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

// ErrUnknownGood is returned by IsShippableGood when goodKey isn't in the
// goods catalog at all — distinct from a known-but-unshippable good (cult),
// so callers can give "unknown good" and "cult can't be shipped" different
// 400 messages.
var ErrUnknownGood = errors.New("unknown good")

// IsShippableGood reports whether a good may be physically moved between
// settlements — internal transfer (api/handlers/province.go Trade), trade
// offer, or any other caravan. Answers by reading the catalog's weight
// column, never a hardcoded Go list: the goods table holds 17 keys today,
// while the Good* constants above cover only 15 (stone and livestock are
// missing) — an allowlist built from those constants would wrongly refuse
// two real goods. Weight 0 marks a good produced and consumed in place,
// never carried — today that is cult alone (migration 055, weight=0):
// cult is devotion, produced by temple labor and converted to kharis in
// place (internal/kharis/tick.go), never a stock that accrues in
// settlement_goods (migration 094 deleted the last such rows; sack.go
// already relies on this same weight>0 signal to exclude cult from loot for
// the identical reason).
//
// Silver is deliberately NOT excluded here — migration 057 made it an
// ordinary settlement_goods row, and CLAUDE.md requires it stay transferable
// between a Wanax's own settlements. The messenger trade-offer surface
// (api/handlers/messenger.go) also excludes silver from offers, but for an
// unrelated reason (an offer negotiates silver-for-goods, never
// silver-for-silver) — don't reuse that predicate's exclusion set here.
//
// Returns the good's weight (for the caller's own transport-cost math) and
// ErrUnknownGood if goodKey isn't in the catalog at all.
func IsShippableGood(ctx context.Context, tx Tx, goodKey string) (weight float64, shippable bool, err error) {
	err = tx.QueryRow(ctx, `SELECT weight FROM goods WHERE key = $1`, goodKey).Scan(&weight)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, ErrUnknownGood
		}
		return 0, false, fmt.Errorf("look up good %q: %w", goodKey, err)
	}
	return weight, weight > 0, nil
}
