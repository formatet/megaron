package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func diplomacyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diplomacy",
		Short: "List known and rumour-known Wanaxes (ruler-centric view of `cities`)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/diplomacy", cfg.WorldID))
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
			if len(entries) == 0 {
				fmt.Println("No other Wanaxes known yet — expand your vision or wait for gossip to reach you.")
				return nil
			}
			fmt.Printf("%-18s %-14s %-6s %-7s\n", "Wanax", "Kingdom", "Known", "Rumour")
			fmt.Println("──────────────────────────────────────────────")
			for _, e := range entries {
				owner, _ := e["owner"].(string)
				kingdom, _ := e["kingdom"].(string)
				known, _ := e["known_cities"].(float64)
				rumour, _ := e["rumour_cities"].(float64)
				rumourOnly, _ := e["rumour_only"].(bool)
				label := owner
				if rumourOnly {
					label += " (rumour only)"
				}
				fmt.Printf("%-18s %-14s %-6.0f %-7.0f\n", label, kingdom, known, rumour)
			}
			return nil
		},
	}
}
