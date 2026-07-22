package messenger

// Regression test for the offer dead-zone measured 2026-07-22
// (rapport_bronsmatning_20260722.md).
//
// A trade offer can only be read or accepted while its bearer stands at the
// destination: the inbox query and both trade-accept paths in
// api/handlers/messenger.go all require status='delivered', and the messenger
// leaves that state the moment ReturnHandler fires. The offer's own expiry was
// scheduled 168 ticks after arrival while every messenger — offer-bearing or
// not — was sent home after 48. Between those two numbers the offer was still
// 'pending', invisible in the recipient's inbox, impossible to accept, and the
// sender's escrow stayed locked for the remaining 120 ticks.
//
// The invariant: an offer-bearing messenger's stay IS the offer's life. Both
// must come from the same constant, so neither can be tuned without the other.

import "testing"

func TestStayTicks_OfferBearerStaysAsLongAsItsOfferLives(t *testing.T) {
	if got := stayTicks(true); got != OfferExpiryTicks {
		t.Fatalf("offer bearer stays %d ticks, offer lives %d — the gap is a window "+
			"where the offer is pending but nobody can accept it", got, OfferExpiryTicks)
	}
}

func TestStayTicks_PlainMessageKeepsShortStay(t *testing.T) {
	if got := stayTicks(false); got != ReplyStayTicks {
		t.Fatalf("plain message stay = %d, want %d", got, ReplyStayTicks)
	}
}

// A plain message must not be held as long as an offer — the short stay is what
// makes an unanswered messenger come home promptly.
func TestStayTicks_OfferStayIsLongerThanPlainStay(t *testing.T) {
	if stayTicks(true) <= stayTicks(false) {
		t.Fatalf("offer stay %d must exceed plain stay %d", stayTicks(true), stayTicks(false))
	}
}
