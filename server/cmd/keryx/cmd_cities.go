package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func citiesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "cities",
		Aliases: []string{"wanaxes"}, // kept working for one version — see temenos_gossip.md PASS 2b
		Short:   "List known and rumour-known settlements — city-centric (see `diplomacy` for the ruler-centric view)",
		Long: `List known and rumour-known settlements.

"known" rows (seen, remembered, or contacted) show exact terrain/deposit/position
and are safe to trade or send a messenger to.

"rumour" rows were only heard OF through gossip — a fuzzy bearing and a coarse
industry hint, never exact coordinates. They are NOT contactable yet: explore
there (march/colonize) to turn a rumour into a real contact.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/cities", cfg.WorldID))
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
			fmt.Printf("%-22s %-16s %-8s %-10s %-12s  %-7s  %s\n",
				"Name", "Owner", "Terrain", "Culture", "Kingdom", "Deposit", "Settlement ID")
			fmt.Println("────────────────────────────────────────────────────────────────────────────────────")
			var rumours []map[string]any
			for _, e := range entries {
				if knowledge, _ := e["knowledge"].(string); knowledge == "rumour" {
					rumours = append(rumours, e)
					continue
				}
				name, _ := e["name"].(string)
				owner, _ := e["owner"].(string)
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
				if owner == "" {
					owner = "—"
				}
				deposit := "—"
				if silver {
					deposit = "⛏silver"
				} else if copper {
					deposit = "⛏copper"
				} else if tin {
					deposit = "⛏tin"
				}
				fmt.Printf("%s%-21s %-16s %-8s %-10s %-12s  %-7s  %s\n",
					marker, name, owner, terrain, culture, kingdom, deposit, sid)
			}
			if len(entries)-len(rumours) <= 1 {
				fmt.Println("\nNo other settlements within your vision — this directory is FOW-gated, not global.")
				fmt.Println("Trade needs a visible neighbour: send a unit outward (`march`) or colonise to")
				fmt.Println("expand your vision and discover Wanaxes you can trade with.")
			}
			if len(rumours) > 0 {
				fmt.Println("\nRUMOUR-KNOWN (heard of, not yet contactable — explore to confirm):")
				for _, e := range rumours {
					name, _ := e["name"].(string)
					bearing, _ := e["bearing"].(string)
					hint, _ := e["industry_hint"].(string)
					if hint != "" {
						fmt.Printf("  %s — %s, rich in %s\n", name, bearing, hint)
					} else {
						fmt.Printf("  %s — %s\n", name, bearing)
					}
				}
			}
			return nil
		},
	}
}
