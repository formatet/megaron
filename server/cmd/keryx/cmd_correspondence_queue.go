package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

type correspondenceItem struct {
	ID                string     `json:"id"`
	FromName          string     `json:"from_name"`
	ToName            string     `json:"to_name"`
	Message           string     `json:"message"`
	ReplyText         *string    `json:"reply_text"`
	Group             string     `json:"queue_group"`
	OfferState        string     `json:"offer_state"`
	Actions           []string   `json:"actions"`
	MissingPayment    string     `json:"missing_payment"`
	ReplyBefore       *time.Time `json:"reply_before"`
	ReplyDeadlineTick *int       `json:"reply_deadline_tick"`
	ReplyWindowClosed bool       `json:"reply_window_closed"`
	ExpiresAt         *time.Time `json:"expires_at"`
	TradeOffer        *struct {
		Kind        string  `json:"kind"`
		OfferGood   string  `json:"offer_good"`
		OfferQty    float64 `json:"offer_qty"`
		WantGood    string  `json:"want_good"`
		WantQty     float64 `json:"want_qty"`
		OfferSilver float64 `json:"offer_silver"`
		WantSilver  float64 `json:"want_silver"`
	} `json:"trade_offer"`
}

func showCorrespondenceQueue(limit int, cursor string) error {
	q := url.Values{"limit": {fmt.Sprint(limit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	c := newClient(cfg)
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/messengers/queue?%s", cfg.WorldID, q.Encode()))
	if err != nil {
		return err
	}
	var page struct {
		Items      []correspondenceItem `json:"items"`
		NextCursor string               `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}
	if jsonMode {
		printRawJSON(data)
		return nil
	}
	fmt.Println("Incoming correspondence, newest first. Groups apply to this page; older unanswered letters may be on the next page.")
	for _, group := range []struct{ key, title string }{{"waiting_you", "Waiting on you"}, {"completed", "Past correspondence"}} {
		heading := false
		for _, m := range page.Items {
			if m.Group != group.key {
				continue
			}
			if !heading {
				fmt.Printf("\n%s\n", group.title)
				heading = true
			}
			state := map[string]string{"pending": "Awaiting your answer", "expired": "Expired without answer", "accepted": "Accepted", "declined": "Declined", "cancelled": "Cancelled"}[m.OfferState]
			if state == "" {
				if m.Group == "waiting_you" {
					state = "Awaiting your reply"
				} else {
					state = "No reply pending"
				}
			}
			if m.TradeOffer == nil && m.ReplyText != nil && *m.ReplyText != "" {
				state = "Reply dispatched"
			} else if m.ReplyWindowClosed {
				state = "Return post closed"
			}
			fmt.Printf("From: %s → %s · %s\n  %s\n", m.FromName, m.ToName, state, m.Message)
			if o := m.TradeOffer; o != nil {
				if o.Kind == "sell" {
					fmt.Printf("  Buy %g %s · pay %g silver\n", o.OfferQty, o.OfferGood, o.WantSilver)
				} else {
					fmt.Printf("  Sell %g %s · receive %g silver\n", o.WantQty, o.WantGood, o.OfferSilver)
				}
			}
			if m.ExpiresAt != nil {
				fmt.Printf("  Answer deadline: %s\n", clockTime(*m.ExpiresAt))
			}
			if m.ReplyDeadlineTick != nil {
				fmt.Printf("  Return post before %s\n", formatWorldTime(c, *m.ReplyDeadlineTick))
			} else if m.ReplyBefore != nil {
				fmt.Printf("  Return post before %s\n", clockTime(*m.ReplyBefore))
			}
			if m.MissingPayment != "" {
				fmt.Printf("  %s. You can still decline.\n", m.MissingPayment)
			}
			if m.ReplyText != nil && *m.ReplyText != "" {
				fmt.Printf("  Your reply: %s\n", *m.ReplyText)
			}
			for _, action := range m.Actions {
				switch action {
				case "reply":
					if m.ReplyWindowClosed {
						continue
					}
					if m.ReplyDeadlineTick == nil && m.ReplyBefore == nil {
						fmt.Println("  Return post deadline unavailable")
					}
					fmt.Printf("  reply: keryx reply --id %s --text \"...\"\n", m.ID)
				case "trade_accept":
					fmt.Printf("  accept: keryx trade-accept --id %s\n", m.ID)
				case "trade_decline":
					fmt.Printf("  decline: keryx trade-decline --id %s\n", m.ID)
				}
			}
		}
	}
	if len(page.Items) == 0 {
		fmt.Println("No received correspondence on this page.")
	}
	if page.NextCursor != "" {
		fmt.Printf("\nOlder correspondence: keryx inbox --queue --limit %d --cursor %q\n", limit, page.NextCursor)
	}
	return nil
}
