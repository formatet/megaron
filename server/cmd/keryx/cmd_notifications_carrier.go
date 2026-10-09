package main

import (
	"encoding/json"
	"fmt"
)

func printMessengerFateLine(n notificationItem) {
	var b struct {
		Reason   string `json:"reason"`
		HomeTick *int   `json:"home_tick"`
		Envelope struct {
			Kind                string `json:"kind"`
			SentTick            *int   `json:"sent_tick"`
			Origin, Destination struct {
				Name      string `json:"name"`
				WanaxName string `json:"wanax_name"`
				UnitName  string `json:"unit_name"`
				OwnUnit   bool   `json:"own_unit"`
				Q, R      *int
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
		home := "Home on an unknown day."
		if b.HomeTick != nil {
			home = fmt.Sprintf("Home on day %d.", *b.HomeTick)
		}
		account, landed := "", false
		for _, j := range b.Journey {
			if j.Ship != "" {
				if account == "" {
					account = "Your runner was rescued at sea by the " + j.Ship
				} else {
					account += ", then was rescued at sea by the " + j.Ship
				}
				landed = false
			}
			if j.Port != "" {
				switch {
				case account == "":
					account = "Your runner was put ashore at " + j.Port
				case !landed:
					account += " and put ashore at " + j.Port
				default:
					account += ", then went ashore at " + j.Port
				}
				landed = true
			}
		}
		if account == "" {
			account = "Your runner brings an account of its rescue at sea"
		}
		fmt.Printf("    %s %s.\n", home, account)
		return
	}
	from := "you"
	if b.Envelope.Origin.Name != "" {
		from += ", at " + b.Envelope.Origin.Name + ","
	}
	d := b.Envelope.Destination
	to := "an unknown Wanax"
	if d.WanaxName != "" {
		to = "Wanax " + d.WanaxName
	}
	if d.Name != "" {
		to += " at " + d.Name
	}
	if b.Envelope.Kind == "order" && d.OwnUnit {
		name := d.UnitName
		if name == "" {
			name = "unit"
		}
		to = "your " + name
		if d.Q != nil && d.R != nil {
			to += fmt.Sprintf(" at (%d, %d)", *d.Q, *d.R)
		}
	}
	sent := "Sent on an unknown day"
	if b.Envelope.SentTick != nil {
		sent = fmt.Sprintf("Sent on day %d", *b.Envelope.SentTick)
	}
	reason := " with no surviving rescue ship."
	if b.Reason == "storm" {
		reason = " in a storm."
	}
	fmt.Printf("    Your runner to %s was lost at sea%s\n", to, reason)
	fmt.Printf("    From %s to %s\n    %s.\n", from, to, sent)
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
