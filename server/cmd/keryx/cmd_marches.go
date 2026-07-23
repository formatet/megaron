package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func inboxCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inbox",
		Short: "Show messenger inbox",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/inbox", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var msgs []map[string]any
			if err := json.Unmarshal(data, &msgs); err != nil {
				return err
			}
			if len(msgs) == 0 {
				fmt.Println("Inbox empty.")
				return nil
			}
			for _, m := range msgs {
				id, _ := m["id"].(string)
				from, _ := m["from_name"].(string)
				text, _ := m["message"].(string)
				arrivedStr, _ := m["arrived_at"].(string)
				var when string
				if t, err := time.Parse(time.RFC3339, arrivedStr); err == nil {
					ago := time.Since(t)
					if ago < time.Hour {
						when = fmt.Sprintf("%dm ago", int(ago.Minutes()))
					} else if ago < 24*time.Hour {
						when = fmt.Sprintf("%dh ago", int(ago.Hours()))
					} else {
						when = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
					}
				}
				// A delivered messenger may carry a trade offer — either a buy offer
				// (sender wants a good from you, offers silver) or a sell offer
				// (sender offers a good, wants silver from you). Render it from the
				// RECEIVER's side with the exact commands to act on it. Without this,
				// a human player could see the message but never the offer or its id.
				if offer, ok := m["trade_offer"].(map[string]any); ok {
					kind, _ := offer["kind"].(string)
					fmt.Printf("From: %s  (%s)\n", from, when)
					if text != "" {
						fmt.Printf("  \"%s\"\n", text)
					}
					if kind == "sell" {
						good, _ := offer["offer_good"].(string)
						qty, _ := offer["offer_qty"].(float64)
						silver, _ := offer["want_silver"].(float64)
						fmt.Printf("  ⇄ TRADE OFFER — they want to sell %.0f %s, asking %.0f silver\n", qty, good, silver)
						fmt.Printf("    → you BUY %.0f %s and pay %.0f silver\n", qty, good, silver)
					} else {
						good, _ := offer["want_good"].(string)
						qty, _ := offer["want_qty"].(float64)
						silver, _ := offer["offer_silver"].(float64)
						fmt.Printf("  ⇄ TRADE OFFER — they want to buy %.0f %s, paying %.0f silver\n", qty, good, silver)
						fmt.Printf("    → you SELL %.0f %s and receive %.0f silver\n", qty, good, silver)
					}
					// Fas 2a: an offer you can't currently afford still shows up here
					// (it used to vanish from the inbox entirely, leaving no way to
					// find its id to decline it) — affordable==false says so plainly.
					if afford, ok := m["affordable"].(bool); ok && !afford {
						fmt.Println("    you can't yet afford this — wait for silver/goods to accrue, or decline it")
					}
					fmt.Printf("    accept:  keryx trade-accept --id %s\n", id)
					fmt.Printf("    decline: keryx trade-decline --id %s\n", id)
					if offerStatus, _ := offer["status"].(string); offerStatus == "pending" {
						if expStr, ok := m["expires_at"].(string); ok {
							if expT, err := time.Parse(time.RFC3339, expStr); err == nil {
								fmt.Printf("    expires in %s (escrow refunds if left unanswered)\n", countdown(expT))
							}
						}
					}
					fmt.Println()
					continue
				}
				fmt.Printf("From: %s  (%s)\n  \"%s\"\n    reply: keryx reply --id %s --text \"...\"\n\n", from, when, text, id)
			}
			return nil
		},
	}
}
