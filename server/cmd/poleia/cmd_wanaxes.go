package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func wanaxesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wanaxes",
		Short: "List all Wanaxes in the world (trade-discovery directory)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/wanaxes", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var entries []map[string]any
			if err := json.Unmarshal(data, &entries); err != nil {
				return err
			}
			fmt.Printf("%-22s %-8s %-10s %-12s  %-7s  %s\n",
				"Name", "Terrain", "Culture", "Kingdom", "Deposit", "Settlement ID")
			fmt.Println("──────────────────────────────────────────────────────────────────────────")
			for _, e := range entries {
				name, _ := e["name"].(string)
				terrain, _ := e["terrain"].(string)
				culture, _ := e["culture"].(string)
				kingdom, _ := e["kingdom"].(string)
				sid, _ := e["settlement_id"].(string)
				own, _ := e["own"].(bool)
				copper, _ := e["copper_deposit"].(bool)
				tin, _ := e["tin_deposit"].(bool)
				silver, _ := e["silver_deposit"].(bool)
				marker := " "
				if own {
					marker = "★"
				}
				deposit := "—"
				if silver {
					deposit = "⛏silver"
				} else if copper {
					deposit = "⛏copper"
				} else if tin {
					deposit = "⛏tin"
				}
				fmt.Printf("%s%-21s %-8s %-10s %-12s  %-7s  %s\n",
					marker, name, terrain, culture, kingdom, deposit, sid)
			}
			return nil
		},
	}
}
