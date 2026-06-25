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
				// A delivered messenger may carry a trade offer (a buyer asking to
				// purchase a good from you). Render it from the RECEIVER's side — you
				// are the seller — with the exact commands to act on it. Without this,
				// a human player could see the message but never the offer or its id.
				if offer, ok := m["trade_offer"].(map[string]any); ok {
					good, _ := offer["want_good"].(string)
					qty, _ := offer["want_qty"].(float64)
					silver, _ := offer["offer_silver"].(float64)
					fmt.Printf("From: %s  (%s)\n", from, when)
					if text != "" {
						fmt.Printf("  \"%s\"\n", text)
					}
					fmt.Printf("  ⇄ TRADE OFFER — they want to buy %.0f %s, paying %.0f silver\n", qty, good, silver)
					fmt.Printf("    → you SELL %.0f %s and receive %.0f silver\n", qty, good, silver)
					fmt.Printf("    accept:  poleia trade-accept --id %s\n", id)
					fmt.Printf("    decline: poleia trade-decline --id %s\n\n", id)
					continue
				}
				fmt.Printf("From: %s  (%s)\n  \"%s\"\n    reply: poleia reply --id %s --text \"...\"\n\n", from, when, text, id)
			}
			return nil
		},
	}
}
