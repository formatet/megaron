package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func gossipCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gossip",
		Short: "Show recent rumours and events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/gossip", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var items []map[string]any
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No rumours yet.")
				return nil
			}
			for _, g := range items {
				region, _ := g["source_region"].(string)
				text, _ := g["text"].(string)
				tsStr, _ := g["generated_at"].(string)
				var when string
				if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
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
				fmt.Printf("[%s]  %s\n  %s\n\n", when, region, text)
			}
			return nil
		},
	}
}

func messengerCmd() *cobra.Command {
	var destName, message string
	cmd := &cobra.Command{
		Use:     "messenger",
		Short:   "Send a messenger to another settlement",
		Example: `  poleia messenger --to Korinth --message "Greetings, shall we trade?"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Load visible provinces to resolve own settlement ID and destination.
			provinces, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			if err := json.Unmarshal(provinces, &markers); err != nil {
				return err
			}
			var destID, destSettleName, ownSettlementID string
			for _, m := range markers {
				if own, _ := m["own"].(bool); own {
					ownSettlementID, _ = m["settlement_id"].(string)
				}
				n, _ := m["name"].(string)
				if n == destName {
					destID, _ = m["settlement_id"].(string)
					destSettleName = n
				}
			}
			if ownSettlementID == "" {
				return fmt.Errorf("could not find own settlement")
			}
			if destID == "" {
				return fmt.Errorf("no visible settlement named %q", destName)
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			data, err := c.post(path, map[string]any{
				"destination_id": destID,
				"message":        message,
			})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			arrivesAt, _ := resp["arrives_at"].(string)
			fmt.Printf("Messenger dispatched to %s · arrives %s\n", destSettleName, arrivesAt)
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "message text (required)")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}
