package main

import (
	"strings"
	"testing"
)

// TestEscrowExposureSummary covers P5c: a Wanax with several pending trade
// offers out had no single place to see total silver/goods locked in escrow
// — each offer only showed its own lock on its own outbox line. This is a
// regression test for the aggregate summary line outboxCmd now prints below
// the per-offer listing.
func TestEscrowExposureSummary(t *testing.T) {
	offer := func(kind, status string, extra map[string]any) map[string]any {
		o := map[string]any{"kind": kind, "status": status}
		for k, v := range extra {
			o[k] = v
		}
		return map[string]any{"trade_offer": o}
	}

	t.Run("no pending offers returns empty", func(t *testing.T) {
		msgs := []map[string]any{
			offer("buy", "accepted", map[string]any{"offer_silver": 80.0}),
			offer("sell", "declined", map[string]any{"offer_good": "copper", "offer_qty": 20.0}),
			{"message": "just chatting, no trade_offer key at all"},
		}
		if got := escrowExposureSummary(msgs); got != "" {
			t.Fatalf("expected empty summary for no pending offers, got %q", got)
		}
	})

	t.Run("aggregates silver across multiple pending buy offers", func(t *testing.T) {
		msgs := []map[string]any{
			offer("buy", "pending", map[string]any{"offer_silver": 80.0}),
			offer("buy", "pending", map[string]any{"offer_silver": 45.0}),
		}
		got := escrowExposureSummary(msgs)
		if !strings.Contains(got, "125 silver") {
			t.Errorf("expected total 125 silver, got %q", got)
		}
		if !strings.Contains(got, "2 pending offers") {
			t.Errorf("expected count of 2 pending offers, got %q", got)
		}
	})

	t.Run("aggregates goods across multiple pending sell offers, same good", func(t *testing.T) {
		msgs := []map[string]any{
			offer("sell", "pending", map[string]any{"offer_good": "copper", "offer_qty": 20.0}),
			offer("sell", "pending", map[string]any{"offer_good": "copper", "offer_qty": 15.0}),
		}
		got := escrowExposureSummary(msgs)
		if !strings.Contains(got, "35 copper") {
			t.Errorf("expected total 35 copper, got %q", got)
		}
	})

	t.Run("mixes silver and multiple distinct goods, singular offer wording", func(t *testing.T) {
		msgs := []map[string]any{
			offer("buy", "pending", map[string]any{"offer_silver": 80.0}),
			offer("sell", "pending", map[string]any{"offer_good": "wine", "offer_qty": 10.0}),
		}
		got := escrowExposureSummary(msgs)
		if !strings.Contains(got, "80 silver") || !strings.Contains(got, "10 wine") {
			t.Errorf("expected both 80 silver and 10 wine locked, got %q", got)
		}
		if !strings.Contains(got, "2 pending offers") {
			t.Errorf("expected count of 2 pending offers, got %q", got)
		}
	})

	t.Run("singular wording for exactly one pending offer", func(t *testing.T) {
		msgs := []map[string]any{
			offer("buy", "pending", map[string]any{"offer_silver": 50.0}),
		}
		got := escrowExposureSummary(msgs)
		if !strings.Contains(got, "1 pending offer") || strings.Contains(got, "1 pending offers") {
			t.Errorf("expected singular 'offer' wording, got %q", got)
		}
	})

	t.Run("expired offers do not count as pending exposure", func(t *testing.T) {
		msgs := []map[string]any{
			offer("buy", "expired", map[string]any{"offer_silver": 80.0}),
			offer("sell", "cancelled", map[string]any{"offer_good": "copper", "offer_qty": 20.0}),
		}
		if got := escrowExposureSummary(msgs); got != "" {
			t.Fatalf("expected empty summary once offers resolved, got %q", got)
		}
	})
}
