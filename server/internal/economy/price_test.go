package economy

import (
	"math"
	"testing"

	"formatet/megaron/server/internal/events"
)

func TestLocalPrice_AtReference(t *testing.T) {
	// Stock exactly at reference (rate×TicksPerDay×referenceBufferDays) with
	// that same rate → projected lands exactly on reference → price = baseValue.
	baseValue := 1.0
	rate := 1.0
	reference := ProductionReference(rate)
	stock := reference - rate*float64(events.TicksPerDay) // projected = stock + rate*24 = reference
	price := LocalPrice(baseValue, stock, rate)
	if math.Abs(price-baseValue) > 0.01 {
		t.Errorf("at reference stock price should equal base: got %.3f want %.3f", price, baseValue)
	}
}

func TestLocalPrice_Shortage(t *testing.T) {
	// Empty stock, no production → price should be ~3× base (shortage=1 → multiplier at max)
	price := LocalPrice(1.0, 0, 0)
	if price < 2.5 {
		t.Errorf("empty stock should give high price, got %.3f", price)
	}
}

func TestLocalPrice_Surplus(t *testing.T) {
	// Stock at 2× the reference anchor → price should be ~0.5× base (surplus clamps at 1).
	baseValue, rate := 1.0, 0.0
	stock := 2 * ProductionReference(rate)
	price := LocalPrice(baseValue, stock, rate)
	if price > 0.6 {
		t.Errorf("stock at 2x reference should give low price, got %.3f", price)
	}
}

func TestLocalPrice_NeverNegative(t *testing.T) {
	// Price must never go negative regardless of parameters
	for _, stock := range []float64{0, 50, 100, 200} {
		p := LocalPrice(1.0, stock, -0.5)
		if p < 0 {
			t.Errorf("price should never be negative, got %.3f at stock=%.0f", p, stock)
		}
	}
}

func TestLocalPrice_RateProjection(t *testing.T) {
	// High positive rate should push projected stock up → lower price than zero-rate
	priceWithRate := LocalPrice(1.0, 0, 1.0) // producing fast
	priceZeroRate := LocalPrice(1.0, 0, 0)   // not producing
	if priceWithRate >= priceZeroRate {
		t.Errorf("positive rate should reduce shortage price: withRate=%.3f zeroRate=%.3f",
			priceWithRate, priceZeroRate)
	}
}

// TestLocalPrice_PerTickLookahead pins the tick-based lookahead so an accidental
// revert to the old per-minute constant (60×24=1440) is caught. With rate=1/tick
// the reference anchor is rate-derived (72 = 1×24×3) and the tick-correct
// projection (24) sits well below it → mild shortage, price above base. A
// wrongly-reintroduced per-minute lookahead (1440) would sit far above any
// reasonable reference → deep surplus, price below base. The two constants
// give opposite price directions, so this stays a decisive regression guard.
func TestLocalPrice_PerTickLookahead(t *testing.T) {
	projected := 0.0 + 1.0*float64(events.TicksPerDay)
	if projected != 24 {
		t.Fatalf("test premise broke: expected TicksPerDay=24, got projected=%.1f", projected)
	}
	price := LocalPrice(1.0, 0, 1.0)
	if price <= 1.0 {
		t.Errorf("per-tick lookahead should yield a mild-shortage price above base, got %.3f "+
			"(a per-minute lookahead would over-project into deep surplus and give a price below base)", price)
	}
}
