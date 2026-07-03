package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// notificationsCmd surfaces the persistent notifications feed (server since
// mig 045/06-10) — previously invisible in keryx entirely: arrivals, colony
// foundings, build/train completions, trade events etc. fired server-side
// with nowhere to see them in the CLI (Fas 2h/keryx-surface rule: everything
// in temenos must be visible AND actionable in keryx).
func notificationsCmd() *cobra.Command {
	var unreadOnly, markRead bool
	cmd := &cobra.Command{
		Use:     "notifications",
		Short:   "Show your notification feed (arrivals, completions, trade events, ...)",
		Example: `  poleia notifications
  poleia notifications --unread
  poleia notifications --mark-read`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			if markRead {
				if _, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/notifications/read-all", cfg.WorldID), nil); err != nil {
					return err
				}
				fmt.Println("All notifications marked read.")
				return nil
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/notifications", cfg.WorldID)
			if unreadOnly {
				path += "?unread=true"
			}
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Notifications []struct {
					ID        string          `json:"id"`
					Kind      string          `json:"kind"`
					Level     int             `json:"level"`
					Body      json.RawMessage `json:"body"`
					CreatedAt string          `json:"created_at"`
					ReadAt    *string         `json:"read_at"`
				} `json:"notifications"`
				Unread int `json:"unread"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if len(resp.Notifications) == 0 {
				fmt.Println("No notifications.")
				return nil
			}
			fmt.Printf("%d notification(s), %d unread\n", len(resp.Notifications), resp.Unread)
			fmt.Println("────────────────────────────────────────────────────────────")
			for _, n := range resp.Notifications {
				marker := " "
				if n.ReadAt == nil {
					marker = "*"
				}
				var when string
				if t, err := time.Parse(time.RFC3339, n.CreatedAt); err == nil {
					ago := time.Since(t)
					switch {
					case ago < time.Hour:
						when = fmt.Sprintf("%dm ago", int(ago.Minutes()))
					case ago < 24*time.Hour:
						when = fmt.Sprintf("%dh ago", int(ago.Hours()))
					default:
						when = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
					}
				}
				fmt.Printf("%s[%s]  %-20s  %s\n", marker, when, n.Kind, string(n.Body))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&unreadOnly, "unread", false, "show only unread notifications")
	cmd.Flags().BoolVar(&markRead, "mark-read", false, "mark all notifications as read and exit")
	return cmd
}
