package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func riteCmd() *cobra.Command {
	var settlementID string

	cmd := &cobra.Command{
		Use:   "rite",
		Short: "Perform a divine rite at your settlement (costs 5 grain, requires temple)",
		Example: `  poleia rite
  poleia rite --settlement <settlement-uuid>
  poleia rite --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Resolve own capital settlement if --settlement not given.
			if settlementID == "" {
				provs, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
				if err != nil {
					return err
				}
				var markers []map[string]any
				_ = json.Unmarshal(provs, &markers)
				for _, m := range markers {
					if own, _ := m["own"].(bool); own {
						if isCapital, _ := m["is_capital"].(bool); isCapital {
							settlementID, _ = m["settlement_id"].(string)
							break
						}
					}
				}
				// Fallback: any own settlement.
				if settlementID == "" {
					for _, m := range markers {
						if own, _ := m["own"].(bool); own {
							settlementID, _ = m["settlement_id"].(string)
							break
						}
					}
				}
			}
			if settlementID == "" {
				return fmt.Errorf("could not find own settlement — use --settlement <id>")
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/rite", cfg.WorldID, settlementID)
			data, err := c.post(path, nil)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp struct {
				Success   bool       `json:"success"`
				Mood      string     `json:"mood"`
				Chance    int        `json:"chance"`
				Effect    string     `json:"effect"`
				ExpiresAt *time.Time `json:"expires_at"`
				Message   string     `json:"message"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Printf("Divine mood: %s (%d%% chance)\n", resp.Mood, resp.Chance)
			if resp.Success {
				fmt.Printf("✓ %s\n", resp.Message)
				if resp.ExpiresAt != nil {
					fmt.Printf("  Battle frenzy active until %s\n", resp.ExpiresAt.Local().Format("15:04 Jan 2"))
				}
			} else {
				fmt.Printf("✗ %s\n", resp.Message)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&settlementID, "settlement", "", "settlement UUID (defaults to your capital)")
	return cmd
}
