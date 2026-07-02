package economy

import (
	"math"
	"testing"

	"github.com/poleia/server/internal/events"
)

func TestLocalPrice_AtReference(t *testing.T) {
	// Stock exactly at reference (cap×0.3) with 0 rate → price = baseValue
	baseValue := 1.0
	cap := 100.0
	stock := cap * referenceRatio // 30
	price := LocalPrice(baseValue, stock, 0, cap)
	if math.Abs(price-baseValue) > 0.01 {
		t.Errorf("at reference stock price should equal base: got %.3f want %.3f", price, baseValue)
	}
}

func TestLocalPrice_Shortage(t *testing.T) {
	// Empty stock → price should be ~3× base (shortage=1 → multiplier at max)
	price := LocalPrice(1.0, 0, 0, 100)
	if price < 2.5 {
		t.Errorf("empty stock should give high price, got %.3f", price)
	}
}

func TestLocalPrice_Surplus(t *testing.T) {
	// Full stock → price should be ~0.5× base
	price := LocalPrice(1.0, 100, 0, 100)
	if price > 0.6 {
		t.Errorf("full stock should give low price, got %.3f", price)
	}
}

func TestLocalPrice_NeverNegative(t *testing.T) {
	// Price must never go negative regardless of parameters
	for _, stock := range []float64{0, 50, 100, 200} {
		p := LocalPrice(1.0, stock, -0.5, 100)
		if p < 0 {
			t.Errorf("price should never be negative, got %.3f at stock=%.0f", p, stock)
		}
	}
}

func TestLocalPrice_RateProjection(t *testing.T) {
	// High positive rate should push projected stock up → lower price than zero-rate
	priceWithRate := LocalPrice(1.0, 0, 1.0, 100) // producing fast
	priceZeroRate := LocalPrice(1.0, 0, 0, 100)   // not producing
	if priceWithRate >= priceZeroRate {
		t.Errorf("positive rate should reduce shortage price: withRate=%.3f zeroRate=%.3f",
			priceWithRate, priceZeroRate)
	}
}

// TestLocalPrice_PerTickLookahead pins the tick-based lookahead so an accidental
// revert to the old per-minute constant (60×24=1440) is caught. With rate=1/tick
// the projected stock is stock + rate×TicksPerDay = 0 + 24 = 24 (< reference 30 →
// mild shortage), NOT 0 + 1440 = 1440 (capped, deep surplus). The two constants
// give opposite price directions here, so this is a decisive regression guard.
func TestLocalPrice_PerTickLookahead(t *testing.T) {
	cap := 100.0
	reference := cap * referenceRatio // 30
	projected := 0.0 + 1.0*float64(events.TicksPerDay)
	if projected >= reference {
		t.Fatalf("test premise broke: projected %.1f should be below reference %.1f with per-tick lookahead", projected, reference)
	}
	// Per-tick: projected 24 < ref 30 → price ABOVE base (shortage).
	price := LocalPrice(1.0, 0, 1.0, cap)
	if price <= 1.0 {
		t.Errorf("per-tick lookahead should yield a mild-shortage price above base, got %.3f "+
			"(a per-minute lookahead would over-project to cap and give a surplus price below base)", price)
	}
}
