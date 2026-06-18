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
