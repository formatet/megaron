package economy

import "math"

// CapWarningLeadTicks is a forecast horizon, not elapsed wall time.
const CapWarningLeadTicks = 24

// StorageForecast is a projection at current rates and known scheduled receipts.
// A missing FullInTicks means the current projection never fills the store.
type StorageForecast struct {
	GoodKey     string   `json:"good_key"`
	Amount      float64  `json:"amount"`
	Cap         float64  `json:"cap"`
	NetPerTick  float64  `json:"net_per_tick"`
	Incoming    float64  `json:"incoming"`
	FullInTicks *float64 `json:"full_in_ticks"`
	Warning     bool     `json:"warning"`
}
type StorageReceipt struct {
	InTicks  float64
	Quantity float64
}

// ProjectStorage walks arrival times rather than pretending distant cargo is
// already in store. Constant net flow is an explicit forecast, never a promise.
func ProjectStorage(good string, stock, cap, net float64, receipts []StorageReceipt) StorageForecast {
	out := StorageForecast{GoodKey: good, Amount: math.Min(cap, math.Max(0, stock)), Cap: cap, NetPerTick: net}
	at, amount := 0., out.Amount
	filled := func(when float64) {
		if out.FullInTicks == nil {
			out.FullInTicks = &when
			out.Warning = when <= CapWarningLeadTicks
		}
	}
	if amount >= cap {
		filled(0)
	}
	for _, r := range receipts {
		arrival := math.Max(at, r.InTicks)
		if net > 0 && amount < cap && amount+net*(arrival-at) >= cap {
			filled(at + (cap-amount)/net)
		}
		amount = math.Max(0, amount+net*(arrival-at)) + r.Quantity
		out.Incoming += r.Quantity
		if amount >= cap {
			filled(arrival)
		}
		at = arrival
	}
	if out.FullInTicks == nil && net > 0 {
		filled(at + math.Max(0, cap-amount)/net)
	}
	return out
}
