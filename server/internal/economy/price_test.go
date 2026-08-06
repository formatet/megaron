package economy

import (
	"math"
	"testing"
)

func TestLocalPrice_AtReference(t *testing.T) {
	// Stock exactly at reference (rate×referenceBufferTicks) with that same
	// rate → projected lands exactly on reference → price = baseValue.
	baseValue := 1.0
	rate := 1.0
	reference := ProductionReference(rate)
	stock := reference - rate // projected = stock + rate = reference
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
// revert to a stale, much-larger lookahead constant (like the retired
// per-minute value 60×24=1440) is caught. ⭐ CANON 2026-08-06: a tick IS the
// day now, so with rate=1/tick the correct lookahead projects 1 unit ahead,
// while the reference anchor (referenceBufferTicks × rate = 3) sits below
// referenceFloorUnits (10) and floors there — projected(1) stays well below
// reference(10) → shortage, price above base. A wrongly-reintroduced large
// lookahead (e.g. 1440) would blow the projection far past any reasonable
// reference → deep surplus, price below base. The two constants give
// opposite price directions, so this stays a decisive regression guard.
func TestLocalPrice_PerTickLookahead(t *testing.T) {
	// Guard the actual behaviour (not a deleted constant): the reference
	// anchor for rate=1 is referenceBufferTicks×1=3, which floors to
	// referenceFloorUnits (10) — pin this so the floor/no-floor branch below
	// doesn't silently change out from under the assertion if
	// referenceBufferTicks is ever retuned.
	if got := ProductionReference(1.0); got != referenceFloorUnits {
		t.Fatalf("test premise broke: ProductionReference(1.0) = %.1f, want the floor %.1f", got, referenceFloorUnits)
	}
	price := LocalPrice(1.0, 0, 1.0)
	if price <= 1.0 {
		t.Errorf("per-tick lookahead should yield a shortage price above base, got %.3f "+
			"(a stale, much-larger lookahead would over-project into deep surplus and give a price below base)", price)
	}
}
