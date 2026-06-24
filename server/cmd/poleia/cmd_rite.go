package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func riteCmd() *cobra.Command {
	var settlementID string
	var prayerID string
	var targetID string
	var list bool

	cmd := &cobra.Command{
		Use:   "rite",
		Short: "Perform a cultural prayer at your settlement (requires temple + offering)",
		Example: `  poleia rite --list
  poleia rite --prayer <prayer-id>
  poleia rite --prayer <prayer-id> --settlement <settlement-uuid>
  poleia rite --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// --list: show this culture's available prayers (id + affordability) from
			// the province status endpoint, which exposes `available_prayers`. Works for
			// any culture — no hardcoded akhaier_* IDs.
			if list {
				data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, cfg.ProvinceID))
				if err != nil {
					return err
				}
				if jsonMode {
					printRawJSON(data)
					return nil
				}
				var p struct {
					Settlement struct {
						AvailablePrayers []struct {
							ID         string             `json:"id"`
							Name       string             `json:"name"`
							God        string             `json:"god"`
							MinKharis  float64            `json:"min_kharis"`
							Offering   map[string]float64 `json:"offering"`
							Affordable bool               `json:"affordable"`
						} `json:"available_prayers"`
					} `json:"settlement"`
				}
				if err := json.Unmarshal(data, &p); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				if len(p.Settlement.AvailablePrayers) == 0 {
					fmt.Println("No prayers available (no settlement here, or none for this culture).")
					return nil
				}
				fmt.Printf("%-28s  %-20s  %-8s  %-22s  %s\n", "Prayer ID", "Name", "MinKhar", "Offering", "Affordable")
				for _, pr := range p.Settlement.AvailablePrayers {
					afford := "no"
					if pr.Affordable {
						afford = "yes"
					}
					// Render the material offering (the real blocker when kharis is met),
					// e.g. "grain×25 oil×15", sorted for stable output.
					keys := make([]string, 0, len(pr.Offering))
					for g := range pr.Offering {
						keys = append(keys, g)
					}
					sort.Strings(keys)
					offer := ""
					for _, g := range keys {
						offer += fmt.Sprintf("%s×%.0f ", g, pr.Offering[g])
					}
					if offer == "" {
						offer = "—"
					}
					fmt.Printf("%-28s  %-20s  %-8.0f  %-22s  %s\n", pr.ID, pr.Name, pr.MinKharis, strings.TrimSpace(offer), afford)
				}
				return nil
			}

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

	cmd.Flags().BoolVar(&list, "list", false, "list this culture's available prayers (id + affordability) and exit")
	cmd.Flags().StringVar(&settlementID, "settlement", "", "settlement UUID (defaults to your capital)")
	cmd.Flags().StringVar(&prayerID, "prayer", "", "prayer ID (run --list to see your culture's prayers; defaults to culture battle_frenzy)")
	cmd.Flags().StringVar(&targetID, "target", "", "target province UUID (for future targeted prayers)")
	return cmd
}
