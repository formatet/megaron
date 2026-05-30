package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func marchesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "marches",
		Short: "Show active army movements",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/marches", cfg.WorldID, cfg.ProvinceID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var marches []map[string]any
			if err := json.Unmarshal(data, &marches); err != nil {
				return err
			}
			active := make([]map[string]any, 0)
			for _, m := range marches {
				if resolved, _ := m["resolved"].(bool); !resolved {
					active = append(active, m)
				}
			}
			if len(active) == 0 {
				fmt.Println("No active marches.")
				return nil
			}
			fmt.Printf("%-10s  %-10s  %-10s  %s\n", "Intent", "Direction", "ETA", "Report")
			fmt.Println("──────────────────────────────────────────────────────────────")
			now := time.Now()
			for _, m := range active {
				intent, _ := m["intent"].(string)
				outgoing, _ := m["outgoing"].(bool)
				dir := "incoming"
				if outgoing {
					dir = "outgoing"
				}
				arrivesStr, _ := m["arrives_at"].(string)
				var eta string
				if t, err := time.Parse(time.RFC3339, arrivesStr); err == nil {
					remaining := t.Sub(now)
					if remaining < 0 {
						eta = "arrived"
					} else if remaining < time.Hour {
						eta = fmt.Sprintf("%dm", int(remaining.Minutes()))
					} else {
						eta = fmt.Sprintf("%dh%dm", int(remaining.Hours()), int(remaining.Minutes())%60)
					}
				}
				report, _ := m["combat_report"].(string)
				if len(report) > 30 {
					report = report[:27] + "…"
				}
				fmt.Printf("%-10s  %-10s  %-10s  %s\n", intent, dir, eta, report)
			}
			return nil
		},
	}
}

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
				fmt.Printf("From: %s  (%s)\n  \"%s\"\n\n", from, when, text)
			}
			return nil
		},
	}
}
