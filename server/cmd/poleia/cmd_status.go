package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show your province status (defaults to your capital; --province inspects a colony)",
		Example: `  poleia status
  poleia status --province <province-id>   # inspect a colony`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			// Default to the capital; --province lets you inspect any province you own
			// (the server FOW/ownership-gates it), mirroring `build --province`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				prov = provinceID
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, prov)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var p map[string]any
			if err := json.Unmarshal(data, &p); err != nil {
				return err
			}
			sett, _ := p["settlement"].(map[string]any)
			if sett == nil {
				fmt.Println("No settlement here.")
				return nil
			}
			name, _ := sett["name"].(string)
			culture, _ := sett["culture"].(string)
			pop, _ := sett["population"].(float64)
			labor, _ := sett["labor_pool"].(float64)
			walls, _ := sett["walls"].(float64)
			loyalty, _ := sett["loyalty"].(float64)
			coastal, _ := p["coastal"].(bool)
			coastalNote := ""
			if coastal {
				coastalNote = "  [coastal — can build harbour → ships]"
			}
			fmt.Printf("%s [%s]  Pop: %s  Labor: %s  Walls: %.0f/3  Loyalty: %.0f%s\n\n",
				name, culture, resource(pop), resource(labor), walls, loyalty, coastalNote)

			// Resources: silver lives in resources as a {amount,rate,cap} object;
			// kharis is the per-Wanax pool exposed at the settlement top level.
			fmt.Println("Resources")
			if res, ok := sett["resources"].(map[string]any); ok {
				if sd, ok := res["silver"].(map[string]any); ok {
					amt, _ := sd["amount"].(float64)
					rt, _ := sd["rate"].(float64)
					fmt.Printf("  %-8s %6s  %s\n", "Silver", resource(amt), rate(rt))
				}
			}
			kv, _ := sett["kharis"].(float64)
			kr, _ := sett["kharis_rate"].(float64)
			fmt.Printf("  %-8s %6s  %s\n", "Kharis", resource(kv), rate(kr))
			fmt.Println()

			army, _ := sett["army"].(map[string]any)
			if army != nil {
				fmt.Println("Army")
				units := []struct{ key, label string }{
					{"Spearman", "Hoplites"}, {"WarChariot", "War Chariot"}, {"Priest", "Hiereus"},
					{"Ship", "Trireme"}, {"EliteInfantry", "Agema"},
				}
				for _, u := range units {
					v, _ := army[u.key].(float64)
					if v > 0 {
						fmt.Printf("  %-10s %4.0f\n", u.label, v)
					}
				}
			}

			// Completed buildings — so the agent doesn't re-queue what already exists.
			if bs, ok := sett["buildings"].([]any); ok && len(bs) > 0 {
				fmt.Println("\nBuildings")
				for _, it := range bs {
					m, _ := it.(map[string]any)
					t, _ := m["type"].(string)
					lvl, _ := m["level"].(float64)
					fmt.Printf("  %-12s L%.0f\n", t, lvl)
				}
			}

			unitLabels := map[string]string{
				"spearman": "Hoplites", "war_chariot": "War Chariot", "priest": "Hiereus",
				"ship": "Trireme", "elite_infantry": "Agema",
			}

			if bq, ok := sett["build_queue"].([]any); ok && len(bq) > 0 {
				fmt.Println("\nConstruction")
				for _, it := range bq {
					m, _ := it.(map[string]any)
					t, _ := m["type"].(string)
					ca, _ := m["complete_at"].(string)
					fmt.Printf("  %-12s done %s\n", t, ca)
				}
			}

			if tq, ok := sett["training_queue"].([]any); ok && len(tq) > 0 {
				fmt.Println("\nTraining")
				for _, it := range tq {
					m, _ := it.(map[string]any)
					u, _ := m["unit"].(string)
					c, _ := m["count"].(float64)
					ca, _ := m["complete_at"].(string)
					label := unitLabels[u]
					if label == "" {
						label = u
					}
					fmt.Printf("  %.0f× %-10s done %s\n", c, label, ca)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}
