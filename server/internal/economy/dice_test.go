package economy

// fakeDice is a Dice test double that always returns a fixed result, so a
// test can force "the caravan always survives" or "the caravan always sinks"
// instead of depending on math/rand's distribution — used to pin tests whose
// subject is something other than the loss roll itself (e.g. the stale-cap
// bug in trade_delivery_stale_cap_test.go), and to prove the seam actually
// drives DeliveryHandler/TradeReturnHandler's behaviour (dice_seam_test.go).
type fakeDice struct {
	float float64 // returned by Float64()
	n     int     // returned by Intn(...)
}

func (d fakeDice) Float64() float64 { return d.float }
func (d fakeDice) Intn(int) int     { return d.n }

// neverLosesDice rolls 1.0, always >= tradeRiskPct — the caravan is never lost.
func neverLosesDice() Dice { return fakeDice{float: 1, n: 0} }

// alwaysLosesDice rolls 0.0, always < tradeRiskPct — the caravan is always lost.
func alwaysLosesDice() Dice { return fakeDice{float: 0, n: 0} }
