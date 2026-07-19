package main

import (
	"strings"
	"testing"
	"time"
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

// TestDeliveryETALine covers P5 (leverans-ETA): before this, `poleia outbox`
// showed "[accepted]" for an accepted trade offer with no further detail — the
// only place goods_arrives_at/silver_arrives_at ever appeared was the one-shot
// response to `trade-accept` itself. Once that terminal scrolled away, a Wanax
// checking outbox later had no way to tell "still in transit" from "silently
// lost". TradeAccept (api/handlers/messenger.go) now persists both ETAs onto
// trade_offer at accept time; this is a regression test for the CLI formatter
// that reads them back.
func TestDeliveryETALine(t *testing.T) {
	future := func(d time.Duration) string { return time.Now().Add(d).Format(time.RFC3339) }
	past := func(d time.Duration) string { return time.Now().Add(-d).Format(time.RFC3339) }

	t.Run("no ETA fields present (older offer, or non-accepted) returns empty", func(t *testing.T) {
		if got := deliveryETALine(map[string]any{"status": "accepted"}); got != "" {
			t.Fatalf("expected empty line with no ETA fields, got %q", got)
		}
	})

	t.Run("both legs still in transit show countdowns for both", func(t *testing.T) {
		offer := map[string]any{
			"goods_arrives_at":  future(2 * time.Hour),
			"silver_arrives_at": future(4 * time.Hour),
		}
		got := deliveryETALine(offer)
		if !strings.Contains(got, "goods in") {
			t.Errorf("expected a goods countdown, got %q", got)
		}
		if !strings.Contains(got, "silver in") {
			t.Errorf("expected a silver countdown, got %q", got)
		}
	})

	t.Run("a leg already in the past reads as delivered, not a negative countdown", func(t *testing.T) {
		offer := map[string]any{
			"goods_arrives_at":  past(time.Hour), // leg 1 already landed
			"silver_arrives_at": future(time.Hour),
		}
		got := deliveryETALine(offer)
		if !strings.Contains(got, "goods delivered") {
			t.Errorf("expected past leg to read 'goods delivered', got %q", got)
		}
		if !strings.Contains(got, "silver in") {
			t.Errorf("expected future leg to still show a countdown, got %q", got)
		}
	})

	t.Run("malformed timestamp is ignored, not a crash", func(t *testing.T) {
		offer := map[string]any{"goods_arrives_at": "not-a-timestamp"}
		if got := deliveryETALine(offer); got != "" {
			t.Errorf("expected empty line for malformed timestamp, got %q", got)
		}
	})
}
