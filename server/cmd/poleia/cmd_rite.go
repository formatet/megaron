package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func riteCmd() *cobra.Command {
	var settlementID string
	var prayerID string
	var targetID string

	cmd := &cobra.Command{
		Use:   "rite",
		Short: "Perform a cultural prayer at your settlement (costs 5 grain, requires temple)",
		Example: `  poleia rite
  poleia rite --prayer akhaier_oracle_deposits
  poleia rite --prayer akhaier_battle_frenzy --settlement <settlement-uuid>
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

			// Build request body. Empty fields are omitted so the server applies defaults.
			body := map[string]any{}
			if prayerID != "" {
				body["prayer"] = prayerID
			}
			if targetID != "" {
				body["target"] = targetID
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/rite", cfg.WorldID, settlementID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp struct {
				Success    bool           `json:"success"`
				Mood       string         `json:"mood"`
				Chance     int            `json:"chance"`
				Prayer     string         `json:"prayer"`
				EffectType string         `json:"effect_type"`
				Effect     map[string]any `json:"effect"`
				Message    string         `json:"message"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Printf("Divine mood: %s (%d%% chance)\n", resp.Mood, resp.Chance)
			if resp.Prayer != "" {
				fmt.Printf("Prayer: %s\n", resp.Prayer)
			}
			if resp.Success {
				fmt.Printf("Success: %s\n", resp.Message)
				if resp.EffectType != "" {
					fmt.Printf("Effect: %s\n", resp.EffectType)
				}
			} else {
				fmt.Printf("Failed: %s\n", resp.Message)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&settlementID, "settlement", "", "settlement UUID (defaults to your capital)")
	cmd.Flags().StringVar(&prayerID, "prayer", "", "prayer ID (e.g. akhaier_oracle_deposits; defaults to culture battle_frenzy)")
	cmd.Flags().StringVar(&targetID, "target", "", "target province UUID (for future targeted prayers)")
	return cmd
}
