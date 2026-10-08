package main

import (
	"encoding/json"
	"fmt"
)

func printMessengerFateLine(n notificationItem) {
	var b struct {
		Reason   string `json:"reason"`
		Envelope struct {
			Sent                string `json:"sent_at"`
			Origin, Destination struct {
				Name string `json:"name"`
				Q, R *int
			}
			Message string          `json:"message_text"`
			Reply   *string         `json:"reply_text"`
			Offer   json.RawMessage `json:"trade_offer"`
			Order   json.RawMessage `json:"order_payload"`
		} `json:"envelope"`
		Journey []struct {
			Ship string `json:"ship"`
			Port string `json:"port"`
		} `json:"journey"`
	}
	if json.Unmarshal(n.Body, &b) != nil {
		return
	}
	if n.Kind == "MessengerRescuedAtSea" {
		fmt.Println("    Your runner is home with its account of rescue at sea.")
		for _, j := range b.Journey {
			if j.Ship != "" {
				fmt.Printf("    Rescued aboard %s.\n", j.Ship)
			}
			if j.Port != "" {
				fmt.Printf("    Ashore at %s.\n", j.Port)
			}
		}
		return
	}
	endpoint := func(name string, q, r *int) string {
		if name != "" {
			return name
		}
		if q != nil && r != nil {
			return fmt.Sprintf("(%d,%d)", *q, *r)
		}
		return "unknown"
	}
	fmt.Printf("    Your runner was lost at sea. Sent: %s. From: %s. To: %s.\n", b.Envelope.Sent, endpoint(b.Envelope.Origin.Name, b.Envelope.Origin.Q, b.Envelope.Origin.R), endpoint(b.Envelope.Destination.Name, b.Envelope.Destination.Q, b.Envelope.Destination.R))
	fmt.Printf("    Letter:\n%s\n", b.Envelope.Message)
	if b.Envelope.Reply != nil {
		fmt.Printf("    Reply:\n%s\n", *b.Envelope.Reply)
	}
	for _, part := range []struct {
		label string
		raw   json.RawMessage
	}{{"Trade offer", b.Envelope.Offer}, {"Order", b.Envelope.Order}} {
		if len(part.raw) > 0 && string(part.raw) != "null" {
			fmt.Printf("    %s:\n%s\n", part.label, part.raw)
		}
	}
}
