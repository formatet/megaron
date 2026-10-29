package handlers

import (
	"testing"

	"formatet/megaron/server/internal/economy"
)

// A refused hex placement names what the hex does yield (GPT-player logs
// 2026-10-24: timber on cedar said only "no production option").
func TestHexNoOptionMessageNamesTheYield(t *testing.T) {
	opt := &economy.HexOption{RatePerGood: map[string]float64{"cedar": 0.5, "grain": 0, "timber": 0}}
	if got, want := hexNoOptionMessage(opt, "timber"), "this hex gives no timber — it yields cedar"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if got := hexNoOptionMessage(nil, "timber"); got != "this hex has no production option for that good" {
		t.Errorf("unknown hex message = %q", got)
	}
}
