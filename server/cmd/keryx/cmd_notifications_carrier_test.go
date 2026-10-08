package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessengerFate_ActualArchivedPayloads(t *testing.T) {
	for _, tc := range []struct {
		kind, file string
		terms      []string
	}{{"MessengerLostAtSea", "messenger_lost_trade.json", []string{"bronze", "145.7", "Sent:", "From:", "To:"}}, {"MessengerLostAtSea", "messenger_lost_order.json", []string{"march", "hold_to_last_man", "Sent:", "From:", "To:"}}, {"MessengerLostAtSea", "messenger_lost_at_sea.json", []string{"Your runner was lost", "Sent:", "From:", "To:", "hello"}}, {"MessengerRescuedAtSea", "messenger_rescued_at_sea.json", []string{"home", "Sacred Dolphin", "Ashore at"}}} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "static", "js", "megaron", "ui", "testdata", tc.file))
		if err != nil {
			t.Fatal(err)
		}
		out, _ := captureStdout(t, func() error { printNotificationDetail(nil, notificationItem{Kind: tc.kind, Body: raw}); return nil })
		for _, term := range tc.terms {
			if !strings.Contains(out, term) {
				t.Fatalf("%s missing %q: %s", tc.kind, term, out)
			}
		}
	}
}

func TestMessengerLoss_WholeTradeAndOrderTerms(t *testing.T) {
	letter := strings.Repeat("sealed long letter ", 200)
	raw, _ := json.Marshal(map[string]any{"envelope": map[string]any{"message_text": letter, "trade_offer": map[string]any{"offer_good": "bronze", "want_silver": 145.7}, "order_payload": map[string]any{"verb": "march", "unit_id": "physical-unit", "standing_orders": map[string]any{"hold_to_last_man": true}}}})
	out, _ := captureStdout(t, func() error {
		printNotificationDetail(nil, notificationItem{Kind: "MessengerLostAtSea", Body: raw})
		return nil
	})
	for _, term := range []string{letter, "bronze", "145.7", "march", "physical-unit", "hold_to_last_man"} {
		if !strings.Contains(out, term) {
			t.Fatalf("whole sealed terms missing %q", term)
		}
	}
}
